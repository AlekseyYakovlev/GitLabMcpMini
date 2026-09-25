package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const listBranchesDescription = "Ветки GitLab-проекта: одна строка на ветку в виде " +
	"«имя короткий-SHA [маркеры] заголовок последнего коммита». " +
	"Маркеры: [default] — ветка по умолчанию; [protected] — ветка может быть защищена, " +
	"писать в неё напрямую нельзя, коммитьте в свою ветку, созданную create_branch; " +
	"[merged] — ветка уже влита. " +
	"С search показываются только ветки, в имени которых есть эта подстрока. " +
	"Для следующей страницы вызовите снова с page из подсказки внизу."

const createBranchDescription = "Создаёт новую ветку в GitLab-проекте от ref " +
	"(коммит SHA, ветка или тег; по умолчанию — ветка по умолчанию проекта). " +
	"Если ветка с таким именем уже есть, вернётся ошибка и ничего не изменится. " +
	"В ответе: имя ветки, от чего создана, SHA конца ветки и ссылка. " +
	"Запись требует токен со scope api и роль Developer или выше. " +
	"Дальше коммитьте в созданную ветку и открывайте Merge Request."

// branchTitleRunes caps the commit title shown next to a branch.
const branchTitleRunes = 100

// BranchesIn is the input of list_branches.
type BranchesIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Search  string `json:"search,omitempty" jsonschema:"substring of the branch name to filter by"`
	Page    int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}

// CreateBranchIn is the input of create_branch.
type CreateBranchIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Branch  string `json:"branch" jsonschema:"name of the new branch, for example feature/x"`
	Ref     string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA to branch from; default the project's default branch"`
}

// listBranches returns the handler for the list_branches tool.
func listBranches(d Deps) func(ctx context.Context, in BranchesIn) (string, error) {
	return func(ctx context.Context, in BranchesIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		opts := &gitlab.ListBranchesOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
		}
		search := strings.TrimSpace(in.Search)
		if search != "" {
			opts.Search = gitlab.Ptr(search)
		}

		branches, resp, err := d.GL.Branches.ListBranches(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("проект", err)
		}

		header := "ветки " + project
		if search != "" {
			header += ", поиск: " + search
		}
		if len(branches) == 0 {
			return header + "\nветки не найдены", nil
		}

		lines := make([]string, 0, len(branches)+1)
		lines = append(lines, header)
		for _, b := range branches {
			lines = append(lines, branchLine(b))
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// branchLine renders one branch as "name shortSHA [default] [protected]
// [merged] title". A branch without commit data shows "-" instead of the SHA.
func branchLine(b *gitlab.Branch) string {
	sha := "-"
	title := ""
	if b.Commit != nil {
		if b.Commit.ShortID != "" {
			sha = b.Commit.ShortID
		}
		title = oneLine(b.Commit.Title, branchTitleRunes)
	}
	parts := []string{b.Name, sha}
	if b.Default {
		parts = append(parts, "[default]")
	}
	if b.Protected {
		parts = append(parts, "[protected]")
	}
	if b.Merged {
		parts = append(parts, "[merged]")
	}
	if title != "" {
		parts = append(parts, title)
	}
	return strings.Join(parts, " ")
}

// createBranch returns the handler for the create_branch tool. The POST is
// sent exactly once and never retried.
func createBranch(d Deps) func(ctx context.Context, in CreateBranchIn) (string, error) {
	return func(ctx context.Context, in CreateBranchIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		branch := strings.TrimSpace(in.Branch)
		if branch == "" {
			return "", errors.New("не указано имя ветки")
		}
		ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
		if err != nil {
			return "", err
		}

		b, _, err := d.GL.Branches.CreateBranch(project, &gitlab.CreateBranchOptions{
			Branch: gitlab.Ptr(branch),
			Ref:    gitlab.Ptr(ref),
		}, gitlab.WithContext(ctx))
		if err != nil {
			return "", withProject(project, withWrite(opCreateBranch, "проект или ветка", err))
		}

		tip := "-"
		if b.Commit != nil && b.Commit.ID != "" {
			tip = b.Commit.ID
		}
		return fmt.Sprintf("ветка %s создана от %s\nконец ветки: %s\n%s",
			b.Name, refLabel(ref, isDefault), tip, b.WebURL), nil
	}
}
