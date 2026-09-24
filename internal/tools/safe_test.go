package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
