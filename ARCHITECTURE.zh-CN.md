# 架构设计

> [English](ARCHITECTURE.md)

本文讲 code-review-agent 是怎么设计的、每个关键决策背后做了哪些取舍，给想搞懂系统「为什么这样建」的审阅者和面试官看。

## 概览

code-review-agent 是一个自动审查 Pull Request 的 GitHub Webhook 服务。本质上是用 Go 写的一层轻量编排，把真正的「思考」交给外部的 **agent-go cognition** 服务，通过 gRPC 调用。

```
GitHub webhook ──▶ Go 服务 ──▶ agent-go cognition (gRPC)
                     │                    │
                     ▼                    └─ LLM (deepseek / planner 模型)
              SQLite + SSE + Prometheus
```

## 组件

| 组件 | 路径 | 职责 |
|------|------|------|
| `webhook` | `internal/webhook` | HMAC 签名校验、事件解析 |
| `diff` | `internal/diff` | 把 GitHub unified diff 解析成结构化文件 |
| `reviewer` | `internal/reviewer` | 编排 diff → 分块 → 认知面 → 投递 |
| `cognition` | `internal/cognition` | 到 agent-go 的 gRPC 客户端，带熔断器 |
| `prompt` | `internal/prompt` | 构造审查 prompt、截断 |
| `review` | `internal/review` | 解析认知面输出、合并、投递到 GitHub |
| `github` | `internal/github` | GitHub REST 客户端（diff、review、评论） |
| `store` | `internal/store` | SQLite 历史 + 审计日志 |
| `sse` | `internal/sse` | 实时进度广播到前端 |
| `metrics` | `internal/metrics` | Prometheus 指标 |
| `otel` | `internal/otel` | OpenTelemetry 追踪 + trace-id 日志 |
| `eval` | `eval` | 评估框架（precision/recall/F1） |

## 审查流水线

```
webhook 事件
  └─▶ 提取 owner/repo/pr/head_sha
        └─▶ 拉取 unified diff（GitHub）
              └─▶ 解析为 []FileDiff
                    └─▶ 若总行数超过阈值：切分成多个 chunk（800 行）
                          └─▶ 逐块调用认知面
                                └─▶ 解析 JSON 输出
                                      └─▶ 合并所有 chunk 为一个 ReviewOutput
                                            └─▶ 投递 review + 评论（GitHub）
```

每条审查记录写进 SQLite，进度通过 SSE 广播，让仪表盘反映真实状态。

## 关键设计决策与取舍

### 1. 分块审查，而不是整段一次过

大 PR 会超出 LLM 的上下文窗口。与其直接截断（会静默丢掉代码），不如把 diff 切成约 800 行一块，逐块独立审查。

- **取舍**：分块会丢掉跨文件的上下文（跨两块才成立的 bug 可能被漏掉，或被重复报）。能接受，是因为块内正确性比全局视角更重要，而且 GitHub 评论本来就是按行定位的。

### 2. 靠 agent-go 的角色做模型路由

agent-go 认知面是按**角色**而不是按请求来路由模型的。code-review-agent 把两种策略映射到这些角色上：

- `react`（小 PR）→ `executor` 角色 → 便宜、快的模型（deepseek）
- `plan_execute`（10+ 文件且能塞进单条 prompt）→ `planner` 角色 → 更强的模型
- `react` 分块（diff 过大）→ `executor` 角色，切成约 800 行一块

这样不用自己复刻一套 LLM 技术栈，就能在成本和效果之间做选择。

### 3. gRPC 边界上熔断 + 重试

认知面客户端带熔断器，让不稳定的认知面服务快速失败，而不是级联出一堆超时。GitHub 的 HTTP 调用有带 `Retry-After` 退避的有界重试；重试前会重置请求体，POST 永远不会发出空 payload。

### 4. 让 LLM 输出结构化 JSON

prompt 强制要求严格的 JSON schema，还附了一个 one-shot 示例。对输出做容错解析，再在多个 chunk 之间合并。这样评估就能用 precision/recall 量化，而不是对着自由文本猜。

### 5. 可观测性

- **追踪**：OpenTelemetry SDK + OTLP gRPC exporter，用 W3C TraceContext 在 HTTP 和 gRPC 之间传播。要端到端串通，还需要在 agent-go 里开启 OTel（`COGNITION_OTEL_ENABLED=true` + collector 地址）。
- **指标**：`/metrics` 提供 Prometheus text 格式（审查次数、延迟分位数、错误率）。
- **日志**：结构化 `slog`，自动带上 trace-id。

### 6. 用 SSE 做实时进度

进度通过 Server-Sent Events 推到浏览器。hub 支持每个 session 多个订阅者（比如多个标签页），对慢的消费者直接丢事件，而不是拖住审查流水线。

## 评估方法论

`eval/` 里有一批标注过的 PR diff 和对应的期望问题。运行器按用例、也按整体算 precision、recall、F1。

- **Mock 模式**（`go run ./cmd/eval/`）：用确定性的关键词审查器，用来验证评估框架本身（匹配、指标、不重复计数）。
- **真实模式**（`go run ./cmd/eval/ -real`）：走 gRPC 调 agent-go 认知面，衡量真实模型质量。
- **多 bug 用例**：近期的语料里一个用例会塞多个 bug、再掺一些非问题项，防止「一个显眼 bug」的用例把 recall 刷成虚高。
