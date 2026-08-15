package review

import (
	"strings"
	"testing"
)

func TestParseResult(t *testing.T) {
	// 1. direct JSON
	direct := `{"summary":"ok","issues":[{"file":"a.go","line":1,"severity":"high","category":"bug","title":"t","description":"d","suggestion":"s"}]}`
	out, err := parseResult(direct)
	if err != nil {
		t.Fatalf("parseResult(direct) error = %v", err)
	}
	if out.Summary != "ok" || len(out.Issues) != 1 {
		t.Errorf("parseResult(direct) = %+v, want summary=ok 1 issue", out)
	}

	// 2. extract from a ```json code block
	block := "here is my review:\n```json\n" + direct + "\n```\nthanks"
	out2, err := parseResult(block)
	if err != nil {
		t.Fatalf("parseResult(block) error = %v", err)
	}
	if out2.Summary != "ok" {
		t.Errorf("parseResult(block) summary = %q, want ok", out2.Summary)
	}

	// 3. unparseable → error
	if _, err := parseResult("not json at all"); err == nil {
		t.Error("parseResult(garbage) should error")
	}
}

func TestExtractJSONBlock(t *testing.T) {
	// ```json marker
	got := extractJSONBlock("```json\n{\"a\":1}\n```")
	if strings.TrimSpace(got) != "{\"a\":1}" {
		t.Errorf("extractJSONBlock(json) = %q, want {\"a\":1}", got)
	}

	// preamble text should still locate the ```json block correctly
	got2 := extractJSONBlock("here is my review:\n```json\n{\"a\":1}\n```\nthanks")
	if strings.TrimSpace(got2) != "{\"a\":1}" {
		t.Errorf("extractJSONBlock(with preamble) = %q, want {\"a\":1}", got2)
	}

	// no marker → empty
	if got := extractJSONBlock("no block here"); got != "" {
		t.Errorf("extractJSONBlock(no block) = %q, want empty", got)
	}
}

func TestBuildInlineComments(t *testing.T) {
	issues := []Issue{
		{File: "a.go", Line: 5, Severity: "high", Category: "bug", Title: "nil deref", Description: "x", Suggestion: "fix"},
		{File: "", Line: 3, Severity: "high", Category: "bug", Title: "no file", Description: "skip me", Suggestion: "fix"}, // should be skipped
		{File: "b.go", Line: 0, Severity: "low", Category: "style", Title: "no line", Description: "skip me", Suggestion: "fix"}, // should be skipped
	}
	comments := buildInlineComments(issues)
	if len(comments) != 1 {
		t.Fatalf("buildInlineComments() = %d comments, want 1", len(comments))
	}
	if comments[0].Path != "a.go" || comments[0].Line != 5 {
		t.Errorf("comments[0] = %+v, want a.go:5", comments[0])
	}
}

func TestSeverityIcon(t *testing.T) {
	cases := map[string]string{
		"high":   "🔴",
		"medium": "🟡",
		"low":    "🟢",
		"unknown": "⚪",
	}
	for in, want := range cases {
		if got := severityIcon(in); got != want {
			t.Errorf("severityIcon(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPlainComment(t *testing.T) {
	got := formatPlainComment("some result")
	if got == "" {
		t.Error("formatPlainComment() returned empty")
	}
	if got[:len("## AI Code Review")] != "## AI Code Review" {
		t.Errorf("formatPlainComment() missing header, got %q", got)
	}
}
