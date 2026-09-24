---
phase: 02-history-and-write
reviewed: 2026-09-24T00:00:00Z
depth: standard
files_reviewed: 22
files_reviewed_list:
  - e2e/python_smoke_test.go
  - e2e/stdio_test.go
  - internal/testutil/fakegitlab.go
  - internal/tools/branch.go
  - internal/tools/branch_test.go
  - internal/tools/commits.go
  - internal/tools/commits_test.go
  - internal/tools/compare.go
  - internal/tools/compare_test.go
  - internal/tools/diff.go
  - internal/tools/diff_test.go
  - internal/tools/errors.go
  - internal/tools/errors_test.go
  - internal/tools/file.go
  - internal/tools/register.go
  - internal/tools/schema.go
  - internal/tools/schema_test.go
  - internal/tools/testdata/tools_list.golden.json
  - internal/tools/upsert_test.go
  - internal/tools/write.go
  - internal/tools/write_test.go
  - scripts/smoke.py
findings:
  critical: 1
  warning: 5
  info: 5
  total: 11
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-09-24
**Depth:** standard
**Files Reviewed:** 22
**Status:** issues_found

## Summary

Reviewed the history/compare/write tools and their tests. Checked against the stated constraints:

- **Token leak:** no path found. Errors go through `toToolText` and `safe` redaction, and raw 5xx bodies are dropped in `glclient.classifyResponse`.
- **Non-idempotent POST retry:** none found. `checkRetry` refuses to retry anything except GET/HEAD, and the tests assert exactly one POST.
- **Stdout hygiene:** no `fmt.Print*` in the reviewed code.

The defects are in write-safety semantics rather than transport:

- **commit_files:** a documented-required field is not actually required, so a create or update with no `content` silently writes an empty file.
- **create_or_update_file:** the advertised concurrent-edit protection does not protect against what the model has seen.
- **Failure wording:** several write failure kinds do not tell the model the write may have been applied.
- **compare_refs:** a huge `page` value overflows and panics.
- **smoke.py:** it will run the write tools against any server given by `--base-url`, including real gitlab.com.

Structural findings (fallow): none were provided.

Convention check: `gsd-tools verify conventions` could not be run (plugin root not resolvable in this environment). No CONVENTION findings are emitted.

## Critical Issues

### CR-01: commit_files silently commits an EMPTY file when `content` is omitted for create/update

**File:** `internal/tools/write.go:130-134` (also `internal/tools/schema.go:34`, `internal/tools/write.go:59-64`)
**Issue:** `ActionIn.Content` is a plain `string` with `omitempty`, and the hand-written schema lists only `action` and `file_path` as required. The description promises "content ... required for create and update" but nothing enforces it. A missing `content` is indistinguishable from `""`, and for `create`/`update` `toFileAction` sets `sendContent = true` unconditionally. So `{"action":"update","file_path":"src/main.go"}` (a plausible model mistake, since content is optional for `move` and forbidden for `delete`) commits an empty `src/main.go` and reports success. `TestCommitFilesEmptyContentIsSent` locks in that `""` is valid, but no test covers the omitted case. This is a silent destructive write on the tool that is explicitly marked `DestructiveHint: true`.
**Fix:** Make presence detectable. The schema is hand-written, so a pointer field does not leak `["null", ...]` into it:
```go
type ActionIn struct {
    Action       string  `json:"action"`
    FilePath     string  `json:"file_path"`
    Content      *string `json:"content,omitempty"`
    PreviousPath string  `json:"previous_path,omitempty"`
}
// in toFileAction:
case actCreate, actUpdate:
    if a.Content == nil {
        return fileAction{}, fmt.Errorf("actions[%d]: для %s нужен content (пустая строка допустима для пустого файла)", i, action)
    }
    fa.Content, fa.sendContent = *a.Content, true
case actDelete:
    if a.Content != nil { ... }
case actMove:
    if a.Content != nil { fa.Content, fa.sendContent = *a.Content, true }
```
Add a test with a create and an update action that carry no `content` key and assert no request is sent.

## Warnings

### WR-01: Write failures of kind Canceled / Decode / TooLarge / Other do not say the write may have been applied

**File:** `internal/tools/errors.go:214-224`
**Issue:** `writeText` appends `writeUnknownOutcome` only for `KindServer`, `KindTimeout` and `KindNetwork`. A POST can equally have been applied when:
- `KindDecode`: GitLab answered 201 but the JSON could not be parsed.
- `KindTooLarge`: the response body exceeded the 8 MB cap.
- `KindCanceled`: the MCP client sent `notifications/cancelled` after the request left.
- `KindOther`.

These fall through to `baseText`, which says "Неожиданный ответ GitLab", "Вызов отменён.", etc. The model reads that as "nothing happened" and retries, producing a duplicate commit or branch. For `KindServer` the message is also self-contradictory: `baseText` says "повторите позже", then the suffix says "проверьте состояние перед повтором".
**Fix:** Handle every kind for which the request may have reached GitLab, and drop the "повторите позже" phrase for writes:
```go
case glclient.KindServer, glclient.KindTimeout, glclient.KindNetwork,
     glclient.KindCanceled, glclient.KindDecode, glclient.KindTooLarge, glclient.KindOther:
    return baseText(e, subject) + writeUnknownOutcome, true
```
Add table cases in `errors_test.go` for Canceled and Decode on `opCommit`.

### WR-02: create_or_update_file's "stale write" protection does not protect against what the model has seen

**File:** `internal/tools/write.go:234-252` (input at `:202-208`, description at `:25-31`)
**Issue:** The tool reads the file itself immediately before the commit and sends that fresh `last_commit_id`. The model cannot pass the `last_commit_id`/blob it saw in its earlier `get_file_contents`, because `UpsertFileIn` has no such field. So any edit made between the model's read (seconds to minutes ago) and this call is silently overwritten. The lock only covers the few milliseconds between the tool's own GET and POST. Yet the description says "Защищает от перезаписи чужих правок ... при конфликте перечитайте файл и повторите", which the model will take as real protection. `TestCreateOrUpdateFileUpdatesWithLastCommitID` also validates only the internal read-then-write.
**Fix:** Add an optional `last_commit_id` input (or `expected_blob_id`; `get_file_contents` would need to expose it in its header). When supplied, send it instead of the freshly read value. If it is not supplied and the file exists, either state in the description that no concurrency protection is offered against earlier reads, or require the field for updates. At minimum, correct the description.

### WR-03: Integer overflow in compare_refs paging causes a slice-bounds panic

**File:** `internal/tools/compare.go:99-110` (and `internal/tools/format.go:71-75`)
**Issue:** `page` is an unbounded `int` from the model. `lo := (page-1)*perPage` and `hi := page*perPage` overflow for large pages. Example: `page=4611686018427387904, per_page=20` gives `lo=-20`, `hi=0`. Then `lo >= total` is false and `res.Diffs[lo:hi]` panics. `safe` recovers it, so the server survives, but the model gets "внутренняя ошибка: runtime error: slice bounds out of range" instead of the friendly "на странице N файлов нет". The same unbounded `page+1` appears in `PageFooter`.
**Fix:**
```go
if page > total/perPage+1 {            // beyond the last page: no overflow possible
    lines = append(lines, fmt.Sprintf("файлов: %d", total),
        fmt.Sprintf("на странице %d файлов нет: всего файлов %d", page, total))
} else { lo, hi := (page-1)*perPage, min(page*perPage, total) ... }
```
Add a test with `page: 1<<62`.

### WR-04: smoke.py runs the write tools against whatever `--base-url` it is given, including real gitlab.com

**File:** `scripts/smoke.py:167-206` (mode decision at `:235-241`)
**Issue:** The docstring says write tools run "only in hermetic mode", but "hermetic" is decided solely by `if base_url:`. `--base-url https://gitlab.com --project me/repo` (a natural way to run "live") makes the script:
- create branch `smoke/branch`;
- commit `smoke/a.md` and a delete of `old.txt`;
- overwrite `README.md` with `# Hello\nworld\n` on that branch.

That is unexpected writes to a real project with a real PAT, from a script the documentation calls read-only for live use.
**Fix:** Gate writes on an explicit signal, for example a loopback check on the URL host (`127.0.0.1`/`localhost`) or a `--allow-writes` flag that defaults to false:
```python
from urllib.parse import urlparse
hermetic = base_url is not None and urlparse(base_url).hostname in {"127.0.0.1", "localhost", "::1"}
if hermetic: ...
```

### WR-05: create_or_update_file silently overwrites an existing binary file with text

**File:** `internal/tools/write.go:239-245`
**Issue:** `isBinary(raw)` is used only to skip the CRLF warning. If the target path holds a binary file (image, archive), the tool replaces it with UTF-8 text as a normal "updated" success, with no warning. The tool is described as text-only, and `TestCreateOrUpdateFileBinaryExistingIsUpdated` asserts this destructive behaviour as intended.
**Fix:** Refuse, or at least warn, when the existing blob is binary:
```go
if isBinary(raw) {
    return "", errors.New("файл на ветке бинарный: create_or_update_file пишет только текст; заменить его нельзя")
}
```
Alternatively append a warning line as is done for CRLF, and update the test.

## Info

### IN-01: `emptyPatchReason` is dead in production code

**File:** `internal/tools/diff.go:93-98`
**Issue:** It is referenced only from `diff_test.go`. `emptyPatchHeading` calls `emptyPatchKind` directly.
**Fix:** Delete it and have the test call `emptyPatchKind`. Similarly, `errorText` in `safe.go:26` is a one-line pass-through to `toToolText`.

### IN-02: Timeout wording hard-codes 25 s

**File:** `internal/tools/errors.go:103`
**Issue:** "Превышено время ожидания (25 с)" is fixed text, but the deadline is `Deps.Timeout` (configurable, `CallTimeout` is only the default). The underlying HTTP client timeout is 15 s, so a single stalled POST hits 15 s, not 25 s.
**Fix:** Drop the number from the message, or thread the real value through.

### IN-03: `until` given as a plain date excludes that whole day

**File:** `internal/tools/commits.go:71-76`
**Issue:** `until="2026-09-24"` parses to `2026-09-24T00:00:00Z`, so commits made on the 24th are excluded, which is the opposite of what a model or user usually means. The description does not warn about it. `since="2026-09-01"` is correct.
**Fix:** For a date-only `until`, add 24 h (minus 1 s), or state the semantics in `listCommitsDescription`.

### IN-04: UTF-8 validation of `content` can never fire; U+FFFD content is committed silently

**File:** `internal/tools/write.go:159`
**Issue:** Content arrives via `encoding/json`, which replaces invalid UTF-8 with U+FFFD before the handler runs. `!utf8.ValidString(a.Content)` is therefore always false, while the corrupted text is committed.
**Fix:** Also reject or warn on `strings.ContainsRune(a.Content, utf8.RuneError)` when the input does not legitimately contain U+FFFD, or drop the dead branch and keep only the NUL check.

### IN-05: smoke.py bypasses token redaction for its own failures; e2e Python smoke can hang offline

**File:** `scripts/smoke.py:245-249`; `e2e/python_smoke_test.go:15-18, 82`
**Issue:**
- For a `SmokeFailure` the `.replace(token, "[REDACTED]")` result is overwritten by `str(leaf)`. `SmokeFailure` messages embed tool result text. That text is already redacted server-side, but the client-side guard is then inconsistent.
- `TestPythonSmoke` skips only when `uv` is missing. With `uv` present but offline it tries to fetch `mcp==1.30.0` and fails after up to 3 minutes.

**Fix:** Apply the redaction to `SmokeFailure` too (`message = str(leaf).replace(token, "[REDACTED]")`). Add a short pre-flight (`uv run --offline` or a `-short` skip) so a run without network fails or skips quickly.

---

_Reviewed: 2026-09-24_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
