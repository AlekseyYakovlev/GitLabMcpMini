package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"gitlab-mcp/internal/testutil"
)

func TestStdioHandshakeAndWhoami(t *testing.T) {
	fake := newFakeWithWhoami(t)
	s := startSession(t, fake.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The Go client negotiates the newest revision the SDK knows; the
	// 2025-11-25 revision used by the agent's client is asserted in smoke.py.
	if s.InitializeResult().ProtocolVersion == "" {
		t.Error("initialize returned no protocol version")
	}

	list, err := s.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "whoami" {
			found = true
		}
	}
	if !found {
		t.Fatalf("whoami not in tool list: %+v", list.Tools)
	}

	if reqs := fake.Requests(); len(reqs) != 0 {
		t.Fatalf("fake GitLab saw requests during initialize + tools/list: %v", reqs)
	}

	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("CallTool whoami: %v", err)
	}
	text := resultText(res)
	if res.IsError {
		t.Fatalf("whoami returned isError: %s", text)
	}
	if !strings.Contains(text, "@alice") {
		t.Errorf("whoami text %q does not contain @alice", text)
	}
	if strings.Contains(text, testToken) {
		t.Errorf("whoami text leaks the token: %q", text)
	}
	if reqs := fake.Requests(); len(reqs) == 0 || reqs[0] != "GET /api/v4/user" {
		t.Errorf("requests = %v, want first GET /api/v4/user", reqs)
	}

	if err := s.Close(); err != nil {
		t.Logf("Close: %v", err)
	}
	if s.cmd.ProcessState == nil {
		t.Fatal("process has not exited after session Close")
	}
	if code := s.cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code after stdin close = %d, want 0", code)
	}
	if strings.Contains(s.stderr.String(), testToken) {
		t.Errorf("stderr leaks the token: %q", s.stderr.String())
	}
}

func TestStdioMissingToken(t *testing.T) {
	cmd := exec.Command(binPath)
	cmd.Env = envWithout("GITLAB_TOKEN", "GITLAB_URL")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("Wait error = %v, want *exec.ExitError with code 1", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("binary did not exit within 5 s without GITLAB_TOKEN")
	}

	if !strings.Contains(stderr.String(), "GITLAB_TOKEN") {
		t.Errorf("stderr %q does not mention GITLAB_TOKEN", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty, got %q", stdout.String())
	}
}

// TestStdioWirePaths drives the real binary and checks the exact bytes that
// reach GitLab: project and file paths must be percent-encoded exactly once.
func TestStdioWirePaths(t *testing.T) {
	const projectJSON = `{"id":1,"path_with_namespace":"g/p","default_branch":"main"}`
	const fileJSON = `{"file_name":"f","file_path":"f","size":6,"encoding":"base64","content":"aGVsbG8K","ref":"main","blob_id":"b1","last_commit_id":"c1"}`
	const treeJSON = `[{"id":"a","name":"x.go","type":"blob","path":"x.go","mode":"100644"}]`

	const files = "/api/v4/projects/g%2Fp/repository/files/"
	cases := []struct {
		name string
		tool string
		args map[string]any
		want string // expected "METHOD RequestURI"
		body string
	}{
		{
			"project with dot", "get_project",
			map[string]any{"project": "group/sub/my.proj"},
			"GET /api/v4/projects/group%2Fsub%2Fmy%2Eproj", projectJSON,
		},
		{
			"project with space and Cyrillic", "get_project",
			map[string]any{"project": "gr oup/проект"},
			"GET /api/v4/projects/gr%20oup%2F%D0%BF%D1%80%D0%BE%D0%B5%D0%BA%D1%82", projectJSON,
		},
		{
			"file with space, hash and ref with slash", "get_file_contents",
			map[string]any{"project": "g/p", "path": "dir with space/a#b.txt", "ref": "feature/x"},
			"GET " + files + "dir%20with%20space%2Fa%23b%2Etxt?ref=feature%2Fx", fileJSON,
		},
		{
			"file with plus", "get_file_contents",
			map[string]any{"project": "g/p", "path": "a+b.txt", "ref": "main"},
			"GET " + files + "a+b%2Etxt?ref=main", fileJSON,
		},
		{
			"file with percent", "get_file_contents",
			map[string]any{"project": "g/p", "path": "100%.txt", "ref": "main"},
			"GET " + files + "100%25%2Etxt?ref=main", fileJSON,
		},
		{
			"file with Cyrillic", "get_file_contents",
			map[string]any{"project": "g/p", "path": "файл/имя.go", "ref": "main"},
			"GET " + files + "%D1%84%D0%B0%D0%B9%D0%BB%2F%D0%B8%D0%BC%D1%8F%2Ego?ref=main", fileJSON,
		},
		{
			"tree with awkward path", "list_repository_tree",
			map[string]any{
				"project": "g/sub/p", "path": "dir with space/ф+#%", "ref": "feature/x",
				"page": 2, "per_page": 20, "recursive": true,
			},
			"GET /api/v4/projects/g%2Fsub%2Fp/repository/tree?page=2&path=dir+with+space%2F%D1%84%2B%23%25&per_page=20&recursive=true&ref=feature%2Fx",
			treeJSON,
		},
	}

	fake := testutil.NewFakeGitLab(t)
	for _, tc := range cases {
		route := strings.TrimPrefix(tc.want, "GET ")
		route, _, _ = strings.Cut(route, "?")
		fake.JSON("GET", route, 200, tc.body, nil)
	}
	s := startSession(t, fake.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	list, err := s.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	wantNames := []string{"get_file_contents", "get_project", "list_projects", "list_repository_tree", "whoami"}
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("tools = %v, want %v", names, wantNames)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake.Reset()
			res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.tool, err)
			}
			if res.IsError {
				t.Fatalf("%s returned isError: %s", tc.tool, resultText(res))
			}
			reqs := fake.Requests()
			if len(reqs) != 1 || reqs[0] != tc.want {
				t.Errorf("requests = %v, want exactly [%s]", reqs, tc.want)
			}
		})
	}

	if strings.Contains(s.stderr.String(), testToken) {
		t.Errorf("stderr leaks the token: %q", s.stderr.String())
	}
}

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
