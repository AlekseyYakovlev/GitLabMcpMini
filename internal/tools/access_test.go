package tools

import (
	"strings"
	"testing"

	"gitlab-mcp/internal/testutil"
)

const genericForbidden = "403: запись отклонена: ветка может быть защищена"

// projectBody builds a GET /projects/:id answer; extra is spliced in as raw
// JSON members such as `"archived":true`.
func projectBody(extra string) string {
	body := `{"id":7,"path_with_namespace":"g/p","default_branch":"main"`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

// forbiddenBranchFake answers create_branch with 403 and the project lookup
// with the given status and body.
func forbiddenBranchFake(t testing.TB, projectStatus int, projectJSON string) *testutil.FakeGitLab {
	t.Helper()
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("POST", branchesPath, 403, `{"message":"403 Forbidden"}`, nil)
	fake.JSON("GET", projectRoute, projectStatus, projectJSON, nil)
	return fake
}

func createBranchExplicitRef(t testing.TB, fake *testutil.FakeGitLab) string {
	t.Helper()
	cs := newTestSession(t, fake)
	text, isErr := callText(t, cs, "create_branch", map[string]any{"project": "g/p", "branch": "feature/x", "ref": "main"})
	if !isErr {
		t.Fatalf("expected a tool error, got %q", text)
	}
	return text
}

func TestWriteForbiddenNamesTheReason(t *testing.T) {
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{"marked for deletion", projectBody(`"marked_for_deletion_on":"2026-10-01","archived":true,"permissions":{"project_access":{"access_level":20}}`),
			"403: проект запланирован к удалению (2026-10-01), запись невозможна"},
		{"archived", projectBody(`"archived":true,"permissions":{"project_access":{"access_level":20}}`),
			"403: проект в архиве, запись запрещена"},
		{"role below developer", projectBody(`"permissions":{"project_access":{"access_level":20}}`),
			"403: роль токена Reporter ниже Developer; нужен Developer+"},
		{"role from group access only", projectBody(`"permissions":{"project_access":null,"group_access":{"access_level":10}}`),
			"403: роль токена Guest ниже Developer; нужен Developer+"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := forbiddenBranchFake(t, 200, tc.project)
			text := createBranchExplicitRef(t, fake)
			if text != tc.want {
				t.Errorf("text = %q, want %q", text, tc.want)
			}
			if n := len(requestsTo(fake, "POST ")); n != 1 {
				t.Errorf("POST sent %d times, want exactly 1", n)
			}
			if n := len(requestsTo(fake, "GET "+projectRoute)); n != 1 {
				t.Errorf("project lookups = %d, want exactly 1", n)
			}
		})
	}
}

func TestWriteForbiddenKeepsGenericTextWhenNothingExplainsIt(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		project string
	}{
		{"developer on a live project", 200, projectBody(`"permissions":{"project_access":{"access_level":30}}`)},
		{"group access outranks project access", 200, projectBody(`"permissions":{"project_access":{"access_level":20},"group_access":{"access_level":40}}`)},
		{"permissions missing", 200, projectBody("")},
		{"lookup answers 404", 404, `{"message":"404 Project Not Found"}`},
		{"lookup answers 500", 500, `{"message":"boom"}`},
		{"lookup answers garbage", 200, `not json`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := forbiddenBranchFake(t, tc.status, tc.project)
			text := createBranchExplicitRef(t, fake)
			if !strings.HasPrefix(text, genericForbidden) {
				t.Errorf("text = %q, want the generic 403 wording", text)
			}
			if strings.Contains(text, "boom") || strings.Contains(text, "Project Not Found") {
				t.Errorf("text %q echoes the lookup response", text)
			}
			if n := len(requestsTo(fake, "POST ")); n != 1 {
				t.Errorf("POST sent %d times, want exactly 1", n)
			}
		})
	}
}

func TestWriteForbiddenReasonAppliesToOtherWrites(t *testing.T) {
	const deleted = "403: проект запланирован к удалению (2026-10-01), запись невозможна"
	project := projectBody(`"marked_for_deletion_on":"2026-10-01"`)

	t.Run("create_or_update_file", func(t *testing.T) {
		fake := newUpsertFake(t, 404, `{"message":"404 File Not Found"}`)
		fake.JSON("POST", commitPostPath, 403, `{"message":"403 Forbidden"}`, nil)
		fake.JSON("GET", projectRoute, 200, project, nil)
		cs := newTestSession(t, fake)
		text, isErr := callText(t, cs, "create_or_update_file", upsertArgs("hello\n"))
		if !isErr || text != deleted {
			t.Fatalf("isErr=%v text=%q, want %q", isErr, text, deleted)
		}
	})

	t.Run("create_merge_request", func(t *testing.T) {
		fake := newCreateMRFake(t)
		fake.JSON("POST", mrsPath, 403, `{"message":"403 Forbidden"}`, nil)
		fake.JSON("GET", projectRoute, 200, project, nil)
		cs := newTestSession(t, fake)
		text, isErr := callText(t, cs, "create_merge_request", map[string]any{
			"project": "g/p", "source_branch": "feature/x", "title": "Add x", "target_branch": "main",
		})
		if !isErr || text != deleted {
			t.Fatalf("isErr=%v text=%q, want %q", isErr, text, deleted)
		}
	})
}

func TestReadForbiddenIsNotExplained(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", branchesPath, 403, `{"message":"403 Forbidden"}`, nil)
	fake.JSON("GET", projectRoute, 200, projectBody(`"marked_for_deletion_on":"2026-10-01"`), nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_branches", map[string]any{"project": "g/p"})
	if !isErr || !strings.HasPrefix(text, "403: недостаточно прав") {
		t.Fatalf("isErr=%v text=%q, want the generic read 403", isErr, text)
	}
	for _, r := range fake.Requests() {
		if r == "GET "+projectRoute {
			t.Errorf("project lookup %q made for a read failure", r)
		}
	}
}

func TestGetProjectShowsDeletionAndAccess(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    []string
		notWant []string
	}{
		{"marked for deletion, project access", projectBody(`"marked_for_deletion_on":"2026-10-01","permissions":{"project_access":{"access_level":30}}`),
			[]string{"marked_for_deletion_on: 2026-10-01\n", "your_access: Developer (30)\n"}, nil},
		{"not marked, group access wins", projectBody(`"permissions":{"project_access":{"access_level":20},"group_access":{"access_level":50}}`),
			[]string{"your_access: Owner (50)\n"}, []string{"marked_for_deletion_on"}},
		{"null deletion date, no permissions", projectBody(`"marked_for_deletion_on":null,"permissions":null`),
			[]string{"your_access: unknown\n"}, []string{"marked_for_deletion_on"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := testutil.NewFakeGitLab(t)
			fake.JSON("GET", projectRoute, 200, tc.body, nil)
			cs := newTestSession(t, fake)

			text, isErr := callText(t, cs, "get_project", map[string]any{"project": "g/p"})
			if isErr {
				t.Fatalf("unexpected tool error: %s", text)
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("text %q lacks %q", text, w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(text, w) {
					t.Errorf("text %q contains %q", text, w)
				}
			}
		})
	}
}

func TestListProjectsMarksDeletedAndArchived(t *testing.T) {
	fake := testutil.NewFakeGitLab(t)
	fake.JSON("GET", "/api/v4/projects", 200, `[
		{"id":1,"path_with_namespace":"g/live","default_branch":"main"},
		{"id":2,"path_with_namespace":"g/gone","default_branch":"main","marked_for_deletion_on":"2026-10-01","description":"old"},
		{"id":3,"path_with_namespace":"g/old","archived":true},
		{"id":4,"path_with_namespace":"g/both","archived":true,"marked_for_deletion_on":"2026-10-01"}
	]`, nil)
	cs := newTestSession(t, fake)

	text, isErr := callText(t, cs, "list_projects", map[string]any{})
	if isErr {
		t.Fatalf("unexpected tool error: %s", text)
	}
	lines := strings.Split(text, "\n")
	want := []string{
		"1 g/live (main)",
		"2 g/gone (main) [scheduled for deletion] — old",
		"3 g/old [archived]",
		"4 g/both [scheduled for deletion] [archived]",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}
