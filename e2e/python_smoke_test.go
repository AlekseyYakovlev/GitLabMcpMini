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
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/branches", 200,
		`[{"name":"main","default":true,"protected":true,"merged":false,"commit":{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init"}}]`, nil)
	fake.JSON("POST", "/api/v4/projects/g%2Fp/repository/branches", 201,
		`{"name":"smoke/branch","commit":{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init"},"web_url":"https://gitlab.example/g/p/-/tree/smoke/branch"}`, nil)

	const commitRoute = "/api/v4/projects/g%2Fp/repository/commits/a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0"
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits", 200,
		`[{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init","author_name":"Alice","committed_date":"2026-09-20T10:00:00Z"}]`, nil)
	fake.JSON("GET", commitRoute, 200,
		`{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init","author_name":"Alice","authored_date":"2026-09-20T10:00:00Z","message":"Init","parent_ids":[],"stats":{"additions":2,"deletions":1,"total":3},"web_url":"https://gitlab.example/g/p/-/commit/a1b2c3d4"}`, nil)
	fake.JSON("GET", commitRoute+"/diff", 200,
		`[{"diff":"@@ -1 +1,2 @@\n-old\n+new\n+more\n","new_path":"README.md","old_path":"README.md","a_mode":"100644","b_mode":"100644"},`+
			`{"diff":"","new_path":"big.dat","old_path":"big.dat","a_mode":"100644","b_mode":"100644","too_large":true}]`, nil)

	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/compare", 200,
		`{"commits":[{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init","author_name":"Alice","committed_date":"2026-09-20T10:00:00Z"}],`+
			`"diffs":[{"diff":"@@ -1 +1 @@\n-old\n+new\n","new_path":"README.md","old_path":"README.md","a_mode":"100644","b_mode":"100644"}],`+
			`"compare_timeout":true,"compare_same_ref":false}`, nil)

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
	if !strings.Contains(stdout.String(), "[protected]") {
		t.Errorf("stdout does not contain the branch protection marker:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "smoke/branch") {
		t.Errorf("stdout does not contain the created branch:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Hello") {
		t.Errorf("stdout does not contain the decoded README content:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "a1b2c3d4 2026-09-20 Alice Init") {
		t.Errorf("stdout does not contain the commit list line:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "too_large") {
		t.Errorf("stdout does not contain the too_large reason for the diff file:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "diff может быть неполным") {
		t.Errorf("stdout does not contain the compare_timeout warning:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
		t.Errorf("smoke output leaks the token")
	}
}
