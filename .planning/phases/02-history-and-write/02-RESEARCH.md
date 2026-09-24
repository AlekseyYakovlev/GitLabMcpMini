# Phase 2: История и запись в репозиторий - Research

**Researched:** 2026-09-24
**Domain:** Seven more MCP tools (branches, commits, compare, create branch, multi-file commit, single-file write) on top of the Phase 1 Go skeleton (`go-sdk` v1.8.0 + `client-go/v2` v2.64.0), consumed by Python `mcp` 1.30.x
**Confidence:** HIGH for everything that touches our own code, client-go v2.64.0 and go-sdk v1.8.0 (read in the module cache and executed in a scratch spike); MEDIUM for GitLab server-side error wording and branch pagination mode (only doc/source excerpts and forum reports, no live gitlab.com call yet; live check is Phase 4)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

### commit_files: схема actions[]
- **D-01:** Поддерживаются действия `create`, `update`, `delete`, `move` (для `move` — поле `previous_path`). `chmod` не поддерживается.
- **D-02:** Параметры инструмента: `project`, `branch` (обязателен, ветка должна существовать), `commit_message` (обязателен), `actions[]`. Поля `start_branch`, `force` и `encoding` не вводятся; новую ветку создаёт `create_branch`. Запись только UTF-8 текста, без base64 и бинарных файлов.
- **D-03:** Схема `actions[]` — массив объектов с инлайн-схемой элемента (`action`, `file_path`, `content`, `previous_path`), без `$ref`/`anyOf`/`null`, простые типы с `omitempty`, не указатели (FND-08, D-15 Phase 1). Принятие схемы клиентом `mcp` 1.30.x проверяется тестом на реальном клиенте (закрывает blocker из STATE.md).
- **D-04:** Предохранители записи, ошибка до обращения к GitLab: непустой `actions[]` и не больше ~50 элементов; суммарный размер `content` ≤ 1 МБ; `commit_message` не пуст; пути проходят через `NormalizeRepoPath`. Это здравый смысл, а не read-only режим (он в Out of Scope).
- **D-05:** Запросы записи не повторяются автоматически (FND-07, D-03 Phase 1). Успешный ответ: короткий SHA, ветка, по одной строке на файл (действие + путь, статистика +/−) и `web_url` коммита.

### create_or_update_file
- **D-06:** Тонкая обёртка над `commit_files` (один путь кода). Параметры: `project`, `path`, `content`, `branch`, `commit_message`. Create или update определяется автоматически: сначала GET файла на ветке (404 → create, иначе update), затем один вызов `commit_files`. В ответе явно указано `created` или `updated`.
- **D-07:** При update в действие передаётся `last_commit_id` из GET. Если GitLab отвечает 400-конфликтом, сообщение: «файл изменился с момента чтения — прочитайте его заново».
- **D-08:** Содержимое пишется как есть, ничего не меняется молча. Если у текущего файла были CRLF, а в новом `content` их нет, в ответ добавляется предупреждение «концы строк изменились CRLF→LF». Проверка есть только у `create_or_update_file` (у `commit_files` нет GET).
- **D-09:** Ошибки записи: 400 «not allowed to push» → «ветка защищена: создайте ветку (`create_branch`), коммитьте туда, затем MR»; 401/403 → подсказка про scope `api` и роль Developer+. Расширяется существующий маппер ошибок из Phase 1, тексты только на русском.

### Вывод diff (get_commit, compare_refs)
- **D-10:** Порядок: шапка → сводка «файлов: N» → по каждому файлу заголовок (путь, `new`/`deleted`/`renamed`) и patch, обрезанный до ~2000 символов на файл с пометкой. Когда общий бюджет 15 000 исчерпан, оставшиеся файлы перечисляются без патчей с пометкой «патч не показан». Модель всегда видит полный список файлов.
- **D-11:** Если GitLab не вернул patch (`too_large`, `collapsed`, бинарный, только переименование или режим), причина указывается явно в заголовке файла со ссылкой на чтение файла через `get_file_contents` на нужном ref. Пустой patch не должен выглядеть как «нет изменений». При `compare_timeout=true` выводится предупреждение «diff может быть неполным».
- **D-12:** `compare_refs` принимает `from` и `to`, без `straight` (поведение GitLab по умолчанию, от merge-base). В шапке «from → to». Compare API не пагинируется, поэтому сервер запрашивает сравнение целиком и режет файлы на страницы `page`/`per_page` сам; футер как в Phase 1. Коммиты сравнения показываются одной короткой секцией (до ~20 строк).
- **D-13:** `get_commit` всегда возвращает шапку и diff, без флага `include_diff`. Шапка: полный SHA, автор, дата, родители, stats (+/−), сообщение (до ~1000 символов), `web_url`. Параметры: `project`, `sha` (SHA, ветка или тег), `page`/`per_page` для файлов diff (пагинация на стороне GitLab).

### list_commits, list_branches, create_branch
- **D-14:** `list_commits`: параметры `ref` (по умолчанию ветка проекта, D-11 Phase 1), `path`, `since`, `until` (ISO 8601), `author`, `page`/`per_page`. Строка: короткий SHA, дата, автор, заголовок сообщения.
- **D-15:** `list_branches`: параметр `search` (подстрока имени) плюс `page`/`per_page`. Строка: имя, короткий SHA, пометки `[default]`, `[protected]`, `[merged]` и заголовок последнего коммита. Пометки помогают агенту не писать в защищённую ветку.
- **D-16:** `create_branch`: параметры `project`, `branch`, `ref` (SHA, ветка или тег; по умолчанию ветка проекта, D-11 Phase 1). Если ветка уже есть, приходит понятная ошибка «ветка уже существует» (400 из GitLab), ничего не меняется молча. Вывод: имя, исходный ref, SHA конца ветки, `web_url`.

### Уже зафиксировано (не переоткрывать)
- **D-17:** Из Phase 1 действуют: имена без префикса, компактный текстовый вывод одной строкой на элемент, футеры пагинации и обрезки, общий бюджет 15 000 символов, ветка по умолчанию при пустом `ref`, ошибки как `isError`, только stdio и логи в stderr, токен нигде не выводится.

### Claude's Discretion
- Точное значение лимита patch на файл (ориентир ~2000 символов), значения `per_page` по умолчанию (ориентир 20), максимум actions (ориентир 50).
- Точный формат строк вывода и текстов ошибок; формулировки описаний инструментов (описание ≤900 символов, имя ≤30).
- `mcp.ToolAnnotations` для новых инструментов (чтение — `ReadOnlyHint`, запись — без него; `DestructiveHint` для `commit_files`/`create_or_update_file` на усмотрение планировщика).
- Как именно обернуть GET в `create_or_update_file`: через существующие `RepositoryFiles.GetFile` и `resolveRef` из Phase 1.
- Раскладка файлов в `internal/tools/` (по образцу `tree.go`, `file.go`, `projects.go`).

### Deferred Ideas (OUT OF SCOPE)
- `start_branch` в `commit_files` (создание ветки прямо в коммите) — сейчас дублирует `create_branch`; вернуться, если окажется неудобно агенту.
- Запись бинарных файлов (`encoding: base64`) и `chmod` — не нужны учебному проекту.
- `delete_branch` — уже в v2 (WRT-04).
- Опциональный `straight` для `compare_refs` — при необходимости позже.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| READ-05 | `list_branches` | `Branches.ListBranches(pid, *ListBranchesOptions{ListOptions, Search})`; `Branch{Name, Protected, Merged, Default, Commit *Commit}`. Pagination mode (offset vs keyset) is uncertain, see Open Question 1 |
| READ-06 | `list_commits` по ref/пути | `Commits.ListCommits(pid, *ListCommitsOptions{ListOptions, RefName, Since, Until, Path, Author})`; `Since`/`Until` are `*time.Time` (parse the ISO 8601 string ourselves) |
| READ-07 | `get_commit` с diff | Two calls: `Commits.GetCommit(pid, sha, nil)` (stats default true) + `Commits.GetCommitDiff(pid, sha, *GetCommitDiffOptions{ListOptions})`; `Diff` lacks `collapsed`/`too_large` in client-go, infer the reason (Pattern 4) |
| UTIL-02 | `compare_refs` | `Repositories.Compare(pid, *CompareOptions{From, To})` returns `Compare{Commit, Commits, Diffs, CompareTimeout, CompareSameRef, WebURL}`; unpaginated, slice `Diffs` ourselves |
| WRT-01 | `create_branch` | `Branches.CreateBranch(pid, *CreateBranchOptions{Branch, Ref})` (POST, returns `*Branch`); `resolveRef` for empty `ref` |
| WRT-02 | `commit_files` | `Commits.CreateCommit(pid, *CreateCommitOptions{Branch, CommitMessage, Actions []*CommitActionOptions})`; JSON body verified on the wire; nested `actions[]` schema needs the null-array patch (Pattern 1) |
| WRT-03 | `create_or_update_file` | `RepositoryFiles.GetFile` (branch as ref) then the shared commit core with `LastCommitID` on update |
</phase_requirements>

## Summary

Phase 2 adds no new dependency and no new architecture. Every tool follows the Phase 1 handler shape (`func(ctx, in T) (string, error)` wrapped by `safe`), reuses `NormalizeProject`/`NormalizeRepoPath`, `resolveRef`, `ClampPaging`, `composeList`, `Budget`, `PageFooter` and the `withSubject` error mapper. All service methods needed exist in `client-go/v2` v2.64.0 with typed options (verified by reading the module source). The baseline (`go build`, `go vet`, `go test ./...`) is green on this machine.

Three facts found by execution or source reading change the plan relative to CONTEXT.md, and the planner must handle them explicitly:

1. **A Go slice field in the tool input struct is rendered as `"type": ["null","array"]`** by the go-sdk schema inference (verified in a spike). That violates FND-08/D-03 and would fail the existing schema guard `TestToolSchemas` (`type is an array`). The `commit_files` schema must therefore be set explicitly. The verified fix: build the `InputSchema` yourself (hand-written `map[string]any`, or `jsonschema.For` plus patching `Type="array"`, `Types=nil`) and pass it in `mcp.Tool.InputSchema`; the SDK then validates arguments against that schema and still unmarshals into the Go struct. The rest of the schema stays flat and `additionalProperties:false` applies to the array items too.
2. **client-go's `Diff` struct (commits and compare) has no `collapsed`/`too_large` fields** (only the MR diff type has them). D-11's "explicit reason" therefore has to be inferred from `diff == ""` plus `new_file`/`deleted_file`/`renamed_file`/modes, or the response has to be decoded with a custom struct. Inference is recommended.
3. **`FakeGitLab` (Phase 1) records only `METHOD RequestURI`, not request bodies.** CONTEXT says the fake records requests "to check that `actions[]` really went to GitLab"; for POST that needs body capture. A small extension is a Wave 0 task.

Smaller but plan-relevant: GitLab answers a push to a forbidden branch with **403** at the API layer (`forbidden!("You are not allowed to push into this branch")`) and with **400** from `Commits::CreateService`; the mapper must treat both. `create_or_update_file` does not need `resolveRef` because `branch` is a required argument. A 5xx/timeout on a write means "outcome unknown"; the error text must say so.

**Primary recommendation:** Add seven thin handlers in three files (`history.go` for branches/commits/compare/diff rendering, `write.go` for create_branch/commit core/create_or_update_file, plus a `schema.go` helper for the explicit `commit_files` schema), extend `errors.go` with a write-aware layer, extend `FakeGitLab` with body capture, and prove the nested schema with a golden `tools/list` snapshot and the Python `mcp==1.30.0` smoke script.

## Architectural Responsibility Map

Single local stdio process; "tiers" are the process layers established in Phase 1.

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Tool schemas incl. nested `actions[]` | Tool layer (`internal/tools`, explicit `InputSchema`) | go-sdk (validation) | SDK inference cannot express a non-nullable array; explicit schema is ours, validation is the SDK's |
| Write safety limits (non-empty, <=50 actions, <=1 MiB, message non-empty, path normalisation) | Tool layer (before any request) | `glclient.NormalizeRepoPath` | D-04: fail before touching GitLab |
| No automatic retry of POST | `glclient` (client-level GET/HEAD-only policy, Phase 1) | Tool layer (per-request `WithRequestRetry` optional belt and braces) | Already verified in `client_test.go` (POST 500/429, PUT 503 = 1 attempt) |
| Create-vs-update decision, CRLF check | Tool layer (`create_or_update_file`) | client-go `GetFile` | Needs the file content and `last_commit_id` from one GET |
| Write error wording (protected branch, conflict, exists) | Tool layer (`errors.go`) | `glclient.Classify` (Kind, Status, Detail) | Wording depends on the tool; Classify only gives kind + capped detail |
| Diff rendering, per-file patch cap, budget split | Tool layer (`diff.go`/`history.go`) | `format.go` (`Budget`) | Pure presentation for the LLM |
| Compare pagination | Tool layer (slice `Diffs`) | `PageFooter` with a synthetic `Response` | GitLab compare is not paginated (D-12) |
| Branch/commit/file persistence | GitLab (external) | — | No local state |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 (already in go.mod) | `mcp.AddTool`, `ToolAnnotations`, in-memory transport | Locked (Phase 1 D-12) [VERIFIED: go.mod, executed in spike] |
| `gitlab.com/gitlab-org/api/client-go/v2` | v2.64.0 (already in go.mod) | `Branches`, `Commits`, `Repositories.Compare`, `RepositoryFiles.GetFile` | Locked (Phase 1 D-01) [VERIFIED: module cache source read] |
| stdlib `time`, `strings`, `bytes`, `unicode/utf8`, `errors` | bundled | ISO 8601 parsing, CRLF check, rune counting | No new dependency |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/jsonschema-go` | v0.4.3 (indirect today) | Only if you choose `jsonschema.For` + patch for the nested schema | CLAUDE.md says "do not add as a direct dependency"; the hand-written `map[string]any` schema avoids it (recommended) |
| Python `mcp` | ==1.30.0 via `uv run scripts/smoke.py` | Real-client acceptance of the nested schema (D-03) | Extend the existing script; `uv` 0.12.5 and Python 3.13.15 are present |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-written `map[string]any` schema for `commit_files` | `jsonschema.For[T]` then patch `Type`/`Types` | Keeps schema in sync with the struct but makes `jsonschema-go` a direct dependency (contradicts CLAUDE.md). Both verified to produce the same accepted schema |
| Infer empty-patch reason | Decode diffs with own struct via `Client.NewRequest`/`Do` | Exposes `too_large`/`collapsed` exactly, but requires hand-building the escaped path (breaks "encoding is client-go's job", Phase 1 Pattern 4). Not worth it |
| GET file for create-vs-update | `RepositoryFiles.GetFileMetaData` (HEAD) | HEAD gives `last_commit_id` cheaply but no content, so the CRLF check (D-08) is impossible; D-06 says GET |

**Installation:** none. `go.mod` is unchanged (verified: the two required modules and `go-retryablehttp` are already direct requirements).

**Version verification:** `go.mod` pins go-sdk v1.8.0 and client-go/v2 v2.64.0; both were the latest per the Phase 1 registry check (2026-09-24). `go build ./...`, `go vet ./...`, `go test ./...` pass on the current tree [VERIFIED: executed today].

## Package Legitimacy Audit

No new external packages are introduced in this phase. Existing modules (go-sdk, client-go/v2, go-retryablehttp) were approved in Phase 1. `slopcheck` does not cover Go modules (see Phase 1 audit).

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| (none new) | — | — | — | — | n/a | Approved (nothing to install) |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
Agent (Python mcp 1.30.x)  --tools/call-->  go-sdk: validate args against tool schema
                                              | invalid -> isError (readable)
                                              v
                                      tools.safe[T]  (recover | 25 s deadline | redact | isError)
                                              |
      +--------------+--------------+---------+-----------+------------------+------------------+
      |              |              |                     |                  |                  |
 list_branches  list_commits   get_commit / compare_refs  create_branch   commit_files   create_or_update_file
      |              |              |                     |                  |                  |
      |              |              |                     |                  |                  +-- GET file @branch
      |              |              |                     |                  |                      404 -> create | 200 -> update(+last_commit_id, CRLF check)
      |              |              |                     |                  |                      identical content -> "без изменений" (no POST)
      |              |              |                     |                  |                             |
      |              |              |                     |                  +<-------- commitCore <-------+
      |              |              |                     |                  |   validate (D-04) -> POST /repository/commits (1 attempt only)
      |              |              |                     |   resolveRef?    |
      v              v              v                     v   -> POST       v
   NormalizeProject / NormalizeRepoPath / ClampPaging -> client-go (d.GL) -> GitLab REST v4
                                              |
   success -> formatter (one line per item, diff budget split, footers, Budget 15 000 runes)
   failure -> withSubject / withWrite -> toToolText -> Russian isError text (5xx/timeout on write: "результат неизвестен")
```

### Recommended Project Structure
```
internal/tools/
├── register.go        # +7 AddTool calls (read tools ReadOnlyHint; write tools annotated)
├── history.go         # list_branches, list_commits (+ line renderers, parseTime)
├── commit.go          # get_commit, compare_refs handlers
├── diff.go            # renderDiffFiles: per-file cap 2000, shared 15 000 budget, empty-patch reason
├── branch.go          # create_branch
├── write.go           # commitCore, commit_files handler, create_or_update_file handler, write limits
├── schema.go          # commitFilesSchema() explicit InputSchema (nested actions[])
├── errors.go          # extend: withWrite(), write-aware branch of toToolText
└── testdata/tools_list.golden.json   # golden snapshot of tools/list (or per-tool for commit_files)
internal/testutil/fakegitlab.go       # extend: record request bodies
scripts/smoke.py                      # EXPECTED_TOOLS 5 -> 12, call the new tools incl. nested actions
e2e/python_smoke_test.go              # register fake routes for the new endpoints
```
File names are a recommendation (discretion area); keep one concern per file as in Phase 1.

### Pattern 1: Explicit schema for `commit_files` (verified)
**What:** `[]ActionIn` inferred by go-sdk yields `"type": ["null","array"]`. Provide the schema yourself. The SDK validates arguments against the supplied schema (verified: `actions: null` rejected with `has type "null", want "array"`, missing `file_path` rejected, extra properties in items rejected) and unmarshals into the struct as usual.
**When to use:** only `commit_files` (all other new inputs are flat scalars).
**Example:**
```go
// Source: scratch spike (scratchpad/spike/spike2_test.go), go-sdk v1.8.0, executed
type ActionIn struct {
    Action       string `json:"action"`
    FilePath     string `json:"file_path"`
    Content      string `json:"content,omitempty"`
    PreviousPath string `json:"previous_path,omitempty"`
}
type CommitFilesIn struct {
    Project       string     `json:"project"`
    Branch        string     `json:"branch"`
    CommitMessage string     `json:"commit_message"`
    Actions       []ActionIn `json:"actions"`
}

func commitFilesSchema() map[string]any {
    str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
    return map[string]any{
        "type": "object", "additionalProperties": false,
        "required": []any{"project", "branch", "commit_message", "actions"},
        "properties": map[string]any{
            "project":        str(`numeric project ID as a string ("12345") or full path group/subgroup/project`),
            "branch":         str("existing branch to commit to; create it first with create_branch"),
            "commit_message": str("commit message, not empty"),
            "actions": map[string]any{
                "type": "array", "description": "file changes of one commit, 1..50 items",
                "items": map[string]any{
                    "type": "object", "additionalProperties": false,
                    "required": []any{"action", "file_path"},
                    "properties": map[string]any{
                        "action":        str("create, update, delete or move"),
                        "file_path":     str("path of the file in the repository"),
                        "content":       str("new full UTF-8 text content (create, update; optional for move)"),
                        "previous_path": str("old path, required for move"),
                    },
                },
            },
        },
    }
}
// register: mcp.AddTool(s, &mcp.Tool{Name: "commit_files", Description: ..., InputSchema: commitFilesSchema(), Annotations: ...}, safe(d, commitFiles(d)))
```
Notes: `required` in the array items must list `action`/`file_path` (verified rejection message names the missing field). No `enum` (an `enum` keyword is not forbidden by the guard but keep the schema simplest; validate the action in the handler for a Russian message). The existing `TestToolSchemas` walks `items` recursively, so it covers the nested schema automatically and also requires every top-level property to have a description.

### Pattern 2: Shared commit core (one code path, D-06)
`create_or_update_file` must pass `last_commit_id`, which is not in the public `commit_files` schema (D-03). So the tool input struct and the internal request differ:
```go
type fileAction struct{ Action, Path, PreviousPath, Content, LastCommitID string; hasContent bool }

func commitCore(ctx context.Context, d Deps, project, branch, message string, acts []fileAction) (*gitlab.Commit, error) {
    opts := &gitlab.CreateCommitOptions{Branch: gitlab.Ptr(branch), CommitMessage: gitlab.Ptr(message)}
    for _, a := range acts {
        act := gitlab.FileActionValue(a.Action)
        o := &gitlab.CommitActionOptions{Action: &act, FilePath: gitlab.Ptr(a.Path)}
        if a.PreviousPath != "" { o.PreviousPath = gitlab.Ptr(a.PreviousPath) }
        if a.hasContent        { o.Content = gitlab.Ptr(a.Content) }   // non-nil pointer to "" is still sent
        if a.LastCommitID != "" { o.LastCommitID = gitlab.Ptr(a.LastCommitID) }
        opts.Actions = append(opts.Actions, o)
    }
    c, _, err := d.GL.Commits.CreateCommit(project, opts, gitlab.WithContext(ctx))
    return c, withWrite("проект", err)
}
```
Verified wire (client-go v2.64.0, executed): `POST /api/v4/projects/g%2Fp/repository/commits`, `Content-Type: application/json`, body `{"branch":"main","commit_message":"m","actions":[{"action":"create","file_path":"a/b.txt","content":"привет\r\n"}]}`; CRLF and Cyrillic survive untouched. No `stats`/`force`/`start_branch` keys are sent when their pointers are nil.

### Pattern 3: Write-aware error mapping (extends Phase 1 `toToolText`)
`glclient.Classify` gives `Kind`, `Status`, capped `Detail`. For a JSON error `{"message":"..."}` client-go's `parseError` renders `Detail` as `{message: <text>}` (verified: `400 {message: You are not allowed to push into this branch}`), so match by case-insensitive substring, never by equality.

Add a `write bool` to `subjectError` (or a sibling `writeError`) via `withWrite(subject, err)`, and in `toToolText` handle write errors before the generic switch:

| Situation (source) | Kind/Status | Detail contains (lowercase) | Text (Russian) |
|---|---|---|---|
| Push to protected/forbidden branch, API layer [CITED: gitlab-foss `lib/api/commits.rb`: `forbidden!("You are not allowed to push into this branch")`] | Forbidden 403 | — | "403: запись отклонена: ветка защищена, либо у токена нет scope `api`, либо роль ниже Developer. Создайте ветку (`create_branch`), коммитьте в неё, затем MR." |
| Push denied, service layer [CITED: gitlab-foss `Commits::CreateService#validate_permissions!`] | BadRequest 400 | `not allowed to push` | D-09 text: "ветка защищена: создайте ветку (`create_branch`), коммитьте туда, затем MR" |
| `last_commit_id` conflict [CITED: `Files::MultiService`: "The file has changed since you started editing it: <path>"] | 400 | `changed since you started editing` | D-07: "файл изменился с момента чтения — прочитайте его заново" |
| Branch missing (commit API sets `start_branch ||= branch`) [CITED: `Commits::CreateService#validate_on_branch!`; GitLab issue 60226] | 400 | `you can only create or edit files when you are on a branch` | "ветка не найдена: создайте её через `create_branch`" |
| File exists on `create` / missing on `update`,`delete` [CITED: GitLab forum thread 69027, MEDIUM] | 400 | `a file with this name already exists` / `a file with this name doesn't exist` | "файл уже существует: используйте action=update" / "файла нет на ветке: используйте action=create" |
| Branch exists on `create_branch` (D-16) | 400 | `already exists` (also `a branch called`) | "ветка уже существует" |
| Bad ref / bad branch name on `create_branch` [CITED: `Branches::CreateService`: "Failed to create branch '...': invalid reference name '...'"] | 400 | `invalid reference name` / `ref is missing` / `branch name is invalid` | "исходный ref не найден" / "недопустимое имя ветки" |
| 401 on write | 401 | — | existing text + "для записи нужен scope `api`" |
| 5xx, timeout, network on a write | Server/Timeout/Network | — | existing text + "**Результат записи неизвестен**: коммит мог быть создан. Проверьте `list_commits` перед повтором." |

Always append the capped raw detail for 400s that match no rule (existing behaviour). Because write requests are never retried, the "outcome unknown" sentence is the only protection against a duplicate commit when the agent retries.

### Pattern 4: Diff rendering with a shared budget (D-10, D-11)
Presentation order: header, `файлов: N`, file headings (all of them), patches. The model must always see the full file list, so file headings are budgeted first and patches share what remains.
```go
const patchCap = 2000 // runes per file (discretion, orientation ~2000)

func renderDiffFiles(diffs []*gitlab.Diff, refForHint string, budget int) string {
    // 1) headings (always all), 2) remaining = budget - runes(headings) 3) patches in order
    ...
    for i, d := range diffs {
        if d.Diff == "" { heading += " — " + emptyPatchReason(d, refForHint); continue }
        if remaining < 200 { heading += " — патч не показан (бюджет вывода исчерпан)"; continue }
        p, cut := Budget(d.Diff, min(patchCap, remaining))   // Budget cuts on a line boundary
        if cut { p += fmt.Sprintf("\n[патч обрезан: показано %d из %d символов]", ...) }
        remaining -= utf8.RuneCountInString(p)
    }
}

func emptyPatchReason(d *gitlab.Diff, ref string) string {
    hint := "; содержимое: get_file_contents с ref=" + ref
    switch {
    case d.RenamedFile:            return "переименование без изменений содержимого"
    case d.NewFile || d.DeletedFile: return "патч не возвращён (пустой файл, бинарный или слишком большой)" + hint
    case d.AMode != d.BMode:        return "изменён только режим файла (" + d.AMode + "→" + d.BMode + ")"
    default:                        return "патч не возвращён GitLab (бинарный, слишком большой или свёрнут)" + hint
    }
}
```
Order of the switch matters: for new/deleted files GitLab reports `a_mode`/`b_mode` of `"0"` vs `"100644"`, so the mode check must come after `NewFile`/`DeletedFile`. Final guard: pass the whole assembled body through `Budget(..., OutputBudget)` and append `TruncatedFooter` if it still overflows (possible with 100 files of very long paths); then the page footer.

Heading vocabulary (D-10): `new`, `deleted`, `renamed old→new`, else `modified`. Put `compare_timeout=true` warning right under the header ("diff может быть неполным").

### Pattern 5: `compare_refs` pagination on our side (D-12)
```go
cmp, _, err := d.GL.Repositories.Compare(project, &gitlab.CompareOptions{From: gitlab.Ptr(from), To: gitlab.Ptr(to)}, gitlab.WithContext(ctx))
page, perPage = ClampPaging(in.Page, in.PerPage)
lo, hi := (page-1)*perPage, min(page*perPage, len(cmp.Diffs))   // guard lo > len -> empty page with a hint
resp := &gitlab.Response{}                                       // synthetic, only NextPage is read by PageFooter
if hi < len(cmp.Diffs) { resp.NextPage = int64(page + 1) }
footer := PageFooter(page, perPage, resp)
```
`PageFooter` only reads `resp.NextPage`/`resp.NextLink`, so a synthetic `*gitlab.Response{NextPage: n}` reuses it without refactoring `format.go`. Handle `cmp.CompareSameRef` ("ветки совпадают, различий нет"), a nil `cmp.Commit`, and `len(cmp.Commits)` above 20 ("показаны первые 20 из N"). "compare" is symmetric only with `straight=false` semantics: `from` is the base; say "from → to" in the header (D-12). Tags/branches with `/` are query params, encoded by client-go.

### Pattern 6: `create_or_update_file` flow (D-06, D-07, D-08)
1. Normalise project/path; validate `commit_message` non-empty, content size, no NUL byte.
2. `f, _, err := d.GL.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(branch)}, gitlab.WithContext(ctx))`. `branch` is required, so `resolveRef` is unnecessary (it would return immediately for a non-empty ref anyway).
3. `glclient.Classify(err).Kind == KindNotFound` -> create. Any other error -> return it. A 404 caused by a missing project or branch is not distinguishable here; the following POST yields the exact error (404 project / 400 "on a branch"), so do not add pre-checks.
4. Update: decode base64 (reuse the `file.go` code); if existing content is binary (`isBinary`) -> error "файл бинарный: запись бинарных файлов не поддерживается". If `existing == new content` -> return "содержимое не изменилось, коммит не создан" without POST (recommended, see Assumption A4). CRLF check: `bytes.Contains(raw, "\r\n") && !strings.Contains(content, "\r\n")` -> append warning "концы строк изменились CRLF→LF" (D-08). Pass `LastCommitID: f.LastCommitID`.
5. `commitCore` with a single action; output `created`/`updated`, short SHA, branch, path, `web_url`, warnings.

### Pattern 7: Registration and annotations
```go
mcp.AddTool(s, &mcp.Tool{Name: "list_branches", Description: listBranchesDescription,
    Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, safe(d, listBranches(d)))
// additive write:
f := false
mcp.AddTool(s, &mcp.Tool{Name: "create_branch", Description: ...,
    Annotations: &mcp.ToolAnnotations{DestructiveHint: &f}}, safe(d, createBranch(d)))
// commit_files / create_or_update_file: leave DestructiveHint nil (spec default true, they can delete/overwrite)
```
`ToolAnnotations.DestructiveHint` is `*bool` and only meaningful when `ReadOnlyHint` is false [VERIFIED: go-sdk v1.8.0 `protocol.go`]. `ReadOnlyHint`/`IdempotentHint` are always serialised as `false` when unset (no `omitempty`), which is fine. Annotations are not part of the schema guard.

### Anti-Patterns to Avoid
- **`[]ActionIn` with default inference** — emits `["null","array"]`; fails FND-08 and the guard (verified).
- **Pointer fields in tool input structs** — same `["null", ...]` problem (Phase 1 D-15).
- **`jsonschema:"path=..."`-style tags** (start with `word=`) — panic at `AddTool` (Phase 1 Pitfall 4). Applies to any new descriptions.
- **Retrying or pre-checking writes "to be safe"** — D-05; never wrap `CreateCommit`/`CreateBranch` in a loop.
- **Rendering `diff == ""` as "no changes"** (PITFALLS #10).
- **Using `X-Total`** — never (Phase 1 D-07).
- **Sending `encoding`, `start_branch`, `force`, `chmod`, `execute_filemode`** — out of scope (D-02, D-01).
- **Asserting `[protected]` == "cannot write"** — a Maintainer/Owner can usually push to a protected default branch on gitlab.com; the marker is a warning, and the tool text must say "может быть защищена" rather than forbid.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP path/query encoding for project, sha, path, refs | manual `url.PathEscape` | client-go option structs and `ProjectID`/`withPath` | Phase 1 verified wire table (`.`→`%2E`, one pass). Branch/tag names in `:sha` (e.g. `feature/x`) are escaped by client-go the same way |
| POST JSON body with nested actions | own `map` marshalling | `gitlab.CreateCommitOptions` + `CommitActionOptions` | Verified body shape; `omitempty` on pointers handles optional fields |
| Pagination flag | parsing headers | `Response.NextPage`/`NextLink` + `PageFooter` | Already in Phase 1 |
| Cutting text on a line boundary | manual slicing | `Budget(text, limit)` | Rune-based, returns `truncated` |
| Schema validation of nested args | hand-validation of arrays | go-sdk validation against the explicit `InputSchema` | Verified readable errors for missing/extra/null items |
| Retry protection for writes | per-call loops/guards | Phase 1 client policy (GET/HEAD-only) | Tested (`client_test.go`); add one tool-level test that a 503 on POST yields exactly 1 request |
| Time parsing for `since`/`until` | regex | `time.Parse` with RFC3339 and date-only layouts | Small helper; client-go formats `*time.Time` as RFC3339 in the query |

**Key insight:** the risky part of this phase is not the API calls (each is one typed client-go call) but (a) the shape of the schema the client sees, (b) truthful error and outcome wording for writes, and (c) keeping diff output bounded and honest.

## Runtime State Inventory

Not applicable: this is a greenfield feature phase, not a rename/refactor/migration. No stored data, live-service config, OS registration, secrets or build artifacts carry a renamed string. (Verified: no rename in scope.)

## Common Pitfalls

### Pitfall 1: Nullable array in the inferred schema
**What goes wrong:** `Actions []ActionIn` becomes `"type":["null","array"]`; `TestToolSchemas` fails and the LLM sees a nullable type.
**Why it happens:** jsonschema-go treats slices as nullable by default (verified).
**How to avoid:** Pattern 1 (explicit schema). Keep the golden snapshot so a future SDK bump cannot silently change it.
**Warning signs:** `type is an array [null array]` from `walkSchema`.

### Pitfall 2: client-go `Diff` has no `too_large`/`collapsed`
**What goes wrong:** an implementation that trusts those fields never gets them; an empty patch prints as "no changes".
**How to avoid:** Pattern 4 inference with the exact switch order; a test per branch (rename, new empty/binary, mode-only, default).
**Warning signs:** a file heading with no patch and no reason.

### Pitfall 3: Two different status codes for "not allowed to push"
**What goes wrong:** matching only "400 not allowed to push" (D-09) misses the API-layer 403 with the same message, so a protected-branch write is reported as a scope problem or as a generic 403.
**How to avoid:** the mapper handles 403 (protected OR scope OR role, combined wording) and 400 with the substring. Tests for both.

### Pitfall 4: A write error is not proof that nothing was written
**What goes wrong:** timeout/5xx/network on `POST /commits` may occur after GitLab created the commit; the agent retries and creates a duplicate (or a `create` conflict).
**How to avoid:** "результат неизвестен" wording (Pattern 3); success text always includes the SHA so `get_commit` can confirm.

### Pitfall 5: `last_commit_id` semantics and update conflicts
**What goes wrong:** passing the branch head instead of the file's `last_commit_id` causes false conflicts; omitting it silently overwrites concurrent edits.
**How to avoid:** take `File.LastCommitID` from the GET on the same branch (D-07); on conflict the message tells the agent to re-read. `commit_files` intentionally has no `last_commit_id` (D-03), so it can overwrite; say so in its description.

### Pitfall 6: Branch list pagination mode
**What goes wrong:** GitLab's branch list uses a keyset-capable pager (`GitalyKeysetPager`); if it answers with `Link rel=next` using `page_token` and no `X-Next-Page`, our footer says "вызовите с page=N+1" while `page` is ignored on keyset responses, so the agent loops on page 1.
**How to avoid:** see Open Question 1. `PageFooter` already falls back to `NextLink`. Add a `list_branches` test with `Link` only and decide the wording; verify live in Phase 4. Default (no `pagination=keyset` param) is expected to be offset.

### Pitfall 7: Empty and CRLF content
**What goes wrong:** creating an empty file needs `content:""` sent (non-nil pointer to empty string is serialised by `omitempty`; verified semantics: omitted only for nil pointers). Using a plain `string` in the input struct cannot distinguish "empty" from "missing" for `update`/`create`; the handler must treat `content` as required for create/update by action, not by emptiness (so empty file creation stays possible) and reject `delete`/`move` handling separately.
**How to avoid:** `hasContent = action is create or update` (and `move` only if `content` non-empty).

### Pitfall 8: Diff output exceeds the agent's cut
**What goes wrong:** many files with long paths plus patches exceed 20 000 chars, the agent cuts silently.
**How to avoid:** headings first, patches share the remainder, final `Budget` guard, `per_page` default 20 (max 100).

### Pitfall 9: Guard and description limits with Cyrillic
Descriptions are counted in runes (agent `len(str)`); keep each <=900 runes; names <=30 (`create_or_update_file` is 21). Do not start any `jsonschema` tag with `word=`.

### Pitfall 10: Fake server route keys
`FakeGitLab` matches exact raw path without query. New routes: `POST /api/v4/projects/g%2Fp/repository/commits`, `GET .../repository/commits/<sha>`, `.../commits/<sha>/diff`, `GET .../repository/compare`, `GET|POST .../repository/branches`, `GET .../repository/files/<path with .→%2E>`. Branch names in `:sha` are escaped (`feature%2Fx`, `release-1%2E0`). Unregistered routes return 404 by default, which hides forgotten routes; assert on `fake.Requests()`.

## Code Examples

### ISO 8601 parsing for `since`/`until`
```go
// Source: stdlib time; client-go serialises *time.Time as RFC3339 in the query string
func parseTime(name, s string) (*time.Time, error) {
    s = strings.TrimSpace(s)
    if s == "" { return nil, nil }
    for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
        if t, err := time.Parse(layout, s); err == nil { return &t, nil }
    }
    return nil, fmt.Errorf("%s: неверный формат даты %q, ожидается ISO 8601, например 2026-09-01 или 2026-09-01T12:00:00Z", name, s)
}
```

### Line formats (discretion; keep one line per item)
```
list_branches:  main [default] [protected] a1b2c3d4 Заголовок коммита
                feature/x [merged] 9f8e7d6c Заголовок
list_commits:   a1b2c3d4 2026-09-24 Автор Заголовок сообщения
create_branch:  ветка feature/x создана от main; конец ветки a1b2c3d4…; <web_url>
commit_files:   коммит a1b2c3d4 в feature/x (+12/-3): 2 файла
                create docs/a.md
                update src/main.go
                <web_url>
create_or_update_file: updated src/main.go, коммит a1b2c3d4 в feature/x [+ предупреждение CRLF→LF]
```
Per-file `+/-` (D-05) cannot be produced for `commit_files`: the create-commit response carries only commit-level `stats` {additions, deletions, total} [VERIFIED: `Commit.Stats`, `CommitStats` in client-go]; there is no per-file breakdown and no GET before the write. Show action + path per file and the total `+/-` in the header line; per-file stats are visible afterwards via `get_commit`. (Deviation from D-05's wording, flagged in Assumptions A1.)

### FakeGitLab body capture (Wave 0)
```go
// internal/testutil/fakegitlab.go: keep Requests() as is; add bodies alongside
type Recorded struct{ Method, URI, Body string }
func (f *FakeGitLab) Recorded() []Recorded { ... }
// in serve(): read r.Body (io.ReadAll), restore with io.NopCloser(bytes.NewReader(b)) before calling the handler
```
Then tests assert `POST /api/v4/projects/g%2Fp/repository/commits` body JSON has the expected `actions` (decode with `json.Unmarshal` into a map and compare, do not string-match key order).

### Golden `tools/list` snapshot (Specifics in CONTEXT)
Marshal `ListTools` result with `json.MarshalIndent` (encoding/json sorts map keys; `required` order comes from the definition) and compare with `internal/tools/testdata/tools_list.golden.json`; regenerate with `go test ./internal/tools -run TestToolsListGolden -update` (flag defined in the test). Covers the nested `commit_files` schema so an SDK upgrade cannot change it silently.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `xanzy/go-gitlab` | `gitlab.com/gitlab-org/api/client-go/v2` v2.64.0 | 2024-12 | Already adopted (Phase 1) |
| Diff flags only on MR diffs | `collapsed`/`too_large` exposed for commit diff and compare responses in the REST docs (added in GitLab 18.4 per PITFALLS.md) | 2025 | Server may send them, client-go v2.64.0 `Diff` type ignores them; infer instead |
| Offset pagination for all lists | Some lists (branches) can use keyset via `page_token`; `X-Next-Page` absent on keyset | ongoing | Keep `NextLink` fallback; verify live |

**Deprecated/outdated:** none relevant beyond the above.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | D-05's "статистика +/- per file" is not obtainable from `CreateCommit`; show commit-level totals plus per-file action/path only | Code Examples | Planner may want per-file stats; only possible with an extra `GetCommitDiff` call after the write (an extra request, not a retry) — user to confirm the reduced form |
| A2 | Branch list uses offset pagination when only `page`/`per_page` are sent; the "next page" hint `page=N+1` works [ASSUMED: from gitlab-foss `GitalyKeysetPager` excerpt; not observed live] | Pitfall 6 | Agent could loop on page 1 for large branch lists; verify in Phase 4 live run |
| A3 | Error wording from GitLab ("A file with this name already exists/doesn't exist", "Branch already exists", "invalid reference name") [CITED: forum thread 69027 (MEDIUM), gitlab-foss service sources]; exact strings/status may vary by GitLab version | Pattern 3 | Mapper falls back to the generic 400 text with the raw capped detail, so failure mode is a less friendly message, not a wrong action |
| A4 | Identical-content update is short-circuited client-side ("содержимое не изменилось") instead of sending an empty commit; unverified whether GitLab would create an empty commit or reject | Pattern 6 | If the user prefers "write as is" (D-08 literal), remove the shortcut; then an unchanged update may produce an empty commit or a GitLab error |
| A5 | Commit/compare diffs on gitlab.com are collapsed/too_large only for large files; `diff==""` inference covers them [ASSUMED] | Pattern 4 | A binary new file and a too-large new file share one reason text; acceptable since both point to `get_file_contents` |
| A6 | Branch/tag names with `/` or `.` in the `:sha` path segment are accepted by GitLab when escaped by client-go (`feature%2Fx`, `%2E`) [ASSUMED: docs say "Commit hash or branch/tag name"] | Don't Hand-Roll | `get_commit`/diff by branch name would 404; workaround is passing a SHA; check live in Phase 4 |
| A7 | Content-empty-string create is accepted by GitLab (`content:""`) | Pitfall 7 | Creating an empty file could return 400 "content is missing"; the mapper shows the detail; low impact |

## Open Questions

1. **Does `GET /projects/:id/repository/branches?page=N` paginate by offset with `X-Next-Page` on gitlab.com?**
   - What we know: the endpoint is served by a keyset-capable Gitaly pager; the source excerpt shows keyset is one of three branches of the pager logic and the docs are silent.
   - What's unclear: which branch a plain `page/per_page` request takes.
   - Recommendation: implement with the existing `PageFooter` (NextPage then NextLink fallback), add a fake test for "Link only", and record the live result in the Phase 4 checklist. If keyset is confirmed, switch the hint to explain `page_token` or cap `list_branches` output and ask to narrow with `search`.
2. **Is a per-file `+/-` line for `commit_files` worth an extra `GetCommitDiff` request?**
   - Recommendation: no. Totals in the header, `get_commit` for detail (A1).
3. **Should identical-content updates be blocked or written?** (A4) Recommendation: block with a clear message; it is cheap and avoids an ambiguous GitLab reaction.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build and tests | yes | 1.27.1 (go.mod: 1.25.0) | — |
| `uv` | Python `mcp` 1.30.0 smoke and e2e test | yes | 0.12.5 | `python` 3.13.15 with `mcp==1.30.0` installed; the e2e test skips without `uv` |
| Python | smoke client | yes | 3.13.15 (`python3` alias is a Store stub, use `python`/`uv`) | — |
| gitlab.com + PAT scope `api` + test project | live verification of writes | not needed in Phase 2 | — | Deferred to Phase 4 (QA-03); Phase 2 is verified against the fake server |
| C compiler / `-race` | — | no | — | Not required; do not use `-race` |

**Missing dependencies with no fallback:** none for Phase 2.

## Validation Architecture

`workflow.nyquist_validation` is `false` in `.planning/config.json`, so no formal validation section is required. The test map below reflects the Phase 1 test tiers and is the recommended acceptance evidence.

| Req | Behavior | Test type | Where | Notes |
|-----|----------|-----------|-------|-------|
| READ-05 | branch lines with `[default]`/`[protected]`/`[merged]`, `search`, paging footer, Link-only next | tool (in-memory + fake) | `internal/tools/history_test.go` | `commit` may be nil in fixtures |
| READ-06 | `ref` default via project GET, `path` wire, `since/until` RFC3339 on wire, invalid date error before request | tool | `history_test.go` | assert raw `RequestURI` |
| READ-07 | header fields, diff rendering, per-file cap, budget exhaustion note, empty-patch reasons (4 cases), pagination footer from `X-Next-Page` | tool + unit for `renderDiffFiles` | `commit_test.go`, `diff_test.go` | rune counts < 15 500 incl. footers |
| UTIL-02 | from/to on wire, same-ref, `compare_timeout` warning, server-side slicing pages, commits section <=20 | tool | `commit_test.go` | |
| WRT-01 | POST body `{branch, ref}`, default ref, "уже существует" (400), invalid ref | tool | `branch_test.go` | then `list_branches` shows it (SC-2) using fake state |
| WRT-02 | body has all actions (create/update/delete/move + previous_path), guards (empty, >50, >1 MiB, empty message, `..`), no `force`/`start_branch`/`encoding` keys, POST 503 -> exactly 1 request and "результат неизвестен", 403 and 400 protected text | tool + fake with body capture | `write_test.go` | needs Wave 0 fake extension |
| WRT-02 (schema) | nested `actions[]` accepted by real client 1.30.0, golden `tools/list`, guard passes | golden + `TestToolSchemas` + Python smoke | `schema_test.go`, `scripts/smoke.py` | closes the STATE.md blocker |
| WRT-03 | 404 -> create; 200 -> update with `last_commit_id`; CRLF warning; conflict message; identical content; binary existing file refused; "уже существует"/"не существует" messages | tool | `write_test.go` | |

### Sampling Rate
- Per task commit: `go test ./internal/tools/... ./internal/glclient/...`
- Per wave merge / phase gate: `go build ./... && go vet ./... && go test ./...` (includes `e2e`, which builds the binary; the Python smoke runs when `uv` is present)

### Wave 0 Gaps
- [ ] Extend `internal/testutil/fakegitlab.go` to capture request bodies (`Recorded()`), keeping `Requests()` intact
- [ ] Add the shared response helpers/fixtures for commits, branches, compare, diff in a `_test.go` helper file
- [ ] Update `scripts/smoke.py` `EXPECTED_TOOLS` (12 names), add calls for the new tools, including a `commit_files` call with nested `actions`, and `e2e/python_smoke_test.go` fake routes
- [ ] Golden snapshot test scaffold with an `-update` flag
- [ ] Update the other e2e tests only if they assert the tool count (check `stdio_test.go` for fixed tool lists)

## Security Domain

`security_enforcement` is not set to false, so this section applies.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (token) | PAT from `GITLAB_TOKEN`, header-only, redacted everywhere (Phase 1); write needs scope `api` (document, error text says so) |
| V3 Session Management | no | stdio process, no sessions |
| V4 Access Control | yes | Enforced by GitLab (role + branch protection). We surface 403/400 messages; no client-side allow-list (read-only mode is out of scope) |
| V5 Input Validation | yes | go-sdk schema validation (explicit nested schema, `additionalProperties:false`), `NormalizeProject`/`NormalizeRepoPath` (rejects `..`, URLs), write guards D-04, date parsing, action whitelist, NUL-byte rejection |
| V6 Cryptography | no | none implemented; TLS via net/http |
| V7/V10 Logging and errors | yes | `safe` redaction as last step; no raw GitLab bodies echoed (capped detail only) |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Prompt injection via repository content (a file or diff tells the agent to commit something) | Tampering | Out of our control server-side; keep write tools' descriptions explicit and never auto-write; PAT scope and role are the boundary (accepted by author, PROJECT.md "запись без ограничений") |
| Path traversal in `file_path`/`previous_path` | Tampering | `NormalizeRepoPath` on every path, including inside `actions[]` (reject `..`, strip leading `/`, backslashes) |
| Oversized payload / DoS from a giant commit | Denial of service | <=50 actions, <=1 MiB content, 8 MiB response cap, 25 s deadline |
| Duplicate writes after ambiguous failures | Tampering / repudiation | No automatic retry; "outcome unknown" wording; SHA in success output |
| Token leakage in tool text or logs | Information disclosure | Central redactor; tests assert results never contain the token (`callText` helper already does) |
| Force overwrite / history rewrite | Tampering | No `force` param exposed; `start_branch` not exposed |

## Sources

### Primary (HIGH confidence)
- client-go v2.64.0 source in the module cache (`commits.go`, `branches.go`, `repositories.go`, `repository_files.go`, `types.go`, `gitlab.go`, `request_options.go`) — signatures, option structs, `FileActionValue`, `ErrorResponse`/`CheckResponse`/`parseError`, `Response` fields
- go-sdk v1.8.0 source (`mcp/protocol.go` `ToolAnnotations`) and executed spikes (`scratchpad/spike/*_test.go`): nullable slice schema, explicit `InputSchema` (map and patched `jsonschema.For`), validation messages, POST JSON body/URI on the wire
- Project source read this session: `internal/tools/{register,safe,format,errors,ref,file,tree}.go`, `internal/glclient/{client,errors,normalize}.go`, `internal/testutil/{fakegitlab,session}.go`, `internal/tools/{helpers,schema,tree}_test.go`, `scripts/smoke.py`, `e2e/python_smoke_test.go`; `go build/vet/test` green
- `C:\Projects\AiAdventAgentV2\agent\mcp_tools.py` — agent forwards `inputSchema` as OpenAI function `parameters` (strips `title`/`$schema`), so nested items schemas pass through

### Secondary (MEDIUM confidence)
- https://docs.gitlab.com/api/commits/ — create commit attributes (actions[], `last_commit_id`, `stats`), diff fields incl. `collapsed`/`too_large`, list-commits params, `since`/`until` ISO 8601
- https://docs.gitlab.com/api/repositories/ — compare parameters, `compare_timeout` ("commits always complete, diffs might be incomplete"), diff flags
- https://docs.gitlab.com/api/branches/ — `search` (with `^`/`$` anchors), `regex`, branch fields; create returns 201
- https://docs.gitlab.com/api/rest/ — keyset vs offset headers; `X-Total` omitted over 10 000
- gitlab-foss sources (raw, summarised by a fetch tool): `lib/api/commits.rb` (403 `forbidden!`, `render_api_error!(..., 400)`, diff paginated via Kaminari), `app/services/commits/create_service.rb` (validation messages), `app/services/files/multi_service.rb` (last_commit_id conflict text), `lib/api/branches.rb` and `app/services/branches/create_service.rb` (400 on failure, messages), `lib/gitlab/pagination/gitaly_keyset_pager.rb`
- https://forum.gitlab.com/t/rest-post-request-a-file-with-this-name-already-exists/69027 — 400 "A file with this name already exists"/"doesn't exist"
- https://gitlab.com/gitlab-org/gitlab-foss/-/work_items/60226 — 400 "You can only create or edit files when you are on a branch" for a missing branch

### Tertiary (LOW confidence)
- Exact GitLab wording for "Branch already exists" (no source line retrieved; matched by substring `already exists`)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no change from Phase 1; all APIs read from the pinned module source
- Architecture: HIGH — extends proven Phase 1 patterns; nested-schema behaviour executed
- Pitfalls: MEDIUM-HIGH — schema/diff/fake-recorder findings verified; GitLab server error strings and branch pagination mode are documented-but-not-observed (live check in Phase 4)

**Research date:** 2026-09-24
**Valid until:** 2026-10-24 (stable; recheck if go-sdk or client-go versions are bumped, since inference and struct fields are version-dependent)
