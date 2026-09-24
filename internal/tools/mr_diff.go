package tools

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const getMergeRequestDiffsDescription = "Diff Merge Request по файлам: iid — номер после ! в GitLab. " +
	"Patch каждого файла ограничен 2000 символами, весь вывод — 15000. " +
	"Если patch у файла не показан, рядом указана причина (too_large, collapsed, только переименование, " +
	"только смена режима, пустой или бинарный файл) и как прочитать содержимое через get_file_contents. " +
	"Если в MR больше 1000 файлов, GitLab отдаёт не все, и вывод говорит «часть файлов не вернулась». " +
	"page и per_page листают файлы; подсказка внизу говорит, есть ли следующая страница."

const (
	// mrDiffPending is shown when the diff of a fresh MR is not computed yet.
	mrDiffPending = "diff ещё готовится, повторите через несколько секунд"

	// mrDiffEmpty is shown when a computed diff page holds no files.
	mrDiffEmpty = "изменений в файлах нет (страница за пределами списка или MR без diff)"
)

// MRDiffsIn is the input of get_merge_request_diffs.
type MRDiffsIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	IID     int    `json:"iid" jsonschema:"merge request IID, the number after ! in GitLab"`
	Page    int    `json:"page,omitempty" jsonschema:"page of files, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"files per page, default 20, max 100"`
}

// mrDiffToDiffFile adapts a merge request diff entry to the shared renderer.
func mrDiffToDiffFile(m *gitlab.MergeRequestDiff) diffFile {
	return diffFile{
		Diff:        m.Diff,
		NewPath:     m.NewPath,
		OldPath:     m.OldPath,
		AMode:       m.AMode,
		BMode:       m.BMode,
		NewFile:     m.NewFile,
		RenamedFile: m.RenamedFile,
		DeletedFile: m.DeletedFile,
		Collapsed:   m.Collapsed,
		TooLarge:    m.TooLarge,
	}
}

// getMergeRequestDiffs returns the handler for the get_merge_request_diffs tool.
func getMergeRequestDiffs(d Deps) func(ctx context.Context, in MRDiffsIn) (string, error) {
	return func(ctx context.Context, in MRDiffsIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		if err := checkIID(in.IID); err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		mr, _, err := d.GL.MergeRequests.GetMergeRequest(project, int64(in.IID), nil, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject(mrSubject, err)
		}
		opts := &gitlab.ListMergeRequestDiffsOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
		}
		diffs, resp, err := d.GL.MergeRequests.ListMergeRequestDiffs(project, int64(in.IID), opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject(mrSubject, err)
		}

		files := make([]diffFile, 0, len(diffs))
		for _, m := range diffs {
			if m != nil {
				files = append(files, mrDiffToDiffFile(m))
			}
		}

		header := mrDiffsHeader(mr, len(files))
		body := mrDiffEmpty
		if len(files) == 0 && strings.TrimSpace(mr.ChangesCount) == "" {
			body = mrDiffPending
		}
		if len(files) > 0 {
			// The SHAs are only known once GitLab has computed the diff refs.
			newRef := mr.SHA
			if newRef == "" {
				newRef = mr.SourceBranch
			}
			oldRef := mr.DiffRefs.BaseSha
			if oldRef == "" {
				oldRef = mr.TargetBranch
			}
			budget := OutputBudget - utf8.RuneCountInString(header) - diffBodySlack
			if budget < 0 {
				budget = 0
			}
			body = renderDiffFiles(files, newRef, oldRef, budget)
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

// mrDiffsHeader renders the lines that precede the diff of a merge request.
// fileCount is the number of files on the current page.
func mrDiffsHeader(mr *gitlab.MergeRequest, fileCount int) string {
	lines := []string{
		fmt.Sprintf("!%d %s", mr.IID, oneLine(mr.Title, mrTitleRunes)),
		"ветки: " + mr.SourceBranch + "→" + mr.TargetBranch,
		fmt.Sprintf("файлов на странице: %d", fileCount),
	}
	// GitLab has no overflow flag on /diffs; changes_count is capped at "1000+".
	if count := strings.TrimSpace(mr.ChangesCount); strings.HasSuffix(count, "+") {
		lines = append(lines, fmt.Sprintf("часть файлов не вернулась: в MR больше 1000 файлов (changes_count=%s); GitLab отдаёт не все файлы", count))
	}
	return strings.Join(lines, "\n")
}
