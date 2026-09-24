// Package e2e runs the compiled gitlab-mcp binary as a real subprocess.
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/testutil"
)

const testToken = "glpat-TESTSECRET"

// binPath is the path of the binary built once by TestMain.
var binPath string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "gitlab-mcp-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: create temp dir:", err)
		return 1
	}
	defer os.RemoveAll(dir)

	name := "gitlab-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binPath = filepath.Join(dir, name)

	build := exec.Command("go", "build", "-o", binPath, "./cmd/gitlab-mcp")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build binary: %v\n%s\n", err, out)
		return 1
	}
	return m.Run()
}

// envWithout returns os.Environ() minus the given variable names, so a
// developer's real GITLAB_TOKEN or GITLAB_URL never leaks into a test.
func envWithout(keys ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		skip := false
		for _, k := range keys {
			if strings.EqualFold(name, k) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, kv)
		}
	}
	return out
}

// newFakeWithWhoami starts a fake GitLab that answers the two whoami routes.
func newFakeWithWhoami(t testing.TB) *testutil.FakeGitLab {
	t.Helper()
	f := testutil.NewFakeGitLab(t)
	f.JSON("GET", "/api/v4/user", 200,
		`{"id":42,"username":"alice","name":"Alice Liddell","state":"active","web_url":"https://gitlab.com/alice"}`, nil)
	f.JSON("GET", "/api/v4/personal_access_tokens/self", 200,
		`{"id":1,"name":"t","scopes":["api"],"expires_at":"2026-12-31","active":true}`, nil)
	return f
}

// session bundles a connected client session with the process behind it.
type session struct {
	*mcp.ClientSession
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

// startSession launches the built binary against fakeURL and completes the
// MCP initialize handshake. Cleanup closes the session first and then waits
// for the process, so the temp dir can be removed on Windows.
func startSession(t *testing.T, fakeURL string) *session {
	t.Helper()
	cmd := exec.Command(binPath)
	cmd.Env = append(envWithout("GITLAB_TOKEN", "GITLAB_URL"),
		"GITLAB_TOKEN="+testToken, "GITLAB_URL="+fakeURL)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect to %s: %v\nstderr: %s", binPath, err, stderr.String())
	}
	s := &session{ClientSession: cs, cmd: cmd, stderr: stderr}
	t.Cleanup(func() { _ = cs.Close() })
	return s
}
