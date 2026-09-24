package tools

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

const mr5DiffsPath = mr5Path + "/diffs"

func TestGetMergeRequestDiffs(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"changes_count":"2","sha":"cafe1234","diff_refs":{"base_sha":"base5678"}`), nil)
	fake.JSON("GET", mr5DiffsPath, 200, `[
		{"diff":"@@ -1 +1 @@\n-a\n+b\n","new_path":"a.go","old_path":"a.go","a_mode":"100644","b_mode":"100644"},
		{"diff":"","new_path":"big.bin","old_path":"big.bin","a_mode":"100644","b_mode":"100644","too_large":true}
	]`, map[string]string{"X-Next-Page": "2"})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	want := []string{"GET " + mr5Path, "GET " + mr5DiffsPath + "?page=1&per_page=20"}
	if len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}
	for _, w := range []string{
		"!5 Add x", "ветки: f→main", "файлов на странице: 2",
		"### a.go [modified]\n@@ -1 +1 @@\n-a\n+b",
		"### big.bin [modified] — патч не показан: GitLab пометил файл как too_large; содержимое: get_file_contents path=big.bin ref=cafe1234",
	} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
	if strings.Contains(text, "часть файлов не вернулась") {
		t.Errorf("no overflow line expected for changes_count 3:\n%s", text)
	}
	if strings.Contains(text, "изменений в файлах нет") {
		t.Errorf("a page with files must not say there are no changes:\n%s", text)
	}
	if !strings.HasSuffix(text, "вызовите с page=2]") {
		t.Errorf("text must end with the next-page footer: %q", text)
	}
}

func TestGetMergeRequestDiffsEmptyPatchReasons(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"changes_count":"3"`), nil)
	fake.JSON("GET", mr5DiffsPath, 200, `[
		{"diff":"","new_path":"c.txt","old_path":"c.txt","a_mode":"100644","b_mode":"100644","collapsed":true},
		{"diff":"","new_path":"new.txt","old_path":"old.txt","a_mode":"100644","b_mode":"100644","renamed_file":true},
		{"diff":"","new_path":"huge.txt","old_path":"huge.txt","a_mode":"100644","b_mode":"100644","too_large":true}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	// With no SHAs in the MR the hints fall back to the branch names.
	for _, w := range []string{
		"### c.txt [modified] — патч не показан: GitLab свернул diff (collapsed); содержимое: get_file_contents path=c.txt ref=f",
		"### old.txt → new.txt [renamed] — только переименование, содержимое не менялось",
		"— патч не показан: GitLab пометил файл как too_large",
	} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
	if strings.Contains(text, "изменений в файлах нет") || strings.Contains(text, "diff ещё готовится") {
		t.Errorf("empty patches must never read as no changes:\n%s", text)
	}
}

func TestGetMergeRequestDiffsOverflowLine(t *testing.T) {
	for count, wantLine := range map[string]bool{"1000+": true, "999": false, "3": false} {
		fake := testutil.NewFakeGitLab(t)
		fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"changes_count":"`+count+`"`), nil)
		fake.JSON("GET", mr5DiffsPath, 200, `[{"diff":"@@ -1 +1 @@\n-a\n+b\n","new_path":"a.go","old_path":"a.go"}]`, nil)
		cs := newTestSession(t, fake)

		text, _ := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5})
		if has := strings.Contains(text, "часть файлов не вернулась"); has != wantLine {
			t.Errorf("changes_count %q: overflow line = %v, want %v:\n%s", count, has, wantLine, text)
		}
		if wantLine && !strings.Contains(text, "1000+") {
			t.Errorf("the overflow line must show changes_count:\n%s", text)
		}
	}
}

func TestGetMergeRequestDiffsEmptyPage(t *testing.T) {
	tests := []struct {
		name, extra, page, want, notWant string
	}{
		{"diff not ready", `"changes_count":""`, "", "diff ещё готовится, повторите через несколько секунд", "изменений в файлах нет"},
		{"page out of range", `"changes_count":"2"`, "5", "изменений в файлах нет (страница за пределами списка или MR без diff)", "diff ещё готовится"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", tt.extra), nil)
			fake.JSON("GET", mr5DiffsPath, 200, `[]`, nil)
			cs := newTestSession(t, fake)

			args := map[string]any{"project": "g/p", "iid": 5}
			if tt.page != "" {
				args["page"] = 5
			}
			text, isErr := callText(t, cs, "get_merge_request_diffs", args)
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			if !strings.Contains(text, tt.want) {
				t.Errorf("text lacks %q:\n%s", tt.want, text)
			}
			if strings.Contains(text, tt.notWant) {
				t.Errorf("text must not contain %q:\n%s", tt.notWant, text)
			}
			if !strings.Contains(text, "файлов на странице: 0") {
				t.Errorf("text lacks the file count:\n%s", text)
			}
		})
	}
}

func TestGetMergeRequestDiffsBudgeted(t *testing.T) {
	var files []string
	for i := 0; i < 30; i++ {
		files = append(files, fmt.Sprintf(`{"diff":%q,"new_path":"dir/file%02d.go","old_path":"dir/file%02d.go"}`, patchOf(5000), i, i))
	}
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"changes_count":"30"`), nil)
	fake.JSON("GET", mr5DiffsPath, 200, "["+strings.Join(files, ",")+"]", nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5, "per_page": 30})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	if n := utf8.RuneCountInString(text); n >= OutputBudget+500 {
		t.Errorf("text has %d runes, want within the output budget plus footers", n)
	}
	for i := 0; i < 30; i++ {
		if !strings.Contains(text, fmt.Sprintf("### dir/file%02d.go [modified]", i)) {
			t.Errorf("heading of file %d is missing", i)
		}
	}
	if !strings.Contains(text, "[патч обрезан:") && !strings.Contains(text, TruncatedFooter) {
		t.Errorf("cut patches must be marked:\n%.500s", text)
	}
	if !strings.Contains(text, "патч не показан (бюджет вывода исчерпан)") {
		t.Errorf("late files must say their patch is not shown")
	}
}

func TestGetMergeRequestDiffsPaging(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", mr5Path, 200, mrJSON("mergeable", `"changes_count":"1"`), nil)
	fake.JSON("GET", mr5DiffsPath, 200, `[]`, nil)
	cs := newTestSession(t, fake)

	if _, isErr := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5, "page": 3, "per_page": 500}); isErr {
		t.Fatal("unexpected tool error")
	}
	reqs := fake.Requests()
	if len(reqs) != 2 || reqs[1] != "GET "+mr5DiffsPath+"?page=3&per_page=100" {
		t.Errorf("requests = %v", reqs)
	}
}

func TestGetMergeRequestDiffsErrors(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 0})
	if !isErr || !strings.Contains(text, "iid") {
		t.Errorf("iid 0: isError=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("iid 0 must not reach GitLab, got %v", reqs)
	}

	text, isErr = callText(t, cs, "get_merge_request_diffs", map[string]any{"project": "g/p", "iid": 5})
	if !isErr || !strings.Contains(text, "404: не найдено (MR (iid) или проект)") {
		t.Errorf("missing MR: isError=%v text=%q", isErr, text)
	}
	for _, r := range fake.Requests() {
		if strings.Contains(r, "/diffs") {
			t.Errorf("no /diffs request after a 404 MR: %v", fake.Requests())
		}
	}
}
