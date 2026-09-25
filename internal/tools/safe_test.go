package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/glclient"
	"gitlab-mcp/internal/testutil"
)

func TestSafeRecoversPanic(t *testing.T) {
	deps := newTestDeps(t, testutil.NewFakeGitLab(t))

	s := mcp.NewServer(&mcp.Implementation{Name: "safe-test", Version: "0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "boom", Description: "test"},
		safe(deps, func(ctx context.Context, in struct{}) (string, error) {
			panic("kaboom " + testToken)
		}))
	mcp.AddTool(s, &mcp.Tool{Name: "ok", Description: "test"},
		safe(deps, func(ctx context.Context, in struct{}) (string, error) {
			return "fine", nil
		}))
	cs := testutil.Connect(t, s)

	text, isErr := callText(t, cs, "boom", nil)
	if !isErr {
		t.Errorf("panic result isError = false, text %q", text)
	}
	if !strings.HasPrefix(text, "внутренняя ошибка: ") {
		t.Errorf("text %q does not start with the internal error prefix", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("text %q does not contain [REDACTED]", text)
	}
	if strings.Contains(text, testToken) {
		t.Errorf("text %q leaks the token", text)
	}

	// The server must survive the panic and answer the next call.
	text, isErr = callText(t, cs, "ok", nil)
	if isErr || text != "fine" {
		t.Errorf("after panic: text %q isError %v, want \"fine\" false", text, isErr)
	}
}

func TestSafeRedactsErrorText(t *testing.T) {
	deps := newTestDeps(t, testutil.NewFakeGitLab(t))

	s := mcp.NewServer(&mcp.Implementation{Name: "safe-test", Version: "0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "leaky", Description: "test"},
		safe(deps, func(ctx context.Context, in struct{}) (string, error) {
			return "", errors.New("bad " + testToken)
		}))
	cs := testutil.Connect(t, s)

	text, isErr := callText(t, cs, "leaky", nil)
	if !isErr {
		t.Errorf("error result isError = false, text %q", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("text %q does not contain [REDACTED]", text)
	}
	if strings.Contains(text, testToken) {
		t.Errorf("text %q leaks the token", text)
	}
}

func TestDeadlineAsServerError(t *testing.T) {
	timeout := &glclient.Error{Kind: glclient.KindTimeout}
	tests := []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{name: "read deadline after 500", err: withSubject("проекты", context.DeadlineExceeded), status: 500,
			want: "500: ошибка сервера GitLab, повторите позже."},
		{name: "read deadline after 502", err: timeout, status: 502,
			want: "502: ошибка сервера GitLab, повторите позже."},
		{name: "deadline without 5xx stays a timeout", err: timeout, status: 0,
			want: "Превышено время ожидания (25 с)."},
		{name: "write deadline keeps unknown-outcome wording", err: withWrite(opCommit, "проект", timeout), status: 500,
			want: "Превышено время ожидания (25 с). Результат записи неизвестен"},
		{name: "non-timeout error untouched", err: &glclient.Error{Kind: glclient.KindNotFound, Status: 404}, status: 500,
			want: "404: не найдено"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toToolText(deadlineAsServerError(tt.err, tt.status))
			if !strings.Contains(got, tt.want) {
				t.Errorf("text = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// A read that keeps getting 5xx until the call deadline reports the real
// status, through the real client and retry policy.
func TestListProjectsDeadlineAfter5xxReportsStatus(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 500, `<html>Internal Server Error</html>`, nil)

	s := mcp.NewServer(&mcp.Implementation{Name: "gitlab-mcp-test", Version: "0"}, nil)
	d := newTestDeps(t, fake)
	d.Timeout = 300 * time.Millisecond // shorter than the 500 ms retry backoff
	Register(s, d)
	cs := testutil.Connect(t, s)

	text, isErr := callText(t, cs, "list_projects", map[string]any{})
	if !isErr {
		t.Fatalf("want a tool error, got %q", text)
	}
	if text != "500: ошибка сервера GitLab, повторите позже." {
		t.Errorf("text = %q, want the real 500 status instead of a timeout", text)
	}
}
