# Code Review Agent

[![CI](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/LingMi1/code-review-agent/branch/main/graph/badge.svg)](https://codecov.io/gh/LingMi1/code-review-agent)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.6-3178C6?logo=typescript)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> [简体中文](README.zh-CN.md)

An AI code review tool that catches bugs, security holes, and performance problems in pull requests — with all of its "thinking" delegated to [agent-go](https://github.com/LingMi1/agent-go), a production-grade multi-agent platform. This repo ships no LLM SDK of its own.

## Screenshots

| Review Dashboard | Review Detail |
|---|---|
| <img src="assets/review-list.png" width="480" alt="Review list" /> | <img src="assets/review-detail.png" width="480" alt="Review detail" /> |

## How It Works

```
GitHub PR webhook / manual trigger
      │
      ▼
┌─────────────────────────────────┐
│ code-review-agent (Go)          │
│  • HMAC-SHA256 webhook verify   │
│  • Fetch PR diff                 │
│  • Parse & chunk by file         │
│  • Smart model routing           │
│  • Build review prompt           │
└──────────┬──────────────────────┘
           │ gRPC (CognitionService.Run, server-streaming)
           ▼
┌─────────────────────────────────┐
│ agent-go cognition (Python)     │
│  • ReAct / Plan-Execute Agent   │
│  • Multi-agent collaboration    │
│  • Outputs structured JSON      │
└──────────┬──────────────────────┘
           │
           ▼
┌─────────────────────────────────┐
│ GitHub PR Review                 │
│  • Inline comments on code      │
│  • Severity: 🔴 high 🟡 med 🟢 low │
└─────────────────────────────────┘
```

## What Makes This Different

Most AI code review tools are little more than a thin wrapper around an LLM API: build a prompt, send it to OpenAI or Claude, paste the reply back onto the PR. This project stands apart in three ways:

- **No LLM SDK — cognition is a separate platform**: every review is handled by [agent-go](https://github.com/LingMi1/agent-go), a production multi-agent platform with ReAct / Plan-Execute, tool calling, and structured JSON output. This repo only builds prompts and parses results over gRPC — the exact split you'd own in an "agent application on top of an agent platform" role.
- **Measured, not vibes**: a 30-case labeled corpus (security / bug / performance / style, plus negative and multi-file cases) is scored against a real LLM (DeepSeek) and published with honest P/R/F1 — including where the LLM falls short (precision 0.50, false positives on negative cases). Both mock and real numbers are reproducible.
- **Production engineering, not a demo**: HMAC webhook verification, rate limiting, panic recovery, graceful degradation, de-duplication, OTel tracing, Prometheus metrics, e2e tests, and CI (test/lint/vet/eval/web build) — the parts a "script-style" code review usually skips.

## Features

### Core Pipeline

- **Real PR Processing**: Handles `opened`, `synchronize`, and `reopened` PR events
- **HMAC-SHA256 Webhook Verification**: Prevents forged requests
- **Diff Chunking**: Large PRs split by file into ~28KB chunks (large files split at hunk boundaries), reviewed independently and merged
- **Structured JSON Output**: LLM returns machine-parseable review with file/line/severity/category/suggestion
- **Graceful Degradation**: Single-shot reviews fall back to a plain-text comment when JSON parsing fails
- **GitHub API Rate Limit Handling**: Respects `Retry-After` headers with automatic retry
- **Deduplication**: Webhook delivery-id dedup prevents duplicate reviews
- **Generated File Filtering**: Skips lockfiles, protobuf stubs, vendor dirs, binaries
- **Audit Trail**: Every review action logged with timestamps
- **Powered by agent-go**: Zero LLM SDK in this repo — all cognition delegated to agent-go via gRPC

### Cost Control

- **Model Routing**: `react` mode → agent-go `executor` role (default DeepSeek, cheap); `plan_execute` mode → agent-go `planner` role (agent-go picks a stronger model via its own `COGNITION_PLANNER_MODEL`)
- **Prompt Truncation**: Prompts capped at 32000 bytes (~31 KB); oversized diffs are split into ~28KB chunks instead of silently dropping code

### Multi-Agent (Plan-Execute)

PRs with 10+ files that still fit in a single prompt are reviewed in **plan-execute mode**: the agent first breaks the review into independent sub-tasks (security, bug detection, performance, code quality), runs them in parallel, then folds the results into one structured review. Larger diffs fall back to chunked react review to avoid prompt truncation.

### Security & Reliability

- **Rate Limiting**: Per-IP sliding-window limiter (120 req/min) returns `429 Too Many Requests` when exceeded. `X-Forwarded-For` is only honored when `TRUST_X_FORWARDED_FOR=true` (i.e. behind a trusted reverse proxy); otherwise the limiter keys on `RemoteAddr` to prevent header spoofing.
- **Panic Recovery**: Middleware catches handler panics, logs the stack trace, and returns 500 instead of crashing the process
- **ReadHeaderTimeout**: 5s header-read timeout mitigates slowloris-style attacks
- **Constant-Time Token Comparison**: API token checks use `crypto/subtle.ConstantTimeCompare` to prevent timing attacks
- **Startup Config Validation**: `GITHUB_TOKEN` is required on boot; `WEBHOOK_SECRET` and `API_TOKEN` are optional and disable verification/auth when unset (with a warning)

### Observability

- **OpenTelemetry**: Real OTel Go SDK with OTLP gRPC exporter. Spans propagate across the HTTP → gRPC boundary via W3C TraceContext (`otelgrpc` on the gRPC client plus a custom HTTP middleware).
- **Tracing config**: Set `OTEL_EXPORTER_OTLP_ENDPOINT` to send traces to a collector (Jaeger/Tempo/etc.). If unset, traces are sampled locally but not exported.
- **End-to-end traces**: Enable OTel on agent-go's cognition too (`COGNITION_OTEL_ENABLED=true` + `COGNITION_OTEL_EXPORTER_OTLP_ENDPOINT` → same collector) to join Go and Python spans in one trace.
- **`X-Trace-ID`**: Every response carries a trace ID header for correlating logs and traces.
- **Prometheus**: `/metrics` endpoint with review counts, latency, and error rates
- **Structured Logging**: `log/slog` with `trace_id` / `span_id` fields
- **DB Migration Versioning**: SQLite schema tracked via `PRAGMA user_version`; pending migrations apply automatically on startup

### Frontend Dashboard

- **React + TypeScript + Vite**: Review list and detail pages
- **SSE Real-time Progress**: Live review lifecycle events (started/progress/completed/failed) via `/api/reviews/stream`
- **Manual Trigger**: Trigger a review on any PR from the UI (no webhook required)

### Evaluation

- **30 Labeled PR Test Cases**: Curated corpus spanning security, bugs, performance, and style — including multi-bug, distractor, negative (clean code), and multi-file cases
- **Precision / Recall / F1 Metrics**: Automated evaluation runner measures agent quality
- See [Evaluation](#evaluation) below for real numbers

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (for agent-go cognition)
- The [agent-go](https://github.com/LingMi1/agent-go) repo cloned as a sibling of this repo, i.e. at `../agent-go` (needed by `docker compose` to build the cognition image)
- GitHub Personal Access Token with `repo` scope
- LLM API key (DeepSeek or Anthropic)

### 1. Clone & Setup

```bash
git clone https://github.com/LingMi1/code-review-agent.git
git clone https://github.com/LingMi1/agent-go.git
cd code-review-agent
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your tokens
```

### 3. Start Everything (cognition + app)

```bash
export GITHUB_TOKEN=ghp_xxxx
export LLM_API_KEY=sk-xxxx
docker compose up -d --build
```

This builds and runs both the agent-go cognition service and this app; the app reaches cognition over Docker's internal network (`cognition:50051`).

### 4. Run the Server Locally (alternative)

If you prefer to run the Go server outside Docker, first expose the cognition gRPC port by uncommenting `ports: "50051:50051"` in `docker-compose.yml` (or run agent-go's cognition natively), then:

```bash
docker compose up -d cognition
export GITHUB_TOKEN=ghp_xxxx
export WEBHOOK_SECRET=mysecret
export COGNITION_ADDR=localhost:50051

go run ./cmd/server/
```

### 5. Run the Frontend (optional)

```bash
cd web
npm install
npm run dev
# Dashboard at http://localhost:5173 (proxies to :8080)
```

### 6. Expose via ngrok (Local Dev)

```bash
ngrok http 8080
```

### 7. Configure GitHub Webhook

1. Go to your repo → Settings → Webhooks → Add webhook
2. Payload URL: `https://xxxx.ngrok.io/webhook`
3. Content type: `application/json`
4. Secret: same as `WEBHOOK_SECRET`
5. Events: "Pull requests"

### 8. Test

Push a PR to your repo. The agent will post a review comment within 30-60 seconds. Alternatively, trigger a review manually from the dashboard UI.

## Architecture

```
code-review-agent/
├── cmd/
│   ├── server/main.go          # HTTP server entry
│   └── eval/main.go            # Evaluation runner (--real for real LLM)
├── internal/
│   ├── webhook/                # GitHub webhook: HMAC verify + dedup
│   ├── github/                 # GitHub API: fetch diff, post review
│   ├── diff/                   # Unified diff parser + chunker + filter
│   ├── prompt/                 # Review prompt builder (react / plan-execute)
│   ├── cognition/              # gRPC client → agent-go cognition
│   ├── reviewer/               # Orchestrates diff → chunk → cognition → post
│   ├── review/                 # JSON parser → GitHub review poster
│   ├── store/                  # SQLite: review history + audit log
│   ├── otel/                   # OpenTelemetry tracing
│   ├── metrics/                # Prometheus /metrics endpoint
│   ├── middleware/             # HTTP middleware (OTel instrumentation)
│   └── sse/                    # SSE broadcast hub (real-time agent stream)
├── eval/                       # Evaluation framework
│   ├── corpus/                 # 30 labeled test PRs
│   ├── expected/               # Expected issues for each case
│   ├── runner.go               # Precision/Recall/F1 computation
│   └── reviewer_cognition.go   # Real agent-go cognition reviewer
├── web/                        # React + TypeScript + Vite frontend
├── assets/                     # Screenshots used in this README
├── docker-compose.yml          # cognition + app
├── Dockerfile                  # Multi-stage Go build
└── go.mod                      # Module definition
```

## gRPC Integration

This project consumes the agent-go cognition service via its public `CognitionService.Run` RPC (server-streaming):

```go
// internal/reviewer/reviewer.go wraps the CognitionService.Run RPC via RunReview
result, err := client.RunReview(ctx, cognition.ReviewRequest{
    SessionID: fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
    Query:     reviewPrompt,
    AgentType: "react", // or "plan_execute" for large PRs
    MaxSteps:  5,
})
```

No LLM SDK and no tool definitions in this repo — agent-go handles the entire Agent loop. This repo only builds the review prompts it sends over gRPC.

## Evaluation

The evaluation framework measures agent quality against a 30-case labeled corpus (6 negative + 6 multi-file cases). Each case contains a diff with one or more known issues (SQL injection, race condition, XSS, nil-pointer dereference, etc.) and an expected issue annotation. The last three single-file cases (016–018) are intentionally harder: multiple bugs in a single diff plus distractors, so a single glaring bug no longer inflates Recall.

Run with the mock reviewer (baseline) or the real agent-go cognition:

```bash
# Mock baseline (rule-based, no LLM)
go run ./cmd/eval/

# Real agent-go cognition (DeepSeek via gRPC)
go run ./cmd/eval/ -real
```

### Results (DeepSeek-chat)

> The DeepSeek numbers below are measured on the full 30-case corpus
> (`go run ./cmd/eval/ -real -cognition <addr>`).

| Metric | Mock Baseline (30 cases) | DeepSeek (30 cases) |
|---|---|---|
| Pass Rate (F1 ≥ 0.5) | 73% (22/30) | **67% (20/30)** |
| Macro Precision | 0.69 | **0.50** |
| Macro Recall | 0.73 | **0.86** |
| Macro F1 | 0.71 | **0.59** |

> Mock baseline metrics are measured under 30 cases; run `go run ./cmd/eval/` to reproduce.
>
> Real-LLM metrics reflect both model capability and corpus boundaries: negative cases demand zero false positives (a high bar for an LLM), and multi-file cases require cross-file coverage. See "Key findings" below.

Key findings:

- **Recall is high (0.86)**: the real LLM hits the vast majority of labeled issues across the 30 cases (SQL injection, XSS, command injection, path traversal, division-by-zero, hardcoded passwords), which shows it is strong at finding real problems.
- **Precision is lower (0.50)**: the agent reports findings beyond the labeled set (deprecated APIs, missing error context, style-level suggestions). Some are legitimate but outside the human annotation; others are over-caution — the negative cases demand zero false positives, which is a high bar for an LLM (5 of 6 negatives were clean; 1 still emitted 2 style suggestions).
- **Multi-bug / multi-file cases**: 016–018 (several defects in one diff) and 025–030 (multi-file) require hitting every defect while avoiding distractors; the real LLM keeps recall at 1.0 on these.

## Key Difficulties and Tradeoffs

A few engineering decisions here are prime interview follow-up material; the trade-offs are spelled out up front:

- **Why chunk by bytes instead of lines**: The agent prompt has a byte cap (32KB). Chunking by lines ignores line length — a single very long line (e.g. a giant one-line JSON or generated code) can push a "chunk" well over the cap and get truncated, silently dropping data. Chunking by 28KB bytes keeps every chunk under the cap, and we log a `WARN` before truncation rather than dropping silently.
- **Why de-duplicate chunk results**: A large PR is split into multiple chunks, and the same issue can be reported by two adjacent chunks. Before posting we de-duplicate by `file:line:category` to avoid spamming duplicate comments on the PR.
- **Why SSE has no `WriteTimeout`**: SSE is a long-lived connection; setting a write timeout lets `net/http` cut the stream off. So we only set `ReadTimeout`/`IdleTimeout`/`ReadHeaderTimeout`, trading write-timeout protection for stream stability.
- **Why eval reuses the production prompt**: Evaluation must test "the prompt the production path actually sends." If eval wrote a separate prompt, the resulting F1 wouldn't reflect the real pipeline.
- **Why webhook verification uses constant-time HMAC comparison**: `hmac.Equal` prevents the signature check from leaking timing side channels.

## Review Format

The agent returns structured JSON:

```json
{
  "summary": "Added user authentication middleware with session management",
  "issues": [
    {
      "file": "src/auth/middleware.go",
      "line": 42,
      "severity": "high",
      "category": "security",
      "title": "SQL injection in user query",
      "description": "User input is concatenated directly into SQL string...",
      "suggestion": "Use parameterized query: db.Query('SELECT * FROM users WHERE id = $1', userID)"
    }
  ]
}
```

## API

| Endpoint | Method | Description |
|---|---|---|
| `/webhook` | POST | GitHub webhook receiver (HMAC-verified) |
| `/health` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |
| `/api/reviews` | GET | List recent reviews |
| `/api/reviews` | POST | Manually trigger a review (owner/repo/pr_number) |
| `/api/reviews/:id` | GET | Get review detail |
| `/api/reviews/stream` | GET | SSE stream of review events |

An OpenAPI 3.0 specification of these endpoints is available at [`docs/openapi.yaml`](docs/openapi.yaml).

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, net/http |
| Agent Engine | **agent-go** (Python/LangGraph via gRPC) |
| Frontend | React 18, TypeScript, Vite, Tailwind CSS |
| Storage | SQLite (local), PostgreSQL (via agent-go) |
| Observability | OpenTelemetry, Prometheus, `log/slog` |
| Deployment | Docker, Docker Compose |
| LLM | DeepSeek / Claude (via agent-go) |

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Found a security issue? Please follow the disclosure process in [SECURITY.md](SECURITY.md).

## License

MIT
