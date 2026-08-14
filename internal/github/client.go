// Package github 封装 GitHub API 客户端：获取 PR diff、提交 review comment。
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client 是 GitHub API 的轻量客户端。
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// New 创建一个新的 GitHub API 客户端。
func New(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.github.com",
	}
}

// PRDiff 获取 PR 的 unified diff。
func (c *Client) PRDiff(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, prNumber)
	slog.Info("github: fetching PR diff", "url", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req, "application/vnd.github.v3.diff")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("fetch PR diff: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB limit
	if err != nil {
		return "", fmt.Errorf("read PR diff: %w", err)
	}
	return string(data), nil
}

// ReviewComment 是一条 review 评论。
type ReviewComment struct {
	Path     string `json:"path"`
	Position int    `json:"position,omitempty"` // diff 中的行号（传统方式）
	Line     int    `json:"line,omitempty"`     // 新文件行号
	Body     string `json:"body"`
}

// PostReview 在 PR 上提交 review（含多条评论）。
func (c *Client) PostReview(ctx context.Context, owner, repo string, prNumber int, headSHA string, body string, comments []ReviewComment) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, prNumber)

	payload := map[string]interface{}{
		"body":     body,
		"event":    "COMMENT",
		"comments": comments,
	}
	// 如果指定了 headSHA，可以关联到具体 commit
	if headSHA != "" {
		payload["commit_id"] = headSHA
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal review payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req, "application/vnd.github.v3+json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("post review: %w", err)
	}
	defer resp.Body.Close()

	// HTTP 201 Created on success
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("post review: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// PostIssueComment 在 PR 上发一条简单评论（用于跨文件 review 汇总）。
func (c *Client) PostIssueComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, prNumber)

	payload := map[string]string{"body": body}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal comment: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req, "application/vnd.github.v3+json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("post comment: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// setHeaders 设置 GitHub API 必要的 headers。
func (c *Client) setHeaders(req *http.Request, accept string) {
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "code-review-agent/1.0")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// doWithRetry 执行请求，处理 GitHub API 限流（429）。
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	const maxRetries = 3
	var lastErr error

	for i := range maxRetries {
		// 重试前重置 body：手动循环里复用同一个 req，bytes.Reader 在第一次 Do 后
		// 已读到 EOF，直接重试会发出空 body。GetBody 会返回全新的 reader。
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewind request body: %w", err)
			}
			req.Body = body
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			select {
			case <-time.After(time.Duration(i+1) * time.Second):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
			continue
		}

		// 处理 API 限流
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			retryAfter := resp.Header.Get("Retry-After")
			slog.Warn("github: rate limited", "retry_after", retryAfter)
			lastErr = fmt.Errorf("github rate limited (HTTP 429, Retry-After=%s)", retryAfter)
			wait := 60 * time.Second
			if d, err := time.ParseDuration(retryAfter + "s"); err == nil {
				wait = d
			}
			select {
			case <-time.After(wait):
				continue
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}

		// 401 或 403（非限流）表示 token 无效
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, fmt.Errorf("github auth failed (HTTP %d): %s", resp.StatusCode, string(body))
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
