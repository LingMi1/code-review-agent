// Package sse 提供 Server-Sent Events 广播，用于实时推送审查进度。
package sse

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Event 是一条 SSE 事件。
type Event struct {
	Type string `json:"type"` // "review.started", "review.progress", "review.completed", "review.failed"
	Data any    `json:"data"`
}

// Hub 是 SSE 广播中心。同一个 session 支持多个订阅者（如多个浏览器标签页）。
type Hub struct {
	mu      sync.RWMutex
	subs    map[string]map[chan Event]struct{} // session_id → 订阅者 channel 集合
	allowed map[string]bool                    // 允许的跨域 origin 白名单（nil 表示不开放跨域）
}

// NewHub 创建新的 SSE hub。
func NewHub() *Hub {
	return &Hub{
		subs: make(map[string]map[chan Event]struct{}),
	}
}

// SetAllowedOrigins 设置允许跨域访问的 origin 白名单（复用主服务的 ALLOWED_ORIGINS）。
// 通常在服务启动、开始对外服务前调用一次。
func (h *Hub) SetAllowedOrigins(origins map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.allowed = origins
}

// Subscribe 注册一个 SSE 订阅。返回事件 channel 和取消函数。
// 取消函数只会关闭并移除该订阅者自己的 channel，不影响同一 session 的其他订阅者。
func (h *Hub) Subscribe(sessionID string) (<-chan Event, func()) {
	ch := make(chan Event, 32)

	h.mu.Lock()
	if h.subs[sessionID] == nil {
		h.subs[sessionID] = make(map[chan Event]struct{})
	}
	h.subs[sessionID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if set, ok := h.subs[sessionID]; ok {
				if _, exists := set[ch]; exists {
					delete(set, ch)
					close(ch)
				}
				if len(set) == 0 {
					delete(h.subs, sessionID)
				}
			}
		})
	}

	return ch, cancel
}

// Publish 发送一条事件到指定 session 的所有订阅者。
// 非阻塞：订阅者 channel 满时丢弃该事件，避免慢消费者拖垮广播。
func (h *Hub) Publish(sessionID string, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	set, ok := h.subs[sessionID]
	if !ok {
		return
	}
	for ch := range set {
		select {
		case ch <- evt:
		default:
			slog.Warn("sse: channel full, dropping event", "session", sessionID, "type", evt.Type)
		}
	}
}

// ServeHTTP 为 SSE 连接服务。
// URL: GET /api/reviews/stream?session={sessionID}
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session parameter", http.StatusBadRequest)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	h.mu.RLock()
	allowed := h.allowed
	h.mu.RUnlock()
	if origin := r.Header.Get("Origin"); origin != "" && allowed != nil && allowed[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch, cancel := h.Subscribe(sessionID)
	defer cancel()

	ctx := r.Context()
	ticker := time.NewTicker(15 * time.Second) // keepalive
	defer ticker.Stop()

	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data := evt.Data
			if s, ok := data.(string); ok {
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, s)
			} else {
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %v\n\n", evt.Type, data)
			}
			flusher.Flush()
		}
	}
}
