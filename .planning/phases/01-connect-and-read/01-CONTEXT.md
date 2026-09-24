# Phase 1: Подключение и чтение проекта - Context

**Gathered:** 2026-09-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Рабочий stdio-сервер `gitlab-mcp.exe` (Go): агент (Python `mcp` 1.30.x) проходит handshake, проверяет токен (`whoami`) и читает проекты, дерево файлов и содержимое файлов на gitlab.com (`list_projects`, `get_project`, `list_repository_tree`, `get_file_contents`). В этой же фазе создаются общие помощники (stdio-харнесс, клиент GitLab, маппер ошибок, бюджет вывода) и тестовый харнесс (`httptest`, in-memory MCP-сессия, e2e по stdio, Python smoke-скрипт), которые переиспользуют фазы 2-4.

Требования: FND-01..08, UTIL-01, READ-01..04, QA-01, QA-02. Ветки, коммиты, запись и MR относятся к следующим фазам.

</domain>

<decisions>
## Implementation Decisions

### Клиент GitLab
- **D-01:** Используется `gitlab.com/gitlab-org/api/client-go/v2` (v2.64.0, версия зафиксирована в `go.mod`), а не самописный `net/http`-клиент. Это решение автора вопреки рекомендации исследования (hand-rolled). Blocker «выбор клиента» из STATE.md закрыт этим решением, отдельный спайк не нужен.
- **D-02:** Ретраи только для GET (429 и 5xx): до 2 повторов, суммарное ожидание ≤10 с с учётом `Retry-After`, общий бюджет вызова ≤25 с. Дальше `isError` с сообщением вида «повторите через N с».
- **D-03:** Запросы записи (POST/PUT/DELETE) никогда не повторяются автоматически (FND-07). Встроенные ретраи client-go (go-retryablehttp) не должны применяться к записи: политику ретраев нужно настроить или заменить, а не полагаться на значения по умолчанию.
- **D-04:** Токен нигде не должен попадать в вывод и логи (FND-03): редактирование `glpat-…` централизованно в слое логирования и в текстах ошибок, включая ошибки, которые возвращает сам client-go.

### Имена инструментов
- **D-05:** Имена без префикса: `list_projects`, `get_project`, `list_repository_tree`, `get_file_contents`, `whoami`. Агент сам добавляет `mcp__<slug>__`, поэтому префикс `gitlab_` избыточен. Имена ≤30 символов, описания ≤900, схемы плоские (FND-08). Правки REQUIREMENTS.md не нужны.

### Формат вывода
- **D-06:** Инструменты возвращают компактный текст, а не JSON: одна строка на элемент, например `123 group/proj (main) — описание`. Одиночные объекты (`get_project`, `whoami`) тоже выводятся текстом с ключевыми полями, а не сырым объектом GitLab.
- **D-07:** Футер пагинации и обрезки — одна строка-подсказка для модели. Примеры: `[page 1, per_page 20 — есть следующая страница: вызовите с page=2]`, `[вывод обрезан на 15000 символах; сузьте путь или уменьшите per_page]`. Признак следующей страницы берётся из `X-Next-Page`; `X-Total` не используется (FND-06).
- **D-08:** Общий бюджет вывода ~15 000 символов (ниже отсечки агента 20 000). Важные данные идут первыми, а обрезка всегда явная, без молчаливой потери.

### Чтение файла
- **D-09:** `get_file_contents` без диапазона строк ограничен общим бюджетом ~15 000 символов. Большой файл возвращается головой с пометкой вида «обрезан, всего N строк; используйте start_line/end_line». Параметры `start_line`/`end_line` входят в схему инструмента.
- **D-10:** Бинарный файл определяется по NUL-байту или невалидному UTF-8 в начале. Возвращается только пометка без содержимого и без base64, например «файл бинарный, N байт, blob_id …».
- **D-11:** Если `ref` не указан, `get_file_contents` и `list_repository_tree` берут ветку по умолчанию проекта (`default_branch` из `GET /projects/:id`) и указывают её в выводе. Значение `HEAD` не используется.

### Уже зафиксировано в требованиях и STACK.md (не переоткрывать)
- **D-12:** Go 1.25+, `modelcontextprotocol/go-sdk` v1.8.0, один `gitlab-mcp.exe`, `CGO_ENABLED=0`. Только stdio; stdout содержит только JSON-RPC, логи только в stderr через `slog`. Без `fmt.Print*` в stdout.
- **D-13:** Без `GITLAB_TOKEN` сервер сразу завершается с понятным сообщением в stderr и кодом 1 (fail fast). `GITLAB_URL` необязателен (по умолчанию `https://gitlab.com`) и служит швом для hermetic-тестов.
- **D-14:** Проект указывается числовым ID или путём `group/subgroup/project`; путь кодируется через один нормализатор. Тесты проверяют путь на проводе (subgroup, пробел, `#`, `+`, `%`, кириллица, `feature/x`).
- **D-15:** Ошибки 401/403/404/429/5xx и сетевые сбои приходят как `isError: true` с короткими понятными сообщениями, процесс не падает. Опциональные аргументы — простые типы с `omitempty`, не указатели (иначе в схеме появляется `["null", ...]`).

### Claude's Discretion
- Раскладка пакетов и имя Go-модуля (ориентир: `cmd/gitlab-mcp`, `internal/{config,logging,tools,server,testutil}`; отдельный слой `internal/gitlab` нужен только если оборачивание client-go в него оправдано).
- Точные значения `per_page` по умолчанию (ориентир 20, максимум 100) и лимиты на рекурсивное дерево.
- Точный формат текстовых строк вывода и набор полей проекта/дерева.
- Способ ограничения ретраев client-go (`WithCustomRetry`/`WithHTTPClient` и т. п.) и таймаутов (ориентир: HTTP-таймаут и дедлайн вызова ≈25 с).
- Расположение и структура smoke-скрипта и остальной тестовой обвязки (ориентир: `scripts/` с venv или `uv run --with mcp==1.30.0`).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Проект и требования
- `.planning/PROJECT.md` — Core Value, ограничения (stdio, без Node, PAT, только gitlab.com)
- `.planning/REQUIREMENTS.md` — FND-01..08, UTIL-01, READ-01..04, QA-01, QA-02 (границы фазы)
- `.planning/ROADMAP.md` §Phase 1 — цель и 5 критериев успеха
- `CLAUDE.md` §Technology Stack — версии, Go-специфика, «What NOT to use», паттерны обработки ошибок

### Исследование
- `.planning/research/STACK.md` — Go + go-sdk v1.8.0, эмпирика спайка, client-go v2, чего избегать
- `.planning/research/ARCHITECTURE.md` — слои, таблица маппинга ошибок, паттерны пагинации и ограниченного вывода, тестовая архитектура (4 уровня)
- `.planning/research/PITFALLS.md` — stdout-чистота, лимиты клиента агента, кодирование путей, редактирование токена
- `.planning/research/SUMMARY.md` — сводка и открытые вопросы

### Клиент (агент)
- `C:\Projects\AiAdventAgentV2\agent\mcp_client.py`, `agent\mcp_config.py`, `agent\mcp_tools.py`, `requirements.txt`, `tests\fixtures\mcp_stdio_server.py` — как агент запускает серверы (command/args/env, stderr во временный файл, connect timeout, отсечка результата 20 000, таймаут вызова 30 с)

### Внешние спецификации
- https://docs.gitlab.com/api/rest/ — пагинация, кодирование путей проектов
- https://docs.gitlab.com/api/repository_files/ — чтение файлов, `last_commit_id`, base64
- https://docs.gitlab.com/api/repositories/ — дерево репозитория
- MCP spec 2025-11-25 (transports, tools): https://modelcontextprotocol.io/specification/2025-11-25/server/tools

</canonical_refs>

<code_context>
## Existing Code Insights

Репозиторий пока без кода: только `.planning/` и `CLAUDE.md`.

### Reusable Assets
- Спайк из исследования (scratch, вне репозитория): подтверждает работу go-sdk v1.8.0 с клиентом `mcp` 1.30.0 и компиляцию client-go v2.64.0. Использовать как справку, не как код для копирования.
- `mcp.NewInMemoryTransports()` (go-sdk) — in-process тесты `tools/list` и `tools/call`.
- `httptest.Server` — фейковый GitLab для тестов клиента и инструментов.

### Established Patterns
- Пока нет; фаза 1 закладывает паттерны для фаз 2-4: тонкие обработчики, единый `toToolError`, единый форматтер (бюджет, футер обрезки/пагинации), единый нормализатор проекта, мок на границе HTTP (а не на интерфейсе клиента).

### Integration Points
- Агент AiAdventAgentV2 запускает `gitlab-mcp.exe` по stdio с `GITLAB_TOKEN` в `env` (`StdioServerParameters`); подключение к агенту вне проекта.
- `GITLAB_URL` — шов для e2e-тестов реального бинарника против фейкового GitLab.

</code_context>

<specifics>
## Specific Ideas

- Клиент GitLab выбран автором осознанно (client-go v2) вопреки рекомендации hand-rolled: исследователю и планировщику нужно явно проверить три вещи: ретраи записи, редактирование токена в ошибках и кодирование путей (subgroup, файлы с `/`, `#`, `+`, `%`, кириллица), поскольку исследование опиралось на «полный контроль» самописного клиента.
- Формат вывода — «как для модели»: короткие строки и подсказка в футере, а не сырой JSON.

</specifics>

<deferred>
## Deferred Ideas

Идей вне объёма фазы не возникло.

Заметка для Phase 4 (README): client-go v2 тянет большой граф зависимостей; проверить размер итогового `.exe` (в спайке 9-12 МБ) и что сборка остаётся без cgo.

</deferred>

---

*Phase: 01-connect-and-read*
*Context gathered: 2026-09-24*
