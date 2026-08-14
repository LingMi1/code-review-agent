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

// Hub 是 SSE 广播中心。
type Hub struct {
	mu       sync.RWMutex
	channels map[string]chan Event // session_id → event channel
	done     map[string]chan struct{}
	allowed  map[string]bool // 允许的跨域 origin 白名单（SetAllowedOrigins 配置，nil 表示不开放跨域）
}

// NewHub 创建新的 SSE hub。
func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]chan Event),
		done:     make(map[string]chan struct{}),
	}
}

// SetAllowedOrigins 设置允许跨域访问的 origin 白名单（复用主服务的 ALLOWED_ORIGINS）。
func (h *Hub) SetAllowedOrigins(origins map[string]bool) {
	h.allowed = origins
}

// Subscribe 注册一个 SSE 订阅。返回事件 channel 和取消函数。
func (h *Hub) Subscribe(sessionID string) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 32)
	done := make(chan struct{})
	h.channels[sessionID] = ch
	h.done[sessionID] = done

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.channels[sessionID]; ok {
			close(h.channels[sessionID])
			delete(h.channels, sessionID)
		}
		if _, ok := h.done[sessionID]; ok {
			close(h.done[sessionID])
			delete(h.done, sessionID)
		}
	}

	return ch, cancel
}

// Publish 发送一条事件到指定 session。
func (h *Hub) Publish(sessionID string, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch, ok := h.channels[sessionID]
	if !ok {
		return
	}

	select {
	case ch <- evt:
	default:
		slog.Warn("sse: channel full, dropping event", "session", sessionID, "type", evt.Type)
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
	if origin := r.Header.Get("Origin"); origin != "" && h.allowed != nil && h.allowed[origin] {
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
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data := evt.Data
			if s, ok := data.(string); ok {
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, s)
			} else {
				fmt.Fprintf(w, "event: %s\ndata: %v\n\n", evt.Type, data)
			}
			flusher.Flush()
		}
	}
}
