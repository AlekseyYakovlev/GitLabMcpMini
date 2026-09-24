package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/glclient"
	"gitlab-mcp/internal/testutil"
)

// patchOf builds a patch of exactly n runes made of full 100-rune lines.
func patchOf(n int) string {
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString("+" + strings.Repeat("x", 98) + "\n")
	}
	return sb.String()[:n]
}

func TestFetchCommitDiffWireMatchesClientGo(t *testing.T) {
	cases := []struct{ project, sha, wantPath string }{
		{"g/p", "feature/x", "/api/v4/projects/g%2Fp/repository/commits/feature%2Fx/diff"},
		{"g/p", "release-1.0", "/api/v4/projects/g%2Fp/repository/commits/release-1%2E0/diff"},
	}
	for _, tc := range cases {
		t.Run(tc.sha, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			fake.JSON("GET", tc.wantPath, 200, `[]`, nil)
			d := newTestDeps(t, fake)
			ctx := context.Background()

			if _, _, err := fetchCommitDiff(ctx, d, tc.project, tc.sha, 1, 20); err != nil {
				t.Fatalf("fetchCommitDiff: %v", err)
			}
			mine := fake.Recorded()
			fake.Reset()

			if _, _, err := d.GL.Commits.GetCommitDiff(tc.project, tc.sha, nil); err != nil {
				t.Fatalf("GetCommitDiff: %v", err)
			}
			theirs := fake.Recorded()

			if len(mine) != 1 || len(theirs) != 1 {
				t.Fatalf("recorded mine=%v theirs=%v, want one request each", mine, theirs)
			}
			if want := tc.wantPath + "?page=1&per_page=20"; mine[0].URI != want {
				t.Errorf("fetchCommitDiff RequestURI = %q, want %q", mine[0].URI, want)
			}
			myPath, _, _ := strings.Cut(mine[0].URI, "?")
			theirPath, _, _ := strings.Cut(theirs[0].URI, "?")
			if myPath != theirPath {
				t.Errorf("path differs from client-go: mine %q, theirs %q", myPath, theirPath)
			}
		})
	}
}

func TestFetchCommitDiffDecodesFlags(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/commits/abc/diff", 200, `[
		{"diff":"","new_path":"big.bin","old_path":"big.bin","a_mode":"100644","b_mode":"100644","too_large":true},
		{"diff":"","new_path":"c.txt","old_path":"c.txt","a_mode":"100644","b_mode":"100644","collapsed":true},
		{"diff":"@@ -1 +1 @@\n-a\n+b\n","new_path":"n.txt","old_path":"n.txt","a_mode":"100644","b_mode":"100644"}
	]`, map[string]string{"X-Next-Page": "2"})
	d := newTestDeps(t, fake)

	files, resp, err := fetchCommitDiff(context.Background(), d, "g/p", "abc", 1, 20)
	if err != nil {
		t.Fatalf("fetchCommitDiff: %v", err)
	}
	if len(files) != 3 || !files[0].TooLarge || files[0].Collapsed || !files[1].Collapsed || files[1].TooLarge || files[2].Diff == "" {
		t.Fatalf("decoded files = %+v", files)
	}
	if resp == nil || resp.NextPage != 2 {
		t.Errorf("NextPage = %v, want 2", resp)
	}
}

func TestFetchCommitDiffNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	d := newTestDeps(t, fake)

	_, _, err := fetchCommitDiff(context.Background(), d, "g/p", "nope", 1, 20)
	if err == nil {
		t.Fatal("expected an error")
	}
	if e := glclient.Classify(err); e == nil || e.Kind != glclient.KindNotFound {
		t.Errorf("Classify(%v) = %+v, want KindNotFound", err, e)
	}
}

func TestDiffHeadings(t *testing.T) {
	files := []diffFile{
		{Diff: "@@\n+a\n", NewPath: "docs/a.md", OldPath: "docs/a.md", NewFile: true},
		{Diff: "@@\n-a\n", NewPath: "old.txt", OldPath: "old.txt", DeletedFile: true},
		{Diff: "@@\n-a\n+b\n", NewPath: "b.go", OldPath: "a.go", RenamedFile: true},
		{Diff: "@@\n-a\n+b\n", NewPath: "m.go", OldPath: "m.go"},
	}
	out := renderDiffFiles(files, "new", "old", 5000)
	for _, want := range []string{
		"### docs/a.md [new]\n",
		"### old.txt [deleted]\n",
		"### a.go → b.go [renamed]\n",
		"### m.go [modified]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks heading %q:\n%s", want, out)
		}
	}
}

func TestEmptyPatchReasons(t *testing.T) {
	const ref, parent = "1111111111111111111111111111111111111111", "2222222222222222222222222222222222222222"
	cases := []struct {
		name    string
		f       diffFile
		want    []string
		notWant []string
	}{
		{
			"too_large", diffFile{NewPath: "big.bin", OldPath: "big.bin", TooLarge: true},
			[]string{"too_large", "get_file_contents path=big.bin ref=" + ref}, nil,
		},
		{
			"collapsed", diffFile{NewPath: "c.txt", OldPath: "c.txt", Collapsed: true},
			[]string{"collapsed", "get_file_contents path=c.txt ref=" + ref}, nil,
		},
		{
			"rename only", diffFile{NewPath: "b.go", OldPath: "a.go", RenamedFile: true},
			[]string{"только переименование"}, []string{"get_file_contents"},
		},
		{
			"mode only", diffFile{NewPath: "run.sh", OldPath: "run.sh", AMode: "100644", BMode: "100755"},
			[]string{"изменён только режим файла (100644→100755)"}, []string{"get_file_contents"},
		},
		{
			"new empty or binary", diffFile{NewPath: "e.bin", OldPath: "e.bin", NewFile: true},
			[]string{"пустой или бинарный", "get_file_contents path=e.bin ref=" + ref}, nil,
		},
		{
			"deleted uses the parent", diffFile{NewPath: "gone.bin", OldPath: "gone.bin", DeletedFile: true},
			[]string{"пустой или бинарный", "get_file_contents path=gone.bin ref=" + parent}, []string{"ref=" + ref},
		},
		{
			"default", diffFile{NewPath: "x", OldPath: "x", AMode: "100644", BMode: "100644"},
			[]string{"патч не возвращён"}, nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderDiffFiles([]diffFile{tc.f}, ref, parent, 5000)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("output must not contain %q:\n%s", w, out)
				}
			}
			if r := emptyPatchReason(tc.f); r == "" || !strings.Contains(out, r) {
				t.Errorf("emptyPatchReason = %q, not in output:\n%s", r, out)
			}
		})
	}
}

func TestRenderCapsLongPatch(t *testing.T) {
	patch := patchOf(5000)
	out := renderDiffFiles([]diffFile{{Diff: patch, NewPath: "a.go", OldPath: "a.go"}}, "n", "o", 15000)

	body, marker, ok := strings.Cut(out, "\n[патч обрезан: показано ")
	if !ok {
		t.Fatalf("no cut marker in output of %d runes", utf8.RuneCountInString(out))
	}
	if !strings.HasSuffix(marker, " из 5000 символов]") {
		t.Errorf("marker = %q", marker)
	}
	patchPart := strings.TrimPrefix(body, "### a.go [modified]\n")
	if n := utf8.RuneCountInString(patchPart); n > patchCapRunes || n < patchCapRunes-100 {
		t.Errorf("shown patch has %d runes, want at most %d and a line boundary cut", n, patchCapRunes)
	}
	if !strings.HasSuffix(patchPart, strings.Repeat("x", 98)) {
		t.Errorf("patch must end on a whole line")
	}
}

func TestRenderBudgetKeepsEveryHeading(t *testing.T) {
	var files []diffFile
	for i := 0; i < 30; i++ {
		p := fmt.Sprintf("dir/file%02d.go", i)
		files = append(files, diffFile{Diff: patchOf(1900), NewPath: p, OldPath: p})
	}
	budget := OutputBudget - 300
	out := renderDiffFiles(files, "n", "o", budget)

	if n := utf8.RuneCountInString(out); n >= 15500 {
		t.Errorf("output has %d runes, want < 15500", n)
	}
	if n := utf8.RuneCountInString(out); n > budget {
		t.Errorf("output has %d runes, want within the %d budget", n, budget)
	}
	for _, f := range files {
		if !strings.Contains(out, "### "+f.NewPath+" [modified]") {
			t.Errorf("heading of %s is missing", f.NewPath)
		}
	}
	if !strings.Contains(out, "### dir/file29.go [modified] — патч не показан (бюджет вывода исчерпан)") {
		t.Errorf("the last file must say its patch is not shown:\n%s", out[len(out)-400:])
	}
	if !strings.Contains(out, "### dir/file00.go [modified]\n+xxx") {
		t.Errorf("the first file must keep its patch")
	}
}

func TestCountPatchLines(t *testing.T) {
	added, removed := countPatchLines("@@ -1 +1,2 @@\n-a\n+b\n+c\n\\ No newline at end of file")
	if added != 2 || removed != 1 {
		t.Errorf("countPatchLines = +%d/-%d, want +2/-1", added, removed)
	}
	if a, r := countPatchLines(""); a != 0 || r != 0 {
		t.Errorf("empty patch = +%d/-%d, want 0/0", a, r)
	}
}

func TestDiffFileJSONTags(t *testing.T) {
	raw, err := json.Marshal(diffFile{TooLarge: true, Collapsed: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"too_large":true`, `"collapsed":true`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("%s lacks %s", raw, key)
		}
	}
}
