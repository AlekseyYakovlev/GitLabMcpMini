---
phase: 01-connect-and-read
reviewed: 2026-09-24T00:00:00Z
depth: standard
files_reviewed: 41
files_reviewed_list:
  - cmd/gitlab-mcp/main.go
  - e2e/main_test.go
  - e2e/purity_test.go
  - e2e/python_smoke_test.go
  - e2e/stdio_test.go
  - go.mod
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/glclient/client.go
  - internal/glclient/client_test.go
  - internal/glclient/errors.go
  - internal/glclient/errors_test.go
  - internal/glclient/normalize.go
  - internal/glclient/normalize_test.go
  - internal/glclient/retry.go
  - internal/glclient/transport.go
  - internal/glclient/wire_test.go
  - internal/logging/logging.go
  - internal/logging/logging_test.go
  - internal/server/server.go
  - internal/testutil/fakegitlab.go
  - internal/testutil/session.go
  - internal/tools/errors.go
  - internal/tools/errors_test.go
  - internal/tools/file.go
  - internal/tools/file_test.go
  - internal/tools/format.go
  - internal/tools/format_test.go
  - internal/tools/helpers_test.go
  - internal/tools/projects.go
  - internal/tools/projects_test.go
  - internal/tools/ref.go
  - internal/tools/ref_test.go
  - internal/tools/register.go
  - internal/tools/safe.go
  - internal/tools/safe_test.go
  - internal/tools/schema_test.go
  - internal/tools/tree.go
  - internal/tools/tree_test.go
  - internal/tools/whoami.go
  - scripts/smoke.py
findings:
  critical: 0
  warning: 4
  info: 6
  total: 10
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-09-24
**Depth:** standard
**Files Reviewed:** 41
**Status:** issues_found

## Summary

The phase is in good shape overall: stdout purity is enforced (os.Stdout is redirected to stderr and the transport gets the real handle), the token is redacted at every sink (logs, tool text, panic text), retries are limited to GET/HEAD, response bodies are size-capped, timeouts are bounded at every stage, and path encoding is verified on the wire. No blocker-level defects were found.

The findings below are real but bounded: one token-forwarding hardening gap on HTTP redirects, one data-reachability defect in `get_file_contents` for very long lines, an off-by-one at the body-cap boundary, and a misleading timeout message. The rest is maintainability.

The `verify conventions` module was not available in this environment (no gsd plugin root), and the Go files have no JS/TS rule pack, so no CONVENTION-tier findings were emitted.

## Structural Findings (fallow)

Not provided for this review.

## Narrative Findings (AI reviewer)

## Warnings

### WR-01: PRIVATE-TOKEN header is forwarded on cross-host and https-to-http redirects

**File:** `internal/glclient/client.go:54-67`
**Issue:** `newHTTPClient` leaves `http.Client.CheckRedirect` at the default. Go's client strips only `Authorization`, `Www-Authenticate`, `Cookie` and `Cookie2` when a redirect crosses domains. client-go authenticates with the custom `Private-Token` header (`request_options.go:194`), which is copied to the redirect target unchanged, including on a downgrade from https to http. A redirect from the configured host to any other host (misconfigured `GITLAB_URL`, a compromised proxy, an open redirect upstream) therefore delivers the PAT, which carries the full `api` scope, to that host. The redactor cannot help here because the leak is on the wire, not in logs.
**Fix:** Refuse redirects that leave the configured origin or downgrade the scheme:
```go
func newHTTPClient(base *url.URL, l limits) *http.Client {
    return &http.Client{
        Timeout: 15 * time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= 5 {
                return errors.New("too many redirects")
            }
            if req.URL.Host != base.Host || req.URL.Scheme != base.Scheme {
                return errors.New("redirect to a different origin refused")
            }
            return nil
        },
        // ...Transport as before
    }
}
```
Add a wire test with a fake that answers 302 to a second httptest server and asserts that the second server never sees `Private-Token`.

### WR-02: A file line longer than 15000 characters (minified JS/JSON) cannot be read

**File:** `internal/tools/file.go:92-105`, `internal/tools/format.go:35-50`
**Issue:** When the selected range holds no newline inside the first 15000 runes, `Budget` performs a hard rune cut in the middle of a line. The footer then reports `показаны строки N-N` and tells the caller to use `start_line/end_line`. Those arguments only address whole lines, so the rest of that line is unreachable: `start_line=N+1` skips it, `start_line=N` returns the same first 15000 characters again. The same footer arithmetic is also wrong when the kept part begins with an empty line followed by a very long line. `Budget` then cuts at the first `\n` and returns an empty body, so `shown` is 0 and the footer claims lines `from-(from-1)`. The header still says `строки from-to из total`, which is misleading in both cases. Minified bundles, lock files and single-line JSON are exactly the kind of files an agent will try to read.
**Fix:** Detect the partial-line case and say so, and offer a way to continue, for example a `start_column`/`offset` argument, or at least state honestly that the line was cut:
```go
if truncated && body != "" && !strings.HasSuffix(strings.Join(lines[:shown], "\n"), body) {
    // hard cut inside line `from+shown-1`
    fmt.Fprintf(&sb, "\n[строка %d длиннее 15000 символов и обрезана; остаток недоступен через start_line/end_line]", from+shown-1)
}
```
Also guard `shown == 0` so the footer never prints an inverted range.

### WR-03: capBody rejects a body that is exactly the cap size

**File:** `internal/glclient/transport.go:18-28`
**Issue:** Once `left` reaches 0, every further `Read` returns `ErrBodyTooLarge`, including the read that would have returned `io.EOF`. A response of exactly `maxBody` bytes (8 MiB) is therefore reported as too large whenever the consumer reads to EOF (`io.ReadAll`, which client-go uses on error bodies and json decoders use for trailing data). The cap is documented as "more than left bytes", so the boundary is off by one. The reader also never distinguishes "cap reached and more data exists" from "cap reached and stream ended".
**Fix:** Probe one extra byte before failing:
```go
func (c *capBody) Read(p []byte) (int, error) {
    if c.left <= 0 {
        var one [1]byte
        n, err := c.ReadCloser.Read(one[:])
        if n > 0 {
            return 0, ErrBodyTooLarge
        }
        return 0, err // io.EOF for an exact-size body
    }
    if int64(len(p)) > c.left {
        p = p[:c.left]
    }
    n, err := c.ReadCloser.Read(p)
    c.left -= int64(n)
    return n, err
}
```
Add a test with a body of exactly `maxBody` bytes and one of `maxBody+1`.

### WR-04: Timeout message hard-codes "25 s" and is wrong for the 15 s HTTP client timeout

**File:** `internal/tools/errors.go:69-70`, `internal/glclient/client.go:56`, `internal/tools/safe.go:45-49`
**Issue:** `http.Client.Timeout` is 15 s per attempt, and since Go's client timeout error satisfies `errors.Is(err, context.DeadlineExceeded)`, `Classify` maps it to `KindTimeout`. The user then reads "Превышено время ожидания (25 с)" although the limit that fired was 15 s. The text also ignores `Deps.Timeout`, which is configurable and used by the tests with 200 ms, so the number can be wrong for any non-default deployment. Retries (up to 3 attempts at 15 s) further blur which limit expired.
**Fix:** Drop the number from the message, or pass the effective timeout into `toToolText`:
```go
case glclient.KindTimeout:
    return "Превышено время ожидания ответа GitLab."
```
and update the three tests that assert `(25 с)`.

## Info

### IN-01: NormalizeProject checks for a URL before percent-decoding

**File:** `internal/glclient/normalize.go:20-31`
**Issue:** `https%3A%2F%2Fgitlab.com%2Fg%2Fp` passes the scheme check, is then unescaped to `https://gitlab.com/g/p` and sent to GitLab as a project path (404 with a misleading "not found"). The prefix check is also case-sensitive (`HTTPS://...`) and does not catch a scheme-less `gitlab.com/g/p`.
**Fix:** Unescape first, then run a case-insensitive `strings.HasPrefix(strings.ToLower(s), "http://")` check; optionally also reject a first segment that contains a dot and matches the configured host.

### IN-02: Duplicated helpers and constants across packages; trivial wrapper

**File:** `internal/tools/errors.go:81-90` vs `internal/glclient/errors.go:16,134-141`; `internal/tools/safe.go:25-28`
**Issue:** `maxDetailRunes` and the rune-capping function exist twice (`capDetail` and `capRunes`), and `Classify` already caps `Detail`, so the second cap in `toToolText` is redundant for classified errors. `errorText` only forwards to `toToolText`.
**Fix:** Export one helper (or rely on the cap in `Classify`) and call `toToolText` directly from `safe`.

### IN-03: Redaction rewrites legitimate file content

**File:** `internal/tools/safe.go:60`
**Issue:** Every successful result passes through `redact`, so a repository file that contains a `glpat-...` example or fixture is returned as `[REDACTED]`. That is defensible for real secrets but silently alters "file contents" and the header line still reports the original byte size. The behaviour is undocumented in the tool description.
**Fix:** Keep the behaviour but mention it in `getFileContentsDescription`, or redact only the exact configured token in successful results.

### IN-04: Errors swallowed without any log in tokenLine

**File:** `internal/tools/whoami.go:37-40`
**Issue:** Any failure of `GetSinglePersonalAccessToken` (network, 403 for a token that cannot introspect itself, decode) produces the fixed line `scopes unavailable` with no diagnostic. The one tool meant for debugging token problems hides the reason.
**Fix:** Log the classified error at debug level (`d.Logger.Debug("token scopes unavailable", "error", err)`) and include the status class in the line, for example `scopes unavailable (403)`.

### IN-05: Tree/list truncation drops entries that the page footer says are covered

**File:** `internal/tools/format.go:82-93`, `internal/tools/tree.go:75-80`
**Issue:** When `Budget` cuts a page, the dropped items belong to the current page, yet the footer points to `page+1`. Entries between the cut and the end of the page are never shown unless the caller lowers `per_page` and re-pages from the start. The truncation footer does mention `уменьшите per_page`, so this is documented, but it still loses data silently for the common `recursive=true, per_page=100` call.
**Fix:** When truncated, state how many entries were shown and suggest the exact `per_page` (`shown`) to use so page boundaries line up, or compute `per_page` adaptively.

### IN-06: smoke.py hard-codes the tool set; agent may pass numeric project IDs

**File:** `scripts/smoke.py:34-40`, `internal/tools/projects.go:37`
**Issue:** `EXPECTED_TOOLS` must be edited on every new tool, or the smoke fails for the wrong reason. Separately, project IDs are numeric in GitLab, and the schema rejects a JSON number (`TestGetProjectNumberArgumentRejectedBySchema` locks that in). An LLM sending `123` gets a schema error and must retry. This is a usability risk rather than a bug.
**Fix:** Make the smoke assert a subset (`EXPECTED_TOOLS <= names`) for phases in progress. Consider accepting both string and number for `project` by declaring the field as `json.Number`-compatible in a custom schema, or keep the description explicit about the string form (already done).

---

_Reviewed: 2026-09-24_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
