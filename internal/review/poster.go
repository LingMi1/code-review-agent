// Package review 将 Agent 输出解析为结构化 review 并投递到 GitHub。
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

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

// PostResult 是审查投递结果，供上层更新审查记录。
type PostResult struct {
	Issues  int    // 发现的问题数
	Summary string // 审查摘要
}

// Poster 是审查结果投递所需的 GitHub 操作子集。
type Poster interface {
	PostReview(ctx context.Context, owner, repo string, prNumber int, headSHA string, body string, comments []github.ReviewComment) error
	PostIssueComment(ctx context.Context, owner, repo string, prNumber int, body string) error
}

// ParseResult 解析单段 Agent 输出为结构化审查结果。失败返回 error，由调用方决定降级策略。
func ParseResult(text string) (*ReviewOutput, error) {
	return parseResult(text)
}

// MergeOutputs 合并分块审查的多段结构化结果：issues 拼接、summary 分段汇总。
func MergeOutputs(outputs []*ReviewOutput) *ReviewOutput {
	merged := &ReviewOutput{}
	summaries := make([]string, 0, len(outputs))
	for i, out := range outputs {
		if out == nil {
			continue
		}
		merged.Issues = append(merged.Issues, out.Issues...)
		if out.Summary != "" {
			summaries = append(summaries, fmt.Sprintf("Chunk %d: %s", i+1, out.Summary))
		}
	}
	merged.Summary = strings.Join(summaries, "\n")
	return merged
}

// Post 将已解析的结构化审查结果投递到 GitHub PR。
// 返回结构化结果（问题数 + 摘要），避免上层再次解析文本。
func Post(ctx context.Context, gh Poster, owner, repo string, prNumber int, headSHA string, out *ReviewOutput) (*PostResult, error) {
	if len(out.Issues) == 0 {
		if err := gh.PostIssueComment(ctx, owner, repo, prNumber, fmt.Sprintf(
			"## AI Code Review\n\n%s\n\n**No issues found.** Great job!",
			out.Summary,
		)); err != nil {
			return nil, err
		}
		return &PostResult{Issues: 0, Summary: out.Summary}, nil
	}

	// 构建 review body
	body := fmt.Sprintf("## AI Code Review\n\n%s\n\n### Issues Found\n\n", out.Summary)
	for i, issue := range out.Issues {
		severityEmoji := severityIcon(issue.Severity)
		body += fmt.Sprintf("%d. %s **%s** — `%s:%d` (%s/%s)\n   %s\n   > Suggestion: %s\n\n",
			i+1, severityEmoji, issue.Title, issue.File, issue.Line,
			issue.Severity, issue.Category,
			issue.Description,
			issue.Suggestion,
		)
	}

	// 尝试构造 inline comments
	comments := buildInlineComments(out.Issues)
	if len(comments) > 0 {
		if err := gh.PostReview(ctx, owner, repo, prNumber, headSHA, body, comments); err != nil {
			return nil, err
		}
	} else if err := gh.PostIssueComment(ctx, owner, repo, prNumber, body); err != nil {
		// 无法构造 inline comments → 降级为 issue comment
		return nil, err
	}

	return &PostResult{Issues: len(out.Issues), Summary: out.Summary}, nil
}

// ParseAndPost 解析 Agent 的审查结果并投递到 GitHub PR（单段输出的便捷入口）。
func ParseAndPost(ctx context.Context, gh Poster, owner, repo string, prNumber int, headSHA string, resultText string) (*PostResult, error) {
	out, err := ParseResult(resultText)
	if err != nil {
		// JSON 解析失败 → 降级为纯文本 comment
		slog.Warn("review: failed to parse structured output, falling back to plain text", "error", err)
		if err := gh.PostIssueComment(ctx, owner, repo, prNumber, formatPlainComment(resultText)); err != nil {
			return nil, err
		}
		return &PostResult{Issues: 0, Summary: truncateSummary(resultText)}, nil
	}
	return Post(ctx, gh, owner, repo, prNumber, headSHA, out)
}

// truncateSummary 截断摘要，用于降级路径的展示。
func truncateSummary(s string) string {
	const maxLen = 200
	if len(s) <= maxLen {
		return s
	}
	cut := s[:maxLen]
	// 按字节截断可能落在多字节 UTF-8 字符中间，回退到最近的合法 rune 边界。
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
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
		"## AI Code Review\n\n%s\n\n---\n*Powered by [agent-go](https://github.com/LingMi1/agent-go)* | Review ID: `%s`",
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
