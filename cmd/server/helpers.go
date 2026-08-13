package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/LingMi1/code-review-agent/internal/sse"
)

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

var (
	allowedOriginsOnce sync.Once
	allowedOriginsSet  map[string]bool
)

// allowedOrigins 从环境变量 ALLOWED_ORIGINS 读取允许的 CORS origin（逗号分隔）。
func allowedOrigins() map[string]bool {
	allowedOriginsOnce.Do(func() {
		raw := os.Getenv("ALLOWED_ORIGINS")
		if raw == "" {
			raw = "http://localhost:5173"
		}
		set := map[string]bool{}
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				set[o] = true
			}
		}
		allowedOriginsSet = set
	})
	return allowedOriginsSet
}

// enableCORS 为 API 响应添加 CORS 头，仅允许白名单内的 origin。
func enableCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && allowedOrigins()[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-ID")
}

// parseID 从 URL path 中提取数字 ID。
func parseID(path, prefix string) int {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.TrimSuffix(idStr, "/")
	id, _ := strconv.Atoi(idStr)
	return id
}

// sseEvent 构造一条 SSE 事件，data 用 JSON 序列化以避免手工拼接导致的转义问题。
func sseEvent(typ string, v any) sse.Event {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte("{}")
	}
	return sse.Event{Type: typ, Data: string(b)}
}

// authorizeManualTrigger 校验手动触发请求的 API token。
// 若未配置 API_TOKEN，则允许（开发模式）并记录警告。
func authorizeManualTrigger(r *http.Request) bool {
	token := os.Getenv("API_TOKEN")
	if token == "" {
		slog.Warn("API_TOKEN not set; manual review trigger is unauthenticated")
		return true
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(auth, prefix) {
		auth = strings.TrimPrefix(auth, prefix)
	}
	return auth == token || r.Header.Get("X-API-Token") == token
}
