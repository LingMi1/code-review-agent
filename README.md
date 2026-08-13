# Code Review Agent

[![CI](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.6-3178C6?logo=typescript)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> [简体中文](README.zh-CN.md)

AI-powered code review that catches bugs, security issues, and performance problems in your pull requests — **powered by [agent-go](https://github.com/LingMi1/agent-go)**, the production-grade multi-agent platform.

## Screenshots

| Review Dashboard | Review Detail |
|---|---|
| ![Review list](assets/review-list.png) | ![Review detail](assets/review-detail.png) |

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

## Features

### Core Pipeline

- **Real PR Processing**: Handles `opened`, `synchronize`, and `reopened` PR events
- **HMAC-SHA256 Webhook Verification**: Prevents forged requests
- **Incremental Diff Chunking**: Large PRs split by file to fit token budgets
- **Structured JSON Output**: LLM returns machine-parseable review with file/line/severity/category/suggestion
- **Graceful Degradation**: Falls back to plain-text comment when JSON parsing fails
- **GitHub API Rate Limit Handling**: Respects `Retry-After` headers with automatic retry
- **Deduplication**: Webhook delivery-id dedup prevents duplicate reviews
- **Generated File Filtering**: Skips lockfiles, protobuf stubs, vendor dirs, binaries
- **Audit Trail**: Every review action logged with timestamps
- **Powered by agent-go**: Zero LLM SDK in this repo — all cognition delegated to agent-go via gRPC

### Cost Control

- **Token Budget Enforcement**: Per-file and per-PR token limits prevent runaway costs
- **Smart Model Routing**: Small PRs use cheap models, large/complex PRs escalate to stronger models
- **Diff Truncation**: Oversized hunks trimmed before prompt construction

### Multi-Agent (Plan-Execute)

Large PRs are reviewed in **plan-execute mode**: the agent first decomposes the review into independent sub-tasks (security, bug detection, performance, code quality), executes each in parallel, then aggregates results into a single structured review.

### Observability

- **OpenTelemetry**: Distributed tracing across Go → gRPC → agent-go cognition
- **Prometheus**: `/metrics` endpoint with review counts, latency, and error rates
- **Structured Logging**: `log/slog` with request-scoped fields

### Frontend Dashboard

- **React + TypeScript + Vite**: Review list and detail pages
- **SSE Real-time Streaming**: Live agent thinking stream via `/api/reviews/stream`
- **Manual Trigger**: Trigger a review on any PR from the UI (no webhook required)

### Evaluation

- **15 Labeled PR Test Cases**: Curated corpus spanning security, bugs, performance, and style
- **Precision / Recall / F1 Metrics**: Automated evaluation runner measures agent quality
- See [Evaluation](#evaluation) below for real numbers

## Quick Start

### Prerequisites

- Go 1.25+
- Docker (for agent-go cognition)
- GitHub Personal Access Token with `repo` scope
- LLM API key (DeepSeek or Anthropic)

### 1. Clone & Setup

```bash
git clone https://github.com/LingMi1/code-review-agent.git
cd code-review-agent

# agent-go must be at ../agent-go/agent-go (go.mod replace)
# If not, adjust the replace directive in go.mod
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your tokens
```

### 3. Start agent-go Cognition

```bash
docker compose up -d cognition
```

### 4. Run the Server

```bash
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
│   ├── review/                 # JSON parser → GitHub review poster
│   ├── store/                  # SQLite: review history + audit log
│   ├── otel/                   # OpenTelemetry tracing
│   ├── metrics/                # Prometheus /metrics endpoint
│   ├── middleware/             # HTTP middleware (OTel instrumentation)
│   └── sse/                    # SSE broadcast hub (real-time agent stream)
├── eval/                       # Evaluation framework
│   ├── corpus/                 # 15 labeled test PRs
│   ├── expected/               # Expected issues for each case
│   ├── runner.go               # Precision/Recall/F1 computation
│   └── reviewer_cognition.go   # Real agent-go cognition reviewer
├── web/                        # React + TypeScript + Vite frontend
├── assets/                     # Screenshots used in this README
├── docker-compose.yml          # cognition + app
├── Dockerfile                  # Multi-stage Go build
└── go.mod                      # replace → ../agent-go/agent-go/control-plane
```

## gRPC Integration

This project consumes the agent-go cognition service via its public `CognitionService.Run` RPC (server-streaming):

```go
// From internal/cognition/client.go
stream, err := svc.Run(ctx, &agentv1.RunRequest{
    RunId:     uuid.New().String(),
    SessionId: fmt.Sprintf("pr-review-%d", prNumber),
    Query:     reviewPrompt,
    AgentType: "react",   // or "plan_execute" for large PRs
    MaxSteps:  5,
})
// Collect events...
```

No LLM SDK, no prompt templates, no tool definitions in this repo. agent-go handles the entire Agent loop.

## Evaluation

The evaluation framework measures agent quality against a 15-case labeled corpus. Each case contains a diff with a known issue (SQL injection, race condition, XSS, nil-pointer dereference, etc.) and an expected issue annotation.

Run with the mock reviewer (baseline) or the real agent-go cognition:

```bash
# Mock baseline (rule-based, no LLM)
go run ./cmd/eval/

# Real agent-go cognition (DeepSeek via gRPC)
go run ./cmd/eval/ -real
```

### Results (DeepSeek-chat)

| Metric | Mock Baseline | DeepSeek (real) |
|---|---|---|
| Pass Rate (F1 ≥ 0.5) | 47% (7/15) | **73% (11/15)** |
| Macro Precision | 0.39 | **0.51** |
| Macro Recall | 0.47 | **1.00** |
| Macro F1 | 0.42 | **0.68** |

Key findings:

- **Recall 1.00**: The real LLM finds every labeled bug and security vulnerability (zero false negatives) — the most important property for a code review tool.
- **Precision 0.51**: The agent also reports additional findings beyond the labeled set (e.g. deprecated APIs, missing error context). Many of these are legitimate but not in the human annotation, which lowers precision under the strict matching rule.
- **Line-number tolerance**: LLM line estimates and human annotations both carry ±1-2 line noise, so matching uses a ±3 line tolerance.

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

## License

MIT
