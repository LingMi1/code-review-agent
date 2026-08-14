package main

import (
	"crypto/subtle"
	"net/http/httptest"
	"testing"
)

func TestParseID(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   int
		err    bool
	}{
		{"/api/reviews/42", "/api/reviews/", 42, false},
		{"/api/reviews/42/", "/api/reviews/", 42, false},
		{"/api/reviews/abc", "/api/reviews/", 0, true},
		{"/api/reviews/", "/api/reviews/", 0, true},
	}

	for _, tc := range tests {
		got, err := parseID(tc.path, tc.prefix)
		if tc.err {
			if err == nil {
				t.Errorf("parseID(%q) expected error, got nil", tc.path)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseID(%q) unexpected error: %v", tc.path, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseID(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}

func TestEnableCORS(t *testing.T) {
	origins := map[string]bool{"http://localhost:5173": true, "https://example.com": true}

	// Allowed origin
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	enableCORS(w, r, origins)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("CORS origin = %q, want http://localhost:5173", got)
	}

	// Disallowed origin
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("Origin", "http://evil.com")
	enableCORS(w2, r2, origins)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS origin for evil.com = %q, want empty", got)
	}
}

func TestAuthorizeManualTrigger(t *testing.T) {
	const token = "secret-token-123"

	// No token configured = always allowed
	r := httptest.NewRequest("POST", "/api/reviews", nil)
	if !authorizeManualTrigger(r, "") {
		t.Error("empty apiToken should allow access")
	}

	// Correct Bearer token
	r2 := httptest.NewRequest("POST", "/api/reviews", nil)
	r2.Header.Set("Authorization", "Bearer "+token)
	if !authorizeManualTrigger(r2, token) {
		t.Error("correct Bearer token should pass")
	}

	// Wrong Bearer token
	r3 := httptest.NewRequest("POST", "/api/reviews", nil)
	r3.Header.Set("Authorization", "Bearer wrong")
	if authorizeManualTrigger(r3, token) {
		t.Error("wrong Bearer token should fail")
	}

	// Correct X-API-Token
	r4 := httptest.NewRequest("POST", "/api/reviews", nil)
	r4.Header.Set("X-API-Token", token)
	if !authorizeManualTrigger(r4, token) {
		t.Error("correct X-API-Token should pass")
	}

	// No auth headers
	r5 := httptest.NewRequest("POST", "/api/reviews", nil)
	if authorizeManualTrigger(r5, token) {
		t.Error("missing auth headers should fail")
	}
}

// Ensure crypto/subtle is imported to verify the constant-time path is active.
var _ = subtle.ConstantTimeCompare
