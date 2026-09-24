# Phase 4: Живая проверка и поставка - Research

**Researched:** 2026-09-25
**Domain:** live end-to-end verification of an existing Go MCP stdio server against real gitlab.com (Python `mcp` 1.30.0 client script), Windows single-binary delivery, Russian README
**Confidence:** MEDIUM-HIGH (local code, agent code and toolchain facts are verified first-hand; a few gitlab.com response details can only be confirmed by the live run itself and are flagged)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Формат живого прогона**
- **D-01:** Живой прогон — отдельный скрипт `scripts/live.py` на том же `mcp==1.30.0` (`stdio_client` с `env`, как в `scripts/smoke.py`). `scripts/smoke.py` остаётся герметичным и не получает возможности писать в gitlab.com: защита 999.5 (`is_loopback`) не ослабляется.
- **D-02:** `live.py` — явный opt-in: без обязательных `--project GROUP/PROJECT` и токена `GITLAB_TOKEN` не стартует; ничего не пишет по умолчанию «случайно». Сообщения запуска и `--help` на русском/английском в стиле `smoke.py`; stdout/stderr переключаются на UTF-8 (как в `smoke.py`).
- **D-03:** Результат фиксируется транскриптом: скрипт пишет `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md` (шаги, аргументы вызовов, тексты ответов, PASS/FAIL/SKIPPED по каждому шагу, дата, размер и SHA `.exe`). Токены редактируются и в файл не попадают (`check(token not in text)` как в `smoke.py`). Файл коммитится как доказательство для verifier.
- **D-04:** Баги, найденные живым прогоном (расхождение реального gitlab.com с фейком, неверные тексты и т. п.), чинятся в Phase 4: gap-closure планы или встроенные правки, прогон повторяется до зелёного. Для каждого фикса — герметичный тест на фейке, воспроизводящий проблему.

**Тестовый проект, токены, очистка**
- **D-05:** Живая запись идёт в постоянный пустой приватный sandbox-проект на gitlab.com (создаёт автор один раз, в `main` лежит `README.md`). Скрипт переиспользует его при каждом прогоне; имена веток уникальные (`live/<timestamp>-…`), чтобы повторные прогоны не сталкивались. Реальные проекты автора не затрагиваются. Sandbox создан автором: `AlekseyYakovlev/sanbox` (https://gitlab.com/AlekseyYakovlev/sanbox; написание «sanbox» — как в URL, не опечатка планировщика). Закрывает blocker «нужен проект от автора». Путь проекта не секрет и передаётся `--project AlekseyYakovlev/sanbox`; токены в репозиторий и планы не попадают.
- **D-06:** Два токена через env: `GITLAB_TOKEN` (scope `api`) обязателен; `GITLAB_TOKEN_READONLY` (scope `read_api`) необязателен — без него шаг «запись read-only токеном → понятный 403» пропускается с явной пометкой SKIPPED в транскрипте (не молчаливо). Токены нигде не логируются и не коммитятся; проект и токены не хардкодятся в репозитории.
- **D-07:** Очистка делает сам скрипт в `finally` напрямую через REST (`urllib`, заголовок `PRIVATE-TOKEN`), а не через сервер: закрывает свои MR и удаляет свои ветки `live/<ts>-*` (в том числе при падении на полпути). Это помечается в транскрипте как «cleanup (вне сервера)». Инструмент `delete_branch` в сервер не добавляется (WRT-04 остаётся в v2); при успешном merge ветку в основном сценарии убирает `remove_source_branch`.
- **D-08:** Реальные ошибки проверяются на gitlab.com, помимо критерия (запись read-only токеном, несуществующий проект): невалидный токен → понятный 401 без утечки токена; несуществующая ветка/файл → 404; отказ merge (Draft MR / неполная проверка) — `merge_merge_request` объясняет причину по `detailed_merge_status` без сырой ошибки; дубль — `create_branch` на уже существующую ветку и `create_merge_request` на ветку с открытым MR (409 → «открытый MR уже есть: !N»). Ожидаемые сообщения сверяются с русскими текстами из `internal/tools/errors.go`/`mr_status.go`.
- **D-09:** Основной сценарий закрывает связку между шагами: `iid`/ветки из вывода одного инструмента подаются следующему; перед merge после `create_merge_request` вызывается `get_merge_request` (статус `checking` → повтор через несколько секунд, как подсказывает сервер).

**Бэклог 999.x до живого прогона**
- **D-10:** В Phase 4 закрывается только 999.1: `commit_files` create/update без `content` отклоняется до запроса. Указатель `*string` не вводится (иначе в схеме `["null","string"]`, нарушение FND-08): `content` остаётся `string` с `omitempty`; пустой `content` при `create`/`update` — ошибка до обращения к GitLab. Создание совершенно пустого файла не поддерживается (обход: один перевод строки) — документируется в описании инструмента и README. Тест «нет запроса к GitLab без content» обязателен; golden-снимок `tools/list` и schema guard остаются зелёными. 999.1 в `ROADMAP.md` помечается закрытым.
- **D-11:** 999.3, 999.4 и статус 999.5 в Phase 4 не берутся (остаются в бэклоге). README при этом не должен завышать защиту `create_or_update_file`: сервер сам перечитывает файл перед POST, защиты от чужих правок между чтением и записью нет (сверить формулировку с 999.3).

**Поставка: сборка и README**
- **D-12:** Каноническая сборка: `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`. Отдельного build-скрипта и Makefile нет; команда — в README. `gitlab-mcp.exe` остаётся в корне и в `.gitignore` (в репозиторий и релизы не кладётся); агенту указывается абсолютный путь.
- **D-13:** Критерий «без внешних зависимостей, без Node/npx» подтверждается проверкой в рамках прогона (например `go version -m gitlab-mcp.exe` для `CGO_ENABLED=0` и отсутствие внешних DLL) и фиксируется в `LIVE-RUN.md` вместе с размером файла (ориентир из спайка: 9–12 МБ). Форма проверки (отдельный шаг `live.py` или тест) — на усмотрение планировщика.
- **D-14:** README — на русском, компактный. Разделы: что это; сборка (D-12); токен и scope (`api` для записи — обязательно для `commit_files`/`create_or_update_file`/MR-записей; `read_api` достаточно только для чтения; `write_repository` недостаточен); подключение к агенту — поля `command` (абсолютный путь к `gitlab-mcp.exe`), `args` (`[]`, JSON-массив), `env` (`{"GITLAB_TOKEN": "..."}`, JSON-словарь; опционально `GITLAB_URL`) и готовый JSON-фрагмент, потому что агент хранит серверы как поля в БД через UI, а не файлом `mcpServers`; таблица 20 инструментов (по одной строке); ограничения («Mini», запись без ограничений — безопасность обеспечивается правами токена, пустой файл не создаётся, `delete_branch` нет, `sha`-защиты merge нет, только gitlab.com); как проверить (`scripts/smoke.py`, `scripts/live.py`, `go test ./...`).

**Уже зафиксировано (не переоткрывать)**
- **D-15:** Из Phase 1–3 действуют: Go + `go-sdk` v1.8.0 + `client-go` v2.64.0; только stdio, stdout только JSON-RPC; токен нигде не выводится; запись (POST/PUT) не повторяется автоматически; ошибки как `isError` на русском; плоские схемы без указателей; 20 инструментов без префикса; `go test -race` не используется (нет cgo).

### Claude's Discretion
- Структура `live.py` (шаги, хелперы, формат PASS/FAIL/SKIPPED), точные имена веток/MR/заголовков сценария, паузы и число повторов для статуса `checking`.
- Точные формулировки описания `commit_files` про пустой `content` и разделов README.
- Форма проверки сборки (D-13) и способ вычисления SHA/размера `.exe` в транскрипте.
- Раскладка задач и волн; нужны ли gap-closure планы после первого живого прогона (D-04).

### Deferred Ideas (OUT OF SCOPE)
- `delete_branch` (WRT-04) — v2; в Phase 4 очистку делает скрипт через REST.
- Запись пустого файла (`content=""`) — потребует указателя или ручной правки схемы; вернуться вместе с бэклогом, если понадобится.
- 999.3 (описание `create_or_update_file`/бинарная перезапись), 999.4 (переполнение `page` в `compare_refs`), 999.5 (закрытие пункта после подтверждения автора) — остаются в бэклоге.
- `search_code` (SRCH-01), inline-комментарии MR (MRX-01), `approve` (MRX-02), `sha`-защита merge (MRX-03), tool annotations (UX-01), проект по URL (UX-02), structured output (UX-03) — v2.
- Build-скрипт/Makefile, публикация `.exe` в репозитории или релизе, RU+EN README — не нужны для личного инструмента.
- Подключение к AiAdventAgentV2 и проверка внутри самого агента — выполняется в проекте агента.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QA-03 | Живой прогон на тестовом проекте gitlab.com: чтение, ветка, коммит, MR, комментарий, merge, очистка | F1 (first contact with real gitlab.com), F5-F9 (scenario design, unique per-run paths, poll rules, cleanup REST), F12 (read-only token step), F13 (staleness), Pattern 1-3, Code Examples 1-3, Pitfalls 1-8 |
| QA-04 | Сборка в один `gitlab-mcp.exe` (Go) и README с фрагментом конфигурации для агента и требуемыми scope токена (`api` для записи) | F3 (how the agent really stores servers: UI lines + REST JSON), F4 (verified build, size, PE imports, reproducible hash, PowerShell syntax), Code Examples 4-5, Pitfall 9-10 |
</phase_requirements>

## Summary

The whole server (20 tools, 3 phases of hermetic tests) has never touched real gitlab.com: every prior test uses the `httptest` fake, and `scripts/smoke.py` only reads live. Phase 4 is therefore the first contact with the real API, and D-04 (fix what the live run finds, with a hermetic regression test per fix) is not a formality: the plan needs explicit room for at least one gap-closure loop. The work splits cleanly into four independent deliverables: (a) the 999.1 code fix with tests, (b) `scripts/live.py` with a redacted transcript and REST cleanup, (c) the build/no-external-deps proof and a Russian README, (d) the actual live run, which needs the author's tokens (`GITLAB_TOKEN`, `GITLAB_TOKEN_READONLY` are NOT set in the executor's shell) and is a human-action checkpoint before the executor can run the script.

Key first-hand findings that change how things should be written: the `mcp` 1.30.0 client builds the child environment as a small whitelist plus the explicit `env`, so the parent's `GITLAB_TOKEN` is not inherited and `live.py` must pass every token explicitly (also the basis for the invalid-token and read-only sub-sessions). The agent stores servers as `name/command/args/env/cwd/enabled`, and its UI takes `args` and `env` as newline-separated text (`KEY=VALUE`), while its REST endpoint takes JSON; the README must show both. The `.exe` built with the canonical command is 11,445,760 bytes, imports only `kernel32.dll` (48 symbols, checked with Go's `debug/pe`, no gcc/dumpbin needed), and rebuilds to an identical SHA-256, so a hermetic Go test can assert "no external DLLs" cheaply. The 999.1 fix has one trap: the existing test `TestCommitFilesEmptyContentIsSent` locks in the opposite behavior and must be rewritten, and `create_or_update_file` (required `content`, no emptiness check) must get the same guard to keep the documented "no empty files" rule consistent.

Live-scenario design must avoid three self-inflicted failures: reusing fixed file paths (the second run's `create` fails because the first run merged the file into `main`), overwriting `README.md` with identical content, and testing "write to protected branch" (the author is Owner, and on gitlab.com Maintainers may push to the default branch, so that test would succeed and pollute `main`). Merge on gitlab.com's API is synchronous, but source-branch removal after merge is asynchronous, so "branch gone" must be polled (or be a warning), never a hard immediate assert.

**Primary recommendation:** Build in this order: 999.1 fix -> `live.py` + exe-deps test + README (parallel) -> rebuild exe -> human-action (export both tokens) -> run `live.py` against `AlekseyYakovlev/sanbox` -> gap-closure loop until green -> commit `LIVE-RUN.md`, close 999.1 in ROADMAP, mark QA-03/QA-04.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Live scenario orchestration, transcript, PASS/FAIL | Test tooling (`scripts/live.py`, Python `mcp` 1.30.0 client) | — | Must mirror the agent's client exactly (same SDK, stdio, env passing); not part of the server |
| Tool behavior, error wording, 999.1 guard | MCP server (`internal/tools`) | GitLab REST (final authority) | Guard must fire before any HTTP request; wording lives in `errors.go`/`mr_status.go` |
| Cleanup (close MRs, delete branches) | Test tooling via direct REST (`urllib`) | — | D-07: server has no `delete_branch`; cleanup must work even if the server is broken |
| "No external deps" proof | Go test in `e2e/` (`debug/pe` on a fresh canonical build) | `live.py` records size/SHA/`go version -m` | Hermetic, runs in `go test ./...`; transcript carries the evidence for the verifier |
| Agent connection instructions | README (docs) | Agent UI/REST (external project) | Agent stores servers as DB fields; README maps them 1:1 |
| Token/scope enforcement | GitLab (PAT scope + role) | Server message wording | Server does not restrict writes (locked); it only explains 401/403 |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go toolchain | 1.27.1 local (`go 1.25` in go.mod) | Build `gitlab-mcp.exe`, run tests | Already installed [VERIFIED: `go version` -> go1.27.1 windows/amd64] |
| `github.com/modelcontextprotocol/go-sdk` | v1.8.0 | Server (unchanged) | Locked (D-15) [VERIFIED: `go version -m` on built exe] |
| `gitlab.com/gitlab-org/api/client-go/v2` | v2.64.0 | GitLab client (unchanged) | Locked (D-15) [VERIFIED: `go version -m` on built exe] |
| Python `mcp` | ==1.30.0 | `live.py` client (`stdio_client`, `ClientSession`) | Same version as the agent's `requirements.txt` [VERIFIED: agent `requirements.txt` line 40; `uv run --with mcp==1.30.0` works locally] |
| Python stdlib: `urllib.request`, `urllib.parse`, `hashlib`, `re`, `asyncio`, `argparse`, `pathlib`, `subprocess`, `datetime`, `secrets` | Python 3.13.15 local (script needs >=3.10) | REST cleanup, SHA, transcript | No extra dependency; matches `smoke.py` PEP 723 header style |
| Go stdlib `debug/pe` | bundled | Assert PE imports in the no-external-deps test | Works without gcc/dumpbin/objdump [VERIFIED: local run, output `kernel32.dll:48`] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `uv` | 0.12.5 local | `uv run scripts/live.py ...` resolves `mcp==1.30.0` from the PEP 723 header | Primary way to run the script [VERIFIED: `uv --version`] |
| `internal/testutil` (`NewFakeGitLab`, `Handle`, `JSON`, `Recorded`, `Requests`) + `newTestSession`/`callText` helpers | in repo | Regression tests for 999.1 and any D-04 fixes | Every fix needs a hermetic test |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Go `debug/pe` test | Pure-Python PE import parser in `live.py` | Duplicates logic, ~40 lines, no benefit; the Go test runs in the normal suite |
| Copy helpers from `smoke.py` into `live.py` | `from smoke import ...` | Import works (smoke is `main`-guarded) but D-01 says do not mix the code; copying ~30 lines keeps each script standalone under `uv run` |
| `urllib` cleanup | `requests`/`httpx` | New dependency for 3 calls; D-07 says `urllib` |

**Installation:** no new packages. `go.mod`/`go.sum` do not change. The script header stays:
```python
# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.30.0"]
# ///
```

## Package Legitimacy Audit

No new external packages are introduced in this phase (Go modules unchanged; Python `mcp==1.30.0` is already pinned by the agent and used by `smoke.py`; everything else is stdlib). slopcheck was not run because there is nothing new to check.

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| mcp==1.30.0 | PyPI | existing dependency | n/a | github.com/modelcontextprotocol/python-sdk | not run (already in agent requirements) | Approved (existing) |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

## Architecture Patterns

### System Architecture Diagram

```
                       (human: export GITLAB_TOKEN [api], GITLAB_TOKEN_READONLY [read_api])
                                             |
 uv run scripts/live.py --project AlekseyYakovlev/sanbox
   |  preflight: tokens present, project given, exe exists (+ staleness warning), sha256/size,
   |             `go version -m` excerpt, redactor armed with every token
   v
 [Session A: token=api]  --stdio(env={GITLAB_TOKEN})-->  gitlab-mcp.exe  --HTTPS-->  gitlab.com API
   whoami(scopes has api) -> get_project -> tree/file(README.md must exist) ->
   create_branch(live/<ts>-main) -> commit_files(2 files, unique dir) -> create_or_update_file(update + create) ->
   create_merge_request -> note -> list notes -> get_merge_request (poll while preparing/checking/unchecked) ->
   merge_merge_request(remove_source_branch) -> get_merge_request(state=merged) -> get_file_contents(main) ->
   get_commit(merge sha) -> list_branches (poll: branch gone, else WARN)
   error block: dup create_branch, dup create_merge_request (409 !N), 404 file/ref/project, missing content (999.1),
                Draft MR: create(draft) -> merge refused (draft_status) -> update state_event=close ; merge merged MR -> refused
   |
 [Session B: token=invalid dummy]  whoami -> "401" text, dummy not in output
 [Session C: token=read_api]       whoami (scopes == [read_api]) -> list_branches OK -> create_branch -> "403" + scope hint
   |                                (branch name registered for cleanup BEFORE the call)
   v
 finally: cleanup (outside server, urllib + PRIVATE-TOKEN): close still-open own MRs (PUT state_event=close),
          DELETE own live/<ts>-* branches (204 ok, 404 ok) -> marked "cleanup (вне сервера)"
   v
 LIVE-RUN.md (redacted, assert no token) -> exit 0 / 1 / 2
```

### Recommended Project Structure
```
scripts/
├── smoke.py            # unchanged (hermetic, loopback-only writes)
└── live.py             # NEW: live scenario, transcript, REST cleanup
e2e/
├── build_test.go       # NEW: canonical build + debug/pe import assertion (+ live.py --help/usage exit-code test, optional)
README.md               # NEW (root), Russian
internal/tools/write.go # 999.1 guard (+ schema.go description, golden regenerated)
.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md   # generated evidence, committed
```

### Pattern 1: One MCP session per token, explicit env
**What:** `stdio_client` builds the child env as `{**get_default_environment(), **server.env}`. `get_default_environment()` on Windows only copies `APPDATA, HOMEDRIVE, HOMEPATH, LOCALAPPDATA, PATH, PATHEXT, PROCESSOR_ARCHITECTURE, SYSTEMDRIVE, SYSTEMROOT, TEMP, USERNAME, USERPROFILE` [VERIFIED: mcp 1.30.0 `mcp/client/stdio/__init__.py`, inspected locally]. `GITLAB_TOKEN` from the parent shell is never inherited, `HTTP_PROXY`/`HTTPS_PROXY` are not either.
**When to use:** Every session in `live.py` (main, invalid-token, read-only) is an `@asynccontextmanager` that spawns a fresh exe with its own `env={"GITLAB_TOKEN": tok}` and never sets `GITLAB_URL` (default gitlab.com).
**Example:**
```python
# Source: scripts/smoke.py (existing pattern) + mcp 1.30.0 stdio_client behavior
@asynccontextmanager
async def open_session(exe: str, token: str):
    params = StdioServerParameters(command=exe, args=[], env={"GITLAB_TOKEN": token})
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            init = await session.initialize()
            yield session, init
```
Mirror the agent's call path: the agent wraps `session.call_tool(name, arguments=...)` in `asyncio.wait_for(..., 30)` (`MCP_TOOL_CALL_TIMEOUT=30.0`), caps the result at `MCP_TOOL_RESULT_MAX_CHARS=20000`, and gives the handshake `MCP_CONNECT_TIMEOUT=10.0` [VERIFIED: `AiAdventAgentV2/agent/mcp_tools.py`, `shared/config.py`]. `live.py` should use the same 30 s `wait_for`, assert `len(text) <= 20000`, and assert `initialize` finished under 10 s.

### Pattern 2: Step recorder with three outcomes
**What:** A tiny `Step` recorder collects `(n, title, tool, args, text, status)` where status is `PASS`, `FAIL` or `SKIPPED` (with reason). The transcript is written in `finally`, so a crash mid-scenario still yields a `LIVE-RUN.md` (with FAIL marks); only a fully green run should be committed as evidence. Redaction: `redact(s)` replaces every known token (`GITLAB_TOKEN`, `GITLAB_TOKEN_READONLY`, the dummy invalid token) with `[REDACTED]` before writing, then `assert all(t not in doc for t in tokens)` (D-03).
**Exit codes:** 0 all PASS (SKIPPED only allowed for the read-only step when `GITLAB_TOKEN_READONLY` is unset), 1 a step failed, 2 usage/env error (mirror `smoke.py`). A missing read-only token must make the transcript's criterion 2 read "PARTIAL", so the verifier does not count a SKIPPED run as full evidence.

### Pattern 3: Cleanup outside the server, tracked not discovered
**What:** Register every branch name and MR iid in a list at the moment it is *about to be created* (before the call), not after. In `finally`: for each tracked MR whose state is `opened`, `PUT /projects/:id/merge_requests/:iid` with `state_event=close`; for each tracked branch, `DELETE /projects/:id/repository/branches/:branch` (204 = deleted, 404 = already gone, e.g. removed by `remove_source_branch`). Refuse to delete any name not starting with `live/` and never the default branch. Encode with `urllib.parse.quote(x, safe="")` (project path and branch both contain `/`). Send `PRIVATE-TOKEN` and an explicit `User-Agent`.
**Why tracked:** listing by `live/` prefix could delete another concurrent run's branches; tracking guarantees only this run's objects are touched (D-07 "свои").

### Anti-Patterns to Avoid
- **Fixed file paths across runs:** after a successful run the files live in `main`; the next run's `create` action fails ("file already exists"). Use `live-run/<ts>/a.md` etc.
- **Touching `README.md`:** D-05 relies on it as the sandbox sentinel; updating it with identical content may produce an empty/no-op commit. The `create_or_update_file` update path should update a file the same run created (`live-run/<ts>/notes.md`), and the create path should make a second new file.
- **Reading tokens from the inherited parent env inside the child:** see Pattern 1.
- **Strict immediate assert that the source branch is gone after merge:** removal is asynchronous (Pitfall 4).
- **Writing to `main` in an error test** (Pitfall 3).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PE import inspection | Manual PE header parser or dumpbin/objdump dependency | Go `debug/pe` `ImportedSymbols()` (entries are `symbol:dll`) | Available in stdlib; `ImportedLibraries()` returns an empty list for this exe, so use `ImportedSymbols` [VERIFIED locally] |
| URL-encoding project path / branch in cleanup | String concatenation | `urllib.parse.quote(s, safe="")` | Path `AlekseyYakovlev/sanbox` and `live/<ts>-x` contain `/` |
| SHA-256 / size of the exe | Shelling out to `certutil`/`Get-FileHash` | `hashlib.sha256` + `os.path.getsize` | Portable, no subprocess |
| Tool-argument schema for the new guard | A new schema | Keep the existing hand-written `commitFilesSchema()` and only change descriptions | FND-08 schema guard + golden already pin it |
| Fake GitLab | New mock server | `internal/testutil.NewFakeGitLab` | Records requests; needed to prove "no request sent" |
| Retry/backoff for `checking` | Custom framework | A 3 s sleep loop, max ~60 s, in one helper | Statuses are a small closed set |

**Key insight:** this phase adds almost no production code; its value is evidence. Anything clever in `live.py` is a new place for false failures against a real service, so keep it linear, explicit and heavily printed.

## Common Pitfalls

### Pitfall 1: The first real contact finds bugs the fake never could
**What goes wrong:** All 20 tools were tested only against a route-exact fake; real gitlab.com differs in status codes, fields and timing. **Why:** the fake encodes our assumptions. **How to avoid:** plan for a gap-closure wave (D-04), keep `live.py` output verbose (full response text per step), and require a hermetic regression test per fix. **Warning signs:** a step fails with a plausible-looking text (wrong wording rather than crash), an `isError` where success was expected.

### Pitfall 2: Fixed paths / same content across runs
See Anti-Patterns. Use a per-run directory `live-run/<ts>/` and a per-run branch `live/<ts>-<tag>`; make content contain the timestamp so every commit has a diff. Successful runs leave merged files in the sandbox `main` (acceptable; note it in the transcript).

### Pitfall 3: Testing "protected branch" as the Owner
**What goes wrong:** the author is Owner of a personal-namespace project; on gitlab.com the default branch `main` is protected with "Allowed to push: Maintainers", so a `commit_files` to `main` by the author would **succeed** and pollute `main` [ASSUMED: default gitlab.com protection; Owner is above Maintainer]. **How to avoid:** never call any write tool with `branch=main` in `live.py`. The protected-branch wording is not in D-08 and is covered hermetically.

### Pitfall 4: Asynchronous branch removal and over-strict merge assertions
**What goes wrong:** immediately after `merge_merge_request remove_source_branch=true`, `list_branches` may still list the branch. **Why:** source-branch deletion after merge is a background job [ASSUMED: GitLab behavior, widely observed; not confirmed in docs excerpt]. The merge itself is synchronous: the API endpoint calls `MergeRequests::MergeService#execute` inline and returns the updated MR [CITED: gitlab-foss `lib/api/merge_requests.rb`, read via fetch tool, MEDIUM]. **How to avoid:** after merge call `get_merge_request` (expect `state: merged`, allow a short retry), poll `list_branches search=<branch>` up to ~20 s, and downgrade "still listed" to a WARN that cleanup then resolves (404 on DELETE = fine). The server text for a non-merged response (`state=... слияние ещё не завершено`) is already handled in `mergeSuccessText`.

### Pitfall 5: `detailed_merge_status` right after MR creation
**What goes wrong:** `create_merge_request` returns `checking`/`preparing`/`unchecked`; merging immediately is refused by the server's own pre-check (correct behavior, wrong test order). **How to avoid (D-09):** `get_merge_request` in a loop (sleep 3 s, up to ~60 s) while the status is in `{preparing, checking, unchecked}`; parse `detailed_merge_status: (\w+)` from the text; stop on `mergeable`; fail with the full text on any other status. Auto DevOps is not enabled by default on gitlab.com new projects [CITED: gitlab.com/gitlab-org/gitlab/-/issues/285128 via search, MEDIUM], and the sandbox has no CI file, so `ci_*` statuses are not expected; if one appears it is a sandbox-configuration finding, not a server bug.

### Pitfall 6: Read-only token step must not be able to write
**What goes wrong:** if `GITLAB_TOKEN_READONLY` was issued with `api` by mistake, `create_branch` succeeds and the "403" assertion fails after polluting the project. **How to avoid:** call `whoami` on the read-only session first and parse `token: scopes [..]` (regex `scopes \[([^\]]*)\]`); require the list to equal `["read_api"]` (note `"api" in "read_api"` is a substring trap: compare list elements). If the scopes line is `scopes unavailable` proceed but register the branch for cleanup before the call. Expected wording for the write (no op-specific rule for 403 on `create_branch`, so the generic write 403 text applies): starts with `403: запись отклонена:` and mentions ``scope `api` `` [VERIFIED: `writeText` in `errors.go`]. The real gitlab.com body for a scope failure is `403` with `error: insufficient_scope` [ASSUMED from GitLab forum/issue reports; confirm in the run]. If the live text turns out unclear, the cheap D-04 fix is a `writeRules` entry keyed on the lowercase substring `insufficient_scope` (client-go renders the JSON map as `{error: insufficient_scope}, {error_description: ...}`), with a hermetic test; do not add it pre-emptively.

### Pitfall 7: 401 vs 404 for private/nonexistent projects
Unauthenticated `GET /projects/AlekseyYakovlev%2Fsanbox` returns `404` (private project, consistent with the sandbox being private) and an invalid token returns `401` with body `{"message":"401 Unauthorized"}` [VERIFIED: curl against gitlab.com, 2026-09-25]. So a nonexistent project and an invisible private project are indistinguishable with a valid token; the server text already says "GitLab также отвечает 404, если токен не видит приватный проект". For the invalid-token step use a plausible dummy such as `glpat-` plus 20 random letters, generated per run, never a real token; assert the dummy is absent from every output.

### Pitfall 8: `urllib` details that fail silently or oddly
Set an explicit `User-Agent` (default `Python-urllib` is sometimes rejected by Cloudflare-fronted hosts [ASSUMED]); set `timeout=30`; `HTTPError` for 204 responses is not raised (204 is success), but 404 and 4xx are; do not print the request headers; catch `HTTPError` per object so one failed delete does not abort the rest of the cleanup; report leftovers in the transcript (`cleanup INCOMPLETE: branch X remains`) and exit non-zero.

### Pitfall 9: PowerShell vs bash build syntax
D-12's `CGO_ENABLED=0 go build ...` is bash syntax; in PowerShell it is a parse error. The README must show the PowerShell form (the author is on Windows 11/PowerShell): `$env:CGO_ENABLED = "0"` then `go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp` [VERIFIED: ran exactly this through `powershell -NoProfile`; produced 11,445,760 bytes, `CGO_ENABLED=0`, identical SHA-256 to the bash build]. Go on this machine already reports `CGO_ENABLED=0` by default because no C compiler is installed, but keep it explicit.

### Pitfall 10: README path/quoting mistakes in the agent config
In JSON, Windows backslashes must be doubled (`"C:\\Projects\\GitLabMcpMini\\gitlab-mcp.exe"`) or use forward slashes; in the agent UI text field enter the raw path (single backslashes). Server name is slugged to at most 20 characters and exposed as `mcp__<slug>__<tool>` with a 64-char total limit; a server named `gitlab` yields e.g. `mcp__gitlab__list_merge_request_notes` (37 chars) [VERIFIED: `agent/mcp_tools.py`].

### Pitfall 11: Stale executable
The repo-root `gitlab-mcp.exe` predates the 999.1 fix once it lands. `live.py` should compare the exe mtime with the newest `*.go`/`go.mod` mtime and print a WARN line in the transcript header when the exe is older (warn, not fail), and the plan must rebuild the exe with the canonical command before the live run.

## Code Examples

### 1. 999.1 guard (string content, no pointer; hand-written schema unchanged)
```go
// Source: internal/tools/write.go (existing toFileAction), change for D-10
case actCreate, actUpdate:
    if a.Content == "" {
        return fileAction{}, fmt.Errorf(
            "actions[%d]: для %s нужен непустой content (пустой файл создать нельзя: передайте один перевод строки)", i, action)
    }
    fa.sendContent = true
```
And in `createOrUpdateFile`, before `validateCommitInput` and before any GET, reject `in.Content == ""` with the same wording. Notes for the planner:
- `TestCommitFilesEmptyContentIsSent` (write_test.go:140) asserts the OLD behavior and must be rewritten as "empty content is rejected, no request" (its `fake` then sees zero requests).
- Add rows to `TestCommitFilesGuardsSendNoRequest` for `create` and `update` with the `content` key omitted and with `content: ""` (both arrive as `""`, indistinguishable by design); add an equivalent no-request case for `create_or_update_file` with `content: ""`.
- `move` with empty content stays legal (`sendContent = a.Content != ""`); `delete` with content stays an error.
- Update the descriptions: `commitFilesDescription` (currently 553 chars, cap 900) and the `content` property text in `schema.go` ("non-empty UTF-8 text; required for create and update ... empty files are not supported, use one newline"); `createOrUpdateFileDescription` (562 chars) likewise. Regenerate the golden with `go test ./internal/tools -run TestToolsListGolden -update` and review the diff; the schema guard (no `$ref`/`anyOf`/`null`) must stay green.
- Informational, not re-opening D-10: `commit_files` uses a hand-written `InputSchema` (schema.go), so the schema-inference concern behind "no pointer" applies to inferred tools only; the Phase 2 review (CR-01) had proposed a pointer for exactly this reason. The decision stands: string + reject, empty files unsupported.

### 2. Polling helper for `checking`
```python
PENDING = {"preparing", "checking", "unchecked"}
STATUS_RE = re.compile(r"detailed_merge_status: (\w+)")

async def wait_mergeable(call, project: str, iid: int, tries: int = 20, pause: float = 3.0) -> str:
    text = ""
    for _ in range(tries):
        text = await call("get_merge_request", {"project": project, "iid": iid})
        m = STATUS_RE.search(text)
        status = m.group(1) if m else "?"
        if status == "mergeable":
            return text
        if status not in PENDING:
            raise LiveFailure(f"MR !{iid} is {status}, not mergeable:\n{text}")
        await asyncio.sleep(pause)
    raise LiveFailure(f"MR !{iid} still pending after {tries * pause:.0f}s:\n{text}")
```
Server output formats to parse (all verified in code): `create_merge_request` -> `MR !N создан: src→dst`; `create_branch` -> `ветка X создана от ...`; `commit_files` -> first line `коммит <short8> в <branch> (+a/−r): K файла`, then `create <path> (+a/−r)` lines; `create_or_update_file` -> `created|updated <path>: коммит <short> в <branch>`; note -> `комментарий #ID добавлен к MR !N`; merge -> `MR !N влит (state=merged)` + `merge commit: <40-hex>` (or `squash commit:` / `head sha (fast-forward):` depending on the project's merge method); `get_file_contents` -> header `path @ ref — строки a-b из N, M байт` then the text.

### 3. REST cleanup (outside the server)
```python
def rest(method: str, path: str, token: str, data: dict | None = None) -> int:
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(
        f"https://gitlab.com/api/v4{path}", data=body, method=method,
        headers={"PRIVATE-TOKEN": token, "User-Agent": "gitlab-mcp-live/1.0",
                 **({"Content-Type": "application/json"} if body else {})})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code

proj = urllib.parse.quote(project, safe="")
rest("PUT",    f"/projects/{proj}/merge_requests/{iid}", token, {"state_event": "close"})
rest("DELETE", f"/projects/{proj}/repository/branches/{urllib.parse.quote(branch, safe='')}", token)  # 204 / 404 ok
```
`DELETE` returns 204; default and protected branches cannot be deleted [CITED: docs.gitlab.com/api/branches].

### 4. No-external-deps test (hermetic, in `e2e/`)
```go
// Source: verified locally: ImportedSymbols yields "symbol:dll"; ImportedLibraries() is empty.
func TestBinaryHasNoExternalDependencies(t *testing.T) {
    if runtime.GOOS != "windows" { t.Skip("PE check is Windows-only") }
    out := filepath.Join(t.TempDir(), "gitlab-mcp.exe")
    cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", out, "./cmd/gitlab-mcp")
    cmd.Dir = ".."
    cmd.Env = append(envWithout("CGO_ENABLED"), "CGO_ENABLED=0")
    if b, err := cmd.CombinedOutput(); err != nil { t.Fatalf("build: %v\n%s", err, b) }
    f, err := pe.Open(out); if err != nil { t.Fatal(err) }
    defer f.Close()
    syms, err := f.ImportedSymbols(); if err != nil { t.Fatal(err) }
    for _, s := range syms {
        dll := strings.ToLower(s[strings.LastIndex(s, ":")+1:])
        if dll != "kernel32.dll" { t.Errorf("unexpected import %q", s) }
    }
}
```
Measured on the current tree: 11,445,760 bytes with `-trimpath -ldflags "-s -w"`; only `kernel32.dll` (48 symbols); `go version -m` lists module `gitlab-mcp`, go-sdk v1.8.0, client-go v2.64.0, and `build CGO_ENABLED=0`, `-trimpath=true` (no `-ldflags` line is shown, so do not assert on it) [VERIFIED: local]. `envWithout` already exists in `e2e/main_test.go`.

### 5. README connection snippet (must show both forms)
The agent's UI form has fields Название, Команда, Аргументы («по одному в строке»), Переменные окружения («KEY=VALUE, по одной в строке»), Рабочая директория (необязательно), Включён; the same data goes to `POST /api/v1/mcp/servers` as JSON [VERIFIED: `AiAdventAgentV2/ui/static/index.html`, `ui/static/app.js parseMcpArgs/parseMcpEnv`, `agent/schemas.py McpServerCreate`]. Env values are masked (`•••`) in the UI after saving.
```
UI:   Название: gitlab
      Команда:  C:\Projects\GitLabMcpMini\gitlab-mcp.exe
      Аргументы: (пусто)
      Переменные окружения:  GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```
```json
{
  "name": "gitlab",
  "command": "C:\\Projects\\GitLabMcpMini\\gitlab-mcp.exe",
  "args": [],
  "env": { "GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx" },
  "cwd": null,
  "enabled": true
}
```
Optional `GITLAB_URL` (default `https://gitlab.com`; only gitlab.com is supported). Use a placeholder token, never a real one. This refines D-14's wording: `args` is a JSON array only in the REST body; the UI takes lines.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `xanzy/go-gitlab` | `gitlab.com/gitlab-org/api/client-go/v2` | already adopted (Phase 1) | none for this phase |
| Inspect exe with `dumpbin`/`objdump` (needs VS/MinGW) | Go `debug/pe` in a test | n/a | works on this box (no gcc, no dumpbin) |
| gitlab.com Auto DevOps default-on (11.3) | not enabled by default on gitlab.com new projects | per gitlab issue 285128 | sandbox MRs are not blocked by pipelines |

**Deprecated/outdated:** none relevant. Note GitLab's `merge_status` is deprecated in favor of `detailed_merge_status` (already what the server uses).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | gitlab.com answers a write with a `read_api` PAT as `403` with `error: insufficient_scope` | Pitfall 6 | Wording check may need adjusting; existing generic 403 text is still shown, so low impact |
| A2 | Source-branch removal after merge is asynchronous | Pitfall 4 | If synchronous, polling just exits on the first iteration; no harm |
| A3 | Author (Owner) may push to protected `main` on gitlab.com default settings | Pitfall 3 | If wrong, a protected-branch test would be safe; we skip it anyway |
| A4 | Python `Python-urllib` User-Agent may be rejected by Cloudflare | Pitfall 8 | Setting a custom UA is harmless either way |
| A5 | The sandbox project has no `.gitlab-ci.yml` and default merge settings (merge commit, no required pipeline/approvals) | Pitfall 5 | An unexpected `detailed_merge_status` would appear; `live.py` fails with the full text and the author adjusts sandbox settings |
| A6 | The merge PUT response is `state=merged` synchronously (source: fetch-tool summary of `lib/api/merge_requests.rb`) | Pitfall 4 | Handled by the post-merge `get_merge_request` retry |
| A7 | `GET /personal_access_tokens/self` works with a `read_api` token (whoami scopes line) | Pitfall 6 | The scopes line would read `scopes unavailable`; the read-only step then relies on cleanup tracking only |

## Open Questions (RESOLVED)

1. **Does the sandbox `main` really contain `README.md`, and what are its merge settings?**
   - What we know: the project exists and is private (unauthenticated GET returns 404); the author says `README.md` is in `main`.
   - What's unclear: cannot be verified without a token; merge method (merge commit vs fast-forward) changes the merge text (`merge commit:` vs `head sha (fast-forward):`).
   - Recommendation: `live.py` step 1 reads `README.md` from `main` and fails with a clear Russian message; accept any of the three merge-commit text forms.
   - RESOLVED: handled by the live.py step 4 preflight (README.md read from the default branch with a clear message; all three merge-text forms accepted); sandbox configuration problems are an author action routed in 04-04 Task 3.

2. **Should `create_or_update_file` reject empty `content` too?**
   - What we know: D-10 speaks about `commit_files`; the single-file tool has required `content` and today happily writes `""`.
   - Recommendation: reject in both (Code Example 1) so the documented "no empty files" limitation is true for every write path; costs one guard and one test.
   - RESOLVED: adopted in Plan 01 (empty `content` rejected in both `commit_files` and `create_or_update_file`).

3. **Is a second read-only-token proof of `create_or_update_file`/`commit_files` needed?**
   - Recommendation: no; `create_branch` with the read-only token satisfies criterion 2. Optionally add `create_or_update_file` (pre-read succeeds, POST 403) to show the read-then-write path, with a tracked branch name.
   - RESOLVED: decided no; `create_branch` with the read-only token is the single read-only write proof (Plan 02 Task 3, session C).

4. **Hermetic test for `live.py` itself?**
   - Recommendation: only cheap checks in `go test ./...` (e.g. `uv run scripts/live.py` without args exits 2 without network; `--help` works). Do not build a full fake for the scenario; the first real run is the test. `e2e` must never invoke `live.py` with a project (it would spawn network writes); existing e2e only runs `smoke.py` with explicit `--base-url` [VERIFIED: `e2e/python_smoke_test.go`].
   - RESOLVED: adopted in Plan 02 (e2e/live_script_test.go: usage/exit code 2 without token and `--help` only; never runs live.py with a token).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | build, tests | yes | 1.27.1 | — |
| uv | `uv run scripts/live.py`, existing `TestPythonSmoke` | yes | 0.12.5 | Python venv with `pip install mcp==1.30.0` |
| Python | script runtime | yes | 3.13.15 | — |
| Network to gitlab.com | live run | yes (curl 404/401 responses received) | — | — |
| `GITLAB_TOKEN` (scope `api`) | live run | **no** (unset in this shell) | — | human-action: author exports it |
| `GITLAB_TOKEN_READONLY` (scope `read_api`) | criterion 2 | **no** (unset) | — | step becomes SKIPPED (criterion 2 PARTIAL); prefer to obtain it |
| Sandbox project `AlekseyYakovlev/sanbox` | live run | exists, private (unauth GET -> 404) | — | — |
| gcc / dumpbin / objdump | not needed | no | — | Go `debug/pe` |
| Node / npx | not needed | present on PATH but irrelevant | — | — |

**Missing dependencies with no fallback:** `GITLAB_TOKEN` in the executor's environment. Plan a `checkpoint:human-action` ("export both tokens in the shell that runs the executor, then confirm") before the live-run task. Tokens were pasted into a chat during discussion; they must not be copied into any file, and should be revoked and reissued after the phase (CONTEXT `<specifics>`).
**Missing dependencies with fallback:** `GITLAB_TOKEN_READONLY` (see above).

## Security Domain

`security_enforcement` is not set to false, so this section applies.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no (PAT is issued by GitLab) | — |
| V3 Session Management | no | — |
| V4 Access Control | yes | Enforced by PAT scope + project role at GitLab; `live.py` proves 401/403 wording; cleanup deletes only tracked `live/` branches |
| V5 Input Validation | yes | 999.1 guard before any request; existing `NormalizeProject`/`NormalizeRepoPath` |
| V6 Cryptography | no | `hashlib.sha256` only for the exe fingerprint, not a security control |
| V7 Error handling / Logging | yes | Redact tokens in transcript and stdout; assert absence; server already redacts stderr |
| V8 Data protection | yes | Tokens only via env; nothing in repo, README (placeholder `glpat-xxxx`), plans, or `LIVE-RUN.md` |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Token leaked into transcript/stdout/README | Information disclosure | `redact()` on every recorded string + final `assert token not in doc`; placeholder tokens in README |
| Live script writes to the wrong project | Tampering | Mandatory `--project`, banner "WRITES to <project> on gitlab.com" before the first write, no default project |
| Cleanup deletes someone else's branch or `main` | Tampering / DoS | Delete only branches registered by this run, name must start with `live/`, never the default branch |
| "Read-only" token actually has write scope | Elevation of privilege | Verify `scopes == [read_api]` via `whoami` before the write attempt; register the branch for cleanup first |
| Invalid-token echo | Information disclosure | Dummy token per run, assert absent from every output |
| Unsigned exe flagged by AV | Availability | Not mitigated (personal tool launched by the agent as a subprocess); mention nothing extra |

## Project Constraints (from CLAUDE.md)

- Stdio only; stdout is JSON-RPC only (`fmt.Println`/`log` to stdout forbidden); logs to stderr.
- Runtime without Node/npx: native `.exe` or Python script; PAT via env; gitlab.com only; "Mini" scope (no new tools).
- Versions locked: go-sdk v1.8.0, client-go v2.64.0, Python `mcp` ==1.30.0 for the client/test tooling; never `pip install mcp` unpinned (2.x).
- No pointer fields for optional tool args (`["null",...]` schema); flat schemas.
- `go test -race` is not used (no cgo/gcc); use `go test ./...` and `go vet`.
- PAT scope `api` is required for writes; `write_repository` is insufficient; document in README.
- Build: `go build -trimpath -ldflags "-s -w"`; `CGO_ENABLED=0`.
- GSD workflow enforcement: file changes go through GSD commands (plan/execute), not ad-hoc edits.
- Documentation and messages for the author in Russian (README Russian; script messages in the smoke.py style).

## Sources

### Primary (HIGH confidence)
- Local code, read first-hand: `scripts/smoke.py`, `internal/tools/{write,schema,errors,mr_status,mr_merge,mr,branch,file,whoami,register}.go`, `internal/tools/write_test.go`, `e2e/{main_test,python_smoke_test,stdio_test,purity_test}.go`, `internal/testutil`, `cmd/gitlab-mcp/main.go`, `internal/config/config.go`, `internal/glclient/{errors,retry}.go`
- Local experiments: `go build` (bash and PowerShell), `go version -m`, `debug/pe` import listing, SHA-256 comparison, `go test ./...` (all green, 4 s), `curl` to gitlab.com (404 for the private sandbox, 401 body for a bad token)
- mcp 1.30.0 source inspected via `uv run --with mcp==1.30.0` (`stdio_client`, `get_default_environment`, `call_tool` signature)
- `C:\Projects\AiAdventAgentV2`: `agent/mcp_config.py`, `agent/mcp_client.py`, `agent/mcp_tools.py`, `agent/schemas.py`, `shared/models.py`, `shared/config.py`, `ui/static/index.html`, `ui/static/app.js`, `requirements.txt`
- https://docs.gitlab.com/api/branches/ (delete branch 204; default/protected cannot be deleted)
- https://docs.gitlab.com/security/tokens/access_token_scopes/ (`api` vs `read_api` vs `write_repository`)
- client-go v2.64.0 source (`CheckResponse`, `parseError`, `PathEscape` escapes `.` as `%2E`)

### Secondary (MEDIUM confidence)
- https://gitlab.com/gitlab-org/gitlab-foss/-/raw/master/lib/api/merge_requests.rb (merge is executed synchronously; summarized by a fetch tool)
- https://gitlab.com/gitlab-org/gitlab/-/issues/285128 (Auto DevOps not default on gitlab.com; via search summary)
- https://docs.gitlab.com/api/merge_requests/ (async mergeability, `detailed_merge_status`; the fetched excerpt was partial)
- Phase 2/3 RESEARCH.md (409 duplicate `!N` message, 405/422 merge codes)

### Tertiary (LOW confidence)
- GitLab forum/issue search results for `insufficient_scope` (403 with too-narrow scope); exact body text not confirmed, to be observed in the live run

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH (nothing new; versions verified from the built exe)
- Architecture: HIGH for tooling/agent/build facts; MEDIUM for gitlab.com runtime behavior (branch removal timing, exact 403 body), by design confirmed only by the live run
- Pitfalls: MEDIUM-HIGH

**Research date:** 2026-09-25
**Valid until:** 2026-10-25 (stable; gitlab.com rate-limit tightening for unauthenticated traffic on 2026-10-19 does not affect PAT-authenticated calls)
