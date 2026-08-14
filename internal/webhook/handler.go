// Package webhook 处理 GitHub webhook 事件：签名验证、事件分发、去重。
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PRHandler 是收到 PR 事件时的回调。携带 context 以传播 trace 和取消信号。
type PRHandler func(ctx context.Context, event *PullRequestEvent) error

// PullRequestEvent 是 webhook 中 pull_request 事件的精简表示。
type PullRequestEvent struct {
	Action       string // "opened", "synchronize", "reopened"
	PRNumber     int
	RepoFullName string // "owner/repo"
	CloneURL     string // 可用于 git clone
	HeadSHA      string
	BaseSHA      string
	DeliveryID   string // X-GitHub-Delivery header，用于去重
}

// Handler 处理 GitHub webhook 请求。
type Handler struct {
	secret    string
	onPR      PRHandler
	wg        sync.WaitGroup       // 跟踪 in-flight review goroutine
	mu        sync.Mutex
	seen      map[string]time.Time // delivery_id → 首次接收时间
	seenClean time.Time            // 上次清理 seen 的时间
}

// New 创建一个新的 webhook handler。
func New(secret string, onPR PRHandler) *Handler {
	return &Handler{
		secret:    secret,
		onPR:      onPR,
		seen:      make(map[string]time.Time),
		seenClean: time.Now(),
	}
}

// Wait 等待所有 in-flight review goroutine 完成（用于优雅关闭）。
func (h *Handler) Wait() {
	h.wg.Wait()
}

// ServeHTTP 实现 http.Handler。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	eventType := r.Header.Get("X-GitHub-Event")
	sigHeader := r.Header.Get("X-Hub-Signature-256")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		slog.Error("webhook: read body", "error", err)
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// 1. HMAC-SHA256 签名验证
	if !h.verifySignature(body, sigHeader) {
		slog.Warn("webhook: signature verification failed", "delivery", deliveryID)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. 去重：GitHub 可能重试 webhook 投递
	if h.isDuplicate(deliveryID) {
		slog.Info("webhook: duplicate delivery, skipped", "delivery", deliveryID)
		w.WriteHeader(http.StatusOK)
		return
	}

	slog.Info("webhook: received", "delivery", deliveryID, "event", eventType)

	// 3. 只处理 pull_request 事件
	if eventType != "pull_request" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 4. 解析 payload
	payload, err := parsePREvent(body)
	if err != nil {
		slog.Error("webhook: parse payload", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payload.DeliveryID = deliveryID

	// 5. 只处理 opened / synchronize / reopened
	switch payload.Action {
	case "opened", "synchronize", "reopened":
		// process
	default:
		slog.Info("webhook: skipping non-review action", "action", payload.Action)
		w.WriteHeader(http.StatusOK)
		return
	}

	// 6. 异步处理（立即返回 200 给 GitHub，避免 webhook 超时）
	// 使用 context.WithoutCancel 保留 trace 链路但脱离请求生命周期
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		ctx := context.WithoutCancel(r.Context())
		if err := h.onPR(ctx, payload); err != nil {
			slog.Error("webhook: PR handler failed", "pr", payload.PRNumber, "error", err)
		}
	}()

	w.WriteHeader(http.StatusOK)
}

// verifySignature 验证 X-Hub-Signature-256 header。
func (h *Handler) verifySignature(body []byte, sigHeader string) bool {
	if h.secret == "" || sigHeader == "" {
		return h.secret == "" // 开发环境无 secret 时跳过验证
	}
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}
	sigBytes, err := hex.DecodeString(sigHeader[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, sigBytes)
}

// isDuplicate 检查 delivery_id 是否已处理（去重）。
func (h *Handler) isDuplicate(deliveryID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 每 10 分钟清理一次超过 1 小时的记录
	if time.Since(h.seenClean) > 10*time.Minute {
		cutoff := time.Now().Add(-1 * time.Hour)
		for id, t := range h.seen {
			if t.Before(cutoff) {
				delete(h.seen, id)
			}
		}
		h.seenClean = time.Now()
	}

	if _, exists := h.seen[deliveryID]; exists {
		return true
	}
	h.seen[deliveryID] = time.Now()
	return false
}
