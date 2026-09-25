---
phase: 04-live-verification-and-delivery
verified: 2026-09-25T10:30:00Z
status: human_needed
score: 4/4 must-haves verified
has_blocking_gaps: false
overrides_applied: 0
human_verification:
  - test: "Добавить сервер в AiAdventAgentV2 по фрагменту из README (форма UI или POST /api/v1/mcp/servers) и убедиться, что агент подключился и показывает инструменты mcp__gitlab__*"
    expected: "Статус подключения connected, в списке 20 инструментов с префиксом mcp__gitlab__"
    why_human: "Реальное подключение к работающему агенту нельзя проверить из этого репозитория; поля README сверены со схемой McpServerCreate агента, а живой прогон шёл через тот же клиент mcp 1.30.0"
---

# Phase 4: Live Verification and Delivery - Verification Report

**Phase Goal:** Автор подтверждает Core Value на реальном gitlab.com и получает готовый к подключению агентом `gitlab-mcp.exe` с инструкцией
**Verified:** 2026-09-25
**Status:** human_needed (все автоматические проверки пройдены; остался один необязательный ручной шаг)
**Re-verification:** No - initial verification

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Живой прогон на тестовом проекте gitlab.com проходит весь сценарий чтение -> ветка -> коммит -> MR -> комментарий -> merge -> очистка, результат записан | VERIFIED | `LIVE-RUN.md` (коммит b4fd7be): 37 шагов, все `PASS`, ни одной строки `FAIL`. Шаги 1-20: tools/list (20 инструментов), whoami (scopes [api]), get_project, tree, get_file_contents, create_branch, commit_files (3 create), create_or_update_file (update и create), get_commit, compare_refs, create_merge_request (MR !1), create_merge_request_note (#3903154203), list notes, MR diffs, ожидание mergeable, merge_merge_request (merge commit 7f58ddc8), проверка state=merged, c.md виден в `main`. Раздел `## cleanup (вне сервера)`: ветки `-main` и `-ro` отсутствуют (404), `-draft` удалена. Это независимые данные gitlab.com (id проекта, SHA коммитов, id заметки), а не пересказ SUMMARY. |
| 2 | Проверка реальных ошибок: понятные сообщения (read-only токен на write-инструменте; несуществующий проект) | VERIFIED | Шаг 37 (C3): read-only токен (шаг 35: scopes [read_api]) -> `create_branch` -> `403: запись отклонена ... нет scope api ...`; шаг 36: чтение тем же токеном работает. Шаг 32 (E9): `404: не найдено (проект)...`. Дополнительно: 401 без эха токена (шаг 34), дубль ветки 400, дубль MR, пустой content, merge Draft MR (`draft_status`), merge уже влитого MR, 404 файла и ref. |
| 3 | `go build` даёт один `gitlab-mcp.exe` без внешних зависимостей, запуск без Node/npx | VERIFIED | `sha256sum gitlab-mcp.exe` = `955f26d0...853a71` совпадает с SHA-256 в шапке LIVE-RUN.md; размер на диске 11446784 байт совпадает; `go version -m` в LIVE-RUN.md: `CGO_ENABLED=0`, `-trimpath=true`. Файл `.exe` игнорируется git (`git check-ignore` подтверждает). Ни один файл в `internal`, `cmd`, `go.mod`, `go.sum` не новее exe, то есть исходники не менялись после сборки, которая проверена живым прогоном. `go test ./e2e -run TestBinaryHasNoExternalDependencies` выполнен лично: PASS (48 импортируемых символов, только kernel32). |
| 4 | README содержит фрагмент конфигурации агента (`command`/`env`) и требуемый scope (`api` для записи); автор может подключить сервер к агенту по README | VERIFIED (автоматическая часть); живое подключение вынесено в human verification | README.md: разделы "Токен и scope" (таблица: `read_api` для чтения, `api` обязателен для 7 write-инструментов, `write_repository` не подходит), "Подключение к AiAdventAgentV2" (таблица полей UI и JSON для `POST /api/v1/mcp/servers` с `command`, `args`, `env: {GITLAB_TOKEN}`, `cwd`, `enabled`). Сверено с `C:\Projects\AiAdventAgentV2`: маршрут `/api/v1/mcp/servers` существует (agent/main.py:1024), поля `McpServerCreate` (name, command, args, env, cwd, enabled) совпадают, префикс `mcp__` подтверждён (agent/mcp_tools.py:29). |

**Score:** 4/4 truths verified

### PLAN must-haves (04-01..04-04)

| Plan | Must-have | Status | Evidence |
|------|-----------|--------|----------|
| 04-01 | Пустой content отвергается в write-инструментах (fix backlog 999.1) | VERIFIED | `internal/tools/write.go:134` и `:233` - валидация до запроса; тесты `write_test.go:148,242,243`, `upsert_test.go:253`; вживую подтверждено шагом 24 (E3) |
| 04-02 | `scripts/live.py` существует, opt-in, с тестом использования | VERIFIED | `scripts/live.py` присутствует; `TestLiveScriptUsage` (no_arguments, no_token, help) выполнен лично: PASS. Сам скрипт отработал вживую |
| 04-03 | Тест на отсутствие внешних зависимостей + README | VERIFIED | `e2e/build_test.go` PASS; README проверен по существу (см. выше) |
| 04-04 | LIVE-RUN.md: дата, размер, SHA-256, go version -m, все PASS, cleanup, без токена | VERIFIED | Дата 2026-09-25T09:47:34Z, метка 20260925-094734 согласована с именами веток; SHA-256 совпал; 0 `FAIL`; раздел cleanup есть; `glpat`/`PRIVATE-TOKEN` не найдены, вхождения `GITLAB_TOKEN` - только текст сообщения 401; идентичность в whoami замаскирована (`[identity redacted]`) |

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md` | VERIFIED | 914 строк, содержательный транскрипт, только он в коммите b4fd7be |
| `gitlab-mcp.exe` | VERIFIED | Существует, SHA-256 совпадает, gitignored |
| `README.md` | VERIFIED | Русский, сборка, scope, конфиг агента, список 20 инструментов, ограничения, проверка |
| `scripts/live.py`, `e2e/live_script_test.go`, `e2e/build_test.go` | VERIFIED | Существуют, тесты зелёные |
| `internal/tools/write.go`, `schema.go`, golden | VERIFIED | Guard на пустой content присутствует и покрыт тестами |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| LIVE-RUN.md | gitlab-mcp.exe | SHA-256 в шапке = `sha256sum` | WIRED |
| scripts/live.py | AlekseyYakovlev/sanbox | транскрипт содержит project и реальные id/SHA | WIRED |
| README (env `GITLAB_TOKEN`) | internal/config | сервер читает токен из `GITLAB_TOKEN`, README согласован с FND-03 | WIRED |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Гермeтичный набор зелёный | `go test ./... -count=1` | e2e, config, glclient, logging, tools: ok | PASS |
| Статические проверки | `go vet ./...` | без вывода | PASS |
| Бинарь без внешних зависимостей | `go test ./e2e -run TestBinaryHasNoExternalDependencies -count=1 -v` | PASS | PASS |
| Live-скрипт: использование без сети | `go test ./e2e -run TestLiveScriptUsage -count=1 -v` | 3 подтеста PASS | PASS |
| Хэш exe | `sha256sum gitlab-mcp.exe` | совпал с LIVE-RUN.md | PASS |

Живой прогон на gitlab.com повторно не выполнялся (по условию задачи); проверялся сам файл-свидетельство.

### Probe Execution

Step 7c: SKIPPED (фаза не объявляет `probe-*.sh`).

### Requirements Coverage

Все ID из frontmatter PLAN: 04-01 [QA-03], 04-02 [QA-03], 04-03 [QA-04], 04-04 [QA-03, QA-04]. В REQUIREMENTS.md для Phase 4 назначены ровно QA-03 и QA-04; осиротевших требований нет.

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| QA-03 | 04-01, 04-02, 04-04 | Живой прогон на тестовом проекте gitlab.com: чтение, ветка, коммит, MR, комментарий, merge, очистка | SATISFIED | LIVE-RUN.md: 37/37 PASS, cleanup раздел, MR !1 влит |
| QA-04 | 04-03, 04-04 | Сборка в один `gitlab-mcp.exe` (Go) и README с фрагментом конфига агента и scope (`api` для записи) | SATISFIED | exe 11.4 МБ, CGO_ENABLED=0, README с таблицей scope и конфигом; тест зависимостей PASS |

### Anti-Patterns Found

Поиск `TBD|FIXME|XXX` по `internal`, `scripts`, `e2e`, `README.md`: значимых вхождений нет (кроме заглушек `glpat-xxxx` в README). Блокирующих антипаттернов нет.

Предупреждения из 04-REVIEW.md (0 blockers / 6 warnings / 6 info) учтены как контекст, не как пробелы: они касаются вспомогательного скрипта `scripts/live.py` (sandbox-гейт, `BaseExceptionGroup` на Python 3.10, `assert` в защите от утечки, отсутствие тестов на Python-логику) и не ломают ни одну must-have истину. Наблюдения по самому свидетельству:

| File | Observation | Severity |
|------|-------------|----------|
| LIVE-RUN.md шаг 20 | Записан PASS, хотя примечание `WARN: ветка ещё не удалена (удаление асинхронное)`; вывод самого шага при этом "ветки не найдены", а cleanup подтверждает отсутствие ветки (соответствует IN-02 ревью) | Info |
| LIVE-RUN.md шаги 16-19 | В транскрипте встречается имя проект-бота `@project_86873071_bot_...` и `expires 2027-09-24` (IN-01); это не секрет токена, но и не полностью скрытая идентичность | Info |
| 04-04-SUMMARY.md | Пишет "43 PASS"; реально 37 шагов со статусом PASS (43 - число строк с подстрокой PASS, включая таблицу критериев). Расхождение только в формулировке | Info |
| README.md | Утверждение "Запись проверяется на локальной подделке GitLab" для smoke.py корректно; текст про `live.py` соответствует его поведению | Info |

### Human Verification Required

#### 1. Подключение к AiAdventAgentV2 по README

**Test:** В UI агента (или через `POST /api/v1/mcp/servers`) добавить сервер `gitlab` с командой `C:\Projects\GitLabMcpMini\gitlab-mcp.exe` и env `GITLAB_TOKEN=<токен с api>`, нажать connect.
**Expected:** Подключение успешно, агент видит 20 инструментов `mcp__gitlab__*`.
**Why human:** Требуется запущенный агент. По условию REQUIREMENTS подключение как таковое лежит в проекте AiAdventAgentV2 (Out of Scope здесь), поэтому пункт не блокирует фазу; README сверен со схемой агента, а живой прогон использовал тот же клиент `mcp 1.30.0`.

### Gaps Summary

Пробелов нет. Все четыре критерия ROADMAP подтверждены артефактами и независимыми проверками: свидетельство живого прогона целостно (дата, SHA-256 равен хэшу exe на диске, 0 FAIL, есть cleanup, нет секретов), гермeтичный набор `go test ./... -count=1` и `go vet ./...` зелёные, README согласован с реальной схемой агента. Единственный остаточный пункт - необязательная ручная проверка реального подключения к агенту.

---

_Verified: 2026-09-25_
_Verifier: Claude (gsd-verifier)_
