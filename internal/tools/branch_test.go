package tools

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const (
	branchesPath = "/api/v4/projects/g%2Fp/repository/branches"
	createdJSON  = `{"name":"feature/x","commit":{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Init"},"web_url":"https://gitlab.example/g/p/-/tree/feature/x"}`
)

func TestListBranchesRendersMarkers(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", branchesPath, 200, `[
		{"name":"main","default":true,"protected":true,"merged":false,"commit":{"id":"a1b2c3d4e5","short_id":"a1b2c3d4","title":"Init"}},
		{"name":"feature/x","merged":true,"commit":{"id":"9f8e7d6c11","short_id":"9f8e7d6c","title":"Add x\n\nlong body"}},
		{"name":"orphan","commit":null}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_branches", map[string]any{"project": "g/p"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET "+branchesPath+"?page=1&per_page=20" {
		t.Fatalf("requests = %v", reqs)
	}
	want := []string{
		"ветки g/p",
		"main a1b2c3d4 [default] [protected] Init",
		"feature/x 9f8e7d6c [merged] Add x",
		"orphan -",
		"[page 1, per_page 20 — последняя страница]",
	}
	lines := strings.Split(text, "\n")
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), text)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestListBranchesSearchQuery(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", branchesPath, 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_branches", map[string]any{"project": "g/p", "search": "feat"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "ветки не найдены") || !strings.Contains(text, "поиск: feat") {
		t.Errorf("text = %q", text)
	}
	reqs := fake.Requests()
	vals, err := url.ParseQuery(reqs[0][strings.Index(reqs[0], "?")+1:])
	if err != nil {
		t.Fatal(err)
	}
	if vals.Get("search") != "feat" {
		t.Errorf("search = %q", vals.Get("search"))
	}

	fake.Reset()
	if _, isErr := callText(t, cs, "list_branches", map[string]any{"project": "g/p", "search": "  "}); isErr {
		t.Fatal("unexpected tool error")
	}
	if strings.Contains(fake.Requests()[0], "search") {
		t.Errorf("blank search must not be sent: %v", fake.Requests())
	}
}

func TestListBranchesNextPageFooter(t *testing.T) {
	t.Run("X-Next-Page", func(t *testing.T) {
		fake := testutil.NewFakeGitLab(t)
		fake.JSON("GET", branchesPath, 200, `[{"name":"main","commit":{"short_id":"a1b2c3d4","title":"Init"}}]`,
			map[string]string{"X-Next-Page": "2"})
		cs := newTestSession(t, fake)
		text, _ := callText(t, cs, "list_branches", map[string]any{"project": "g/p"})
		if !strings.HasSuffix(text, "есть следующая страница: вызовите с page=2]") {
			t.Errorf("text = %q", text)
		}
	})
	t.Run("Link only", func(t *testing.T) {
		fake := testutil.NewFakeGitLab(t)
		link := "<" + fake.URL + "/api/v4/projects/g%2Fp/repository/branches?page=2&per_page=20>; rel=\"next\""
		fake.JSON("GET", branchesPath, 200, `[{"name":"main","commit":{"short_id":"a1b2c3d4","title":"Init"}}]`,
			map[string]string{"Link": link})
		cs := newTestSession(t, fake)
		text, _ := callText(t, cs, "list_branches", map[string]any{"project": "g/p"})
		if !strings.Contains(text, "есть следующая страница") {
			t.Errorf("Link-only answer must still announce a next page: %q", text)
		}
	})
}

func TestCreateBranchExplicitRef(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", branchesPath, 201, createdJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/x", "ref": "main"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	rec := fake.Recorded()
	if len(rec) != 1 || rec[0].Method != "POST" || rec[0].URI != branchesPath {
		t.Fatalf("recorded = %+v, want exactly one POST %s", rec, branchesPath)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(rec[0].Body), &body); err != nil {
		t.Fatalf("body %q: %v", rec[0].Body, err)
	}
	if len(body) != 2 || body["branch"] != "feature/x" || body["ref"] != "main" {
		t.Errorf("body = %v", body)
	}
	for _, want := range []string{"feature/x", "main", "a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0", "https://gitlab.example/g/p/-/tree/feature/x"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q lacks %q", text, want)
		}
	}
}

func TestCreateBranchDefaultRef(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, treeProjectJSON, nil)
	fake.JSON("POST", branchesPath, 201, createdJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/x"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	rec := fake.Recorded()
	if len(rec) != 2 || rec[0].Method != "GET" || rec[1].Method != "POST" {
		t.Fatalf("recorded = %+v, want GET project then POST", rec)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(rec[1].Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["ref"] != "main" {
		t.Errorf("ref = %q, want main", body["ref"])
	}
	if !strings.Contains(text, "(ветка по умолчанию)") {
		t.Errorf("text = %q", text)
	}
}

func TestCreateBranchBlankName(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "  "})
	if !isErr || !strings.Contains(text, "не указано имя ветки") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request expected, got %v", reqs)
	}
}

func TestCreateBranchNotFoundNamesProjectOrBranch(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", branchesPath, 404, `{"message":"404 Project Not Found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/x", "ref": "main"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "не найдено (проект или ветка)") {
		t.Errorf("text = %q", text)
	}
}

// TestCreateThenListShowsNewBranch covers the create-then-list flow against a
// fake that keeps state.
func TestCreateThenListShowsNewBranch(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	var mu sync.Mutex
	branches := []string{`{"name":"main","default":true,"commit":{"short_id":"a1b2c3d4","title":"Init"}}`}
	fake.Handle("GET", branchesPath, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + strings.Join(branches, ",") + "]"))
	})
	fake.Handle("POST", branchesPath, func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Branch string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		branches = append(branches, `{"name":"`+in.Branch+`","commit":{"short_id":"a1b2c3d4","title":"Init"}}`)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"name":"` + in.Branch + `","commit":{"id":"a1b2c3d4e5","short_id":"a1b2c3d4","title":"Init"},"web_url":"https://gitlab.example/x"}`))
	})
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/new", "ref": "main"}); isErr {
		t.Fatalf("create_branch: %s", text)
	}
	text, isErr := callText(t, cs, "list_branches", map[string]any{"project": "g/p"})
	if isErr {
		t.Fatalf("list_branches: %s", text)
	}
	if !strings.Contains(text, "feature/new") {
		t.Errorf("list lacks the created branch:\n%s", text)
	}
}

func TestFakeRecordedAndReset(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", "/x", 200, `{}`, nil)
	resp, err := http.Post(fake.URL+"/x?a=b", "application/json", strings.NewReader(`{"k":1}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	rec := fake.Recorded()
	if len(rec) != 1 || rec[0].Method != "POST" || rec[0].URI != "/x?a=b" || rec[0].Body != `{"k":1}` {
		t.Fatalf("recorded = %+v", rec)
	}
	if reqs := fake.Requests(); len(reqs) != 1 || reqs[0] != "POST /x?a=b" {
		t.Errorf("requests = %v", reqs)
	}
	fake.Reset()
	if len(fake.Recorded()) != 0 || len(fake.Requests()) != 0 {
		t.Error("Reset must clear both")
	}
}
