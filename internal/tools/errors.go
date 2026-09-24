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
	opCreateMR     = "create_merge_request"
	opUpdateMR     = "update_merge_request"
	opMergeMR      = "merge_merge_request"
	opMRNote       = "create_merge_request_note"
)

// withWrite labels err as the failure of a state-changing request. op is the
// operation key (opCreateBranch, opCommit, opCreateMR, ...) and subject names what could be
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
	// status restricts the rule to one HTTP status; 0 matches any status.
	status int
	// substrings are lowercase; the rule matches when any is contained in the
	// lowercased GitLab detail. A rule without substrings matches on kind, op
	// and status alone.
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
		kind:   glclient.KindBadRequest,
		op:     opMergeMR,
		status: 405,
		text:   "слияние отклонено: MR сейчас нельзя влить (состояние изменилось после проверки или MR уже влит/закрыт). Вызовите get_merge_request.",
	},
	{
		kind:   glclient.KindBadRequest,
		op:     opMergeMR,
		status: 406,
		text:   "слияние отклонено: MR сейчас нельзя влить (состояние изменилось после проверки или MR уже влит/закрыт). Вызовите get_merge_request.",
	},
	{
		kind:   glclient.KindBadRequest,
		op:     opMergeMR,
		status: 409,
		text:   "ветка-источник изменилась после проверки; вызовите get_merge_request и повторите слияние осознанно.",
	},
	{
		kind:       glclient.KindBadRequest,
		op:         opMergeMR,
		substrings: []string{"branch cannot be merged"},
		text:       "GitLab не смог влить MR (вероятно конфликт или изменилась целевая ветка). Вызовите get_merge_request.",
	},
	{
		kind:       glclient.KindBadRequest,
		op:         opMergeMR,
		substrings: []string{"sha must be provided"},
		text:       "в группе или инстансе включено требование sha при merge; этот инструмент sha не передаёт (MRX-03, v2). Влейте MR в GitLab.",
	},
	{
		kind: glclient.KindUnauthorized,
		op:   opMergeMR,
		text: "нет прав вливать этот MR (роль ниже Developer/Maintainer или защита целевой ветки).",
	},
	{
		kind: glclient.KindForbidden,
		op:   opMergeMR,
		text: "нет прав на слияние: целевая ветка защищена или ваша роль не позволяет вливать; у токена должен быть scope api.",
	},
	{
		kind:       glclient.KindBadRequest,
		op:         opCreateMR,
		substrings: []string{"you must select different branches", "same project/branch", "same branch"},
		text:       "ветка-источник совпадает с целевой: укажите другую target_branch.",
	},
	{
		kind:       glclient.KindBadRequest,
		op:         opCreateMR,
		substrings: []string{"does not exist"},
		text:       "ветка не найдена: проверьте source_branch/target_branch (list_branches). %s",
	},
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
	{
		kind:       glclient.KindBadRequest,
		substrings: []string{"changed since you started editing"},
		text:       "файл изменился с момента чтения — прочитайте его заново (get_file_contents) и повторите.",
	},
}

// writeUnknownOutcome is appended to failures after which a write may still
// have been applied. unknownOutcomeHint adds where to check.
const writeUnknownOutcome = " Результат записи неизвестен: изменение могло быть применено. "

// unknownOutcomeHint says where to look before repeating a write whose outcome
// is unknown.
func unknownOutcomeHint(op string) string {
	switch op {
	case opCreateMR:
		return "Проверьте list_merge_requests (source_branch=<ветка>) перед повтором."
	case opUpdateMR, opMergeMR:
		return "Проверьте состояние через get_merge_request перед повтором."
	case opMRNote:
		return "Проверьте list_merge_request_notes перед повтором."
	}
	return "Проверьте состояние (list_branches, list_commits) перед повтором."
}

// ruleMatches reports whether r words the failure e of write operation op.
func ruleMatches(r writeRule, e *glclient.Error, op, detail string) bool {
	if r.kind != e.Kind || (r.op != "" && r.op != op) || (r.status != 0 && r.status != e.Status) {
		return false
	}
	if len(r.substrings) == 0 {
		return true
	}
	for _, sub := range r.substrings {
		if strings.Contains(detail, sub) {
			return true
		}
	}
	return false
}

// writeText words a failure of a state-changing request. It returns false when
// the generic wording of baseText should be used instead.
func writeText(e *glclient.Error, op, subject string) (string, bool) {
	detail := strings.ToLower(e.Detail)
	for _, r := range writeRules {
		if !ruleMatches(r, e, op, detail) {
			continue
		}
		text := r.text
		if strings.Contains(text, "%s") {
			text = fmt.Sprintf(text, capDetail(e.Detail))
		}
		return statusPrefix(e) + text, true
	}

	switch e.Kind {
	case glclient.KindForbidden:
		return "403: запись отклонена: ветка может быть защищена, у токена может не быть scope `api`, " +
			"или ваша роль в проекте ниже Developer. " +
			"Создайте свою ветку (create_branch), коммитьте в неё, затем откройте MR.", true
	case glclient.KindUnauthorized:
		return baseText(e, subject) + " Для записи нужен токен со scope `api`.", true
	case glclient.KindServer:
		// Not baseText: "повторите позже" would invite a blind retry of a write.
		return fmt.Sprintf("%d: ошибка сервера GitLab.", e.Status) + writeUnknownOutcome + unknownOutcomeHint(op), true
	case glclient.KindTimeout, glclient.KindNetwork, glclient.KindCanceled,
		glclient.KindDecode, glclient.KindTooLarge, glclient.KindOther:
		return baseText(e, subject) + writeUnknownOutcome + unknownOutcomeHint(op), true
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
