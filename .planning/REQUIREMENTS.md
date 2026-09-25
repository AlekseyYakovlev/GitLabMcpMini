# Requirements: GitLabMcpMini

**Defined:** 2026-09-24
**Core Value:** Небольшой, понятный и работающий набор GitLab-инструментов, которые корректно отрабатывают на реальном gitlab.com через стандартный MCP-протокол (stdio: initialize → tools/list → tools/call).

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Server foundation

- [x] **FND-01**: Сервер работает как stdio MCP-сервер: проходит `initialize` и `tools/list` с клиентом `mcp` 1.30.x без обращений к сети при старте
- [x] **FND-02**: В stdout идёт только JSON-RPC, логи только в stderr; процесс завершается при закрытии stdin
- [x] **FND-03**: Токен берётся из `GITLAB_TOKEN`; при его отсутствии сервер сразу завершается с понятным сообщением в stderr; токен не попадает ни в вывод, ни в логи
- [x] **FND-04**: Проект можно указать числовым ID или путём `group/subgroup/project`
- [x] **FND-05**: Ошибки GitLab и сети (401/403/404/405/409/422/429/5xx) превращаются в результат `isError` с понятным сообщением; сервер не падает
- [x] **FND-06**: Списочные инструменты принимают `page`/`per_page` и сообщают, есть ли следующая страница; вывод ограничен ~15 000 символов с явной пометкой об обрезке
- [x] **FND-07**: Вызов укладывается в ~25 с; запросы записи (POST/PUT) никогда не повторяются автоматически
- [x] **FND-08**: Имена инструментов ≤30 символов, описания ≤900, схемы плоские (без `$ref`/`anyOf`/`null`)

### Чтение кода

- [x] **READ-01**: `list_projects` — проекты пользователя (по умолчанию `membership=true`)
- [x] **READ-02**: `get_project` — данные проекта, включая ветку по умолчанию
- [x] **READ-03**: `list_repository_tree` — дерево файлов по пути и ref, с пагинацией
- [x] **READ-04**: `get_file_contents` — текст файла по ref (декодирование base64, диапазон строк, защита от больших и бинарных файлов)
- [x] **READ-05**: `list_branches` — ветки проекта
- [x] **READ-06**: `list_commits` — коммиты по ref/пути
- [x] **READ-07**: `get_commit` — коммит вместе с diff

### Merge Requests

- [x] **MR-01**: `list_merge_requests` — список MR с фильтрами по состоянию и веткам
- [x] **MR-02**: `get_merge_request` — детали MR, включая `detailed_merge_status`
- [x] **MR-03**: `get_merge_request_diffs` — постраничный diff по файлам; флаги `collapsed`/`too_large`/`overflow` показываются явно
- [x] **MR-04**: `list_merge_request_notes` — комментарии MR (системные помечены)
- [x] **MR-05**: `create_merge_request` — создание MR из ветки
- [x] **MR-06**: `create_merge_request_note` — общий комментарий к MR
- [x] **MR-07**: `merge_merge_request` — merge с проверкой `detailed_merge_status` и понятными сообщениями об отказе
- [x] **MR-08**: `update_merge_request` — правка заголовка, описания, состояния MR

### Запись в репозиторий

- [x] **WRT-01**: `create_branch` — создание ветки от ref
- [x] **WRT-02**: `commit_files` — один коммит с несколькими файлами через Commits API (`actions[]`), без `force`
- [x] **WRT-03**: `create_or_update_file` — создание или обновление одного файла (обёртка над `commit_files`)

### Вспомогательные

- [x] **UTIL-01**: `whoami` — текущий пользователь по токену (быстрая проверка токена и прав)
- [x] **UTIL-02**: `compare_refs` — сравнение двух веток/коммитов

### Качество и поставка

- [x] **QA-01**: Автотесты: `httptest` с фейковым GitLab, in-memory MCP-сессия, e2e по stdio на реальном бинарнике, включая проверку чистоты stdout
- [x] **QA-02**: Smoke-скрипт на Python `mcp` 1.30.0 (`stdio_client` с `env`), повторяющий подключение агента
- [x] **QA-03**: Живой прогон на тестовом проекте gitlab.com: чтение, ветка, коммит, MR, комментарий, merge, очистка
- [x] **QA-04**: Сборка в один `gitlab-mcp.exe` (Go) и README с фрагментом конфигурации для агента и требуемыми scope токена (`api` для записи)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Read / MR extras

- **SRCH-01**: `search_code` — поиск по коду в рамках проекта (глобальный/групповой поиск на Free недоступен)
- **MRX-01**: Inline-комментарии к строкам diff (нужен `diff_refs`)
- **MRX-02**: `approve_merge_request`
- **MRX-03**: Защита merge через `sha`

### Protocol / UX extras

- **UX-01**: Tool annotations (`readOnlyHint` и др.)
- **UX-02**: Указание проекта по URL gitlab.com
- **UX-03**: Structured output
- **WRT-04**: `delete_branch`

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Подключение к агенту AiAdventAgentV2 | Выполняется в рамках проекта AiAdventAgentV2 |
| Issues, work items | «Mini»: только чтение кода, MR и запись |
| Pipelines / CI, Wiki, релизы, группы | Вне заявленного объёма |
| Self-hosted GitLab | Цель только gitlab.com |
| OAuth-авторизация | Для одного пользователя достаточно PAT |
| HTTP / SSE транспорты | Агент подключает серверы только по stdio |
| Read-only режим | Решение автора: запись без ограничений, безопасность через права токена |
| Универсальный `gitlab_request` | Обходит типизацию и контроль вывода |
| Force-push и деструктивные операции (удаление проекта и т. п.) | Слишком опасно, не нужно для основных сценариев |
| Глобальный / групповой поиск по коду | Недоступен на Free-плане |
| Создание и форк репозиториев | Вне заявленного объёма |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| FND-01 | Phase 1 | Complete |
| FND-02 | Phase 1 | Complete |
| FND-03 | Phase 1 | Complete |
| FND-04 | Phase 1 | Complete |
| FND-05 | Phase 1 | Complete |
| FND-06 | Phase 1 | Complete |
| FND-07 | Phase 1 | Complete |
| FND-08 | Phase 1 | Complete |
| UTIL-01 | Phase 1 | Complete |
| READ-01 | Phase 1 | Complete |
| READ-02 | Phase 1 | Complete |
| READ-03 | Phase 1 | Complete |
| READ-04 | Phase 1 | Complete |
| QA-01 | Phase 1 | Complete |
| QA-02 | Phase 1 | Complete |
| READ-05 | Phase 2 | Complete |
| READ-06 | Phase 2 | Complete |
| READ-07 | Phase 2 | Complete |
| UTIL-02 | Phase 2 | Complete |
| WRT-01 | Phase 2 | Complete |
| WRT-02 | Phase 2 | Complete |
| WRT-03 | Phase 2 | Complete |
| MR-01 | Phase 3 | Complete |
| MR-02 | Phase 3 | Complete |
| MR-03 | Phase 3 | Complete |
| MR-04 | Phase 3 | Complete |
| MR-05 | Phase 3 | Complete |
| MR-06 | Phase 3 | Complete |
| MR-07 | Phase 3 | Complete |
| MR-08 | Phase 3 | Complete |
| QA-03 | Phase 4 | Complete |
| QA-04 | Phase 4 | Complete |

**Coverage:**
- v1 requirements: 32 total
- Mapped to phases: 32
- Unmapped: 0 ✓

---
*Requirements defined: 2026-09-24*
*Last updated: 2026-09-24 after roadmap creation (traceability filled)*
