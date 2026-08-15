# 参与 code-review-agent 开发

> [English](CONTRIBUTING.md)

感谢你的参与兴趣。code-review-agent 是一个 AI 驱动的 GitHub PR 审查机器人，后端用 Go（`cmd/`、`internal/`、`eval/`），前端用 React + Vite（`web/`）。

## 环境准备

- **Go** 1.25+
- **Node.js** 20+（含 npm）
- [golangci-lint](https://golangci-lint.run/) v1.64+（代码检查用）

## 本地开发

```bash
git clone https://github.com/LingMi1/code-review-agent.git
cd code-review-agent

# 后端 —— Go 服务直接读环境变量（os.Getenv），不读 .env 文件。
# .env 只配合 docker compose 使用，由 compose 注入容器。
export GITHUB_TOKEN=your_token
export WEBHOOK_SECRET=your_secret
go run ./cmd/server/

# 前端（另开一个终端）
cd web
npm install
npm run dev
```

API 服务跑在 `http://localhost:8080`，前端开发服务器跑在 `http://localhost:5173`。

## 测试

```bash
# Go 测试（开启竞态检测）
go test -race -count=1 ./...

# 前端测试
cd web && npm test
```

## 代码检查

```bash
golangci-lint run
go vet ./...
```

## 评测

评测套件用 30 个标注用例（含负例和多文件用例）来衡量召回率，本地和 CI 都跑 mock 模式：

```bash
go run ./cmd/eval/
```

## 提交 PR 的约定

- 往 `main` 分支提 PR。
- 提交信息遵循 **conventional commit**（`feat:`、`fix:`、`refactor:`、`test:`、`docs:`、`chore:`）。
- 我们采用 **squash merge** —— 保持分支干净，squash 的标题会成为最终提交信息。
- 一个 PR 只做一件事。
- 有行为变更就要补或改测试。

## CI 门禁

合入前所有 CI 任务必须通过：`Test (Go)`、`Lint (golangci-lint)`、`go vet`、`Eval (mock)`、`Build (Web)`、`Test (Web)`。

本地先跑 `make test lint web-test` 能提前发现问题。感谢参与！
