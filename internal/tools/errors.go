package tools

import (
	"errors"
	"fmt"
	"strings"

	"gitlab-mcp/internal/glclient"
)

// defaultRetryAfterSeconds is used when a 429 carries no usable Retry-After.
const defaultRetryAfterSeconds = 60

// defaultNotFoundSubject names what could be missing when a tool gives no
// more specific subject.
const defaultNotFoundSubject = "проект, путь или ref"

// subjectError attaches to an error what the failing request was looking for,
// so a bare 404 can be worded as "файл не найден" or "проект не найден".
type subjectError struct {
	subject string
	err     error
	// write marks an error from a state-changing request; op then names the
	// write operation so wording rules can key on it instead of the subject.
	write bool
	op    string
}

func (e *subjectError) Error() string { return e.err.Error() }
func (e *subjectError) Unwrap() error { return e.err }

// withSubject labels err with the object a request was about, for example
// "проект" or "файл". A nil err stays nil.
func withSubject(subject string, err error) error {
	if err == nil {
		return nil
	}
	return &subjectError{subject: subject, err: err}
}

// Write operation keys used by withWrite and by the write wording rules.
const (
	opCreateBranch = "create_branch"
	opCommit       = "commit"
)

// withWrite labels err as the failure of a state-changing request. op is the
// operation key (opCreateBranch, opCommit) and subject names what could be
// missing for a 404. A nil err stays nil.
func withWrite(op, subject string, err error) error {
	if err == nil {
		return nil
	}
	return &subjectError{subject: subject, err: err, write: true, op: op}
}

// toToolText turns any handler error into the short Russian message shown to
// the model. Raw response bodies are never echoed.
func toToolText(err error) string {
	subject := defaultNotFoundSubject
	var se *subjectError
	hasSubject := errors.As(err, &se)
	if hasSubject && se.subject != "" {
		subject = se.subject
	}

	e := glclient.Classify(err)
	if e == nil {
		return ""
	}

	if hasSubject && se.write {
		if text, ok := writeText(e, se.op, subject); ok {
			return text
		}
	}
	return baseText(e, subject)
}

// baseText is the generic wording of a classified failure, shared by read and
// write tools.
func baseText(e *glclient.Error, subject string) string {
	switch e.Kind {
	case glclient.KindUnauthorized:
		return "401: GitLab отклонил токен (недействителен, просрочен или отозван). Проверьте GITLAB_TOKEN."
	case glclient.KindForbidden:
		return "403: недостаточно прав: у токена нет нужного scope (read_api/api) или ваша роль в проекте не позволяет это действие."
	case glclient.KindNotFound:
		return fmt.Sprintf("404: не найдено (%s). GitLab также отвечает 404, если токен не видит приватный проект.", subject)
	case glclient.KindRateLimited:
		secs := int(e.RetryAfter.Seconds())
		if secs <= 0 {
			secs = defaultRetryAfterSeconds
		}
		return fmt.Sprintf("429: превышен лимит запросов GitLab, повторите через %d с.", secs)
	case glclient.KindServer:
		return fmt.Sprintf("%d: ошибка сервера GitLab, повторите позже.", e.Status)
	case glclient.KindBadRequest:
		return fmt.Sprintf("%d: GitLab отклонил запрос: %s", e.Status, capDetail(e.Detail))
	case glclient.KindNetwork:
		return "Не удалось связаться с GitLab: " + capDetail(e.Detail)
	case glclient.KindTimeout:
		return "Превышено время ожидания (25 с)."
	case glclient.KindCanceled:
		return "Вызов отменён."
	case glclient.KindTooLarge:
		return "Ответ GitLab слишком большой (больше 8 МБ) — объект слишком велик для чтения через API."
	case glclient.KindDecode:
		return "Неожиданный ответ GitLab (не удалось разобрать JSON)."
	}
	return capDetail(e.Detail)
}

// statusPrefix renders the real HTTP status GitLab answered with. GitLab maps
// several statuses (400, 409, 422, ...) to one failure kind, so the wording
// must not hard-code a code.
func statusPrefix(e *glclient.Error) string {
	if e.Status == 0 {
		return "400: "
	}
	return fmt.Sprintf("%d: ", e.Status)
}

// writeRule words one recognisable write failure. text carries no status
// digits (writeText prepends the real status); a "%s" in text is replaced by
// the capped GitLab detail.
type writeRule struct {
	kind glclient.Kind
	// op restricts the rule to one write operation; "" matches any.
	op string
	// substrings are lowercase; the rule matches when any is contained in the
	// lowercased GitLab detail.
	substrings []string
	text       string
}

// writeRules are consulted in order before the kind-level write wording.
//
// Rules keyed on an op only fire for that operation. The branch-exists rule
// is keyed on opCreateBranch, so a commit failure that says "already exists"
// about a file reaches the file rule below instead of being read as a branch
// clash. Rules without an op apply to every write and are matched by their
// specific phrases only.
var writeRules = []writeRule{
	{
		kind:       glclient.KindBadRequest,
		op:         opCreateBranch,
		substrings: []string{"already exists"},
		text:       "ветка уже существует: выберите другое имя или используйте существующую ветку.",
	},
	{
		kind:       glclient.KindBadRequest,
		op:         opCreateBranch,
		substrings: []string{"branch name is invalid"},
		text:       "недопустимое имя ветки: %s",
	},
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"invalid reference name", "ref is missing"},
		text:       "исходный ref не найден: укажите существующую ветку, тег или SHA. %s",
	},
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"not allowed to push"},
		text:       "ветка защищена: создайте ветку (create_branch), коммитьте туда, затем MR.",
	},
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"you can only create or edit files when you are on a branch"},
		text:       "ветка не найдена: создайте её через create_branch.",
	},
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"a file with this name already exists"},
		text:       "файл уже существует на ветке (для commit_files используйте action=update).",
	},
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"a file with this name doesn't exist", "a file with this name does not exist"},
		text:       "файла нет на ветке (для commit_files используйте action=create или проверьте путь).",
	},
}

// writeUnknownOutcome is appended to failures after which a write may still
// have been applied.
const writeUnknownOutcome = " Результат записи неизвестен: изменение могло быть применено. " +
	"Проверьте состояние (list_branches, list_commits) перед повтором."

// writeText words a failure of a state-changing request. It returns false when
// the generic wording of baseText should be used instead.
func writeText(e *glclient.Error, op, subject string) (string, bool) {
	detail := strings.ToLower(e.Detail)
	for _, r := range writeRules {
		if r.kind != e.Kind || (r.op != "" && r.op != op) {
			continue
		}
		for _, sub := range r.substrings {
			if !strings.Contains(detail, sub) {
				continue
			}
			text := r.text
			if strings.Contains(text, "%s") {
				text = fmt.Sprintf(text, capDetail(e.Detail))
			}
			return statusPrefix(e) + text, true
		}
	}

	switch e.Kind {
	case glclient.KindForbidden:
		return "403: запись отклонена: ветка может быть защищена, у токена может не быть scope `api`, " +
			"или ваша роль в проекте ниже Developer. " +
			"Создайте свою ветку (create_branch), коммитьте в неё, затем откройте MR.", true
	case glclient.KindUnauthorized:
		return baseText(e, subject) + " Для записи нужен токен со scope `api`.", true
	case glclient.KindServer, glclient.KindTimeout, glclient.KindNetwork:
		return baseText(e, subject) + writeUnknownOutcome, true
	}
	return "", false
}

// maxDetailRunes caps GitLab- or network-supplied text in a tool message.
const maxDetailRunes = 300

func capDetail(s string) string {
	r := []rune(s)
	if len(r) <= maxDetailRunes {
		return s
	}
	return string(r[:maxDetailRunes])
}
