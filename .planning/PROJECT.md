# GitLabMcpMini

## What This Is

Учебный MCP-сервер (stdio) с компактным набором основных инструментов для работы с репозиториями на gitlab.com: чтение кода, Merge Requests и запись в репозиторий. Сервер предназначен для личного использования автором и подключается к его собственному агенту (AiAdventAgentV2, Python, `mcp` SDK 1.30.x), который запускает MCP-серверы как stdio-подпроцессы.

## Core Value

Небольшой, понятный и работающий набор GitLab-инструментов, которые корректно отрабатывают на реальном gitlab.com через стандартный MCP-протокол (stdio: initialize → tools/list → tools/call).

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

(None yet — ship to validate)

### Active

<!-- Current scope. Building toward these. -->

- [ ] Сервер работает по stdio-транспорту MCP и совместим с клиентом агента AiAdventAgentV2 (initialize handshake, tools/list, tools/call)
- [ ] Авторизация в gitlab.com через Personal Access Token (переменная окружения, например `GITLAB_TOKEN`)
- [ ] Инструменты чтения кода: список/поиск проектов, дерево файлов, содержимое файла, ветки, коммиты, поиск по коду
- [ ] Инструменты Merge Requests: просмотр списка и деталей, diff, создание MR, комментарии, merge
- [ ] Инструменты записи в репозиторий: создание/изменение файлов, коммиты, создание веток
- [ ] Понятные ошибки инструментов (сетевые ошибки, 401/403/404, лимиты GitLab API) без падения сервера
- [ ] Автотесты плюс ручная проверка через stdio-клиент (скрипт или MCP Inspector) на реальном gitlab.com

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
- Язык реализации не выбран — решается на этапе исследования (кандидаты: Go как один .exe без зависимостей, либо Python с тем же SDK, что и в агенте).

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
| stdio-транспорт | Единственный транспорт, который поддерживает агент | — Pending |
| Авторизация через PAT | Один пользователь; OAuth избыточен | — Pending |
| Запись без ограничений | Решение автора; безопасность через права токена | — Pending |
| Подключение к агенту — вне проекта | Выполняется в AiAdventAgentV2 | — Pending |
| Язык реализации (Go vs Python) | Решить в исследовании | — Pending |
| «Готово» = автотесты + ручная проверка через stdio-клиент на реальном gitlab.com | Интеграция с агентом вне рамок проекта | — Pending |

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
*Last updated: 2026-09-24 after initialization*
