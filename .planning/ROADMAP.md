# Roadmap: GitLabMcpMini

## Overview

Путь от пустого репозитория до рабочего `gitlab-mcp.exe`, который агент AiAdventAgentV2 запускает как stdio-подпроцесс и который корректно работает на реальном gitlab.com. Проект в режиме Vertical MVP: каждая фаза даёт сквозную пользовательскую возможность (агент подключился и читает код; агент изучает историю и вносит изменения; агент ведёт Merge Request до merge; автор проверяет всё вживую и принимает результат). Общие помощники (stdio-харнесс, клиент GitLab, маппер ошибок, бюджет вывода) создаются в первом срезе и переиспользуются всеми последующими.

**Mode:** mvp (Vertical MVP), granularity: coarse (4 фазы вместо 6 слоистых фаз из исследования).

## Phases

**Phase Numbering:**

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [x] **Phase 1: Подключение и чтение проекта** - Рабочий stdio-сервер: агент проходит handshake, проверяет токен и читает проекты, дерево и файлы; фундамент и тестовый харнесс (completed 2026-09-24)
- [x] **Phase 2: История и запись в репозиторий** - Агент смотрит ветки, коммиты, сравнения и вносит изменения: ветка, коммит нескольких файлов, правка одного файла
 (completed 2026-09-24)
- [ ] **Phase 3: Merge Requests** - Агент просматривает MR (детали, diff, комментарии), создаёт, комментирует, правит и вливает MR
- [ ] **Phase 4: Живая проверка и поставка** - Полный сценарий на реальном gitlab.com пройден; один `gitlab-mcp.exe` и README готовы к подключению агента

## Phase Details

### Phase 1: Подключение и чтение проекта

**Goal**: Агент (Python `mcp` 1.30.x) запускает `gitlab-mcp.exe` по stdio, проходит handshake, проверяет токен и читает проекты, дерево файлов и содержимое файлов на gitlab.com
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: FND-01, FND-02, FND-03, FND-04, FND-05, FND-06, FND-07, FND-08, UTIL-01, READ-01, READ-02, READ-03, READ-04, QA-01, QA-02
**Success Criteria** (what must be TRUE):

  1. Smoke-скрипт на Python `mcp` 1.30.0 (`stdio_client` с `env`) проходит `initialize` и `tools/list` без обращений к сети при старте, а stdout за всю сессию (включая ошибочные вызовы) содержит только JSON-RPC; процесс завершается при закрытии stdin
  2. Без `GITLAB_TOKEN` сервер сразу завершается с понятным сообщением в stderr; с токеном вызов `whoami` возвращает текущего пользователя, а сам токен нигде не появляется в выводе и логах
  3. Пользователь может вызвать `list_projects`, `get_project`, `list_repository_tree` и `get_file_contents`, указав проект числовым ID или путём `group/subgroup/project`; списки показывают `page`/`per_page` и признак следующей страницы; большой или бинарный файл и слишком длинный вывод дают явную пометку об обрезке, а не молчаливую потерю
  4. Ошибки 401/403/404/429/5xx и сетевые сбои приходят как результат `isError` с понятным сообщением, сервер не падает; вызов укладывается в ~25 с; имена инструментов ≤30 символов, описания ≤900, схемы плоские
  5. Автотесты (`httptest` с фейковым GitLab, in-memory MCP-сессия, e2e по stdio на реальном бинарнике) проходят и проверяют wire-путь кодирования проекта и путей файлов; харнесс готов для расширения в следующих фазах

**Plans**: 6 plans

Plans:
**Wave 1**

- [x] 01-01-PLAN.md — Walking skeleton: module, config fail-fast, redacting logger, stdout guard, whoami via client-go, e2e on real binary + Python smoke

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 01-02-PLAN.md — Client hardening: GET-only retries with Retry-After, no write retries, timeouts, body cap, error classification/wording, project/path normalisers with wire tests
- [x] 01-03-PLAN.md — Test harness: in-memory session helper, schema guard (FND-08), panic/redaction tests, raw stdout purity + EOF test

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 01-04-PLAN.md — Output budget/pagination footer + list_projects and get_project

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 01-05-PLAN.md — Default-branch resolution + list_repository_tree

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 01-06-PLAN.md — get_file_contents (base64, ranges, truncation, binary/oversize) + real-binary wire e2e + five-tool Python smoke

### Phase 2: История и запись в репозиторий

**Goal**: Агент изучает ветки, коммиты и различия и вносит изменения в репозиторий: создаёт ветку и коммитит один или несколько файлов
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: READ-05, READ-06, READ-07, UTIL-02, WRT-01, WRT-02, WRT-03
**Success Criteria** (what must be TRUE):

  1. Пользователь может получить список веток, список коммитов по ref/пути, конкретный коммит вместе с diff и сравнение двух веток/коммитов (`compare_refs`), с пагинацией и без молчаливой обрезки
  2. Пользователь может создать ветку от указанного ref (`create_branch`) и увидеть её в `list_branches`
  3. Пользователь может одним коммитом создать/изменить/удалить несколько файлов (`commit_files`, `actions[]`, без `force`) и увидеть новый коммит через `get_commit`; схема вложенных `actions[]` принимается клиентом 1.30.x
  4. Пользователь может создать или обновить один файл (`create_or_update_file`); при «уже существует»/«не существует» и при защищённой ветке или недостаточном scope приходит понятное сообщение; запросы записи не повторяются автоматически

**Plans**: 5 plans

Plans:
**Wave 1**

- [x] 02-01-PLAN.md — Branches slice: list_branches + create_branch, write-aware error wording, fake body capture, 7-tool e2e/smoke

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 02-02-PLAN.md — History slice: list_commits + get_commit with budgeted diff rendering (explicit too_large/collapsed reasons)

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 02-03-PLAN.md — compare_refs with server-side file paging, compare_timeout/same-ref states

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 02-04-PLAN.md — commit_files: explicit nested actions[] schema, guards, single POST, per-file +/-, golden tools/list, real-client proof

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 02-05-PLAN.md — create_or_update_file: GET-based create/update, last_commit_id conflict, CRLF warning, all 12 tools e2e

### Phase 3: Merge Requests

**Goal**: Агент ведёт Merge Request от просмотра до merge: список, детали, diff, комментарии, создание, правка и влитие
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: MR-01, MR-02, MR-03, MR-04, MR-05, MR-06, MR-07, MR-08
**Success Criteria** (what must be TRUE):

  1. Пользователь может получить список MR с фильтрами по состоянию и веткам и детали MR по `iid`, включая `detailed_merge_status`
  2. Пользователь может постранично читать diff MR по файлам; флаги `collapsed`/`too_large`/`overflow` показываются явно и пустой patch не выглядит как «нет изменений»; комментарии MR читаются с пометкой системных
  3. Пользователь может создать MR из ветки с изменениями (из Phase 2), добавить общий комментарий и изменить заголовок, описание или состояние MR
  4. Пользователь может влить MR (`merge_merge_request`): сервер проверяет `detailed_merge_status` и при отказе (405/406/409, непрошедшие проверки) возвращает понятное объяснение, а не сырую ошибку

**Plans**: 5 plans

Plans:
**Wave 1**

- [x] 03-01-PLAN.md — MR read slice: list_merge_requests (state=opened by default) + get_merge_request with the shared detailed_merge_status table, 14-tool e2e/smoke

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 03-02-PLAN.md — MR review slice: get_merge_request_diffs (renderDiffFiles, overflow from changes_count) + list_merge_request_notes ([system]), 16 tools

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 03-03-PLAN.md — create_merge_request (compare pre-check, Draft prefix, 409 !N), MR write error layer + backlog 999.2, loopback-only smoke writes, 17 tools

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 03-04-PLAN.md — create_merge_request_note + update_merge_request (optional fields, state_event, draft), 19 tools

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 03-05-PLAN.md — merge_merge_request (GET pre-check gate, single PUT, 400/401/403/405/406/409/422 wording), full MR flow test, all 20 tools

### Phase 4: Живая проверка и поставка

**Goal**: Автор подтверждает Core Value на реальном gitlab.com и получает готовый к подключению агентом `gitlab-mcp.exe` с инструкцией
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: QA-03, QA-04
**Success Criteria** (what must be TRUE):

  1. Живой прогон на тестовом проекте gitlab.com проходит полный сценарий: чтение файлов, создание ветки, коммит, создание MR, комментарий, merge и очистка; результат зафиксирован
  2. Проверка реальных ошибок подтверждает понятные сообщения (например, токен только на чтение при вызове инструмента записи; несуществующий проект)
  3. `go build` выдаёт один `gitlab-mcp.exe` без внешних зависимостей, запускаемый без Node/npx
  4. README содержит фрагмент конфигурации агента (`command`/`env`) и требуемые scope токена (`api` для записи); автор может подключить сервер к AiAdventAgentV2 по README

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Подключение и чтение проекта | 6/6 | Complete   | 2026-09-24 |
| 2. История и запись в репозиторий | 5/5 | Complete   | 2026-09-24 |
| 3. Merge Requests | 3/5 | In Progress|  |
| 4. Живая проверка и поставка | 0/TBD | Not started | - |

## Backlog

### Phase 999.1: commit_files: create/update без content не должен коммитить пустой файл (BACKLOG)

**Goal:** Сделать `ActionIn.Content` указателем (`*string`), отклонять create/update без `content` до запроса, разрешить явную пустую строку; тест «нет запроса без content». Источник: 02-REVIEW CR-01, 02-VERIFICATION. Рекомендуется закрыть до живого прогона Phase 4 (тихая деструктивная запись).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.2: Тексты ошибок записи: «запись могла примениться» для Canceled/Decode/TooLarge/Other (CLOSED in Phase 3, plan 03-03)

**Goal:** Добавить эти виды сбоев в ветку `writeUnknownOutcome` в `errors.go` (`writeText`), чтобы модель не повторяла запись и не создавала дубль коммита. Источник: 02-REVIEW WR-01.
**Closed:** writeText covers Canceled/Decode/TooLarge/Other with op-specific hints (Phase 3, D-15).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.3: create_or_update_file: описание завышает защиту от чужих правок (BACKLOG)

**Goal:** Либо добавить входной `last_commit_id`, либо исправить описание инструмента: сервер сам перечитывает файл перед POST. Источник: 02-REVIEW WR-02 (и WR-05: перезапись бинарного файла текстом как обычное «updated»).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.4: compare_refs: переполнение при огромном page (BACKLOG)

**Goal:** Ограничить `page` (и `page+1` в `PageFooter`), чтобы вместо recovered-паники в `safe` возвращалось понятное сообщение «страница вне диапазона». Источник: 02-REVIEW WR-03.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.5: smoke.py: записи только против loopback/фейка, не против gitlab.com (BACKLOG)

**Goal:** Запускать `create_branch`, `commit_files` и перезапись README только при loopback-хосте или явном флаге. Иначе любой `--base-url`, включая gitlab.com, считается «hermetic». Обязательно до Phase 4. Источник: 02-REVIEW WR-04.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)
