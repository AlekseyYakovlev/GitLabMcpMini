package tools

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const listMergeRequestNotesDescription = "Заметки (обсуждение) Merge Request: iid — номер после ! в GitLab. " +
	"Все заметки, новые первыми. Системные события GitLab (коммит добавлен, ветка обновлена, " +
	"статус изменён) помечены [system] и занимают одну короткую строку. " +
	"Заметка человека выводится как «#id @автор дата» и текст, текст длиннее 1000 символов обрезается. " +
	"Для длинного обсуждения уменьшите per_page. " +
	"Для следующей страницы вызовите снова с page из подсказки внизу."

const (
	// noteBodyRunes caps the body of one user note.
	noteBodyRunes = 1000

	// noteSystemRunes caps the body of one system note.
	noteSystemRunes = 120
)

// MRNotesIn is the input of list_merge_request_notes.
type MRNotesIn struct {
	Project string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	IID     int    `json:"iid" jsonschema:"merge request IID, the number after ! in GitLab"`
	Page    int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"notes per page, default 20, max 100"`
}

// listMergeRequestNotes returns the handler for the list_merge_request_notes
// tool. Order is left to GitLab, which sorts by created_at descending.
func listMergeRequestNotes(d Deps) func(ctx context.Context, in MRNotesIn) (string, error) {
	return func(ctx context.Context, in MRNotesIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		if err := checkIID(in.IID); err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)

		opts := &gitlab.ListMergeRequestNotesOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
		}
		notes, resp, err := d.GL.Notes.ListMergeRequestNotes(project, int64(in.IID), opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject(mrSubject, err)
		}

		header := fmt.Sprintf("заметки MR !%d проекта %s (новые первыми)", in.IID, project)
		if len(notes) == 0 {
			return header + "\nзаметок нет", nil
		}

		lines := make([]string, 0, len(notes)+1)
		lines = append(lines, header)
		for _, n := range notes {
			if n != nil {
				lines = append(lines, noteLines(n))
			}
		}
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}

// noteLines renders one note. A system note is a single flattened line marked
// [system]; a user note is a "#id @user date" heading followed by its body.
func noteLines(n *gitlab.Note) string {
	if n.System {
		return "[system] " + oneLine(n.Body, noteSystemRunes)
	}
	date := "-"
	if n.CreatedAt != nil {
		date = n.CreatedAt.Format(commitDateLayout)
	}
	author := n.Author.Username
	if author == "" {
		author = "-"
	}
	out := fmt.Sprintf("#%d @%s %s", n.ID, author, date)
	body, cut := Budget(strings.TrimSpace(n.Body), noteBodyRunes)
	if body != "" {
		out += "\n" + body
	}
	if cut {
		out += "\n[заметка обрезана]"
	}
	return out
}
