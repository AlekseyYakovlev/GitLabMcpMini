# Phase 2: История и запись в репозиторий - Context

**Gathered:** 2026-09-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Агент изучает ветки, коммиты и различия и вносит изменения в репозиторий на gitlab.com: `list_branches`, `list_commits`, `get_commit` (с diff), `compare_refs`, `create_branch`, `commit_files` (несколько файлов одним коммитом) и `create_or_update_file` (один файл). Всё строится на помощниках Phase 1 (клиент, маппер ошибок, бюджет вывода, нормализаторы, тестовый харнесс).

Требования: READ-05, READ-06, READ-07, UTIL-02, WRT-01, WRT-02, WRT-03. Merge Requests (Phase 3), удаление веток, `start_branch`, бинарная запись, force-операции — вне фазы.

</domain>

<decisions>
## Implementation Decisions

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

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Проект и требования
- `.planning/PROJECT.md` — Core Value, ограничения (stdio, без Node, PAT, только gitlab.com), решение «запись без ограничений»
- `.planning/REQUIREMENTS.md` — READ-05, READ-06, READ-07, UTIL-02, WRT-01..03 (границы фазы)
- `.planning/ROADMAP.md` §Phase 2 — цель и 4 критерия успеха
- `.planning/STATE.md` §Blockers — схема вложенных `actions[]` для клиента 1.30.x
- `CLAUDE.md` §Technology Stack — версии, паттерны ошибок, «What NOT to use» (указатели в схеме, fmt.Print в stdout)

### Решения Phase 1
- `.planning/phases/01-connect-and-read/01-CONTEXT.md` — D-01..D-15: клиент, ретраи, формат вывода, ветка по умолчанию, плоские схемы
- `.planning/phases/01-connect-and-read/01-RESEARCH.md` — детали client-go v2, сигнатуры и ловушки
- `.planning/phases/01-connect-and-read/SKELETON.md` — скелет и общие помощники

### Исследование
- `.planning/research/PITFALLS.md` §Pitfall 3/9 (схемы клиента, вложенные объекты `actions[]`), §10 (diff-лимиты, пустой patch, `compare_timeout`), §11 (base64, CRLF, лимиты размера, `last_commit_id`), §13 (не ретраить POST/PUT)
- `.planning/research/FEATURES.md` — `commit_files` как примитив, `create_or_update_file` как обёртка, `compare_refs`
- `.planning/research/ARCHITECTURE.md` — таблица маппинга ошибок, паттерны пагинации и ограниченного вывода, тестовая архитектура

### Клиент (агент)
- `C:\Projects\AiAdventAgentV2\agent\mcp_tools.py`, `agent\mcp_client.py` — как агент пересылает `inputSchema` (для проверки `actions[]`), отсечка результата 20 000, таймаут вызова 30 с

### Внешние спецификации
- https://docs.gitlab.com/api/commits/ — список коммитов, коммит, diff, создание коммита (`actions[]`, `last_commit_id`)
- https://docs.gitlab.com/api/branches/ — ветки, создание ветки
- https://docs.gitlab.com/api/repositories/ — compare (`compare_timeout`, `from`/`to`)
- https://docs.gitlab.com/api/repository_files/ — GET файла, `last_commit_id`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/tools/format.go`: `Budget`, `ClampPaging`, `PageFooter`, `composeList`, `oneLine`, `OutputBudget`, `TruncatedFooter` — базовые блоки для всех списков и diff-вывода.
- `internal/tools/ref.go`: `resolveRef`, `refLabel` — ветка по умолчанию при пустом `ref` (нужна `list_commits`, `create_branch`, `create_or_update_file`).
- `internal/tools/errors.go`, `withSubject` — маппер ошибок; расширить текстами для записи (защищённая ветка, конфликт `last_commit_id`, «ветка уже существует»).
- `internal/glclient/normalize.go`: `NormalizeProject`, `NormalizeRepoPath` — единые нормализаторы для project и путей (в том числе для `file_path`/`previous_path` в `actions[]`).
- `internal/glclient/{client,retry,transport}.go` — политика ретраев GET-only; POST не повторяются (проверить, что `commit_files`/`create_branch` идут именно через неё).
- `internal/tools/register.go` — образец `mcp.AddTool(... safe(d, handler(d)))` с `Annotations`.
- `internal/tools/schema_test.go` — schema guard FND-08; новые инструменты, включая вложенный `actions[]`, должны проходить его.
- `internal/testutil/{fakegitlab,session}.go` — фейковый GitLab и in-memory сессия; фейк записывает запросы, чтобы проверять, что `actions[]` реально ушли в GitLab.
- `scripts/smoke.py`, `e2e/` — Python smoke на `mcp` 1.30.0 и e2e на реальном бинарнике; расширить новыми инструментами и проверкой приёма схемы `actions[]`.

### Established Patterns
- Тонкие обработчики: нормализация входа → `resolveRef` → вызов client-go → форматтер; вывод — компактный текст с футером.
- Мок на границе HTTP (`httptest`), а не на интерфейсе клиента; проверка провода (кодирование путей, тело POST).
- `TreeIn`/`FileIn`: плоские структуры входа с `json:",omitempty"` и `jsonschema`-описаниями, без указателей.

### Integration Points
- `internal/tools/register.go` — регистрация 7 новых инструментов: `list_branches`, `list_commits`, `get_commit`, `compare_refs`, `create_branch`, `commit_files`, `create_or_update_file`.
- Клиент `d.GL` (client-go v2): `Branches`, `Commits`, `Repositories.Compare`, `RepositoryFiles.GetFile`.
- Живая проверка записи на gitlab.com отложена на Phase 4 (нужны тестовый проект и PAT со scope `api`).

</code_context>

<specifics>
## Specific Ideas

- Главный риск фазы — вложенная схема `actions[]`: нужен тест на реальном клиенте `mcp` 1.30.0, плюс golden-снимок `tools/list` (по образцу schema guard из Phase 1).
- После `commit_files` агент должен сразу видеть, что именно записано (строка по файлам), и мог вызвать `get_commit` по SHA.
- Пометки `[protected]`/`[default]` в `list_branches` нужны, чтобы агент заранее выбирал новую ветку вместо `main`.

</specifics>

<deferred>
## Deferred Ideas

- `start_branch` в `commit_files` (создание ветки прямо в коммите) — сейчас дублирует `create_branch`; вернуться, если окажется неудобно агенту.
- Запись бинарных файлов (`encoding: base64`) и `chmod` — не нужны учебному проекту.
- `delete_branch` — уже в v2 (WRT-04).
- Опциональный `straight` для `compare_refs` — при необходимости позже.

</deferred>

---

*Phase: 02-history-and-write*
*Context gathered: 2026-09-24*
