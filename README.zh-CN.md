# Code Review Agent

[![CI](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/LingMi1/code-review-agent/actions/workflows/ci.yml)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.6-3178C6?logo=typescript)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> [English](README.md)

一个 AI 代码审查工具，自动揪出 PR 里的 bug、安全漏洞和性能问题。它的「思考能力」完全来自生产级多 Agent 平台 [agent-go](https://github.com/LingMi1/agent-go)，本仓库不引入任何 LLM SDK。

## 界面截图

| 审查列表 | 审查详情 |
|---|---|
| <img src="assets/review-list.png" width="480" alt="审查列表" /> | <img src="assets/review-detail.png" width="480" alt="审查详情" /> |

## 工作原理

```
GitHub PR webhook / 手动触发
      │
      ▼
┌─────────────────────────────────┐
│ code-review-agent (Go)          │
│  • HMAC-SHA256 webhook 验签     │
│  • 拉取 PR diff                 │
│  • 按文件解析与分块             │
│  • 智能模型路由                 │
│  • 构造审查 prompt              │
└──────────┬──────────────────────┘
           │ gRPC (CognitionService.Run, server-streaming)
           ▼
┌─────────────────────────────────┐
│ agent-go 认知面 (Python)        │
│  • ReAct / Plan-Execute Agent   │
│  • 多 Agent 协作                │
│  • 输出结构化 JSON              │
└──────────┬──────────────────────┘
           │
           ▼
┌─────────────────────────────────┐
│ GitHub PR Review                 │
│  • 在代码行上内联评论            │
│  • 严重级：🔴 高 🟡 中 🟢 低     │
└─────────────────────────────────┘
```

## 和一般 AI Code Review 工具的区别

市面上大多数 AI code review 工具，本质上是给 LLM API 套了层壳：拼一段 prompt、丢给 OpenAI/Claude、再把回复贴回 PR。这个项目有三点不一样：

- **不碰 LLM SDK，认知是独立平台**：每一次审查都交给 [agent-go](https://github.com/LingMi1/agent-go)（生产级多 Agent 平台，支持 ReAct / Plan-Execute、工具调用、结构化 JSON 输出）。本仓库只负责通过 gRPC 拼 prompt、收结果——这正是「在 Agent 平台上做 Agent 应用」这类岗位会做的事。
- **用数据说话，不靠感觉**：30 个标注用例（安全 / bug / 性能 / 风格，外加负例与多文件用例）直接拿真实 LLM（DeepSeek）打分，并公开不带水分的 P/R/F1——包括 LLM 翻车的地方（Precision 0.50、负例误报）。Mock 和真实数字都能复现。
- **生产级工程，不是 demo**：HMAC webhook 验签、限流、panic 恢复、优雅降级、去重、OTel 追踪、Prometheus 指标、e2e 测试和 CI（test/lint/vet/eval/web build）——这些恰恰是「脚本式 code review」最常跳过的部分。

## 功能特性

### 核心流程

- **真实 PR 处理**：处理 `opened`、`synchronize`、`reopened` 三类 PR 事件
- **HMAC-SHA256 Webhook 验签**：防止伪造请求
- **Diff 分块**：大 PR 按文件拆成约 28KB 的块（大文件会在 hunk 边界进一步切分），逐块独立审查后合并
- **结构化 JSON 输出**：LLM 返回可被程序解析的审查结果（文件/行号/严重级/类别/建议）
- **优雅降级**：单次审查在 JSON 解析失败时回退为纯文本评论
- **GitHub API 限流处理**：遵循 `Retry-After` 响应头自动重试
- **去重**：基于 webhook delivery-id 去重，防止重复审查
- **生成文件过滤**：跳过 lockfile、protobuf stub、vendor 目录、二进制文件
- **审计日志**：每次审查操作带时间戳记录
- **由 agent-go 驱动**：本仓库不含任何 LLM SDK——认知能力全部通过 gRPC 交给 agent-go 完成

### 成本控制

- **模型路由**：`react` 模式 → agent-go `executor` 角色（默认 DeepSeek，便宜）；`plan_execute` 模式 → agent-go `planner` 角色（由 agent-go 通过其自身的 `COGNITION_PLANNER_MODEL` 选择更强模型）
- **Prompt 截断**：prompt 上限 32000 字节（约 31 KB）；超长 diff 通过约 28KB 分块而非静默丢弃代码来适配上下文

### 多 Agent（Plan-Execute）

当 PR 文件数达到 10+ 且整份 diff 能装进单条 prompt 时，走 **plan-execute 模式**：Agent 先把审查拆成几个独立子任务（安全、bug 检测、性能、代码质量），并行跑完再汇总成一份结构化结果。diff 太大时则回退为分块 react 审查，避免 prompt 被截断。

### 安全与可靠性

- **IP 限流**：基于滑动窗口的按 IP 限流（120 次/分钟），超限返回 `429 Too Many Requests`。仅当 `TRUST_X_FORWARDED_FOR=true`（部署在受信反向代理后）时才信任 `X-Forwarded-For`；否则用 `RemoteAddr`，防止伪造请求头绕过限流
- **Panic 恢复**：中间件捕获 handler 里的 panic，记下堆栈后返回 500，避免进程崩溃
- **ReadHeaderTimeout**：5 秒请求头读取超时，抵御 slowloris 慢速攻击
- **常量时间 token 比较**：API token 校验用 `crypto/subtle.ConstantTimeCompare`，防止时序攻击
- **启动时配置校验**：`GITHUB_TOKEN` 缺失时拒绝启动；`WEBHOOK_SECRET` 与 `API_TOKEN` 可选，未设置时会禁用验签/鉴权并给出警告

### 可观测性

- **OpenTelemetry**：真正的 OTel Go SDK + OTLP gRPC exporter，跨 HTTP → gRPC 边界通过 W3C TraceContext 传播（gRPC 客户端用 `otelgrpc`，HTTP 侧用自研中间件埋点）
- **追踪配置**：设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 把 trace 送到 collector（如 Jaeger/Tempo）；不设置时本地采样但不导出
- **端到端追踪**：在 agent-go 认知面同样开启 OTel（`COGNITION_OTEL_ENABLED=true` + `COGNITION_OTEL_EXPORTER_OTLP_ENDPOINT` 指向同一 collector），即可把 Go 与 Python 两段 span 串进同一条 trace
- **`X-Trace-ID`**：每个响应都带 trace ID 头，方便把日志和 trace 对上
- **Prometheus**：`/metrics` 端点对外提供审查数量、延迟、错误率
- **结构化日志**：`log/slog` 带 `trace_id` / `span_id` 字段
- **数据库迁移版本管理**：SQLite schema 通过 `PRAGMA user_version` 追踪版本，启动时自动应用未执行的迁移

### 前端界面

- **React + TypeScript + Vite**：审查列表页和详情页
- **SSE 实时进度**：通过 `/api/reviews/stream` 推送审查生命周期事件（started/progress/completed/failed）
- **手动触发**：不用 webhook，直接在 UI 上对任意 PR 触发审查

### 评估体系

- **30 个标注 PR 测试用例**：覆盖安全、bug、性能、风格四类，含多 bug、干扰项、负例（干净代码）与多文件用例
- **Precision / Recall / F1 指标**：自动化评估运行器衡量 Agent 质量
- 真实数据见下方 [评估](#评估)

## 快速开始

### 前置条件

- Go 1.25+
- Docker（用于 agent-go 认知面）
- 把 [agent-go](https://github.com/LingMi1/agent-go) 仓库 clone 到与本仓库同级的位置，即 `../agent-go`（`docker compose` 构建认知面镜像需要）
- 具备 `repo` 权限的 GitHub Personal Access Token
- LLM API key（DeepSeek 或 Anthropic）

### 1. 克隆与设置

```bash
git clone https://github.com/LingMi1/code-review-agent.git
git clone https://github.com/LingMi1/agent-go.git
cd code-review-agent
```

### 2. 配置环境

```bash
cp .env.example .env
# 编辑 .env 填入你的 token
```

### 3. 一键启动（认知面 + 应用）

```bash
export GITHUB_TOKEN=ghp_xxxx
export LLM_API_KEY=sk-xxxx
docker compose up -d --build
```

该命令会同时构建并运行 agent-go 认知面与本应用，应用通过 Docker 内部网络（`cognition:50051`）访问认知面。

### 4. 本地运行服务（可选）

如果你更想在本机直接跑 Go 服务，需要先暴露认知面的 gRPC 端口：取消 `docker-compose.yml` 中 `ports: "50051:50051"` 的注释（或本机原生运行 agent-go 认知面），然后：

```bash
docker compose up -d cognition
export GITHUB_TOKEN=ghp_xxxx
export WEBHOOK_SECRET=mysecret
export COGNITION_ADDR=localhost:50051

go run ./cmd/server/
```

### 5. 启动前端（可选）

```bash
cd web
npm install
npm run dev
# 面板地址 http://localhost:5173（代理到 :8080）
```

### 6. 用 ngrok 暴露公网（本地开发）

```bash
ngrok http 8080
```

### 7. 配置 GitHub Webhook

1. 进入你的仓库 → Settings → Webhooks → Add webhook
2. Payload URL：`https://xxxx.ngrok.io/webhook`
3. Content type：`application/json`
4. Secret：与 `WEBHOOK_SECRET` 一致
5. Events：选择 "Pull requests"

### 8. 测试

向你的仓库提一个 PR，Agent 会在 30-60 秒内发布审查评论。也可以直接在面板 UI 上手动触发审查。

## 架构

```
code-review-agent/
├── cmd/
│   ├── server/main.go          # HTTP 服务入口
│   └── eval/main.go            # 评估运行器（--real 使用真实 LLM）
├── internal/
│   ├── webhook/                # GitHub webhook：HMAC 验签 + 去重
│   ├── github/                 # GitHub API：拉取 diff、发布审查
│   ├── diff/                   # Unified diff 解析器 + 分块 + 过滤
│   ├── prompt/                 # 审查 prompt 构造（react / plan-execute）
│   ├── cognition/              # gRPC 客户端 → agent-go 认知面
│   ├── reviewer/               # 编排 diff → 分块 → 认知面 → 发布
│   ├── review/                 # JSON 解析 → GitHub 审查发布
│   ├── store/                  # SQLite：审查历史 + 审计日志
│   ├── otel/                   # OpenTelemetry 追踪
│   ├── metrics/                # Prometheus /metrics 端点
│   ├── middleware/             # HTTP 中间件（OTel 埋点）
│   └── sse/                    # SSE 广播中心（实时审查进度）
├── eval/                       # 评估框架
│   ├── corpus/                 # 30 个标注测试 PR
│   ├── expected/               # 每个用例的期望问题
│   ├── runner.go               # Precision/Recall/F1 计算
│   └── reviewer_cognition.go   # 真实 agent-go 认知面审查器
├── web/                        # React + TypeScript + Vite 前端
├── assets/                     # README 使用的截图
├── docker-compose.yml          # cognition + app
├── Dockerfile                  # 多阶段 Go 构建
└── go.mod                      # 模块定义
```

## gRPC 集成

本项目通过 agent-go 认知面的公开 RPC `CognitionService.Run`（server-streaming）获取认知能力：

```go
// internal/reviewer/reviewer.go 通过 RunReview 封装 CognitionService.Run RPC
result, err := client.RunReview(ctx, cognition.ReviewRequest{
    SessionID: fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
    Query:     reviewPrompt,
    AgentType: "react", // 大 PR 使用 "plan_execute"
    MaxSteps:  5,
})
```

本仓库不含 LLM SDK 和工具定义——agent-go 负责整个 Agent 循环；本仓库只负责构造通过 gRPC 发送的审查 prompt。

## 评估

评估框架基于 30 个标注用例（含 6 个负例 + 6 个多文件用例）衡量 Agent 质量。每个用例包含一段带一个或多个已知问题的 diff（SQL 注入、竞态条件、XSS、空指针解引用等）和对应的问题标注。最后三个单文件用例（016–018）刻意加难：单个 diff 内含多个 bug 并混入干扰项，避免「一个显眼 bug 就拉满 Recall」的注水现象。

用 mock 审查器（基准）或真实 agent-go 认知面运行：

```bash
# Mock 基准（基于规则，无 LLM）
go run ./cmd/eval/

# 真实 agent-go 认知面（DeepSeek via gRPC）
go run ./cmd/eval/ -real
```

### 结果（DeepSeek-chat）

> 以下 DeepSeek 数据为 30 用例下的实测值（`go run ./cmd/eval/ -real -cognition <addr>`）。

| 指标 | Mock 基准（30 用例） | DeepSeek（30 用例） |
|---|---|---|
| 通过率（F1 ≥ 0.5） | 73%（22/30） | **67%（20/30）** |
| Macro Precision | 0.69 | **0.50** |
| Macro Recall | 0.73 | **0.86** |
| Macro F1 | 0.71 | **0.59** |

> Mock 基准为 30 用例下的实测值，运行 `go run ./cmd/eval/` 可复现。
>
> 真实 LLM 指标同时反映模型能力与语料边界：负例要求零误报，对 LLM 是高压线；多文件用例要求跨文件覆盖。详见下方「关键发现」。

关键发现：

- **Recall 高（0.86）**：真实 LLM 在 30 用例下命中绝大多数标注问题（SQL 注入、XSS、命令注入、路径穿越、除零、硬编码密码等），说明「抓真问题」能力强。
- **Precision 偏低（0.50）**：Agent 会报告标注集之外的额外发现（废弃 API、缺少错误上下文、style 级建议）。其中部分是真实但不在人工标注内，部分是 LLM 的「过度谨慎」——负例要求零误报，对 LLM 是高压线（6 个负例 5 个做到零误报，1 个仍给出 2 条 style 建议）。
- **多 bug / 多文件用例**：016–018（单文件多缺陷）与 025–030（多文件）要求同时命中多个缺陷并避开干扰项，真实 LLM 的 recall 均保持 1.0。

## 关键难点与权衡

这个项目里有几个容易在面试里被追问的工程决策，提前把取舍说清楚：

- **为什么按字节分块而不是按行**：Agent 的 prompt 有字节上限（32KB）。按行分块会忽略行本身的长度，长行（如单行巨型 JSON / 生成的代码）可能让某一「块」远超上限而被截断，造成静默丢数据。改按 28KB 字节分块后，每块恒小于上限；截断前还会打 `WARN`，不再静默。
- **为什么对分块结果去重**：大 PR 被拆成多块后，同一处问题可能被相邻两块重复报告。投递前按 `file:line:category` 去重，避免在 PR 里刷出重复评论。
- **为什么 SSE 不设 `WriteTimeout`**：SSE 是长连接，设了写超时会被 `net/http` 直接掐断流。所以只设 `ReadTimeout`/`IdleTimeout`/`ReadHeaderTimeout`，牺牲「写超时保护」换取流稳定。
- **为什么 eval 复用生产 prompt**：评测必须测「生产实际会发出去的 prompt」。若 eval 另写一套 prompt，测出来的 F1 就不代表真实链路。
- **为什么 webhook 校验用 HMAC 常量时间比较**：`hmac.Equal` 避免签名校验被时序侧信道攻击。

## 审查格式

Agent 返回结构化 JSON：

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

| 端点 | 方法 | 说明 |
|---|---|---|
| `/webhook` | POST | GitHub webhook 接收（HMAC 验签） |
| `/health` | GET | 健康检查 |
| `/metrics` | GET | Prometheus 指标 |
| `/api/reviews` | GET | 列出最近审查 |
| `/api/reviews` | POST | 手动触发审查（owner/repo/pr_number） |
| `/api/reviews/:id` | GET | 获取审查详情 |
| `/api/reviews/stream` | GET | 审查事件的 SSE 流 |

各端点的 OpenAPI 3.0 规范见 [`docs/openapi.yaml`](docs/openapi.yaml)。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25、net/http |
| Agent 引擎 | **agent-go**（Python/LangGraph via gRPC） |
| 前端 | React 18、TypeScript、Vite、Tailwind CSS |
| 存储 | SQLite（本地）、PostgreSQL（via agent-go） |
| 可观测性 | OpenTelemetry、Prometheus、`log/slog` |
| 部署 | Docker、Docker Compose |
| LLM | DeepSeek / Claude（via agent-go） |

## 贡献

欢迎贡献代码！请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。如发现安全问题，请按 [SECURITY.md](SECURITY.md) 中的流程上报。

## 许可证

MIT
