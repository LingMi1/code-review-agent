package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// newTestClient 返回一个指向测试服务器 baseURL 的客户端。
func newTestClient(baseURL string) *Client {
	c := New("test-token")
	c.baseURL = baseURL
	return c
}

func TestPostReviewRetriesWithBodyIntact(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"comments"`) {
			t.Errorf("retry body lost payload: %s", string(body))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.PostReview(context.Background(), "owner", "repo", 1, "sha", "summary",
		[]ReviewComment{{Path: "a.go", Line: 1, Body: "bug"}})
	if err != nil {
		t.Fatalf("PostReview() error = %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2 (one rate-limit + one retry)", got)
	}
}

func TestPostIssueCommentChecksStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte("validation failed"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.PostIssueComment(context.Background(), "owner", "repo", 1, "hello")
	if err == nil {
		t.Fatal("PostIssueComment() expected error for HTTP 422")
	}
}

func TestPostIssueCommentSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.PostIssueComment(context.Background(), "owner", "repo", 1, "hello"); err != nil {
		t.Fatalf("PostIssueComment() error = %v", err)
	}
}

func TestPRDiffAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.PRDiff(context.Background(), "owner", "repo", 1); err == nil {
		t.Fatal("PRDiff() expected auth error for HTTP 401")
	}
}

func TestPRDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("diff --git a/a.go b/a.go\n"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.PRDiff(context.Background(), "owner", "repo", 1)
	if err != nil {
		t.Fatalf("PRDiff() error = %v", err)
	}
	if !strings.Contains(got, "diff --git") {
		t.Fatalf("PRDiff() = %q, want diff content", got)
	}
}
