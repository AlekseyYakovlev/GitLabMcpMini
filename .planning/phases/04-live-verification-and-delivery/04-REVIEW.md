---
phase: 04-live-verification-and-delivery
reviewed: 2026-09-25T00:00:00Z
depth: standard
files_reviewed: 9
files_reviewed_list:
  - e2e/build_test.go
  - e2e/live_script_test.go
  - internal/tools/schema.go
  - internal/tools/testdata/tools_list.golden.json
  - internal/tools/upsert_test.go
  - internal/tools/write.go
  - internal/tools/write_test.go
  - README.md
  - scripts/live.py
findings:
  critical: 0
  warning: 6
  info: 6
  total: 12
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-09-25
**Depth:** standard
**Files Reviewed:** 9
**Status:** issues_found

## Summary

No blockers. Token redaction in `scripts/live.py` is consistently applied (console, stderr, transcript, plus post-redaction asserts), and the write-target guard and cleanup correctly restrict themselves to the `live/<ts>-` prefix. The Go write tools are sound (single POST, validation before any request, follow-up read never fails a committed write). The weaknesses are the sandbox safety gate (a script that merges into the default branch of any project has no real opt-in beyond a flag), a Python-version mismatch in the error path, a read-only-token step that can write when scopes cannot be read, security-relevant asserts, and missing tests for the Python safety logic.

Structural findings (fallow): none provided.
The convention checker could not be run (the gsd plugin `gsd-tools.cjs` was not found in this environment; the JS/TS packs do not apply to Go/Python files anyway).

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: Sandbox safety gate is only a warning in a docstring; step 4 "sentinel" is not a sentinel

**File:** `scripts/live.py:404-407`, `scripts/live.py:855`
**Issue:** The script creates branches, commits and merges an MR into the default branch of whatever `--project` names and the token can write to. The only protection is documentation. The "D-05 sandbox sentinel" (`README.md` exists) is satisfied by practically every repository, so it does not distinguish a sandbox from a real project. A typo in `--project` (or a copied real project path) silently writes and merges into production history (files under `live-run/<ts>/` are permanent). The project brief states the script "must never write to the default branch" of a real project, but the merge into the default branch is intentional and unguarded against wrong targets.
**Fix:** Require an explicit sandbox marker before the first write, for example a `.live-sandbox` file in the default branch (checked via `get_file_contents`) or a required `--i-know-this-writes` / `--confirm-project GROUP/PROJECT` flag that must equal `--project`. Fail with exit code 2 before any write if it is absent.

### WR-02: Read-only-token session can perform a write when scopes cannot be determined

**File:** `scripts/live.py:652-670`
**Issue:** The C-session bails out only when `scopes is not None and "api" in scopes`. If the read-only whoami fails (non-fatal), or returns `scopes unavailable` (`parse_scopes` returns `None`), the script proceeds to `create_branch` with a token it could not prove is read-only. If `GITLAB_TOKEN_READONLY` actually holds an `api` token, a real branch `live/<ts>-ro` is created (the step then FAILs, and cleanup removes it, but the write already happened and the "read-only" claim is untested). The comment/README promise a read-only token.
**Fix:** Treat unknown scopes as not-proven: `if scopes is None or "api" in scopes:` mark the step FAIL (or SKIPPED with reason) and return without writing.

### WR-03: `BaseExceptionGroup` used while the script declares `requires-python >= 3.10`

**File:** `scripts/live.py:2`, `scripts/live.py:160`
**Issue:** `BaseExceptionGroup` is a builtin only on Python 3.11+. On 3.10 (allowed by the PEP 723 header) `leaf_exceptions` raises `NameError` inside the `except` handler of `main`, masking the real failure; the `finally` still runs cleanup but the run ends with a traceback and no recorded step. anyio on 3.10 raises `exceptiongroup.BaseExceptionGroup`, not the builtin.
**Fix:** Either raise the floor (`requires-python = ">=3.11"`) or import: `try: BaseExceptionGroup except NameError: from exceptiongroup import BaseExceptionGroup` (the `exceptiongroup` backport is already a dependency of anyio on 3.10).

### WR-04: Secret-leak guards implemented with `assert`

**File:** `scripts/live.py:829-832`, `scripts/live.py:901-904`
**Issue:** The last line of defence against writing a token or identity into the transcript uses `assert` and catches `AssertionError`. Under `python -O` (or `PYTHONOPTIMIZE`, including via a wrapper) asserts are stripped and an unredacted transcript would be written silently. Also the generic `except AssertionError` would swallow an unrelated assertion from a dependency.
**Fix:** Raise a dedicated exception explicitly:
```python
class TranscriptLeak(Exception): ...
if any(s and s in doc for s in SECRETS) or GLPAT_RE.search(doc) or IDENTITY_LINE.search(doc):
    raise TranscriptLeak("...")
```
and catch `TranscriptLeak` in `main`.

### WR-05: No automated tests for the safety-critical Python logic

**File:** `e2e/live_script_test.go:15-67`, `scripts/live.py:138-147`, `scripts/live.py:258-279`, `scripts/live.py:712-744`
**Issue:** The only test covers argument/usage exit codes. `check_write_target` (default-branch guard), `redact`/`redact_identity`, `crit_status`/`has_failures` and the cleanup prefix guard have no tests at all, yet they are the features the project relies on to avoid leaking tokens or touching the default branch. A regression there would only surface during a real write run.
**Fix:** Add a small offline test (Python `unittest` invoked from a Go test, or a `scripts/test_live.py`) that imports `live.py` and asserts: guard rejects `branch == DEFAULT`, non-prefixed branches and non-string values; accepts `live/<ts>-x`; `redact` masks all secrets longest-first; `write_transcript` refuses a doc containing a token.

### WR-06: `TestLiveScriptUsage` claims "no network" but `uv run` resolves `mcp==1.30.0`; no timeout

**File:** `e2e/live_script_test.go:12-35`
**Issue:** The doc comment says "without any network access", but `uv run scripts/live.py` resolves and installs the PEP 723 dependency (`mcp==1.30.0`) on first use, which needs network or a warm cache. In an offline environment the subtests fail with an unexpected exit code (the `exitErr.ExitCode()` is uv's, possibly 1/2 by coincidence, so "exit code 2" can even pass for the wrong reason: a uv resolution failure). `exec.Command` also has no timeout, so a stalled resolve hangs `go test` until the global timeout.
**Fix:** Use `exec.CommandContext` with a timeout (for example 2 minutes), assert on a script-specific message (for "no arguments" check that output names `--project`), and correct the comment or skip when `uv` cannot resolve offline (`--offline` plus skip on failure).

## Info

### IN-01: Transcript redacts only the whoami identity; other steps leak username/name

**File:** `scripts/live.py:145-147`, `scripts/live.py:288-289`
**Issue:** `redact_identity` is applied only for `whoami`. `get_commit`, `get_merge_request`, `list_merge_request_notes` and `get_merge_request_diffs` typically contain the author's username/name/email and profile URLs, which go verbatim into `LIVE-RUN.md` under `.planning/` (committed). This contradicts the apparent intent of hiding the identity line.
**Fix:** Also redact the username learned from `whoami` (add `@username` and the `web_url` host path to a second redaction set), or state in the docstring that only the whoami line is hidden.

### IN-02: Weak checks that can pass for the wrong reason

**File:** `scripts/live.py:603-604`, `scripts/live.py:499-502`, `scripts/live.py:520-523`
**Issue:** E6 uses `check=lambda t: True` (the close is never verified). Step 16 treats any text containing the substring `"merged"` as merged and ignores `is_err`. Step 18 records PASS unconditionally even when the branch was not removed (only the note differs).
**Fix:** E6: assert `"closed"` in the text or re-read the MR; step 16: match `state: merged` and require `not is_err`; step 18: record status `PASS` only when `gone`, otherwise a distinct WARN status.

### IN-03: `schema.go` hand-written schema is weaker than its own descriptions

**File:** `internal/tools/schema.go:28-39`, `internal/tools/schema.go:36-39`
**Issue:** The `actions` description promises "1..50 items" and `action` says "create, update, delete or move", but the schema has no `minItems`/`maxItems` and no `enum`. Clients and models get no machine-readable constraint; enforcement happens only in handler code (`validateCommitInput`, `toFileAction`).
**Fix:** Add `"minItems": 1, "maxItems": maxCommitActions` and `"enum": []any{actCreate, actUpdate, actDelete, actMove}` (then regenerate `tools_list.golden.json`). Note `toFileAction` lower-cases and trims the action, so an enum would make that leniency unreachable; decide which contract is intended.

### IN-04: `create_or_update_file` silently drops the concurrency guard when `LastCommitID` is empty, and hard-fails on undecodable content

**File:** `internal/tools/write.go:244-253`, `internal/tools/write.go:193-195`
**Issue:** If the read returns an empty `LastCommitID`, `commitCore` omits `last_commit_id` and the "protected against overwriting others' edits" promise (see the tool description) silently no longer holds. Separately, a base64 decode error of the existing file aborts the update even though the content is only needed for an optional CRLF warning.
**Fix:** Either treat empty `LastCommitID` on an existing file as an error/warning, and make the CRLF probe best-effort (`if raw, err := decodeFileContent(f); err == nil { ... }`).

### IN-05: Empty `WebURL` yields a trailing blank line in tool output

**File:** `internal/tools/write.go:277`, `internal/tools/write.go:394`
**Issue:** `lines = append(lines, commit.WebURL)` is unconditional, so an empty `web_url` produces a trailing newline in the MCP text. Cosmetic, but the tests assert exact line counts only for the non-empty case.
**Fix:** Append only when `commit.WebURL != ""`.

### IN-06: Test assertions that match too loosely; PE import allowlist brittle

**File:** `internal/tools/write_test.go:231`, `internal/tools/write_test.go:240`, `e2e/build_test.go:45-51`
**Issue:** `"too many"` only checks the substring `"50"` and `"delete with content"` only `"delete"` (the unknown-action message also contains the word `delete`), so these could pass on the wrong error. `TestBinaryHasNoExternalDependencies` fails hard if a future Go toolchain adds any additional system DLL (for example `ntdll.dll`) even though the binary is still self-contained; the size ceiling (25 MB) is more than twice the documented 11 MB, so a regression would not be noticed.
**Fix:** Match on the full expected phrase (`"не больше 50 файлов"`, `"для delete content не нужен"`); document the allowlist as intentional or extend it deliberately; tighten `maxExeSize` (for example 16 MB).

---

_Reviewed: 2026-09-25_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
