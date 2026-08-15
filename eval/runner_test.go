package eval

import (
	"testing"
)

func TestIsMatch(t *testing.T) {
	tests := []struct {
		name     string
		found    FoundIssue
		expected ExpectedIssue
		want     bool
	}{
		{
			name:     "exact match",
			found:    FoundIssue{File: "a.go", Line: 10, Category: "security"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     true,
		},
		{
			name:     "line within tolerance",
			found:    FoundIssue{File: "a.go", Line: 12, Category: "security"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     true,
		},
		{
			name:     "line outside tolerance",
			found:    FoundIssue{File: "a.go", Line: 20, Category: "security"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     false,
		},
		{
			name:     "category mismatch",
			found:    FoundIssue{File: "a.go", Line: 10, Category: "bug"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     false,
		},
		{
			name:     "file mismatch",
			found:    FoundIssue{File: "b.go", Line: 10, Category: "security"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     false,
		},
		{
			name:     "file path contains basename",
			found:    FoundIssue{File: "src/a.go", Line: 10, Category: "security"},
			expected: ExpectedIssue{File: "a.go", LineStart: 10, LineEnd: 10, Category: "security"},
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMatch(tc.found, tc.expected); got != tc.want {
				t.Errorf("isMatch() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluateCaseNoDoubleCount(t *testing.T) {
	exp := ExpectedResult{
		CaseID: "case-016",
		Issues: []ExpectedIssue{
			{File: "upload_handler.go", LineStart: 12, LineEnd: 12, Category: "security"},
			{File: "upload_handler.go", LineStart: 13, LineEnd: 13, Category: "bug"},
			{File: "upload_handler.go", LineStart: 14, LineEnd: 15, Category: "bug"},
		},
	}
	found := []FoundIssue{
		{File: "upload_handler.go", Line: 12, Category: "security"},
		{File: "upload_handler.go", Line: 13, Category: "bug"},
		{File: "upload_handler.go", Line: 14, Category: "bug"},
	}

	r := evaluateCase(EvalCase{ID: "case-016", Name: "x", Category: "security"}, exp, found)

	if r.TruePositives != 3 {
		t.Errorf("TruePositives = %d, want 3 (no double count)", r.TruePositives)
	}
	if r.FalsePositives != 0 {
		t.Errorf("FalsePositives = %d, want 0", r.FalsePositives)
	}
	if r.FalseNegatives != 0 {
		t.Errorf("FalseNegatives = %d, want 0", r.FalseNegatives)
	}
	if r.Precision != 1.0 || r.Recall != 1.0 || r.F1 != 1.0 {
		t.Errorf("P/R/F1 = %.2f/%.2f/%.2f, want 1.0 each", r.Precision, r.Recall, r.F1)
	}
	if !r.Passed {
		t.Error("expected case to pass")
	}
}

func TestEvaluateCaseFalsePositive(t *testing.T) {
	exp := ExpectedResult{
		CaseID: "case-x",
		Issues: []ExpectedIssue{
			{File: "a.go", LineStart: 10, LineEnd: 10, Category: "bug"},
		},
	}
	found := []FoundIssue{
		{File: "a.go", Line: 10, Category: "bug"},   // TP
		{File: "a.go", Line: 50, Category: "style"}, // FP（行号与分类都不匹配）
	}

	r := evaluateCase(EvalCase{ID: "case-x", Name: "x", Category: "bug"}, exp, found)

	if r.TruePositives != 1 {
		t.Errorf("TruePositives = %d, want 1", r.TruePositives)
	}
	if r.FalsePositives != 1 {
		t.Errorf("FalsePositives = %d, want 1", r.FalsePositives)
	}
	if r.FalseNegatives != 0 {
		t.Errorf("FalseNegatives = %d, want 0", r.FalseNegatives)
	}
}

func TestEvaluateCaseNegative(t *testing.T) {
	// 负例：期望 0 个 issue，审查器正确返回空 → 应判为通过（F1=1），而非 F1=0 误判失败。
	exp := ExpectedResult{CaseID: "case-neg", Issues: []ExpectedIssue{}}

	clean := evaluateCase(EvalCase{ID: "case-neg", Name: "clean", Category: "bug"}, exp, nil)
	if !clean.Passed || clean.F1 != 1.0 || clean.Precision != 1.0 || clean.Recall != 1.0 {
		t.Errorf("clean case = P/R/F1 %.2f/%.2f/%.2f passed=%v, want 1/1/1 true",
			clean.Precision, clean.Recall, clean.F1, clean.Passed)
	}

	// 负例但审查器误报 → 应判为失败（F1=0），并计入 FalsePositives。
	fp := evaluateCase(EvalCase{ID: "case-neg", Name: "clean", Category: "bug"}, exp,
		[]FoundIssue{{File: "a.go", Line: 1, Category: "style"}})
	if fp.Passed || fp.F1 != 0.0 || fp.FalsePositives != 1 {
		t.Errorf("false-positive case = F1 %.2f FP=%d passed=%v, want F1=0 FP=1 passed=false",
			fp.F1, fp.FalsePositives, fp.Passed)
	}
}
