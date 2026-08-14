# 架构设计

> [English](ARCHITECTURE.md)

本文档说明 **code-review-agent** 的设计以及关键决策背后的权衡取舍，面向想理解系统「为什么这样建」的审阅者和面试官。

## 概览

code-review-agent 是一个自动审查 Pull Request 的 GitHub App。它本质上是 Go 写的一层薄编排，通过 gRPC 把真正的「思考」委托给外部的 **agent-go cognition** 服务。

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
| `diff` | `internal/diff` | 将 GitHub unified diff 解析为结构化文件 |
| `reviewer` | `internal/reviewer` | 编排 diff → 分块 → 认知面 → 投递 |
| `cognition` | `internal/cognition` | 到 agent-go 的 gRPC 客户端，含熔断器 |
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
                    └─▶ 若总行数超过阈值：切分为多个 chunk（800 行）
                          └─▶ 逐块调用认知面
                                └─▶ 解析 JSON 输出
                                      └─▶ 合并所有 chunk 为一个 ReviewOutput
                                            └─▶ 投递 review + 评论（GitHub）
```

每条审查记录写入 SQLite，进度通过 SSE 广播，让仪表盘反映真实状态。

## 关键设计决策与权衡

### 1. 分块审查 vs 单段审查

大型 PR 会超出 LLM 上下文窗口。与其截断（会静默丢弃代码），不如把 diff 切成约 800 行一块，逐块独立审查。

- **权衡**：分块会丢失*跨文件*上下文（跨两块才成立的 bug 可能被漏掉或重复报告）。之所以接受，是因为块内正确性比全局视角更重要，而且 GitHub 评论本来就是按行定位的。

### 2. 通过 agent-go 角色做模型路由

agent-go 认知面按**角色**而非按请求路由模型。code-review-agent 把两种策略映射到这些角色：

- `react`（小 PR）→ `executor` 角色 → 便宜/快的模型（deepseek）
- `plan_execute`（大 PR）→ `planner` 角色 → 强模型

这样在不复刻 LLM 技术栈的前提下实现成本/质量控制。

### 3. gRPC 边界的熔断器 + 重试

认知面客户端用熔断器，让抖动的认知面服务快速失败，而不是级联超时。GitHub HTTP 调用有带 `Retry-After` 退避的有界重试；重试时会回卷请求体，POST 永远不会发出空 payload。

### 4. 要求 LLM 输出结构化 JSON

prompt 强制要求严格的 JSON schema，并附带一个 one-shot 示例。输出被防御性解析并在各 chunk 间合并。这让评估可量化（precision/recall），而不是自由文本。

### 5. 可观测性

- **追踪**：OpenTelemetry SDK + OTLP gRPC exporter，通过 W3C TraceContext 在 HTTP 与 gRPC 间传播。端到端 span 需要在 agent-go 中开启 OTel（`COGNITION_OTEL_ENABLED=true` + collector 地址）。
- **指标**：`/metrics` 提供 Prometheus text 格式（审查计数、延迟分位数、错误率）。
- **日志**：结构化 `slog`，自动注入 trace-id。

### 6. 用 SSE 做实时进度

进度通过 Server-Sent Events 推送到浏览器。hub 支持每个 session 多个订阅者（如多个标签页），对慢消费者丢弃事件而不是阻塞审查流水线。

## 评估方法论

`eval/` 含一批标注过的 PR diff 及其期望问题。运行器按用例和总体计算 precision、recall、F1。

- **Mock 模式**（`go run ./cmd/eval/`）：确定性关键词审查器，用于验证框架本身（匹配、指标、不重复计数）。
- **真实模式**（`go run ./cmd/eval/ -real`）：通过 gRPC 调用 agent-go 认知面，衡量真实模型质量。
- **多 bug 用例**：近期语料包含多个 bug 加上非问题项，防止「单个明显 bug」用例把 recall 刷成虚高。
