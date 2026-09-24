---
phase: 01-connect-and-read
plan: 02
subsystem: api
tags: [go, client-go, retryablehttp, error-mapping, url-encoding, tdd]

requires:
  - phase: 01-connect-and-read (plan 01)
    provides: glclient.New skeleton, tools.safe wrapper, FakeGitLab test double, whoami tool
provides:
  - GET-only retry policy (max 2 retries, waits <= 5 s each), writes sent exactly once
  - No-op limiter and 8 MiB response body cap (ErrBodyTooLarge)
  - glclient.Classify plus Kind/Error types for every failure class
  - tools.toToolText / withSubject short Russian isError wording (no raw bodies)
  - glclient.NormalizeProject / NormalizeRepoPath and wire-verified encoding tests
affects: [01-03, 01-04, 01-05, 01-06, phase-02-writes, phase-03]

tech-stack:
  added: [github.com/hashicorp/go-retryablehttp v0.7.8 (now a direct requirement)]
  patterns:
    - "Retry policy lives in client construction, not in call sites"
    - "Classify first, word second: glclient classifies, tools word the message"
    - "Normalisers never percent-encode; client-go encodes once; wire tests lock it"

key-files:
  created:
    - internal/glclient/retry.go
    - internal/glclient/transport.go
    - internal/glclient/errors.go
    - internal/glclient/normalize.go
    - internal/glclient/client_test.go
    - internal/glclient/errors_test.go
    - internal/glclient/normalize_test.go
    - internal/glclient/wire_test.go
    - internal/tools/errors.go
    - internal/tools/errors_test.go
  modified:
    - go.mod
    - internal/glclient/client.go
    - internal/tools/safe.go
    - internal/tools/whoami.go

key-decisions:
  - "Custom CheckRetry/Backoff/RetryMax(2)/no-op limiter instead of WithoutRetries or WithOnlyIdempotentRetries (both wrong for D-02/D-03)"
  - "Body cap fails on the read after max bytes; a body of exactly the cap size is also rejected (accepted simplification)"
  - "Classify short-circuits on an already-classified *glclient.Error so tools can construct and wrap errors in tests"
  - "5xx Detail is always empty so HTML error pages are never echoed (D-15)"

patterns-established:
  - "TDD per task: test commit (RED, does not compile) then feat commit (GREEN)"
  - "Wire assertions compare the fake server's raw RequestURI, never the decoded path"

requirements-completed: [FND-04, FND-05, FND-07, QA-01]

duration: 25min
completed: 2026-09-24
---

# Phase 01 Plan 02: Retry policy, error mapping, path normalisation Summary

**Read-only retries (GET/HEAD, max 2, Retry-After aware), writes never repeated, every GitLab failure class mapped to a short Russian isError text, and single-encoding project/file paths locked by exact-wire tests.**

## Performance

- **Duration:** about 25 min
- **Tasks:** 3 of 3
- **Files modified:** 14 (10 created, 4 modified)

## Accomplishments

- `glclient.New` now applies `WithCustomRetry`, `WithCustomBackoff`, `WithCustomRetryMax(2)` and `WithCustomLimiter(noLimiter{})`. GET 503 gives 3 attempts, GET 429 with `Retry-After: 1` gives 3 attempts in about 2 s, a 429 with `Retry-After: 30` gives 1 attempt, and POST/PUT/DELETE give exactly 1 attempt whatever the status.
- A response body over the cap fails with `ErrBodyTooLarge`; a context deadline ends a slow call in about 0.3 s with `errors.Is(err, context.DeadlineExceeded)`.
- `Classify` covers 401/403/404/429/5xx/other 4xx/network/timeout/cancel/oversize/decode. `toToolText` produces the texts from the plan; `safe()` delegates to it and logs failures at Warn; `whoami` labels its 404 subject.
- `NormalizeProject` / `NormalizeRepoPath` plus a wire table (subgroup, dot, space, `#`, `+`, `%`, Cyrillic, dotfile, `feature/x y+z#1`, tree query) all matched the RESEARCH values on the first run, with no double encoding.

## Task Commits

1. **Task 1: retry policy, backoff, limiter, body cap**
   - `49e8cb5` test (RED: build fails, symbols undefined)
   - `871fdb4` feat (GREEN)
2. **Task 2: error classification and Russian wording**
   - `d12c571` test (RED)
   - `de0e19f` feat (GREEN)
3. **Task 3: normalisers and wire tests**
   - `1403b34` test (RED)
   - `91c1d7f` feat (GREEN)

## Verification (commands actually run, results observed)

- `go test ./internal/glclient/ -run 'Retry|Backoff|Limiter|Deadline|BodyCap|Refused' -count=1 -v`: PASS (all attempt-count cases, including POST/PUT/DELETE = 1).
- `go test ./internal/glclient/ ./internal/tools/ ./e2e/ -count=1`: `ok` for all three.
- `go test ./internal/glclient/ -run 'Normalize|Wire' -count=1 -v`: PASS.
- `go test ./... -count=1`: all packages `ok`. `go vet ./...`: clean.
- Acceptance greps: `WithCustomRetryMax(2)`, `WithCustomLimiter(noLimiter{})` present; no `WithoutRetries`/`WithOnlyIdempotentRetries` in non-test code; no `PathEscape`/`QueryEscape` in `normalize.go`; `go.mod` lists go-retryablehttp v0.7.8 as a direct requirement; `safe.go` contains `toToolText` and `внутренняя ошибка: `.
- Not verified: behaviour against real gitlab.com (all evidence is from the local fake server and client-go v2.64.0); the 8 MiB cap is proven at a 1 MiB test limit only.

## Deviations from Plan

None to the plan's scope. Minor implementation notes:

- `toToolText` also caps Detail at 300 runes (in addition to `Classify`), because tests build `*glclient.Error` values directly; this follows the plan's "detail capped at 300 runes" line.
- `Classify` returns an already-classified `*glclient.Error` unchanged (needed so tool-layer tests can feed constructed errors through `toToolText`).
- `gofmt -l .` lists many pre-existing files (working-tree CRLF from git autocrlf); none of the files added here are affected in a content sense. Left untouched as out of scope.

## Known Limitations

- A body whose size is exactly the cap is rejected too (the read after the cap returns `ErrBodyTooLarge`). Harmless at 8 MiB.
- The 15 s per-attempt `http.Client.Timeout` surfaces as a network error, not `KindTimeout`; only the call-level context deadline (25 s) yields "Превышено время ожидания (25 с)".

## Known Stubs

None.

## Threat Flags

None. Mitigations T-01-07..T-01-11 are implemented and covered by tests (write methods 1 attempt, body cap, no HTML echo, URL project refs rejected, `..` rejected).

## Self-Check: PASSED

All created files exist and all six commits are present in the worktree branch history.
