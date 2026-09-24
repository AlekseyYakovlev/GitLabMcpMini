// Diff retrieval and rendering shared by get_commit and the other tools that
// show changes.
//
// The commit diff endpoint is decoded into diffFile instead of client-go's
// gitlab.Diff, because gitlab.Diff drops the collapsed and too_large flags.
// Those flags are the only honest explanation for a file whose patch is empty
// although it changed, so the reason for a missing patch is always stated.
package tools

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	// patchCapRunes caps the patch shown for one file.
	patchCapRunes = 2000

	// minPatchRunes is the smallest remaining budget worth spending on a patch.
	minPatchRunes = 200

	// commitMessageRunes caps the commit message in the get_commit header.
	commitMessageRunes = 1000

	// budgetExhaustedSuffix marks a file whose patch was skipped because the
	// output budget ran out.
	budgetExhaustedSuffix = " — патч не показан (бюджет вывода исчерпан)"

	// patchCutMarkerReserve is the room kept for the "[патч обрезан: ...]" line
	// when a patch may be cut.
	patchCutMarkerReserve = 80
)

// diffFile is one file entry of a GitLab diff, including the flags that
// gitlab.Diff does not carry.
type diffFile struct {
	Diff        string `json:"diff"`
	NewPath     string `json:"new_path"`
	OldPath     string `json:"old_path"`
	AMode       string `json:"a_mode"`
	BMode       string `json:"b_mode"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
	Collapsed   bool   `json:"collapsed"`
	TooLarge    bool   `json:"too_large"`
}

// fetchCommitDiff reads one page of the diff of a commit. sha may be a commit
// SHA, a branch or a tag. project must already be normalised. The request is a
// GET, so the shared client's retry policy and body cap apply unchanged.
func fetchCommitDiff(ctx context.Context, d Deps, project, sha string, page, perPage int) ([]diffFile, *gitlab.Response, error) {
	path := "projects/" + gitlab.PathEscape(project) + "/repository/commits/" + gitlab.PathEscape(sha) + "/diff"
	opts := &gitlab.GetCommitDiffOptions{
		ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}
	req, err := d.GL.NewRequest(http.MethodGet, path, opts, []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var files []diffFile
	resp, err := d.GL.Do(req, &files)
	if err != nil {
		return nil, resp, err
	}
	return files, resp, nil
}

// emptyPatchKind explains why a changed file came without a patch. withHint is
// false when the content is known not to have changed (rename only, mode only),
// so pointing at get_file_contents would be noise.
func emptyPatchKind(f diffFile) (reason string, withHint bool) {
	switch {
	case f.TooLarge:
		return "патч не показан: GitLab пометил файл как too_large", true
	case f.Collapsed:
		return "патч не показан: GitLab свернул diff (collapsed)", true
	case f.RenamedFile:
		return "только переименование, содержимое не менялось", false
	case !f.NewFile && !f.DeletedFile && f.AMode != f.BMode:
		return fmt.Sprintf("изменён только режим файла (%s→%s)", f.AMode, f.BMode), false
	case f.NewFile || f.DeletedFile:
		return "патч пуст: пустой или бинарный файл", true
	}
	return "патч не возвращён GitLab (вероятно, бинарный файл)", true
}

// emptyPatchReason is the explicit reason a file has no patch. An empty patch
// must never read as "no changes".
func emptyPatchReason(f diffFile) string {
	reason, _ := emptyPatchKind(f)
	return reason
}

// diffHeading renders the "### path [state]" line of a file.
func diffHeading(f diffFile) string {
	switch {
	case f.RenamedFile:
		return fmt.Sprintf("### %s → %s [renamed]", f.OldPath, f.NewPath)
	case f.NewFile:
		return fmt.Sprintf("### %s [new]", f.NewPath)
	case f.DeletedFile:
		return fmt.Sprintf("### %s [deleted]", f.OldPath)
	}
	return fmt.Sprintf("### %s [modified]", f.NewPath)
}

// emptyPatchHeading is the heading of a file without a patch: the reason and,
// where the content may have changed, how to read the file.
func emptyPatchHeading(f diffFile, newRef, oldRef string) string {
	reason, withHint := emptyPatchKind(f)
	h := diffHeading(f) + " — " + reason
	if !withHint {
		return h
	}
	path, ref := f.NewPath, newRef
	if f.DeletedFile {
		path = f.OldPath
		if oldRef != "" {
			ref = oldRef
		}
	}
	return fmt.Sprintf("%s; содержимое: get_file_contents path=%s ref=%s", h, path, ref)
}

// renderDiffFiles renders every file of a diff. Every file always gets a
// heading; patches are cut to patchCapRunes and to what is left of budget
// runes. The room for all headings is reserved first, so a late file is listed
// with "патч не показан" instead of vanishing when the budget runs out.
// newRef is the ref the changed files can be read at, oldRef the one for
// deleted files.
func renderDiffFiles(files []diffFile, newRef, oldRef string, budget int) string {
	headings := make([]string, len(files))
	reserve := 0
	for i, f := range files {
		if f.Diff == "" {
			headings[i] = emptyPatchHeading(f, newRef, oldRef)
		} else {
			headings[i] = diffHeading(f)
			reserve += utf8.RuneCountInString(budgetExhaustedSuffix)
		}
		reserve += utf8.RuneCountInString(headings[i]) + 1
	}
	remaining := budget - reserve

	blocks := make([]string, 0, len(files))
	for i, f := range files {
		if f.Diff == "" {
			blocks = append(blocks, headings[i])
			continue
		}
		if remaining < minPatchRunes {
			blocks = append(blocks, headings[i]+budgetExhaustedSuffix)
			continue
		}

		allowed := patchCapRunes
		if room := remaining - patchCutMarkerReserve; room < allowed {
			allowed = room
		}
		total := utf8.RuneCountInString(f.Diff)
		shown, cut := Budget(f.Diff, allowed)
		block := headings[i] + "\n" + shown
		if cut {
			block += fmt.Sprintf("\n[патч обрезан: показано %d из %d символов]", utf8.RuneCountInString(shown), total)
		}
		blocks = append(blocks, block)
		// The patch spends budget; the suffix reserved for this file is not needed.
		remaining -= utf8.RuneCountInString(block) - utf8.RuneCountInString(headings[i])
		remaining += utf8.RuneCountInString(budgetExhaustedSuffix)
	}
	return strings.Join(blocks, "\n")
}

// countPatchLines counts added and removed lines of a GitLab patch. GitLab
// patches start at the first "@@" hunk and carry no ---/+++ file headers, so
// every line starting with "+" or "-" is a change. "\ No newline at end of
// file" lines are ignored.
func countPatchLines(patch string) (added, removed int) {
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}
