package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// enableCORS 为 API 响应添加 CORS 头。
func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Trace-ID")
}

// parseID 从 URL path 中提取数字 ID。
func parseID(path, prefix string) int {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.TrimSuffix(idStr, "/")
	id, _ := strconv.Atoi(idStr)
	return id
}

// parseIssueCount 估算 Agent 输出中发现了多少问题。
func parseIssueCount(resultText string) int {
	count := 0
	for _, line := range strings.Split(resultText, "\n") {
		if strings.Contains(line, `"file"`) {
			count++
		}
	}
	return count
}

// parseSummary 从 Agent 输出中提取 summary 字段。
func parseSummary(resultText string) string {
	idx := strings.Index(resultText, `"summary"`)
	if idx == -1 {
		return ""
	}
	start := strings.Index(resultText[idx:], `"`)
	if start == -1 {
		return ""
	}
	start += idx + 1
	end := strings.Index(resultText[start+1:], `"`)
	if end == -1 {
		return resultText[start : start+200]
	}
	summary := resultText[start+1 : start+1+end]
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}
	return summary
}
