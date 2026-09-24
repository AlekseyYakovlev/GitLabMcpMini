# Phase 3: Merge Requests - Research

**Researched:** 2026-09-24
**Domain:** GitLab REST v4 Merge Requests API driven through `client-go/v2` v2.64.0, exposed as 8 MCP tools in an existing Go stdio server
**Confidence:** HIGH (API behaviour checked against GitLab's own docs and `lib/api/merge_requests.rb` source on master, and against the `client-go/v2` v2.64.0 source in the local module cache)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** `list_merge_requests` без фильтров возвращает только `state=opened`, все авторы, в рамках проекта. Фильтры: `state` (`opened`/`closed`/`merged`/`all`), `source_branch`, `target_branch`, `author_username`, `search`, плюс `page`/`per_page` (футер пагинации как в Phase 1). Параметра `scope` нет.
- **D-02:** Строка списка — одна строка на MR: `!iid`, state, пометка draft, заголовок, `source→target`, автор. Формат в стиле `list_commits`/`list_branches`.
- **D-03:** `get_merge_request` возвращает короткую шапку: `iid`, заголовок, state, draft, автор, `source→target`, описание (до ~1500 символов с явной пометкой обрезки), признак конфликтов, статус head-pipeline, `web_url` и число изменённых файлов, если оно есть в ответе. Reviewers, assignees, labels, milestone, approvals не показываются.
- **D-04:** `detailed_merge_status` показывается как сырое значение плюс русское пояснение «что делать». Одна общая таблица значение→пояснение (`mergeable`, `ci_still_running`, `ci_must_pass`, `not_approved`, `draft_status`, `conflict`, `need_rebase`, `discussions_not_resolved`, `checking`, `unchecked` и др.) используется и в `get_merge_request`, и в предпроверке `merge_merge_request`. Неизвестное значение выводится как есть с общей подсказкой.
- **D-05:** Статус `checking`/`unchecked` (проверка GitLab асинхронная, часто сразу после создания MR) сообщается честно: «проверка ещё идёт, повторите через несколько секунд». Никакого ожидания и опроса внутри `get_merge_request` и `create_merge_request`, ровно один HTTP-запрос.
- **D-06:** `get_merge_request_diffs` использует тот же рендер, что `get_commit` из Phase 2 (`renderDiffFiles`, D-10/D-11 Phase 2): patch до ~2000 символов на файл, список файлов всегда полный, причины пустого patch (`collapsed`, `too_large`, бинарный, только переименование) указаны явно. Пагинация файлов на стороне GitLab (`page`/`per_page`, эндпоинт `/merge_requests/:iid/diffs`). Флаг `overflow` выводится отдельной строкой «часть файлов не вернулась». Отдельного `path`-фильтра и повышенного лимита patch нет.
- **D-07:** `list_merge_request_notes` показывает все заметки в одном списке, системные помечены `[system]` и выводятся короткой строкой (MR-04). Порядок «новые первыми» (desc, как у GitLab), `page`/`per_page` с футером. Тело заметки обрезается до ~1000 символов с пометкой, чтобы одна огромная заметка не вытесняла остальные.
- **D-08:** `create_merge_request`: обязательны `source_branch` и `title`; `target_branch` по умолчанию ветка проекта (как `resolveRef` в Phase 2, ветка указывается в выводе); опциональны `description` и `draft`. Параметров `squash`, `remove_source_branch`, assignee/reviewer/labels в create нет. Запись не повторяется автоматически.
- **D-09:** Дубль или пустой diff в `create_merge_request` — понятная ошибка, ничего не меняется молча. Если открытый MR из этой ветки уже есть (GitLab 409), сообщение «открытый MR уже есть: !N» со ссылкой (iid из ответа GitLab или поиском по `source_branch`). Если ветки не отличаются, сообщение «нет изменений между ветками» с подсказкой про `commit_files`. Идемпотентного «вернуть существующий как успех» нет.
- **D-10:** `update_merge_request` принимает `title`, `description`, `target_branch`, `state_event` (`close`/`reopen`) и `draft` (bool). Непустое значение меняет поле, пустое означает «не менять»; очистка описания и снятие флагов через пустое значение не поддерживаются, это документируется в описании инструмента. Если ничего не передано, ошибка до обращения к GitLab. Поля простые, без указателей (D-15 Phase 1).
- **D-11:** `create_merge_request_note` — общий (не inline) комментарий: `project`, `iid`, `body`. Пустой `body` отклоняется до запроса. Успешный ответ: id заметки, `web_url` MR.
- **D-12:** Предпроверка: сначала GET MR; если `detailed_merge_status` не `mergeable`, возвращается `isError` с русским пояснением из общей таблицы (D-04), и запрос слияния не отправляется. Слияние — единственный PUT, не повторяется автоматически. Auto-merge («влить после успешного pipeline») не поддерживается.
- **D-13:** Параметры слияния: `squash`, `remove_source_branch`, `merge_commit_message`. Не переданные не отправляются в GitLab, действуют настройки проекта/MR. `false` означает «не передано» (явно выключить уже включённый флаг нельзя, документируется). `sha`-защита остаётся в v2 (MRX-03).
- **D-14:** Отказы GitLab 405/406/409 при самом PUT (состояние изменилось между GET и PUT, конфликт, MR закрыт) мапятся в понятные тексты по образцу `writeRules` в `errors.go`. Успешный ответ: `iid`, state (`merged`), SHA слияния, ветка удалена или нет, `web_url`.
- **D-15:** Бэклог 999.2 берётся в Phase 3: в ветку `writeUnknownOutcome` (`internal/tools/errors.go`) добавляются виды сбоев Canceled/Decode/TooLarge/Other для записей. Для MR-записей (особенно merge) подсказка: «проверьте состояние через `get_merge_request` перед повтором». Позиция 999.2 в `ROADMAP.md` помечается как закрытая в Phase 3.
- **D-16 (уже зафиксировано):** Из Phase 1–2 действуют: имена без префикса; компактный текстовый вывод одной строкой на элемент; футеры пагинации и обрезки; бюджет вывода ~15 000 символов; ошибки как `isError` на русском; запросы записи (POST/PUT) не повторяются автоматически; `project` — числовой ID или путь; плоские схемы без `$ref`/`anyOf`/`null`, без указателей; имена ≤30, описания ≤900 символов; токен нигде не выводится; `mcp.ToolAnnotations` (чтение — `ReadOnlyHint`, запись — `DestructiveHint`; `merge_merge_request` и `update_merge_request` как destructive на усмотрение планировщика).

### Claude's Discretion

- Точные значения: лимит описания MR (~1500), тела заметки (~1000), `per_page` по умолчанию (~20), формат строк вывода и точные русские тексты таблицы `detailed_merge_status`.
- Как получать число файлов и `overflow` для diff (поля ответа `/diffs` или заголовки), и как искать существующий открытый MR при 409.
- Раскладка файлов в `internal/tools/` (по образцу `commits.go`, `write.go`, `diff.go`) и структура общей таблицы статусов.
- Что именно вернуть при `draft` в `update_merge_request` (механика префикса `Draft:` или отдельное поле client-go).

### Deferred Ideas (OUT OF SCOPE)

- Inline-комментарии к строкам diff (MRX-01), `approve_merge_request` (MRX-02), защита merge через `sha` (MRX-03) — v2.
- Auto-merge при идущем pipeline (`merge_when_pipeline_succeeds`) — потребовал бы отслеживания «запланированного» состояния; вне «Mini».
- Очистка `description` и снятие булевых флагов через update/merge (нужны указатели или отдельные флаги) — вернуться вместе с бэклогом 999.1, если понадобится.
- Assignee/reviewer/labels/milestone в create/get, `scope` в списке MR, `path`-фильтр для diff — при необходимости позже.
- `delete_branch` (WRT-04) — v2; после merge ветку убирает `remove_source_branch`.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| MR-01 | `list_merge_requests` с фильтрами state/ветки | `Client.MergeRequests.ListProjectMergeRequests` + `ListProjectMergeRequestsOptions` (`State`, `SourceBranch`, `TargetBranch`, `AuthorUsername`, `Search`). GitLab's own default is `state=all`, so `opened` must be sent explicitly (Finding F3). |
| MR-02 | `get_merge_request` с `detailed_merge_status` | `MergeRequests.GetMergeRequest`; status table (Finding F11, "detailed_merge_status table"); `changes_count` is a string, empty on fresh MRs (F7). |
| MR-03 | `get_merge_request_diffs` постранично, флаги явно | `MergeRequests.ListMergeRequestDiffs` returns `MergeRequestDiff` WITH `Collapsed`/`TooLarge` (unlike `gitlab.Diff`); adapter into `diffFile` then `renderDiffFiles`. `overflow` does not exist on `/diffs` (F1): use `changes_count` ending in `+`. |
| MR-04 | `list_merge_request_notes`, системные помечены | `Notes.ListMergeRequestNotes` (`Note.System`, `Note.Author.Username`); server default order is `created_at desc` (D-07 matches, send nothing or `sort=desc` explicitly). |
| MR-05 | `create_merge_request` | `MergeRequests.CreateMergeRequest`; no `draft` REST parameter, use title prefix (F2); empty-diff branches are NOT rejected by GitLab, pre-check with existing `fetchCompare` (F4); 409 duplicate carries `!N` in the message (F5). |
| MR-06 | `create_merge_request_note` | `Notes.CreateMergeRequestNote`; quick actions in body are executed by GitLab (F8). |
| MR-07 | `merge_merge_request` с проверкой статуса | GET pre-check, then `MergeRequests.AcceptMergeRequest` (`PUT .../merge`). Real failure statuses: 400 (SHA required), 401 (no merge permission, not a bad token), 405, 409 (SHA mismatch), 422 `Branch cannot be merged` (F6). |
| MR-08 | `update_merge_request` | `MergeRequests.UpdateMergeRequest`; `UpdateMergeRequestOptions` has NO `Draft` field (F2); needs a prior GET only for `draft` without `title`. |
</phase_requirements>

## Summary

Phase 3 adds 8 tools (12 to 20) on top of a finished Phase 1-2 foundation. No new dependency is required: everything is available in `client-go/v2` v2.64.0, which is already in `go.mod` [VERIFIED: local module cache `client-go/v2@v2.64.0/merge_requests.go`, `notes.go`]. The work is thin handlers plus three shared pieces: a `detailed_merge_status` table, an MR-aware extension of the write error mapper (`errors.go`, including backlog 999.2), and an adapter from `MergeRequestDiff` to the existing `diffFile`/`renderDiffFiles`.

Checking the locked decisions against GitLab's actual behaviour turned up five places where a naive implementation of CONTEXT.md would be wrong or impossible as worded. They are recorded as Findings F1-F12 below and each has a prescriptive resolution that stays inside the locked decisions: (1) `/diffs` has no `overflow` field, so the "overflow" line must be derived from `changes_count` (`"1000+"`); (2) GitLab REST has no `draft` parameter for create/update, draft is a title prefix; (3) GitLab defaults the MR list to `state=all` while D-01 wants `opened`; (4) GitLab silently creates an MR (auto-marked Draft) for branches with no commits, so D-09's "нет изменений между ветками" needs a client-side pre-check; (5) merge failure codes are 400/401/405/409/422 (not 406), and a 401 on merge means "no permission to merge", not "bad token".

**Primary recommendation:** Build eight thin handlers in new files `mr.go`, `mr_diff.go`, `mr_notes.go`, `mr_merge.go`, `mr_status.go`; add MR ops (`opCreateMR`, `opUpdateMR`, `opMergeMR`, `opMRNote`) and a `status` field to `writeRule` in `errors.go`; reuse `renderDiffFiles`, `composeList`, `resolveRef`, `fetchCompare` unchanged; drive every tool through the fake-GitLab in-memory MCP harness.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| MR list/get/diff/notes (read) | MCP tool layer (`internal/tools`) | GitLab REST (source of truth) | Handlers normalise input, call client-go, format bounded text |
| Merge gating (`detailed_merge_status` pre-check) | MCP tool layer | GitLab REST (final authority: 405/422) | Pre-check is advisory UX; GitLab still enforces at PUT |
| Draft handling | MCP tool layer (title prefix) | GitLab (interprets prefix) | REST has no draft field |
| Error wording for MR writes | `internal/tools/errors.go` | `internal/glclient` (Classify: kind/status/detail) | Wording is tool-specific, classification is generic |
| Retry policy (never retry POST/PUT) | `internal/glclient` (already GET-only) | — | No change needed, verified by Phase 2 tests |
| Output budget/pagination | `internal/tools/format.go` | — | Reuse `Budget`, `composeList`, `PageFooter` |

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `gitlab.com/gitlab-org/api/client-go/v2` | v2.64.0 (already in go.mod) | Typed MR/Notes calls | Already the project client; has every method needed [VERIFIED: module cache source] |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 (already in go.mod) | `mcp.AddTool`, in-memory transports for tests | Project standard |

No new packages. Nothing to install.

### client-go v2.64.0 methods to use (all verified in source)

| Tool | Call | Options type (key fields, all pointers) | Returns |
|------|------|------------------------------------------|---------|
| list_merge_requests | `d.GL.MergeRequests.ListProjectMergeRequests(project, opts, ...)` | `ListProjectMergeRequestsOptions{ListOptions, State, SourceBranch, TargetBranch, AuthorUsername, Search}` | `[]*BasicMergeRequest`, `*Response` |
| get_merge_request | `MergeRequests.GetMergeRequest(project, iid int64, nil, ...)` | `GetMergeRequestsOptions` (leave nil) | `*MergeRequest` (embeds `BasicMergeRequest`; has `ChangesCount string`, `HeadPipeline *Pipeline`, `DiffRefs`, `HasConflicts`, `Draft`, `DetailedMergeStatus`, `SHA`) |
| get_merge_request_diffs | `MergeRequests.ListMergeRequestDiffs(project, iid, &ListMergeRequestDiffsOptions{ListOptions}, ...)` | `Unidiff` left nil | `[]*MergeRequestDiff{OldPath,NewPath,AMode,BMode,Diff,NewFile,RenamedFile,DeletedFile,GeneratedFile,Collapsed,TooLarge}` |
| list_merge_request_notes | `Notes.ListMergeRequestNotes(project, iid, &ListMergeRequestNotesOptions{ListOptions, OrderBy, Sort}, ...)` | `OrderBy`/`Sort` optional | `[]*Note{ID, Body, System, Author.Username, CreatedAt, Position}` |
| create_merge_request | `MergeRequests.CreateMergeRequest(project, &CreateMergeRequestOptions{Title, Description, SourceBranch, TargetBranch}, ...)` | | `*MergeRequest` |
| create_merge_request_note | `Notes.CreateMergeRequestNote(project, iid, &CreateMergeRequestNoteOptions{Body}, ...)` | | `*Note` |
| update_merge_request | `MergeRequests.UpdateMergeRequest(project, iid, &UpdateMergeRequestOptions{Title, Description, TargetBranch, StateEvent}, ...)` | NO `Draft` field | `*MergeRequest` |
| merge_merge_request | `MergeRequests.AcceptMergeRequest(project, iid, &AcceptMergeRequestOptions{Squash, ShouldRemoveSourceBranch, MergeCommitMessage}, ...)` | note the name `ShouldRemoveSourceBranch` (tool arg stays `remove_source_branch`); `SHA` exists but is deferred (MRX-03) | `*MergeRequest` |

`iid` is `int64` in client-go; the tool input should be a plain `int` field (`json:"iid"`), validated `> 0` before any request, converted with `int64(in.IID)`.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Typed `ListMergeRequestDiffs` + adapter to `diffFile` | Raw `NewRequest`/`Do` decoding straight into `[]diffFile` (the `fetchCommitDiff` pattern) | In v2.64.0 `MergeRequestDiff` already carries `Collapsed`/`TooLarge`, so the typed call is enough; the raw pattern is only needed where `gitlab.Diff` drops flags (commit diff). Raw decode is an acceptable fallback if a field is missing. |
| `GetMergeRequestChanges` (has `overflow`) | — | Deprecated, scheduled for removal in API v5 [CITED: docs.gitlab.com/api/merge_requests/#retrieve-merge-request-changes]. Do not use. |

## Package Legitimacy Audit

No external packages are added by this phase (`go.mod` unchanged). slopcheck not applicable.

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
MCP client (agent)  --tools/call-->  go-sdk (schema validation, unknown args rejected)
                                        |
                                   safe(d, handler)  25 s timeout, panic recover, token redaction
                                        |
        +-------------------------------+--------------------------------------------+
        | normalise: NormalizeProject, iid > 0, enum/empty checks (Russian errors, no request)
        v
  READ tools                         WRITE tools (never retried by glclient)
  list_mr -> ListProjectMRs          create_mr:  [resolveRef target] -> fetchCompare(target,source)
  get_mr  -> GetMR  -> status table                 -> POST MRs ; 409 -> parse "!N" or list(source_branch)
  mr_diffs-> GetMR + ListMRDiffs     update_mr:  [GET MR only if draft && no title] -> PUT MR
            -> adapter -> renderDiffFiles       note:       POST notes (empty body rejected locally)
  mr_notes-> ListMRNotes             merge_mr:   GET MR -> state/status gate (status table, no PUT if blocked)
        |                                          -> PUT .../merge -> success text | mapped 400/401/405/409/422
        v                                        errors: withWrite(opXxx, subject, err) -> toToolText -> writeText
  composeList / Budget / PageFooter      unknown outcome (5xx/timeout/network/canceled/decode/toolarge/other)
        |                                 -> "Результат записи неизвестен ... get_merge_request"
        v
  text result (isError false)  or  short Russian isError message
```

### Recommended Project Structure

```
internal/tools/
├── mr.go            # list_merge_requests, get_merge_request, create_merge_request, update_merge_request + In structs, line/header renderers, draft helpers
├── mr_status.go     # detailed_merge_status table + statusAdvice(status, state) used by get and merge
├── mr_diff.go       # get_merge_request_diffs + adapter mrDiffToDiffFile
├── mr_notes.go      # list_merge_request_notes, create_merge_request_note
├── mr_merge.go      # merge_merge_request (pre-check + PUT + success text)
├── errors.go        # + opCreateMR/opUpdateMR/opMergeMR/opMRNote, writeRule.status, MR rules, 999.2
├── register.go      # + 8 AddTool
└── *_test.go        # one test file per new source file
```
(One concern per file, lowercase names, `<name>_test.go`; matches Phase 1-2 convention.) Also touched: `internal/tools/testdata/tools_list.golden.json` (regenerate with `-update`), `scripts/smoke.py` (`EXPECTED_TOOLS` 12 to 20), `e2e/python_smoke_test.go` (fake routes), `.planning/ROADMAP.md` (close 999.2).

### Pattern 1: Thin handler (unchanged from Phase 1-2)
**What:** `func name(d Deps) func(ctx, in T) (string, error)`; normalise, one client-go call with `gitlab.WithContext(ctx)`, `withSubject`/`withWrite` on error, format via `composeList`/`Budget`. Register with `safe(d, handler)`.
**Source:** `internal/tools/commits.go` (`listCommits`, `getCommit`), `branch.go` (`createBranch`).

### Pattern 2: get_merge_request_diffs = GET MR + GET diffs (two reads)
The MR read supplies (a) 404 wording "MR не найден", (b) refs for `renderDiffFiles` hints (`newRef = mr.SHA`, fallback `mr.SourceBranch`; `oldRef = mr.DiffRefs.BaseSha`, fallback `mr.TargetBranch`), (c) `changes_count` for the overflow line, (d) a way to tell "diff not ready yet" from "no changes".
```go
// Source: client-go v2.64.0 MergeRequestDiff (verified) + internal/tools/diff.go
func mrDiffToDiffFile(m *gitlab.MergeRequestDiff) diffFile {
	return diffFile{
		Diff: m.Diff, NewPath: m.NewPath, OldPath: m.OldPath, AMode: m.AMode, BMode: m.BMode,
		NewFile: m.NewFile, RenamedFile: m.RenamedFile, DeletedFile: m.DeletedFile,
		Collapsed: m.Collapsed, TooLarge: m.TooLarge,
	}
}
// overflow surrogate (F1): changes_count is a string; GitLab caps it at 1000 and returns "1000+"
overflow := strings.HasSuffix(strings.TrimSpace(mr.ChangesCount), "+")
```
Output order (same as `getCommit`): header (`!iid title`, `source→target`, `файлов на странице: N`), optional line `часть файлов не вернулась: в MR больше 1000 файлов (changes_count=1000+)`, body from `renderDiffFiles(files, newRef, oldRef, OutputBudget-len(header)-diffBodySlack)`, `TruncatedFooter` if cut, `PageFooter`. Empty page: if `len(files)==0` and (`mr.ChangesCount == ""` or status is `preparing`) say "diff ещё готовится, повторите через несколько секунд", otherwise "изменений в файлах нет (страница за пределами списка или MR без diff)". An empty patch must never read as "no changes" (already enforced per file by `emptyPatchKind`).

### Pattern 3: Draft via title prefix (F2)
```go
// GitLab recognises "[Draft]", "Draft:" and "(Draft)" (case-insensitive) at the start of the title
// [CITED: docs.gitlab.com/user/project/merge_requests/drafts]; GitLab itself writes "Draft: <title>".
var draftPrefix = regexp.MustCompile(`(?i)^\s*(\[draft\]|\(draft\)|draft:)`)

func withDraftPrefix(title string) string {
	if draftPrefix.MatchString(title) {
		return title // already draft, never double-prefix
	}
	return "Draft: " + title
}
```
- `create_merge_request`: `draft=true` sends `Title: withDraftPrefix(title)`.
- `update_merge_request`: `draft=true` with `title` given sends `withDraftPrefix(title)`; with no `title` do ONE `GetMergeRequest` first (read, safe) and send `withDraftPrefix(current.Title)`; skip the PUT field if already draft. Un-drafting through `draft=false` stays unsupported (D-10); tell the agent in the `draft_status` advice to send `title` without the prefix (see status table).
- Show the real state from the response (`mr.Draft`), not the intention.

### Pattern 4: merge_merge_request flow (D-12/D-13/D-14)
```go
mr, _, err := d.GL.MergeRequests.GetMergeRequest(project, iid, nil, gitlab.WithContext(ctx))
if err != nil { return "", withSubject("MR (iid) или проект", err) }
if mr.State != "opened" { return "", errors.New(stateAdvice(mr)) }        // merged / closed / locked, no PUT
if mr.DetailedMergeStatus != "mergeable" { return "", errors.New(statusAdvice(mr)) } // no PUT
opts := &gitlab.AcceptMergeRequestOptions{}
if in.Squash { opts.Squash = gitlab.Ptr(true) }
if in.RemoveSourceBranch { opts.ShouldRemoveSourceBranch = gitlab.Ptr(true) }
if m := strings.TrimSpace(in.MergeCommitMessage); m != "" { opts.MergeCommitMessage = gitlab.Ptr(m) }
merged, _, err := d.GL.MergeRequests.AcceptMergeRequest(project, iid, opts, gitlab.WithContext(ctx))
if err != nil { return "", withWrite(opMergeMR, "MR (iid) или проект", err) }
```
Success text: `MR !iid влит (state=merged)`, merge commit `merged.MergeCommitSHA` (may be empty for fast-forward merge projects: fall back to `SquashCommitSHA`, then to `merged.SHA`, label the fallback), `remove_source_branch: запрошено` (deletion is performed by GitLab after the merge, do not claim "удалена"; read `merged.ShouldRemoveSourceBranch`), `web_url`. If `merged.State != "merged"` print the state and tell the agent to verify with `get_merge_request`.
A returned `isError` from the pre-check must be a plain `errors.New(...)` with the Russian text (pass-through for `KindOther`, as Phase 2 guards do); never wrap it in `withWrite`.

### Pattern 5: create_merge_request flow (D-08/D-09)
1. Guards: `source_branch`, `title` non-empty (Russian error, no request).
2. `target, isDefault, err := resolveRef(ctx, d, project, in.TargetBranch)` (1 GET only if empty).
3. If `source == target`: local error "ветка-источник совпадает с целевой".
4. Pre-check: `res, err := fetchCompare(ctx, d, project, target, source)`; on 404 -> `withSubject("ветка или проект", err)`; if `!res.CompareTimeout && len(res.Commits) == 0` -> error "нет изменений между ветками: в <source> нет коммитов, которых нет в <target>. Сначала закоммитьте изменения (commit_files)". This exists because GitLab does not reject this case (F4).
5. POST once. On error `withWrite(opCreateMR, ...)`; rules below map 409 duplicate (parse `!N`) and 422 branch problems.
6. Success text: `MR !iid создан: source→target (target: ветка по умолчанию)`, `draft`, `web_url`, and the mandatory hint: "статус слияния обычно checking: перед merge_merge_request вызовите get_merge_request" (D-05, do not poll). Print `detailed_merge_status` raw with the advice from the table.

### Anti-Patterns to Avoid
- **Reading GitLab's list default:** omitting `state` returns ALL MRs (open, closed, merged) [CITED: docs.gitlab.com/api/merge_requests, list project MRs: "Defaults to `all`"]. Always send `state` explicitly (`opened` when the tool arg is empty).
- **Using `/changes`** (deprecated, removal in v5) to obtain `overflow`.
- **Echoing raw bodies** for 405/422 (`405 Method Not Allowed`, `Branch cannot be merged`): map to Russian text.
- **Polling or sleeping** for `checking`/`unchecked` (D-05). One GET, honest message.
- **Pointer fields in tool inputs** (schema would emit `["null", ...]`; FND-08 guard fails).
- **Retrying or "verify then retry" a PUT/POST inside a handler.** The only extra requests permitted around a write are reads before it (target branch, compare, MR GET).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Per-file diff rendering, empty-patch reasons | New renderer | `renderDiffFiles`, `emptyPatchKind`, `diffHeading` (`diff.go`) + 10-line adapter | Already handles collapsed/too_large/rename/mode/binary, budget reservation for all headings |
| Paging, budget, footers | Custom | `ClampPaging`, `composeList`, `PageFooter`, `Budget`, `oneLine` (`format.go`) | Rune-based budget matching the agent's `len(str)` cut |
| Default target branch | Own project lookup | `resolveRef`, `refLabel` (`ref.go`) | One GET, empty-repo error included |
| Branch-range emptiness check | New compare client | `fetchCompare` (`compare.go`) | Already decodes `commits`, `compare_timeout`, `compare_same_ref` |
| Error classification/retry | New HTTP layer | `glclient.Classify`, GET-only retry policy | POST/PUT already never retried (`retry.go` `checkRetry`) |
| Fake GitLab, MCP session | New harness | `testutil.NewFakeGitLab` (`Recorded()` captures bodies), `newTestSession`, `callText` | Token-leak check is automatic in `callText` |
| HTTP body encoding of options | Manual JSON | client-go options structs | client-go encodes GET options as query, POST/PUT as JSON body |

**Key insight:** every risky part of this phase is wording and ordering (status gating, error mapping, "unknown outcome"), not transport. Keep handlers thin and put the knowledge in two tables (status advice, write rules) that are unit-testable without HTTP.

## Runtime State Inventory

Not a rename/refactor/migration phase. Omitted.

## Common Pitfalls

### Pitfall 1 (F1): `overflow` is not in the `/diffs` response
**What goes wrong:** D-06 wants an `overflow` line, but `GET /merge_requests/:iid/diffs` returns only `collapsed`, `too_large`, `generated_file` per file. `overflow` exists only in the deprecated `/changes` [CITED: docs.gitlab.com/api/merge_requests: `/diffs` attribute table vs `/changes` text]. A Go decode of a non-existent `overflow` yields silent `false` forever.
**How to avoid:** derive it from the MR's `changes_count` (a STRING; capped at 1000 and returned as `"1000+"`) [CITED: same page]. Emit "часть файлов не вернулась: в MR больше 1000 файлов". Per-file `too_large`/`collapsed` remain the per-file signals. State this in the tool description honestly.
**Warning signs:** a test that sets `"overflow": true` in the fake `/diffs` JSON (it does not exist upstream).

### Pitfall 2 (F2): no `draft` parameter on create/update
**What goes wrong:** assuming a `Draft` field on `CreateMergeRequestOptions`/`UpdateMergeRequestOptions` (neither has it in v2.64.0) [VERIFIED: source]; GitLab REST create/update params list has no `draft` [VERIFIED: docs + `lib/api/merge_requests.rb`]. Only list has a `draft` filter.
**How to avoid:** title prefix (Pattern 3). Also: GitLab auto-prefixes `Draft:` itself when the source has no commits ahead [VERIFIED: `build_service.rb` `set_draft_title_if_needed`], which the pre-check in Pattern 5 prevents.

### Pitfall 3 (F3): list default is `state=all`
**How to avoid:** map empty `state` to `opened`; validate against `{opened, closed, merged, all}` in the handler with a Russian error (GitLab also accepts `locked`, not exposed). Show the effective state in the header line (`MR проекта g/p, state=opened`).

### Pitfall 4 (F4): GitLab creates MRs for branches with no changes
**What goes wrong:** D-09 promises "нет изменений между ветками", but GitLab does not fail: it creates the MR as Draft with an empty diff. Same-branch (`source == target`) does fail with 422 (`You must select different branches` / `You can't use same project/branch...`), nonexistent branches fail with 422 (`Source branch "x" does not exist`) [VERIFIED: `build_service.rb` validations, `merge_request.rb` `validate_branches`].
**How to avoid:** the `fetchCompare` pre-check in Pattern 5 (cost: one GET, may return a large body; the shared client already caps bodies at 8 MB). Also keep 422 rules for the same-branch and does-not-exist messages as a backstop.

### Pitfall 5 (F5): duplicate open MR is a 409 whose message carries the reference
GitLab: `Another open merge request already exists for this source branch: !5` with HTTP 409 [VERIFIED: `merge_request.rb` `conflicting_mr_message`, `handle_merge_request_errors!` `conflict!`]. client-go renders array messages as `[text]` into `Detail` (capped at 300 runes) [VERIFIED: `parseError` in `gitlab.go`].
**How to avoid:** regex `!(\d+)` on the detail; if absent, one safe GET `ListProjectMergeRequests(State=opened, SourceBranch=source, PerPage=1)` and use its `iid`/`web_url`. Text: `открытый MR уже есть: !N (<web_url>)`. The recovery GET must not turn the result into success (D-09).

### Pitfall 6 (F6): merge failure codes and what they mean
Real endpoint behaviour [VERIFIED: docs table + `lib/api/merge_requests.rb` `execute_merge`]:
| Status | GitLab message | Meaning | Text to give (RU) |
|--------|----------------|---------|-------------------|
| 400 | `SHA must be provided when merging` | group/instance setting "require SHA" is on (GitLab 19.2) | «в группе включено требование sha при merge; инструмент sha не передаёт (MRX-03, v2)» |
| 401 | `401 Unauthorized` | **no permission to merge** (`can_be_merged_by?`), NOT an invalid token (the pre-check GET just succeeded with that token) | «нет прав вливать этот MR (роль ниже Developer/Maintainer или защита целевой ветки)» |
| 405 | `405 Method Not Allowed` | MR not mergeable now, or already merged/closed | «слияние отклонено: состояние MR изменилось после проверки; вызовите get_merge_request» |
| 406 | (not in merge docs; appears for cancel-auto-merge) | defensive | treat like 405 |
| 409 | `SHA does not match HEAD of source branch` | head moved (only with `sha`) | «ветка-источник изменилась» (only reachable if MRX-03 lands; map anyway) |
| 422 | `Branch cannot be merged` | merge failed at execution (conflict, hook, race) | «GitLab не смог влить: вероятно конфликт или изменилась целевая ветка; проверьте get_merge_request» |
The generic `writeText` 401 branch says "токен недействителен ... нужен scope api", which is wrong for merge; add an op-keyed rule for `opMergeMR` (and a 403 rule: no permission to merge into the protected target branch). Because `writeRule` currently matches only on kind + detail substring and 405/406 messages are bare strings, add an optional `status int` field to `writeRule` (match when `r.status == 0 || r.status == e.Status`) and allow rules with empty `substrings` to match on kind/status/op alone.

### Pitfall 7 (F7): fresh MRs have empty/partial fields
`diff_refs` and `changes_count` are empty right after creation and fill asynchronously [CITED: docs.gitlab.com/api/merge_requests#empty-api-fields-for-new-merge-requests]. `detailed_merge_status` is typically `checking`, `unchecked` or `preparing`. Do not print "файлов: 0" or `base_sha` blanks as facts. Show `число файлов: ещё не известно` when `ChangesCount == ""`; use fallbacks for refs (Pattern 2). `HeadPipeline` may be nil: print "pipeline: нет".

### Pitfall 8 (F8): quick actions in bodies run on GitLab
`create_merge_request_note` body and the MR description are processed for quick actions (`/close`, `/target_branch x`, `/assign`, ...). `/merge` requires `merge_request_diff_head_sha`, so an accidental `/merge` fails [CITED: docs.gitlab.com/api/notes: `merge_request_diff_head_sha` "Required for the /merge quick action"]. A body that is only commands may produce a note object without a usable id [ASSUMED]. Handler: print the note id only when `note.ID > 0`, else "комментарий принят (id не возвращён: возможно, тело содержало только быстрые команды)". Document in tool description that lines starting with `/` are GitLab commands.

### Pitfall 9 (F9): merge argument naming
`AcceptMergeRequestOptions` field is `ShouldRemoveSourceBranch` (`should_remove_source_branch`); create/update use `remove_source_branch`. Squash may be overridden by project settings ("Require"/"Do not allow") [CITED: docs squash param]. Do not report the flag as effective; report what was requested and the state returned.

### Pitfall 10 (F10): 404 is a bodyless sentinel
client-go returns `ErrNotFound` for any 404 (no message), and GitLab returns 404 for invisible private projects too. Use subjects: `"MR (iid) или проект"` for iid-based calls, `"проект или ветка"` for create. The existing `baseText` 404 wording already adds the private-project hint.

### Pitfall 11 (F11): merge-status recheck is triggered by the read
Fetching the single MR asynchronously re-evaluates mergeability; poll by calling `get_merge_request` again [CITED: docs "Single merge request response notes"]. `has_conflicts` is `false` unless `merge_status` is `cannot_be_merged`. So `checking` + `has_conflicts:false` does not mean "no conflicts"; print conflicts only as "признак конфликтов" alongside the status.

### Pitfall 12 (999.2): unknown-outcome wording
`writeText` currently adds `writeUnknownOutcome` only for Server/Timeout/Network, and the suffix mentions `list_branches, list_commits`. Extend to `KindCanceled, KindDecode, KindTooLarge, KindOther` and select the hint by op (WR-01 in 02-REVIEW also asks to drop "повторите позже" for writes because it contradicts the suffix; check `errors_test.go` expectations when changing `baseText` use):
- `opCreateMR`: «проверьте list_merge_requests (source_branch=...) перед повтором»
- `opMergeMR`, `opUpdateMR`: «проверьте состояние через get_merge_request перед повтором»
- `opMRNote`: «проверьте list_merge_request_notes перед повтором»
429 on a write is not an unknown outcome (GitLab rejected it before processing): keep the plain rate-limit text.

### Pitfall 13: system notes and multi-line bodies
Notes are multi-line and system notes are noisy ("added 1 commit\n\n* abc - x"). System note: `oneLine(body, ~120)` after `[system]`. User note: header line `#id @username date`, then body cut with `Budget(body, 1000)` plus `[заметка обрезана]`. 20 notes x 1000 runes exceeds the 15 000 budget; `composeList` handles the cut and footer, but state in the description that `per_page` should be lowered for long threads. `Note.Position != nil` marks an inline diff note; a `[inline path:line]` marker is optional (MRX-01 is v2).

### Pitfall 14: smoke script write guard (999.5 still open)
`scripts/smoke.py` treats any `--base-url` as hermetic. New MR writes (create, note, update, merge) must not run against gitlab.com. Either close 999.5 minimally (loopback-only guard) in this phase's smoke changes, or put the new write calls behind the same guard AND add an explicit loopback host check there; Phase 4's live run depends on it.

## detailed_merge_status table (D-04)

Source of values: docs.gitlab.com/api/merge_requests#merge-status [CITED]. Proposed RU advice (planner may refine wording; `unknown value -> raw + generic hint`):

| Value | Advice (RU, proposal) |
|-------|-----------------------|
| `mergeable` | можно вливать (merge_merge_request) |
| `checking` | GitLab проверяет возможность слияния; повторите get_merge_request через несколько секунд |
| `unchecked` | проверка слияния ещё не выполнена; повторите get_merge_request через несколько секунд |
| `preparing` | GitLab ещё готовит diff MR; повторите через несколько секунд |
| `approvals_syncing` | аппрувы синхронизируются; повторите позже |
| `ci_still_running` | pipeline ещё выполняется; дождитесь завершения (auto-merge не поддерживается) |
| `ci_must_pass` | нужен успешный pipeline: исправьте сбои CI или дождитесь его запуска |
| `commits_status` | в ветке-источнике нет коммитов или ветка не существует |
| `conflict` | конфликты с целевой веткой; разрешите их вне сервера (rebase/merge целевой ветки) |
| `need_rebase` | MR нужно перебазировать (инструмента rebase в сервере нет; сделайте в GitLab) |
| `discussions_not_resolved` | есть неразрешённые обсуждения; разрешите их в GitLab |
| `draft_status` | MR помечен как Draft; снимите пометку: update_merge_request с title без префикса «Draft:» |
| `not_approved` | нужен approve (approve в сервере не поддерживается, MRX-02) |
| `requested_changes` | ревьюер запросил изменения |
| `not_open` | MR не открыт; см. state (влит/закрыт; закрытый можно открыть update_merge_request state_event=reopen) |
| `merge_request_blocked` | MR блокируется другим MR |
| `merge_time` | слияние запрещено до указанного времени (merge_after) |
| `status_checks_must_pass` | должны пройти внешние status checks |
| `security_policy_pipeline_check`, `security_policy_violations` | требования политик безопасности не выполнены |
| `jira_association_missing`, `title_regex`, `locked_paths`, `locked_lfs_files` | требование проекта не выполнено (см. значение); исправьте в GitLab |
Structure: `var mergeStatusAdvice = map[string]string{...}` and `func statusAdvice(mr *gitlab.MergeRequest) string` that returns `"detailed_merge_status: <raw> — <advice>"`. `stateAdvice` for `merged`/`closed`/`locked` checked before the status (a merged MR reports `not_open`). Empty status -> "статус слияния не вернулся; повторите get_merge_request".

## Code Examples

### MR write rules (extends `errors.go`)
```go
const (
	opCreateMR = "create_merge_request"
	opUpdateMR = "update_merge_request"
	opMergeMR  = "merge_merge_request"
	opMRNote   = "create_merge_request_note"
)

type writeRule struct { // add: status int  // 0 = any
	kind glclient.Kind; op string; status int; substrings []string; text string
}
// match: r.kind == e.Kind && (r.op == "" || r.op == op) && (r.status == 0 || r.status == e.Status)
//        && (len(r.substrings) == 0 || any substring in lower(detail))

{kind: glclient.KindBadRequest, op: opCreateMR, status: 409, substrings: []string{"another open merge request already exists"},
	text: "открытый MR из этой ветки уже есть. %s"}, // handler enriches with !N before/after; or handle 409 in the handler
{kind: glclient.KindBadRequest, op: opCreateMR, substrings: []string{"you must select different branches", "same project/branch"},
	text: "ветка-источник совпадает с целевой."},
{kind: glclient.KindBadRequest, op: opCreateMR, substrings: []string{"does not exist"},
	text: "ветка не найдена: %s"},
{kind: glclient.KindUnauthorized, op: opMergeMR, text: "401: нет прав вливать этот MR (роль ниже Developer/Maintainer или защита целевой ветки)."},
{kind: glclient.KindBadRequest, op: opMergeMR, status: 405, text: "слияние отклонено: MR сейчас нельзя влить (состояние изменилось после проверки или MR уже влит/закрыт). Вызовите get_merge_request."},
{kind: glclient.KindBadRequest, op: opMergeMR, status: 422, substrings: []string{"branch cannot be merged"}, text: "GitLab не смог влить MR (конфликт или изменилась целевая ветка). Вызовите get_merge_request."},
```
(`writeText` prepends `statusPrefix(e)`, so texts carry no status digits. 401 is a `KindUnauthorized`, which currently bypasses the rules loop only when no rule matches; the loop runs first, so an op-keyed rule wins.)

### 409 duplicate: extract iid
```go
var mrRef = regexp.MustCompile(`!(\d+)`)
// detail example from client-go parseError: "[Another open merge request already exists for this source branch: !5]"
if m := mrRef.FindStringSubmatch(e.Detail); m != nil { /* iid = m[1] */ }
```
Handle this in `createMergeRequest` before delegating to `withWrite` (Classify the error, check `Status == 409` and the substring), so the message includes `!N` and `web_url` (from the fallback list call when needed).

### Fake GitLab routes for tests (raw, encoded paths)
```go
fake.JSON("GET",  "/api/v4/projects/g%2Fp/merge_requests", 200, `[{"iid":5,"state":"opened","draft":false,"title":"t","source_branch":"f","target_branch":"main","author":{"username":"alice"}}]`, nil)
fake.JSON("GET",  "/api/v4/projects/g%2Fp/merge_requests/5", 200, `{"iid":5,"state":"opened","detailed_merge_status":"mergeable","sha":"abc","changes_count":"2","has_conflicts":false,"web_url":"https://gitlab.com/g/p/-/merge_requests/5"}`, nil)
fake.JSON("GET",  "/api/v4/projects/g%2Fp/merge_requests/5/diffs", 200, `[{"old_path":"a","new_path":"a","diff":"@@ -1 +1 @@\n-x\n+y\n","collapsed":false,"too_large":false}]`, map[string]string{"X-Next-Page": ""})
fake.JSON("PUT",  "/api/v4/projects/g%2Fp/merge_requests/5/merge", 405, `{"message":"405 Method Not Allowed"}`, nil)
fake.JSON("POST", "/api/v4/projects/g%2Fp/merge_requests", 409, `{"message":["Another open merge request already exists for this source branch: !5"]}`, nil)
```
Assert with `fake.Recorded()` (bodies) that a blocked merge sent NO `PUT .../merge`, that optional merge/update fields are absent when not passed, that a failed 5xx write was sent exactly once, and that `state=opened` is on the wire for a default list.

### Registration annotations (discretion, recommended)
`list_*`/`get_*`: `ReadOnlyHint: true`. `create_merge_request`, `create_merge_request_note`: `DestructiveHint: gitlab.Ptr(false)` (additive). `update_merge_request`, `merge_merge_request`: `DestructiveHint: gitlab.Ptr(true)` (close/merge are not undoable). Descriptions in Russian, <= 900 runes, names <= 30 (`list_merge_request_notes` = 24, `create_merge_request_note` = 25).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `GET /merge_requests/:iid/changes` | `GET .../diffs` (paginated) | `/changes` deprecated GitLab 15.7, removal in API v5 | Use `/diffs`; lose the `overflow` field (F1) |
| `merge_status` (`can_be_merged`) | `detailed_merge_status` | GitLab 15.6+ | Use the detailed one (already required by MR-02) |
| `merge_when_pipeline_succeeds` | `auto_merge` | Deprecated GitLab 17.11 | Out of scope (auto-merge unsupported) |
| Optional `sha` on merge | Group/instance setting can REQUIRE `sha` (400 if missing) | GitLab 19.2 | New 400 case to word; see Open Question O1 |
| `xanzy/go-gitlab` | `client-go/v2` | already adopted | n/a |

**Deprecated/outdated:** `AcceptMergeRequestOptions.MergeWhenPipelineSucceeds` (use `AutoMerge`, not used here); `MergeRequest.WorkInProgress` (use `Draft`); `BasicMergeRequest.MergedBy` (use `MergeUser`).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | A note body consisting only of quick actions returns a note object without a usable `id` (handler prints id only if > 0) | Pitfall 8 | Cosmetic: slightly wrong success wording |
| A2 | GitLab draft prefix set is exactly `[Draft]`, `Draft:`, `(Draft)` case-insensitive; docs list these three, source regex was not retrieved | Pattern 3 | A title with another prefix form could get a second `Draft:` prefix; harmless |
| A3 | `fetchCompare(target, source)` with an empty `commits` list reliably means "no changes" (`compare_timeout` false) | Pattern 5 | Could wrongly block creating a valid MR in an edge case; mitigated by skipping the check when `CompareTimeout` is true |
| A4 | Merge PUT may answer 406 in some GitLab versions (defensively mapped like 405) | Pitfall 6 | None: extra rule is harmless |
| A5 | Fast-forward merge projects return an empty `merge_commit_sha` | Pattern 4 | Success text falls back to `SquashCommitSHA`/`sha`, labelled |
| A6 | gitlab.com currently serves the GitLab version documented on master (19.x docs mention 19.2 features) | Pitfall 6 | The `sha`-required 400 may not apply yet; mapping is harmless |

## Open Questions (RESOLVED)

1. **Send the pre-checked `sha` on merge?** RESOLVED: follow D-13, no sha sent (plan 03-05); the author question is carried in the 03-05 SUMMARY.
   - What we know: the merge handler already has `mr.SHA` from the pre-check GET; sending it as `sha` closes the GET-to-PUT race (409 if the head moved) and satisfies a group "require SHA" setting (400 otherwise). CONTEXT D-13 / MRX-03 say the sha protection stays v2 as a user-facing parameter.
   - What's unclear: whether the author considers an internal, non-parameter `sha` a violation of the deferral.
   - Recommendation: follow the locked text (do not send `sha`), map the 400 "SHA must be provided" text clearly, and list the internal-sha idea for the discuss-phase/user. If accepted it is a one-line change (`SHA: gitlab.Ptr(mr.SHA)`).
2. **Un-drafting is impossible via `draft` (D-10)** RESOLVED in 03-04 (title without the `Draft:` prefix; documented in the update_merge_request description).
   - `draft=false` means "not passed", yet `draft_status` blocks merge. Resolution inside D-10: the status advice tells the agent to send `title` without the `Draft:` prefix (works because non-empty `title` changes the field). Document the same in the `update_merge_request` description. Flag to the user if a real `draft=false` is wanted later (needs the 999.1-style pointer/flag approach).
3. **Cost of the `create_merge_request` compare pre-check** RESOLVED (accepted) in 03-03.
   - Adds up to 2 reads (project for default branch, compare) before the POST. Acceptable for a write tool; if the author objects, the alternative is to accept GitLab's silent empty Draft MR and warn after creation (violates D-09's "ничего не меняется молча").
4. **Optional `[inline path:line]` marker for diff notes**: not required by MR-04; planner may skip. RESOLVED (skipped).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build/test | yes | 1.27.1 (go.mod `go 1.25.0`) | — |
| `go build ./...` / `go test ./...` baseline | all | green at research time | — | — |
| Python + `mcp` 1.30.0 | `scripts/smoke.py` | yes | Python 3.13.15, mcp 1.30.0 | — |
| `uv` | `e2e/python_smoke_test.go` (skips if absent) | not checked | — | test self-skips |
| Real gitlab.com test project + PAT (`api`) | live MR checks | not in this phase | — | Phase 4 (QA-03); Phase 3 is verified against the fake only |

**Missing dependencies with no fallback:** none for this phase.

## Testing Notes

`workflow.nyquist_validation` is `false` in `.planning/config.json`, so no Validation Architecture section. Framework: Go `testing` + `net/http/httptest` fake + `mcp.NewInMemoryTransports` (Phase 1-2 harness); quick run `go test ./internal/tools/...`, full `go test ./...` (no `-race` on this machine). Wave 0 gaps: none in tooling; new `mr_*_test.go` files, regenerate golden (`go test ./internal/tools -run TestToolsListGolden -update`), extend `scripts/smoke.py` `EXPECTED_TOOLS` and `e2e/python_smoke_test.go` routes, extend `errors_test.go` with MR write cases (incl. Canceled/Decode/TooLarge/Other for 999.2). Must-have assertions per requirement: MR-01 wire has `state=opened` by default and filters only when non-empty; MR-02 raw status + advice, `checking` is honest, exactly 1 request; MR-03 empty patch never reads as "no changes", `changes_count:"1000+"` yields the overflow line, footer from `X-Next-Page`; MR-04 `[system]` marker, body cut at ~1000; MR-05 no request when guards fail, no POST when compare shows no commits, 409 yields `!N`; MR-06 empty body sends nothing; MR-07 blocked pre-check sends no PUT, 5xx PUT sent once with "get_merge_request" hint, 401/405/422 texts; MR-08 nothing-to-change error before any request, `draft` without `title` does exactly GET then one PUT.

## Security Domain

`security_enforcement` is not disabled; applicable controls:

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (single PAT from env, unchanged) | — |
| V3 Session Management | no | — |
| V4 Access Control | yes (server-side permission is GitLab's; the tool must not imply more) | Map 401/403 on merge to "no permission" wording; no privilege escalation paths |
| V5 Input Validation | yes | go-sdk schema validation + explicit guards: `iid > 0`, `state`/`state_event` enums, non-empty `title`/`body`/`source_branch`, `NormalizeProject`; ensure branch names travel only in JSON/query values (client-go encodes) |
| V6 Cryptography | no | — |
| V7 Error handling/logging | yes | `safe()` redaction of the token last; never echo raw response bodies (`capDetail` 300 runes) |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Prompt injection via MR title/description/notes returned to the model | Tampering / Elevation | Output is data; frame results (header lines, `[system]` markers); do not add instructions derived from GitLab content; system-note bodies cut to one line |
| GitLab quick actions in description/note bodies (`/close`, `/target_branch`, `/assign`) | Tampering | Document in tool descriptions; `/merge` needs head sha so it cannot run accidentally (F8) |
| Accidental irreversible merge | Repudiation / Tampering | Pre-check gate (D-12), single non-retried PUT, unknown-outcome message tells to verify before retry (D-15), `DestructiveHint: true` |
| Token leakage in error text | Information disclosure | Existing redactor + `callText` leak assertion in every test |
| Oversized bodies (description up to 1,048,576 chars) | DoS | Output caps (1500/1000/2000 runes); inputs pass through as GitLab limits them |

## Project Constraints (from CLAUDE.md)

- stdio only; nothing on stdout except JSON-RPC; logs to stderr; never `fmt.Print*`/`log.Println` to stdout.
- Go + `go-sdk` v1.8.0 + `client-go/v2` v2.64.0, pinned; no Node; no config frameworks.
- Auth: PAT in `GITLAB_TOKEN`; writes need scope `api`; token never printed.
- Target gitlab.com only; "Mini": no Issues/Pipelines/approvals scope creep.
- Tool input: plain typed fields with `json:"x,omitempty"` + `jsonschema:"..."` tags; NO pointer fields (schema becomes `["null", ...]`); do not add `jsonschema-go` as a direct dependency.
- Map GitLab failures to short actionable Russian messages inside the handler ("401/403/404/429 ..."); surface `Retry-After` on 429.
- Truncate at documented caps and say so; cap/default `per_page` (default 20, max 100); never depend on `X-Total`.
- `go test ./...` and `go vet`; no `go test -race` on this box.
- GSD workflow: file edits happen through GSD execute-phase, not directly.

## Sources

### Primary (HIGH confidence)
- `client-go/v2@v2.64.0` source in the local Go module cache: `merge_requests.go` (options structs, `MergeRequestDiff`, `AcceptMergeRequestOptions`), `notes.go` (`ListMergeRequestNotes`, `CreateMergeRequestNote`, `Note`), `gitlab.go` (`CheckResponse`, `parseError`, `ErrNotFound`), `pipelines.go`, `users.go`
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/api/merge_requests.md (fetched 2026-09-24): merge-status values, diffs endpoint attributes, changes/overflow, create/update/merge parameters and failure table, `changes_count`, empty-fields troubleshooting
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/api/notes.md: MR notes list/create parameters, `merge_request_diff_head_sha`
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/lib/api/merge_requests.rb: merge endpoint flow and status codes, `handle_merge_request_errors!`, optional params (no `draft`), diffs endpoint
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/app/models/merge_request.rb (`validate_branches`, conflicting-MR message, draft helpers) and `app/services/merge_requests/build_service.rb` (same-branch error, auto-draft when no commits)
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/lib/gitlab/pagination/offset_header_builder.rb, `offset_pagination.rb`: pagination headers on `/diffs`
- Repo: `internal/tools/{diff,errors,commits,compare,branch,ref,format,register,safe}.go`, `internal/glclient/{errors,retry}.go`, `internal/testutil/{fakegitlab,session}.go`, `.planning/phases/02-history-and-write/{02-PATTERNS,02-REVIEW}.md`, `.planning/ROADMAP.md` backlog 999.2/999.5

### Secondary (MEDIUM confidence)
- https://gitlab.com/gitlab-org/gitlab/-/raw/master/doc/user/project/merge_requests/drafts.md: draft prefix forms

### Tertiary (LOW confidence)
- None relied on. Items depending on GitLab runtime behaviour not testable offline are in the Assumptions Log.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, all calls and struct fields read from the pinned module source.
- Architecture: HIGH, reuses proven Phase 1-2 patterns; new parts are two tables and small helpers.
- Pitfalls: HIGH for F1-F6, F9-F11 (docs plus GitLab source); MEDIUM for F8 quick-action edge case and the compare pre-check semantics (A1, A3).

**Research date:** 2026-09-24
**Valid until:** 2026-10-24 (GitLab API stable; re-check the merge `sha` requirement setting and rate-limit changes dated 2026-10-19 before Phase 4)
