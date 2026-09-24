---
phase: 01-connect-and-read
plan: 01
subsystem: infra
tags: [go, mcp, go-sdk, client-go, stdio, gitlab, testing, python-smoke]

requires: []
provides:
  - "Buildable Go module gitlab-mcp (go 1.25.0) with pinned go-sdk v1.8.0 and client-go/v2 v2.64.0"
  - "gitlab-mcp.exe with one tool, whoami, served over stdio with a stdout guard"
  - "config.Load fail-fast (GITLAB_TOKEN, GITLAB_URL, LOG_LEVEL) and redacting stderr logger"
  - "tools.safe generic wrapper (recover, 25 s deadline, isError mapping, last-step redaction)"
  - "testutil.FakeGitLab (raw RequestURI recorder), real-binary e2e tests, PEP 723 smoke client on mcp==1.30.0"
affects: [01-02, 01-03, 01-04, 01-05, 01-06]

tech-stack:
  added: [go-sdk v1.8.0, client-go/v2 v2.64.0, python mcp 1.30.0 (test tooling only)]
  patterns:
    - "composition root in cmd/gitlab-mcp/main.go; os.Stdout = os.Stderr after capturing the real stdout for mcp.IOTransport"
    - "every tool handler is a func(ctx, in) (string, error) wrapped by tools.safe"
    - "client construction split into newHTTPClient() and clientOptions() so retry/limiter/body-cap extend rather than rewrite"
    - "hermetic tests: FakeGitLab behind GITLAB_URL, real binary via mcp.CommandTransport and via Python stdio_client"

key-files:
  created:
    - go.mod
    - go.sum
    - .gitignore
    - cmd/gitlab-mcp/main.go
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/logging/logging.go
    - internal/logging/logging_test.go
    - internal/glclient/client.go
    - internal/tools/register.go
    - internal/tools/safe.go
    - internal/tools/whoami.go
    - internal/server/server.go
    - internal/testutil/fakegitlab.go
    - e2e/main_test.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py
  modified: []

key-decisions:
  - "Go-client e2e test does not pin the negotiated protocol version; the 2025-11-25 assertion lives only in smoke.py, because the Go client negotiates the newer 2026-07-28 revision"
  - "smoke.py unwraps anyio exception groups so a SMOKE FAIL line shows the real reason instead of an opaque ExceptionGroup"
  - "ReadOnlyHint: true is set on whoami (partially delivers v2 requirement UX-01 ahead of schedule; Plans 04-06 do the same for the read tools)"

patterns-established:
  - "Test tiers: unit (config/logging), real-binary Go e2e, Python mcp 1.30.0 smoke via uv run"
  - "Redaction is applied to log messages, string attrs, error attrs and as the final step of every tool result"

requirements-completed: [FND-01, FND-02, FND-03, UTIL-01, QA-01, QA-02]

duration: 25min
completed: 2026-09-24
---

# Phase 1 Plan 01: Walking Skeleton Summary

**Go stdio MCP server (go-sdk v1.8.0 + client-go/v2 v2.64.0) with a `whoami` tool, fail-fast config, token-redacting logger, stdout guard, and a real-binary test harness including a Python mcp 1.30.0 smoke client.**

## Performance

- **Duration:** ~25 min
- **Completed:** 2026-09-24
- **Tasks:** 3 (all TDD; 5 commits including RED commits)
- **Files created:** 18

## Accomplishments

- Real `gitlab-mcp.exe` (10.9 MB, `CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`) handshakes with both the Go `CommandTransport` client and the Python `mcp` 1.30.0 client, lists `whoami`, and answers it from a fake GitLab through a real `GET /api/v4/user` via client-go.
- Fail fast without `GITLAB_TOKEN`: exit code 1, message naming `GITLAB_TOKEN` on stderr, empty stdout.
- Token never leaks: redactor covers slog messages, string attrs, error attrs and final tool text; e2e and smoke tests assert `glpat-TESTSECRET` is absent from stdout, stderr and results.
- Fake GitLab observed 0 requests during initialize + tools/list (FND-01).

## Task Commits

1. **Task 1 (RED): failing e2e tests, fake GitLab, smoke script** - `2ed7b3d` (test)
2. **Task 2 (RED): failing config/logging tests** - `8643127` (test)
3. **Task 2 (GREEN): config + redacting logger** - `5d81b5e` (feat)
4. **Task 3 (GREEN): whoami skeleton** - `4f53e16` (feat)

## Verification Actually Run

- `go vet ./...`: clean. `gofmt -l cmd internal e2e`: empty.
- `go test ./... -count=1`: all packages pass (config, logging, e2e). In e2e: `TestStdioHandshakeAndWhoami` PASS, `TestStdioMissingToken` PASS, `TestPythonSmoke` PASS (uv 0.12.5 present, so it ran, not skipped).
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: succeeded (10,944,512 bytes).
- Negative check of the smoke script: pointing it at a closed port (`--base-url http://127.0.0.1:1`) exits 1 with `SMOKE FAIL: whoami returned isError: ...connectex...`, showing the script really exercises the tool and fails when it should.
- `grep fmt.Print` over `cmd/` and `internal/` (non-test): no matches.
- Task 1 red phase confirmed before implementation: `go test ./e2e/` failed because `cmd/gitlab-mcp` did not exist; Task 2 red confirmed by `undefined: Load` build failures.
- Not verified: live gitlab.com behaviour (`GET /personal_access_tokens/self` on a real PAT, real `whoami`). Acceptance for this plan is hermetic; the live check belongs to the optional live mode and Phase 4.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Go e2e test asserted the wrong protocol version**
- **Found during:** Task 3 (first full test run)
- **Issue:** `TestStdioHandshakeAndWhoami` asserted `ProtocolVersion == "2025-11-25"` on the Go SDK client, which negotiates the newer `2026-07-28` revision with the v1.8.0 server. The plan's truth (2025-11-25) concerns the Python 1.30.0 client (the agent), not the Go test client.
- **Fix:** The Go test now only checks that a non-empty protocol version is returned; the strict `2025-11-25` assertion stays in `scripts/smoke.py`, where it passes.
- **Files modified:** `e2e/stdio_test.go`
- **Commit:** `4f53e16`

**2. [Rule 1 - Bug] smoke.py hid the failure reason behind an ExceptionGroup**
- **Found during:** Task 3 negative check
- **Issue:** `SmokeFailure` raised inside anyio task groups surfaced as `ExceptionGroup: unhandled errors in a TaskGroup`, so `SMOKE FAIL: <reason>` was unreadable.
- **Fix:** Added `leaf_exceptions()` and print each leaf's real message (token redacted).
- **Files modified:** `scripts/smoke.py`
- **Commit:** `4f53e16`

**3. [Rule 3 - Blocking] go.mod resolved to `go 1.27.1` and lacked a go.sum entry for jsonschema-go**
- **Found during:** Task 1
- **Issue:** `go get` recorded the local toolchain version; `go test ./e2e/` failed on a missing go.sum entry before reaching the intended red state.
- **Fix:** `go mod edit -go=1.25.0` (kept through `go mod tidy`) and `go get github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0`. No modules beyond the approved ones were added; `go mod tidy` later pulled only their transitive dependencies.
- **Files modified:** `go.mod`, `go.sum`
- **Commit:** `2ed7b3d`, `4f53e16`

**4. Process note:** Task 2's tests were committed as a RED commit and the implementation as a separate GREEN commit; Task 1 and 3 followed the plan's structure (Task 1 RED, Task 3 GREEN).

## Assumption Drift (advisory)

- **Found during:** Task 3
- **Planned:** the e2e Go client would see protocol `2025-11-25` (plan truth #1 read as applying to any client).
- **Actual:** the Go SDK client negotiates `2026-07-28`; only the Python `mcp` 1.30.0 client negotiates `2025-11-25`.
- **Why it matters:** compatibility with the agent is proven by `smoke.py`, not by the Go e2e test.

## Notes for Traceability

- `ReadOnlyHint: true` on `whoami` (and on the read tools in Plans 04-06) partially delivers v2 requirement UX-01 (tool annotations) ahead of schedule.
- `errorText` currently returns `err.Error()`; Plan 02 replaces its body. `internal/glclient` exposes `newHTTPClient()` and `clientOptions()` as the extension points for retry policy, limiter and body cap.
- `internal/glclient` and `internal/server`/`internal/tools` have no unit tests of their own yet; they are covered through the real-binary e2e tests. Plan 02/03 add package-level tests.

## Known Stubs

None.

## Threat Flags

None beyond the plan's threat register. Mitigations T-01-01 through T-01-06 are implemented as planned (redactor, https/loopback-only `GITLAB_URL`, stdout guard, `recover()` and deadline in `safe`, token trimming with fail-fast, whoami output without email or token).

## Self-Check: PASSED

All 18 listed files exist in the worktree; commits `2ed7b3d`, `8643127`, `5d81b5e`, `4f53e16` exist in `git log`.
