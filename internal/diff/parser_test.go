package diff

import "testing"

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

func TestChunkBySize(t *testing.T) {
	files := []FileDiff{
		{FileName: "a.go", Lines: 10},
		{FileName: "b.go", Lines: 10},
		{FileName: "c.go", Lines: 1000}, // 大文件
		{FileName: "d.go", Lines: 10},
	}

	// maxLines=100：a+b 一组(20)，c 单独(1000)，d 一组(10)
	chunks := ChunkBySize(files, 100)
	if len(chunks) != 3 {
		t.Fatalf("ChunkBySize() = %d chunks, want 3", len(chunks))
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

	// maxLines<=0 应默认 500
	if got := ChunkBySize(files, 0); got == nil {
		t.Error("ChunkBySize(maxLines=0) returned nil, want chunks")
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

	// 空 allFiles 时 Ratio 应为 0
	empty := CalcPRSize(nil, filtered)
	if empty.Ratio != 0 {
		t.Errorf("CalcPRSize(nil) Ratio = %v, want 0", empty.Ratio)
	}
}

func TestShouldUsePlanExecute(t *testing.T) {
	if ShouldUsePlanExecute(PRSize{Files: 9, Lines: 100}) {
		t.Error("9 files 100 lines should NOT use plan-execute")
	}
	if !ShouldUsePlanExecute(PRSize{Files: 10, Lines: 100}) {
		t.Error("10 files should use plan-execute")
	}
	if !ShouldUsePlanExecute(PRSize{Files: 1, Lines: 3000}) {
		t.Error("3000 lines should use plan-execute")
	}
}
