package tools

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

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

const (
	commitSHA    = "a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0"
	commitParent = "9f8e7d6c5b4a39281706f5e4d3c2b1a099887766"
	commitRoute  = "/api/v4/projects/g%2Fp/repository/commits/a1b2c3d4"
)

func commitJSON(message string) string {
	msg, _ := json.Marshal(message)
	return `{"id":"` + commitSHA + `","short_id":"a1b2c3d4","title":"Fix login","author_name":"Иван Петров",` +
		`"authored_date":"2026-09-20T10:00:00Z","committed_date":"2026-09-20T10:00:00Z","message":` + string(msg) + `,` +
		`"parent_ids":["` + commitParent + `"],"stats":{"additions":12,"deletions":3,"total":15},"web_url":"https://gitlab.com/g/p/-/commit/` + commitSHA + `"}`
}

func TestGetCommit(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", commitRoute, 200, commitJSON("Fix login\n\nLong body"), nil)
	fake.JSON("GET", commitRoute+"/diff", 200, `[
		{"diff":"@@ -1 +1 @@\n-a\n+b\n","new_path":"a.go","old_path":"a.go","a_mode":"100644","b_mode":"100644"},
		{"diff":"","new_path":"big.bin","old_path":"big.bin","a_mode":"100644","b_mode":"100644","too_large":true}
	]`, map[string]string{"X-Next-Page": "2"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_commit", map[string]any{"project": "g/p", "sha": "a1b2c3d4"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	want := []string{"GET " + commitRoute, "GET " + commitRoute + "/diff?page=1&per_page=20"}
	if len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}
	for _, w := range []string{
		"коммит " + commitSHA,
		"автор: Иван Петров, дата: 2026-09-20T10:00:00Z",
		"родители: 9f8e7d6c",
		"изменения: +12/−3",
		"Fix login\n\nLong body",
		"https://gitlab.com/g/p/-/commit/" + commitSHA,
		"файлов на странице: 2",
		"### a.go [modified]\n@@ -1 +1 @@\n-a\n+b",
		"### big.bin [modified] — патч не показан: GitLab пометил файл как too_large; содержимое: get_file_contents path=big.bin ref=" + commitSHA,
	} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
	if !strings.HasSuffix(text, "вызовите с page=2]") {
		t.Errorf("text must end with the next-page footer: %q", text[len(text)-80:])
	}
	if strings.Index(text, "файлов на странице") > strings.Index(text, "### a.go") {
		t.Errorf("the file count must precede the diff")
	}
}

func TestGetCommitLongMessageIsCut(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", commitRoute, 200, commitJSON("Title\n"+strings.Repeat("m", 3000)), nil)
	fake.JSON("GET", commitRoute+"/diff", 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_commit", map[string]any{"project": "g/p", "sha": "a1b2c3d4"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "[сообщение обрезано]") {
		t.Errorf("text lacks the message cut marker:\n%s", text)
	}
	if n := utf8.RuneCountInString(text); n > 1500 {
		t.Errorf("text has %d runes, the message must be capped near 1000", n)
	}
	if !strings.Contains(text, "изменений в файлах нет") {
		t.Errorf("text lacks the empty diff note:\n%s", text)
	}
}

func TestGetCommitBudgetedDiff(t *testing.T) {
	var files []string
	for i := 0; i < 30; i++ {
		files = append(files, fmt.Sprintf(`{"diff":%q,"new_path":"dir/file%02d.go","old_path":"dir/file%02d.go"}`, patchOf(1900), i, i))
	}
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", commitRoute, 200, commitJSON("msg"), nil)
	fake.JSON("GET", commitRoute+"/diff", 200, "["+strings.Join(files, ",")+"]", nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_commit", map[string]any{"project": "g/p", "sha": "a1b2c3d4", "per_page": 30})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if n := utf8.RuneCountInString(text); n >= 15500 {
		t.Errorf("text has %d runes, want < 15500", n)
	}
	if strings.Contains(text, TruncatedFooter) {
		t.Errorf("the renderer must fit the budget without the final cut")
	}
	for i := 0; i < 30; i++ {
		if !strings.Contains(text, fmt.Sprintf("### dir/file%02d.go [modified]", i)) {
			t.Errorf("heading of file %d is missing", i)
		}
	}
	if !strings.Contains(text, "патч не показан (бюджет вывода исчерпан)") {
		t.Errorf("late files must say their patch is not shown")
	}
}

func TestGetCommitErrors(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_commit", map[string]any{"project": "g/p", "sha": "  "})
	if !isErr || !strings.Contains(text, "не указан sha") {
		t.Errorf("empty sha: isError=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request must reach GitLab, got %v", reqs)
	}

	text, isErr = callText(t, cs, "get_commit", map[string]any{"project": "g/p", "sha": "deadbeef"})
	if !isErr || !strings.Contains(text, "404") || !strings.Contains(text, "коммит") {
		t.Errorf("missing commit: isError=%v text=%q", isErr, text)
	}
}
