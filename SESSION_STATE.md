# Session State — Code Review Agent

> 如果你回来时聊天记录丢了，读这个文件接上。

## 当前进度

| Phase | 状态 | 提交 |
|-------|------|------|
| Phase 1 核心链路 | ✅ 完成 | `bee4ee2` |
| Phase 2 生产化 | ✅ 完成 | `de7affc` |
| Phase 3 评估体系 | ✅ 完成 | 最新提交 |
| Phase 4 React 面板 | ⬜ 下一步 | — |
| Phase 5 Plan-Execute + 成本 | ⬜ 未开始 | — |

## 仓库

- agent-go: https://github.com/LingMi1/agent-go
- code-review-agent: https://github.com/LingMi1/code-review-agent

## Phase 3 已交付

- `eval/eval.go` — 数据结构（EvalCase / ExpectedResult / FoundIssue / CaseResult / Report）
- `eval/runner.go` — 评估运行器：加载用例、逐用例匹配、计算 precision/recall/F1
- `eval/mock_reviewer.go` — 基于关键字的 mock 审查器（离线验证用）
- `eval/corpus/` — 15 个标注测试 PR（JSON diff）
- `eval/expected/` — 15 个对应期望输出
- `cmd/eval/main.go` — 运行入口：`go run ./cmd/eval/`
- `eval/report.json` — 评估报告输出

用例覆盖：
- security: SQL注入、硬编码密码、命令注入、XSS、路径穿越、日志敏感数据、输入验证缺失、明文密码存储
- bug: 未关闭文件句柄、缺少错误处理、race condition、nil 指针、无限循环、整数溢出、ReadFile OOM

## Phase 4 要做的事

React 面板：
- 审查列表页：所有 PR 审查记录（状态、时间、问题数）
- 审查详情页：单次审查完整结果（按文件分组的问题列表 + 严重程度标签）
- 技术栈：React + Vite + Tailwind + shadcn/ui（和 agent-go 保持一致）
- 复用 agent-go 的 SSE 逻辑做实时审查流展示

## 重连时对 AI 说的话

"我们正在做 Code Review Agent 项目，Phase 3 评估体系已完成，现在继续 Phase 4 React 面板。项目在 d:\agent\code-review-agent，已推送到 github.com/LingMi1/code-review-agent。"
