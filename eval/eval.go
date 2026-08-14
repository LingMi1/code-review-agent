// Package eval 提供 Code Review Agent 的评估体系。
//
// 用法：
//
//	go run ./cmd/eval/
//
// 该命令会：
//  1. 加载 eval/corpus/ 下的测试用例
//  2. 加载 eval/expected/ 下的期望输出
//  3. 计算 precision / recall / F1 / latency
//  4. 输出评估报告到 stdout 和 eval/report.json
package eval

// EvalCase 是一个标注的测试 PR。
type EvalCase struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // bug, security, performance, style
	Diff     string `json:"diff"`
	Language string `json:"language"`
}

// ExpectedIssue 是期望 Agent 发现的问题。
type ExpectedIssue struct {
	File       string `json:"file"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
}

// ExpectedResult 是一个用例的完整期望输出。
type ExpectedResult struct {
	CaseID       string          `json:"case_id"`
	ShouldReject bool            `json:"should_reject"` // 是否应该拒绝（CI 阻断级）
	Issues       []ExpectedIssue `json:"issues"`
}

// FoundIssue 是 Agent 实际输出中的问题（用于匹配）。
type FoundIssue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Exact    bool   `json:"exact"` // 是否精确匹配了期望的 issue
}

// CaseResult 是一个测试用例的评估结果。
type CaseResult struct {
	CaseID          string       `json:"case_id"`
	CaseName        string       `json:"case_name"`
	Category        string       `json:"category"`
	Passed          bool         `json:"passed"`
	Precision       float64      `json:"precision"`
	Recall          float64      `json:"recall"`
	F1              float64      `json:"f1"`
	ExpectedCount   int          `json:"expected_count"`
	FoundCount      int          `json:"found_count"`
	TruePositives   int          `json:"true_positives"`
	FalsePositives  int          `json:"false_positives"`
	FalseNegatives  int          `json:"false_negatives"`
	MatchedIssues   []FoundIssue `json:"matched_issues"`
	UnmatchedIssues []FoundIssue `json:"unmatched_issues"`
	MissedIssues    []ExpectedIssue `json:"missed_issues"`
}

// Report 是完整的评估报告。
type Report struct {
	TotalCases       int     `json:"total_cases"`
	PassedCases      int     `json:"passed_cases"`
	PassRate         float64 `json:"pass_rate"`
	MacroPrecision   float64 `json:"macro_precision"`
	MacroRecall      float64 `json:"macro_recall"`
	MacroF1          float64 `json:"macro_f1"`
	MicroPrecision   float64 `json:"micro_precision"`
	MicroRecall      float64 `json:"micro_recall"`
	MicroF1          float64 `json:"micro_f1"`
	Results          []CaseResult `json:"results"`
}

// ReviewFunc 是审查函数签名：给定 PR diff，返回找到的问题列表。
type ReviewFunc func(diff string, language string) ([]FoundIssue, error)
