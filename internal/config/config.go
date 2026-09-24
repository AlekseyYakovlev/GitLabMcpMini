// Package config reads the server configuration from environment variables.
package config

import (
	"errors"
	"log/slog"
	"net/url"
	"strings"
)

// DefaultBaseURL is the GitLab instance used when GITLAB_URL is not set.
const DefaultBaseURL = "https://gitlab.com"

// Config is the validated server configuration. It deliberately has no
// String method so the token cannot be printed by accident.
type Config struct {
	Token    string
	BaseURL  string
	LogLevel slog.Level
}

// Load builds a Config from the environment. getenv is normally os.Getenv.
// Error texts never contain the token value.
func Load(getenv func(string) string) (Config, error) {
	token := strings.TrimSpace(getenv("GITLAB_TOKEN"))
	if token == "" {
		return Config{}, errors.New("GITLAB_TOKEN не задан: укажите Personal Access Token " +
			"(scope read_api для чтения, api для записи) в переменной окружения GITLAB_TOKEN")
	}

	baseURL, err := parseBaseURL(getenv("GITLAB_URL"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Token:    token,
		BaseURL:  baseURL,
		LogLevel: parseLogLevel(getenv("LOG_LEVEL")),
	}, nil
}

var errBadURL = errors.New("GITLAB_URL должен начинаться с https:// (http:// допускается только для localhost)")

func parseBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultBaseURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", errBadURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
		default:
			return "", errBadURL
		}
	default:
		return "", errBadURL
	}
	return strings.TrimRight(raw, "/"), nil
}

func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}
