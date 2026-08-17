// Package config 集中管理应用配置：从环境变量读取并启动时校验。
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Config 是应用的全部配置项。
type Config struct {
	GitHubToken        string
	WebhookSecret      string
	APIToken           string
	CognitionAddr      string
	ListenAddr         string
	SQLitePath         string
	AllowedOrigins     map[string]bool
	AllowedOwners      map[string]bool
	OtelEndpoint       string
	OtelServiceName    string
	TrustXForwardedFor bool
}

// Load 从环境变量读取配置。
func Load() *Config {
	return &Config{
		GitHubToken:        os.Getenv("GITHUB_TOKEN"),
		WebhookSecret:      os.Getenv("WEBHOOK_SECRET"),
		APIToken:           os.Getenv("API_TOKEN"),
		CognitionAddr:      envDefault("COGNITION_ADDR", "localhost:50051"),
		ListenAddr:         envDefault("LISTEN_ADDR", ":8080"),
		SQLitePath:         envDefault("SQLITE_PATH", "./data/reviews.db"),
		AllowedOrigins:     parseOrigins(envDefault("ALLOWED_ORIGINS", "http://localhost:5173")),
		AllowedOwners:      parseOwners(os.Getenv("ALLOWED_OWNERS")),
		OtelEndpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OtelServiceName:    envDefault("OTEL_SERVICE_NAME", "code-review-agent"),
		TrustXForwardedFor: envBool("TRUST_X_FORWARDED_FOR", false),
	}
}

// Validate 校验必填项。缺少时返回 error。
func (c *Config) Validate() error {
	if c.GitHubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if c.WebhookSecret == "" {
		slog.Warn("WEBHOOK_SECRET not set; webhook signature verification is DISABLED — only safe for local development")
	}
	if c.APIToken == "" {
		slog.Warn("API_TOKEN not set; manual review trigger is unauthenticated — only safe for local development")
	}
	if len(c.AllowedOwners) == 0 {
		slog.Warn("ALLOWED_OWNERS not set; bot will review ANY repository — only safe for local development")
	}
	return nil
}

// envDefault 返回环境变量或默认值。
func envDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// envBool 解析布尔型环境变量，支持 1/true/yes/on（大小写不敏感）。
func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// parseOrigins 解析逗号分隔的 CORS origin 白名单。
func parseOrigins(raw string) map[string]bool {
	set := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			set[o] = true
		}
	}
	return set
}

// parseOwners 解析逗号分隔的仓库 owner 白名单，统一转小写（GitHub owner 大小写不敏感）。
func parseOwners(raw string) map[string]bool {
	set := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.ToLower(strings.TrimSpace(o))
		if o != "" {
			set[o] = true
		}
	}
	return set
}

// AllowRepo 判断某仓库（owner/repo）是否在允许名单内。空白名单 = 不限制（仅限本地开发）。
func (c *Config) AllowRepo(repoFullName string) bool {
	if len(c.AllowedOwners) == 0 {
		return true
	}
	owner, _, ok := strings.Cut(repoFullName, "/")
	if !ok || owner == "" {
		return false
	}
	return c.AllowedOwners[strings.ToLower(owner)]
}
