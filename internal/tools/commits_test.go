package tools

import (
	"net/url"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const commitsProjectJSON = `{"id":1,"path_with_namespace":"g/p","default_branch":"main"}`

const commitsListJSON = `[
	{"id":"a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"a1b2c3d4","title":"Fix login","author_name":"Иван Петров","committed_date":"2026-09-20T10:00:00Z"},
	{"id":"b1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0","short_id":"b1b2c3d4","title":"Add tests","author_name":"Anna","committed_date":"2026-09-19T08:00:00Z"}
]`

func queryOf(t *testing.T, req string) url.Values {
	t.Helper()
	i := strings.Index(req, "?")
	if i < 0 {
		t.Fatalf("request %q has no query", req)
	}
	vals, err := url.ParseQuery(req[i+1:])
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	return vals
}

func TestListCommitsDefaultBranch(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, commitsProjectJSON, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits", 200, commitsListJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_commits", map[string]any{"project": "g/p"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	if len(reqs) != 2 || reqs[0] != "GET /api/v4/projects/g%2Fp" {
		t.Fatalf("requests = %v, want project then commits", reqs)
	}
	if !strings.HasPrefix(reqs[1], "GET /api/v4/projects/g%2Fp/repository/commits?") {
		t.Fatalf("second request = %q", reqs[1])
	}
	vals := queryOf(t, reqs[1])
	for k, want := range map[string]string{"ref_name": "main", "page": "1", "per_page": "20"} {
		if vals.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, vals.Get(k), want)
		}
	}
	for _, k := range []string{"since", "until", "path", "author"} {
		if _, has := vals[k]; has {
			t.Errorf("query must not carry %s: %v", k, vals)
		}
	}

	lines := strings.Split(text, "\n")
	want := []string{
		"коммиты g/p @ main (ветка по умолчанию)",
		"a1b2c3d4 2026-09-20 Иван Петров Fix login",
		"b1b2c3d4 2026-09-19 Anna Add tests",
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

func TestListCommitsFilters(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits", 200, commitsListJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_commits", map[string]any{
		"project": "g/p", "ref": "feature/x", "path": "src/main.go", "author": "ivan",
		"since": "2026-09-01", "until": "2026-09-24T12:00:00Z",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly one (no project GET for an explicit ref)", reqs)
	}
	vals := queryOf(t, reqs[0])
	for k, want := range map[string]string{
		"ref_name": "feature/x", "path": "src/main.go", "author": "ivan",
		"since": "2026-09-01T00:00:00Z", "until": "2026-09-24T12:00:00Z",
	} {
		if vals.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, vals.Get(k), want)
		}
	}
	header := strings.Split(text, "\n")[0]
	if header != "коммиты g/p @ feature/x, путь: src/main.go" {
		t.Errorf("header = %q", header)
	}
}

func TestListCommitsInvalidDate(t *testing.T) {
	for _, field := range []string{"since", "until"} {
		t.Run(field, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "list_commits", map[string]any{"project": "g/p", field: "вчера"})
			if !isErr {
				t.Fatalf("expected isError, got %q", text)
			}
			if !strings.Contains(text, field) || !strings.Contains(text, "ISO 8601") {
				t.Errorf("text = %q, want the field name and ISO 8601", text)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("no request must reach GitLab, got %v", reqs)
			}
		})
	}
}

func TestListCommitsRejectsDotDot(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_commits", map[string]any{"project": "g/p", "ref": "main", "path": "../x"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "..") {
		t.Errorf("text = %q, want a mention of '..'", text)
	}
	if reqs := requestsTo(fake, "GET /api/v4/projects/g%2Fp/repository/commits"); len(reqs) != 0 {
		t.Errorf("commits must not be requested: %v", reqs)
	}
}

func TestListCommitsEmptyAndNextPage(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits", 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_commits", map[string]any{"project": "g/p", "ref": "main"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "коммиты не найдены") || !strings.HasPrefix(text, "коммиты g/p @ main") {
		t.Errorf("text = %q, want header and 'коммиты не найдены'", text)
	}

	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits", 200, commitsListJSON, map[string]string{"X-Next-Page": "2"})
	text, isErr = callText(t, cs, "list_commits", map[string]any{"project": "g/p", "ref": "main"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.HasSuffix(text, "вызовите с page=2]") {
		t.Errorf("text must end with the next-page footer: %q", text)
	}
}

func TestListCommitsNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_commits", map[string]any{"project": "g/p", "ref": "nope"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	if !strings.Contains(text, "404") || !strings.Contains(text, "ref или путь") {
		t.Errorf("text = %q, want 404 and 'ref или путь'", text)
	}
}

func TestParseTime(t *testing.T) {
	for _, in := range []string{"2026-09-01", "2026-09-01T12:00:00", "2026-09-01T12:00:00Z", "2026-09-01T12:00:00+03:00"} {
		got, err := parseTime("since", in)
		if err != nil || got == nil {
			t.Errorf("parseTime(%q) = %v, %v; want a time", in, got, err)
		}
	}
	if got, err := parseTime("since", "  "); got != nil || err != nil {
		t.Errorf("parseTime(blank) = %v, %v; want nil, nil", got, err)
	}
	if _, err := parseTime("until", "01.09.2026"); err == nil || !strings.Contains(err.Error(), "until") {
		t.Errorf("parseTime(bad) error = %v, want one naming until", err)
	}
}
