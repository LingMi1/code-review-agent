package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/LingMi1/code-review-agent/internal/diff"
)

func TestBuildReviewPrompt(t *testing.T) {
	files := []diff.FileDiff{
		{FileName: "main.go", Lines: 5, Hunk: "@@ -1,5 +1,6 @@\n+import \"fmt\"\n"},
	}

	p := BuildReviewPrompt("fix: add fmt import", files)

	checks := []string{
		"expert code reviewer",
		"PR Title: fix: add fmt import",
		"## Changed Files",
		"`main.go` (5 lines changed)",
		"## Diff",
		"### main.go",
		"## Response Format",
		`"issues"`,
	}
	for _, want := range checks {
		if !strings.Contains(p, want) {
			t.Errorf("BuildReviewPrompt() missing %q", want)
		}
	}
}

func TestBuildPlanExecutePrompt(t *testing.T) {
	files := []diff.FileDiff{
		{FileName: "main.go", Lines: 5, Hunk: "@@ -1,5 +1,6 @@\n+import \"fmt\"\n"},
	}

	p := BuildPlanExecutePrompt("large refactor", files)

	checks := []string{
		"plan-execute mode",
		"## Review Plan",
		"Security Review",
		"Bug Detection",
		"Performance Analysis",
		"Code Quality",
		"PR Title: large refactor",
	}
	for _, want := range checks {
		if !strings.Contains(p, want) {
			t.Errorf("BuildPlanExecutePrompt() missing %q", want)
		}
	}
}

func TestTruncatePrompt(t *testing.T) {
	short := "hello world"
	if got := TruncatePrompt(short, 100); got != short {
		t.Errorf("TruncatePrompt(short) = %q, want %q (unchanged)", got, short)
	}

	long := strings.Repeat("x", 200)
	got := TruncatePrompt(long, 100)
	if len(got) >= 200 {
		t.Errorf("TruncatePrompt() len = %d, want < 200", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("TruncatePrompt() missing truncated marker, got %q", got)
	}

	// 带换行的长文本应在最后一个完整行处截断（结果远短于原文）
	multi := strings.Repeat("line of text\n", 50) // 650 字节
	gotMulti := TruncatePrompt(multi, 100)
	if len(gotMulti) >= len(multi) {
		t.Errorf("TruncatePrompt(multiline) len = %d, want < %d", len(gotMulti), len(multi))
	}
	if !strings.HasSuffix(gotMulti, "truncated due to size)") {
		t.Errorf("TruncatePrompt(multiline) should end with truncated marker, got ...%q", gotMulti[len(gotMulti)-20:])
	}
}

func TestTruncatePromptUTF8Boundary(t *testing.T) {
	// 每个中文字符 3 字节，maxBytes=100 会落在 '你' 中间
	s := strings.Repeat("你", 100)
	got := TruncatePrompt(s, 100)
	if !utf8.ValidString(got) {
		t.Errorf("TruncatePrompt() produced invalid UTF-8: %q", got)
	}
}
