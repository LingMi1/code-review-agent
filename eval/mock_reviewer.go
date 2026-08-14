// Package eval 提供 mock reviewer，用于在没有 agent-go cognition 时跑评估。
package eval

import (
	"encoding/json"
	"fmt"
	"strings"
)

// mockReview 是一个基于关键字的 mock 审查器。
// 它在 diff 中搜索已知 bug 模式并返回 FoundIssue。
// 生产环境中应替换为真实的 agent-go cognition gRPC 调用。
func mockReview(diff string, language string) ([]FoundIssue, error) {
	var issues []FoundIssue

	checks := []struct {
		pattern  string
		file     string
		line     int
		severity string
		category string
		title    string
	}{
		{"\"SELECT * FROM users WHERE name = '\" +", "user_handler.go", 42, "high", "security", "SQL injection vulnerability"},
		{"db.Query(\"SELECT", "user_handler.go", 42, "high", "security", "SQL injection risk in raw query"},
		{"fmt.Sprintf(\"SELECT", "user_handler.go", 42, "high", "security", "SQL injection in formatted query"},
		{`password = "`, "config.go", 12, "high", "security", "Hardcoded password"},
		{`secretKey = "`, "config.go", 15, "high", "security", "Hardcoded secret in config"},
		{`token = "`, "config.go", 15, "high", "security", "Hardcoded API token"},
		{`api_key = "`, "config.go", 15, "high", "security", "Hardcoded API key"},
		{"os.Open(", "file_handler.go", 30, "medium", "bug", "File opened without deferred close"},
		{"file, err :=", "file_handler.go", 30, "medium", "bug", "Potential unclosed file handle"},
		{"if err != nil", "file_handler.go", 32, "low", "style", "Missing error handling"},
		{"http.Get(", "fetch.go", 18, "medium", "performance", "HTTP request without timeout"},
		{"timeout = 0", "fetch.go", 18, "medium", "bug", "Zero timeout may cause hang"},
		{"go func()", "worker.go", 55, "high", "bug", "Race condition risk in goroutine"},
		{"sync.Mutex", "worker.go", 55, "medium", "bug", "Mutex usage needs review"},
		{`xss =`, "render.go", 25, "high", "security", "Potential XSS: unescaped output"},
		{"innerHTML", "render.go", 25, "high", "security", "innerHTML XSS vulnerability"},
		{`exec.Command(`, "executor.go", 10, "high", "security", "Command injection risk"},
		{`os.Exec(`, "executor.go", 10, "high", "security", "Command injection via os.Exec"},
		{`ioutil.ReadFile(`, "reader.go", 8, "medium", "bug", "ReadFile without size limit"},
		{`i := 0; i < 1000000`, "loop.go", 5, "medium", "performance", "Hardcoded loop bound"},
		{`for {`, "loop.go", 5, "high", "bug", "Infinite loop without break condition"},
		// 多 bug 用例：每个文件多个问题，line 为“目标行号 - 命中序号”以配合下方 fileHits 偏移。
		{`filepath.Join("/uploads", name)`, "upload_handler.go", 11, "high", "security", "Path traversal via unsanitized filename"},
		{`io.ReadAll(r.Body)`, "upload_handler.go", 11, "medium", "bug", "Unbounded request body read without size limit"},
		{`f, _ := os.Create(path)`, "upload_handler.go", 11, "medium", "bug", "Error from os.Create ignored"},
		{`SELECT * FROM items WHERE name LIKE '%`, "search_handler.go", 10, "high", "security", "SQL injection in raw query"},
		{`client := &http.Client{}`, "search_handler.go", 10, "medium", "performance", "HTTP client without timeout"},
		{`rows, err := db.Query(q)`, "search_handler.go", 13, "medium", "bug", "Error from db.Query ignored"},
		{`n, _ := strconv.Atoi(s)`, "parser.go", 8, "medium", "bug", "Error from strconv.Atoi ignored"},
		{`value := 100 / n`, "parser.go", 8, "medium", "bug", "Division by zero when n is 0"},
	}

	// 为每个 diff 文件行标记检查结果
	fileHits := make(map[string]int)

	for _, check := range checks {
		if strings.Contains(diff, check.pattern) {
			fileHits[check.file]++
			issues = append(issues, FoundIssue{
				File:     check.file,
				Line:     check.line + fileHits[check.file],
				Severity: check.severity,
				Category: check.category,
				Title:    check.title,
			})
		}
	}

	_ = language
	return issues, nil
}

// unmarshalAsIssues 从 JSON 字符串解析 issue 列表（用于 parseResult 的本地副本）。
func unmarshalAsIssues(data []byte) ([]FoundIssue, error) {
	var issues []FoundIssue
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// truncateFileName 截短文件名用于显示。
func truncateFileName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return "..." + name[len(name)-maxLen+3:]
}

// suppress unused warns
var _ = fmt.Sprintf
var _ = truncateFileName
