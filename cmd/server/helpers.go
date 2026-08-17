package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// writeJSON 写入 JSON 响应（HTTP 200）。
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON: encode failed", "error", err)
	}
}

// writeError 写入 JSON 格式的错误响应。msg 对外可见，不得包含内部细节。
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		slog.Error("writeError: encode failed", "error", err)
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

// signStreamToken 签发一个绑定 session、短时有效的 HMAC 签名 token，
// 供 EventSource 通过 query 参数使用（EventSource 无法自定义 Authorization header）。
// 格式："{unix过期秒}.{hex(hmacSHA256(secret, session:exp))}"
func signStreamToken(secret, session string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	return strconv.FormatInt(exp, 10) + "." + streamHMAC(secret, session, exp)
}

// streamHMAC 计算 session 流的 HMAC 摘要。
func streamHMAC(secret, session string, exp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(session))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyStreamToken 校验 SSE 流 token：格式合法、未过期、签名匹配（常数时间比较）。
func verifyStreamToken(secret, session, token string) bool {
	expStr, sig, ok := strings.Cut(token, ".")
	if !ok || expStr == "" || sig == "" {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() >= exp {
		return false
	}
	expected := streamHMAC(secret, session, exp)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}

// sseAuth 为 SSE 流增加鉴权：secret 为空时放行（开发模式），否则校验 query 中的短时签名 token。
func sseAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret == "" {
			next.ServeHTTP(w, r)
			return
		}
		session := r.URL.Query().Get("session")
		if session == "" || !verifyStreamToken(secret, session, r.URL.Query().Get("token")) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
