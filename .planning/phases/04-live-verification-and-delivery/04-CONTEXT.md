# Phase 4: Живая проверка и поставка - Context

**Gathered:** 2026-09-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Автор подтверждает Core Value на реальном gitlab.com и получает готовый к подключению агентом `gitlab-mcp.exe` с инструкцией: живой сквозной прогон (чтение → ветка → коммит → MR → комментарий → merge → очистка) через тот же `mcp` 1.30.0 stdio-клиент, что использует агент; проверка реальных ошибок; воспроизводимая сборка одного `.exe` без внешних зависимостей; README на русском.

Требования: QA-03, QA-04. Новые инструменты (в т. ч. `delete_branch`, WRT-04), `search_code`, inline-комментарии MR, `sha`-защита merge, подключение к AiAdventAgentV2 (это проект агента) — вне фазы. Набор из 20 инструментов не меняется.

</domain>

<decisions>
## Implementation Decisions

### Формат живого прогона
- **D-01:** Живой прогон — отдельный скрипт `scripts/live.py` на том же `mcp==1.30.0` (`stdio_client` с `env`, как в `scripts/smoke.py`). `scripts/smoke.py` остаётся герметичным и не получает возможности писать в gitlab.com: защита 999.5 (`is_loopback`) не ослабляется.
- **D-02:** `live.py` — явный opt-in: без обязательных `--project GROUP/PROJECT` и токена `GITLAB_TOKEN` не стартует; ничего не пишет по умолчанию «случайно». Сообщения запуска и `--help` на русском/английском в стиле `smoke.py`; stdout/stderr переключаются на UTF-8 (как в `smoke.py`).
- **D-03:** Результат фиксируется транскриптом: скрипт пишет `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md` (шаги, аргументы вызовов, тексты ответов, PASS/FAIL/SKIPPED по каждому шагу, дата, размер и SHA `.exe`). Токены редактируются и в файл не попадают (`check(token not in text)` как в `smoke.py`). Файл коммитится как доказательство для verifier.
- **D-04:** Баги, найденные живым прогоном (расхождение реального gitlab.com с фейком, неверные тексты и т. п.), чинятся в Phase 4: gap-closure планы или встроенные правки, прогон повторяется до зелёного. Для каждого фикса — герметичный тест на фейке, воспроизводящий проблему.

### Тестовый проект, токены, очистка
- **D-05:** Живая запись идёт в постоянный пустой приватный sandbox-проект на gitlab.com (создаёт автор один раз, в `main` лежит `README.md`). Скрипт переиспользует его при каждом прогоне; имена веток уникальные (`live/<timestamp>-…`), чтобы повторные прогоны не сталкивались. Реальные проекты автора не затрагиваются. Закрывает blocker «нужен проект от автора» после того, как автор создаст проект.
- **D-06:** Два токена через env: `GITLAB_TOKEN` (scope `api`) обязателен; `GITLAB_TOKEN_READONLY` (scope `read_api`) необязателен — без него шаг «запись read-only токеном → понятный 403» пропускается с явной пометкой SKIPPED в транскрипте (не молчаливо). Токены нигде не логируются и не коммитятся; проект и токены не хардкодятся в репозитории.
- **D-07:** Очистка делает сам скрипт в `finally` напрямую через REST (`urllib`, заголовок `PRIVATE-TOKEN`), а не через сервер: закрывает свои MR и удаляет свои ветки `live/<ts>-*` (в том числе при падении на полпути). Это помечается в транскрипте как «cleanup (вне сервера)». Инструмент `delete_branch` в сервер не добавляется (WRT-04 остаётся в v2); при успешном merge ветку в основном сценарии убирает `remove_source_branch`.
- **D-08:** Реальные ошибки проверяются на gitlab.com, помимо критерия (запись read-only токеном, несуществующий проект): невалидный токен → понятный 401 без утечки токена; несуществующая ветка/файл → 404; отказ merge (Draft MR / неполная проверка) — `merge_merge_request` объясняет причину по `detailed_merge_status` без сырой ошибки; дубль — `create_branch` на уже существующую ветку и `create_merge_request` на ветку с открытым MR (409 → «открытый MR уже есть: !N»). Ожидаемые сообщения сверяются с русскими текстами из `internal/tools/errors.go`/`mr_status.go`.
- **D-09:** Основной сценарий закрывает связку между шагами: `iid`/ветки из вывода одного инструмента подаются следующему; перед merge после `create_merge_request` вызывается `get_merge_request` (статус `checking` → повтор через несколько секунд, как подсказывает сервер).

### Бэклог 999.x до живого прогона
- **D-10:** В Phase 4 закрывается только 999.1: `commit_files` create/update без `content` отклоняется до запроса. Указатель `*string` не вводится (иначе в схеме `["null","string"]`, нарушение FND-08): `content` остаётся `string` с `omitempty`; пустой `content` при `create`/`update` — ошибка до обращения к GitLab. Создание совершенно пустого файла не поддерживается (обход: один перевод строки) — документируется в описании инструмента и README. Тест «нет запроса к GitLab без content» обязателен; golden-снимок `tools/list` и schema guard остаются зелёными. 999.1 в `ROADMAP.md` помечается закрытым.
- **D-11:** 999.3, 999.4 и статус 999.5 в Phase 4 не берутся (остаются в бэклоге). README при этом не должен завышать защиту `create_or_update_file`: сервер сам перечитывает файл перед POST, защиты от чужих правок между чтением и записью нет (сверить формулировку с 999.3).

### Поставка: сборка и README
- **D-12:** Каноническая сборка: `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`. Отдельного build-скрипта и Makefile нет; команда — в README. `gitlab-mcp.exe` остаётся в корне и в `.gitignore` (в репозиторий и релизы не кладётся); агенту указывается абсолютный путь.
- **D-13:** Критерий «без внешних зависимостей, без Node/npx» подтверждается проверкой в рамках прогона (например `go version -m gitlab-mcp.exe` для `CGO_ENABLED=0` и отсутствие внешних DLL) и фиксируется в `LIVE-RUN.md` вместе с размером файла (ориентир из спайка: 9–12 МБ). Форма проверки (отдельный шаг `live.py` или тест) — на усмотрение планировщика.
- **D-14:** README — на русском, компактный. Разделы: что это; сборка (D-12); токен и scope (`api` для записи — обязательно для `commit_files`/`create_or_update_file`/MR-записей; `read_api` достаточно только для чтения; `write_repository` недостаточен); подключение к агенту — поля `command` (абсолютный путь к `gitlab-mcp.exe`), `args` (`[]`, JSON-массив), `env` (`{"GITLAB_TOKEN": "..."}`, JSON-словарь; опционально `GITLAB_URL`) и готовый JSON-фрагмент, потому что агент хранит серверы как поля в БД через UI, а не файлом `mcpServers`; таблица 20 инструментов (по одной строке); ограничения («Mini», запись без ограничений — безопасность обеспечивается правами токена, пустой файл не создаётся, `delete_branch` нет, `sha`-защиты merge нет, только gitlab.com); как проверить (`scripts/smoke.py`, `scripts/live.py`, `go test ./...`).

### Уже зафиксировано (не переоткрывать)
- **D-15:** Из Phase 1–3 действуют: Go + `go-sdk` v1.8.0 + `client-go` v2.64.0; только stdio, stdout только JSON-RPC; токен нигде не выводится; запись (POST/PUT) не повторяется автоматически; ошибки как `isError` на русском; плоские схемы без указателей; 20 инструментов без префикса; `go test -race` не используется (нет cgo).

### Claude's Discretion
- Структура `live.py` (шаги, хелперы, формат PASS/FAIL/SKIPPED), точные имена веток/MR/заголовков сценария, паузы и число повторов для статуса `checking`.
- Точные формулировки описания `commit_files` про пустой `content` и разделов README.
- Форма проверки сборки (D-13) и способ вычисления SHA/размера `.exe` в транскрипте.
- Раскладка задач и волн; нужны ли gap-closure планы после первого живого прогона (D-04).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Проект и требования
- `.planning/PROJECT.md` — Core Value, ограничения (stdio, без Node, PAT, только gitlab.com), решение «запись без ограничений», критерий «готово»
- `.planning/REQUIREMENTS.md` — QA-03, QA-04 (границы фазы); v2: SRCH-01, MRX-*, UX-*, WRT-04
- `.planning/ROADMAP.md` §Phase 4 — цель и 4 критерия успеха; §Backlog 999.1 (закрывается здесь), 999.3–999.5 (остаются)
- `.planning/STATE.md` §Blockers — «нужны тестовый проект и PAT со scope `api`»
- `CLAUDE.md` §Technology Stack — версии, «What NOT to use» (указатели в схеме), сборка `go build -trimpath -ldflags "-s -w"`, scope PAT (`api`), запрет `go test -race`

### Решения предыдущих фаз
- `.planning/phases/01-connect-and-read/01-CONTEXT.md` — D-01..D-15, заметка для Phase 4 про размер `.exe`
- `.planning/phases/02-history-and-write/02-CONTEXT.md` — запись, тексты ошибок записи, D-04 предохранители
- `.planning/phases/02-history-and-write/02-REVIEW.md` — CR-01 (999.1), WR-02 (999.3), WR-04 (999.5)
- `.planning/phases/03-merge-requests/03-CONTEXT.md` — D-04 таблица `detailed_merge_status`, D-09/D-12/D-14 ошибки MR, сквозной сценарий агента
- `.planning/phases/03-merge-requests/03-REVIEW.md` — замечания WR-01..WR-03

### Исследование
- `.planning/research/PITFALLS.md` — лимиты клиента агента, ретраи записи, ловушки MR
- `.planning/research/STACK.md` — эмпирика спайка (размер бинарника, handshake)

### Код и скрипты
- `scripts/smoke.py` — образец `stdio_client`, `check`, `walk_schema`, `is_loopback`, UTF-8 вывода; не расширять записью в gitlab.com
- `internal/tools/write.go`, `internal/tools/schema_test.go`, `internal/tools/testdata/` — `commit_files`, `ActionIn`, schema guard и golden-снимок (999.1)
- `internal/tools/errors.go`, `internal/tools/mr_status.go` — русские тексты ошибок для сверки на живом прогоне
- `internal/testutil/{fakegitlab,session}.go`, `e2e/` — герметичные тесты для фиксов и регрессий (D-04)
- `cmd/gitlab-mcp/main.go`, `go.mod` — точка сборки
- `.gitignore` — `*.exe` игнорируется

### Клиент (агент)
- `C:\Projects\AiAdventAgentV2\agent\mcp_config.py`, `agent\mcp_client.py`, `agent\mcp_tools.py`, `requirements.txt` — как агент хранит и запускает серверы (`command`/`args_json`/`env_json` в БД, значения `env` маскируются в UI, stderr во временный файл), отсечка результата 20 000, таймаут вызова 30 с

### Внешние спецификации
- https://docs.gitlab.com/security/tokens/access_token_scopes/ — scope PAT (`api`, `read_api`, `write_repository`)
- https://docs.gitlab.com/api/merge_requests/ — `detailed_merge_status`, merge, закрытие MR
- https://docs.gitlab.com/api/branches/ — удаление ветки в cleanup-скрипте (`DELETE /projects/:id/repository/branches/:branch`)
- https://docs.gitlab.com/api/rest/ — пагинация, коды ошибок

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `scripts/smoke.py`: паттерн `stdio_client`/`ClientSession`, `check()`, `walk_schema`, `result_text`, `default_exe()`, проверка «токен не в выводе», UTF-8 stdout/stderr; live-чтение (без `--base-url`) уже есть, запись заблокирована `is_loopback`. Основа для `live.py`, но код не смешивать.
- `e2e/`: e2e на реальном бинарнике и Python-smoke (`python_smoke_test.go`); при добавлении `live.py` следить, чтобы герметичные тесты его не запускали.
- `internal/testutil/{fakegitlab,session}.go`: фейк записывает запросы — использовать для регрессионных тестов фиксов D-04 и теста 999.1.
- `internal/tools/{write,errors,mr_status}.go`: готовые русские тексты для сверки на живом прогоне.

### Established Patterns
- Мок на границе HTTP, тонкие обработчики, плоские схемы без указателей, schema guard + golden-снимок на все инструменты.
- Скрипты на Python — `mcp==1.30.0`, запуск через `uv run` или venv в `scripts/`, без включения в сервер.

### Integration Points
- Живой gitlab.com через `GITLAB_TOKEN` (api) и sandbox-проект; `GITLAB_URL` не задаётся (по умолчанию `https://gitlab.com`).
- Итоговая интеграция с AiAdventAgentV2 — вне проекта, но README должен быть достаточен для подключения по полям `command`/`args`/`env`.
- `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md` — артефакт доказательства для verifier.

</code_context>

<specifics>
## Specific Ideas

- Порядок сценария `live.py`: whoami → чтение (проект, дерево, файл) → `create_branch` → `commit_files` (несколько файлов) → `create_or_update_file` → `create_merge_request` → `create_merge_request_note` → `get_merge_request` (ждать `mergeable`) → `merge_merge_request` с `remove_source_branch` → `list_branches`/`get_commit` для сверки результата → блок ошибок (D-08) → cleanup (D-07).
- Автору перед первым прогоном нужно: создать приватный sandbox-проект с `README.md` в `main`, выпустить PAT `api` (и, по желанию, второй `read_api`), выставить `GITLAB_TOKEN`/`GITLAB_TOKEN_READONLY`. Это зафиксировать в начале плана как human-action.
- В README явно сказать: запись без ограничений (решение автора), безопасность — правами токена.

</specifics>

<deferred>
## Deferred Ideas

- `delete_branch` (WRT-04) — v2; в Phase 4 очистку делает скрипт через REST.
- Запись пустого файла (`content=""`) — потребует указателя или ручной правки схемы; вернуться вместе с бэклогом, если понадобится.
- 999.3 (описание `create_or_update_file`/бинарная перезапись), 999.4 (переполнение `page` в `compare_refs`), 999.5 (закрытие пункта после подтверждения автора) — остаются в бэклоге.
- `search_code` (SRCH-01), inline-комментарии MR (MRX-01), `approve` (MRX-02), `sha`-защита merge (MRX-03), tool annotations (UX-01), проект по URL (UX-02), structured output (UX-03) — v2.
- Build-скрипт/Makefile, публикация `.exe` в репозитории или релизе, RU+EN README — не нужны для личного инструмента.
- Подключение к AiAdventAgentV2 и проверка внутри самого агента — выполняется в проекте агента.

</deferred>

---

*Phase: 04-live-verification-and-delivery*
*Context gathered: 2026-09-24*
