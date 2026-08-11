# Session State — Code Review Agent

> 如果你回来时聊天记录丢了，读这个文件接上。

## 当前进度

| Phase | 状态 | 提交 |
|-------|------|------|
| Phase 1 核心链路 | ✅ 完成 | `bee4ee2` |
| Phase 2 生产化 | ✅ 完成 | `de7affc` |
| Phase 3 评估体系 | ⬜ 下一步 | — |
| Phase 4 React 面板 | ⬜ 未开始 | — |
| Phase 5 Plan-Execute + 成本 | ⬜ 未开始 | — |

## 仓库

- agent-go: https://github.com/LingMi1/agent-go
- code-review-agent: https://github.com/LingMi1/code-review-agent

## Phase 3 要做的事

构建 15-20 个标注 PR 测试用例，每个用例包含：
- 一段有已知 bug 的 diff
- 期望的 review 输出（文件、行号、问题类型）
- 跑 Agent → 对比期望输出 → 计算 precision/recall/F1

输出文件放在 `eval/` 目录：
```
eval/
├── corpus/    # 测试 PR diff（JSON）
├── expected/  # 期望输出（JSON）
└── runner.go  # 评估运行器
```

## 重连时对 AI 说的话

"我们正在做 Code Review Agent 项目，Phase 2 已完成，现在继续 Phase 3 评估体系。项目在 d:\agent\code-review-agent，已推送到 github.com/LingMi1/code-review-agent。"
