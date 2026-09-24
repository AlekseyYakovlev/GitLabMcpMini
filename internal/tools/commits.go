package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

const getCommitDescription = "Один коммит GitLab-проекта по SHA, ветке или тегу: " +
	"шапка (полный SHA, автор, дата, родители, число добавленных и удалённых строк, сообщение, ссылка) " +
	"и diff по файлам. Patch каждого файла ограничен 2000 символами, весь вывод — 15000. " +
	"Если patch у файла не показан, рядом указана причина (too_large, collapsed, только переименование, " +
	"только смена режима, пустой или бинарный файл) и как прочитать содержимое через get_file_contents. " +
	"page и per_page листают файлы diff."

const (
	// diffBodySlack is the room kept between the rendered diff and OutputBudget
	// for lines the renderer cannot predict.
	diffBodySlack = 300

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

// CommitIn is the input of get_commit.
type CommitIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	SHA     string `json:"sha" jsonschema:"commit SHA, branch or tag name"`
	Page    int    `json:"page,omitempty" jsonschema:"page of the file list of the diff, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"files per page, default 20, max 100"`
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

// getCommit returns the handler for the get_commit tool.
func getCommit(d Deps) func(ctx context.Context, in CommitIn) (string, error) {
	return func(ctx context.Context, in CommitIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		sha := strings.TrimSpace(in.SHA)
		if sha == "" {
			return "", errors.New("не указан sha")
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		const subject = "коммит (sha, ветка или тег)"
		c, _, err := d.GL.Commits.GetCommit(project, sha, nil, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject(subject, err)
		}
		files, resp, err := fetchCommitDiff(ctx, d, project, sha, page, perPage)
		if err != nil {
			return "", withSubject(subject, err)
		}

		header := commitHeader(c, len(files))
		body := "изменений в файлах нет (коммит без diff или страница за пределами списка)"
		if len(files) > 0 {
			oldRef := c.ID
			if len(c.ParentIDs) > 0 {
				oldRef = c.ParentIDs[0]
			}
			budget := OutputBudget - utf8.RuneCountInString(header) - diffBodySlack
			if budget < 0 {
				budget = 0
			}
			body = renderDiffFiles(files, c.ID, oldRef, budget)
		}

		out, truncated := Budget(header+"\n"+body, OutputBudget)
		var sb strings.Builder
		sb.WriteString(out)
		if truncated {
			sb.WriteString("\n")
			sb.WriteString(TruncatedFooter)
		}
		sb.WriteString("\n")
		sb.WriteString(PageFooter(page, perPage, resp))
		return sb.String(), nil
	}
}

// commitHeader renders the summary of a commit that precedes its diff.
// fileCount is the number of files on the current diff page.
func commitHeader(c *gitlab.Commit, fileCount int) string {
	date := "-"
	if c.AuthoredDate != nil {
		date = c.AuthoredDate.Format(time.RFC3339)
	}
	parents := "нет"
	if len(c.ParentIDs) > 0 {
		short := make([]string, len(c.ParentIDs))
		for i, p := range c.ParentIDs {
			short[i] = shortSHA(p)
		}
		parents = strings.Join(short, ", ")
	}

	lines := []string{
		"коммит " + c.ID,
		fmt.Sprintf("автор: %s, дата: %s", c.AuthorName, date),
		"родители: " + parents,
	}
	if c.Stats != nil {
		lines = append(lines, fmt.Sprintf("изменения: +%d/−%d", c.Stats.Additions, c.Stats.Deletions))
	}
	msg, cut := Budget(strings.TrimSpace(c.Message), commitMessageRunes)
	lines = append(lines, "сообщение:", msg)
	if cut {
		lines = append(lines, "[сообщение обрезано]")
	}
	if c.WebURL != "" {
		lines = append(lines, c.WebURL)
	}
	lines = append(lines, fmt.Sprintf("файлов на странице: %d", fileCount))
	return strings.Join(lines, "\n")
}

// shortSHA returns the first 8 characters of a commit ID.
func shortSHA(id string) string {
	r := []rune(id)
	if len(r) > 8 {
		return string(r[:8])
	}
	return id
}
