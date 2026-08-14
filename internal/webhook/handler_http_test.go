package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestServeHTTPRejectsGET verifies that non-POST methods are rejected.
func TestServeHTTPRejectsGET(t *testing.T) {
	h := New("secret", nil)
	req := httptest.NewRequest("GET", "/webhook", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /webhook: got %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// TestServeHTTPRejectsBadSignature verifies that invalid signatures are rejected.
func TestServeHTTPRejectsBadSignature(t *testing.T) {
	h := New("secret", nil)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "pull_request")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("bad signature: got %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestServeHTTPIgnoresNonPREvents verifies that non-pull_request events return 200.
func TestServeHTTPIgnoresNonPREvents(t *testing.T) {
	h := New("", nil) // no secret = skip verification
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "del-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("push event: got %d, want 200", rr.Code)
	}
}

// TestServeHTTPProcessesPREvent verifies the full webhook → handler flow.
func TestServeHTTPProcessesPREvent(t *testing.T) {
	secret := "test-secret"
	var gotCtx context.Context
	var gotAction string

	h := New(secret, func(ctx context.Context, event *PullRequestEvent) error {
		gotCtx = ctx
		gotAction = event.Action
		return nil
	})

	body := []byte(`{
		"action": "opened",
		"pull_request": {"number": 42, "head": {"sha": "abc"}, "base": {"sha": "def"}},
		"repository": {"full_name": "owner/repo", "clone_url": "https://github.com/owner/repo.git"}
	}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "del-2")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("valid PR: got %d, want 200", rr.Code)
	}

	// Wait for async goroutine
	h.Wait()

	if gotAction != "opened" {
		t.Errorf("handler action = %q, want opened", gotAction)
	}
	if gotCtx == nil {
		t.Error("handler context should not be nil")
	}
}
