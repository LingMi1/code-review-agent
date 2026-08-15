package webhook

import (
	"encoding/json"
	"fmt"
)

// parsePREvent 从 GitHub webhook body 解析 PR 事件信息。
// 只提取必要字段，避免依赖完整的 GitHub webhook payload struct。
func parsePREvent(body []byte) (*PullRequestEvent, error) {
	var raw struct {
		Action string `json:"action"`
		PR     *struct {
			Number int `json:"number"`
			Head   struct {
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
		Repository *struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse webhook JSON: %w", err)
	}

	if raw.PR == nil || raw.Repository == nil {
		return nil, fmt.Errorf("missing pull_request or repository in webhook payload (action=%q)", raw.Action)
	}

	return &PullRequestEvent{
		Action:       raw.Action,
		PRNumber:     raw.PR.Number,
		RepoFullName: raw.Repository.FullName,
		CloneURL:     raw.Repository.CloneURL,
		HeadSHA:      raw.PR.Head.SHA,
		BaseSHA:      raw.PR.Base.SHA,
	}, nil
}
