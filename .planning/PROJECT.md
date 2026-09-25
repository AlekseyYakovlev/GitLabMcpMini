# GitLabMcpMini

## What This Is

Учебный MCP-сервер (stdio) с компактным набором основных инструментов для работы с репозиториями на gitlab.com: чтение кода, Merge Requests и запись в репозиторий. Сервер предназначен для личного использования автором и подключается к его собственному агенту (AiAdventAgentV2, Python, `mcp` SDK 1.30.x), который запускает MCP-серверы как stdio-подпроцессы.

## Core Value

Небольшой, понятный и работающий набор GitLab-инструментов, которые корректно отрабатывают на реальном gitlab.com через стандартный MCP-протокол (stdio: initialize → tools/list → tools/call).

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

- ✓ Сервер работает по stdio-транспорту MCP и совместим с клиентом `mcp` 1.30.x (initialize, tools/list, tools/call) — Phase 1 (проверено гибридно: e2e на реальном бинарнике + Python-smoke; живой gitlab.com — Phase 4)
- ✓ Авторизация через PAT из `GITLAB_TOKEN` (fail fast, токен не попадает в вывод и логи) — Phase 1
- ✓ Понятные ошибки инструментов (401/403/404/429/5xx, сеть) без падения сервера — Phase 1
- ✓ Чтение истории: ветки, коммиты по ref/пути, коммит с diff, сравнение двух ref (`list_branches`, `list_commits`, `get_commit`, `compare_refs`) — Phase 2 (герметично; живой gitlab.com — Phase 4)
- ✓ Запись в репозиторий: создание ветки, один коммит с несколькими файлами (`commit_files`), правка одного файла (`create_or_update_file`) без повторов POST — Phase 2 (герметично; живой gitlab.com — Phase 4; 5 минорных замечаний ревью в бэклоге 999.1–999.5)
- ✓ Инструменты Merge Requests: список и детали с пояснением `detailed_merge_status`, diff постранично, комментарии, создание, комментирование, правка и безопасное влитие MR (`list_merge_requests`, `get_merge_request`, `get_merge_request_diffs`, `list_merge_request_notes`, `create_merge_request`, `create_merge_request_note`, `update_merge_request`, `merge_merge_request`; всего 20 tools) — Phase 3 (герметично; живой gitlab.com — Phase 4; замечания ревью WR-01..WR-03 в 03-REVIEW.md)

- ✓ Живой прогон на реальном gitlab.com (`scripts/live.py`, sandbox `AlekseyYakovlev/sanbox`): чтение → ветка → коммит → MR → комментарий → merge → очистка, плюс реальные ошибки (401/403/404, дубли, Draft, пустой content); один `gitlab-mcp.exe` (`CGO_ENABLED=0`) и README с конфигурацией для AiAdventAgentV2 — Phase 4 (LIVE-RUN.md; подключение к работающему агенту остаётся в 04-HUMAN-UAT.md; замечания ревью WR-01..WR-06 в 04-REVIEW.md)

- ✓ Диагностика 403 при записи: точная причина (проект запланирован к удалению, в архиве, роль токена ниже Developer), `marked_for_deletion_on` и `your_access` в `get_project`, пометки в `list_projects` — quick 260926-24l (проверено на gitlab.com; инструментов по-прежнему 20)
- ✓ Подключение к работающему агенту AiAdventAgentV2 подтверждено вручную (04-HUMAN-UAT.md) — v1.0

### Active

<!-- Current scope. Building toward these. -->

(Пусто: следующий milestone не определён. Кандидаты: поиск по коду, бэклог 999.3–999.5.)

### Out of Scope

- Подключение к агенту AiAdventAgentV2 — выполняется в рамках проекта AiAdventAgentV2, а не здесь
- Issues, Pipelines/CI, Wiki, релизы и прочие области GitLab — «Mini»: только чтение кода, MR и запись
- Self-hosted GitLab — цель только https://gitlab.com (URL можно вынести в конфиг, но не проверяется)
- OAuth-авторизация — для одного пользователя достаточно PAT, OAuth-флоу для stdio-сервера избыточно сложен
- Нестдио-транспорты (Streamable HTTP / SSE) — агент подключает серверы только по stdio
- Многопользовательность и публичное распространение — личный учебный инструмент
- Ограничения на операции записи (read-only режим, подтверждения) — по решению автора инструменты доступны без ограничений; безопасность обеспечивается правами токена

## Context

- Учебный проект: цель — разобраться, как устроены MCP-серверы, и получить рабочий инструмент.
- Клиент — агент `C:\Projects\AiAdventAgentV2`: Python, MCP SDK `mcp` 1.30.x, только stdio-подпроцессы, поддерживаются нативные бинарники (например, Go `filesystem.exe`), Node/npx не используется. Ранее агент уже проверялся на локальном Go-сервере filesystem (17 tools).
- Платформа разработки: Windows 11.
- Целевой сервис: https://gitlab.com (REST API v4).
- Состояние на v1.0: Go, `modelcontextprotocol/go-sdk` v1.8.0 + `client-go/v2` v2.64.0, один `gitlab-mcp.exe` без зависимостей (`CGO_ENABLED=0`), 20 инструментов, ~11.5k строк Go и ~1.2k Python (smoke/live-клиенты на `mcp==1.30.0`). Агент подключён и использует сервер в работе.

## Constraints

- **Transport**: stdio — агент подключает MCP-серверы только как подпроцессы со stdio
- **Runtime**: результат должен запускаться без Node/npx — нативный бинарник или Python-скрипт, запускаемый агентом
- **Auth**: Personal Access Token через переменную окружения — единственный пользователь, простейший рабочий вариант
- **Target**: только gitlab.com — заявленная цель, self-hosted не проверяется
- **Scope**: «Mini» — компактный набор инструментов, без расширения на остальные области GitLab
- **Compatibility**: совместимость с MCP SDK 1.30.x на стороне клиента (протокол, формат inputSchema)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| stdio-транспорт | Единственный транспорт, который поддерживает агент | ✓ Good |
| Авторизация через PAT | Один пользователь; OAuth избыточен | ✓ Good |
| Запись без ограничений | Решение автора; безопасность через права токена | ✓ Good (403 теперь называет причину) |
| Подключение к агенту — вне проекта | Выполняется в AiAdventAgentV2 | ✓ Good (проверено UAT) |
| Язык реализации: Go + go-sdk + client-go/v2 | Один .exe без зависимостей; спайк против `mcp` 1.30.0 прошёл | ✓ Good |
| «Готово» = автотесты + ручная проверка через stdio-клиент на реальном gitlab.com | Интеграция с агентом вне рамок проекта | ✓ Good (LIVE-RUN.md) |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/bm:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/bm:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-09-26 after v1.0 milestone*
