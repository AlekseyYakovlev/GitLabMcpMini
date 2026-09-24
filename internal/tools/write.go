package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)

const commitFilesDescription = "Один коммит с несколькими файлами в существующую ветку " +
	"(создайте её через create_branch; в защищённую ветку не писать). " +
	"Действия: create, update, delete, move (для move обязателен previous_path — старый путь). " +
	"Только UTF-8 текст, до 50 файлов и до 1 МБ суммарно; content — полный новый текст файла, для create и update обязателен и не может быть пустым (пустой файл создать нельзя: передайте один перевод строки). " +
	"commit_files не проверяет конкурентные правки; для одного файла с проверкой " +
	"используйте create_or_update_file. Запрос не повторяется автоматически. " +
	"В ответе: SHA коммита (его можно передать в get_commit), +/- по файлам и ссылка. " +
	"Нужен токен со scope api."

const createOrUpdateFileDescription = "Создаёт или обновляет один текстовый файл одним коммитом в существующую ветку " +
	"(создайте её через create_branch). Сам определяет, создать файл или обновить, и пишет об этом " +
	"в ответе (created/updated). Защищает от перезаписи чужих правок: при обновлении передаётся " +
	"last_commit_id, прочитанный с этой же ветки; при конфликте перечитайте файл и повторите. " +
	"Содержимое пишется как есть; если в файле были CRLF, а в новом тексте их нет, будет предупреждение. " +
	"content не может быть пустым (пустой файл создать нельзя: передайте один перевод строки). " +
	"Для нескольких файлов используйте commit_files. Запрос не повторяется автоматически. " +
	"Нужен токен со scope api."

const crlfWarning = "предупреждение: концы строк изменились CRLF→LF (в прежней версии файла были CRLF)"

const (
	// maxCommitActions is the most file changes accepted in one commit.
	maxCommitActions = 50

	// maxCommitContentBytes is the most content bytes accepted in one commit.
	maxCommitContentBytes = 1 << 20

	// commitDiffPerPage is the page size of the follow-up diff read; it covers
	// maxCommitActions files in one page.
	commitDiffPerPage = 100

	// statUnknown stands in for "+a/−r" when GitLab did not report the lines.
	statUnknown = "+?/−?"
)

// Actions accepted by commit_files.
const (
	actCreate = "create"
	actUpdate = "update"
	actDelete = "delete"
	actMove   = "move"
)

// ActionIn is one file change of commit_files.
type ActionIn struct {
	Action       string `json:"action"`
	FilePath     string `json:"file_path"`
	Content      string `json:"content,omitempty"`
	PreviousPath string `json:"previous_path,omitempty"`
}

// CommitFilesIn is the input of commit_files.
type CommitFilesIn struct {
	Project       string     `json:"project"`
	Branch        string     `json:"branch"`
	CommitMessage string     `json:"commit_message"`
	Actions       []ActionIn `json:"actions"`
}

// fileAction is one validated file change ready to be sent to GitLab. It is
// shared by commit_files and the single-file tools.
type fileAction struct {
	Action       string
	Path         string
	PreviousPath string
	Content      string
	LastCommitID string
	// sendContent says whether Content goes into the request. create and
	// update always send it (an empty content is rejected before that); for
	// move an empty content means "keep the file content" and is not sent.
	sendContent bool
}

// toFileAction validates one ActionIn and converts it. i is the position in
// actions[] used in messages.
func toFileAction(i int, a ActionIn) (fileAction, error) {
	action := strings.ToLower(strings.TrimSpace(a.Action))
	switch action {
	case actCreate, actUpdate, actDelete, actMove:
	default:
		return fileAction{}, fmt.Errorf("actions[%d]: неизвестное действие %q: допустимы create, update, delete, move", i, a.Action)
	}

	path, err := glclient.NormalizeRepoPath(a.FilePath)
	if err != nil {
		return fileAction{}, fmt.Errorf("actions[%d]: %w", i, err)
	}
	if path == "" {
		return fileAction{}, fmt.Errorf("actions[%d]: не указан file_path", i)
	}

	fa := fileAction{Action: action, Path: path, Content: a.Content}

	prev := strings.TrimSpace(a.PreviousPath)
	switch {
	case action == actMove:
		prev, err = glclient.NormalizeRepoPath(prev)
		if err != nil {
			return fileAction{}, fmt.Errorf("actions[%d]: previous_path: %w", i, err)
		}
		if prev == "" {
			return fileAction{}, fmt.Errorf("actions[%d]: для move нужен previous_path — старый путь файла", i)
		}
		if prev == path {
			return fileAction{}, fmt.Errorf("actions[%d]: previous_path совпадает с file_path", i)
		}
		fa.PreviousPath = prev
	case prev != "":
		return fileAction{}, fmt.Errorf("actions[%d]: previous_path нужен только для move", i)
	}

	switch action {
	case actDelete:
		if a.Content != "" {
			return fileAction{}, fmt.Errorf("actions[%d]: для delete content не нужен", i)
		}
	case actCreate, actUpdate:
		if a.Content == "" {
			return fileAction{}, fmt.Errorf("actions[%d]: для %s нужен непустой content (пустой файл создать нельзя: передайте один перевод строки)", i, action)
		}
		fa.sendContent = true
	case actMove:
		fa.sendContent = a.Content != ""
	}
	return fa, nil
}

// validateCommitInput checks everything that can be checked without a request.
func validateCommitInput(branch, message string, acts []fileAction) error {
	if strings.TrimSpace(branch) == "" {
		return errors.New("не указана ветка")
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("commit_message не может быть пустым")
	}
	if len(acts) == 0 {
		return errors.New("actions: нужен хотя бы один файл")
	}
	if len(acts) > maxCommitActions {
		return fmt.Errorf("actions: не больше %d файлов в одном коммите", maxCommitActions)
	}

	total := 0
	for i, a := range acts {
		if !a.sendContent {
			continue
		}
		total += len(a.Content)
		if !utf8.ValidString(a.Content) || strings.ContainsRune(a.Content, 0) {
			return fmt.Errorf("actions[%d]: content содержит NUL или не UTF-8 — бинарные файлы не поддерживаются, только UTF-8 текст", i)
		}
	}
	if total > maxCommitContentBytes {
		return errors.New("суммарный размер content больше 1 МБ")
	}
	return nil
}

// commitCore sends one POST /repository/commits with the given actions. It is
// never retried; a failure is wrapped as a write failure of opCommit.
func commitCore(ctx context.Context, d Deps, project, branch, message string, acts []fileAction) (*gitlab.Commit, error) {
	opts := &gitlab.CreateCommitOptions{
		Branch:        gitlab.Ptr(branch),
		CommitMessage: gitlab.Ptr(message),
		Actions:       make([]*gitlab.CommitActionOptions, 0, len(acts)),
	}
	for _, a := range acts {
		o := &gitlab.CommitActionOptions{
			Action:   gitlab.Ptr(gitlab.FileActionValue(a.Action)),
			FilePath: gitlab.Ptr(a.Path),
		}
		if a.sendContent {
			o.Content = gitlab.Ptr(a.Content)
		}
		if a.PreviousPath != "" {
			o.PreviousPath = gitlab.Ptr(a.PreviousPath)
		}
		if a.LastCommitID != "" {
			o.LastCommitID = gitlab.Ptr(a.LastCommitID)
		}
		opts.Actions = append(opts.Actions, o)
	}

	commit, _, err := d.GL.Commits.CreateCommit(project, opts, gitlab.WithContext(ctx))
	if err != nil {
		return nil, withWrite(opCommit, "проект или ветка", err)
	}
	return commit, nil
}

// UpsertFileIn is the input of create_or_update_file.
type UpsertFileIn struct {
	Project       string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Path          string `json:"path" jsonschema:"file path inside the repository"`
	Content       string `json:"content" jsonschema:"full new UTF-8 text content of the file; must not be empty (empty files are not supported, pass one newline)"`
	Branch        string `json:"branch" jsonschema:"existing branch to commit to; create it first with create_branch"`
	CommitMessage string `json:"commit_message" jsonschema:"commit message, not empty"`
}

// createOrUpdateFile returns the handler for the create_or_update_file tool.
// The file is read on the target branch to decide between create and update
// (an update carries the read last_commit_id so a concurrent change is
// rejected by GitLab); the commit POST is sent exactly once and never retried.
func createOrUpdateFile(d Deps) func(ctx context.Context, in UpsertFileIn) (string, error) {
	return func(ctx context.Context, in UpsertFileIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		path, err := glclient.NormalizeRepoPath(in.Path)
		if err != nil {
			return "", err
		}
		if path == "" {
			return "", errors.New("не указан путь к файлу")
		}
		if in.Content == "" {
			return "", errors.New("нужен непустой content (пустой файл создать нельзя: передайте один перевод строки)")
		}
		branch := strings.TrimSpace(in.Branch)
		fa := fileAction{Path: path, Content: in.Content, sendContent: true}
		if err := validateCommitInput(branch, in.CommitMessage, []fileAction{fa}); err != nil {
			return "", err
		}

		var warning string
		f, _, err := d.GL.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(branch)}, gitlab.WithContext(ctx))
		switch {
		case err == nil:
			fa.Action = actUpdate
			fa.LastCommitID = f.LastCommitID
			raw, decErr := decodeFileContent(f)
			if decErr != nil {
				return "", decErr
			}
			if !isBinary(raw) && bytes.Contains(raw, []byte("\r\n")) && !strings.Contains(in.Content, "\r\n") {
				warning = crlfWarning
			}
		case glclient.Classify(err).Kind == glclient.KindNotFound:
			fa.Action = actCreate
		default:
			return "", withSubject("файл", err)
		}

		commit, err := commitCore(ctx, d, project, branch, in.CommitMessage, []fileAction{fa})
		if err != nil {
			return "", err
		}

		verb := "created"
		if fa.Action == actUpdate {
			verb = "updated"
		}
		short := commit.ShortID
		if short == "" {
			short = shortSHA(commit.ID)
		}
		lines := []string{fmt.Sprintf("%s %s: коммит %s в %s", verb, path, short, branch)}
		if warning != "" {
			lines = append(lines, warning)
		}
		lines = append(lines, commit.WebURL)
		return strings.Join(lines, "\n"), nil
	}
}

// fileStat is the number of added and removed lines of one file of a commit.
// known is false when GitLab did not report them.
type fileStat struct {
	added, removed int
	known          bool
}

// perFileStats counts changed lines per file of a commit diff, keyed by the
// new path (and the old path for deleted files).
//
// A too_large or collapsed file has unknown counts. Any other file without a
// patch (rename only, empty new file, mode-only change, empty deleted file)
// changed no lines, so an empty patch alone never makes the counts unknown.
func perFileStats(files []diffFile) map[string]fileStat {
	stats := make(map[string]fileStat, len(files))
	for _, f := range files {
		var st fileStat
		switch {
		case f.TooLarge || f.Collapsed:
		case f.Diff != "":
			st.added, st.removed = countPatchLines(f.Diff)
			st.known = true
		default:
			st.known = true
		}
		key := f.NewPath
		if f.DeletedFile && f.OldPath != "" {
			key = f.OldPath
		}
		stats[key] = st
	}
	return stats
}

// pluralFiles renders a Russian-pluralised file count.
func pluralFiles(n int) string {
	word := "файлов"
	if m := n % 100; m < 11 || m > 14 {
		switch n % 10 {
		case 1:
			word = "файл"
		case 2, 3, 4:
			word = "файла"
		}
	}
	return fmt.Sprintf("%d %s", n, word)
}

// statText renders "+a/−r" or "+?/−?".
func statText(s fileStat, ok bool) string {
	if !ok || !s.known {
		return statUnknown
	}
	return fmt.Sprintf("+%d/−%d", s.added, s.removed)
}

// commitFiles returns the handler for the commit_files tool. The commit POST is
// sent exactly once; the diff read that follows it is a plain GET whose failure
// never turns a successful commit into an error.
func commitFiles(d Deps) func(ctx context.Context, in CommitFilesIn) (string, error) {
	return func(ctx context.Context, in CommitFilesIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		acts := make([]fileAction, 0, len(in.Actions))
		for i, a := range in.Actions {
			fa, err := toFileAction(i, a)
			if err != nil {
				return "", err
			}
			acts = append(acts, fa)
		}
		branch := strings.TrimSpace(in.Branch)
		if err := validateCommitInput(branch, in.CommitMessage, acts); err != nil {
			return "", err
		}

		commit, err := commitCore(ctx, d, project, branch, in.CommitMessage, acts)
		if err != nil {
			return "", err
		}

		var stats map[string]fileStat
		files, _, diffErr := fetchCommitDiff(ctx, d, project, commit.ID, 1, commitDiffPerPage)
		if diffErr == nil {
			stats = perFileStats(files)
		}

		short := commit.ShortID
		if short == "" {
			short = shortSHA(commit.ID)
		}
		head := fmt.Sprintf("коммит %s в %s", short, branch)
		if commit.Stats != nil {
			head += fmt.Sprintf(" (+%d/−%d)", commit.Stats.Additions, commit.Stats.Deletions)
		}
		head += ": " + pluralFiles(len(acts))

		lines := make([]string, 0, len(acts)+3)
		lines = append(lines, head)
		for _, a := range acts {
			label := a.Path
			if a.Action == actMove {
				label = a.PreviousPath + " → " + a.Path
			}
			st, ok := stats[a.Path]
			lines = append(lines, fmt.Sprintf("%s %s (%s)", a.Action, label, statText(st, ok)))
		}
		if diffErr != nil {
			lines = append(lines, "статистика по файлам недоступна: см. get_commit sha="+commit.ID)
		}
		lines = append(lines, commit.WebURL)
		return strings.Join(lines, "\n"), nil
	}
}
