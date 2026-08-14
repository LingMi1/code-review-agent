// Package metrics 暴露 Prometheus 格式的指标端点 /metrics。
//
// 指标列表：
//   - review_total          counter   审查次数（按 status 标签分组）
//   - review_latency_ms     histogram 单次审查耗时（毫秒）
//   - review_issues_found   gauge     最近一次审查发现的问题数
//   - review_output_bytes   histogram Agent 输出字节数
//   - cognition_errors      counter   认知面调用失败次数
//
// 这些指标可被 Prometheus 抓取，Grafana 可视化。
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Recorder 收集并暴露指标。
type Recorder struct {
	mu sync.Mutex

	// review_total{status="success|failed"}
	reviewSuccess int64
	reviewFailed  int64

	// 延迟分布（毫秒）
	latencies []int64 // 最近 1000 次
	latCount  int

	// 输出字节分布
	outputBytes []int64
	byteCount   int

	// 最后一次审查的问题数
	lastIssuesFound int

	// 认知面错误
	cognitionErrors int64

	// 审查耗时 P50/P95/P99（每 30 秒更新一次）
	lastLatencyCalc time.Time
	p50, p95, p99   int64
}

// New 创建 Recorder。
func New() *Recorder {
	return &Recorder{}
}

// RecordSuccess 记录一次成功审查。
func (r *Recorder) RecordSuccess(latencyMs int64, outputBytes int, issuesFound int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reviewSuccess++
	r.latencies = append(r.latencies, latencyMs)
	if len(r.latencies) > 1000 {
		r.latencies = r.latencies[1:]
	}
	r.latCount++
	r.outputBytes = append(r.outputBytes, int64(outputBytes))
	if len(r.outputBytes) > 1000 {
		r.outputBytes = r.outputBytes[1:]
	}
	r.byteCount++
	r.lastIssuesFound = issuesFound
}

// RecordFailure 记录一次失败审查。
func (r *Recorder) RecordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reviewFailed++
}

// RecordCognitionError 记录认知面调用错误。
func (r *Recorder) RecordCognitionError() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cognitionErrors++
}

// ServeHTTP 输出 Prometheus text format 指标。
func (r *Recorder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.recalcLatencies()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	write := func(s string) { _, _ = io.WriteString(w, s) }

	// HELP / TYPE
	write("# HELP review_total Total number of code reviews.\n")
	write("# TYPE review_total counter\n")
	write(fmt.Sprintf("review_total{status=\"success\"} %d\n", r.reviewSuccess))
	write(fmt.Sprintf("review_total{status=\"failed\"} %d\n", r.reviewFailed))

	write("# HELP review_latency_ms Review latency in milliseconds.\n")
	write("# TYPE review_latency_ms summary\n")
	if len(r.latencies) > 0 {
		write(fmt.Sprintf("review_latency_ms{quantile=\"0.5\"} %d\n", r.p50))
		write(fmt.Sprintf("review_latency_ms{quantile=\"0.95\"} %d\n", r.p95))
		write(fmt.Sprintf("review_latency_ms{quantile=\"0.99\"} %d\n", r.p99))
		write(fmt.Sprintf("review_latency_ms_sum %d\n", sum(r.latencies)))
		write(fmt.Sprintf("review_latency_ms_count %d\n", r.latCount))
	}

	write("# HELP review_issues_found Issues found in last review.\n")
	write("# TYPE review_issues_found gauge\n")
	write(fmt.Sprintf("review_issues_found %d\n", r.lastIssuesFound))

	write("# HELP review_output_bytes Size of agent output in bytes.\n")
	write("# TYPE review_output_bytes summary\n")
	if len(r.outputBytes) > 0 {
		write(fmt.Sprintf("review_output_bytes_sum %d\n", sum(r.outputBytes)))
		write(fmt.Sprintf("review_output_bytes_count %d\n", r.byteCount))
	}

	write("# HELP cognition_errors_total Number of cognition call failures.\n")
	write("# TYPE cognition_errors_total counter\n")
	write(fmt.Sprintf("cognition_errors_total %d\n", r.cognitionErrors))

	write("# HELP review_success_ratio Ratio of successful reviews.\n")
	write("# TYPE review_success_ratio gauge\n")
	total := r.reviewSuccess + r.reviewFailed
	if total > 0 {
		write(fmt.Sprintf("review_success_ratio %.2f\n", float64(r.reviewSuccess)/float64(total)))
	} else {
		write("review_success_ratio 0.00\n")
	}
}

// recalcLatencies 计算延迟分位数（调用方持有锁）。
func (r *Recorder) recalcLatencies() {
	if len(r.latencies) == 0 || time.Since(r.lastLatencyCalc) < 30*time.Second {
		return
	}
	r.lastLatencyCalc = time.Now()

	sorted := make([]int64, len(r.latencies))
	copy(sorted, r.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	r.p50 = sorted[n*50/100]
	r.p95 = sorted[n*95/100]
	r.p99 = sorted[n*99/100]
}

func sum(a []int64) int64 {
	var s int64
	for _, v := range a {
		s += v
	}
	return s
}
