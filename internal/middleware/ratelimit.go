package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter 是一个基于滑动窗口的简单全局限流器。
// 每个 IP 在 window 内最多允许 maxRequests 次请求。
type RateLimiter struct {
	mu         sync.Mutex
	ipHits     map[string][]time.Time
	maxPerWin  int
	window     time.Duration
	trustProxy bool // 是否信任 X-Forwarded-For（仅当部署在受信反向代理后）
}

// NewRateLimiter 创建一个全局 IP 限流器。
//
//   maxPerWin  — 窗口内允许的最大请求数
//   window     — 时间窗口（如 1m）
//   trustProxy — 是否信任 X-Forwarded-For；未受信反向代理时应为 false，
//                否则攻击者可伪造该头绕过限流
func NewRateLimiter(maxPerWin int, window time.Duration, trustProxy bool) *RateLimiter {
	rl := &RateLimiter{
		ipHits:     make(map[string][]time.Time),
		maxPerWin:  maxPerWin,
		window:     window,
		trustProxy: trustProxy,
	}
	go rl.gc()
	return rl
}

// Allow 检查 clientIP 是否被允许。若允许则记录本次访问。
func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// 过滤掉过期记录
	hits := rl.ipHits[clientIP]
	fresh := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}

	if len(fresh) >= rl.maxPerWin {
		rl.ipHits[clientIP] = fresh
		return false
	}

	rl.ipHits[clientIP] = append(fresh, now)
	return true
}

// gc 定期清理过期的 IP 记录，防止内存无限增长。
func (rl *RateLimiter) gc() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for ip, hits := range rl.ipHits {
			var fresh []time.Time
			for _, t := range hits {
				if t.After(cutoff) {
					fresh = append(fresh, t)
				}
			}
			if len(fresh) == 0 {
				delete(rl.ipHits, ip)
			} else {
				rl.ipHits[ip] = fresh
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimit 返回一个中间件：对每个 client IP 限流。
// 超出限额时返回 429 Too Many Requests。
func RateLimit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r, rl.trustProxy)
			if !rl.Allow(ip) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP 提取客户端 IP。
//   - trustProxy 为 true 时，信任 X-Forwarded-For 的第一个值（受信反向代理场景）；
//   - 否则仅使用 RemoteAddr，避免攻击者伪造 X-Forwarded-For 绕过限流。
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
	}
	// RemoteAddr 形如 "host:port" 或 "[::1]:port"，用 SplitHostPort 正确剥离端口，
	// 避免对 IPv6 地址按首个 ':' 截断产生错误结果。
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
