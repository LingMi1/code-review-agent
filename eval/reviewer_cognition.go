// Package eval 提供通过 agent-go 认知面（真实 LLM）审查 diff 的 ReviewFunc。
package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/diff"
	"github.com/LingMi1/code-review-agent/internal/prompt"
	"github.com/LingMi1/code-review-agent/internal/review"
)

// CognitionReview 返回一个调用 agent-go 认知面（DeepSeek/Claude）的审查函数。
// 与 mockReview 不同，这里走真实 gRPC 链路，得到 LLM 的审查结果。
//
// 评估链路复用生产的 prompt 构造（prompt.BuildReviewPrompt）与结果解析
// （review.ParseResult），确保评出的指标对应生产真实行为，而不是另一套
// 更宽松的 eval-only 逻辑。language 参数不注入 prompt：生产 prompt 也不区分语言，
// 注入额外提示会人为抬高指标。
func CognitionReview(addr string) (ReviewFunc, error) {
	client, err := cognition.New(addr)
	if err != nil {
		return nil, err
	}
	return func(diffStr, _ string) ([]FoundIssue, Usage, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// 与生产一致：解析 diff → 构造 prompt → 调用 react 认知面 → 解析 JSON。
		files := diff.Parse(diffStr)
		if len(files) == 0 {
			return nil, Usage{}, fmt.Errorf("eval: diff parsed into no files")
		}
		reviewPrompt := prompt.BuildReviewPrompt("", files)

		result, err := client.RunReview(ctx, cognition.ReviewRequest{
			SessionID: fmt.Sprintf("eval-%d", time.Now().UnixNano()),
			Query:     reviewPrompt,
			AgentType: "react",
			MaxSteps:  5,
		})
		if err != nil {
			return nil, Usage{}, fmt.Errorf("cognition review: %w", err)
		}

		out, err := review.ParseResult(result.Text)
		if err != nil {
			return nil, Usage{}, fmt.Errorf("parse review result: %w", err)
		}
		return toFoundIssues(out.Issues), Usage{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
		}, nil
	}, nil
}

// toFoundIssues 将生产的 review.Issue 映射为评估用 FoundIssue。
func toFoundIssues(issues []review.Issue) []FoundIssue {
	found := make([]FoundIssue, 0, len(issues))
	for _, iss := range issues {
		found = append(found, FoundIssue{
			File:     iss.File,
			Line:     iss.Line,
			Severity: iss.Severity,
			Category: iss.Category,
			Title:    iss.Title,
		})
	}
	return found
}
