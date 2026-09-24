---
phase: 01-connect-and-read
plan: 03
subsystem: testing
tags: [go, mcp, go-sdk, in-memory-transport, json-schema, stdio, redaction]

requires:
  - phase: 01-connect-and-read
    provides: "tools.Register/safe/Deps, glclient.New, logging redactor, FakeGitLab, e2e binPath/envWithout (plan 01)"
provides:
  - "testutil.Connect: in-memory MCP client session for any server"
  - "tools test helpers newTestDeps/newTestSession/callText and testToken"
  - "TestToolSchemas: name, description and flat-schema guard over every registered tool"
  - "TestSafeRecoversPanic/TestSafeRedactsErrorText"
  - "TestStdoutPurity: raw-pipe stdout purity, zero startup network I/O, token-free output at debug, EOF exit"
affects: [01-04, 01-05, 01-06, phase-02, phase-03]

tech-stack:
  added: []
  patterns:
    - "Every tool test uses newTestSession + callText (token-leak assertion built into callText)"
    - "Schema guard iterates ListTools, so new tools are covered automatically"

key-files:
  created:
    - internal/testutil/session.go
    - internal/tools/helpers_test.go
    - internal/tools/schema_test.go
    - internal/tools/safe_test.go
    - e2e/purity_test.go
  modified: []

key-decisions:
  - "Description limit measured in characters via utf8.RuneCountInString (Cyrillic is 2 bytes/char), matching Python len(str)"
  - "TestMissingTokenStdoutEmpty not added: TestStdioMissingToken already asserts empty stdout"

patterns-established:
  - "Raw-pipe e2e tests read stdout on a goroutine feeding a channel, drain it to EOF before cmd.Wait, and kill the process in t.Cleanup"

requirements-completed: [FND-01, FND-02, FND-03, FND-08, QA-01]

duration: 15min
completed: 2026-09-24
---

# Phase 1 Plan 03: Test harness and protocol-hygiene guards Summary

**Reusable in-memory MCP session helper, an automatic schema/name/description guard for every tool, panic and redaction tests for `safe`, and a raw-byte stdout purity test on the real binary.**

## Performance

- **Duration:** ~15 min
- **Completed:** 2026-09-24
- **Tasks:** 2/2
- **Files created:** 5 (test-only, no production code changed)

## Accomplishments

- `testutil.Connect` (imports only `mcp` and `testing`, no cycle with `internal/tools`) plus `newTestDeps`, `newTestSession`, `callText` with the exact signatures later plans rely on.
- `TestToolSchemas` checks name length (<=30), name pattern and no `gitlab_` prefix, description 1..900 characters, schema `type: object`, no `$ref/$defs/anyOf/oneOf/allOf`, no array-valued `type`, non-empty property descriptions.
- `TestSafeRecoversPanic` shows a panic containing the token yields `внутренняя ошибка: ` with `[REDACTED]` and the next call still succeeds; `TestSafeRedactsErrorText` covers plain errors.
- `TestStdoutPurity` runs the real binary at `LOG_LEVEL=debug` over raw pipes: initialize, initialized, tools/list (both with zero fake-GitLab requests), whoami against 401 (isError with "401"), unknown tool, invalid arguments. Every stdout line is a JSON-RPC 2.0 object without `\r`; after stdin close stdout reaches clean EOF and the process exits 0 within 2 s; neither stdout nor stderr contains the token.

## Task Commits

1. **Task 1: session helper, schema guard, safe tests** - `4add869` (test)
2. **Task 2: raw-pipe purity test** - `0e1d990` (test)

## Verification (observed)

- `go test ./internal/tools/ -run 'TestToolSchemas|TestSafe' -count=1 -v`: TestSafeRecoversPanic, TestSafeRedactsErrorText, TestToolSchemas/whoami all PASS.
- `go test ./e2e/ -run TestStdoutPurity -count=1 -v`: PASS (0.08 s).
- `go test ./... -count=1`: all packages ok.
- `go vet ./internal/... ./e2e/`: clean. `grep internal/tools internal/testutil/session.go` has no match.
- Not verified: the negative direction (that the tests fail when stdout is polluted or a token is leaked) was not exercised by mutation.

## Deviations from Plan

None - plan executed as written. The optional `TestMissingTokenStdoutEmpty` was skipped as the plan instructs, because `TestStdioMissingToken` already asserts empty stdout.

Notes: `gofmt -l` lists several pre-existing files from plan 01 (working-copy CRLF); none of the new files are listed. Out of scope, not changed.

## Known Stubs

None.

## Threat Flags

None. T-01-12..T-01-15 are mitigated by the tests above.

## Self-Check: PASSED

- All five created files exist; commits 4add869 and 0e1d990 present in git log.
