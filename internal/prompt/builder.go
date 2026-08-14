// Package prompt 构造发给 Agent 的 code review prompt。
package prompt

import (
	"fmt"
	"strings"
	"unicode/utf8"

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

	writeJSONResponseFormat(&b)
	return b.String()
}

// BuildPlanExecutePrompt 构造 Plan-Execute 模式的审查 prompt。
// Plan-Execute 将审查拆解为多个子任务并行执行后汇总。
func BuildPlanExecutePrompt(prTitle string, files []diff.FileDiff) string {
	var b strings.Builder

	b.WriteString("You are an expert code reviewer analyzing a large pull request. ")
	b.WriteString("Use plan-execute mode: first create a plan decomposing the review ")
	b.WriteString("into independent sub-tasks, then execute each sub-task, and finally ")
	b.WriteString("aggregate the results into a single review.\n\n")

	if prTitle != "" {
		b.WriteString(fmt.Sprintf("PR Title: %s\n\n", prTitle))
	}

	b.WriteString("## Review Plan\n\n")
	b.WriteString("Decompose the review into these sub-tasks:\n")
	b.WriteString("1. **Security Review** — Scan for SQL injection, XSS, hardcoded credentials, ")
	b.WriteString("command injection, path traversal, and insecure dependencies.\n")
	b.WriteString("2. **Bug Detection** — Identify null pointer dereferences, race conditions, ")
	b.WriteString("resource leaks (unclosed files, connections), missing error handling, ")
	b.WriteString("and logic errors.\n")
	b.WriteString("3. **Performance Analysis** — Check for inefficient loops, unnecessary allocations, ")
	b.WriteString("missing connection pooling, large memory footprints, and blocking I/O.\n")
	b.WriteString("4. **Code Quality** — Evaluate naming conventions, code organization, DRY violations, ")
	b.WriteString("excessive complexity, and missing test coverage indicators.\n\n")
	b.WriteString("Execute each sub-task independently. Found issues should include the file, ")
	b.WriteString("line number, and category (security/bug/performance/style).\n\n")

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

	writeJSONResponseFormat(&b)
	return b.String()
}

// writeJSONResponseFormat 写入 JSON 响应格式说明（reused by both prompt builders）。
func writeJSONResponseFormat(b *strings.Builder) {
	b.WriteString("## Response Format\n\n")
	b.WriteString("Output your review as a JSON object with this exact structure:\n\n")
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
	b.WriteString("Example:\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "summary": "Adds user lookup by name; the query is built by string concatenation.",
  "issues": [
    {
      "file": "user_handler.go",
      "line": 43,
      "severity": "high",
      "category": "security",
      "title": "SQL injection via string concatenation",
      "description": "The query concatenates user input directly, allowing SQL injection.",
      "suggestion": "Use a parameterized query: db.QueryRow(\"SELECT ... WHERE name = ?\", name)"
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
	b.WriteString("- Output ONLY the JSON object, no other text.\n")
}

// hunkTrim 去掉文件的 diff 中可能超长的部分。
func hunkTrim(f diff.FileDiff) string {
	const maxLen = 6000
	s := f.Hunk
	if len(s) <= maxLen {
		return s
	}
	cut := s[:maxLen]
	// 按字节截断可能落在多字节 UTF-8 字符中间，回退到最近的合法 rune 边界。
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "\n... (truncated)"
}

// TruncatePrompt 确保 prompt 总长不超过 maxBytes 字节。
func TruncatePrompt(prompt string, maxBytes int) string {
	if len(prompt) <= maxBytes {
		return prompt
	}
	cut := prompt[:maxBytes]
	// 按字节截断可能落在多字节 UTF-8 字符中间，回退到最近的合法 rune 边界。
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	// 在最后一个完整行处截断
	if idx := strings.LastIndex(cut, "\n"); idx > 0 {
		cut = cut[:idx]
	}
	return cut + "\n\n... (truncated due to size)"
}
