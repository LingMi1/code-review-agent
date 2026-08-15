package reviewer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/webhook"
)

// TestWebhookToPostE2E covers the full pipeline: webhook → fetch diff → chunk → cognition (mock) → post.
// It replicates the onPR adaptation logic from main.go (event → owner/repo/pr) to verify a review is produced end-to-end.
func TestWebhookToPostE2E(t *testing.T) {
	st := &mockStore{insertID: 42}
	gh := &mockGitHub{diff: sampleDiff}
	cog := &mockCognition{result: &cognition.ReviewResult{Text: sampleResult}}
	se := &mockSSE{}
	m := &mockMetrics{}

	svc := newService(st, cog, gh, se, m)

	wh := webhook.New("secret", func(ctx context.Context, event *webhook.PullRequestEvent) error {
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo: %s", event.RepoFullName)
		}
		return svc.Review(ctx, parts[0], parts[1], event.PRNumber, event.HeadSHA, 0)
	})

	body := []byte(`{"action":"opened","pull_request":{"number":7,"head":{"sha":"abc123"},"base":{"sha":"def456"}},"repository":{"full_name":"octo/repo","clone_url":"https://github.com/octo/repo.git"}}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-GitHub-Delivery", "delivery-e2e-1")
	req.Header.Set("X-Hub-Signature-256", signBody(body, "secret"))

	rec := httptest.NewRecorder()
	wh.ServeHTTP(rec, req)
	wh.Wait() // wait for the async review goroutine to finish

	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", rec.Code)
	}
	if !gh.posted {
		t.Error("expected review to be posted to GitHub")
	}
	if st.lastStatus != "success" {
		t.Errorf("review status = %q, want success", st.lastStatus)
	}
}

func signBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
