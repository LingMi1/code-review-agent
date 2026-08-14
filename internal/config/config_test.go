package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("COGNITION_ADDR")
	os.Unsetenv("LISTEN_ADDR")
	os.Unsetenv("SQLITE_PATH")
	os.Unsetenv("ALLOWED_ORIGINS")

	cfg := Load()

	if cfg.CognitionAddr != "localhost:50051" {
		t.Errorf("CognitionAddr = %q, want localhost:50051", cfg.CognitionAddr)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.SQLitePath != "./data/reviews.db" {
		t.Errorf("SQLitePath = %q, want ./data/reviews.db", cfg.SQLitePath)
	}
	if cfg.OtelServiceName != "code-review-agent" {
		t.Errorf("OtelServiceName = %q, want code-review-agent", cfg.OtelServiceName)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	t.Setenv("COGNITION_ADDR", "remote:50051")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("ALLOWED_ORIGINS", "http://a.com,http://b.com")

	cfg := Load()

	if cfg.GitHubToken != "ghp_test" {
		t.Errorf("GitHubToken = %q, want ghp_test", cfg.GitHubToken)
	}
	if cfg.CognitionAddr != "remote:50051" {
		t.Errorf("CognitionAddr = %q, want remote:50051", cfg.CognitionAddr)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if !cfg.AllowedOrigins["http://a.com"] || !cfg.AllowedOrigins["http://b.com"] {
		t.Errorf("AllowedOrigins = %v, want http://a.com and http://b.com", cfg.AllowedOrigins)
	}
}

func TestValidateRequiresGitHubToken(t *testing.T) {
	os.Unsetenv("GITHUB_TOKEN")
	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should error when GITHUB_TOKEN is empty")
	}
}

func TestValidatePassesWithToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestParseOrigins(t *testing.T) {
	got := parseOrigins("http://a.com, http://b.com ,, http://c.com")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for _, o := range []string{"http://a.com", "http://b.com", "http://c.com"} {
		if !got[o] {
			t.Errorf("expected %q in origins", o)
		}
	}
}
