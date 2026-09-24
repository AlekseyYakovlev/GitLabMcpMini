package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const (
	upsertFileRoute = "/api/v4/projects/g%2Fp/repository/files/docs%2Fnew%2Emd"
	upsertGetReq    = "GET " + upsertFileRoute
)

func upsertArgs(content string) map[string]any {
	return map[string]any{
		"project":        "g/p",
		"path":           "docs/new.md",
		"content":        content,
		"branch":         "feature/x",
		"commit_message": "docs",
	}
}

// newUpsertFake answers the file read with the given status and body and the
// commit POST with a created commit.
func newUpsertFake(t *testing.T, getStatus int, getBody string) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", upsertFileRoute, getStatus, getBody, nil)
	fake.JSON("POST", commitPostPath, 201, newCommitJSON, nil)
	return fake
}

// singlePostAction returns the only action of the only commit POST.
func singlePostAction(t *testing.T, fake *testutil.FakeGitLab) map[string]any {
	t.Helper()
	var bodies []string
	for _, r := range fake.Recorded() {
		if r.Method == "POST" {
			bodies = append(bodies, r.Body)
		}
	}
	if len(bodies) != 1 {
		t.Fatalf("POST count = %d, want exactly 1; requests: %v", len(bodies), fake.Requests())
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &body); err != nil {
		t.Fatalf("decode body %q: %v", bodies[0], err)
	}
	if body["branch"] != "feature/x" || body["commit_message"] != "docs" {
		t.Errorf("body = %v", body)
	}
	acts, _ := body["actions"].([]any)
	if len(acts) != 1 {
		t.Fatalf("actions = %v, want exactly one", body["actions"])
	}
	m, _ := acts[0].(map[string]any)
	return m
}

func TestCreateOrUpdateFileCreatesWhenMissing(t *testing.T) {
	fake := newUpsertFake(t, 404, `{"message":"404 File Not Found"}`)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("# новый\r\n"))
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	if len(reqs) != 2 || reqs[0] != upsertGetReq+"?ref=feature%2Fx" || reqs[1] != commitPostReq {
		t.Fatalf("requests = %v", reqs)
	}
	m := singlePostAction(t, fake)
	if m["action"] != "create" || m["file_path"] != "docs/new.md" || m["content"] != "# новый\r\n" {
		t.Errorf("action = %v", m)
	}
	if _, has := m["last_commit_id"]; has {
		t.Errorf("create must not carry last_commit_id: %v", m)
	}

	lines := strings.Split(text, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), text)
	}
	if !strings.HasPrefix(lines[0], "created docs/new.md") ||
		!strings.Contains(lines[0], "a1b2c3d4") || !strings.Contains(lines[0], "feature/x") {
		t.Errorf("head line = %q", lines[0])
	}
	if lines[1] != "https://gitlab.example/g/p/-/commit/"+newCommitSHA {
		t.Errorf("web_url line = %q", lines[1])
	}
}

func TestCreateOrUpdateFileUpdatesWithLastCommitID(t *testing.T) {
	fake := newUpsertFake(t, 200, fileJSON("docs/new.md", []byte("old\n")))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("new\n"))
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	m := singlePostAction(t, fake)
	if m["action"] != "update" || m["file_path"] != "docs/new.md" || m["content"] != "new\n" {
		t.Errorf("action = %v", m)
	}
	if m["last_commit_id"] != "c1" {
		t.Errorf("last_commit_id = %v, want c1", m["last_commit_id"])
	}
	if !strings.HasPrefix(text, "updated docs/new.md") {
		t.Errorf("text = %q", text)
	}
	if strings.Contains(text, "CRLF") {
		t.Errorf("unexpected CRLF warning: %q", text)
	}
}

func TestCreateOrUpdateFileCRLFWarning(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		content  string
		warn     bool
	}{
		{"crlf to lf", "a\r\nb\r\n", "a\nb\n", true},
		{"crlf kept", "a\r\nb\r\n", "a\r\nc\r\n", false},
		{"lf to lf", "a\nb\n", "a\nc\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpsertFake(t, 200, fileJSON("docs/new.md", []byte(tc.existing)))
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "create_or_update_file", upsertArgs(tc.content))
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			if got := strings.Contains(text, "концы строк изменились CRLF→LF"); got != tc.warn {
				t.Errorf("warning present = %v, want %v; text:\n%s", got, tc.warn, text)
			}
			if m := singlePostAction(t, fake); m["content"] != tc.content {
				t.Errorf("content = %q, want it byte-for-byte %q", m["content"], tc.content)
			}
		})
	}
}

func TestCreateOrUpdateFileIdenticalContentStillPosts(t *testing.T) {
	fake := newUpsertFake(t, 200, fileJSON("docs/new.md", []byte("same\n")))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("same\n"))
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if m := singlePostAction(t, fake); m["action"] != "update" || m["content"] != "same\n" {
		t.Errorf("action = %v", m)
	}
}

func TestCreateOrUpdateFileBinaryExistingIsUpdated(t *testing.T) {
	fake := newUpsertFake(t, 200, fileJSON("docs/new.md", []byte("\x00\x01\r\n\x02")))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("text\n"))
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if strings.Contains(text, "CRLF") {
		t.Errorf("binary existing file must skip the CRLF check: %q", text)
	}
	if m := singlePostAction(t, fake); m["action"] != "update" || m["content"] != "text\n" {
		t.Errorf("action = %v", m)
	}
}

func TestCreateOrUpdateFileReadFailureSendsNoPost(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"forbidden", 403, `{"message":"403 Forbidden"}`, "403"},
		{"server error", 500, `{"message":"boom"}`, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpsertFake(t, tc.status, tc.body)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("x"))
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			if !strings.Contains(text, tc.want) {
				t.Errorf("text %q does not contain %q", text, tc.want)
			}
			if n := len(requestsTo(fake, "POST")); n != 0 {
				t.Errorf("a failed read must not lead to a write, got %v", fake.Requests())
			}
		})
	}
}

func TestCreateOrUpdateFileGuardsSendNoRequest(t *testing.T) {
	with := func(key string, value any) map[string]any {
		a := upsertArgs("x")
		a[key] = value
		return a
	}
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"dot dot", with("path", "../x"), ".."},
		{"empty path", with("path", " "), "не указан путь"},
		{"empty message", with("commit_message", " "), "commit_message не может быть пустым"},
		{"empty branch", with("branch", ""), "не указана ветка"},
		{"NUL in content", with("content", "a\u0000b"), "бинарн"},
		{"too big", with("content", strings.Repeat("a", maxCommitContentBytes+1)), "1 МБ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newUpsertFake(t, 200, fileJSON("docs/new.md", []byte("x")))
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "create_or_update_file", tc.args)
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			if !strings.Contains(text, tc.want) {
				t.Errorf("error %q does not contain %q", text, tc.want)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("guard must not send requests, got %v", reqs)
			}
		})
	}
}
