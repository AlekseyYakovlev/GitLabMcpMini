---
phase: 01-connect-and-read
plan: 06
subsystem: tools
tags: [go, mcp, gitlab, repository-files, e2e, python-smoke]
requires:
  - phase: 01-connect-and-read (plan 05)
    provides: resolveRef, refLabel, errEmptyRepo, Budget/OutputBudget, withSubject
provides:
  - get_file_contents tool (READ-04) with base64 decode, line ranges, truncation footer, binary marker, oversize error
  - TestStdioWirePaths (real binary, once-encoded wire paths)
  - five-tool Python mcp 1.30.0 smoke with hermetic and live read-only modes
affects: [phase 02 tools, README]
tech-stack:
  added: []
  patterns: ["file-specific truncation footer replaces the list footer for single-object tools", "smoke.py forces UTF-8 output for piped Windows stdout"]
key-files:
  created:
    - internal/tools/file.go
    - internal/tools/file_test.go
  modified:
    - internal/tools/register.go
    - e2e/stdio_test.go
    - e2e/python_smoke_test.go
    - scripts/smoke.py
key-decisions:
  - "Empty file returns '<path> @ <ref> — файл пуст, N байт' instead of a 0-0 line range"
  - "Truncation footer K is the count of complete lines kept, reported as a-(a+K-1) so it stays correct with start_line"
requirements-completed: [READ-04, FND-04, FND-06, QA-01, QA-02]
metrics:
  completed: 2026-09-24
---

# Phase 1 Plan 06: get_file_contents Summary

**`get_file_contents` reads any text file whole, by inclusive line range, or head-truncated with an explicit footer, returns a marker for binary files, and all five tools are now proven through the real binary and the Python mcp 1.30.0 client.**

## Accomplishments
- `get_file_contents`: normalises project and path (rejects empty path and `..`), resolves the default branch when `ref` is empty (explicit ref costs no extra request), decodes base64, detects binary content (NUL or invalid UTF-8 within the first 8000 bytes, with the probe trimmed back to a rune boundary), strips a BOM, trims CR, and returns a header `path @ ref (ветка по умолчанию) — строки a-b из N, M байт`.
- Output is cut at 15 000 runes on a line boundary with the footer `[файл обрезан на 15000 символах: показаны строки a-K из N; используйте start_line/end_line]`.
- Invalid ranges (start past the end of the file, end before start, negatives) return isError; an end past the file is clamped.
- 8 MiB transport cap surfaces as the readable "слишком большой" error; 404 is worded with the subject "файл".
- `TestStdioWirePaths` drives the real binary and asserts exact RequestURIs for dotted project, Cyrillic/space project, file paths with space, `#`, `+`, `%`, Cyrillic, ref with `/`, and the awkward tree path; it also asserts the tool list is exactly the five names.
- `scripts/smoke.py` now checks the tool set and calls every tool plus a missing project (isError with 404). Hermetic mode = `--base-url`; live mode = no `--base-url`, requires `--project`, read-only. `TestPythonSmoke` covers the hermetic mode.

## Task Commits
1. Task 1: get_file_contents tool - `b97c8de`
2. Task 2: wire-path e2e and five-tool smoke - `b523891`

## Verification (actually run)
- `go test ./internal/tools/ -count=1`: ok (all file tests plus TestToolSchemas with 5 tools).
- `go vet ./...`: clean.
- `go test ./... -count=1`: all packages ok, including e2e (TestStdioWirePaths 7 subtests PASS, TestPythonSmoke PASS and not skipped, TestStdoutPurity).
- `GITLAB_TOKEN=x uv run scripts/smoke.py` (no `--base-url`, no `--project`): exit 2 with the "requires --project" message.
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`: produced an 11 MB binary (gitignored, not committed).
- `grep -En "\*int|\*string" internal/tools/file.go`: no output.
- Not verified: live mode against real gitlab.com (needs a real token and project), and the A2/A3 open questions (`default_branch` under `simple=true`, pagination footer on real data). Only the fake GitLab was used.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] smoke.py crashed printing Russian text to a piped Windows stdout**
- **Found during:** Task 2 (first TestPythonSmoke run: `UnicodeEncodeError: 'charmap' codec`)
- **Fix:** `sys.stdout`/`sys.stderr` are reconfigured to UTF-8 at the start of `main()`.
- **Files modified:** scripts/smoke.py
- **Commit:** b523891

**2. [Rule 3 - Blocking] Literal BOM in source**
- **Found during:** Task 1 (`invalid BOM in the middle of the file` compile error; the write tool expanded the unicode escape of the byte order mark into the literal character)
- **Fix:** a named constant `utf8BOM = "\xEF\xBB\xBF"` in file.go.
- **Commit:** b97c8de

`gofmt -l` lists every file because of CRLF working-copy line endings (pre-existing, noted in plan 05); `go vet` is clean.

## Known Stubs
None.

## Threat Flags
None beyond the plan's threat model (T-01-23..27 mitigations implemented: body cap, budget, binary marker, `..` rejection, wire-encoding test, token never printed, range clamping).

## Self-Check: PASSED
Files file.go, file_test.go, register.go, stdio_test.go, python_smoke_test.go, smoke.py exist; commits b97c8de and b523891 exist.
