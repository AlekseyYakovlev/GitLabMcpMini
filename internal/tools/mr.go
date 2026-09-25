package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

const createMergeRequestDescription = "Создаёт Merge Request из source_branch в target_branch " +
	"(по умолчанию — ветка по умолчанию проекта). " +
	"draft=true помечает MR как Draft через префикс «Draft:» в заголовке. " +
	"Вернётся ошибка, если из этой ветки уже есть открытый MR или между ветками нет различий " +
	"(сначала закоммитьте изменения через commit_files). " +
	"Строки описания, начинающиеся с «/», GitLab выполняет как quick actions. " +
	"Сразу после создания статус слияния обычно checking: перед merge_merge_request вызовите get_merge_request. " +
	"Запрос никогда не повторяется автоматически. " +
	"Запись требует токен со scope api и роль Developer или выше."

const updateMergeRequestDescription = "Изменяет Merge Request: title, description, target_branch, " +
	"state_event, draft. Меняются только непустые поля; пустое значение означает «не менять», " +
	"поэтому описание нельзя очистить, а флаги нельзя сбросить пустым значением. " +
	"state_event=close закрывает MR, reopen открывает закрытый заново. " +
	"draft=true добавляет к заголовку префикс «Draft:»; чтобы снять Draft, передайте title без этого префикса " +
	"(draft=false означает «не передано»). " +
	"Запрос никогда не повторяется автоматически. " +
	"Запись требует токен со scope api и роль Developer или выше."

// checkBeforeMergeHint reminds the agent that a fresh MR has no merge verdict yet.
const checkBeforeMergeHint = "статус слияния обычно ещё checking: перед merge_merge_request вызовите get_merge_request"

// mrRefRe finds the "!N" merge request reference in a GitLab message.
var mrRefRe = regexp.MustCompile(`!(\d+)`)

// draftPrefix matches a title GitLab already treats as a draft.
var draftPrefix = regexp.MustCompile(`(?i)^\s*(\[draft\]|\(draft\)|draft:)`)

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

// CreateMRIn is the input of create_merge_request.
type CreateMRIn struct {
	Project      string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	SourceBranch string `json:"source_branch" jsonschema:"branch with your changes"`
	Title        string `json:"title" jsonschema:"merge request title"`
	TargetBranch string `json:"target_branch,omitempty" jsonschema:"branch to merge into; default the project's default branch"`
	Description  string `json:"description,omitempty" jsonschema:"merge request description in Markdown"`
	Draft        bool   `json:"draft,omitempty" jsonschema:"true marks the MR as Draft (title prefix Draft:)"`
}

// UpdateMRIn is the input of update_merge_request.
type UpdateMRIn struct {
	Project      string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	IID          int    `json:"iid" jsonschema:"merge request IID, the number after ! in GitLab"`
	Title        string `json:"title,omitempty" jsonschema:"new title; empty means unchanged"`
	Description  string `json:"description,omitempty" jsonschema:"new description in Markdown; empty means unchanged"`
	TargetBranch string `json:"target_branch,omitempty" jsonschema:"new target branch; empty means unchanged"`
	StateEvent   string `json:"state_event,omitempty" jsonschema:"close or reopen"`
	Draft        bool   `json:"draft,omitempty" jsonschema:"true marks the MR as Draft; to remove Draft send title without the Draft: prefix"`
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

// withDraftPrefix marks title as a draft. GitLab REST has no draft field, so the
// title prefix is the only way; a title that already carries one stays as is.
func withDraftPrefix(title string) string {
	if draftPrefix.MatchString(title) {
		return title
	}
	return "Draft: " + title
}

// createMergeRequest returns the handler for the create_merge_request tool. The
// POST is sent exactly once and never retried.
func createMergeRequest(d Deps) func(ctx context.Context, in CreateMRIn) (string, error) {
	return func(ctx context.Context, in CreateMRIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		source := strings.TrimSpace(in.SourceBranch)
		if source == "" {
			return "", errors.New("не указана ветка-источник (source_branch)")
		}
		title := strings.TrimSpace(in.Title)
		if title == "" {
			return "", errors.New("не указан заголовок MR (title)")
		}
		errSameBranch := errors.New("ветка-источник совпадает с целевой: укажите другую target_branch")
		explicitTarget := strings.TrimSpace(in.TargetBranch)
		if explicitTarget != "" && explicitTarget == source {
			return "", errSameBranch
		}

		target, isDefault, err := resolveRef(ctx, d, project, explicitTarget)
		if err != nil {
			return "", err
		}
		if target == source {
			return "", errSameBranch
		}

		// GitLab creates an empty Draft MR for a branch without new commits, so
		// the absence of a difference is checked before anything is written.
		res, err := fetchCompare(ctx, d, project, target, source)
		if err != nil {
			return "", withSubject("ветка или проект", err)
		}
		if !res.CompareTimeout && len(res.Commits) == 0 {
			return "", fmt.Errorf("нет изменений между ветками: в %s нет коммитов, которых нет в %s. "+
				"Сначала закоммитьте изменения (commit_files), затем создайте MR.", source, target)
		}

		if in.Draft {
			title = withDraftPrefix(title)
		}
		opts := &gitlab.CreateMergeRequestOptions{
			SourceBranch: gitlab.Ptr(source),
			TargetBranch: gitlab.Ptr(target),
			Title:        gitlab.Ptr(title),
		}
		if desc := strings.TrimSpace(in.Description); desc != "" {
			opts.Description = gitlab.Ptr(desc)
		}

		mr, _, err := d.GL.MergeRequests.CreateMergeRequest(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", createMRError(ctx, d, project, source, err)
		}

		draft := "нет"
		if mr.Draft {
			draft = "да"
		}
		lines := []string{
			fmt.Sprintf("MR !%d создан: %s→%s", mr.IID, source, refLabel(target, isDefault)),
			"draft: " + draft,
			statusAdvice(mr),
			checkBeforeMergeHint,
		}
		if mr.WebURL != "" {
			lines = append(lines, mr.WebURL)
		}
		return strings.Join(lines, "\n"), nil
	}
}

// createMRError words a failed create request. GitLab answers 409 when an open
// merge request from the same branch exists; that is reported as an error that
// names the existing MR, never as success. Every other failure goes through the
// write wording, which flags an unknown outcome where the MR may exist.
func createMRError(ctx context.Context, d Deps, project, source string, err error) error {
	e := glclient.Classify(err)
	if e == nil || e.Status != 409 || !strings.Contains(strings.ToLower(e.Detail), "another open merge request already exists") {
		return withProject(project, withWrite(opCreateMR, "проект или ветка", err))
	}

	ref := "открытый MR из этой ветки уже есть (номер не удалось определить; см. list_merge_requests)"
	if m := mrRefRe.FindStringSubmatch(e.Detail); m != nil {
		ref = "открытый MR уже есть: !" + m[1]
	} else {
		// One read after the failed write finds the existing MR; the POST itself
		// is never repeated.
		mrs, _, lookupErr := d.GL.MergeRequests.ListProjectMergeRequests(project, &gitlab.ListProjectMergeRequestsOptions{
			ListOptions:  gitlab.ListOptions{PerPage: 1},
			State:        gitlab.Ptr("opened"),
			SourceBranch: gitlab.Ptr(source),
		}, gitlab.WithContext(ctx))
		if lookupErr == nil && len(mrs) > 0 {
			ref = fmt.Sprintf("открытый MR уже есть: !%d", mrs[0].IID)
			if mrs[0].WebURL != "" {
				ref += " (" + mrs[0].WebURL + ")"
			}
		}
	}
	return errors.New(ref + ". Используйте его (get_merge_request) или закройте.")
}

// updateMergeRequest returns the handler for the update_merge_request tool. Only
// non-empty fields are sent, so nothing is blanked by accident. The PUT is sent
// exactly once and never retried.
func updateMergeRequest(d Deps) func(ctx context.Context, in UpdateMRIn) (string, error) {
	return func(ctx context.Context, in UpdateMRIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		if err := checkIID(in.IID); err != nil {
			return "", err
		}
		title := strings.TrimSpace(in.Title)
		description := strings.TrimSpace(in.Description)
		target := strings.TrimSpace(in.TargetBranch)
		stateEvent := strings.TrimSpace(in.StateEvent)
		if stateEvent != "" && stateEvent != "close" && stateEvent != "reopen" {
			return "", errors.New("state_event: допустимо close или reopen")
		}
		if title == "" && description == "" && target == "" && stateEvent == "" && !in.Draft {
			return "", errors.New("нечего менять: передайте title, description, target_branch, state_event или draft=true")
		}

		// A Draft MR without a new title needs the current one to prefix.
		var current *gitlab.MergeRequest
		if in.Draft {
			if title != "" {
				title = withDraftPrefix(title)
			} else {
				current, _, err = d.GL.MergeRequests.GetMergeRequest(project, int64(in.IID), nil, gitlab.WithContext(ctx))
				if err != nil {
					return "", withSubject(mrSubject, err)
				}
				if !current.Draft && !draftPrefix.MatchString(current.Title) {
					title = withDraftPrefix(current.Title)
				}
			}
		}

		opts := &gitlab.UpdateMergeRequestOptions{}
		var changed []string
		if title != "" {
			opts.Title = gitlab.Ptr(title)
			changed = append(changed, "title")
		}
		if description != "" {
			opts.Description = gitlab.Ptr(description)
			changed = append(changed, "description")
		}
		if target != "" {
			opts.TargetBranch = gitlab.Ptr(target)
			changed = append(changed, "target_branch")
		}
		if stateEvent != "" {
			opts.StateEvent = gitlab.Ptr(stateEvent)
			changed = append(changed, "state_event")
		}
		if in.Draft && opts.Title != nil {
			changed = append(changed, "draft")
		}

		if len(changed) == 0 {
			out := fmt.Sprintf("MR !%d уже помечен как Draft; изменений нет", in.IID)
			if current != nil && current.WebURL != "" {
				out += "\n" + current.WebURL
			}
			return out, nil
		}

		mr, _, err := d.GL.MergeRequests.UpdateMergeRequest(project, int64(in.IID), opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withProject(project, withWrite(opUpdateMR, mrSubject, err))
		}

		draft := "нет"
		if mr.Draft {
			draft = "да"
		}
		lines := []string{
			fmt.Sprintf("MR !%d обновлён: %s", in.IID, strings.Join(changed, ", ")),
			"state: " + mr.State,
			"draft: " + draft,
			"ветки: " + mr.SourceBranch + "→" + mr.TargetBranch,
		}
		if mr.WebURL != "" {
			lines = append(lines, mr.WebURL)
		}
		return strings.Join(lines, "\n"), nil
	}
}
