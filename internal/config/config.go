package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Server ServerConfig `toml:"server"`
	SMTP   SMTPConfig   `toml:"smtp"`
	DB     DBConfig     `toml:"db"`
	API    APIConfig    `toml:"api"`
}

type ServerConfig struct {
	Name   string `toml:"name"`
	Domain string `toml:"domain"`
}

type SMTPConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	From     string `toml:"from"`
	FromName string `toml:"from_name"`
	TLS      bool   `toml:"tls"`
}

type DBConfig struct {
	Path string `toml:"path"`
}

type APIConfig struct {
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	APIKey     string `toml:"api_key"`
	CORS       string `toml:"cors"`
	RateLimit  int    `toml:"rate_limit"`  // requests per minute per IP
	TrustProxy bool   `toml:"trust_proxy"` // trust X-Forwarded-For headers (enable behind reverse proxy)
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name: "InkDrift Newsletter",
		},
		SMTP: SMTPConfig{
			Port: 587,
			TLS:  true,
		},
		DB: DBConfig{
			Path: "inkdrift.db",
		},
		API: APIConfig{
			Host:      "0.0.0.0",
			Port:      3377,
			CORS:      "*",
			RateLimit: 30,
		},
	}
}

// Load loads config from file, falling back to defaults if no file found.
// Environment variables always override file values.
func Load(path string) (*Config, error) {
	if path == "" {
		path = findConfig()
	}

	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}

		// Use strict decoding to catch config typos (e.g., "smpt" instead of "smtp")
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(cfg); err != nil {
			// Fall back to lenient parsing if strict fails (allows forward compat)
			if err2 := toml.Unmarshal(data, cfg); err2 != nil {
				return nil, fmt.Errorf("parsing config: %w", err2)
			}
			fmt.Fprintf(os.Stderr, "Warning: config has unknown keys: %v\n", err)
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// ConfigPath returns the path of the loaded config, or "" if using defaults.
func ConfigPath() string {
	return findConfig()
}

func findConfig() string {
	candidates := []string{
		"inkdrift.toml",
		filepath.Join(os.Getenv("HOME"), ".config", "inkdrift", "config.toml"),
		"/etc/inkdrift/config.toml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func applyEnvOverrides(cfg *Config) {
	// SMTP
	if v := os.Getenv("INKDRIFT_SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("INKDRIFT_SMTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = port
		}
	}
	if v := os.Getenv("INKDRIFT_SMTP_USERNAME"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("INKDRIFT_SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("INKDRIFT_SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("INKDRIFT_SMTP_FROM_NAME"); v != "" {
		cfg.SMTP.FromName = v
	}
	if v := os.Getenv("INKDRIFT_SMTP_TLS"); v != "" {
		cfg.SMTP.TLS = v == "true" || v == "1" || v == "yes"
	}

	// API
	if v := os.Getenv("INKDRIFT_API_KEY"); v != "" {
		cfg.API.APIKey = v
	}
	if v := os.Getenv("INKDRIFT_API_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.API.Port = port
		}
	}
	if v := os.Getenv("INKDRIFT_API_HOST"); v != "" {
		cfg.API.Host = v
	}
	if v := os.Getenv("INKDRIFT_CORS"); v != "" {
		cfg.API.CORS = v
	}
	if v := os.Getenv("INKDRIFT_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.API.RateLimit = n
		}
	}
	if v := os.Getenv("INKDRIFT_TRUST_PROXY"); v != "" {
		cfg.API.TrustProxy = v == "true" || v == "1"
	}

	// Server
	if v := os.Getenv("INKDRIFT_DOMAIN"); v != "" {
		cfg.Server.Domain = v
	}
	if v := os.Getenv("INKDRIFT_NAME"); v != "" {
		cfg.Server.Name = v
	}

	// DB
	if v := os.Getenv("INKDRIFT_DB_PATH"); v != "" {
		cfg.DB.Path = v
	}
}

// SMTPConfigured returns true if SMTP has the minimum required fields.
func (c *Config) SMTPConfigured() bool {
	return c.SMTP.Host != "" && c.SMTP.From != ""
}

// Validate returns warnings (not errors) about missing configuration.
func (c *Config) Validate() []string {
	var warnings []string

	if !c.SMTPConfigured() {
		warnings = append(warnings, "SMTP not configured — emails will fail to send (run: inkdrift init)")
	}
	if c.Server.Domain == "" {
		warnings = append(warnings, "Domain not set — unsubscribe links will use localhost")
	}
	if c.API.APIKey == "" {
		warnings = append(warnings, "API key not set — admin endpoints will be locked (set api_key in config or INKDRIFT_API_KEY env)")
	}

	return warnings
}

func Save(cfg *Config, path string) error {
	if path == "" {
		path = "inkdrift.toml"
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
