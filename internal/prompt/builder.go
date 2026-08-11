// Package prompt 构造发给 Agent 的 code review prompt。
package prompt

import (
	"fmt"
	"strings"

	"github.com/LingMi1/code-review-agent/internal/diff"
)

// BuildReviewPrompt 基于 PR diff 构造审查 prompt。
// 输出要求 Agent 返回结构化 JSON。
func BuildReviewPrompt(prTitle string, files []diff.FileDiff) string {
	var b strings.Builder

	b.WriteString("You are an expert code reviewer. Review the following pull request diff ")
	b.WriteString("and identify bugs, security vulnerabilities, performance issues, ")
	b.WriteString("and code style problems.\n\n")

	if prTitle != "" {
		b.WriteString(fmt.Sprintf("PR Title: %s\n\n", prTitle))
	}

	b.WriteString("## Changed Files\n\n")
	for _, f := range files {
		b.WriteString(fmt.Sprintf("- `%s` (%d lines changed)\n", f.FileName, f.Lines))
	}
	b.WriteString("\n## Diff\n\n")

	for _, f := range files {
		b.WriteString(fmt.Sprintf("### %s\n\n", f.FileName))
		b.WriteString("```diff\n")
		b.WriteString(hunkTrim(f))
		b.WriteString("\n```\n\n")
	}

	b.WriteString("## Instructions\n\n")
	b.WriteString("Analyze the code changes and output your review as a JSON object ")
	b.WriteString("with this exact structure:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "summary": "A 1-2 sentence summary of the overall changes",
  "issues": [
    {
      "file": "path/to/file.go",
      "line": 42,
      "severity": "high|medium|low",
      "category": "bug|security|performance|style",
      "title": "Short description of the issue",
      "description": "Detailed explanation of the problem",
      "suggestion": "How to fix it (code suggestion if applicable)"
    }
  ]
}`)
	b.WriteString("\n```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Only report real issues. Do not flag correct code.\n")
	b.WriteString("- `line` must be the line number in the NEW file (lines starting with '+').\n")
	b.WriteString("- If you find no issues, return an empty `issues` array.\n")
	b.WriteString("- Be specific: include exact line numbers and concrete code suggestions.\n")
	b.WriteString("- Focus on: correctness bugs > security > performance > style.\n")
	b.WriteString("- Skip issues already addressed in the diff itself.\n")
	b.WriteString("- Output ONLY the JSON object, no other text.\n")

	return b.String()
}

// hunkTrim 去掉文件的 diff 中可能超长的部分。
func hunkTrim(f diff.FileDiff) string {
	const maxLen = 6000
	s := f.Hunk
	if len(s) > maxLen {
		return s[:maxLen] + "\n... (truncated)"
	}
	return s
}

// TruncatePrompt 确保 prompt 总长不超过 maxBytes 字节。
func TruncatePrompt(prompt string, maxBytes int) string {
	if len(prompt) <= maxBytes {
		return prompt
	}
	cut := prompt[:maxBytes]
	// 在最后一个完整行处截断
	if idx := strings.LastIndex(cut, "\n"); idx > 0 {
		cut = cut[:idx]
	}
	return cut + "\n\n... (truncated due to size)"
}
