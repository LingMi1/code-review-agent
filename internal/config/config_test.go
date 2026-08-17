package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	_ = os.Unsetenv("GITHUB_TOKEN")
	_ = os.Unsetenv("COGNITION_ADDR")
	_ = os.Unsetenv("LISTEN_ADDR")
	_ = os.Unsetenv("SQLITE_PATH")
	_ = os.Unsetenv("ALLOWED_ORIGINS")

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
	_ = os.Unsetenv("GITHUB_TOKEN")
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

func TestParseOwners(t *testing.T) {
	got := parseOwners("LingMi1, FOO, lingmi1 ,")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got["lingmi1"] || !got["foo"] {
		t.Errorf("got = %v, want lingmi1 and foo", got)
	}
}

func TestAllowRepo(t *testing.T) {
	cfg := &Config{AllowedOwners: parseOwners("LingMi1")}
	for _, repo := range []string{"LingMi1/agent-go", "LingMi1/code-review-agent", "LINGMI1/x"} {
		if !cfg.AllowRepo(repo) {
			t.Errorf("AllowRepo(%q) = false, want true", repo)
		}
	}
	for _, repo := range []string{"Other/agent-go", "lingmi1", "", "bad-format"} {
		if cfg.AllowRepo(repo) {
			t.Errorf("AllowRepo(%q) = true, want false", repo)
		}
	}
}

func TestAllowRepoEmptyAllowsAll(t *testing.T) {
	cfg := &Config{AllowedOwners: map[string]bool{}}
	if !cfg.AllowRepo("anyone/anything") {
		t.Error("empty allowlist should allow all (dev mode)")
	}
}
