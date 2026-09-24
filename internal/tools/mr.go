package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const listMergeRequestsDescription = "Merge Request'ы GitLab-проекта: одна строка на MR " +
	"«!iid, state, [draft], заголовок, ветка-источник→целевая ветка, @автор». " +
	"Без фильтров показываются только открытые MR (state=opened). " +
	"state: opened, closed, merged или all. " +
	"Фильтры: source_branch и target_branch — точные имена веток; author_username — логин автора; " +
	"search — текст в заголовке и описании. " +
	"Детали одного MR, включая возможность слияния, показывает get_merge_request. " +
	"Для следующей страницы вызовите снова с page из подсказки внизу."

const getMergeRequestDescription = "Один Merge Request GitLab-проекта по iid (номер после ! в GitLab): " +
	"заголовок, state, draft, автор, ветки, описание (до 1500 символов), признак конфликтов, " +
	"статус pipeline, число изменённых файлов и ссылка. " +
	"Строка detailed_merge_status показывает, можно ли вливать MR, и что делать, если нельзя. " +
	"Проверка слияния в GitLab асинхронная: сразу после создания MR статус checking или unchecked " +
	"означает «проверка идёт», повторите вызов через несколько секунд. " +
	"Вызывайте перед merge_merge_request. Diff показывает get_merge_request_diffs."

const (
	// mrTitleRunes caps the MR title shown in a list line.
	mrTitleRunes = 120

	// mrDescriptionRunes caps the MR description shown by get_merge_request.
	mrDescriptionRunes = 1500

	// mrSubject is the 404 subject of every iid-based call: GitLab answers 404
	// both for a missing MR and for a project the token cannot see.
	mrSubject = "MR (iid) или проект"
)

// mrStates are the state filter values list_merge_requests accepts. GitLab also
// knows "locked", which is not exposed.
var mrStates = map[string]bool{"opened": true, "closed": true, "merged": true, "all": true}

// ListMRsIn is the input of list_merge_requests.
type ListMRsIn struct {
	Project        string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	State          string `json:"state,omitempty" jsonschema:"opened (default), closed, merged or all"`
	SourceBranch   string `json:"source_branch,omitempty" jsonschema:"only MRs from this source branch"`
	TargetBranch   string `json:"target_branch,omitempty" jsonschema:"only MRs into this target branch"`
	AuthorUsername string `json:"author_username,omitempty" jsonschema:"GitLab username of the MR author"`
	Search         string `json:"search,omitempty" jsonschema:"text to search in title and description"`
	Page           int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage        int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}

// GetMRIn is the input of get_merge_request.
type GetMRIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	IID     int    `json:"iid" jsonschema:"merge request IID, the number after ! in GitLab"`
}

// checkIID rejects a merge request number that cannot exist, before any request.
func checkIID(iid int) error {
	if iid <= 0 {
		return errors.New("не указан iid MR (целое число > 0, номер после ! в GitLab)")
	}
	return nil
}

// listMergeRequests returns the handler for the list_merge_requests tool.
func listMergeRequests(d Deps) func(ctx context.Context, in ListMRsIn) (string, error) {
	return func(ctx context.Context, in ListMRsIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		state := strings.TrimSpace(in.State)
		if state == "" {
			state = "opened"
		}
		if !mrStates[state] {
			return "", fmt.Errorf("state: неверное значение %q, допустимо: opened, closed, merged, all", state)
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		// GitLab lists all states when state is omitted, so it is always sent.
		opts := &gitlab.ListProjectMergeRequestsOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			State:       gitlab.Ptr(state),
		}
		header := fmt.Sprintf("MR проекта %s, state=%s", project, state)
		if v := strings.TrimSpace(in.SourceBranch); v != "" {
			opts.SourceBranch = gitlab.Ptr(v)
			header += ", source_branch=" + v
		}
		if v := strings.TrimSpace(in.TargetBranch); v != "" {
			opts.TargetBranch = gitlab.Ptr(v)
			header += ", target_branch=" + v
		}
		if v := strings.TrimSpace(in.AuthorUsername); v != "" {
			opts.AuthorUsername = gitlab.Ptr(v)
			header += ", автор=" + v
		}
		if v := strings.TrimSpace(in.Search); v != "" {
			opts.Search = gitlab.Ptr(v)
			header += ", поиск: " + v
		}

		mrs, resp, err := d.GL.MergeRequests.ListProjectMergeRequests(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("проект", err)
		}
		if len(mrs) == 0 {
			return header + "\nMR не найдены", nil
		}

		lines := make([]string, 0, len(mrs)+1)
		lines = append(lines, header)
		for _, m := range mrs {
			lines = append(lines, mrLine(m))
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// mrLine renders one merge request as "!iid state [draft] title source→target
// @author". A merge request without an author shows "@-".
func mrLine(m *gitlab.BasicMergeRequest) string {
	parts := []string{fmt.Sprintf("!%d", m.IID), m.State}
	if m.Draft {
		parts = append(parts, "[draft]")
	}
	author := "-"
	if m.Author != nil && m.Author.Username != "" {
		author = m.Author.Username
	}
	parts = append(parts,
		oneLine(m.Title, mrTitleRunes),
		m.SourceBranch+"→"+m.TargetBranch,
		"@"+author,
	)
	return strings.Join(parts, " ")
}

// getMergeRequest returns the handler for the get_merge_request tool. It sends
// exactly one request: an unfinished merge check is reported, never polled.
func getMergeRequest(d Deps) func(ctx context.Context, in GetMRIn) (string, error) {
	return func(ctx context.Context, in GetMRIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		if err := checkIID(in.IID); err != nil {
			return "", err
		}

		mr, _, err := d.GL.MergeRequests.GetMergeRequest(project, int64(in.IID), nil, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject(mrSubject, err)
		}

		out, truncated := Budget(strings.Join(mrHeader(mr), "\n"), OutputBudget)
		if truncated {
			out += "\n" + TruncatedFooter
		}
		return out, nil
	}
}

// mrHeader renders the lines of the get_merge_request answer.
func mrHeader(mr *gitlab.MergeRequest) []string {
	state := "state: " + mr.State
	if mr.Draft {
		state += " [draft]"
	}
	author := "-"
	if mr.Author != nil && mr.Author.Username != "" {
		author = mr.Author.Username
	}
	lines := []string{
		fmt.Sprintf("!%d %s", mr.IID, oneLine(mr.Title, mrTitleRunes)),
		state,
		"автор: @" + author,
		"ветки: " + mr.SourceBranch + "→" + mr.TargetBranch,
	}
	if advice, ok := stateAdvice(mr); ok {
		lines = append(lines, advice)
	}
	lines = append(lines, statusAdvice(mr))

	// has_conflicts is only reliable once the merge check has finished.
	conflicts := "нет"
	if mr.HasConflicts {
		conflicts = "да"
	}
	if mr.DetailedMergeStatus == "checking" || mr.DetailedMergeStatus == "unchecked" {
		conflicts += " (ещё не проверено)"
	}
	lines = append(lines, "признак конфликтов: "+conflicts)

	if mr.HeadPipeline != nil && mr.HeadPipeline.Status != "" {
		lines = append(lines, "pipeline: "+mr.HeadPipeline.Status)
	} else {
		lines = append(lines, "pipeline: нет")
	}
	if count := strings.TrimSpace(mr.ChangesCount); count != "" {
		lines = append(lines, "файлов: "+count)
	} else {
		lines = append(lines, "число файлов: ещё не известно")
	}

	if desc := strings.TrimSpace(mr.Description); desc != "" {
		body, cut := Budget(desc, mrDescriptionRunes)
		lines = append(lines, "описание:", body)
		if cut {
			lines = append(lines, "[описание обрезано]")
		}
	}
	if mr.WebURL != "" {
		lines = append(lines, mr.WebURL)
	}
	return lines
}
