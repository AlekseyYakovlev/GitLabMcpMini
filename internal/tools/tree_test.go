package tools

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

const treeProjectJSON = `{"id":1,"path_with_namespace":"g/p","default_branch":"main"}`

func TestListRepositoryTreeDefaultBranch(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, treeProjectJSON, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/tree", 200, `[
		{"id":"a","name":"src","type":"tree","path":"src","mode":"040000"},
		{"id":"b","name":"README.md","type":"blob","path":"README.md","mode":"100644"},
		{"id":"c","name":"lib","type":"commit","path":"vendor/lib","mode":"160000"}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %v, want project then tree", reqs)
	}
	if reqs[0] != "GET /api/v4/projects/g%2Fp" {
		t.Errorf("first request = %q", reqs[0])
	}
	if !strings.HasPrefix(reqs[1], "GET /api/v4/projects/g%2Fp/repository/tree?") {
		t.Fatalf("second request = %q", reqs[1])
	}
	vals, err := url.ParseQuery(reqs[1][strings.Index(reqs[1], "?")+1:])
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	for k, want := range map[string]string{"ref": "main", "page": "1", "per_page": "20"} {
		if vals.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, vals.Get(k), want)
		}
	}
	for _, k := range []string{"path", "recursive"} {
		if _, has := vals[k]; has {
			t.Errorf("query must not carry %s: %v", k, vals)
		}
	}

	lines := strings.Split(text, "\n")
	want := []string{
		"g/p @ main (ветка по умолчанию), путь: /",
		"dir  src/",
		"file README.md",
		"sub  vendor/lib",
		"[page 1, per_page 20 — последняя страница]",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), text)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestListRepositoryTreeExplicitRefWire(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fsub%2Fp/repository/tree", 200, `[
		{"id":"a","name":"x.go","type":"blob","path":"dir with space/ф+#%/x.go","mode":"100644"}
	]`, map[string]string{"X-Next-Page": "3"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{
		"project": "g/sub/p", "path": "dir with space/ф+#%", "ref": "feature/x",
		"recursive": true, "page": 2, "per_page": 20,
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	const wantReq = "GET /api/v4/projects/g%2Fsub%2Fp/repository/tree?page=2&path=dir+with+space%2F%D1%84%2B%23%25&per_page=20&recursive=true&ref=feature%2Fx"
	if len(reqs) != 1 || reqs[0] != wantReq {
		t.Fatalf("requests = %v, want exactly [%s]", reqs, wantReq)
	}

	lines := strings.Split(text, "\n")
	if lines[0] != "g/sub/p @ feature/x, путь: dir with space/ф+#%" {
		t.Errorf("header = %q", lines[0])
	}
	if got := lines[len(lines)-1]; got != "[page 2, per_page 20 — есть следующая страница: вызовите с page=3]" {
		t.Errorf("footer = %q", got)
	}
}

func TestListRepositoryTreeRejectsDotDot(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p", "path": "a/../b"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "..") {
		t.Errorf("text = %q, want a mention of '..'", text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request must reach GitLab, got %v", reqs)
	}
}

func TestListRepositoryTreeEmptyRepo(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"","empty_repo":true}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "репозиторий пуст") {
		t.Errorf("text = %q, want 'репозиторий пуст'", text)
	}
	if reqs := requestsTo(fake, "GET /api/v4/projects/g%2Fp/repository/tree"); len(reqs) != 0 {
		t.Errorf("tree must not be requested for an empty repo: %v", reqs)
	}
}

func TestListRepositoryTreeNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p", "ref": "nope"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "404") || !strings.Contains(text, "путь или ref") {
		t.Errorf("text = %q, want 404 and 'путь или ref'", text)
	}
}

func TestListRepositoryTreeEmptyListing(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/tree", 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p", "ref": "main", "path": "docs"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	lines := strings.Split(text, "\n")
	if lines[0] != "g/p @ main, путь: docs" {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(text, "пусто") {
		t.Errorf("text = %q, want 'пусто'", text)
	}
}

func TestListRepositoryTreeTruncatesLargeOutput(t *testing.T) {
	var items []string
	for i := 0; i < 100; i++ {
		p := fmt.Sprintf("%03d-%s", i, strings.Repeat("d", 196))
		items = append(items, fmt.Sprintf(`{"id":"i%d","name":"n","type":"blob","path":%q,"mode":"100644"}`, i, p))
	}
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/tree", 200, "["+strings.Join(items, ",")+"]", map[string]string{"X-Next-Page": "2"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_repository_tree", map[string]any{
		"project": "g/p", "ref": "main", "recursive": true, "per_page": 100,
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, TruncatedFooter) {
		t.Errorf("text lacks truncation footer")
	}
	if n := utf8.RuneCountInString(text); n >= 15500 {
		t.Errorf("text has %d runes, want < 15500", n)
	}
	if !strings.HasSuffix(text, "page=2]") {
		t.Errorf("text must end with the page footer")
	}
}
