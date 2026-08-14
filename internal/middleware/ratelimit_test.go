package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute, false)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute, false)
	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Fatal("third request should be blocked")
	}
}

func TestRateLimiterSeparatesIPs(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute, false)
	if !rl.Allow("1.1.1.1") {
		t.Fatal("first IP should be allowed")
	}
	if !rl.Allow("2.2.2.2") {
		t.Fatal("second IP should be allowed")
	}
	if rl.Allow("1.1.1.1") {
		t.Fatal("second request from first IP should be blocked")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute, false)
	h := RateLimit(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 第一次请求：通过
	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", rr1.Code)
	}

	// 第二次请求：被限流
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req1)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", rr2.Code)
	}
	if rr2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestClientIPFromXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	if ip := clientIP(r, true); ip != "10.0.0.1" {
		t.Fatalf("clientIP = %q, want 10.0.0.1", ip)
	}
}

func TestClientIPIgnoresXFFWhenUntrusted(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:54321"
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	if ip := clientIP(r, false); ip != "192.168.1.1" {
		t.Fatalf("clientIP = %q, want 192.168.1.1 (XFF must be ignored when untrusted)", ip)
	}
}

func TestClientIPFromRemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:54321"
	if ip := clientIP(r, false); ip != "192.168.1.1" {
		t.Fatalf("clientIP = %q, want 192.168.1.1", ip)
	}
}

func TestClientIPFromIPv6RemoteAddr(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[::1]:54321"
	if ip := clientIP(r, false); ip != "::1" {
		t.Fatalf("clientIP = %q, want ::1", ip)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	h := Recovery(panicHandler)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rr.Code)
	}
}

func TestRecoveryMiddlewarePassesThrough(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := Recovery(okHandler)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
}
