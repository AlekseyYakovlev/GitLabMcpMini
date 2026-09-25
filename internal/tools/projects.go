package tools

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const (
	listProjectsDescription = "Список проектов GitLab, доступных по токену (по умолчанию только те, где вы участник). " +
		"Одна строка на проект: id, путь, ветка по умолчанию, описание. " +
		"Для следующей страницы вызовите снова с page из подсказки внизу."

	getProjectDescription = "Ключевые поля одного проекта GitLab: id, путь, ветка по умолчанию, видимость, " +
		"архивный ли, дата последней активности, ссылка и описание. " +
		"Проект задаётся числовым ID или путём group/subgroup/project."

	listProjectLineDescriptionRunes = 120
	getProjectDescriptionRunes      = 300
)

// ListProjectsIn is the input of list_projects.
type ListProjectsIn struct {
	Search        string `json:"search,omitempty" jsonschema:"filter by name or path substring"`
	IncludePublic bool   `json:"include_public,omitempty" jsonschema:"also include public projects you are not a member of; default false = only your projects"`
	Page          int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage       int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}

// ProjectIn is the input of tools that take just a project. Later tools embed
// or mirror it so the project argument is described the same way everywhere.
type ProjectIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
}

// listProjects returns the handler for the list_projects tool.
func listProjects(d Deps) func(ctx context.Context, in ListProjectsIn) (string, error) {
	return func(ctx context.Context, in ListProjectsIn) (string, error) {
		page, perPage := ClampPaging(in.Page, in.PerPage)

		opts := &gitlab.ListProjectsOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			Simple:      gitlab.Ptr(true),
		}
		if !in.IncludePublic {
			opts.Membership = gitlab.Ptr(true)
		}
		if q := strings.TrimSpace(in.Search); q != "" {
			opts.Search = gitlab.Ptr(q)
		}

		projects, resp, err := d.GL.Projects.ListProjects(opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("проекты", err)
		}
		if len(projects) == 0 {
			return "проекты не найдены", nil
		}

		lines := make([]string, 0, len(projects))
		for _, p := range projects {
			lines = append(lines, projectLine(p))
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// projectLine renders "<id> <path> (<default_branch>) — <description>", with
// the branch and description parts omitted when empty.
func projectLine(p *gitlab.Project) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %s", p.ID, p.PathWithNamespace)
	if p.DefaultBranch != "" {
		fmt.Fprintf(&sb, " (%s)", p.DefaultBranch)
	}
	if desc := oneLine(p.Description, listProjectLineDescriptionRunes); desc != "" {
		fmt.Fprintf(&sb, " — %s", desc)
	}
	return sb.String()
}

// getProject returns the handler for the get_project tool.
func getProject(d Deps) func(ctx context.Context, in ProjectIn) (string, error) {
	return func(ctx context.Context, in ProjectIn) (string, error) {
		id, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		p, _, err := d.GL.Projects.GetProject(id, nil, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("проект", err)
		}

		branch := p.DefaultBranch
		if branch == "" {
			branch = "(нет — репозиторий пуст)"
		}
		archived := "no"
		if p.Archived {
			archived = "yes"
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "id: %d\n", p.ID)
		fmt.Fprintf(&sb, "path: %s\n", p.PathWithNamespace)
		fmt.Fprintf(&sb, "default_branch: %s\n", branch)
		fmt.Fprintf(&sb, "visibility: %s\n", p.Visibility)
		fmt.Fprintf(&sb, "archived: %s\n", archived)
		if p.LastActivityAt != nil {
			fmt.Fprintf(&sb, "last_activity: %s\n", p.LastActivityAt.Format("2006-01-02"))
		}
		fmt.Fprintf(&sb, "web_url: %s", p.WebURL)
		if desc := oneLine(p.Description, getProjectDescriptionRunes); desc != "" {
			fmt.Fprintf(&sb, "\ndescription: %s", desc)
		}

		out, truncated := Budget(sb.String(), OutputBudget)
		if truncated {
			out += "\n" + TruncatedFooter
		}
		return out, nil
	}
}
