// Package logging builds the stderr logger and the token redactor.
package logging

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

var patPattern = regexp.MustCompile(`glpat-[A-Za-z0-9_\-]+`)

// NewRedactor returns a function that replaces the exact token (when it is
// not empty) and anything shaped like a GitLab personal access token with
// [REDACTED].
func NewRedactor(token string) func(string) string {
	return func(s string) string {
		if token != "" {
			s = strings.ReplaceAll(s, token, redacted)
		}
		return patPattern.ReplaceAllString(s, redacted)
	}
}

// New returns a text logger writing to w. Messages, string attributes and
// error attributes all pass through redact before they are written.
func New(w io.Writer, level slog.Level, redact func(string) string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.MessageKey {
				return slog.String(a.Key, redact(a.Value.String()))
			}
			switch a.Value.Kind() {
			case slog.KindString:
				return slog.String(a.Key, redact(a.Value.String()))
			case slog.KindAny:
				if err, ok := a.Value.Any().(error); ok {
					return slog.String(a.Key, redact(err.Error()))
				}
			}
			return a
		},
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
