package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const compareRefsDescription = "Сравнение двух веток, тегов или коммитов GitLab-проекта: " +
	"from — база, to — то, что с ней сравнивается; как в GitLab, сравнение идёт от общего предка (merge-base). " +
	"Вывод: коммиты (до 20 строк) и diff по файлам, то есть что принесёт ветка to относительно from — " +
	"удобно проверить перед Merge Request. " +
	"page и per_page листают файлы; patch каждого файла ограничен 2000 символами, весь вывод — 15000. " +
	"Если GitLab прервал сравнение по таймауту, об этом сказано явно."

// compareCommitLines is the number of commits listed before the rest is only counted.
const compareCommitLines = 20

// CompareIn is the input of compare_refs.
type CompareIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	From    string `json:"from" jsonschema:"base branch, tag or commit SHA"`
	To      string `json:"to" jsonschema:"branch, tag or commit SHA to compare with the base"`
	Page    int    `json:"page,omitempty" jsonschema:"page of the file list, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"files per page, default 20, max 100"`
}

// compareResult is the repository compare response. It is decoded into an own
// struct because gitlab.Compare drops the collapsed and too_large flags of the
// files, and those flags are the reason a changed file may come without a patch.
type compareResult struct {
	Commits        []*gitlab.Commit `json:"commits"`
	Diffs          []diffFile       `json:"diffs"`
	CompareTimeout bool             `json:"compare_timeout"`
	CompareSameRef bool             `json:"compare_same_ref"`
	WebURL         string           `json:"web_url"`
}

// fetchCompare reads the comparison of two refs. project must already be
// normalised. from and to travel only as query values.
func fetchCompare(ctx context.Context, d Deps, project, from, to string) (*compareResult, error) {
	path := "projects/" + gitlab.PathEscape(project) + "/repository/compare"
	opts := &gitlab.CompareOptions{From: gitlab.Ptr(from), To: gitlab.Ptr(to)}
	req, err := d.GL.NewRequest(http.MethodGet, path, opts, []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var res compareResult
	if _, err := d.GL.Do(req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// compareRefs returns the handler for the compare_refs tool.
func compareRefs(d Deps) func(ctx context.Context, in CompareIn) (string, error) {
	return func(ctx context.Context, in CompareIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		from, to := strings.TrimSpace(in.From), strings.TrimSpace(in.To)
		if from == "" || to == "" {
			return "", errors.New("укажите from и to")
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		res, err := fetchCompare(ctx, d, project, from, to)
		if err != nil {
			return "", withSubject("проект или ref (from/to)", err)
		}

		lines := []string{fmt.Sprintf("сравнение %s: %s → %s", project, from, to)}
		if res.CompareTimeout {
			lines = append(lines, "предупреждение: GitLab прервал сравнение по таймауту (compare_timeout), diff может быть неполным")
		}
		if res.CompareSameRef {
			lines = append(lines, "from и to указывают на один и тот же коммит: различий нет")
			return strings.Join(lines, "\n"), nil
		}

		lines = append(lines, fmt.Sprintf("коммитов: %d", len(res.Commits)))
		for i, c := range res.Commits {
			if i == compareCommitLines {
				lines = append(lines, fmt.Sprintf("(показаны первые %d из %d)", compareCommitLines, len(res.Commits)))
				break
			}
			lines = append(lines, commitLine(c))
		}

		total := len(res.Diffs)
		lo := (page - 1) * perPage
		hi := page * perPage
		if hi > total {
			hi = total
		}
		hasNext := hi < total

		var body string
		switch {
		case total == 0:
			lines = append(lines, "файлов: 0 (различий в файлах нет)")
		case lo >= total:
			lines = append(lines, fmt.Sprintf("файлов: %d", total),
				fmt.Sprintf("на странице %d файлов нет: всего файлов %d", page, total))
		default:
			lines = append(lines, fmt.Sprintf("файлов: %d, на странице: %d-%d", total, lo+1, hi))
			head := strings.Join(lines, "\n")
			budget := OutputBudget - utf8.RuneCountInString(head) - diffBodySlack
			if budget < 0 {
				budget = 0
			}
			body = renderDiffFiles(res.Diffs[lo:hi], to, from, budget)
		}

		text := strings.Join(lines, "\n")
		if body != "" {
			text += "\n" + body
		}
		out, truncated := Budget(text, OutputBudget)
		var sb strings.Builder
		sb.WriteString(out)
		if truncated {
			sb.WriteString("\n")
			sb.WriteString(TruncatedFooter)
		}
		sb.WriteString("\n")
		resp := &gitlab.Response{}
		if hasNext {
			resp.NextPage = int64(page + 1)
		}
		sb.WriteString(PageFooter(page, perPage, resp))
		return sb.String(), nil
	}
}
