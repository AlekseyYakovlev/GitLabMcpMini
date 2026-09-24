package tools

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"gitlab-mcp/internal/testutil"
)

// TestMergeRequestFlow walks the agent scenario over one stateful fake: create,
// comment, wait for the merge check, merge, confirm.
func TestMergeRequestFlow(t *testing.T) {
	var (
		mu     sync.Mutex
		status = "checking"
		state  = "opened"
	)
	current := func() string {
		mu.Lock()
		defer mu.Unlock()
		body := mrJSON(status, "")
		if state == "merged" {
			body = strings.Replace(mrJSON("not_open", `"merge_commit_sha":"m1"`), `"state":"opened"`, `"state":"merged"`, 1)
		}
		return body
	}
	write := func(code int, body func() string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body()))
		}
	}

	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", compareRoute, 200, compareOneCommit, nil)
	fake.Handle("POST", mrsPath, write(201, current))
	fake.JSON("POST", mr5NotesPath, 201, `{"id":1,"body":"looks good","system":false}`, nil)
	fake.Handle("GET", mr5Path, write(200, current))
	fake.Handle("PUT", mr5MergePath, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		state = "merged"
		mu.Unlock()
		write(200, current)(w, r)
	})
	cs := newTestSession(t, fake)

	step := func(name string, args map[string]any, wants ...string) string {
		t.Helper()
		text, isErr := callText(t, cs, name, args)
		if isErr {
			t.Fatalf("%s: unexpected tool error: %s", name, text)
		}
		for _, w := range wants {
			if !strings.Contains(text, w) {
				t.Fatalf("%s: text %q lacks %q", name, text, w)
			}
		}
		return text
	}

	step("create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "f", "title": "Add x", "target_branch": "main",
	}, "MR !5 создан", "get_merge_request")

	step("create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "looks good"},
		"комментарий #1 добавлен к MR !5")

	step("get_merge_request", map[string]any{"project": "g/p", "iid": 5}, "checking", "проверка ещё идёт")

	mu.Lock()
	status = "mergeable"
	mu.Unlock()

	step("get_merge_request", map[string]any{"project": "g/p", "iid": 5}, "mergeable", "можно вливать")

	step("merge_merge_request", map[string]any{"project": "g/p", "iid": 5}, "MR !5 влит (state=merged)", "merge commit: m1")

	step("get_merge_request", map[string]any{"project": "g/p", "iid": 5}, "state: merged", "MR уже влит")

	creates, notes := 0, 0
	for _, r := range fake.Requests() {
		switch r {
		case "POST " + mrsPath:
			creates++
		case "POST " + mr5NotesPath:
			notes++
		}
	}
	if creates != 1 || notes != 1 {
		t.Errorf("POST merge_requests = %d, POST notes = %d, want 1 and 1", creates, notes)
	}
	if puts := requestsTo(fake, "PUT "); len(puts) != 1 || puts[0] != "PUT "+mr5MergePath {
		t.Errorf("PUT requests = %v, want exactly one merge", puts)
	}
}
