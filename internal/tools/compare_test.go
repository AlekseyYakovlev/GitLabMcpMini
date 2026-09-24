package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/testutil"
)

const compareRoute = "/api/v4/projects/g%2Fp/repository/compare"

// compareJSON builds a compare response with nCommits commits and nDiffs
// modified files named f01.go, f02.go, ...
func compareJSON(nCommits, nDiffs int, extra string) string {
	commits := make([]string, nCommits)
	for i := range commits {
		commits[i] = fmt.Sprintf(`{"id":"%040d","short_id":"%08d","title":"commit %d","author_name":"Anna","committed_date":"2026-09-20T10:00:00Z"}`, i+1, i+1, i+1)
	}
	diffs := make([]string, nDiffs)
	for i := range diffs {
		diffs[i] = fmt.Sprintf(`{"diff":"@@ -1 +1 @@\n-a\n+b%d\n","new_path":"f%02d.go","old_path":"f%02d.go","a_mode":"100644","b_mode":"100644"}`, i+1, i+1, i+1)
	}
	return fmt.Sprintf(`{"commits":[%s],"diffs":[%s]%s}`, strings.Join(commits, ","), strings.Join(diffs, ","), extra)
}

func compareCall(t *testing.T, body string, args map[string]any) (string, bool) {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", compareRoute, 200, body, nil)
	cs := newTestSession(t, fake)
	return callText(t, cs, "compare_refs", args)
}

func TestFetchCompareWireMatchesClientGo(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", compareRoute, 200, `{"commits":[],"diffs":[]}`, nil)
	d := newTestDeps(t, fake)
	ctx := context.Background()

	if _, err := fetchCompare(ctx, d, "g/p", "main", "feature/x"); err != nil {
		t.Fatalf("fetchCompare: %v", err)
	}
	mine := fake.Recorded()
	fake.Reset()

	from, to := "main", "feature/x"
	if _, _, err := d.GL.Repositories.Compare("g/p", &gitlab.CompareOptions{From: &from, To: &to}); err != nil {
		t.Fatalf("Repositories.Compare: %v", err)
	}
	theirs := fake.Recorded()

	if len(mine) != 1 || len(theirs) != 1 {
		t.Fatalf("recorded mine=%v theirs=%v, want one request each", mine, theirs)
	}
	const want = "/api/v4/projects/g%2Fp/repository/compare?from=main&to=feature%2Fx"
	if mine[0].URI != want {
		t.Errorf("fetchCompare RequestURI = %q, want %q", mine[0].URI, want)
	}
	if mine[0].URI != theirs[0].URI {
		t.Errorf("RequestURI differs from client-go: mine %q, theirs %q", mine[0].URI, theirs[0].URI)
	}
}

func TestCompareRefsHeaderCommitsAndFiles(t *testing.T) {
	text, isErr := compareCall(t, compareJSON(2, 3, ""), map[string]any{"project": "g/p", "from": "main", "to": "feature/x"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	for _, want := range []string{
		"сравнение g/p: main → feature/x\n",
		"коммитов: 2\n00000001 2026-09-20 Anna commit 1\n00000002 2026-09-20 Anna commit 2\n",
		"файлов: 3, на странице: 1-3\n",
		"### f01.go [modified]\n@@ -1 +1 @@",
		"### f03.go [modified]\n",
		"+b3",
		"последняя страница",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "показаны первые") {
		t.Errorf("2 commits must not be marked as cut:\n%s", text)
	}
}

func TestCompareRefsCommitCap(t *testing.T) {
	text, _ := compareCall(t, compareJSON(25, 1, ""), map[string]any{"project": "g/p", "from": "a", "to": "b"})
	if got := strings.Count(text, " Anna commit "); got != compareCommitLines {
		t.Errorf("commit lines = %d, want %d:\n%s", got, compareCommitLines, text)
	}
	if !strings.Contains(text, "коммитов: 25") || !strings.Contains(text, "показаны первые 20 из 25") {
		t.Errorf("output lacks the commit cap note:\n%s", text)
	}
}

func TestCompareRefsFilePaging(t *testing.T) {
	body := compareJSON(1, 45, "")
	args := func(page int) map[string]any {
		return map[string]any{"project": "g/p", "from": "a", "to": "b", "page": page, "per_page": 20}
	}

	p1, _ := compareCall(t, body, args(1))
	if !strings.Contains(p1, "файлов: 45, на странице: 1-20") || !strings.Contains(p1, "### f01.go") ||
		!strings.Contains(p1, "### f20.go") || strings.Contains(p1, "### f21.go") {
		t.Errorf("page 1 wrong:\n%s", p1)
	}
	if !strings.Contains(p1, "есть следующая страница: вызовите с page=2") {
		t.Errorf("page 1 lacks the next-page footer:\n%s", p1)
	}

	p3, _ := compareCall(t, body, args(3))
	if !strings.Contains(p3, "файлов: 45, на странице: 41-45") || !strings.Contains(p3, "### f41.go") ||
		!strings.Contains(p3, "### f45.go") || strings.Contains(p3, "### f40.go") {
		t.Errorf("page 3 wrong:\n%s", p3)
	}
	if strings.Contains(p3, "есть следующая страница") || !strings.Contains(p3, "последняя страница") {
		t.Errorf("page 3 must be the last page:\n%s", p3)
	}

	p4, isErr := compareCall(t, body, args(4))
	if isErr {
		t.Fatalf("unexpected tool error: %s", p4)
	}
	if !strings.Contains(p4, "на странице 4 файлов нет: всего файлов 45") {
		t.Errorf("page 4 lacks the out-of-range note:\n%s", p4)
	}
	if strings.Contains(p4, "###") {
		t.Errorf("page 4 must not render files:\n%s", p4)
	}
}

func TestCompareRefsNoFiles(t *testing.T) {
	text, _ := compareCall(t, compareJSON(1, 0, ""), map[string]any{"project": "g/p", "from": "a", "to": "b"})
	if !strings.Contains(text, "файлов: 0 (различий в файлах нет)") {
		t.Errorf("output lacks the empty-files note:\n%s", text)
	}
}

func TestCompareRefsTimeoutWarning(t *testing.T) {
	text, _ := compareCall(t, compareJSON(1, 1, `,"compare_timeout":true`), map[string]any{"project": "g/p", "from": "a", "to": "b"})
	const warn = "предупреждение: GitLab прервал сравнение по таймауту (compare_timeout), diff может быть неполным"
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || lines[0] != "сравнение g/p: a → b" || lines[1] != warn {
		t.Errorf("warning must sit directly under the header:\n%s", text)
	}
}

func TestCompareRefsSameRef(t *testing.T) {
	text, isErr := compareCall(t, `{"commits":[],"diffs":[],"compare_same_ref":true}`, map[string]any{"project": "g/p", "from": "a", "to": "a"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "from и to указывают на один и тот же коммит: различий нет") {
		t.Errorf("output lacks the same-ref sentence:\n%s", text)
	}
}

func TestCompareRefsTooLargeHints(t *testing.T) {
	body := `{"commits":[],"diffs":[
		{"diff":"","new_path":"big.bin","old_path":"big.bin","a_mode":"100644","b_mode":"100644","too_large":true},
		{"diff":"","new_path":"gone.txt","old_path":"gone.txt","a_mode":"100644","b_mode":"000000","deleted_file":true,"too_large":true}
	]}`
	text, _ := compareCall(t, body, map[string]any{"project": "g/p", "from": "main", "to": "feature/x"})
	for _, want := range []string{
		"too_large",
		"get_file_contents path=big.bin ref=feature/x",
		"get_file_contents path=gone.txt ref=main",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

func TestCompareRefsRequiresFromAndTo(t *testing.T) {
	for _, args := range []map[string]any{
		{"project": "g/p", "from": "", "to": "b"},
		{"project": "g/p", "from": "a", "to": "  "},
	} {
		fake := testutil.NewFakeGitLab(t)
		cs := newTestSession(t, fake)
		text, isErr := callText(t, cs, "compare_refs", args)
		if !isErr || !strings.Contains(text, "укажите from и to") {
			t.Errorf("args %v: isError=%v text=%q", args, isErr, text)
		}
		if n := len(fake.Requests()); n != 0 {
			t.Errorf("args %v: %d requests made, want 0", args, n)
		}
	}
}

func TestCompareRefsNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)
	text, isErr := callText(t, cs, "compare_refs", map[string]any{"project": "g/p", "from": "a", "to": "nope"})
	if !isErr || !strings.Contains(text, "404") || !strings.Contains(text, "ref") {
		t.Errorf("isError=%v text=%q, want a 404 error naming the ref", isErr, text)
	}
}

func TestCompareRefsStaysInBudget(t *testing.T) {
	big := strings.Repeat("+"+strings.Repeat("y", 98)+"\\n", 60)
	diffs := make([]string, 30)
	for i := range diffs {
		diffs[i] = fmt.Sprintf(`{"diff":"@@ -1 +1 @@\n%s","new_path":"g%02d.go","old_path":"g%02d.go","a_mode":"100644","b_mode":"100644"}`, big, i+1, i+1)
	}
	body := `{"commits":[],"diffs":[` + strings.Join(diffs, ",") + `]}`
	text, _ := compareCall(t, body, map[string]any{"project": "g/p", "from": "a", "to": "b", "per_page": 30})
	if n := len([]rune(text)); n > OutputBudget+500 {
		t.Errorf("output is %d runes, want within the budget", n)
	}
	for i := 1; i <= 30; i++ {
		if h := fmt.Sprintf("### g%02d.go", i); !strings.Contains(text, h) {
			t.Errorf("output lacks heading %q", h)
		}
	}
}
