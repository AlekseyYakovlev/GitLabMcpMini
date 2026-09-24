package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const mergeMergeRequestDescription = "Вливает Merge Request. Сначала читает MR и проверяет detailed_merge_status: " +
	"если MR не открыт или слияние сейчас невозможно, вернёт объяснение и ничего не изменит " +
	"(статус и причины показывает get_merge_request). " +
	"Автослияние после завершения pipeline не поддерживается. " +
	"squash, remove_source_branch и merge_commit_message передаются в GitLab только если заданы, " +
	"иначе действуют настройки проекта; false не может выключить флаг, уже включённый на MR. " +
	"Слияние необратимо и никогда не повторяется автоматически; " +
	"если результат неизвестен, проверьте get_merge_request, прежде чем повторять. " +
	"Запись требует токен со scope api и роль, которой разрешено вливать в целевую ветку."

// MergeMRIn is the input of merge_merge_request.
type MergeMRIn struct {
	Project            string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	IID                int    `json:"iid" jsonschema:"merge request IID, the number after ! in GitLab"`
	Squash             bool   `json:"squash,omitempty" jsonschema:"true squashes the commits into one; omit to use the project setting"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty" jsonschema:"true asks GitLab to delete the source branch after the merge"`
	MergeCommitMessage string `json:"merge_commit_message,omitempty" jsonschema:"custom merge commit message; omit for the default"`
}

// mergeMergeRequest returns the handler for the merge_merge_request tool. The MR
// is read once and must be open and mergeable, otherwise nothing is written. The
// PUT is sent exactly once and never retried. No sha is sent, so the merge is
// not pinned to the head commit that was read.
func mergeMergeRequest(d Deps) func(ctx context.Context, in MergeMRIn) (string, error) {
	return func(ctx context.Context, in MergeMRIn) (string, error) {
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
		// The state comes first: a merged MR reports not_open as its merge status.
		if text, ok := stateAdvice(mr); ok {
			return "", errors.New(text)
		}
		if mr.DetailedMergeStatus != "mergeable" {
			return "", errors.New("слияние не выполнено: " + statusAdvice(mr))
		}

		opts := &gitlab.AcceptMergeRequestOptions{}
		if in.Squash {
			opts.Squash = gitlab.Ptr(true)
		}
		if in.RemoveSourceBranch {
			opts.ShouldRemoveSourceBranch = gitlab.Ptr(true)
		}
		if msg := strings.TrimSpace(in.MergeCommitMessage); msg != "" {
			opts.MergeCommitMessage = gitlab.Ptr(msg)
		}

		merged, _, err := d.GL.MergeRequests.AcceptMergeRequest(project, int64(in.IID), opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withWrite(opMergeMR, mrSubject, err)
		}
		if merged.IID == 0 {
			merged.IID = int64(in.IID)
		}
		return mergeSuccessText(merged, in.RemoveSourceBranch), nil
	}
}

// mergeSuccessText reports the merge outcome. removeRequested says whether the
// caller asked for the source branch to be removed; GitLab deletes it
// asynchronously, so the line only states that it was requested.
func mergeSuccessText(mr *gitlab.MergeRequest, removeRequested bool) string {
	head := fmt.Sprintf("MR !%d влит (state=merged)", mr.IID)
	if mr.State != "merged" {
		head = fmt.Sprintf("MR !%d: state=%s — слияние ещё не завершено; проверьте get_merge_request", mr.IID, mr.State)
	}

	commit := "merge commit: -"
	switch {
	case mr.MergeCommitSHA != "":
		commit = "merge commit: " + mr.MergeCommitSHA
	case mr.SquashCommitSHA != "":
		commit = "squash commit: " + mr.SquashCommitSHA
	case mr.SHA != "":
		commit = "head sha (fast-forward): " + mr.SHA
	}

	branch := "удаление ветки-источника: не запрошено"
	if removeRequested || mr.ShouldRemoveSourceBranch {
		branch = "удаление ветки-источника: запрошено"
	}

	lines := []string{head, commit, branch}
	if mr.WebURL != "" {
		lines = append(lines, mr.WebURL)
	}
	return strings.Join(lines, "\n")
}
