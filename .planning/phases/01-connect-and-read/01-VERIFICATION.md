---
phase: 01-connect-and-read
verified: 2026-09-24T00:00:00Z
status: passed
score: 5/5 roadmap success criteria verified (plus 31/31 plan-level truths)
has_blocking_gaps: false
overrides_applied: 0
re_verification: false
gaps: []
deferred:
  - truth: "Live run against real gitlab.com (read path)"
    addressed_in: "Phase 4"
    evidence: "REQUIREMENTS QA-03 (live run on a test gitlab.com project) is mapped to Phase 4; task states live verification is out of scope for Phase 1"
warnings:
  - id: WR-01
    text: "PRIVATE-TOKEN header is forwarded on cross-host / https->http redirects (glclient/client.go newHTTPClient has no CheckRedirect). Hardening gap, not a Phase 1 truth (token is not in output/logs)."
  - id: WR-02
    text: "get_file_contents: a single line longer than 15000 chars is hard-cut; footer says use start_line/end_line, which cannot reach the rest of that line. Truncation IS marked explicitly, so SC3 holds, but recovery advice is misleading."
  - id: WR-03
    text: "capBody rejects a body of exactly 8 MiB (off by one at the cap boundary). Practically irrelevant."
  - id: WR-04
    text: "Timeout text hard-codes '25 с' though the 15 s http.Client timeout can fire first. Message inaccuracy only."
  - id: MVP-FORMAT
    text: "ROADMAP marks Phase 1 'Mode: mvp' but the phase goal is not in 'As a ..., I want to ..., so that ...' form (user-story.validate -> false). Verified with the standard goal-backward method against the 5 roadmap SCs as instructed; consider /gsd mvp-phase or removing the mode flag."
---

# Phase 1: Подключение и чтение проекта Verification Report

**Phase Goal:** Агент (Python `mcp` 1.30.x) запускает `gitlab-mcp.exe` по stdio, проходит handshake, проверяет токен и читает проекты, дерево файлов и содержимое файлов на gitlab.com
**Verified:** 2026-09-24
**Status:** passed
**Re-verification:** No, initial verification

Independent build and test evidence (run by the verifier, not taken from SUMMARY):
- `go build ./...` OK; `go vet ./...` OK
- `go test ./... -count=1`: e2e, config, glclient, logging, tools all `ok`; 0 skipped tests (uv is present, so `TestPythonSmoke` really ran against Python `mcp==1.30.0` and passed)
- Built binary without `GITLAB_TOKEN`: exit code 1, 0 bytes on stdout, stderr text names `GITLAB_TOKEN`

## Goal Achievement: Roadmap Success Criteria (the contract)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Python mcp 1.30.0 smoke: `initialize` + `tools/list` with no network at startup; stdout is only JSON-RPC across the whole session incl. error calls; process exits on stdin close | VERIFIED | `main.go` keeps real stdout for `mcp.IOTransport` and sets `os.Stdout = os.Stderr`; `glclient.New` does no I/O (client-go lazy auth). `e2e/stdio_test.go::TestStdioHandshakeAndWhoami` asserts `fake.Requests()==0` after initialize+tools/list and exit code 0 after Close. `e2e/purity_test.go::TestStdoutPurity` (raw pipes, LOG_LEVEL=debug, 401 call, unknown tool, bad args): every stdout line JSON-RPC 2.0, no `\r`, exit after stdin close. `scripts/smoke.py` pinned `mcp==1.30.0`; `TestPythonSmoke` PASS. |
| 2 | Without `GITLAB_TOKEN` server exits at once with clear stderr message; with token `whoami` returns current user; token never in output/logs | VERIFIED | `config.Load` errors on empty token; spot-check: exit=1, stdout empty, stderr message names `GITLAB_TOKEN`. `whoami.go` calls `Users.CurrentUser` and returns `@user (name), id, state, url` + best-effort token scopes line. Redaction: `logging.NewRedactor` (exact token + `glpat-` pattern) applied to log msgs/attrs and in `safe()` as last step on both success and error text; `Config` has no String method. Tests: `TestStdioMissingToken`, `TestStdoutPurity` and `TestStdioHandshakeAndWhoami` assert token absent from stdout/stderr/tool text; `safe_test.go::TestSafeRedactsErrorText`, `logging_test.go`. |
| 3 | `list_projects`, `get_project`, `list_repository_tree`, `get_file_contents` accept numeric ID or `group/subgroup/project`; lists show page/per_page and next-page flag; big/binary file and over-long output give explicit truncation marks | VERIFIED | All four registered in `register.go` via `safe()`; `NormalizeProject`/`NormalizeRepoPath` used in each handler. `PageFooter` reads `X-Next-Page`/Link only (`[page N, per_page M — есть следующая страница…]` / `последняя страница`); `composeList` adds `TruncatedFooter` at 15000 runes. `get_file_contents`: base64 decode, `selectLines` (1-based inclusive), `Budget` + footer `[файл обрезан на 15000 символах: показаны строки a-b из N…]`, `isBinary` (NUL / invalid UTF-8) returns `файл бинарный, N байт, blob_id X` without content; 8 MiB body cap gives `слишком большой` message. Default branch resolved from `GET /projects/:id` (never HEAD), `errEmptyRepo` for empty repos. Covered by projects_test/tree_test/file_test/ref_test (all pass). |
| 4 | 401/403/404/429/5xx and network failures become `isError` with clear message, server does not crash; call within ~25 s; tool names ≤30, descriptions ≤900, flat schemas | VERIFIED | `glclient.Classify` (real client-go `ErrorResponse` mapped to Kind; 5xx bodies never echoed) + `tools.toToolText` (Russian per-status wording, 429 uses Retry-After) + `safe()` (panic recover, `context.WithTimeout(25s)`, `IsError:true`, nil Go error). Retry: GET/HEAD only, max 2, 429 Retry-After ≤5 s honoured, writes never retried (`checkRetry`; `client_test.go::TestRetryPolicyAttempts`, `TestRefusedConnectionWriteNotRetried`). Timeouts at every stage (dial 5 s, TLS 5 s, header 15 s, client 15 s, ctx 25 s). `errors_test.go` covers 401/403/404/429/502/400/422/timeout/network/decode; `safe_test.go::TestSafeRecoversPanic`; `schema_test.go::TestToolSchemas` walks `ListTools` enforcing name ≤30, desc ≤900, type object, no `$ref/$defs/anyOf/oneOf/"null"`. |
| 5 | Autotests (httptest fake GitLab, in-memory MCP session, e2e on real binary) pass and check wire encoding of project and file paths; harness ready for extension | VERIFIED | `internal/testutil/fakegitlab.go` (records Method+RequestURI, exact-raw-path router) and `testutil/session.go` (`Connect` over `NewInMemoryTransports`); `tools/helpers_test.go::newTestSession`. Wire tests: `glclient/wire_test.go` (project dot/Cyrillic/space, file path, tree, token only in PRIVATE-TOKEN header) and `e2e/stdio_test.go::TestStdioWirePaths` on the real binary (7 subtests: dot, space+Cyrillic, `#`/space/ref with slash, `+`, `%`, Cyrillic, awkward tree path). All PASS. |

**Score:** 5/5 roadmap criteria verified.

### Plan-level must-haves (spot cross-check)

All truths in plans 01-01..01-06 frontmatter were mapped to the tests above; none contradicts the code. Notable: whoami via `Users.CurrentUser` (01-01 key link, present), `WithCustomRetryMax(2)`-equivalent via `maxRetries` const + `WithCustomLimiter(noLimiter{})` (01-02, present in `clientOptions`), `resolveRef(` used by tree.go and file.go (01-05/06 key links), `base64.StdEncoding` in file.go, `list_projects`/`get_project`/`list_repository_tree`/`get_file_contents` registered.

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `go.mod`, `cmd/gitlab-mcp/main.go` | VERIFIED | composition root, stdout guard, `mcp.IOTransport` |
| `internal/config/config.go` | VERIFIED | fail-fast token, https-only URL (http for localhost) |
| `internal/glclient/{client,retry,transport,errors,normalize}.go` | VERIFIED | substantive, wired from main/tools, tested |
| `internal/logging/logging.go` | VERIFIED | redacting slog handler to stderr |
| `internal/tools/{safe,errors,format,projects,ref,tree,file,whoami,register}.go` | VERIFIED | substantive, registered, data flows from real client-go calls (no static returns) |
| `internal/testutil/{fakegitlab,session}.go` | VERIFIED | reused by tool and e2e tests |
| `e2e/*_test.go`, `scripts/smoke.py` | VERIFIED | run and pass, smoke pinned `mcp==1.30.0` |

### Key Link Verification

| From | To | Status |
|------|----|--------|
| main.go | `os.Stdout = os.Stderr` guard | WIRED |
| whoami.go | `Users.CurrentUser` | WIRED |
| safe.go | `toToolText` -> `glclient.Classify` | WIRED |
| projects/tree/file handlers | `NormalizeProject` / `NormalizeRepoPath` / `PageFooter` / `resolveRef` | WIRED |
| register.go | all five tools via `mcp.AddTool` + `safe()` | WIRED |
| e2e python_smoke_test.go | scripts/smoke.py via `uv run` | WIRED |

### Requirements Coverage

Requirement IDs declared across the six PLAN frontmatters: 01: FND-01, FND-02, FND-03, UTIL-01, QA-01, QA-02; 02: FND-04, FND-05, FND-07, QA-01; 03: FND-01, FND-02, FND-03, FND-08, QA-01; 04: READ-01, READ-02, FND-04, FND-06, FND-08; 05: READ-03, FND-04, FND-06; 06: READ-04, FND-04, FND-06, QA-01, QA-02. Union = all 15 ROADMAP Phase 1 IDs; no orphaned requirements (REQUIREMENTS.md traceability maps exactly these 15 to Phase 1).

| Requirement | Source Plan | Status | Evidence |
|-------------|------------|--------|----------|
| FND-01 stdio handshake, no network at start | 01-01, 01-03 | SATISFIED | SC1 evidence |
| FND-02 stdout JSON-RPC only, exit on stdin close | 01-01, 01-03 | SATISFIED | TestStdoutPurity |
| FND-03 token from env, fail-fast, no leak | 01-01, 01-03 | SATISFIED | config.go, redactor, tests |
| FND-04 project as ID or path | 01-02, 01-04..06 | SATISFIED | Normalize* + wire tests |
| FND-05 error mapping to isError | 01-02 | SATISFIED | Classify/toToolText/safe; 405/409/422 flow through the 4xx `KindBadRequest` branch (422 tested) |
| FND-06 page/per_page, next-page flag, ~15k cap | 01-04..06 | SATISFIED | format.go, list tool tests |
| FND-07 ~25 s, no write retries | 01-02 | SATISFIED | CallTimeout, checkRetry tests |
| FND-08 name/desc/flat schema limits | 01-03, 01-04 | SATISFIED | schema_test.go |
| UTIL-01 whoami | 01-01 | SATISFIED | whoami.go, e2e |
| READ-01 list_projects (membership default) | 01-04 | SATISFIED | projects.go, tests |
| READ-02 get_project incl. default branch | 01-04 | SATISFIED | projects.go, tests |
| READ-03 list_repository_tree | 01-05 | SATISFIED | tree.go, tests |
| READ-04 get_file_contents | 01-06 | SATISFIED | file.go, tests |
| QA-01 automated tests incl. stdout purity | 01-01..03, 06 | SATISFIED | all suites pass |
| QA-02 Python 1.30.0 smoke | 01-01, 01-06 | SATISFIED | smoke.py + TestPythonSmoke PASS |

### Anti-Patterns Found

None blocking. Grep for TBD/FIXME/XXX/TODO across `*.go` and `*.py`: no matches. No stub returns; handlers issue real client-go requests and format real responses.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Missing token | run built exe without `GITLAB_TOKEN` | exit 1, stdout 0 bytes, stderr names GITLAB_TOKEN | PASS |
| Whole build/vet/test | `go build`, `go vet`, `go test ./... -count=1` | all ok, 0 skips | PASS |

### Code Review Findings Considered (01-REVIEW.md, 4 warnings, 6 info)

Independently re-checked in code; none invalidates a success criterion. WR-01 (PAT forwarded on cross-origin redirects: `newHTTPClient` has no `CheckRedirect`) is a real but bounded hardening gap for a single-user gitlab.com-only tool; WR-02, WR-03, WR-04 confirmed as described (see frontmatter warnings). Recommend fixing WR-01 and WR-02 in a later phase or a quick task; they are non-goal-blocking.

### Human Verification Required

None required for Phase 1 under the stated acceptance bar (hermetic tests). Real gitlab.com behavior is deferred to Phase 4 (QA-03).

### Gaps Summary

No gaps. All five roadmap success criteria are verified against actual code and independently executed tests. Only non-blocking warnings remain (redirect token forwarding, very-long-line truncation advice, cap off-by-one, timeout wording) plus a process note that the ROADMAP `mode: mvp` flag does not match a user-story-formatted goal.

---

_Verified: 2026-09-24_
_Verifier: Claude (gsd-verifier)_
