package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter 是一个基于滑动窗口的简单全局限流器。
// 每个 IP 在 window 内最多允许 maxRequests 次请求。
type RateLimiter struct {
	mu          sync.Mutex
	ipHits      map[string][]time.Time
	maxPerWin   int
	window      time.Duration
}

// NewRateLimiter 创建一个全局 IP 限流器。
//
//   maxPerWin — 窗口内允许的最大请求数
//   window    — 时间窗口（如 1m）
func NewRateLimiter(maxPerWin int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		ipHits:    make(map[string][]time.Time),
		maxPerWin: maxPerWin,
		window:    window,
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
			ip := clientIP(r)
			if !rl.Allow(ip) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP 提取客户端 IP（支持反向代理的 X-Forwarded-For）。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := indexOfByte(xff, ','); idx > 0 {
			return xff[:idx]
		}
		return xff
	}
	if addr := r.RemoteAddr; addr != "" {
		if idx := indexOfByte(addr, ':'); idx > 0 {
			return addr[:idx]
		}
		return addr
	}
	return "unknown"
}

// indexOfByte 返回字节 b 在字符串 s 中首次出现的索引，未找到返回 -1。
func indexOfByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
