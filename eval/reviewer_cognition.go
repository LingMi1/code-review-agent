// Package eval 提供通过 agent-go 认知面（真实 LLM）审查 diff 的 ReviewFunc。
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
)

// cognitionIssue 是 LLM 结构化输出中的单条 issue。
type cognitionIssue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Title    string `json:"title"`
}

// cognitionOutput 是 LLM 的结构化审查结果。
type cognitionOutput struct {
	Summary string           `json:"summary"`
	Issues  []cognitionIssue `json:"issues"`
}

// CognitionReview 返回一个调用 agent-go 认知面（DeepSeek/Claude）的审查函数。
// 与 mockReview 不同，这里走真实 gRPC 链路，得到 LLM 的审查结果。
func CognitionReview(addr string) (ReviewFunc, error) {
	client, err := cognition.New(addr)
	if err != nil {
		return nil, err
	}
	return func(diffStr, language string) ([]FoundIssue, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		prompt := buildEvalPrompt(diffStr, language)
		result, err := client.RunReview(ctx, cognition.ReviewRequest{
			SessionID: fmt.Sprintf("eval-%d", time.Now().UnixNano()),
			Query:     prompt,
			AgentType: "react",
			MaxSteps:  5,
		})
		if err != nil {
			return nil, fmt.Errorf("cognition review: %w", err)
		}
		return parseCognitionOutput(result.Text)
	}, nil
}

// buildEvalPrompt 构造评估用的审查 prompt（直接接收原始 diff 字符串）。
func buildEvalPrompt(diffStr, language string) string {
	var b strings.Builder
	b.WriteString("You are an expert code reviewer. Review the following pull request diff ")
	b.WriteString("and identify bugs, security vulnerabilities, performance issues, and code style problems.\n\n")
	if language != "" {
		b.WriteString(fmt.Sprintf("Language: %s\n\n", language))
	}
	b.WriteString("## Diff\n\n```diff\n")
	b.WriteString(diffStr)
	b.WriteString("\n```\n\n")
	b.WriteString("## Response Format\n\n")
	b.WriteString("Output your review as a JSON object with this exact structure:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "summary": "A 1-2 sentence summary",
  "issues": [
    {
      "file": "path/to/file.go",
      "line": 42,
      "severity": "high|medium|low",
      "category": "bug|security|performance|style",
      "title": "Short description",
      "description": "Detailed explanation",
      "suggestion": "How to fix it"
    }
  ]
}`)
	b.WriteString("\n```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Respond in English only (including title, description, suggestion).\n")
	b.WriteString("- Only report real issues. Do not flag correct code.\n")
	b.WriteString("- `line` must be the line number in the NEW file (lines starting with '+').\n")
	b.WriteString("- Be precise about line numbers: count from the diff hunk header (e.g. @@ +42 @@).\n")
	b.WriteString("- `category` must be exactly one of: bug, security, performance, style.\n")
	b.WriteString("- If you find no issues, return an empty `issues` array.\n")
	b.WriteString("- Output ONLY the JSON object, no other text.\n")
	return b.String()
}

// parseCognitionOutput 从 LLM 输出中解析 issues，转成 []FoundIssue。
func parseCognitionOutput(text string) ([]FoundIssue, error) {
	var out cognitionOutput
	if err := json.Unmarshal([]byte(text), &out); err == nil {
		return toFoundIssues(out.Issues), nil
	}

	// 尝试从 markdown code block 中提取 JSON
	if block := extractJSONBlock(text); block != "" {
		if err := json.Unmarshal([]byte(block), &out); err == nil {
			return toFoundIssues(out.Issues), nil
		}
	}

	return nil, fmt.Errorf("failed to parse cognition output (%d chars)", len(text))
}

func toFoundIssues(issues []cognitionIssue) []FoundIssue {
	found := make([]FoundIssue, 0, len(issues))
	for _, iss := range issues {
		found = append(found, FoundIssue{
			File:     iss.File,
			Line:     iss.Line,
			Severity: iss.Severity,
			Category: iss.Category,
			Title:    iss.Title,
		})
	}
	return found
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
