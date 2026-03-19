package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Name != "InkDrift Newsletter" {
		t.Errorf("expected default name 'InkDrift Newsletter', got %q", cfg.Server.Name)
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("expected default SMTP port 587, got %d", cfg.SMTP.Port)
	}
	if !cfg.SMTP.TLS {
		t.Error("expected TLS to be true by default")
	}
	if cfg.DB.Path != "inkdrift.db" {
		t.Errorf("expected default DB path 'inkdrift.db', got %q", cfg.DB.Path)
	}
	if cfg.API.Host != "0.0.0.0" {
		t.Errorf("expected default API host '0.0.0.0', got %q", cfg.API.Host)
	}
	if cfg.API.Port != 3377 {
		t.Errorf("expected default API port 3377, got %d", cfg.API.Port)
	}
	if cfg.API.CORS != "*" {
		t.Errorf("expected default CORS '*', got %q", cfg.API.CORS)
	}
	if cfg.API.RateLimit != 30 {
		t.Errorf("expected default rate limit 30, got %d", cfg.API.RateLimit)
	}
}

func TestSMTPConfigured(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SMTPConfigured() {
		t.Error("default config should not have SMTP configured")
	}

	cfg.SMTP.Host = "smtp.example.com"
	if cfg.SMTPConfigured() {
		t.Error("should need From too")
	}

	cfg.SMTP.From = "test@example.com"
	if !cfg.SMTPConfigured() {
		t.Error("should be configured with host and from")
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		t.Error("default config should have warnings")
	}

	// Check specific warnings
	hasSmtp := false
	hasDomain := false
	hasApiKey := false
	for _, w := range warnings {
		if contains(w, "SMTP") {
			hasSmtp = true
		}
		if contains(w, "Domain") || contains(w, "domain") {
			hasDomain = true
		}
		if contains(w, "API key") || contains(w, "api_key") {
			hasApiKey = true
		}
	}
	if !hasSmtp {
		t.Error("expected SMTP warning")
	}
	if !hasDomain {
		t.Error("expected domain warning")
	}
	if !hasApiKey {
		t.Error("expected API key warning")
	}

	// Fully configured should have no warnings
	cfg.SMTP.Host = "smtp.example.com"
	cfg.SMTP.From = "test@example.com"
	cfg.Server.Domain = "example.com"
	cfg.API.APIKey = "secret"
	warnings = cfg.Validate()
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")

	content := `
[server]
name = "My Newsletter"
domain = "example.com"

[smtp]
host = "smtp.example.com"
port = 465
username = "user"
password = "pass"
from = "news@example.com"
from_name = "News"
tls = true

[db]
path = "/data/test.db"

[api]
host = "127.0.0.1"
port = 8080
api_key = "test-key"
cors = "https://example.com"
rate_limit = 60
trust_proxy = true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Name != "My Newsletter" {
		t.Errorf("expected name 'My Newsletter', got %q", cfg.Server.Name)
	}
	if cfg.Server.Domain != "example.com" {
		t.Errorf("expected domain 'example.com', got %q", cfg.Server.Domain)
	}
	if cfg.SMTP.Host != "smtp.example.com" {
		t.Errorf("expected SMTP host 'smtp.example.com', got %q", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 465 {
		t.Errorf("expected SMTP port 465, got %d", cfg.SMTP.Port)
	}
	if cfg.SMTP.Username != "user" {
		t.Errorf("expected username 'user', got %q", cfg.SMTP.Username)
	}
	if cfg.SMTP.FromName != "News" {
		t.Errorf("expected from_name 'News', got %q", cfg.SMTP.FromName)
	}
	if cfg.DB.Path != "/data/test.db" {
		t.Errorf("expected DB path '/data/test.db', got %q", cfg.DB.Path)
	}
	if cfg.API.Host != "127.0.0.1" {
		t.Errorf("expected API host '127.0.0.1', got %q", cfg.API.Host)
	}
	if cfg.API.Port != 8080 {
		t.Errorf("expected API port 8080, got %d", cfg.API.Port)
	}
	if cfg.API.APIKey != "test-key" {
		t.Errorf("expected API key 'test-key', got %q", cfg.API.APIKey)
	}
	if cfg.API.RateLimit != 60 {
		t.Errorf("expected rate limit 60, got %d", cfg.API.RateLimit)
	}
	if !cfg.API.TrustProxy {
		t.Error("expected trust_proxy true")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	// Should return defaults when no config file found
	if cfg.Server.Name != "InkDrift Newsletter" {
		t.Errorf("expected default name, got %q", cfg.Server.Name)
	}
}

func TestLoadInvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	os.WriteFile(path, []byte("not valid [[[toml"), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.toml")
	content := `
[server]
name = "Test"

[unknown_section]
key = "value"
`
	os.WriteFile(path, []byte(content), 0o644)

	// Should succeed with lenient fallback
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("should succeed with unknown keys (lenient fallback), got: %v", err)
	}
	if cfg.Server.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", cfg.Server.Name)
	}
}

func TestEnvOverrides(t *testing.T) {
	// Save and restore env
	envVars := []string{
		"INKDRIFT_SMTP_HOST", "INKDRIFT_SMTP_PORT", "INKDRIFT_SMTP_USERNAME",
		"INKDRIFT_SMTP_PASSWORD", "INKDRIFT_SMTP_FROM", "INKDRIFT_SMTP_FROM_NAME",
		"INKDRIFT_SMTP_TLS", "INKDRIFT_API_KEY", "INKDRIFT_API_PORT",
		"INKDRIFT_API_HOST", "INKDRIFT_CORS", "INKDRIFT_RATE_LIMIT",
		"INKDRIFT_TRUST_PROXY", "INKDRIFT_DOMAIN", "INKDRIFT_NAME", "INKDRIFT_DB_PATH",
	}
	saved := make(map[string]string)
	for _, v := range envVars {
		saved[v] = os.Getenv(v)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})

	// Set all env vars
	os.Setenv("INKDRIFT_SMTP_HOST", "env-smtp.example.com")
	os.Setenv("INKDRIFT_SMTP_PORT", "2525")
	os.Setenv("INKDRIFT_SMTP_USERNAME", "envuser")
	os.Setenv("INKDRIFT_SMTP_PASSWORD", "envpass")
	os.Setenv("INKDRIFT_SMTP_FROM", "env@example.com")
	os.Setenv("INKDRIFT_SMTP_FROM_NAME", "Env Sender")
	os.Setenv("INKDRIFT_SMTP_TLS", "false")
	os.Setenv("INKDRIFT_API_KEY", "env-key")
	os.Setenv("INKDRIFT_API_PORT", "9090")
	os.Setenv("INKDRIFT_API_HOST", "127.0.0.1")
	os.Setenv("INKDRIFT_CORS", "https://env.example.com")
	os.Setenv("INKDRIFT_RATE_LIMIT", "100")
	os.Setenv("INKDRIFT_TRUST_PROXY", "true")
	os.Setenv("INKDRIFT_DOMAIN", "env.example.com")
	os.Setenv("INKDRIFT_NAME", "Env Newsletter")
	os.Setenv("INKDRIFT_DB_PATH", "/env/data.db")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.SMTP.Host != "env-smtp.example.com" {
		t.Errorf("SMTP host: got %q", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 2525 {
		t.Errorf("SMTP port: got %d", cfg.SMTP.Port)
	}
	if cfg.SMTP.Username != "envuser" {
		t.Errorf("SMTP username: got %q", cfg.SMTP.Username)
	}
	if cfg.SMTP.Password != "envpass" {
		t.Errorf("SMTP password: got %q", cfg.SMTP.Password)
	}
	if cfg.SMTP.From != "env@example.com" {
		t.Errorf("SMTP from: got %q", cfg.SMTP.From)
	}
	if cfg.SMTP.FromName != "Env Sender" {
		t.Errorf("SMTP from_name: got %q", cfg.SMTP.FromName)
	}
	if cfg.SMTP.TLS {
		t.Error("SMTP TLS should be false")
	}
	if cfg.API.APIKey != "env-key" {
		t.Errorf("API key: got %q", cfg.API.APIKey)
	}
	if cfg.API.Port != 9090 {
		t.Errorf("API port: got %d", cfg.API.Port)
	}
	if cfg.API.Host != "127.0.0.1" {
		t.Errorf("API host: got %q", cfg.API.Host)
	}
	if cfg.API.CORS != "https://env.example.com" {
		t.Errorf("CORS: got %q", cfg.API.CORS)
	}
	if cfg.API.RateLimit != 100 {
		t.Errorf("rate limit: got %d", cfg.API.RateLimit)
	}
	if !cfg.API.TrustProxy {
		t.Error("trust_proxy should be true")
	}
	if cfg.Server.Domain != "env.example.com" {
		t.Errorf("domain: got %q", cfg.Server.Domain)
	}
	if cfg.Server.Name != "Env Newsletter" {
		t.Errorf("name: got %q", cfg.Server.Name)
	}
	if cfg.DB.Path != "/env/data.db" {
		t.Errorf("DB path: got %q", cfg.DB.Path)
	}
}

func TestEnvOverrideInvalidPort(t *testing.T) {
	saved := os.Getenv("INKDRIFT_SMTP_PORT")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_SMTP_PORT")
		} else {
			os.Setenv("INKDRIFT_SMTP_PORT", saved)
		}
	})

	os.Setenv("INKDRIFT_SMTP_PORT", "not-a-number")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	// Should keep default when env value is invalid
	if cfg.SMTP.Port != 587 {
		t.Errorf("expected default port 587 for invalid env, got %d", cfg.SMTP.Port)
	}
}

func TestEnvOverrideTLSVariants(t *testing.T) {
	saved := os.Getenv("INKDRIFT_SMTP_TLS")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_SMTP_TLS")
		} else {
			os.Setenv("INKDRIFT_SMTP_TLS", saved)
		}
	})

	for _, val := range []string{"true", "1", "yes"} {
		os.Setenv("INKDRIFT_SMTP_TLS", val)
		cfg := DefaultConfig()
		cfg.SMTP.TLS = false
		applyEnvOverrides(cfg)
		if !cfg.SMTP.TLS {
			t.Errorf("TLS should be true for env value %q", val)
		}
	}

	for _, val := range []string{"false", "0", "no"} {
		os.Setenv("INKDRIFT_SMTP_TLS", val)
		cfg := DefaultConfig()
		cfg.SMTP.TLS = true
		applyEnvOverrides(cfg)
		if cfg.SMTP.TLS {
			t.Errorf("TLS should be false for env value %q", val)
		}
	}
}

func TestEnvOverrideTrustProxyVariants(t *testing.T) {
	saved := os.Getenv("INKDRIFT_TRUST_PROXY")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_TRUST_PROXY")
		} else {
			os.Setenv("INKDRIFT_TRUST_PROXY", saved)
		}
	})

	os.Setenv("INKDRIFT_TRUST_PROXY", "1")
	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	if !cfg.API.TrustProxy {
		t.Error("trust_proxy should be true for '1'")
	}

	os.Setenv("INKDRIFT_TRUST_PROXY", "false")
	cfg = DefaultConfig()
	applyEnvOverrides(cfg)
	if cfg.API.TrustProxy {
		t.Error("trust_proxy should be false for 'false'")
	}
}

func TestEnvOverrideNegativeRateLimit(t *testing.T) {
	saved := os.Getenv("INKDRIFT_RATE_LIMIT")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_RATE_LIMIT")
		} else {
			os.Setenv("INKDRIFT_RATE_LIMIT", saved)
		}
	})

	os.Setenv("INKDRIFT_RATE_LIMIT", "-5")
	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	// Negative rate limit should be ignored
	if cfg.API.RateLimit != 30 {
		t.Errorf("expected default rate limit 30 for negative env, got %d", cfg.API.RateLimit)
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.toml")

	cfg := DefaultConfig()
	cfg.Server.Name = "Saved Newsletter"
	cfg.SMTP.Host = "smtp.saved.com"
	cfg.SMTP.From = "saved@example.com"

	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("saved config file is empty")
	}

	// Verify file permissions (0600)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}

	// Load it back
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Name != "Saved Newsletter" {
		t.Errorf("expected name 'Saved Newsletter', got %q", loaded.Server.Name)
	}
	if loaded.SMTP.Host != "smtp.saved.com" {
		t.Errorf("expected SMTP host 'smtp.saved.com', got %q", loaded.SMTP.Host)
	}
}

func TestSaveConfigCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "config.toml")

	cfg := DefaultConfig()
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Error("file should have been created with nested directories")
	}
}

func TestSaveConfigDefaultPath(t *testing.T) {
	// Save with empty path should use "inkdrift.toml"
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	cfg := DefaultConfig()
	if err := Save(cfg, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "inkdrift.toml")); err != nil {
		t.Error("should have created inkdrift.toml in current directory")
	}
}

func TestSaveConfigInvalidPath(t *testing.T) {
	cfg := DefaultConfig()
	err := Save(cfg, "/nonexistent/deeply/nested/dir/config.toml")
	// Should succeed because Save calls MkdirAll -- but will fail on WriteFile if permissions issue
	// On most systems this will fail at MkdirAll due to /nonexistent not being writable
	if err == nil {
		// Clean up if it somehow succeeded
		os.RemoveAll("/nonexistent")
	}
	// We just verify it doesn't panic
}

func TestEnvOverrideInvalidAPIPort(t *testing.T) {
	saved := os.Getenv("INKDRIFT_API_PORT")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_API_PORT")
		} else {
			os.Setenv("INKDRIFT_API_PORT", saved)
		}
	})

	os.Setenv("INKDRIFT_API_PORT", "not-a-number")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	if cfg.API.Port != 3377 {
		t.Errorf("expected default port 3377 for invalid env, got %d", cfg.API.Port)
	}
}

func TestEnvOverrideInvalidRateLimit(t *testing.T) {
	saved := os.Getenv("INKDRIFT_RATE_LIMIT")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_RATE_LIMIT")
		} else {
			os.Setenv("INKDRIFT_RATE_LIMIT", saved)
		}
	})

	os.Setenv("INKDRIFT_RATE_LIMIT", "not-a-number")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	if cfg.API.RateLimit != 30 {
		t.Errorf("expected default rate limit 30 for invalid env, got %d", cfg.API.RateLimit)
	}
}

func TestEnvOverrideZeroRateLimit(t *testing.T) {
	saved := os.Getenv("INKDRIFT_RATE_LIMIT")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_RATE_LIMIT")
		} else {
			os.Setenv("INKDRIFT_RATE_LIMIT", saved)
		}
	})

	os.Setenv("INKDRIFT_RATE_LIMIT", "0")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)
	// Zero rate limit should be ignored (n > 0 check)
	if cfg.API.RateLimit != 30 {
		t.Errorf("expected default rate limit 30 for zero env, got %d", cfg.API.RateLimit)
	}
}

func TestEnvOverridesFileValues(t *testing.T) {
	// Env vars should override values from config file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
[smtp]
host = "file-host.com"
from = "file@example.com"
`
	os.WriteFile(path, []byte(content), 0o644)

	saved := os.Getenv("INKDRIFT_SMTP_HOST")
	t.Cleanup(func() {
		if saved == "" {
			os.Unsetenv("INKDRIFT_SMTP_HOST")
		} else {
			os.Setenv("INKDRIFT_SMTP_HOST", saved)
		}
	})

	os.Setenv("INKDRIFT_SMTP_HOST", "env-host.com")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTP.Host != "env-host.com" {
		t.Errorf("env should override file: got %q", cfg.SMTP.Host)
	}
	if cfg.SMTP.From != "file@example.com" {
		t.Errorf("file value should remain when no env override: got %q", cfg.SMTP.From)
	}
}
