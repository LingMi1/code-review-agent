package reviewer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/github"
	"github.com/LingMi1/code-review-agent/internal/sse"
)

// --- mocks ---

type mockStore struct {
	mu          sync.Mutex
	insertID    int64
	lastStatus  string
	lastIssues  int
	lastSummary string
	updateCalls int
}

func (m *mockStore) InsertReview(prNumber int, repoURL, headSHA string) (int64, error) {
	return m.insertID, nil
}

func (m *mockStore) UpdateReview(id int64, status string, issues int, summary, duration, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastStatus = status
	m.lastIssues = issues
	m.lastSummary = summary
	m.updateCalls++
	return nil
}

func (m *mockStore) AuditLog(action string, prNumber int, actor, detail string) {}

type mockCognition struct {
	result *cognition.ReviewResult
	err    error
}

func (m *mockCognition) RunReview(ctx context.Context, req cognition.ReviewRequest) (*cognition.ReviewResult, error) {
	return m.result, m.err
}

func (m *mockCognition) RunPlanExecuteReview(ctx context.Context, sessID, query string, maxSteps int32) (*cognition.ReviewResult, error) {
	return m.result, m.err
}

type mockGitHub struct {
	diff        string
	diffErr     error
	postErr     error
	panicOnDiff bool
	posted      bool
}

func (m *mockGitHub) PRDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	if m.panicOnDiff {
		panic("boom")
	}
	return m.diff, m.diffErr
}

func (m *mockGitHub) PostReview(ctx context.Context, owner, repo string, prNumber int, headSHA, body string, comments []github.ReviewComment) error {
	m.posted = true
	return m.postErr
}

func (m *mockGitHub) PostIssueComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	m.posted = true
	return m.postErr
}

type mockSSE struct {
	events []sse.Event
}

func (m *mockSSE) Publish(sessionID string, evt sse.Event) {
	m.events = append(m.events, evt)
}

type mockMetrics struct {
	success  int
	failures int
	cogErrs  int
}

func (m *mockMetrics) RecordSuccess(latencyMs int64, outputBytes int, issuesFound int) { m.success++ }
func (m *mockMetrics) RecordFailure()                                                    { m.failures++ }
func (m *mockMetrics) RecordCognitionError()                                             { m.cogErrs++ }

// --- fixtures ---

const sampleDiff = `diff --git a/foo.go b/foo.go
index 1111111..2222222 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,3 @@
 func main() {
-    old
+    new
 }
`

const sampleResult = `{"summary":"Looks good overall","issues":[{"file":"foo.go","line":10,"severity":"medium","category":"bug","title":"nil pointer","description":"missing nil check","suggestion":"add a nil check"}]}`

func newService(st Store, cog Cognition, gh GitHub, se SSE, m Metrics) *Service {
	return New(st, cog, gh, se, m, 2)
}

// --- tests ---

func TestReviewSuccess(t *testing.T) {
	st := &mockStore{insertID: 42}
	gh := &mockGitHub{diff: sampleDiff}
	cog := &mockCognition{result: &cognition.ReviewResult{Text: sampleResult, Duration: 123 * time.Millisecond}}
	se := &mockSSE{}
	m := &mockMetrics{}

	svc := newService(st, cog, gh, se, m)
	if err := svc.Review(context.Background(), "owner", "repo", 1, "abc123", 0); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if st.lastStatus != "success" {
		t.Fatalf("expected status success, got %q", st.lastStatus)
	}
	if st.lastIssues != 1 {
		t.Fatalf("expected 1 issue, got %d", st.lastIssues)
	}
	if !gh.posted {
		t.Fatalf("expected review to be posted to GitHub")
	}
	if m.success != 1 {
		t.Fatalf("expected 1 success metric, got %d", m.success)
	}
}

func TestReviewFetchDiffError(t *testing.T) {
	st := &mockStore{insertID: 1}
	gh := &mockGitHub{diffErr: errors.New("github api down")}
	cog := &mockCognition{}
	se := &mockSSE{}
	m := &mockMetrics{}

	svc := newService(st, cog, gh, se, m)
	err := svc.Review(context.Background(), "owner", "repo", 1, "abc", 0)
	if err == nil {
		t.Fatal("expected error from PRDiff, got nil")
	}
	if st.lastStatus != "failed" {
		t.Fatalf("expected status failed, got %q", st.lastStatus)
	}
	if m.failures == 0 {
		t.Fatal("expected failure metric to be recorded")
	}
}

func TestReviewCognitionError(t *testing.T) {
	st := &mockStore{insertID: 1}
	gh := &mockGitHub{diff: sampleDiff}
	cog := &mockCognition{err: errors.New("cognition unavailable")}
	se := &mockSSE{}
	m := &mockMetrics{}

	svc := newService(st, cog, gh, se, m)
	err := svc.Review(context.Background(), "owner", "repo", 1, "abc", 0)
	if err == nil {
		t.Fatal("expected error from cognition, got nil")
	}
	if st.lastStatus != "failed" {
		t.Fatalf("expected status failed, got %q", st.lastStatus)
	}
	if m.cogErrs == 0 {
		t.Fatal("expected cognition error metric to be recorded")
	}
}

func TestReviewPanicRecovered(t *testing.T) {
	st := &mockStore{insertID: 1}
	gh := &mockGitHub{panicOnDiff: true}
	cog := &mockCognition{}
	se := &mockSSE{}
	m := &mockMetrics{}

	svc := newService(st, cog, gh, se, m)
	err := svc.Review(context.Background(), "owner", "repo", 1, "abc", 0)
	if err == nil {
		t.Fatal("expected error from recovered panic, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic error, got %v", err)
	}
	if st.lastStatus != "failed" {
		t.Fatalf("expected status failed, got %q", st.lastStatus)
	}
}
