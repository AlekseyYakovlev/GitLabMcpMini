package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactor(t *testing.T) {
	tests := []struct {
		name  string
		token string
		in    string
		want  string
	}{
		{
			"exact token and pattern",
			"glpat-SECRET123",
			"x glpat-SECRET123 y glpat-OTHER_9-z",
			"x [REDACTED] y [REDACTED]",
		},
		{
			"empty token still redacts pattern",
			"",
			"a glpat-abc_DEF-1 b",
			"a [REDACTED] b",
		},
		{
			"non-pattern token redacted exactly",
			"plain-secret",
			"value plain-secret end",
			"value [REDACTED] end",
		},
		{"nothing to redact", "glpat-SECRET123", "hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewRedactor(tt.token)(tt.in); got != tt.want {
				t.Errorf("redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoggerRedactsMessageStringAndErrorAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelDebug, NewRedactor("glpat-SECRET123"))

	logger.Warn("msg glpat-SECRET123",
		"err", errors.New("tok glpat-SECRET123"),
		"s", "glpat-SECRET123")

	out := buf.String()
	if strings.Contains(out, "glpat-SECRET123") {
		t.Errorf("log output leaks the token: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("log output has no [REDACTED] marker: %q", out)
	}
}

func TestLoggerHonoursLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelWarn, NewRedactor(""))
	logger.Info("quiet")
	if buf.Len() != 0 {
		t.Errorf("info message was written at warn level: %q", buf.String())
	}
	logger.Warn("loud")
	if !strings.Contains(buf.String(), "loud") {
		t.Errorf("warn message missing: %q", buf.String())
	}
}
