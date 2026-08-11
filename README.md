# Code Review Agent

AI-powered code review that catches bugs, security issues, and performance problems in your pull requests — **powered by [agent-go](https://github.com/LingMi1/agent-go)**, the production-grade multi-agent platform.

## How It Works

```
GitHub PR webhook
      │
      ▼
┌─────────────────────────────────┐
│ code-review-agent (Go)          │
│  • HMAC-SHA256 webhook verify   │
│  • Fetch PR diff                 │
│  • Parse & chunk by file         │
│  • Build review prompt           │
└──────────┬──────────────────────┘
           │ gRPC (CognitionService.Run)
           ▼
┌─────────────────────────────────┐
│ agent-go cognition (Python)     │
│  • ReAct Agent analyzes diff    │
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

### 5. Expose via ngrok (Local Dev)

```bash
ngrok http 8080
```

### 6. Configure GitHub Webhook

1. Go to your repo → Settings → Webhooks → Add webhook
2. Payload URL: `https://xxxx.ngrok.io/webhook`
3. Content type: `application/json`
4. Secret: same as `WEBHOOK_SECRET`
5. Events: "Pull requests"

### 7. Test

Push a PR to your repo. The agent will post a review comment within 30-60 seconds.

## Architecture

```
code-review-agent/
├── cmd/server/main.go          # HTTP server entry
├── internal/
│   ├── webhook/                # GitHub webhook: HMAC verify + dedup
│   ├── github/                 # GitHub API: fetch diff, post review
│   ├── diff/                   # Unified diff parser + chunker
│   ├── prompt/                 # Review prompt builder
│   ├── cognition/              # gRPC client → agent-go cognition
│   ├── review/                 # JSON parser → GitHub review poster
│   └── store/                  # SQLite: review history + audit log
├── docker-compose.yml          # cognition + app
├── Dockerfile                  # Multi-stage Go build
└── go.mod                      # replace → ../agent-go/agent-go/control-plane
```

## gRPC Integration

This project consumes the agent-go cognition service via its public `CognitionService.Run` RPC:

```go
// From internal/cognition/client.go
stream, err := svc.Run(ctx, &agentv1.RunRequest{
    RunId:     uuid.New().String(),
    SessionId: fmt.Sprintf("pr-review-%d", prNumber),
    Query:     reviewPrompt,
    AgentType: "react",
    MaxSteps:  5,
})
// Collect events...
```

No LLM SDK, no prompt templates, no tool definitions in this repo. agent-go handles the entire Agent loop.

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
| `/webhook` | POST | GitHub webhook receiver |
| `/health` | GET | Health check |
| `/api/reviews` | GET | List recent reviews |
| `/api/reviews/:id` | GET | Get review detail |

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, net/http |
| Agent Engine | **agent-go** (Python/LangGraph via gRPC) |
| Storage | SQLite (local), PostgreSQL (via agent-go) |
| Deployment | Docker, Docker Compose |
| LLM | DeepSeek / Claude (via agent-go) |

## License

MIT
