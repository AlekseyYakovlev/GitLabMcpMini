package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const listCommitsDescription = "История коммитов GitLab-проекта: одна строка на коммит " +
	"«короткий SHA, дата, автор, заголовок». " +
	"По умолчанию берётся ветка по умолчанию проекта; ref задаёт другую ветку, тег или коммит. " +
	"Фильтры: path — только коммиты, затрагивающие файл или каталог; " +
	"since и until — границы дат в ISO 8601, например 2026-09-01 или 2026-09-01T12:00:00Z; " +
	"author — часть имени или email автора. " +
	"Полный SHA и diff коммита показывает get_commit. " +
	"Для следующей страницы вызовите снова с page из подсказки внизу."

const (
	// commitTitleRunes caps the commit title shown in a list line.
	commitTitleRunes = 120

	// commitDateLayout is the date shown in a list line.
	commitDateLayout = "2006-01-02"
)

// CommitsIn is the input of list_commits.
type CommitsIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Ref     string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA; default the project's default branch"`
	Path    string `json:"path,omitempty" jsonschema:"only commits touching this file or directory"`
	Since   string `json:"since,omitempty" jsonschema:"ISO 8601 date or date-time, for example 2026-09-01 or 2026-09-01T12:00:00Z"`
	Until   string `json:"until,omitempty" jsonschema:"ISO 8601 date or date-time, for example 2026-09-01 or 2026-09-01T12:00:00Z"`
	Author  string `json:"author,omitempty" jsonschema:"author name or email substring"`
	Page    int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}

// parseTime parses an ISO 8601 date or date-time argument. An empty value means
// "no filter" and yields nil. Values without a zone are read as UTC.
func parseTime(name, s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("%s: неверный формат даты %q, ожидается ISO 8601, например 2026-09-01 или 2026-09-01T12:00:00Z", name, s)
}

// listCommits returns the handler for the list_commits tool.
func listCommits(d Deps) func(ctx context.Context, in CommitsIn) (string, error) {
	return func(ctx context.Context, in CommitsIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		path, err := glclient.NormalizeRepoPath(in.Path)
		if err != nil {
			return "", err
		}
		since, err := parseTime("since", in.Since)
		if err != nil {
			return "", err
		}
		until, err := parseTime("until", in.Until)
		if err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
		if err != nil {
			return "", err
		}

		opts := &gitlab.ListCommitsOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			RefName:     gitlab.Ptr(ref),
			Since:       since,
			Until:       until,
		}
		if path != "" {
			opts.Path = gitlab.Ptr(path)
		}
		if author := strings.TrimSpace(in.Author); author != "" {
			opts.Author = gitlab.Ptr(author)
		}

		commits, resp, err := d.GL.Commits.ListCommits(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("ref или путь", err)
		}

		header := fmt.Sprintf("коммиты %s @ %s", project, refLabel(ref, isDefault))
		if path != "" {
			header += ", путь: " + path
		}
		if len(commits) == 0 {
			return header + "\nкоммиты не найдены", nil
		}

		lines := make([]string, 0, len(commits)+1)
		lines = append(lines, header)
		for _, c := range commits {
			lines = append(lines, commitLine(c))
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// commitLine renders one commit as "shortSHA date author title". A commit
// without a committed date shows "-" instead.
func commitLine(c *gitlab.Commit) string {
	date := "-"
	if c.CommittedDate != nil {
		date = c.CommittedDate.Format(commitDateLayout)
	}
	return strings.Join([]string{c.ShortID, date, c.AuthorName, oneLine(c.Title, commitTitleRunes)}, " ")
}
