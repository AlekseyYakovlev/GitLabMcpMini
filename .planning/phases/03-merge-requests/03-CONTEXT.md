# Phase 3: Merge Requests - Context

**Gathered:** 2026-09-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Агент ведёт Merge Request на gitlab.com от просмотра до merge: `list_merge_requests`, `get_merge_request`, `get_merge_request_diffs`, `list_merge_request_notes`, `create_merge_request`, `create_merge_request_note`, `update_merge_request`, `merge_merge_request`. Всё строится на помощниках Phase 1–2 (клиент, маппер ошибок, бюджет вывода, нормализаторы, `renderDiffFiles`, тестовый харнесс).

Требования: MR-01..MR-08. Inline-комментарии к строкам diff (MRX-01), approve (MRX-02), защита merge через `sha` (MRX-03), `delete_branch` (WRT-04), assignee/reviewer/labels, issues и pipelines — вне фазы.

</domain>

<decisions>
## Implementation Decisions

### list_merge_requests и get_merge_request
- **D-01:** `list_merge_requests` без фильтров возвращает только `state=opened`, все авторы, в рамках проекта. Фильтры: `state` (`opened`/`closed`/`merged`/`all`), `source_branch`, `target_branch`, `author_username`, `search`, плюс `page`/`per_page` (футер пагинации как в Phase 1). Параметра `scope` нет.
- **D-02:** Строка списка — одна строка на MR: `!iid`, state, пометка draft, заголовок, `source→target`, автор. Формат в стиле `list_commits`/`list_branches`.
- **D-03:** `get_merge_request` возвращает короткую шапку: `iid`, заголовок, state, draft, автор, `source→target`, описание (до ~1500 символов с явной пометкой обрезки), признак конфликтов, статус head-pipeline, `web_url` и число изменённых файлов, если оно есть в ответе. Reviewers, assignees, labels, milestone, approvals не показываются.
- **D-04:** `detailed_merge_status` показывается как сырое значение плюс русское пояснение «что делать». Одна общая таблица значение→пояснение (`mergeable`, `ci_still_running`, `ci_must_pass`, `not_approved`, `draft_status`, `conflict`, `need_rebase`, `discussions_not_resolved`, `checking`, `unchecked` и др.) используется и в `get_merge_request`, и в предпроверке `merge_merge_request`. Неизвестное значение выводится как есть с общей подсказкой.
- **D-05:** Статус `checking`/`unchecked` (проверка GitLab асинхронная, часто сразу после создания MR) сообщается честно: «проверка ещё идёт, повторите через несколько секунд». Никакого ожидания и опроса внутри `get_merge_request` и `create_merge_request`, ровно один HTTP-запрос.

### get_merge_request_diffs и list_merge_request_notes
- **D-06:** `get_merge_request_diffs` использует тот же рендер, что `get_commit` из Phase 2 (`renderDiffFiles`, D-10/D-11 Phase 2): patch до ~2000 символов на файл, список файлов всегда полный, причины пустого patch (`collapsed`, `too_large`, бинарный, только переименование) указаны явно. Пагинация файлов на стороне GitLab (`page`/`per_page`, эндпоинт `/merge_requests/:iid/diffs`). Флаг `overflow` выводится отдельной строкой «часть файлов не вернулась». Отдельного `path`-фильтра и повышенного лимита patch нет.
- **D-07:** `list_merge_request_notes` показывает все заметки в одном списке, системные помечены `[system]` и выводятся короткой строкой (MR-04). Порядок «новые первыми» (desc, как у GitLab), `page`/`per_page` с футером. Тело заметки обрезается до ~1000 символов с пометкой, чтобы одна огромная заметка не вытесняла остальные.

### create_merge_request и update_merge_request
- **D-08:** `create_merge_request`: обязательны `source_branch` и `title`; `target_branch` по умолчанию ветка проекта (как `resolveRef` в Phase 2, ветка указывается в выводе); опциональны `description` и `draft`. Параметров `squash`, `remove_source_branch`, assignee/reviewer/labels в create нет. Запись не повторяется автоматически.
- **D-09:** Дубль или пустой diff в `create_merge_request` — понятная ошибка, ничего не меняется молча. Если открытый MR из этой ветки уже есть (GitLab 409), сообщение «открытый MR уже есть: !N» со ссылкой (iid из ответа GitLab или поиском по `source_branch`). Если ветки не отличаются, сообщение «нет изменений между ветками» с подсказкой про `commit_files`. Идемпотентного «вернуть существующий как успех» нет.
- **D-10:** `update_merge_request` принимает `title`, `description`, `target_branch`, `state_event` (`close`/`reopen`) и `draft` (bool). Непустое значение меняет поле, пустое означает «не менять»; очистка описания и снятие флагов через пустое значение не поддерживаются, это документируется в описании инструмента. Если ничего не передано, ошибка до обращения к GitLab. Поля простые, без указателей (D-15 Phase 1).
- **D-11:** `create_merge_request_note` — общий (не inline) комментарий: `project`, `iid`, `body`. Пустой `body` отклоняется до запроса. Успешный ответ: id заметки, `web_url` MR.

### merge_merge_request
- **D-12:** Предпроверка: сначала GET MR; если `detailed_merge_status` не `mergeable`, возвращается `isError` с русским пояснением из общей таблицы (D-04), и запрос слияния не отправляется. Слияние — единственный PUT, не повторяется автоматически. Auto-merge («влить после успешного pipeline») не поддерживается.
- **D-13:** Параметры слияния: `squash`, `remove_source_branch`, `merge_commit_message`. Не переданные не отправляются в GitLab, действуют настройки проекта/MR. `false` означает «не передано» (явно выключить уже включённый флаг нельзя, документируется). `sha`-защита остаётся в v2 (MRX-03).
- **D-14:** Отказы GitLab 405/406/409 при самом PUT (состояние изменилось между GET и PUT, конфликт, MR закрыт) мапятся в понятные тексты по образцу `writeRules` в `errors.go`. Успешный ответ: `iid`, state (`merged`), SHA слияния, ветка удалена или нет, `web_url`.
- **D-15:** Бэклог 999.2 берётся в Phase 3: в ветку `writeUnknownOutcome` (`internal/tools/errors.go`) добавляются виды сбоев Canceled/Decode/TooLarge/Other для записей. Для MR-записей (особенно merge) подсказка: «проверьте состояние через `get_merge_request` перед повтором». Позиция 999.2 в `ROADMAP.md` помечается как закрытая в Phase 3.

### Уже зафиксировано (не переоткрывать)
- **D-16:** Из Phase 1–2 действуют: имена без префикса; компактный текстовый вывод одной строкой на элемент; футеры пагинации и обрезки; бюджет вывода ~15 000 символов; ошибки как `isError` на русском; запросы записи (POST/PUT) не повторяются автоматически; `project` — числовой ID или путь; плоские схемы без `$ref`/`anyOf`/`null`, без указателей; имена ≤30, описания ≤900 символов; токен нигде не выводится; `mcp.ToolAnnotations` (чтение — `ReadOnlyHint`, запись — `DestructiveHint`; `merge_merge_request` и `update_merge_request` как destructive на усмотрение планировщика).

### Claude's Discretion
- Точные значения: лимит описания MR (~1500), тела заметки (~1000), `per_page` по умолчанию (~20), формат строк вывода и точные русские тексты таблицы `detailed_merge_status`.
- Как получать число файлов и `overflow` для diff (поля ответа `/diffs` или заголовки), и как искать существующий открытый MR при 409.
- Раскладка файлов в `internal/tools/` (по образцу `commits.go`, `write.go`, `diff.go`) и структура общей таблицы статусов.
- Что именно вернуть при `draft` в `update_merge_request` (механика префикса `Draft:` или отдельное поле client-go).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Проект и требования
- `.planning/PROJECT.md` — Core Value, ограничения (stdio, без Node, PAT, только gitlab.com), решение «запись без ограничений»
- `.planning/REQUIREMENTS.md` — MR-01..MR-08 (границы фазы), v2: MRX-01..03, WRT-04
- `.planning/ROADMAP.md` §Phase 3 — цель и 4 критерия успеха; §Backlog 999.1–999.5 (999.2 закрывается здесь)
- `CLAUDE.md` §Technology Stack — версии, паттерны ошибок, «What NOT to use»

### Решения предыдущих фаз
- `.planning/phases/01-connect-and-read/01-CONTEXT.md` — D-01..D-15: клиент, ретраи, формат вывода, бюджет, плоские схемы
- `.planning/phases/02-history-and-write/02-CONTEXT.md` — D-05..D-13: запись, вывод diff, ветка по умолчанию, тексты ошибок записи
- `.planning/phases/02-history-and-write/02-REVIEW.md` — WR-01 (999.2), CR-01 (999.1), уроки по ошибкам записи
- `.planning/phases/02-history-and-write/02-PATTERNS.md` — паттерны инструментов Phase 2 (структура обработчиков и тестов)
- `.planning/phases/01-connect-and-read/SKELETON.md` — скелет и общие помощники

### Исследование
- `.planning/research/PITFALLS.md` — §10 (diff-лимиты, пустой patch), §13 (не ретраить POST/PUT), MR-специфичные ловушки (`detailed_merge_status`, `/diffs` vs `/changes`)
- `.planning/research/FEATURES.md` — набор MR-инструментов
- `.planning/research/ARCHITECTURE.md` — таблица маппинга ошибок, паттерны пагинации и ограниченного вывода

### Код для переиспользования
- `internal/tools/diff.go` — `renderDiffFiles`, `emptyPatchKind`, `diffHeading` (рендер diff, D-06)
- `internal/tools/errors.go` — `writeRules`, `writeText`, `writeUnknownOutcome`, `withWrite` (D-14, D-15)
- `internal/tools/format.go`, `internal/tools/ref.go` — `Budget`, `ClampPaging`, `PageFooter`, `composeList`, `resolveRef`
- `internal/tools/register.go`, `internal/tools/schema_test.go` — регистрация и schema guard FND-08
- `internal/testutil/{fakegitlab,session}.go`, `e2e/`, `scripts/smoke.py` — фейковый GitLab, in-memory сессия, e2e и Python smoke

### Внешние спецификации
- https://docs.gitlab.com/api/merge_requests/ — список, создание, обновление, merge, `detailed_merge_status`, `/diffs`
- https://docs.gitlab.com/api/notes/ — заметки MR (`system`, порядок, пагинация)
- https://docs.gitlab.com/api/rest/ — пагинация, коды ошибок
- Клиент агента: `C:\Projects\AiAdventAgentV2\agent\mcp_tools.py` — как пересылается `inputSchema`, отсечка результата 20 000

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `renderDiffFiles` и `diffFile` (`internal/tools/diff.go`): пофайловый рендер с бюджетом и явными причинами пустого patch; нужен адаптер из ответа `/merge_requests/:iid/diffs` в `diffFile`.
- `Budget`, `ClampPaging`, `PageFooter`, `composeList`, `oneLine` (`internal/tools/format.go`): списки, футеры и бюджет для всех новых инструментов.
- `resolveRef`, `refLabel` (`internal/tools/ref.go`): ветка проекта по умолчанию для `target_branch` в `create_merge_request`.
- `writeRules`/`writeText`/`withWrite` (`internal/tools/errors.go`): расширяются текстами для 405/406/409 при merge, дубля MR, пустого diff; сюда же 999.2.
- `internal/glclient/normalize.go`: `NormalizeProject`; политика ретраев GET-only в `glclient` (PUT/POST не повторяются).
- `internal/testutil/{fakegitlab,session}.go`: фейк записывает запросы, можно проверять тела PUT/POST.
- `scripts/smoke.py`, `e2e/`: расширить новыми инструментами (всего 20 инструментов).

### Established Patterns
- Тонкие обработчики: нормализация входа → вызов client-go → форматтер; вывод — компактный текст с футером; мок на границе HTTP.
- Плоские входные структуры с `json:",omitempty"` и `jsonschema`-описаниями, без указателей; schema guard на все инструменты.
- Запись через `safe(d, ...)` с `DestructiveHint`, ошибки записи через `withWrite`.

### Integration Points
- `internal/tools/register.go` — регистрация 8 новых инструментов (сейчас 12).
- Клиент `d.GL` (client-go v2): `MergeRequests`, `Notes` (заметки MR); проверить в исследовании методы для `/diffs` и `detailed_merge_status`.
- Живая проверка MR на gitlab.com — Phase 4 (нужны тестовый проект и PAT со scope `api`); smoke-записи только против фейка (бэклог 999.5).

</code_context>

<specifics>
## Specific Ideas

- Сквозной сценарий агента: `create_branch` → `commit_files` → `create_merge_request` → `create_merge_request_note` → `get_merge_request` (проверка статуса) → `merge_merge_request`; каждый шаг должен давать следующему нужные `iid`/ветки в выводе.
- После `create_merge_request` статус почти всегда `checking`: сообщение должно прямо говорить агенту, что перед merge нужно повторно вызвать `get_merge_request`.
- Merge — самая необратимая операция: при неизвестном исходе нельзя молча позволять повтор без проверки состояния.

</specifics>

<deferred>
## Deferred Ideas

- Inline-комментарии к строкам diff (MRX-01), `approve_merge_request` (MRX-02), защита merge через `sha` (MRX-03) — v2.
- Auto-merge при идущем pipeline (`merge_when_pipeline_succeeds`) — потребовал бы отслеживания «запланированного» состояния; вне «Mini».
- Очистка `description` и снятие булевых флагов через update/merge (нужны указатели или отдельные флаги) — вернуться вместе с бэклогом 999.1, если понадобится.
- Assignee/reviewer/labels/milestone в create/get, `scope` в списке MR, `path`-фильтр для diff — при необходимости позже.
- `delete_branch` (WRT-04) — v2; после merge ветку убирает `remove_source_branch`.

</deferred>

---

*Phase: 03-merge-requests*
*Context gathered: 2026-09-24*
