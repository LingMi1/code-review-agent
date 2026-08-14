package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecorderCountersAndRatio(t *testing.T) {
	r := New()
	r.RecordSuccess(10, 100, 2)
	r.RecordSuccess(20, 200, 3)
	r.RecordFailure()
	r.RecordCognitionError()

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, nil)
	body := rr.Body.String()

	for _, want := range []string{
		`review_total{status="success"} 2`,
		`review_total{status="failed"} 1`,
		`cognition_errors_total 1`,
		"review_success_ratio 0.67",
		`review_issues_found 3`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, body)
		}
	}
}

func TestRecorderLatencyPercentiles(t *testing.T) {
	r := New()
	for i := int64(1); i <= 100; i++ {
		r.RecordSuccess(i, 0, 0)
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, nil)
	body := rr.Body.String()

	for _, want := range []string{
		`review_latency_ms{quantile="0.5"} 51`,
		`review_latency_ms{quantile="0.95"} 96`,
		`review_latency_ms{quantile="0.99"} 100`,
		`review_latency_ms_sum 5050`,
		`review_latency_ms_count 100`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, body)
		}
	}
}

func TestRecorderEmpty(t *testing.T) {
	r := New()

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, nil)
	body := rr.Body.String()

	if strings.Contains(body, "review_latency_ms{quantile") {
		t.Errorf("expected no latency quantiles for an empty recorder, got:\n%s", body)
	}
	if !strings.Contains(body, "review_success_ratio 0.00") {
		t.Errorf("expected a zero success ratio, got:\n%s", body)
	}
}
