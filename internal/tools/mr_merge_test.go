package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

const mr5MergePath = mr5Path + "/merge"

// mergedMRJSON is the PUT answer of a successful merge.
func mergedMRJSON(extra string) string {
	body := `"merge_commit_sha":"m1","should_remove_source_branch":false`
	if extra != "" {
		body = extra
	}
	return strings.Replace(mrJSON("not_open", body), `"state":"opened"`, `"state":"merged"`, 1)
}

// newMergeFake answers the pre-check GET with a mergeable MR and the merge PUT
// with a merged one.
func newMergeFake(t testing.TB) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", ""), nil)
	fake.JSON("PUT", mr5MergePath, 200, mergedMRJSON(""), nil)
	return fake
}

// mergeBody returns the decoded body of the only merge PUT. An empty body is
// an object with no keys.
func mergeBody(t testing.TB, fake *testutil.FakeGitLab) map[string]any {
	t.Helper()
	var bodies []string
	for _, r := range fake.Recorded() {
		if r.Method == "PUT" {
			bodies = append(bodies, r.Body)
		}
	}
	if len(bodies) != 1 {
		t.Fatalf("PUT count = %d, want exactly 1", len(bodies))
	}
	body := map[string]any{}
	if len(bodies[0]) > 0 {
		if err := json.Unmarshal([]byte(bodies[0]), &body); err != nil {
			t.Fatalf("body %q: %v", bodies[0], err)
		}
	}
	return body
}

func TestMergeDescriptionLength(t *testing.T) {
	if n := utf8.RuneCountInString(mergeMergeRequestDescription); n > 900 {
		t.Errorf("description has %d runes, want at most 900", n)
	}
}

func TestMergeMergeRequestDefaults(t *testing.T) {
	fake := newMergeFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	want := []string{"GET " + mr5Path, "PUT " + mr5MergePath}
	if reqs := fake.Requests(); len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}
	if body := mergeBody(t, fake); len(body) != 0 {
		t.Errorf("merge body = %v, want no keys", body)
	}
	for _, w := range []string{
		"MR !5 влит (state=merged)",
		"merge commit: m1",
		"удаление ветки-источника: не запрошено",
		"https://gitlab.example/g/p/-/merge_requests/5",
	} {
		if !strings.Contains(text, w) {
			t.Errorf("text %q lacks %q", text, w)
		}
	}
	if strings.Contains(text, "удалена") {
		t.Errorf("text %q must not claim the branch was deleted", text)
	}
}

func TestMergeMergeRequestSendsOptionsOnlyWhenSet(t *testing.T) {
	fake := newMergeFake(t)
	fake.JSON("PUT", mr5MergePath, 200, mergedMRJSON(`"merge_commit_sha":"m1","should_remove_source_branch":true`), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{
		"project": "g/p", "iid": 5, "squash": true, "remove_source_branch": true, "merge_commit_message": "Merge x",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	body := mergeBody(t, fake)
	if len(body) != 3 || body["squash"] != true || body["should_remove_source_branch"] != true || body["merge_commit_message"] != "Merge x" {
		t.Errorf("merge body = %v, want exactly squash, should_remove_source_branch, merge_commit_message", body)
	}
	if !strings.Contains(text, "удаление ветки-источника: запрошено") {
		t.Errorf("text %q lacks the removal request line", text)
	}
}

func TestMergeMergeRequestFalseAndBlankAreNotSent(t *testing.T) {
	fake := newMergeFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{
		"project": "g/p", "iid": 5, "squash": false, "remove_source_branch": false, "merge_commit_message": "   ",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if body := mergeBody(t, fake); len(body) != 0 {
		t.Errorf("merge body = %v, want no keys", body)
	}
}

func TestMergeSuccessTextCommitLine(t *testing.T) {
	cases := []struct {
		name, extra, want string
	}{
		{"merge commit", `"merge_commit_sha":"m1"`, "merge commit: m1"},
		{"squash fallback", `"merge_commit_sha":"","squash_commit_sha":"s1"`, "squash commit: s1"},
		{"head fallback", `"merge_commit_sha":"","squash_commit_sha":"","sha":"h1"`, "head sha (fast-forward): h1"},
		{"nothing", `"merge_commit_sha":""`, "merge commit: -"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newMergeFake(t)
			fake.JSON("PUT", mr5MergePath, 200, mergedMRJSON(tc.extra), nil)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 5})
			if isErr || !strings.Contains(text, tc.want) {
				t.Fatalf("isErr=%v text=%q, want %q", isErr, text, tc.want)
			}
		})
	}
}

func TestMergeMergeRequestNotMergedYet(t *testing.T) {
	fake := newMergeFake(t)
	fake.JSON("PUT", mr5MergePath, 200, mrJSON("mergeable", `"merge_commit_sha":""`), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "state=opened") || !strings.Contains(text, "слияние ещё не завершено") ||
		!strings.Contains(text, "get_merge_request") || strings.Contains(text, "влит (state=merged)") {
		t.Errorf("text = %q", text)
	}
}

func TestMergeMergeRequestRefusedBeforePut(t *testing.T) {
	cases := []struct {
		name  string
		get   string
		wants []string
	}{
		{"ci running", mrJSON("ci_still_running", ""),
			[]string{"detailed_merge_status: ci_still_running", "pipeline ещё выполняется"}},
		{"checking", mrJSON("checking", ""), []string{"проверка ещё идёт"}},
		{"draft", mrJSON("draft_status", ""), []string{"update_merge_request", "Draft:"}},
		{"merged", strings.Replace(mrJSON("not_open", ""), `"state":"opened"`, `"state":"merged"`, 1),
			[]string{"MR уже влит (state=merged)"}},
		{"closed", strings.Replace(mrJSON("not_open", ""), `"state":"opened"`, `"state":"closed"`, 1),
			[]string{"MR закрыт", "state_event=reopen"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newMergeFake(t)
			fake.JSON("GET", mr5Path, 200, tc.get, nil)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 5})
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			for _, w := range tc.wants {
				if !strings.Contains(text, w) {
					t.Errorf("text %q lacks %q", text, w)
				}
			}
			if reqs := fake.Requests(); len(reqs) != 1 || len(requestsTo(fake, "PUT ")) != 0 {
				t.Errorf("requests = %v, want exactly the GET and no PUT", reqs)
			}
		})
	}
}

func TestMergeMergeRequestNotFound(t *testing.T) {
	fake := newMergeFake(t)
	fake.JSON("GET", mr5Path, 404, `{"message":"404 Not found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if !isErr || !strings.Contains(text, "не найдено (MR (iid) или проект)") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if len(requestsTo(fake, "PUT ")) != 0 {
		t.Errorf("no PUT expected, got %v", fake.Requests())
	}
}

func TestMergeMergeRequestZeroIID(t *testing.T) {
	fake := newMergeFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "merge_merge_request", map[string]any{"project": "g/p", "iid": 0})
	if !isErr || !strings.Contains(text, "не указан iid MR") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request expected, got %v", reqs)
	}
}
