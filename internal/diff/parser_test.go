package diff

import (
	"fmt"
	"strings"
	"testing"
)

func sampleDiff() string {
	return `diff --git a/main.go b/main.go
index 1234567..89abcde 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main
 
+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }
diff --git a/README.md b/README.md
new file mode 100644
--- /dev/null
+++ b/README.md
@@ -0,0 +1,3 @@
+# Title
+
+content
diff --git a/go.sum b/go.sum
index 1111111..2222222 100644
--- a/go.sum
+++ b/go.sum
@@ -1,2 +1,2 @@
-old
+new
`
}

func TestParse(t *testing.T) {
	if got := Parse(""); got != nil {
		t.Fatalf("Parse(empty) = %v, want nil", got)
	}

	files := Parse(sampleDiff())
	if len(files) != 3 {
		t.Fatalf("Parse() returned %d files, want 3", len(files))
	}

	if files[0].FileName != "main.go" {
		t.Errorf("files[0].FileName = %q, want main.go", files[0].FileName)
	}
	if files[1].FileName != "README.md" {
		t.Errorf("files[1].FileName = %q, want README.md", files[1].FileName)
	}
	if files[0].Lines == 0 {
		t.Errorf("files[0].Lines = 0, want > 0")
	}
}

func TestChunkByBytes(t *testing.T) {
	// Chunking is driven by the Hunk's byte length (a Hunk without @@ boundaries cannot be split).
	mk := func(name string, n int) FileDiff {
		return FileDiff{FileName: name, Hunk: strings.Repeat("x", n), Lines: 1}
	}
	files := []FileDiff{
		mk("a.go", 10),
		mk("b.go", 10),
		mk("c.go", 1000), // large file (single hunk cannot be split → becomes its own chunk)
		mk("d.go", 10),
	}

	// maxBytes=100: a+b together (20 bytes), c alone (1000), d alone (10)
	chunks := ChunkByBytes(files, 100)
	if len(chunks) != 3 {
		t.Fatalf("ChunkByBytes() = %d chunks, want 3", len(chunks))
	}
	if len(chunks[0]) != 2 {
		t.Errorf("chunks[0] len = %d, want 2", len(chunks[0]))
	}
	if len(chunks[1]) != 1 || chunks[1][0].FileName != "c.go" {
		t.Errorf("chunks[1] = %+v, want single c.go", chunks[1])
	}
	if len(chunks[2]) != 1 || chunks[2][0].FileName != "d.go" {
		t.Errorf("chunks[2] = %+v, want single d.go", chunks[2])
	}

	// maxBytes<=0 should default to 28KB
	if got := ChunkByBytes(files, 0); got == nil {
		t.Error("ChunkByBytes(maxBytes=0) returned nil, want chunks")
	}
}

func TestSkipGeneratedFiles(t *testing.T) {
	files := []FileDiff{
		{FileName: "main.go"},
		{FileName: "api.pb.go"},
		{FileName: "node_modules/foo.js"},
		{FileName: "logo.png"},
		{FileName: "service_grpc.pb.go"},
	}
	got := SkipGeneratedFiles(files)
	if len(got) != 1 {
		t.Fatalf("SkipGeneratedFiles() = %d files, want 1", len(got))
	}
	if got[0].FileName != "main.go" {
		t.Errorf("SkipGeneratedFiles() kept %q, want main.go", got[0].FileName)
	}
}

func TestSkipLockFiles(t *testing.T) {
	files := []FileDiff{
		{FileName: "main.go"},
		{FileName: "go.sum"},
		{FileName: "package-lock.json"},
		{FileName: "CHANGELOG.md"},
	}
	got := SkipLockFiles(files)
	if len(got) != 1 {
		t.Fatalf("SkipLockFiles() = %d files, want 1", len(got))
	}
	if got[0].FileName != "main.go" {
		t.Errorf("SkipLockFiles() kept %q, want main.go", got[0].FileName)
	}
}

func TestSkipDataFiles(t *testing.T) {
	files := []FileDiff{
		{FileName: "main.go"},
		{FileName: "data.csv"},
		{FileName: "schema.proto"},
		{FileName: "query.graphql"},
	}
	got := SkipDataFiles(files)
	if len(got) != 1 {
		t.Fatalf("SkipDataFiles() = %d files, want 1", len(got))
	}
	if got[0].FileName != "main.go" {
		t.Errorf("SkipDataFiles() kept %q, want main.go", got[0].FileName)
	}
}

func TestCalcPRSize(t *testing.T) {
	all := []FileDiff{{FileName: "a.go", Lines: 10}, {FileName: "b.go", Lines: 20}, {FileName: "c.go", Lines: 30}}
	filtered := []FileDiff{{FileName: "a.go", Lines: 10}, {FileName: "b.go", Lines: 20}}

	size := CalcPRSize(all, filtered)
	if size.Files != 2 {
		t.Errorf("size.Files = %d, want 2", size.Files)
	}
	if size.Lines != 30 {
		t.Errorf("size.Lines = %d, want 30", size.Lines)
	}
	if size.Ratio != 2.0/3.0 {
		t.Errorf("size.Ratio = %v, want %v", size.Ratio, 2.0/3.0)
	}

	// empty allFiles should yield Ratio 0
	empty := CalcPRSize(nil, filtered)
	if empty.Ratio != 0 {
		t.Errorf("CalcPRSize(nil) Ratio = %v, want 0", empty.Ratio)
	}
}

func TestShouldUsePlanExecute(t *testing.T) {
	if ShouldUsePlanExecute(PRSize{Files: 9, Lines: 100}) {
		t.Error("9 files should NOT use plan-execute")
	}
	if !ShouldUsePlanExecute(PRSize{Files: 10, Lines: 100}) {
		t.Error("10 files should use plan-execute")
	}
	// Few files but many lines: should NOT use plan-execute, otherwise the whole diff would be truncated by the 32KB prompt.
	if ShouldUsePlanExecute(PRSize{Files: 1, Lines: 3000}) {
		t.Error("1 file with 3000 lines should NOT use plan-execute")
	}
}

func TestChunkByBytesSplitsLargeFile(t *testing.T) {
	// Build a single file with 3 hunks to force splitting at hunk boundaries.
	var hunk strings.Builder
	for h := 0; h < 3; h++ {
		fmt.Fprintf(&hunk, "@@ -%d,5 +%d,5 @@\n", h*10+1, h*10+1)
		for j := 0; j < 5; j++ {
			fmt.Fprintf(&hunk, "+line %d\n", h*10+j)
		}
	}
	f := FileDiff{FileName: "big.go", Hunk: hunk.String(), Lines: 18}

	chunks := ChunkByBytes([]FileDiff{f}, 60)
	if len(chunks) < 2 {
		t.Fatalf("expected large file to be split into multiple chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		for _, part := range c {
			if part.FileName != "big.go" {
				t.Errorf("expected filename big.go, got %q", part.FileName)
			}
			if part.Lines == 0 {
				t.Error("split part should have non-zero lines")
			}
		}
	}
}
