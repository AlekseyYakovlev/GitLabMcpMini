package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestStdioHandshakeAndWhoami(t *testing.T) {
	fake := newFakeWithWhoami(t)
	s := startSession(t, fake.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The Go client negotiates the newest revision the SDK knows; the
	// 2025-11-25 revision used by the agent's client is asserted in smoke.py.
	if s.InitializeResult().ProtocolVersion == "" {
		t.Error("initialize returned no protocol version")
	}

	list, err := s.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "whoami" {
			found = true
		}
	}
	if !found {
		t.Fatalf("whoami not in tool list: %+v", list.Tools)
	}

	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Fatalf("fake GitLab saw requests during initialize + tools/list: %v", reqs)
	}

	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("CallTool whoami: %v", err)
	}
	text := resultText(res)
	if res.IsError {
		t.Fatalf("whoami returned isError: %s", text)
	}
	if !strings.Contains(text, "@alice") {
		t.Errorf("whoami text %q does not contain @alice", text)
	}
	if strings.Contains(text, testToken) {
		t.Errorf("whoami text leaks the token: %q", text)
	}
	if reqs := fake.Requests(); len(reqs) == 0 || reqs[0] != "GET /api/v4/user" {
		t.Errorf("requests = %v, want first GET /api/v4/user", reqs)
	}

	if err := s.Close(); err != nil {
		t.Logf("Close: %v", err)
	}
	if s.cmd.ProcessState == nil {
		t.Fatal("process has not exited after session Close")
	}
	if code := s.cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code after stdin close = %d, want 0", code)
	}
	if strings.Contains(s.stderr.String(), testToken) {
		t.Errorf("stderr leaks the token: %q", s.stderr.String())
	}
}

func TestStdioMissingToken(t *testing.T) {
	cmd := exec.Command(binPath)
	cmd.Env = envWithout("GITLAB_TOKEN", "GITLAB_URL")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("Wait error = %v, want *exec.ExitError with code 1", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("binary did not exit within 5 s without GITLAB_TOKEN")
	}

	if !strings.Contains(stderr.String(), "GITLAB_TOKEN") {
		t.Errorf("stderr %q does not mention GITLAB_TOKEN", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty, got %q", stdout.String())
	}
}

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
