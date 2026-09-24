package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const (
	commitPostPath = "/api/v4/projects/g%2Fp/repository/commits"
	newCommitSHA   = "a1b2c3d4e5f6a7b8c9d0a1b2c3d4e5f6a7b8c9d0"
	newCommitDiff  = commitPostPath + "/" + newCommitSHA + "/diff"
	commitPostReq  = "POST " + commitPostPath
	newCommitJSON  = `{"id":"` + newCommitSHA + `","short_id":"a1b2c3d4","title":"m",` +
		`"stats":{"additions":5,"deletions":2,"total":7},"web_url":"https://gitlab.example/g/p/-/commit/` + newCommitSHA + `"}`
)

func act(action, path string) map[string]any {
	return map[string]any{"action": action, "file_path": path}
}

func actWith(action, path, key, value string) map[string]any {
	a := act(action, path)
	a[key] = value
	return a
}

func commitArgs(actions ...map[string]any) map[string]any {
	list := make([]any, len(actions))
	for i, a := range actions {
		list[i] = a
	}
	return map[string]any{
		"project":        "g/p",
		"branch":         "feature/x",
		"commit_message": "m",
		"actions":        list,
	}
}

func TestCommitFilesSendsOnePostWithAllActions(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", commitPostPath, 201, newCommitJSON, nil)
	fake.JSON("GET", newCommitDiff, 200, `[
		{"new_path":"a/b.txt","old_path":"a/b.txt","new_file":true,"diff":"@@ -0,0 +1 @@\n+привет\n"},
		{"new_path":"src/main.go","old_path":"src/main.go","diff":"@@ -1 +1,4 @@\n-x\n+a\n+b\n+c\n+d\n"},
		{"new_path":"old.txt","old_path":"old.txt","deleted_file":true,"diff":"@@ -1 +0,0 @@\n-gone\n"},
		{"new_path":"docs/new.md","old_path":"docs/old.md","renamed_file":true,"diff":""}
	]`, nil)
	cs := newTestSession(t, fake)

	args := commitArgs(
		actWith("create", "a/b.txt", "content", "привет\r\n"),
		actWith("update", "src/main.go", "content", "x"),
		act("delete", "old.txt"),
		actWith("move", "docs/new.md", "previous_path", "docs/old.md"),
	)
	text, isErr := callText(t, cs, "commit_files", args)
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	if n := len(requestsTo(fake, commitPostReq)); n != 1 {
		t.Fatalf("POST count = %d, want exactly 1; requests: %v", n, fake.Requests())
	}
	var body map[string]any
	for _, r := range fake.Recorded() {
		if r.Method == "POST" {
			if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
				t.Fatalf("decode body %q: %v", r.Body, err)
			}
		}
	}
	if body["branch"] != "feature/x" || body["commit_message"] != "m" {
		t.Errorf("body = %v", body)
	}
	for _, key := range []string{"force", "start_branch", "encoding", "stats"} {
		if _, ok := body[key]; ok {
			t.Errorf("body must not carry %q: %v", key, body)
		}
	}
	acts, _ := body["actions"].([]any)
	if len(acts) != 4 {
		t.Fatalf("actions = %v", body["actions"])
	}
	want := []struct {
		action, path, prev string
		content            *string
	}{
		{"create", "a/b.txt", "", ptrStr("привет\r\n")},
		{"update", "src/main.go", "", ptrStr("x")},
		{"delete", "old.txt", "", nil},
		{"move", "docs/new.md", "docs/old.md", nil},
	}
	for i, w := range want {
		m, _ := acts[i].(map[string]any)
		if m["action"] != w.action || m["file_path"] != w.path {
			t.Errorf("action %d = %v", i, m)
		}
		if prev, has := m["previous_path"]; (w.prev == "") == has || (has && prev != w.prev) {
			t.Errorf("action %d previous_path = %v", i, m)
		}
		c, has := m["content"]
		if (w.content == nil) == has || (has && c != *w.content) {
			t.Errorf("action %d content = %v", i, m)
		}
		for _, key := range []string{"encoding", "execute_filemode", "last_commit_id"} {
			if _, ok := m[key]; ok {
				t.Errorf("action %d must not carry %q: %v", i, key, m)
			}
		}
	}

	if n := len(requestsTo(fake, "GET "+newCommitDiff+"?page=1&per_page=100")); n != 1 {
		t.Errorf("diff GET count = %d, requests: %v", n, fake.Requests())
	}
	lines := strings.Split(text, "\n")
	wantLines := []string{
		"коммит a1b2c3d4 в feature/x (+5/−2): 4 файла",
		"create a/b.txt (+1/−0)",
		"update src/main.go (+4/−1)",
		"delete old.txt (+0/−1)",
		"move docs/old.md → docs/new.md (+0/−0)",
		"https://gitlab.example/g/p/-/commit/" + newCommitSHA,
	}
	if len(lines) != len(wantLines) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(wantLines), text)
	}
	for i := range wantLines {
		if lines[i] != wantLines[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], wantLines[i])
		}
	}
}

func ptrStr(s string) *string { return &s }

func TestCommitFilesEmptyContentIsRejected(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "commit_files", commitArgs(actWith("create", "empty.txt", "content", "")))
	if !isErr {
		t.Fatalf("want a tool error, got %q", text)
	}
	if !strings.Contains(text, "непустой content") {
		t.Errorf("error %q does not mention the non-empty content rule", text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("empty content must not reach GitLab, got %v", reqs)
	}
}

func TestCommitFilesUnknownStatsAreMarked(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", commitPostPath, 201, newCommitJSON, nil)
	fake.JSON("GET", newCommitDiff, 200, `[
		{"new_path":"big.txt","old_path":"big.txt","too_large":true,"diff":""},
		{"new_path":"col.txt","old_path":"col.txt","collapsed":true,"diff":""}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "commit_files", commitArgs(
		actWith("update", "big.txt", "content", "x"),
		actWith("update", "col.txt", "content", "x"),
		actWith("update", "missing.txt", "content", "x"),
	))
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	for _, want := range []string{"update big.txt (+?/−?)", "update col.txt (+?/−?)", "update missing.txt (+?/−?)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "статистика по файлам недоступна") {
		t.Errorf("the follow-up read succeeded, no note expected:\n%s", text)
	}
}

func TestCommitFilesFollowUpFailureKeepsSuccess(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", commitPostPath, 201, newCommitJSON, nil)
	fake.JSON("GET", newCommitDiff, 500, `{"message":"boom"}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "commit_files", commitArgs(
		actWith("create", "a.txt", "content", "x"),
		act("delete", "b.txt"),
	))
	if isErr {
		t.Fatalf("a failed follow-up read must not fail the commit: %s", text)
	}
	if n := len(requestsTo(fake, commitPostReq)); n != 1 {
		t.Errorf("POST count = %d, want 1", n)
	}
	for _, want := range []string{
		"коммит a1b2c3d4 в feature/x",
		"create a.txt (+?/−?)",
		"delete b.txt (+?/−?)",
		"статистика по файлам недоступна: см. get_commit sha=" + newCommitSHA,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestCommitFilesGuardsSendNoRequest(t *testing.T) {
	many := make([]map[string]any, maxCommitActions+1)
	for i := range many {
		many[i] = actWith("create", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt", "content", "x")
	}
	big := strings.Repeat("a", maxCommitContentBytes+1)

	withField := func(key string, value any, actions ...map[string]any) map[string]any {
		a := commitArgs(actions...)
		a[key] = value
		return a
	}
	one := actWith("create", "a.txt", "content", "x")

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"no actions", commitArgs(), "actions: нужен хотя бы один файл"},
		{"too many", commitArgs(many...), "50"},
		{"too big", commitArgs(actWith("create", "a.txt", "content", big)), "1 МБ"},
		{"blank message", withField("commit_message", "  ", one), "commit_message не может быть пустым"},
		{"empty branch", withField("branch", "", one), "не указана ветка"},
		{"unknown action", commitArgs(actWith("chmod", "a.txt", "content", "x")), "create, update, delete, move"},
		{"dot dot", commitArgs(actWith("create", "../x", "content", "x")), ".."},
		{"move without previous", commitArgs(act("move", "b.txt")), "previous_path"},
		{"previous on create", commitArgs(map[string]any{"action": "create", "file_path": "a.txt", "content": "x", "previous_path": "z.txt"}), "previous_path"},
		{"NUL in content", commitArgs(actWith("create", "a.txt", "content", "a\u0000b")), "бинарн"},
		{"delete with content", commitArgs(actWith("delete", "a.txt", "content", "x")), "delete"},
		{"missing file_path", commitArgs(act("create", " ")), "file_path"},
		{"create without content", commitArgs(act("create", "a.txt")), "непустой content"},
		{"update with empty content", commitArgs(actWith("update", "a.txt", "content", "")), "непустой content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "commit_files", tc.args)
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			if !strings.Contains(text, tc.want) {
				t.Errorf("error %q does not contain %q", text, tc.want)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("guard must not send requests, got %v", reqs)
			}
		})
	}
}

func TestCommitFilesSchemaLevelRejections(t *testing.T) {
	cases := map[string]map[string]any{
		"actions null": {
			"project": "g/p", "branch": "b", "commit_message": "m", "actions": nil,
		},
		"extra key in item": {
			"project": "g/p", "branch": "b", "commit_message": "m",
			"actions": []any{map[string]any{"action": "create", "file_path": "a", "content": "x", "mode": "755"}},
		},
		"extra top-level key": {
			"project": "g/p", "branch": "b", "commit_message": "m", "force": true,
			"actions": []any{map[string]any{"action": "create", "file_path": "a", "content": "x"}},
		},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			cs := newTestSession(t, fake)
			text, isErr := callText(t, cs, "commit_files", args)
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			if reqs := fake.Requests(); len(reqs) != 0 {
				t.Errorf("rejected input must not send requests, got %v", reqs)
			}
		})
	}
}

func TestPluralFiles(t *testing.T) {
	cases := map[int]string{
		1: "1 файл", 2: "2 файла", 4: "4 файла", 5: "5 файлов",
		11: "11 файлов", 12: "12 файлов", 14: "14 файлов",
		21: "21 файл", 22: "22 файла", 50: "50 файлов",
	}
	for n, want := range cases {
		if got := pluralFiles(n); got != want {
			t.Errorf("pluralFiles(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPerFileStats(t *testing.T) {
	files := []diffFile{
		{NewPath: "patched.go", OldPath: "patched.go", Diff: "@@ -1 +1,2 @@\n-a\n+b\n+c\n"},
		{NewPath: "new.txt", OldPath: "new.txt", NewFile: true},
		{NewPath: "moved.md", OldPath: "was.md", RenamedFile: true},
		{NewPath: "gone.txt", OldPath: "gone.txt", DeletedFile: true, Diff: "@@ -1 +0,0 @@\n-x\n"},
		{NewPath: "big.txt", OldPath: "big.txt", TooLarge: true},
		{NewPath: "col.txt", OldPath: "col.txt", Collapsed: true, Diff: "@@ -1 +1 @@\n-a\n+b\n"},
	}
	stats := perFileStats(files)

	want := map[string]fileStat{
		"patched.go": {added: 2, removed: 1, known: true},
		"new.txt":    {known: true},
		"moved.md":   {known: true},
		"gone.txt":   {added: 0, removed: 1, known: true},
		"big.txt":    {},
		"col.txt":    {},
	}
	for path, w := range want {
		if got := stats[path]; got != w {
			t.Errorf("stats[%q] = %+v, want %+v", path, got, w)
		}
	}
	if _, ok := stats["was.md"]; ok {
		t.Error("the old path of a rename must not be a key")
	}
	if _, ok := stats["absent.txt"]; ok {
		t.Error("absent path must be missing so the caller shows +?/−?")
	}
}

func TestCommitFilesWriteFailures(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		contains []string
	}{
		{"unknown outcome on 503", 503, `<html>unavailable</html>`, []string{"Результат записи неизвестен"}},
		{"protected branch", 400, `{"message":"You are not allowed to push into this branch"}`, []string{"400: ветка защищена"}},
		{"file exists", 400, `{"message":"A file with this name already exists"}`, []string{"400: файл уже существует"}},
		{"not found", 404, `{"message":"404 Project Not Found"}`, []string{"проект или ветка"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			fake.JSON("POST", commitPostPath, tc.status, tc.body, nil)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "commit_files", commitArgs(actWith("create", "a.txt", "content", "x")))
			if !isErr {
				t.Fatalf("want a tool error, got %q", text)
			}
			for _, want := range tc.contains {
				if !strings.Contains(text, want) {
					t.Errorf("text %q does not contain %q", text, want)
				}
			}
			if n := len(requestsTo(fake, commitPostReq)); n != 1 {
				t.Errorf("POST count = %d, want exactly 1 (a write is never retried)", n)
			}
			if n := len(requestsTo(fake, "GET")); n != 0 {
				t.Errorf("a failed commit must not trigger a diff read, got %v", fake.Requests())
			}
		})
	}
}
