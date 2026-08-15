package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	h := New("test-secret", nil)
	body := []byte(`{"action":"opened"}`)

	// correct signature passes
	if !h.verifySignature(body, sign(body, "test-secret")) {
		t.Error("correct signature should pass")
	}

	// wrong signature rejected
	if h.verifySignature(body, sign(body, "wrong-secret")) {
		t.Error("wrong signature should fail")
	}

	// empty header rejected (when secret is set)
	if h.verifySignature(body, "") {
		t.Error("empty signature header should fail when secret is set")
	}

	// missing prefix rejected
	if h.verifySignature(body, hex.EncodeToString([]byte("abc"))) {
		t.Error("missing sha256= prefix should fail")
	}

	// no secret (dev environment) skips verification
	noSecret := New("", nil)
	if !noSecret.verifySignature(body, "") {
		t.Error("empty secret should skip verification (pass)")
	}
}

func TestIsDuplicate(t *testing.T) {
	h := New("secret", nil)

	if h.isDuplicate("id-1") {
		t.Error("first delivery should NOT be duplicate")
	}
	if !h.isDuplicate("id-1") {
		t.Error("second delivery of same id SHOULD be duplicate")
	}
	if h.isDuplicate("id-2") {
		t.Error("different delivery id should NOT be duplicate")
	}
}

func TestParsePREvent(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 42,
			"head": {"sha": "abc123"},
			"base": {"sha": "def456"}
		},
		"repository": {
			"full_name": "LingMi1/agent-go",
			"clone_url": "https://github.com/LingMi1/agent-go.git"
		}
	}`)

	ev, err := parsePREvent(body)
	if err != nil {
		t.Fatalf("parsePREvent() error = %v", err)
	}
	if ev.Action != "opened" {
		t.Errorf("Action = %q, want opened", ev.Action)
	}
	if ev.PRNumber != 42 {
		t.Errorf("PRNumber = %d, want 42", ev.PRNumber)
	}
	if ev.RepoFullName != "LingMi1/agent-go" {
		t.Errorf("RepoFullName = %q, want LingMi1/agent-go", ev.RepoFullName)
	}
	if ev.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123", ev.HeadSHA)
	}
	if ev.BaseSHA != "def456" {
		t.Errorf("BaseSHA = %q, want def456", ev.BaseSHA)
	}
}

func TestParsePREventMissingFields(t *testing.T) {
	body := []byte(`{"action": "opened"}`)
	if _, err := parsePREvent(body); err == nil {
		t.Error("parsePREvent() should error on missing pull_request/repository")
	}
}
