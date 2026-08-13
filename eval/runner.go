// Package eval 提供评估运行器，计算 Precision / Recall / F1。
package eval

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Run 加载测试用例，执行评估，输出报告。
// reviewFn 是实际的审查函数；如果为 nil，使用 mock reviewer。
func Run(corpusDir, expectedDir string, reviewFn ReviewFunc) (*Report, error) {
	if reviewFn == nil {
		reviewFn = mockReview
	}

	// 1. 加载测试用例
	cases, err := loadCases(corpusDir)
	if err != nil {
		return nil, fmt.Errorf("load cases: %w", err)
	}

	// 2. 加载期望输出
	expected, err := loadExpected(expectedDir)
	if err != nil {
		return nil, fmt.Errorf("load expected: %w", err)
	}

	// 3. 逐用例评估
	var results []CaseResult
	var totalTP, totalFP, totalFN int

	for _, c := range cases {
		exp, ok := expected[c.ID]
		if !ok {
			results = append(results, CaseResult{
				CaseID:   c.ID,
				CaseName: c.Name,
				Category: c.Category,
				Passed:   false,
			})
			continue
		}

		start := time.Now()
		found, err := reviewFn(c.Diff, c.Language)
		latency := time.Since(start)

		result := evaluateCase(c, exp, found, latency)
		results = append(results, result)

		if err == nil && result.Passed {
			totalTP += result.TruePositives
			totalFP += result.FalsePositives
			totalFN += result.FalseNegatives
		}
	}

	// 4. 计算宏观指标（每个用例 F1 的均值）
	var sumPrecision, sumRecall, sumF1 float64
	validCases := 0
	for _, r := range results {
		if !math.IsNaN(r.F1) {
			sumPrecision += r.Precision
			sumRecall += r.Recall
			sumF1 += r.F1
			validCases++
		}
	}

	report := &Report{
		TotalCases:  len(cases),
		PassedCases: countPassed(results),
		Results:     results,
	}
	if report.TotalCases > 0 {
		report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)
	}
	if validCases > 0 {
		report.MacroPrecision = sumPrecision / float64(validCases)
		report.MacroRecall = sumRecall / float64(validCases)
		report.MacroF1 = 2 * report.MacroPrecision * report.MacroRecall / (report.MacroPrecision + report.MacroRecall)
	}

	// 微观指标（全部 issue 的全局 precision/recall）
	if totalTP+totalFP > 0 {
		report.MicroPrecision = float64(totalTP) / float64(totalTP+totalFP)
	}
	if totalTP+totalFN > 0 {
		report.MicroRecall = float64(totalTP) / float64(totalTP+totalFN)
	}
	if report.MicroPrecision+report.MicroRecall > 0 {
		report.MicroF1 = 2 * report.MicroPrecision * report.MicroRecall / (report.MicroPrecision + report.MicroRecall)
	}

	return report, nil
}

// evaluateCase 评估单个用例。
func evaluateCase(c EvalCase, exp ExpectedResult, found []FoundIssue, latency time.Duration) CaseResult {
	result := CaseResult{
		CaseID:    c.ID,
		CaseName:  c.Name,
		Category:  c.Category,
		ExpectedCount: len(exp.Issues),
		FoundCount:    len(found),
	}

	// 检查匹配
	matchedExp := make([]bool, len(exp.Issues))
	matchedFound := make([]bool, len(found))

	for i, fi := range found {
		for j, ei := range exp.Issues {
			if matchedExp[j] {
				continue
			}
			if isMatch(fi, ei) {
				matchedExp[j] = true
				matchedFound[i] = true
				result.TruePositives++
			}
		}
	}

	// 收集未匹配的 issues
	for i, m := range matchedFound {
		found[i].Exact = m
		if !m {
			result.UnmatchedIssues = append(result.UnmatchedIssues, found[i])
			result.FalsePositives++
		} else {
			result.MatchedIssues = append(result.MatchedIssues, found[i])
		}
	}
	for i, m := range matchedExp {
		if !m {
			result.MissedIssues = append(result.MissedIssues, exp.Issues[i])
			result.FalseNegatives++
		}
	}
	// 计算指标
	if result.TruePositives+result.FalsePositives > 0 {
		result.Precision = float64(result.TruePositives) / float64(result.TruePositives+result.FalsePositives)
	}
	if result.TruePositives+result.FalseNegatives > 0 {
		result.Recall = float64(result.TruePositives) / float64(result.TruePositives+result.FalseNegatives)
	}
	if result.Precision+result.Recall > 0 {
		result.F1 = 2 * result.Precision * result.Recall / (result.Precision + result.Recall)
	}

	// 判定通过：F1 >= 0.5
	result.Passed = result.F1 >= 0.5
	_ = latency

	return result
}

// lineTolerance 是行号匹配的容差。
// LLM 对 unified diff 行号（hunk header 的 @@ +N @@ 计算）存在固有噪声，
// 且人工标注也可能有 ±1-2 行的误差，因此允许 ±3 行的容差。
const lineTolerance = 3

// isMatch 判断 Agent 找到的 issue 是否和期望的 issue 匹配。
// 匹配规则：文件名包含匹配 + 行号在容差范围内 + category 一致。
func isMatch(found FoundIssue, expected ExpectedIssue) bool {
	// 文件名：found 的文件名包含 expected 的文件名（或者反过来）
	if !containsAny(found.File, expected.File) && !containsAny(expected.File, found.File) {
		return false
	}
	// 行号：在 expected 范围内（含容差）
	if expected.LineStart > 0 && found.Line > 0 {
		if found.Line < expected.LineStart-lineTolerance || found.Line > expected.LineEnd+lineTolerance {
			return false
		}
	}
	// category：至少一方匹配
	if expected.Category != "" && found.Category != "" {
		if !strings.EqualFold(expected.Category, found.Category) {
			return false
		}
	}
	return true
}

func containsAny(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// loadCases 加载 corpus 目录下的所有 JSON 测试用例。
func loadCases(dir string) ([]EvalCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var cases []EvalCase
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var c EvalCase
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].ID < cases[j].ID
	})
	return cases, nil
}

// loadExpected 加载 expected 目录下的所有 JSON 期望输出。
func loadExpected(dir string) (map[string]ExpectedResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]ExpectedResult)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var exp ExpectedResult
		if err := json.Unmarshal(data, &exp); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		result[exp.CaseID] = exp
	}
	return result, nil
}

func countPassed(results []CaseResult) int {
	n := 0
	for _, r := range results {
		if r.Passed {
			n++
		}
	}
	return n
}

// PrintReport 格式打印评估报告。
func PrintReport(report *Report) {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     Code Review Agent — Evaluation Report        ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Cases:       %d total, %d passed (%.0f%%)\n",
		report.TotalCases, report.PassedCases, report.PassRate*100)
	fmt.Println()
	fmt.Printf("Micro Precision: %.2f\n", report.MicroPrecision)
	fmt.Printf("Micro Recall:    %.2f\n", report.MicroRecall)
	fmt.Printf("Micro F1:        %.2f\n", report.MicroF1)
	fmt.Println()
	fmt.Printf("Macro Precision: %.2f\n", report.MacroPrecision)
	fmt.Printf("Macro Recall:    %.2f\n", report.MacroRecall)
	fmt.Printf("Macro F1:        %.2f\n", report.MacroF1)
	fmt.Println()
	fmt.Println("═ Per-Case Results ═══════════════════════════════")
	fmt.Println()
	for _, r := range report.Results {
		icon := "✅"
		if !r.Passed {
			icon = "❌"
		}
		fmt.Printf("%s %-40s P=%.2f R=%.2f F1=%.2f  (found %d/%d)\n",
			icon, r.CaseName, r.Precision, r.Recall, r.F1, r.FoundCount, r.ExpectedCount)
		for _, m := range r.MissedIssues {
			fmt.Printf("   MISSED: %s:%d %s — %s\n", m.File, m.LineStart, m.Severity, m.Title)
		}
		for _, u := range r.UnmatchedIssues {
			fmt.Printf("   FALSE+: %s:%d %s — %s\n", u.File, u.Line, u.Severity, u.Title)
		}
	}
	fmt.Println()
	fmt.Println("═ Summary ═════════════════════════════════════════")
	fmt.Println()

	// 按类别统计
	catStats := make(map[string]struct{ cases, passed int })
	catTotals := make(map[string]struct{ tp, fp, fn int })
	for _, r := range report.Results {
		s := catStats[r.Category]
		s.cases++
		if r.Passed {
			s.passed++
		}
		catStats[r.Category] = s

		t := catTotals[r.Category]
		t.tp += r.TruePositives
		t.fp += r.FalsePositives
		t.fn += r.FalseNegatives
		catTotals[r.Category] = t
	}
	for cat, s := range catStats {
		t := catTotals[cat]
		var precision, recall, f1 float64
		if t.tp+t.fp > 0 {
			precision = float64(t.tp) / float64(t.tp+t.fp)
		}
		if t.tp+t.fn > 0 {
			recall = float64(t.tp) / float64(t.tp+t.fn)
		}
		if precision+recall > 0 {
			f1 = 2 * precision * recall / (precision + recall)
		}
		fmt.Printf("  %-12s  %d/%d passed  P=%.2f  R=%.2f  F1=%.2f\n",
			cat, s.passed, s.cases, precision, recall, f1)
	}
}

// SaveReport 保存报告到 JSON 文件。
func SaveReport(report *Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
