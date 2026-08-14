package main

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON: encode failed", "error", err)
	}
}

// enableCORS 为 API 响应添加 CORS 头，仅允许白名单内的 origin。
func enableCORS(w http.ResponseWriter, r *http.Request, origins map[string]bool) {
	origin := r.Header.Get("Origin")
	if origin != "" && origins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-ID")
}

// parseID 从 URL path 中提取数字 ID。无效时返回 (0, error)。
func parseID(path, prefix string) (int, error) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/")
	return strconv.Atoi(idStr)
}

// authorizeManualTrigger 校验手动触发请求的 API token。
// apiToken 为空时允许（开发模式）。
func authorizeManualTrigger(r *http.Request, apiToken string) bool {
	if apiToken == "" {
		return true
	}
	// 尝试 Bearer header
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		provided := []byte(strings.TrimPrefix(auth, prefix))
		if subtle.ConstantTimeCompare(provided, []byte(apiToken)) == 1 {
			return true
		}
	}
	// 尝试 X-API-Token header
	if xToken := r.Header.Get("X-API-Token"); xToken != "" {
		if subtle.ConstantTimeCompare([]byte(xToken), []byte(apiToken)) == 1 {
			return true
		}
	}
	return false
}
