# Contributing to code-review-agent

Thanks for your interest in contributing! code-review-agent is an AI-powered GitHub PR review bot with a Go backend (`cmd/`, `internal/`, `eval/`) and a React + Vite frontend (`web/`).

## Prerequisites

- **Go** 1.25+
- **Node.js** 20+ (with npm)
- [golangci-lint](https://golangci-lint.run/) v1.64+ for linting

## Local development

```bash
git clone https://github.com/LingMi1/code-review-agent.git
cd code-review-agent

# Backend — the Go server reads env vars directly (os.Getenv), not a .env file.
# Use .env only with docker compose, which injects it into the container.
export GITHUB_TOKEN=your_token
export WEBHOOK_SECRET=your_secret
go run ./cmd/server/

# Frontend (separate terminal)
cd web
npm install
npm run dev
```

The API server runs on `http://localhost:8080`; the UI dev server runs on `http://localhost:5173`.

## Testing

```bash
# Go tests (race detector enabled)
go test -race -count=1 ./...

# Frontend tests
cd web && npm test
```

## Linting

```bash
golangci-lint run
go vet ./...
```

## Evaluation

The eval suite measures recall against a 30-case labeled corpus (including negative and multi-file cases). It runs in mock mode locally and in CI:

```bash
go run ./cmd/eval/
```

## Pull request guidelines

- Open PRs against `main`.
- Use **conventional commit** messages (e.g. `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`).
- We use **squash merge** — keep your branch tidy; the squash title becomes the final commit message.
- Keep PRs focused: one logical change per PR.
- Add or update tests for any behavioral change.

## CI requirements

All CI jobs must pass before merge: `Test (Go)`, `Lint (golangci-lint)`, `go vet`, `Eval (mock)`, `Build (Web)`, and `Test (Web)`.

Run `make test lint web-test` locally to catch issues early. Thanks for contributing!
