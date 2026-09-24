package glclient

import (
	"net/http"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/testutil"
)

const apiProjects = "GET /api/v4/projects/"

// wireCase issues one real client-go call against a fake GitLab and returns
// the raw request lines the fake recorded.
func wireCase(t *testing.T, call func(c *gitlab.Client)) []string {
	t.Helper()
	f := testutil.NewFakeGitLab(t)
	c := testClient(t, f.URL, limits{maxBody: defaultMaxBody})
	call(c) // unmatched routes answer 404; only the recorded URI matters
	return f.Requests()
}

func TestWireProjectEncoding(t *testing.T) {
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{"subgroup with dot", "group/sub/my.proj", apiProjects + "group%2Fsub%2Fmy%2Eproj"},
		{"numeric id", "123", apiProjects + "123"},
		{"space and cyrillic", "gr oup/проект", apiProjects + "gr%20oup%2F%D0%BF%D1%80%D0%BE%D0%B5%D0%BA%D1%82"},
		{"hash plus percent passed directly", "a#b/c+d/e%f", apiProjects + "a%23b%2Fc+d%2Fe%25f"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wireCase(t, func(c *gitlab.Client) { _, _, _ = c.Projects.GetProject(tc.project, nil) })
			assertOneRequest(t, got, tc.want)
		})
	}
}

func TestWireNormalizedProjectEncodedOnce(t *testing.T) {
	p, err := NormalizeProject("group/sub/my.proj")
	if err != nil {
		t.Fatal(err)
	}
	got := wireCase(t, func(c *gitlab.Client) { _, _, _ = c.Projects.GetProject(p, nil) })
	assertOneRequest(t, got, apiProjects+"group%2Fsub%2Fmy%2Eproj")
	if strings.Contains(got[0], "%25") {
		t.Errorf("double encoding detected in %q", got[0])
	}
}

func TestWireFilePathEncoding(t *testing.T) {
	const base = apiProjects + "g%2Fp/repository/files/"
	tests := []struct {
		name string
		path string
		ref  string
		want string
	}{
		{"plain", "src/main.go", "main", base + "src%2Fmain%2Ego?ref=main"},
		{"space and hash", "dir with space/a#b.txt", "main", base + "dir%20with%20space%2Fa%23b%2Etxt?ref=main"},
		{"plus literal", "a+b.txt", "main", base + "a+b%2Etxt?ref=main"},
		{"percent", "100%.txt", "main", base + "100%25%2Etxt?ref=main"},
		{"cyrillic", "файл/имя.go", "main", base + "%D1%84%D0%B0%D0%B9%D0%BB%2F%D0%B8%D0%BC%D1%8F%2Ego?ref=main"},
		{"dotfile", ".gitignore", "main", base + "%2Egitignore?ref=main"},
		{"ref with slash space plus hash", "a.txt", "feature/x y+z#1", base + "a%2Etxt?ref=feature%2Fx+y%2Bz%231"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wireCase(t, func(c *gitlab.Client) {
				_, _, _ = c.RepositoryFiles.GetFile("g/p", tc.path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(tc.ref)})
			})
			assertOneRequest(t, got, tc.want)
		})
	}
}

func TestWireListTree(t *testing.T) {
	got := wireCase(t, func(c *gitlab.Client) {
		_, _, _ = c.Repositories.ListTree("g/sub/p", &gitlab.ListTreeOptions{
			ListOptions: gitlab.ListOptions{Page: 2, PerPage: 20},
			Path:        gitlab.Ptr("dir with space/ф+#%"),
			Ref:         gitlab.Ptr("feature/x"),
			Recursive:   gitlab.Ptr(true),
		})
	})
	want := apiProjects + "g%2Fsub%2Fp/repository/tree" +
		"?page=2&path=dir+with+space%2F%D1%84%2B%23%25&per_page=20&recursive=true&ref=feature%2Fx"
	assertOneRequest(t, got, want)
}

func TestWireTokenOnlyInPrivateTokenHeader(t *testing.T) {
	f := testutil.NewFakeGitLab(t)
	var headers map[string][]string
	f.Handle("GET", "/api/v4/projects/1", func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	c := testClient(t, f.URL, limits{maxBody: defaultMaxBody})
	if _, _, err := c.Projects.GetProject("1", nil); err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got := headers["Private-Token"]; len(got) != 1 || got[0] != testToken {
		t.Fatalf("Private-Token header = %v, want the token exactly once", got)
	}
	for name, values := range headers {
		if name == "Private-Token" {
			continue
		}
		for _, v := range values {
			if strings.Contains(v, testToken) {
				t.Errorf("token leaked into header %q", name)
			}
		}
	}
}

func assertOneRequest(t *testing.T, got []string, want string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("recorded %d requests %v, want exactly 1", len(got), got)
	}
	if got[0] != want {
		t.Errorf("wire request\n got: %s\nwant: %s", got[0], want)
	}
}
