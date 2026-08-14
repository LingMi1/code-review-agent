package eval

import (
	"testing"
	"time"
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

	r := evaluateCase(EvalCase{ID: "case-016", Name: "x", Category: "security"}, exp, found, time.Millisecond)

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

	r := evaluateCase(EvalCase{ID: "case-x", Name: "x", Category: "bug"}, exp, found, time.Millisecond)

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
