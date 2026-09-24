package tools

import (
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
