package tools

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const (
	projectRoute = "/api/v4/projects/g%2Fp"

	compareOneCommit = `{"commits":[{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Add x"}],"diffs":[],"compare_timeout":false}`
	createdMRJSON    = `{"iid":7,"state":"opened","draft":false,"title":"Add x","source_branch":"feature/x","target_branch":"main","detailed_merge_status":"checking","web_url":"https://gitlab.example/g/p/-/merge_requests/7"}`
)

// newCreateMRFake answers the compare pre-check with one commit and the create
// POST with a fresh MR.
func newCreateMRFake(t testing.TB) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", compareRoute, 200, compareOneCommit, nil)
	fake.JSON("POST", mrsPath, 201, createdMRJSON, nil)
	return fake
}

// postBody returns the decoded JSON body of the only POST the fake saw.
func postBody(t testing.TB, fake *testutil.FakeGitLab) map[string]any {
	t.Helper()
	var bodies []string
	for _, r := range fake.Recorded() {
		if r.Method == "POST" {
			bodies = append(bodies, r.Body)
		}
	}
	if len(bodies) != 1 {
		t.Fatalf("POST count = %d, want exactly 1", len(bodies))
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &body); err != nil {
		t.Fatalf("body %q: %v", bodies[0], err)
	}
	return body
}

func countPosts(fake *testutil.FakeGitLab) int {
	return len(requestsTo(fake, "POST "))
}

func TestCreateMergeRequestExplicitTarget(t *testing.T) {
	fake := newCreateMRFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": "Add x", "target_branch": "main",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %v, want compare then POST", reqs)
	}
	if !strings.HasPrefix(reqs[0], "GET "+compareRoute+"?") {
		t.Fatalf("first request = %q", reqs[0])
	}
	q, err := url.ParseQuery(reqs[0][strings.Index(reqs[0], "?")+1:])
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("from") != "main" || q.Get("to") != "feature/x" {
		t.Errorf("compare query = %v, want from=main to=feature/x", q)
	}
	if reqs[1] != "POST "+mrsPath {
		t.Errorf("second request = %q", reqs[1])
	}

	body := postBody(t, fake)
	if len(body) != 3 || body["source_branch"] != "feature/x" || body["target_branch"] != "main" || body["title"] != "Add x" {
		t.Errorf("POST body = %v, want exactly source_branch, target_branch, title", body)
	}
	if _, ok := body["description"]; ok {
		t.Error("description must not be sent when not passed")
	}
	for _, want := range []string{
		"MR !7 создан: feature/x→main",
		"https://gitlab.example/g/p/-/merge_requests/7",
		"detailed_merge_status: checking — ",
		"перед merge_merge_request вызовите get_merge_request",
		"draft: нет",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
}

func TestCreateMergeRequestDefaultTarget(t *testing.T) {
	fake := newCreateMRFake(t)
	fake.JSON("GET", projectRoute, 200, treeProjectJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": "Add x",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 3 || reqs[0] != "GET "+projectRoute {
		t.Fatalf("requests = %v, want project GET first", reqs)
	}
	if body := postBody(t, fake); body["target_branch"] != "main" {
		t.Errorf("target_branch = %v, want main", body["target_branch"])
	}
	if !strings.Contains(text, "feature/x→main (ветка по умолчанию)") {
		t.Errorf("text = %q", text)
	}
}

func TestCreateMergeRequestDraftPrefix(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		{"plain", "Add x", "Draft: Add x"},
		{"lowercase prefix", "draft: Add x", "draft: Add x"},
		{"bracket prefix", "[Draft] Add x", "[Draft] Add x"},
		{"paren prefix", "(draft) Add x", "(draft) Add x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newCreateMRFake(t)
			fake.JSON("POST", mrsPath, 201, strings.Replace(createdMRJSON, `"draft":false`, `"draft":true`, 1), nil)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "create_merge_request", map[string]any{
				"project": "g/p", "source_branch": "feature/x", "title": tc.title,
				"target_branch": "main", "draft": true,
			})
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			if got := postBody(t, fake)["title"]; got != tc.want {
				t.Errorf("title = %v, want %q", got, tc.want)
			}
			if !strings.Contains(text, "draft: да") {
				t.Errorf("text = %q, want draft taken from the response", text)
			}
		})
	}
}

func TestCreateMergeRequestNoDraftKeepsTitle(t *testing.T) {
	fake := newCreateMRFake(t)
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": " Add x ", "target_branch": "main",
	}); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	body := postBody(t, fake)
	if body["title"] != "Add x" {
		t.Errorf("title = %v", body["title"])
	}
	if _, ok := body["draft"]; ok {
		t.Error("no draft key may be sent")
	}
}

func TestCreateMergeRequestSendsDescription(t *testing.T) {
	fake := newCreateMRFake(t)
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": "Add x",
		"target_branch": "main", "description": "desc",
	}); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if got := postBody(t, fake)["description"]; got != "desc" {
		t.Errorf("description = %v", got)
	}
}

func TestCreateMergeRequestGuardsSendNothing(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"blank source", map[string]any{"project": "g/p", "source_branch": "  ", "title": "t"},
			"не указана ветка-источник (source_branch)"},
		{"blank title", map[string]any{"project": "g/p", "source_branch": "feature/x", "title": ""},
			"не указан заголовок MR (title)"},
		{"source equals explicit target", map[string]any{"project": "g/p", "source_branch": "main", "title": "t", "target_branch": "main"},
			"ветка-источник совпадает с целевой"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newCreateMRFake(t)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "create_merge_request", tc.args)
			if !isErr || !strings.Contains(text, tc.want) {
				t.Fatalf("isErr=%v text=%q, want error containing %q", isErr, text, tc.want)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("no request expected, got %v", reqs)
			}
		})
	}
}

func TestCreateMergeRequestSourceEqualsDefaultTarget(t *testing.T) {
	fake := newCreateMRFake(t)
	fake.JSON("GET", projectRoute, 200, treeProjectJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "main", "title": "t",
	})
	if !isErr || !strings.Contains(text, "ветка-источник совпадает с целевой") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if countPosts(fake) != 0 {
		t.Error("no POST expected")
	}
}

func TestCreateMergeRequestNoChangesBlocksPost(t *testing.T) {
	fake := newCreateMRFake(t)
	fake.JSON("GET", compareRoute, 200, `{"commits":[],"compare_timeout":false}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": "Add x", "target_branch": "main",
	})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	for _, want := range []string{"нет изменений между ветками", "commit_files"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
	if n := countPosts(fake); n != 0 {
		t.Errorf("POST was sent %d times, want 0", n)
	}
}

func TestCreateMergeRequestCompareTimeoutStillPosts(t *testing.T) {
	fake := newCreateMRFake(t)
	fake.JSON("GET", compareRoute, 200, `{"commits":[],"compare_timeout":true}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "feature/x", "title": "Add x", "target_branch": "main",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if n := countPosts(fake); n != 1 {
		t.Errorf("POST count = %d, want 1", n)
	}
}

func TestCreateMergeRequestCompare404NamesBranchOrProject(t *testing.T) {
	fake := newCreateMRFake(t)
	fake.JSON("GET", compareRoute, 404, `{"message":"404 Not Found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request", map[string]any{
		"project": "g/p", "source_branch": "nope", "title": "Add x", "target_branch": "main",
	})
	if !isErr || !strings.Contains(text, "не найдено (ветка или проект)") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if countPosts(fake) != 0 {
		t.Error("no POST expected")
	}
}

func TestWithDraftPrefix(t *testing.T) {
	cases := map[string]string{
		"Add x":         "Draft: Add x",
		"Draft: Add x":  "Draft: Add x",
		"  draft: x":    "  draft: x",
		"[DRAFT] x":     "[DRAFT] x",
		"(Draft) x":     "(Draft) x",
		"Drafting docs": "Draft: Drafting docs",
	}
	for in, want := range cases {
		if got := withDraftPrefix(in); got != want {
			t.Errorf("withDraftPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
