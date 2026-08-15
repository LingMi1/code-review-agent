# 安全策略

> [English](SECURITY.md)

## 报告漏洞

**请不要**为安全问题开公开的 GitHub issue。

请通过以下任一方式私下报告：
- **GitHub 私密漏洞上报**（*Security* 标签页 → *Report a vulnerability*），或
- **邮件**：`LingMi1@users.noreply.github.com`

### 报告里写什么

- 问题描述及潜在影响
- 复现步骤（最小示例、涉及的端点/文件）
- 受影响的版本或 commit SHA
- 你建议的修复或缓解方案

## 响应时限

- **确认收到**：48 小时内。
- **初步评估**：5 个工作日内。
- 在公开披露前，会先和你协调修复或公告。

## 生产部署注意事项

部署 code-review-agent 时，把密钥当敏感信息对待：
- 设置强且唯一的 `WEBHOOK_SECRET`，用于校验进来的 GitHub webhook。
- 通过密钥管理器配置 `API_TOKEN`（以及 `GITHUB_TOKEN`、`LLM_API_KEY`）—— 别提交进仓库，也别打进镜像。
- 把 `ALLOWED_ORIGINS` 限制成你自己的前端源。

本项目采用 MIT 许可。安全报告由维护者 LingMi1 处理。
