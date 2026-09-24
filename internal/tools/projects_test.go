package tools

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

func requestsTo(fake *testutil.FakeGitLab, prefix string) []string {
	var out []string
	for _, r := range fake.Requests() {
		if strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func TestListProjectsDefaultsAndNextPage(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 200, `[
		{"id":101,"path_with_namespace":"group/alpha","default_branch":"main","description":"Alpha project"},
		{"id":102,"path_with_namespace":"group/sub/beta","default_branch":"","description":""}
	]`, map[string]string{"X-Next-Page": "2"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_projects", map[string]any{})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := requestsTo(fake, "GET /api/v4/projects")
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly one", reqs)
	}
	q := reqs[0][strings.Index(reqs[0], "?")+1:]
	vals, err := url.ParseQuery(q)
	if err != nil {
		t.Fatalf("parse query %q: %v", q, err)
	}
	for k, want := range map[string]string{
		"membership": "true", "simple": "true", "page": "1", "per_page": "20", "order_by": "last_activity_at",
	} {
		if vals.Get(k) != want {
			t.Errorf("query %s = %q, want %q (full query %q)", k, vals.Get(k), want, q)
		}
	}

	lines := strings.Split(text, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), text)
	}
	if lines[0] != "101 group/alpha (main) — Alpha project" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != "102 group/sub/beta" {
		t.Errorf("line 1 = %q", lines[1])
	}
	if lines[2] != "[page 1, per_page 20 — есть следующая страница: вызовите с page=2]" {
		t.Errorf("footer = %q", lines[2])
	}
}

func TestListProjectsLastPage(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 200, `[{"id":1,"path_with_namespace":"g/p","default_branch":"main"}]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_projects", map[string]any{})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.HasSuffix(text, "[page 1, per_page 20 — последняя страница]") {
		t.Errorf("text = %q, want last-page footer", text)
	}
}

func TestListProjectsIncludePublicSearchAndClamp(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_projects", map[string]any{
		"include_public": true, "search": "alp", "page": 3, "per_page": 500,
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if text != "проекты не найдены" {
		t.Errorf("empty result text = %q", text)
	}

	reqs := requestsTo(fake, "GET /api/v4/projects")
	if len(reqs) != 1 {
		t.Fatalf("requests = %v", reqs)
	}
	q := reqs[0][strings.Index(reqs[0], "?")+1:]
	vals, _ := url.ParseQuery(q)
	if _, has := vals["membership"]; has {
		t.Errorf("membership must be absent with include_public, query %q", q)
	}
	for k, want := range map[string]string{"search": "alp", "page": "3", "per_page": "100"} {
		if vals.Get(k) != want {
			t.Errorf("query %s = %q, want %q (full query %q)", k, vals.Get(k), want, q)
		}
	}
}

func TestListProjectsTruncatesLargeOutput(t *testing.T) {
	var items []string
	path := strings.Repeat("p", 200)
	desc := strings.Repeat("d", 400)
	for i := 0; i < 100; i++ {
		items = append(items, fmt.Sprintf(`{"id":%d,"path_with_namespace":%q,"default_branch":"main","description":%q}`, 1000+i, path, desc))
	}
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 200, "["+strings.Join(items, ",")+"]", map[string]string{"X-Next-Page": "2"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_projects", map[string]any{"per_page": 100})
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
		t.Errorf("text must still end with the page footer, got tail %q", text[len(text)-80:])
	}
	for _, l := range strings.Split(text, "\n") {
		// Every full project line ends with the shortened description marker.
		if !strings.HasPrefix(l, "[") && !strings.HasSuffix(l, "…") {
			t.Errorf("partial project line: %q", l)
		}
	}
}

const projectJSON = `{"id":7,"path_with_namespace":"group/sub/my.proj","default_branch":"main","visibility":"private",` +
	`"archived":false,"web_url":"https://gitlab.com/group/sub/my.proj","last_activity_at":"2026-09-01T10:00:00Z","description":"My project"}`

func TestGetProjectByPath(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/group%2Fsub%2Fmy%2Eproj", 200, projectJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_project", map[string]any{"project": "group/sub/my.proj"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	for _, want := range []string{
		"id: 7", "path: group/sub/my.proj", "default_branch: main", "visibility: private",
		"archived: no", "last_activity: 2026-09-01", "web_url: https://gitlab.com/group/sub/my.proj",
		"description: My project",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET /api/v4/projects/group%2Fsub%2Fmy%2Eproj" {
		t.Errorf("requests = %v, want the encoded path /api/v4/projects/group%%2Fsub%%2Fmy%%2Eproj", reqs)
	}
}

func TestGetProjectByNumericID(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/123", 200, projectJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_project", map[string]any{"project": "123"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET /api/v4/projects/123" {
		t.Errorf("requests = %v", reqs)
	}
}

func TestGetProjectEmptyRepository(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/9", 200,
		`{"id":9,"path_with_namespace":"g/empty","default_branch":"","empty_repo":true,"visibility":"public","web_url":"https://gitlab.com/g/empty"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_project", map[string]any{"project": "9"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "default_branch: (нет — репозиторий пуст)") {
		t.Errorf("text = %q, want empty-repo default_branch", text)
	}
	if strings.Contains(text, "last_activity:") || strings.Contains(text, "description:") {
		t.Errorf("empty optional fields must be omitted: %q", text)
	}
}

func TestGetProjectRejectsURL(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_project", map[string]any{"project": "https://gitlab.com/g/p"})
	if !isErr {
		t.Fatalf("expected isError for a URL argument, got %q", text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request must reach GitLab, got %v", reqs)
	}
}

func TestGetProjectNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_project", map[string]any{"project": "no/such"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "404") || !strings.Contains(text, "проект") {
		t.Errorf("text = %q, want 404 and 'проект'", text)
	}
}

func TestGetProjectNumberArgumentRejectedBySchema(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/123", 200, projectJSON, nil)
	cs := newTestSession(t, fake)

	if text, isErr := callText(t, cs, "get_project", map[string]any{"project": 123}); !isErr {
		t.Fatalf("expected schema validation error for a JSON number, got %q", text)
	}
	// The session must keep working after the rejected call.
	if text, isErr := callText(t, cs, "get_project", map[string]any{"project": "123"}); isErr {
		t.Fatalf("follow-up call failed: %s", text)
	}
}
