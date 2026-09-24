package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const mr5NotesPath = mr5Path + "/notes"

func TestListMergeRequestNotes(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[
		{"id":10,"system":true,"body":"added 1 commit\n\n* abc - x","author":{"username":"alice"},"created_at":"2026-09-21T10:00:00Z"},
		{"id":11,"system":false,"author":{"username":"bob"},"created_at":"2026-09-20T10:00:00Z","body":"line1\nline2"}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET "+mr5NotesPath+"?page=1&per_page=20" {
		t.Fatalf("requests = %v", reqs)
	}
	want := strings.Join([]string{
		"заметки MR !5 проекта g/p (новые первыми)",
		"[system] added 1 commit",
		"#11 @bob 2026-09-20",
		"line1",
		"line2",
		"[page 1, per_page 20 — последняя страница]",
	}, "\n")
	if text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
}

func TestListMergeRequestNotesSystemLineIsCut(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[{"id":10,"system":true,"body":"`+strings.Repeat("s", 500)+`"}]`, nil)
	cs := newTestSession(t, fake)

	text, _ := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	line := strings.Split(text, "\n")[1]
	if !strings.HasPrefix(line, "[system] ") || len([]rune(line)) > len("[system] ")+noteSystemRunes {
		t.Errorf("system line = %q", line)
	}
}

func TestListMergeRequestNotesLongBodyIsCut(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[
		{"id":12,"system":false,"author":{"username":"bob"},"created_at":"2026-09-20T10:00:00Z","body":"`+strings.Repeat("x", 3000)+`"},
		{"id":11,"system":false,"author":{"username":"carol"},"created_at":"2026-09-19T10:00:00Z","body":"short"}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if !strings.Contains(text, "[заметка обрезана]") {
		t.Errorf("text lacks the note cut marker")
	}
	if strings.Contains(text, strings.Repeat("x", noteBodyRunes+1)) {
		t.Errorf("body must be cut at %d runes", noteBodyRunes)
	}
	if !strings.Contains(text, "#11 @carol 2026-09-19\nshort") {
		t.Errorf("the note after a cut one must still be shown:\n%.300s", text)
	}
}

func TestListMergeRequestNotesNullDateAndAuthor(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[{"id":13,"system":false,"created_at":null,"body":"hi"}]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if isErr || !strings.Contains(text, "#13 @- -\nhi") {
		t.Errorf("isError=%v text=%q", isErr, text)
	}
}

func TestListMergeRequestNotesEmptyAndNextPage(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[]`, nil)
	cs := newTestSession(t, fake)
	text, _ := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if !strings.Contains(text, "заметок нет") {
		t.Errorf("text = %q, want the empty-list line", text)
	}

	fake = testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[{"id":1,"system":false,"author":{"username":"a"},"body":"x"}]`, map[string]string{"X-Next-Page": "2"})
	cs = newTestSession(t, fake)
	text, _ = callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if !strings.HasSuffix(text, "вызовите с page=2]") {
		t.Errorf("text must end with the next-page footer: %q", text)
	}
}

func TestListMergeRequestNotesSendsNoOrdering(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5NotesPath, 200, `[]`, nil)
	cs := newTestSession(t, fake)

	callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5, "page": 2, "per_page": 500})
	q := queryOf(t, fake.Requests()[0])
	for _, key := range []string{"order_by", "sort"} {
		if _, ok := q[key]; ok {
			t.Errorf("%s must not be sent: %v", key, q)
		}
	}
	if q.Get("page") != "2" || q.Get("per_page") != "100" {
		t.Errorf("query = %v", q)
	}
}

func TestListMergeRequestNotesErrors(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 0})
	if !isErr || !strings.Contains(text, "iid") {
		t.Errorf("iid 0: isError=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("iid 0 must not reach GitLab, got %v", reqs)
	}

	text, isErr = callText(t, cs, "list_merge_request_notes", map[string]any{"project": "g/p", "iid": 5})
	if !isErr || !strings.Contains(text, "не найдено (MR (iid) или проект)") {
		t.Errorf("missing MR: isError=%v text=%q", isErr, text)
	}
}

func newNoteFake(t testing.TB) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", ""), nil)
	fake.JSON("POST", mr5NotesPath, 201, `{"id":42,"body":"LGTM","system":false}`, nil)
	return fake
}

func TestCreateMergeRequestNote(t *testing.T) {
	fake := newNoteFake(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "LGTM"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	want := []string{"GET " + mr5Path, "POST " + mr5NotesPath}
	if reqs := fake.Requests(); len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}
	if body := postBody(t, fake); len(body) != 1 || body["body"] != "LGTM" {
		t.Errorf("POST body = %v, want exactly {body: LGTM}", body)
	}
	for _, w := range []string{"комментарий #42 добавлен к MR !5", "https://gitlab.example/g/p/-/merge_requests/5"} {
		if !strings.Contains(text, w) {
			t.Errorf("text %q lacks %q", text, w)
		}
	}
}

func TestCreateMergeRequestNoteGuardsSendNothing(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"blank body", map[string]any{"project": "g/p", "iid": 5, "body": "   "}, "пустой комментарий (body)"},
		{"zero iid", map[string]any{"project": "g/p", "iid": 0, "body": "x"}, "не указан iid MR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newNoteFake(t)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "create_merge_request_note", tc.args)
			if !isErr || !strings.Contains(text, tc.want) {
				t.Fatalf("isErr=%v text=%q, want error containing %q", isErr, text, tc.want)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("no request expected, got %v", reqs)
			}
		})
	}
}

func TestCreateMergeRequestNoteMissingMRSendsNoPost(t *testing.T) {
	fake := newNoteFake(t)
	fake.JSON("GET", mr5Path, 404, `{"message":"404 Not found"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "x"})
	if !isErr || !strings.Contains(text, "не найдено (MR (iid) или проект)") {
		t.Fatalf("isErr=%v text=%q", isErr, text)
	}
	if n := countPosts(fake); n != 0 {
		t.Errorf("POST was sent %d times, want 0", n)
	}
}

func TestCreateMergeRequestNoteWithoutIDMentionsQuickActions(t *testing.T) {
	fake := newNoteFake(t)
	fake.JSON("POST", mr5NotesPath, 201, `{"id":0}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "/label bug"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	for _, w := range []string{"комментарий принят (id не возвращён", "быстрые команды", "https://gitlab.example/g/p/-/merge_requests/5"} {
		if !strings.Contains(text, w) {
			t.Errorf("text %q lacks %q", text, w)
		}
	}
	if strings.Contains(text, "#0") {
		t.Errorf("text %q must not show note #0", text)
	}
}

func TestCreateMergeRequestNoteSendsBodyUnchanged(t *testing.T) {
	fake := newNoteFake(t)
	cs := newTestSession(t, fake)

	body := "  line one\n\nline two\n"
	if text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": body}); isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	var got map[string]any
	for _, r := range fake.Recorded() {
		if r.Method == "POST" {
			if err := json.Unmarshal([]byte(r.Body), &got); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got["body"] != body {
		t.Errorf("body = %q, want %q byte for byte", got["body"], body)
	}
}

func TestCreateMergeRequestNoteServerErrorIsSentOnce(t *testing.T) {
	fake := newNoteFake(t)
	fake.JSON("POST", mr5NotesPath, 503, `<html>unavailable</html>`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "x"})
	if !isErr {
		t.Fatalf("expected isError, got %q", text)
	}
	for _, w := range []string{"Результат записи неизвестен", "list_merge_request_notes"} {
		if !strings.Contains(text, w) {
			t.Errorf("text %q lacks %q", text, w)
		}
	}
	if n := countPosts(fake); n != 1 {
		t.Errorf("POST was sent %d times, want exactly 1", n)
	}
}

func TestCreateMergeRequestNoteForbiddenUsesWriteWording(t *testing.T) {
	fake := newNoteFake(t)
	fake.JSON("POST", mr5NotesPath, 403, `{"message":"403 Forbidden"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "create_merge_request_note", map[string]any{"project": "g/p", "iid": 5, "body": "x"})
	if !isErr || !strings.Contains(text, "запись отклонена") {
		t.Fatalf("isErr=%v text=%q, want the write 403 wording", isErr, text)
	}
}
