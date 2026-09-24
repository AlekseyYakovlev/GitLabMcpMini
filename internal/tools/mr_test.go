package tools

import (
	"fmt"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const (
	mrsPath = "/api/v4/projects/g%2Fp/merge_requests"
	mr5Path = "/api/v4/projects/g%2Fp/merge_requests/5"

	mrListJSON = `[{"iid":5,"state":"opened","draft":true,"title":"Add x","source_branch":"f","target_branch":"main","author":{"username":"alice"}}]`
)

// mrJSON builds a single MR answer; extra is spliced in before the closing brace.
func mrJSON(status, extra string) string {
	s := `{"iid":5,"title":"Add x","state":"opened","draft":false,"source_branch":"f","target_branch":"main",` +
		`"author":{"username":"alice"},"detailed_merge_status":"` + status + `","has_conflicts":false,` +
		`"web_url":"https://gitlab.example/g/p/-/merge_requests/5"`
	if extra != "" {
		s += "," + extra
	}
	return s + "}"
}

func TestListMergeRequestsDefaultsToOpened(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mrsPath, 200, mrListJSON, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %v, want exactly one", reqs)
	}
	q := queryOf(t, reqs[0])
	if q.Get("state") != "opened" || q.Get("page") != "1" || q.Get("per_page") != "20" {
		t.Errorf("query = %v", q)
	}
	for _, key := range []string{"source_branch", "target_branch", "author_username", "search"} {
		if _, ok := q[key]; ok {
			t.Errorf("%s must not be sent by default: %v", key, q)
		}
	}
	want := []string{
		"MR проекта g/p, state=opened",
		"!5 opened [draft] Add x f→main @alice",
		"[page 1, per_page 20 — последняя страница]",
	}
	if got := strings.Split(text, "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("text = %q, want lines %q", text, want)
	}
}

func TestListMergeRequestsSendsFilters(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mrsPath, 200, `[]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_requests", map[string]any{
		"project": "g/p", "state": "merged", "source_branch": "f", "target_branch": "main",
		"author_username": "alice", "search": "fix",
	})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "MR не найдены") {
		t.Errorf("text = %q, want the empty-list line", text)
	}
	q := queryOf(t, fake.Requests()[0])
	for k, v := range map[string]string{
		"state": "merged", "source_branch": "f", "target_branch": "main",
		"author_username": "alice", "search": "fix",
	} {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}

	fake.Reset()
	if _, isErr := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p", "search": "  ", "source_branch": " "}); isErr {
		t.Fatal("unexpected tool error")
	}
	q = queryOf(t, fake.Requests()[0])
	if _, ok := q["search"]; ok {
		t.Errorf("blank search must not be sent: %v", q)
	}
	if _, ok := q["source_branch"]; ok {
		t.Errorf("blank source_branch must not be sent: %v", q)
	}
}

func TestListMergeRequestsRejectsBadState(t *testing.T) {
	for _, state := range []string{"locked", "foo"} {
		fake := testutil.NewFakeGitLab(t)
		cs := newTestSession(t, fake)
		text, isErr := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p", "state": state})
		if !isErr || !strings.Contains(text, "opened, closed, merged, all") {
			t.Errorf("state %q: isErr=%v text=%q", state, isErr, text)
		}
		if reqs := fake.Requests(); len(reqs) != 0 {
			t.Errorf("state %q: requests = %v, want none", state, reqs)
		}
	}
}

func TestMRLineRendering(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mrsPath, 200, `[
		{"iid":6,"state":"merged","draft":false,"title":"Plain\nsecond line","source_branch":"a","target_branch":"b","author":null},
		{"iid":7,"state":"closed","title":"No draft key","source_branch":"c","target_branch":"d","author":{"username":"bob"}}
	]`, nil)
	cs := newTestSession(t, fake)

	text, _ := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p", "state": "all"})
	for _, want := range []string{"!6 merged Plain a→b @-", "!7 closed No draft key c→d @bob"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, missing %q", text, want)
		}
	}
	if strings.Contains(text, "[draft]") {
		t.Errorf("non-draft MRs must not be marked: %q", text)
	}
}

func TestGetMergeRequestHeader(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"head_pipeline":{"status":"success"},"changes_count":"2","description":"Body text"`), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if reqs := fake.Requests(); len(reqs) != 1 || reqs[0] != "GET "+mr5Path {
		t.Fatalf("requests = %v", reqs)
	}
	for _, want := range []string{
		"!5 Add x", "state: opened", "f→main", "@alice", "detailed_merge_status: mergeable — ",
		"pipeline: success", "файлов: 2", "признак конфликтов: нет", "описание:", "Body text",
		"https://gitlab.example/g/p/-/merge_requests/5",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, missing %q", text, want)
		}
	}
}

func TestGetMergeRequestCheckingIsOneRequest(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("checking", `"head_pipeline":null,"changes_count":""`), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "повторите get_merge_request через несколько секунд") {
		t.Errorf("text = %q, want the repeat hint", text)
	}
	for _, want := range []string{"число файлов: ещё не известно", "pipeline: нет", "(ещё не проверено)"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, missing %q", text, want)
		}
	}
	if reqs := fake.Requests(); len(reqs) != 1 {
		t.Errorf("requests = %v, want exactly one (no polling)", reqs)
	}
}

func TestGetMergeRequestUnknownStatusAndMergedState(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("brand_new_value", ""), nil)
	cs := newTestSession(t, fake)
	text, _ := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if !strings.Contains(text, "detailed_merge_status: brand_new_value — "+unknownStatusAdvice) {
		t.Errorf("text = %q", text)
	}

	fake2 := testutil.NewFakeGitLab(t)
	fake2.JSON("GET", mr5Path, 200, strings.Replace(mrJSON("not_open", ""), `"state":"opened"`, `"state":"merged"`, 1), nil)
	cs2 := newTestSession(t, fake2)
	text, _ = callText(t, cs2, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if !strings.Contains(text, "MR уже влит (state=merged)") {
		t.Errorf("text = %q, want the merged-state explanation", text)
	}
}

func TestGetMergeRequestDescriptionCut(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"description":"`+strings.Repeat("я", 2000)+`"`), nil)
	cs := newTestSession(t, fake)

	text, _ := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if !strings.Contains(text, "[описание обрезано]") {
		t.Errorf("text = %q, want the cut marker", text)
	}
	if strings.Contains(text, strings.Repeat("я", 1600)) {
		t.Errorf("description was not cut at %d runes", mrDescriptionRunes)
	}
}

func TestGetMergeRequestRejectsBadIID(t *testing.T) {
	for _, iid := range []int{0, -3} {
		fake := testutil.NewFakeGitLab(t)
		cs := newTestSession(t, fake)
		text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": iid})
		if !isErr || !strings.Contains(text, "не указан iid MR") {
			t.Errorf("iid %d: isErr=%v text=%q", iid, isErr, text)
		}
		if reqs := fake.Requests(); len(reqs) != 0 {
			t.Errorf("iid %d: requests = %v, want none", iid, reqs)
		}
	}

	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)
	text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": "5"})
	if !isErr {
		t.Errorf("a string iid must be rejected by the schema, got %q", text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("string iid: requests = %v, want none", reqs)
	}
}

func TestMergeRequestNotFoundNamesSubject(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 404, `{"message":"404 Not found"}`, nil)
	fake.JSON("GET", mrsPath, 404, `{"message":"404 Project Not Found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "g/p", "iid": 5})
	if !isErr || !strings.Contains(text, "404: не найдено (MR (iid) или проект)") {
		t.Errorf("get: isErr=%v text=%q", isErr, text)
	}
	text, isErr = callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p"})
	if !isErr || !strings.Contains(text, "404: не найдено (проект)") {
		t.Errorf("list: isErr=%v text=%q", isErr, text)
	}
}

func TestListMergeRequestsNextPageFooter(t *testing.T) {
	t.Run("X-Next-Page", func(t *testing.T) {
		fake := testutil.NewFakeGitLab(t)
		fake.JSON("GET", mrsPath, 200, mrListJSON, map[string]string{"X-Next-Page": "2"})
		cs := newTestSession(t, fake)
		text, _ := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p"})
		if !strings.HasSuffix(text, "есть следующая страница: вызовите с page=2]") {
			t.Errorf("text = %q", text)
		}
	})
	t.Run("Link only", func(t *testing.T) {
		fake := testutil.NewFakeGitLab(t)
		link := "<" + fake.URL + "/api/v4/projects/g%2Fp/merge_requests?page=2&per_page=20&state=opened>; rel=\"next\""
		fake.JSON("GET", mrsPath, 200, mrListJSON, map[string]string{"Link": link})
		cs := newTestSession(t, fake)
		text, _ := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p"})
		if !strings.Contains(text, "есть следующая страница") {
			t.Errorf("Link-only answer must still announce a next page: %q", text)
		}
	})
	t.Run("last page", func(t *testing.T) {
		fake := testutil.NewFakeGitLab(t)
		fake.JSON("GET", mrsPath, 200, mrListJSON, nil)
		cs := newTestSession(t, fake)
		text, _ := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p"})
		if !strings.HasSuffix(text, "последняя страница]") {
			t.Errorf("text = %q", text)
		}
	})
}

func TestListMergeRequestsLongOutputIsBudgeted(t *testing.T) {
	var items []string
	long := strings.Repeat("т", 300)
	branch := strings.Repeat("b", 60)
	for i := 1; i <= 100; i++ {
		items = append(items, fmt.Sprintf(
			`{"iid":%d,"state":"opened","title":%q,"source_branch":%q,"target_branch":%q,"author":{"username":"alice"}}`,
			i, long, branch, branch))
	}
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mrsPath, 200, "["+strings.Join(items, ",")+"]", nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_requests", map[string]any{"project": "g/p", "per_page": 100})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, TruncatedFooter) {
		t.Errorf("want the truncation footer, got %d runes", len([]rune(text)))
	}
	if n := len([]rune(text)); n > OutputBudget+len([]rune(TruncatedFooter))+200 {
		t.Errorf("output is %d runes, want at most the budget plus footers", n)
	}
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(line)) > 300 {
			t.Errorf("line not bounded by mrTitleRunes: %d runes", len([]rune(line)))
		}
	}
}

func TestGetMergeRequestNumericProjectID(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/123/merge_requests/5", 200, mrJSON("mergeable", ""), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request", map[string]any{"project": "123", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if reqs := fake.Requests(); len(reqs) != 1 || reqs[0] != "GET /api/v4/projects/123/merge_requests/5" {
		t.Errorf("requests = %v", reqs)
	}
}
