# Architecture

This document explains the design of **code-review-agent** and the trade-offs
behind the key decisions. It is written for reviewers and interviewers who want
to understand *why* the system is built the way it is.

## Overview

code-review-agent is a GitHub App that automatically reviews pull requests. It is
a thin orchestration layer in Go that delegates the actual "thinking" to an
external **agent-go cognition** service over gRPC.

```
GitHub webhook ──▶ Go service ──▶ agent-go cognition (gRPC)
                     │                    │
                     ▼                    └─ LLM (deepseek / planner model)
              SQLite + SSE + Prometheus
```

## Components

| Component | Path | Responsibility |
|-----------|------|----------------|
| `webhook` | `internal/webhook` | HMAC signature verification, event parsing |
| `diff` | `internal/diff` | Parse GitHub unified diff into structured files |
| `reviewer` | `internal/reviewer` | Orchestrate diff → chunk → cognition → post |
| `cognition` | `internal/cognition` | gRPC client to agent-go, circuit breaker |
| `prompt` | `internal/prompt` | Build the review prompt, truncation |
| `review` | `internal/review` | Parse cognition output, merge, post to GitHub |
| `github` | `internal/github` | GitHub REST client (diff, reviews, comments) |
| `store` | `internal/store` | SQLite history + audit log |
| `sse` | `internal/sse` | Real-time progress broadcast to the UI |
| `metrics` | `internal/metrics` | Prometheus metrics |
| `otel` | `internal/otel` | OpenTelemetry tracing + trace-id logging |
| `eval` | `eval` | Evaluation harness (precision/recall/F1) |

## Review pipeline

```
webhook event
  └─▶ extract owner/repo/pr/head_sha
        └─▶ fetch unified diff (GitHub)
              └─▶ parse into []FileDiff
                    └─▶ if total lines > threshold: split into chunks (800 lines)
                          └─▶ for each chunk: call cognition
                                └─▶ parse JSON output
                                      └─▶ merge all chunks into one ReviewOutput
                                            └─▶ post review + comments (GitHub)
```

Each review record is persisted to SQLite and its progress is broadcast over SSE
so the dashboard reflects real state.

## Key design decisions & trade-offs

### 1. Chunked review vs single-pass

Large PRs exceed the LLM context window. Instead of truncating (which silently
drops code), the diff is split into ~800-line chunks and reviewed independently.

- **Trade-off**: chunking loses *cross-file* context (a bug that spans two chunks
  may be missed or reported twice). Accepted because correctness within a chunk
  matters more than a perfect global view, and GitHub comments are line-scoped.

### 2. Model routing via agent-go roles

agent-go's cognition routes requests by **role**, not per-request model
selection. code-review-agent maps two strategies onto those roles:

- `react` (small PR) → `executor` role → cheap/fast model (deepseek)
- `plan_execute` (large PR) → `planner` role → strong model

This gives cost/quality control without forking the LLM stack.

### 3. Circuit breaker + retry on the gRPC boundary

The cognition client uses a circuit breaker so a flaky cognition service fails
fast instead of cascading timeouts. GitHub HTTP calls have bounded retries with
`Retry-After` backoff; request bodies are rewound on retry so POSTs never send
empty payloads.

### 4. Structured JSON output from the LLM

The prompt demands a strict JSON schema and includes a one-shot example. Output
is parsed defensively and merged across chunks. This keeps evaluation measurable
(precision/recall) rather than free-text.

### 5. Observability

- **Tracing**: OpenTelemetry SDK + OTLP gRPC exporter, propagated across HTTP and
  gRPC via W3C TraceContext. End-to-end spans require enabling OTel in agent-go
  (`COGNITION_OTEL_ENABLED=true` + collector endpoint).
- **Metrics**: Prometheus text format at `/metrics` (review counts, latency
  percentiles, error rates).
- **Logs**: structured `slog` with automatic trace-id injection.

### 6. SSE for real-time progress

Progress is pushed to the browser via Server-Sent Events. The hub supports
multiple subscribers per session (e.g. multiple tabs) and drops events for slow
consumers rather than blocking the review pipeline.

## Evaluation methodology

`eval/` contains a corpus of annotated PR diffs and their expected issues.
The runner computes precision, recall, and F1 per case and in aggregate.

- **Mock mode** (`go run ./cmd/eval/`): deterministic keyword-based reviewer,
  used to validate the harness itself (matching, metrics, no double-counting).
- **Real mode** (`go run ./cmd/eval/ -real`): calls agent-go cognition over gRPC
  to measure actual model quality.
- **Multi-bug cases**: recent corpus entries contain several bugs plus
  non-issues to prevent recall from being gamed by single-obvious-bug cases.
