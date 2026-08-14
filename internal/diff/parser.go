// Package diff 解析 unified diff 并按文件分块。
package diff

import (
	"strings"
)

// FileDiff 是一个文件的 diff 内容。
type FileDiff struct {
	FileName string
	Hunk     string // 完整的 unified diff hunk（含 @@ header）
	Lines    int    // 估算行数（新增 + 删除 + 上下文行数）
}

// Parse 解析 unified diff 并按文件分组。
// 返回按文件拆分的 diff 切片。
func Parse(rawDiff string) []FileDiff {
	if rawDiff == "" {
		return nil
	}

	var files []FileDiff
	var current *FileDiff

	lines := strings.Split(rawDiff, "\n")
	for _, line := range lines {
		// diff --git a/xxx b/xxx 表示新文件的开始
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil && len(current.Hunk) > 0 {
				files = append(files, *current)
			}
			current = &FileDiff{
				FileName: parseFileName(line),
			}
			continue
		}

		if current == nil {
			continue
		}

		// 跳过 index / --- / +++ 等 meta 行（保留为 diff 的上下文）
		current.Hunk += line + "\n"
		current.Lines++
	}

	// 最后一个文件
	if current != nil && len(current.Hunk) > 0 {
		files = append(files, *current)
	}

	return files
}

// parseFileName 从 "diff --git a/path b/path" 中提取路径。
func parseFileName(line string) string {
	// 格式: "diff --git a/foo/bar.go b/foo/bar.go"
	parts := strings.SplitN(line, " ", 4)
	if len(parts) >= 4 {
		b := parts[3]
		if strings.HasPrefix(b, "b/") {
			return strings.TrimPrefix(b, "b/")
		}
	}
	return line
}

// ChunkBySize 将文件 diff 按最大行数分块。
// 单个文件超过 maxLines 行时，在 @@ hunk 边界处切分，尽量保证每块 ≤ maxLines 行。
func ChunkBySize(files []FileDiff, maxLines int) [][]FileDiff {
	if maxLines <= 0 {
		maxLines = 500
	}

	// 先展开：把超过 maxLines 行的大文件在 hunk 边界处切分。
	var expanded []FileDiff
	for _, f := range files {
		expanded = append(expanded, splitLargeFile(f, maxLines)...)
	}

	// 再贪心分组。
	var chunks [][]FileDiff
	var currentChunk []FileDiff
	currentLines := 0
	for _, f := range expanded {
		if currentLines+f.Lines > maxLines && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentLines = 0
		}
		currentChunk = append(currentChunk, f)
		currentLines += f.Lines
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// splitLargeFile 将超过 maxLines 行的单个文件在 @@ hunk 边界处切分为多个 FileDiff。
// 若无法切分（无 hunk 边界、空 Hunk 或单个超长 hunk），保持原样返回。
func splitLargeFile(f FileDiff, maxLines int) []FileDiff {
	if f.Lines <= maxLines || f.Hunk == "" {
		return []FileDiff{f}
	}

	lines := strings.Split(f.Hunk, "\n")
	var parts []FileDiff
	var cur strings.Builder
	curLines := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") && curLines > 0 {
			parts = append(parts, FileDiff{FileName: f.FileName, Hunk: cur.String(), Lines: curLines})
			cur.Reset()
			curLines = 0
		}
		cur.WriteString(line + "\n")
		curLines++
	}
	if curLines > 0 {
		parts = append(parts, FileDiff{FileName: f.FileName, Hunk: cur.String(), Lines: curLines})
	}

	if len(parts) <= 1 {
		return []FileDiff{f}
	}
	return parts
}

// SkipGeneratedFiles 过滤掉生成文件（lock 文件、generated 标记、binary 文件等）。
func SkipGeneratedFiles(files []FileDiff) []FileDiff {
	var filtered []FileDiff
	for _, f := range files {
		if isGeneratedFile(f.FileName) {
			continue
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// SkipLockFiles 过滤掉依赖锁文件和大型数据文件。
func SkipLockFiles(files []FileDiff) []FileDiff {
	lockPatterns := []string{
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"go.sum", "Cargo.lock", "Gemfile.lock", "poetry.lock",
		"composer.lock", "Pipfile.lock", "mix.lock",
		"CHANGELOG.md", ".gitignore", ".dockerignore", ".editorconfig",
	}
	var filtered []FileDiff
outer:
	for _, f := range files {
		for _, pattern := range lockPatterns {
			if strings.Contains(f.FileName, pattern) {
				continue outer
			}
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// SkipDataFiles 过滤掉数据文件和配置文件（CSV/JSON 数据等）。
func SkipDataFiles(files []FileDiff) []FileDiff {
	dataPatterns := []string{".csv", ".geojson", ".graphql", ".proto"}
	var filtered []FileDiff
outer:
	for _, f := range files {
		for _, pattern := range dataPatterns {
			if strings.HasSuffix(f.FileName, pattern) {
				continue outer
			}
		}
		filtered = append(filtered, f)
	}
	return filtered
}

// PRSize 计算 PR 的有效文件数和总变更行数。
type PRSize struct {
	Files      int
	Lines      int     // 总变更行数
	Ratio      float64 // 有效文件数 / 总文件数
}

// CalcPRSize 计算 PR 的规模指标。
func CalcPRSize(allFiles, filtered []FileDiff) PRSize {
	s := PRSize{Files: len(filtered)}
	if len(allFiles) > 0 {
		s.Ratio = float64(len(filtered)) / float64(len(allFiles))
	}
	for _, f := range filtered {
		s.Lines += f.Lines
	}
	return s
}

// ShouldUsePlanExecute 判断 PR 复杂度是否值得用 Plan-Execute 模式（10+ 个有效文件）。
// 注意：能否真正使用还取决于 diff 能否塞进单 prompt（约 800 行）。
// 行数过大的 PR 应走 chunked react 而非 plan-execute，否则 prompt 会被截断。
func ShouldUsePlanExecute(size PRSize) bool {
	return size.Files >= 10
}

// isGeneratedFile 判断是否为生成文件。
func isGeneratedFile(name string) bool {
	generatedPatterns := []string{
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"go.sum", "Cargo.lock", "Gemfile.lock",
		".pb.go", "_grpc.pb.go", ".pb.cc",
		".gen.go", ".generated.", "autogen",
		"vendor/", "node_modules/",
		".min.js", ".min.css",
		".svg", ".png", ".jpg", ".jpeg", ".gif", ".ico",
		".woff", ".woff2", ".ttf", ".eot",
		".pb.go", "genproto/",
	}
	for _, pattern := range generatedPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}
	return false
}
