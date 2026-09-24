package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPythonSmoke(t *testing.T) {
	uv, err := exec.LookPath("uv")
	if err != nil {
		t.Skip("uv is not on PATH; skipping the Python mcp 1.30.0 smoke test")
	}

	fake := newFakeWithWhoami(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, uv, "run", "../scripts/smoke.py", "--exe", binPath, "--base-url", fake.URL)
	cmd.Env = append(envWithout("GITLAB_TOKEN", "GITLAB_URL"), "GITLAB_TOKEN="+testToken)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("smoke.py failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "SMOKE OK") {
		t.Errorf("stdout does not contain SMOKE OK:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
		t.Errorf("smoke output leaks the token")
	}
}
