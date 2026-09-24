package tools

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"gitlab-mcp/internal/testutil"
)

const (
	fileProjectRoute = "/api/v4/projects/g%2Fp"
	fileRoute        = "/api/v4/projects/g%2Fp/repository/files/src%2Fmain%2Ego"
)

// fileJSON builds a GitLab file response for content.
func fileJSON(path string, content []byte) string {
	return fmt.Sprintf(`{"file_name":"x","file_path":%q,"size":%d,"encoding":"base64","content":%q,"ref":"main","blob_id":"b1","last_commit_id":"c1"}`,
		path, len(content), base64.StdEncoding.EncodeToString(content))
}

func newFileFake(t *testing.T, content []byte) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", fileProjectRoute, 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"main"}`, nil)
	fake.JSON("GET", fileRoute, 200, fileJSON("src/main.go", content), nil)
	return fake
}

func TestGetFileContentsDefaultBranch(t *testing.T) {
	content := "package main\nfunc main() {}\n// конец\n"
	fake := newFileFake(t, []byte(content))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}

	reqs := fake.Requests()
	want := []string{
		"GET /api/v4/projects/g%2Fp",
		"GET /api/v4/projects/g%2Fp/repository/files/src%2Fmain%2Ego?ref=main",
	}
	if len(reqs) != 2 || reqs[0] != want[0] || reqs[1] != want[1] {
		t.Fatalf("requests = %v, want %v", reqs, want)
	}

	lines := strings.Split(text, "\n")
	wantLines := []string{
		fmt.Sprintf("src/main.go @ main (ветка по умолчанию) — строки 1-3 из 3, %d байт", len(content)),
		"package main",
		"func main() {}",
		"// конец",
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

func TestGetFileContentsExplicitRef(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", fileRoute, 200, fileJSON("src/main.go", []byte("a\nb")), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "feature/x"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 || reqs[0] != "GET /api/v4/projects/g%2Fp/repository/files/src%2Fmain%2Ego?ref=feature%2Fx" {
		t.Fatalf("requests = %v", reqs)
	}
	first := strings.Split(text, "\n")[0]
	if !strings.HasPrefix(first, "src/main.go @ feature/x — строки 1-2 из 2") || strings.Contains(first, "ветка по умолчанию") {
		t.Errorf("header = %q", first)
	}
}

func TestGetFileContentsLineRange(t *testing.T) {
	fake := newFileFake(t, []byte("l1\nl2\nl3\nl4\nl5\n"))
	cs := newTestSession(t, fake)

	tests := []struct {
		name       string
		args       map[string]any
		wantHeader string
		wantBody   string
	}{
		{"middle", map[string]any{"start_line": 2, "end_line": 3}, "строки 2-3 из 5", "l2\nl3"},
		{"end clamped", map[string]any{"start_line": 4, "end_line": 100}, "строки 4-5 из 5", "l4\nl5"},
		{"only start", map[string]any{"start_line": 5}, "строки 5-5 из 5", "l5"},
		{"only end", map[string]any{"end_line": 2}, "строки 1-2 из 5", "l1\nl2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main"}
			for k, v := range tc.args {
				args[k] = v
			}
			text, isErr := callText(t, cs, "get_file_contents", args)
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			head, body, _ := strings.Cut(text, "\n")
			if !strings.Contains(head, tc.wantHeader) {
				t.Errorf("header = %q, want %q", head, tc.wantHeader)
			}
			if body != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}

func TestGetFileContentsBadRange(t *testing.T) {
	fake := newFileFake(t, []byte("l1\nl2\nl3\nl4\nl5\n"))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main", "start_line": 9})
	if !isErr || !strings.Contains(text, "5") {
		t.Errorf("start beyond file: isErr=%v text=%q, want error naming 5 lines", isErr, text)
	}

	text, isErr = callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main", "start_line": 3, "end_line": 1})
	if !isErr {
		t.Errorf("end before start: want isError, got %q", text)
	}
}

func TestGetFileContentsBOMCRLFAndCyrillic(t *testing.T) {
	content := "\uFEFFпривет\r\nмир\r\nячмень\r\n"
	fake := newFileFake(t, []byte(content))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	_, body, _ := strings.Cut(text, "\n")
	if body != "привет\nмир\nячмень" {
		t.Errorf("body = %q", body)
	}
	if strings.ContainsAny(text, "\r\uFEFF") {
		t.Errorf("text still contains CR or BOM: %q", text)
	}
}

func TestGetFileContentsTruncatesLargeFile(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString(strings.Repeat("ж", 19))
		sb.WriteString("\n")
	}
	fake := newFileFake(t, []byte(sb.String()))
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	lines := strings.Split(text, "\n")
	footer := lines[len(lines)-1]
	body := lines[1 : len(lines)-1]
	k := len(body)
	wantFooter := fmt.Sprintf("[файл обрезан на 15000 символах: показаны строки 1-%d из 2000; используйте start_line/end_line]", k)
	if footer != wantFooter {
		t.Errorf("footer = %q, want %q", footer, wantFooter)
	}
	if k == 0 || k >= 2000 {
		t.Fatalf("kept %d lines", k)
	}
	if n := utf8.RuneCountInString(strings.Join(body, "\n")); n > OutputBudget {
		t.Errorf("body has %d runes, want <= %d", n, OutputBudget)
	}
	for i, l := range body {
		if utf8.RuneCountInString(l) != 19 {
			t.Fatalf("line %d is not a complete line: %q", i, l)
		}
	}
}

func TestGetFileContentsTruncatedRangeFooter(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString(strings.Repeat("x", 19))
		sb.WriteString("\n")
	}
	fake := newFileFake(t, []byte(sb.String()))
	cs := newTestSession(t, fake)

	text, _ := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/main.go", "ref": "main", "start_line": 101})
	lines := strings.Split(text, "\n")
	k := len(lines) - 2
	want := fmt.Sprintf("[файл обрезан на 15000 символах: показаны строки 101-%d из 2000; используйте start_line/end_line]", 100+k)
	if lines[len(lines)-1] != want {
		t.Errorf("footer = %q, want %q", lines[len(lines)-1], want)
	}
}

func TestGetFileContentsBinary(t *testing.T) {
	content := append([]byte("PNG"), 0x00, 0x01, 0x02)
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", fileProjectRoute, 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"main"}`, nil)
	fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/files/src%2Flogo%2Epng", 200, fileJSON("src/logo.png", content), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "src/logo.png"})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	want := fmt.Sprintf("src/logo.png @ main (ветка по умолчанию): файл бинарный, %d байт, blob_id b1", len(content))
	if text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if strings.Contains(text, base64.StdEncoding.EncodeToString(content)) {
		t.Errorf("text leaks base64 content")
	}
}

func TestIsBinary(t *testing.T) {
	// 'ж' is 2 bytes; 1 + 5000*2 bytes puts the 8000-byte probe boundary in the
	// middle of a rune.
	midRune := []byte("a" + strings.Repeat("ж", 5000))
	if utf8.RuneStart(midRune[binaryProbeBytes]) {
		t.Fatalf("fixture does not cut inside a rune")
	}

	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"empty", nil, false},
		{"ascii", []byte("hello\n"), false},
		{"cyrillic", []byte("привет"), false},
		{"nul", []byte("ab\x00cd"), true},
		{"invalid utf8", []byte{0xff, 0xfe, 'a', 'b'}, true},
		{"probe cuts a rune", midRune, false},
		{"nul after probe is ignored", append([]byte(strings.Repeat("a", binaryProbeBytes)), 0), false},
		{"invalid inside probe of long file", append([]byte{0xff}, []byte(strings.Repeat("a", 9000))...), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBinary(tc.in); got != tc.want {
				t.Errorf("isBinary = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectLines(t *testing.T) {
	lines, from, to, total, err := selectLines("a\r\nb\nc", 2, 0)
	if err != nil || from != 2 || to != 3 || total != 3 || strings.Join(lines, "|") != "b|c" {
		t.Errorf("got %v %d %d %d %v", lines, from, to, total, err)
	}
	if _, _, _, _, err := selectLines("a\nb", 3, 0); err == nil {
		t.Error("start beyond file: want error")
	}
	if _, _, _, _, err := selectLines("a\nb", 2, 1); err == nil {
		t.Error("end before start: want error")
	}
	if _, _, _, total, err := selectLines("", 0, 0); err != nil || total != 0 {
		t.Errorf("empty text: total=%d err=%v", total, err)
	}
}

func TestGetFileContentsOversize(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.Handle("GET", "/api/v4/projects/g%2Fp/repository/files/big%2Etxt", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"file_name":"big.txt","size":9000000,"encoding":"base64","blob_id":"b1","content":"`))
		chunk := []byte(strings.Repeat("A", 1<<20))
		for i := 0; i < 9; i++ {
			_, _ = w.Write(chunk)
		}
		_, _ = w.Write([]byte(`"}`))
	})
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "big.txt", "ref": "main"})
	if !isErr || !strings.Contains(text, "слишком большой") {
		t.Errorf("isErr=%v text=%q, want a 'слишком большой' error", isErr, text)
	}
}

func TestGetFileContentsNotFound(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "nope.txt", "ref": "main"})
	if !isErr || !strings.Contains(text, "404") || !strings.Contains(text, "файл") {
		t.Errorf("isErr=%v text=%q, want 404 and 'файл'", isErr, text)
	}
}

func TestGetFileContentsBadPath(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": ""})
	if !isErr || !strings.Contains(text, "не указан путь") {
		t.Errorf("empty path: isErr=%v text=%q", isErr, text)
	}
	text, isErr = callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "../x"})
	if !isErr || !strings.Contains(text, "..") {
		t.Errorf("dotdot path: isErr=%v text=%q", isErr, text)
	}
	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Errorf("no request must reach GitLab, got %v", reqs)
	}
}

func TestGetFileContentsEmptyRepo(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", fileProjectRoute, 200, `{"id":1,"path_with_namespace":"g/p","default_branch":"","empty_repo":true}`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "get_file_contents", map[string]any{"project": "g/p", "path": "README.md"})
	if !isErr || !strings.Contains(text, "репозиторий пуст") {
		t.Errorf("isErr=%v text=%q, want 'репозиторий пуст'", isErr, text)
	}
}
