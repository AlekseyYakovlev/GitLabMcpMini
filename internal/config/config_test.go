package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", true},
		{"blank", "   ", "", true},
		{"newline only", "\n", "", true},
		{"trimmed", " glpat-abc\n", "glpat-abc", false},
		{"plain", "glpat-abc", "glpat-abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(env(map[string]string{"GITLAB_TOKEN": tt.token}))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), "GITLAB_TOKEN") {
					t.Errorf("error %q does not name GITLAB_TOKEN", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Token != tt.want {
				t.Errorf("Token = %q, want %q", cfg.Token, tt.want)
			}
		})
	}
}

func TestLoadBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{"default", "", "https://gitlab.com", false},
		{"trailing slash", "https://gitlab.example.com/", "https://gitlab.example.com", false},
		{"https host", "https://gitlab.example.com", "https://gitlab.example.com", false},
		{"loopback ip", "http://127.0.0.1:5555", "http://127.0.0.1:5555", false},
		{"localhost", "http://localhost:1", "http://localhost:1", false},
		{"ipv6 loopback", "http://[::1]:8080", "http://[::1]:8080", false},
		{"plain http remote", "http://evil.example.com", "", true},
		{"ftp", "ftp://x", "", true},
		{"no scheme", "gitlab.com", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(env(map[string]string{"GITLAB_TOKEN": "glpat-x", "GITLAB_URL": tt.url}))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), "GITLAB_URL") {
					t.Errorf("error %q does not name GITLAB_URL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.BaseURL != tt.want {
				t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, tt.want)
			}
		})
	}
}

func TestLoadLogLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"", slog.LevelWarn},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"Warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"nonsense", slog.LevelWarn},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			cfg, err := Load(env(map[string]string{"GITLAB_TOKEN": "glpat-x", "LOG_LEVEL": tt.in}))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.LogLevel != tt.want {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.want)
			}
		})
	}
}

func TestErrorsNeverContainToken(t *testing.T) {
	_, err := Load(env(map[string]string{"GITLAB_TOKEN": "glpat-SECRET123", "GITLAB_URL": "http://evil.example.com"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "glpat-SECRET123") {
		t.Errorf("error leaks the token: %q", err)
	}
}
