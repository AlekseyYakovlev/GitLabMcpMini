package tools

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/config"
	"gitlab-mcp/internal/glclient"
	"gitlab-mcp/internal/logging"
	"gitlab-mcp/internal/testutil"
)

const testToken = "glpat-TESTSECRET"

// newTestDeps builds Deps whose GitLab client talks to the fake server.
func newTestDeps(t testing.TB, fake *testutil.FakeGitLab) Deps {
	t.Helper()
	redact := logging.NewRedactor(testToken)
	logger := logging.New(io.Discard, slog.LevelDebug, redact)
	gl, err := glclient.New(config.Config{Token: testToken, BaseURL: fake.URL}, logger)
	if err != nil {
		t.Fatalf("glclient.New: %v", err)
	}
	return Deps{GL: gl, Logger: logger, Redact: redact, Timeout: CallTimeout}
}

// newTestSession registers every tool on a real server wired to the fake
// GitLab and returns a connected in-memory client session.
func newTestSession(t testing.TB, fake *testutil.FakeGitLab) *mcp.ClientSession {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "gitlab-mcp-test", Version: "0"}, nil)
	Register(s, newTestDeps(t, fake))
	return testutil.Connect(t, s)
}

// callText calls a tool and returns all text content joined together. A
// protocol-level error fails the test; tool errors are reported via isError.
// The text must never contain the token.
func callText(t testing.TB, cs *mcp.ClientSession, name string, args map[string]any) (text string, isError bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: protocol error: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	text = sb.String()
	if strings.Contains(text, testToken) {
		t.Fatalf("tool %s result leaks the token: %q", name, text)
	}
	return text, res.IsError
}
