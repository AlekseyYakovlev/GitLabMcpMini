package tools

import (
	"errors"
	"fmt"

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
	if errors.As(err, &se) && se.subject != "" {
		subject = se.subject
	}

	e := glclient.Classify(err)
	if e == nil {
		return ""
	}

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

// maxDetailRunes caps GitLab- or network-supplied text in a tool message.
const maxDetailRunes = 300

func capDetail(s string) string {
	r := []rune(s)
	if len(r) <= maxDetailRunes {
		return s
	}
	return string(r[:maxDetailRunes])
}
