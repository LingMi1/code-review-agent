// Package review 将 Agent 输出解析为结构化 review 并投递到 GitHub。
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/LingMi1/code-review-agent/internal/github"

	"github.com/google/uuid"
)

// Issue 是一条代码审查发现的问题。
type Issue struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Severity    string `json:"severity"`    // high, medium, low
	Category    string `json:"category"`    // bug, security, performance, style
	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// ReviewOutput 是 Agent 输出的结构化审查结果。
type ReviewOutput struct {
	Summary string  `json:"summary"`
	Issues  []Issue `json:"issues"`
}

// ParseAndPost 解析 Agent 的审查结果并投递到 GitHub PR。
func ParseAndPost(ctx context.Context, gh *github.Client, owner, repo string, prNumber int, headSHA string, resultText string) error {
	review, err := parseResult(resultText)
	if err != nil {
		// JSON 解析失败 → 降级为纯文本 comment
		slog.Warn("review: failed to parse structured output, falling back to plain text", "error", err)
		return gh.PostIssueComment(ctx, owner, repo, prNumber, formatPlainComment(resultText))
	}

	if len(review.Issues) == 0 {
		return gh.PostIssueComment(ctx, owner, repo, prNumber, fmt.Sprintf(
			"## AI Code Review\n\n%s\n\n**No issues found.** Great job!",
			review.Summary,
		))
	}

	// 构建 review body
	body := fmt.Sprintf("## AI Code Review\n\n%s\n\n### Issues Found\n\n", review.Summary)
	for i, issue := range review.Issues {
		severityEmoji := severityIcon(issue.Severity)
		body += fmt.Sprintf("%d. %s **%s** — `%s:%d` (%s/%s)\n   %s\n   > Suggestion: %s\n\n",
			i+1, severityEmoji, issue.Title, issue.File, issue.Line,
			issue.Severity, issue.Category,
			issue.Description,
			issue.Suggestion,
		)
	}

	// 尝试构造 inline comments
	comments := buildInlineComments(review.Issues)
	if len(comments) > 0 {
		return gh.PostReview(ctx, owner, repo, prNumber, headSHA, body, comments)
	}

	// 无法构造 inline comments → 降级为 issue comment
	return gh.PostIssueComment(ctx, owner, repo, prNumber, body)
}

// parseResult 尝试从 LLM 输出中提取 JSON。
func parseResult(text string) (*ReviewOutput, error) {
	// 1. 尝试直接解析
	var out ReviewOutput
	if err := json.Unmarshal([]byte(text), &out); err == nil {
		return &out, nil
	}

	// 2. 尝试从 markdown code block 中提取 JSON
	jsonBlock := extractJSONBlock(text)
	if jsonBlock != "" {
		if err := json.Unmarshal([]byte(jsonBlock), &out); err == nil {
			return &out, nil
		}
	}

	return nil, fmt.Errorf("failed to parse review JSON from output (%d chars)", len(text))
}

// extractJSONBlock 从文本中提取第一个 ```json ... ``` 代码块。
func extractJSONBlock(text string) string {
	marker := strings.Index(text, "```json")
	if marker == -1 {
		marker = strings.Index(text, "```")
	}
	if marker == -1 {
		return ""
	}

	// 跳过 marker 行末尾的换行符，定位 JSON 内容起点。
	// 注意：nl 是相对于 text[marker:] 的偏移，需加回 marker 才是绝对位置。
	nl := strings.Index(text[marker:], "\n")
	if nl == -1 {
		return ""
	}
	text = text[marker+nl+1:]

	end := strings.Index(text, "```")
	if end == -1 {
		return text
	}
	return text[:end]
}

// buildInlineComments 将 issues 转换为 GitHub inline review comments。
func buildInlineComments(issues []Issue) []github.ReviewComment {
	var comments []github.ReviewComment
	for _, issue := range issues {
		if issue.File == "" || issue.Line <= 0 {
			continue
		}
		commentBody := fmt.Sprintf("**%s** [%s/%s]\n\n%s\n\n> Suggestion: %s",
			issue.Title, issue.Severity, issue.Category,
			issue.Description,
			issue.Suggestion,
		)
		comments = append(comments, github.ReviewComment{
			Path: issue.File,
			Line: issue.Line,
			Body: commentBody,
		})
	}
	return comments
}

// formatPlainComment 格式化纯文本审查结果（降级路径）。
func formatPlainComment(resultText string) string {
	return fmt.Sprintf(
		"## AI Code Review\n\n%s\n\n---\n*Powered by [agent-go](https://github.com/yourname/agent-go)* | Review ID: `%s`",
		resultText,
		uuid.New().String()[:8],
	)
}

// severityIcon 返回 severity 对应的图标。
func severityIcon(severity string) string {
	switch severity {
	case "high":
		return "🔴"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "⚪"
	}
}
