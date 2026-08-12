# Session State — Code Review Agent

> 如果你回来时聊天记录丢了，读这个文件接上。

## 当前进度

| Phase | 状态 | 提交 |
|-------|------|------|
| Phase 1 核心链路 | ✅ 完成 | `bee4ee2` |
| Phase 2 生产化 | ✅ 完成 | `de7affc` |
| Phase 3 评估体系 | ✅ 完成 | Phase 3 commit |
| Phase 4 React 面板 | ✅ 完成 | 最新提交 |
| Phase 5 Plan-Execute + 成本 | ⬜ 下一步 | — |

## 仓库

- agent-go: https://github.com/LingMi1/agent-go
- code-review-agent: https://github.com/LingMi1/code-review-agent

## Phase 4 已交付

Go 后端新增：
- `internal/sse/hub.go` — SSE 广播中心（Subscribe/Publish/ServeHTTP）
- `cmd/server/main.go` — 集成 SSE hub + CORS + defer 统一失败事件推送
- `cmd/server/helpers.go` — 新增 enableCORS

React 前端（`web/`）：
- Vite + React 18 + TypeScript + Tailwind CSS + React Router
- 审查列表页：自动轮询、状态徽标、issues 数量
- 审查详情页：统计卡片、Live Agent Activity SSE 实时流
- API 层：fetchReviews / fetchReview / subscribeSSE
- Vite proxy 将 /api 转发到 localhost:8080

## Phase 5 要做的事

Plan-Execute 多 Agent 审查 + 成本优化：
- 把 agent_type 从 "react" 改成 "plan_execute"
- diff 自动拆解为子任务（安全/风格/测试覆盖）并行审查
- 模型路由：简单 PR DeepSeek / 复杂 PR Claude
- diff 过滤优化：跳过更多无意义文件

## 启动方式

```bash
# 后端
cd d:\agent\code-review-agent
$env:GOPROXY="https://goproxy.cn,direct"
go run ./cmd/server/

# 前端（另一个终端）
cd d:\agent\code-review-agent\web
npm install
npm run dev
```

## 重连时对 AI 说的话

"我们正在做 Code Review Agent 项目，Phase 4 React 面板已完成，现在继续 Phase 5。项目在 d:\agent\code-review-agent，已推送到 github.com/LingMi1/code-review-agent。"
