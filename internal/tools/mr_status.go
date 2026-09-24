package tools

import (
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// pendingAdvice is shared by the statuses that mean "GitLab has not finished
// checking yet".
const pendingAdvice = "проверка ещё идёт; повторите get_merge_request через несколько секунд"

// unknownStatusAdvice is shown for a detailed_merge_status this server has no
// wording for.
const unknownStatusAdvice = "неизвестный статус; проверьте MR в GitLab (web_url)"

// mergeStatusAdvice maps GitLab detailed_merge_status values to a Russian hint
// on what the agent should do next. It is shared by get_merge_request and the
// merge pre-check.
var mergeStatusAdvice = map[string]string{
	"mergeable":                      "можно вливать (merge_merge_request)",
	"checking":                       "GitLab проверяет возможность слияния: " + pendingAdvice,
	"unchecked":                      "проверка слияния ещё не выполнена: " + pendingAdvice,
	"preparing":                      "GitLab ещё готовит diff MR; повторите get_merge_request через несколько секунд",
	"approvals_syncing":              "аппрувы синхронизируются; повторите позже",
	"ci_still_running":               "pipeline ещё выполняется; дождитесь завершения и повторите (auto-merge не поддерживается)",
	"ci_must_pass":                   "нужен успешный pipeline: исправьте сбои CI или дождитесь его запуска",
	"commits_status":                 "в ветке-источнике нет коммитов или ветка не существует",
	"conflict":                       "конфликты с целевой веткой; разрешите их вне сервера (rebase или merge целевой ветки)",
	"need_rebase":                    "MR нужно перебазировать (инструмента rebase в сервере нет; сделайте это в GitLab)",
	"discussions_not_resolved":       "есть неразрешённые обсуждения; разрешите их в GitLab",
	"draft_status":                   "MR помечен как Draft; снимите пометку: update_merge_request с title без префикса «Draft:»",
	"not_approved":                   "нужен approve (approve в сервере не поддерживается, MRX-02); одобрите MR в GitLab",
	"requested_changes":              "ревьюер запросил изменения",
	"not_open":                       "MR не открыт; смотрите state (влит или закрыт); закрытый можно открыть: update_merge_request state_event=reopen",
	"merge_request_blocked":          "MR блокируется другим MR; сначала влейте блокирующий MR",
	"merge_time":                     "слияние запрещено до указанного времени (merge_after)",
	"status_checks_must_pass":        "должны пройти внешние status checks",
	"security_policy_pipeline_check": "требования политик безопасности к pipeline не выполнены; см. GitLab",
	"security_policy_violations":     "нарушены политики безопасности; см. GitLab",
	"jira_association_missing":       "требование проекта не выполнено: MR должен быть связан с задачей Jira; исправьте в GitLab",
	"title_regex":                    "требование проекта не выполнено: заголовок MR не подходит под шаблон; исправьте title через update_merge_request",
	"locked_paths":                   "требование проекта не выполнено: изменены заблокированные пути; см. GitLab",
	"locked_lfs_files":               "требование проекта не выполнено: заблокированы LFS-файлы; см. GitLab",
}

// statusAdvice renders the detailed_merge_status line: the raw value followed
// by the Russian advice.
func statusAdvice(mr *gitlab.MergeRequest) string {
	raw := mr.DetailedMergeStatus
	if raw == "" {
		return "detailed_merge_status: — статус слияния не вернулся; повторите get_merge_request"
	}
	advice, ok := mergeStatusAdvice[raw]
	if !ok {
		advice = unknownStatusAdvice
	}
	return "detailed_merge_status: " + raw + " — " + advice
}

// stateAdvice explains a merge request that is not open. ok is false for an
// open one. It is checked before the merge status because a merged MR reports
// not_open there.
func stateAdvice(mr *gitlab.MergeRequest) (string, bool) {
	switch mr.State {
	case "opened":
		return "", false
	case "merged":
		return "MR уже влит (state=merged)", true
	case "closed":
		return "MR закрыт (state=closed); открыть заново: update_merge_request state_event=reopen", true
	case "locked":
		return "MR заблокирован (state=locked), GitLab сейчас его обрабатывает; повторите позже", true
	default:
		return "MR в состоянии " + mr.State, true
	}
}
