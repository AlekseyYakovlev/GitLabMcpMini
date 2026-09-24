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
	// The router matches exact raw paths and ignores the query string. The
	// unknown project the script asks for falls through to the default 404.
	fake.JSON("GET", "/api/v4/projects", 200,
		`[{"id":1,"path_with_namespace":"g/p","default_branch":"main","visibility":"private"}]`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200,
		`{"id":1,"path_with_namespace":"g/p","default_branch":"main","visibility":"private"}`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/tree", 200,
		`[{"id":"a","name":"README.md","type":"blob","path":"README.md","mode":"100644"}]`,
		map[string]string{"X-Next-Page": "2"})
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/files/README%2Emd", 200,
		`{"file_name":"README.md","file_path":"README.md","size":12,"encoding":"base64","content":"IyBIZWxsbwp3b3JsZAo=","ref":"main","blob_id":"b1","last_commit_id":"c1"}`, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, uv, "run", "../scripts/smoke.py", "--exe", binPath, "--base-url", fake.URL,
		"--project", "g/p", "--file", "README.md")
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
	if !strings.Contains(stdout.String(), "есть следующая страница") {
		t.Errorf("stdout does not contain the tree pagination footer:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Hello") {
		t.Errorf("stdout does not contain the decoded README content:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
		t.Errorf("smoke output leaks the token")
	}
}
