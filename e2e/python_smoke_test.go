package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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
			`{"diff":"","new_path":"big.dat","old_path":"big.dat","a_mode":"100644","b_mode":"100644","too_large":true},`+
			`{"diff":"@@ -0,0 +1 @@\n+привет\n","new_path":"smoke/a.md","old_path":"smoke/a.md","a_mode":"0","b_mode":"100644","new_file":true},`+
			`{"diff":"@@ -1 +0,0 @@\n-x\n","new_path":"old.txt","old_path":"old.txt","a_mode":"100644","b_mode":"0","deleted_file":true}]`, nil)
	fake.Handle("POST", "/api/v4/projects/g%2Fp/repository/commits", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Branch  string `json:"branch"`
			Actions []struct {
				Action       string `json:"action"`
				FilePath     string `json:"file_path"`
				Content      string `json:"content"`
				LastCommitID string `json:"last_commit_id"`
			} `json:"actions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("commit body is not JSON: %v", err)
		}
		// Either the two-action commit_files body or the one-action update of
		// create_or_update_file; anything else fails the test.
		isCommitFiles := len(body.Actions) == 2 &&
			body.Actions[0].Action == "create" && body.Actions[0].FilePath == "smoke/a.md" &&
			body.Actions[1].Action == "delete" && body.Actions[1].FilePath == "old.txt"
		isUpsert := len(body.Actions) == 1 &&
			body.Actions[0].Action == "update" && body.Actions[0].FilePath == "README.md" &&
			body.Actions[0].Content == "# Hello\nworld\n" && body.Actions[0].LastCommitID == "c1"
		if body.Branch != "smoke/branch" || (!isCommitFiles && !isUpsert) {
			t.Errorf("unexpected commit body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"smoke commit",` +
			`"stats":{"additions":1,"deletions":3,"total":4},"web_url":"https://gitlab.example/g/p/-/commit/a1b2c3d4"}`))
	})

	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/compare", 200,
		`{"commits":[{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init","author_name":"Alice","committed_date":"2026-09-20T10:00:00Z"}],`+
			`"diffs":[{"diff":"@@ -1 +1 @@\n-old\n+new\n","new_path":"README.md","old_path":"README.md","a_mode":"100644","b_mode":"100644"}],`+
			`"compare_timeout":true,"compare_same_ref":false}`, nil)

	fake.JSON("GET", "/api/v4/projects/g%2Fp/merge_requests", 200,
		`[{"iid":5,"state":"opened","draft":false,"title":"Add x","source_branch":"f","target_branch":"main","author":{"username":"alice"}}]`, nil)
	fake.JSON("POST", "/api/v4/projects/g%2Fp/merge_requests", 201,
		`{"iid":5,"state":"opened","draft":false,"title":"smoke MR","source_branch":"smoke/branch","target_branch":"main",`+
			`"detailed_merge_status":"checking","web_url":"https://gitlab.example/g/p/-/merge_requests/5"}`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/merge_requests/5", 200,
		`{"iid":5,"title":"Add x","state":"opened","draft":false,"source_branch":"f","target_branch":"main","author":{"username":"alice"},`+
			`"detailed_merge_status":"mergeable","changes_count":"1","web_url":"https://gitlab.example/g/p/-/merge_requests/5"}`, nil)
	fake.JSON("PUT", "/api/v4/projects/g%2Fp/merge_requests/5", 200,
		`{"iid":5,"state":"opened","draft":false,"title":"smoke MR renamed","source_branch":"f","target_branch":"main",`+
			`"detailed_merge_status":"mergeable","web_url":"https://gitlab.example/g/p/-/merge_requests/5"}`, nil)
	fake.JSON("PUT", "/api/v4/projects/g%2Fp/merge_requests/5/merge", 200,
		`{"iid":5,"state":"merged","merge_commit_sha":"m1","title":"smoke MR renamed","source_branch":"f","target_branch":"main",`+
			`"web_url":"https://gitlab.example/g/p/-/merge_requests/5"}`, nil)
	fake.JSON("POST", "/api/v4/projects/g%2Fp/merge_requests/5/notes", 201,
		`{"id":42,"body":"smoke note","system":false}`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/merge_requests/5/diffs", 200,
		`[{"diff":"@@ -1 +1 @@\n-old\n+new\n","new_path":"a.txt","old_path":"a.txt","a_mode":"100644","b_mode":"100644"}]`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/merge_requests/5/notes", 200,
		`[{"id":10,"system":true,"body":"added 1 commit","author":{"username":"alice"},"created_at":"2026-09-21T10:00:00Z"},`+
			`{"id":11,"system":false,"body":"looks good","author":{"username":"bob"},"created_at":"2026-09-20T10:00:00Z"}]`, nil)

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
	if !strings.Contains(stdout.String(), "create smoke/a.md (+1/−0)") {
		t.Errorf("stdout does not contain the commit_files created-file line:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "delete old.txt (+0/−1)") {
		t.Errorf("stdout does not contain the commit_files deleted-file line:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "updated README.md") {
		t.Errorf("stdout does not contain the create_or_update_file result:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "!5 opened Add x f→main @alice") {
		t.Errorf("stdout does not contain the merge request list line:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "detailed_merge_status: mergeable") {
		t.Errorf("stdout does not contain the merge request status line:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "MR !5 создан") {
		t.Errorf("stdout does not contain the create_merge_request result:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "комментарий #42 добавлен к MR !5") {
		t.Errorf("stdout does not contain the create_merge_request_note result:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "MR !5 обновлён") {
		t.Errorf("stdout does not contain the update_merge_request result:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "MR !5 влит") {
		t.Errorf("stdout does not contain the merge_merge_request result:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), testToken) || strings.Contains(stderr.String(), testToken) {
		t.Errorf("smoke output leaks the token")
	}
}
