# Pitfalls Research

**Domain:** stdio MCP server wrapping GitLab REST API v4 (gitlab.com, PAT auth), Windows 11 dev box, client = AiAdventAgentV2 (Python, `mcp` 1.30.0)
**Researched:** 2026-09-24
**Confidence:** MEDIUM-HIGH (GitLab and MCP facts verified against current official docs; client-side constraints verified by reading the agent's source and the installed SDK; a few Windows process-lifecycle items are MEDIUM/LOW and flagged)

Phase names used below are placeholders for the roadmapper (rename freely):
- **P1** Skeleton and transport: stdio handshake, config and secrets, HTTP client core, error mapping
- **P2** Read tools: projects, tree, file, branches, commits, code search
- **P3** Merge Request tools: list, details, diff, create, comment, merge
- **P4** Write tools: create and update files, commits, branches
- **P5** Verification: automated tests, real gitlab.com check via stdio client

## Facts about the client that shape everything (verified in `C:\Projects\AiAdventAgentV2`)

These are not GitLab pitfalls, but they turn many generic pitfalls into hard failures here.

| Client behaviour | Source | Consequence for this server |
|------------------|--------|-----------------------------|
| Connect (spawn + initialize + tools/list) must finish in **10 s** (`MCP_CONNECT_TIMEOUT`) | `shared/config.py` | No network I/O at startup. Fast process start. |
| Each tool call is cut off at **30 s** (`MCP_TOOL_CALL_TIMEOUT`) | `shared/config.py`, `agent/mcp_tools.py` | Total time per tool, including HTTP retries, must stay under about 25 s. |
| Tool result text is cut at **20 000 chars**, keeping the head and dropping the tail | `MCP_TOOL_RESULT_MAX_CHARS` | Do your own smarter truncation before this; put the important data first. |
| Exposed tool name is `mcp__<slug<=20>__<tool>` and must be <= 64 chars, else it is hashed | `agent/mcp_tools.py` | Keep tool names <= 30 chars, `[A-Za-z0-9_-]` only. |
| Description is prefixed with `[MCP server: name]` and cut at 1024 chars | `_describe` | Descriptions <= ~900 chars; key usage hint in the first sentence. |
| Max 128 MCP tools across all servers | `MAX_MCP_TOOLS` | Keep the Mini set small (roughly 20-25 tools). |
| Schema is passed on with only `title` and `$schema` stripped; `$ref`/`$defs`/`anyOf` are NOT resolved | `_mcp_parameters` | Flat schemas of primitive types. See Pitfall 9. |
| Subprocess env = SDK whitelist (`PATH`, `SYSTEMROOT`, `APPDATA`, `USERPROFILE`, `TEMP`, ... on Windows) merged with the per-server `env` from the agent config | `mcp/client/stdio/__init__.py` | `GITLAB_TOKEN` is NOT inherited from your shell. Pitfall 5. |
| Server stderr goes to a temp file; its tail is shown to the user on connect failure | `agent/mcp_client.py` | Anything printed to stderr can surface in the UI and persist in a temp file. Pitfall 6. |
| Shutdown: close stdin, wait 2 s, terminate process tree, wait 2 s, kill | SDK `stdio_client` | Server must exit promptly on stdin EOF. Pitfall 8. |

---

## Critical Pitfalls

### Pitfall 1: Anything except JSON-RPC on stdout

**What goes wrong:**
The client sees garbage, the handshake hangs or fails, and `connect` times out with an unhelpful error. This is the number one failure of stdio MCP servers (the spec says the server MUST NOT write anything to stdout that is not a valid MCP message; real-world bug reports include `print()` in a CLI helper, log sinks defaulting to stdout, and library banners).

**Why it happens:**
`print()` for debugging; `logging.basicConfig()` misconfigured or a logger with a `StreamHandler(sys.stdout)`; a dependency that prints; a Go `fmt.Println`/`log` configured to stdout; tracebacks written by a wrapper script; a `.bat` launcher that `echo`es.

**How to avoid:**
- Route all logging to stderr (or a file) from the first line of `main`. Ban `print`/`fmt.Println` by lint rule or code review checklist.
- Never launch through a `.bat`/`.cmd` wrapper (it can echo). Point the agent at the executable or `python.exe script` directly.
- Add an automated "stdout purity" test in P1: spawn the real server as a subprocess, send `initialize`, `initialized`, `tools/list`, one failing `tools/call`, close stdin, and assert that every line on stdout parses as JSON-RPC and that the process exits with code 0.

**Warning signs:**
`McpError: Connection closed`, `JSONDecodeError`/pydantic validation warnings in the client log at handshake, connect timeout at exactly 10 s, works in MCP Inspector but not in the agent.

**Phase to address:** P1 (test lives in P1 and is re-run in P5)

---

### Pitfall 2: Windows CRLF on stdout (Python SDK) and process-launch quirks

**What goes wrong:**
In the Python `mcp` SDK, `stdio_server()` wraps `sys.stdout.buffer` in `TextIOWrapper(..., encoding="utf-8")` without `newline=""`. On Windows this turns every `\n` into `\r\n`, so every message ends with `CRLF`. Confirmed in the installed 1.30.0 (`mcp/server/stdio.py` line 49) and tracked upstream as python-sdk issue #2433 (fix proposed in PR #2552, status not confirmed merged). The agent's own client tolerates a trailing `\r` because the line is JSON-parsed, but other clients or strict NDJSON parsers may not. Separately, `\r\n` inside JSON string values is fine (escaped), but a raw newline inside a message is forbidden by the spec.

**Why it happens:**
Text-mode file objects on Windows translate newlines. Python's default `newline=None` applies to any wrapper you create over `sys.stdout.buffer`; `sys.stdout.reconfigure(newline="")` does not help because the SDK builds a new wrapper on `.buffer`.

**How to avoid:**
- If Go: not an issue (bytes written as-is); prefer this when language is decided on other grounds too.
- If Python: either (a) pin an SDK version where the fix is merged and verify by test, or (b) supply your own stdin/stdout streams to `server.run()` built with `newline=""`/binary buffers, or (c) accept the CRLF knowingly for the personal client and note it as a known limitation. Decide explicitly in P1; do not discover it in MCP Inspector.
- Put the byte-level check in the stdout purity test: read `proc.stdout` in binary mode and assert no `\r\n`.
- Windows text encoding: Python 3.13 uses the locale code page (cp1251/cp866 on a Russian system) for redirected `stderr`/`open()`; always pass `encoding="utf-8"`, and set `PYTHONUTF8=1` in the server env config so Cyrillic in commit messages, MR titles and file contents is not mangled.

**Warning signs:**
Extra `\r` in a hex dump of stdout; mojibake in Cyrillic MR titles; `UnicodeEncodeError` in stderr logging; Inspector complains about malformed messages.

**Phase to address:** P1

---

### Pitfall 3: Wrong project identifier encoding (`group/project` with a raw slash)

**What goes wrong:**
`GET /projects/group/project/...` returns 404 (routing sees extra path segments). Same for subgroups (`group/sub/project`, must be `group%2Fsub%2Fproject`). Users and the LLM will also pass a numeric ID as a string, a full URL (`https://gitlab.com/g/p/-/merge_requests/3`), `g/p.git`, or an SSH-style path.

**Why it happens:**
Language URL builders default to leaving `/` unescaped (`urllib.parse.quote` defaults `safe="/"`; Go `url.PathEscape` escapes `/` but `url.URL.Path` assignments and `JoinPath` re-decode/re-encode `%2F`). Double encoding (`%252F`) happens when an already-encoded value goes through an encoder again.

**How to avoid:**
- One `normalize_project(ref: str) -> str` function used by every tool: strip whitespace, trailing `.git`, leading host, and anything after `/-/`; if all digits keep as integer id; otherwise percent-encode with `/` -> `%2F` (`quote(x, safe="")` in Python, `url.PathEscape` in Go).
- One request builder that takes already-encoded path segments and never re-encodes them. Verified locally: `httpx` preserves `%2F` in the request path (`/projects/group%2Fsub%2Fproj/...`); in Go build the request from a string URL (or set `RawPath`), do not assign encoded text to `URL.Path`.
- Expose a single tool parameter `project` typed as string ("numeric id or path like group/project"); do not use `integer | string` unions in the schema (Pitfall 9).
- Tests: subgroup project, project with `.` and `-` in name, numeric id, full-URL input.

**Warning signs:**
404 "Project Not Found" for a project that clearly exists; works for `user/repo` but not for group/subgroup projects.

**Phase to address:** P1 (helper + tests), used by P2-P4

---

### Pitfall 4: Repository Files API path and ref handling

**What goes wrong:**
- `file_path` must be fully URL-encoded (`src%2Fmain.py`), including in the DELETE/PUT/POST variants. Spaces, `#`, `?`, `%`, `+`, and non-ASCII (Cyrillic) names break when encoded loosely: `#`/`?` truncate the path, `+` becomes space in query strings.
- `ref` is required for `GET .../repository/files/:path` (missing gives 400). Branch names containing `/` (`feature/x`) are fine as a query parameter but must be encoded (`%2F`) when used in a path (e.g. `GET /repository/branches/:branch`).
- Windows-style input from the agent: `src\main.py`, `.\src\main.py`, `/src/main.py`. A leading `/` encodes to a leading `%2F` and yields 404.
- Creating an existing file (POST) or updating a missing one (PUT) returns 400 with a message like "A file with this name already exists"; the docs note that commit failures return a generic 400 (also for empty commits, directory traversal `..`, concurrent modification of the branch).
- Dots are not encoded by `quote` (`lib/class.rb` example in docs shows `%2E`); servers accept the unencoded dot, do not "fix" this.

**Why it happens:**
People test with `README.md` and `src/app.py` only.

**How to avoid:**
- `normalize_repo_path()`: convert `\` to `/`, strip leading `/` and `./`, reject `..` segments before sending, then `quote(safe="")`.
- Default `ref` to the project's default branch: fetch `default_branch` once per project and cache it (cheap; avoids extra calls).
- Provide explicit `create_file` and `update_file` tools (or one tool with `mode`), and map the "already exists"/"doesn't exist" 400s into a specific message telling the model to use the other tool. Do not silently upsert unless deliberately designed.
- Test file set for P5: name with spaces, `#`, `+`, `%`, Cyrillic, nested dirs, a branch named `feature/a-b`.

**Warning signs:**
404 only for some files; "file not found" for files with special characters; create tool 400s on second run.

**Phase to address:** P2 (read) and P4 (write)

---

### Pitfall 5: Token not reaching the server under the agent (works in shell, fails in agent)

**What goes wrong:**
Manual tests in PowerShell or Inspector inherit the full environment, so `GITLAB_TOKEN` is present. The agent's `stdio_client` only passes a small whitelist of variables plus whatever `env` is set in the agent's per-server config. The server starts, the handshake succeeds (no network at startup), and every tool call returns 401.

Related traps: a `.env` file loaded relative to the process CWD (the agent may set `cwd` to something else, or leave the CWD as the agent's directory); a trailing newline/space in the token copied from the GitLab UI (an `Illegal header value b'glpat-...'` protocol error from the HTTP stack may echo the token into an exception message); the token stored in the agent's database in plain text (client side, out of scope, but be aware).

**How to avoid:**
- Read `GITLAB_TOKEN` (and optional `GITLAB_URL`, default `https://gitlab.com`) from the environment only; `.strip()` it; validate `^[A-Za-z0-9_\-\.]+$`-style sanity without echoing the value.
- On missing token: emit a clear stderr line ("GITLAB_TOKEN is not set; configure it in the MCP server env") and either exit non-zero (agent shows stderr tail) or serve tools that return the same message as `isError`. Pick one and document it; do not crash later inside a request.
- Do not verify the token with a network call at startup (10 s connect limit, see table); lazily validate on first call and turn 401 into an actionable message.
- Test P5 through the agent-like path: run the server via the SDK's `stdio_client` with an `env` dict, not from your interactive shell.

**Warning signs:**
"401 Unauthorized" only when launched from the agent; tools/list works but every call fails.

**Phase to address:** P1 (config), P5 (verify with SDK client)

---

### Pitfall 6: Secret leakage through logs, errors, and stderr

**What goes wrong:**
The PAT ends up in: exception text returned to the LLM (which may echo it into chat history), stderr (persisted in the agent's temp file and shown in the UI on connect failure), debug-level HTTP logs (`PRIVATE-TOKEN` header dumped by `httpx`/`httpcore` debug logging or Go `httputil.DumpRequest`), or in URLs (`?private_token=...` is supported by GitLab but lands in every log and proxy).

**How to avoid:**
- Auth only via the `PRIVATE-TOKEN` header (or `Authorization: Bearer`), never the query string.
- Central `redact()` applied to every string that leaves the process (tool errors, stderr logs): replace the configured token value and anything matching `glpat-[A-Za-z0-9_\-]+`.
- Do not enable debug/trace HTTP logging in shipped code; log method + path + status + duration only.
- Never include request headers, and never `repr()` the client/config object (dataclass repr leaks the token field; mark it `repr=False` or use a secret wrapper).
- Test: set a fake token `glpat-TESTSECRET`, force errors (401, DNS failure, timeout, header-injection newline) and grep stdout, stderr and tool results for the value.

**Warning signs:**
Token substring visible in `stderr_tail`, agent logs, or any tool error text.

**Phase to address:** P1

---

### Pitfall 7: Pagination silently truncating results (the LLM believes the list is complete)

**What goes wrong:**
GitLab defaults to `per_page=20` (max 100). The repository tree, branches, commits, projects, MR lists, MR notes and MR diffs all paginate. A tool that returns page 1 without saying so gives the model a false picture: "the directory has 20 files", "the MR changes 20 files" (`/diffs` defaults to 20 per page).
Also: offset pagination omits `X-Total`/`X-Total-Pages` for collections over 10 000 records; code relying on `X-Total-Pages` breaks. `recursive=true` on the tree of a large repo returns enormous output over many pages.

**How to avoid:**
- Every list tool takes `page` and `per_page` (default modest, e.g. 30-50, cap 100) and returns a footer like `page 1, next_page: 2` derived from the `X-Next-Page` header (works without totals). Never loop unbounded internally; cap at a fixed number of pages/items and say so.
- Tree tool: non-recursive by default with `path` argument; recursive only with a hard item cap and an explicit "truncated: true" marker.
- `list_projects`: always pass `membership=true` (or the caller's intent) plus `search`; without it, `GET /projects` returns every public project on gitlab.com, sorted arbitrarily. Use `simple=true` to shrink payloads.
- MR notes: request `order_by=created_at&sort=asc` explicitly and drop or flag `system: true` notes.

**Warning signs:**
Answers that mention exactly 20 items; MR "diff" missing files; huge first-call latency on `list_projects`.

**Phase to address:** P2 and P3 (shared pagination helper built in P1)

---

### Pitfall 8: Process lifecycle on Windows (orphans, slow start, wrong interpreter)

**What goes wrong:**
- The server keeps running after the agent closes stdin (non-daemon threads, an unfinished HTTP request, a signal handler that ignores EOF); the agent waits 2 s, terminates, then kills.
- Child-process chains on Windows: a venv `python.exe` may act as a launcher that starts the real interpreter as a child; a PyInstaller `--onefile` exe starts a bootloader parent plus a child. Killing the parent can leave the child (MEDIUM confidence; the agent repo already contains `test_*orphan*` DB files, so orphaned processes have been a topic there).
- Cold start above 10 s: onefile extraction under Windows Defender, slow imports, first-run bytecode compile.
- `command: python` resolving to the Microsoft Store alias stub, or a different interpreter than the one with dependencies installed.

**How to avoid:**
- Exit cleanly when stdin reaches EOF; cancel in-flight requests; no non-daemon background threads; test by closing stdin and asserting exit within 1-2 s.
- Recommend a Go single `.exe` if the language decision is otherwise a toss-up (fast start, no interpreter path, no child chain). If Python: reference the venv interpreter by absolute path in the agent config, keep imports lean, consider `python -X importtime` check, and avoid `--onefile` (use onedir).
- Measure cold start (`initialize` + `tools/list`) and keep it well under 3 s.
- After a test run, check Task Manager/`Get-Process` for leftovers.

**Warning signs:**
Multiple `python.exe`/server processes after several agent restarts; connect timeouts only on first run after reboot.

**Phase to address:** P1 (design), P5 (measure)

---

### Pitfall 9: Tool schemas that the client or its LLM provider cannot digest

**What goes wrong:**
The agent forwards `inputSchema` almost verbatim to an OpenAI-style tool API; it strips only `title`/`$schema`. Typical generators produce constructs that get rejected or mishandled: `anyOf: [{type: X}, {type: "null"}]` for `Optional[X] = None`, `$ref`/`$defs` for nested models and Enums, `integer | string` unions, `additionalProperties` quirks, arrays of objects (commit `actions[]`). LLMs also send `"5"` for integers, `null` or `""` for omitted optionals, and numbers for string IDs.

**How to avoid:**
- Flat object schemas with primitive properties (`string`, `integer`, `boolean`) and small `enum` lists inline; avoid `Optional[X] = None` (use a concrete default such as `""`/`0`/`false` and treat it as unset) if using Python FastMCP; inspect the generated `tools/list` output in a test and assert no `$ref`, `$defs`, `anyOf`, `oneOf`, `null`.
- Lenient input coercion in the server: accept `"5"` for integers, treat `""`/`null` as missing, normalize project as in Pitfall 3.
- For multi-file commits, prefer several simple parameters or a JSON-encoded string parameter over `array<object>`; or split into `create_file`/`update_file`/`delete_file` (no nested arrays).
- Write descriptions that state units, defaults, and limits; every tool description <= ~900 chars, names <= 30 chars snake_case.
- Golden test: dump `tools/list` and compare with a snapshot; run the agent's `_mcp_parameters` logic (or a copy) over it.

**Warning signs:**
LLM provider 400 on the tools array; model never calls a tool or calls it with wrong argument types; tool shows up hashed (`-1a2b3c4d` suffix) because the name was too long.

**Phase to address:** P1 (conventions), enforced in every later phase; snapshot test in P5

---

### Pitfall 10: Diffs and file contents too large for the pipeline; empty diff mistaken for "no changes"

**What goes wrong:**
- GitLab.com applies diff limits (patch 200 KB, 3 000 files, 100 000 changed lines per the GitLab.com settings docs). When limits apply, MR diff files carry `collapsed` (fetchable on request), `too_large` (cannot be retrieved; both added in 18.4) or the older `overflow` flag, and the `diff` string can be empty. A server that concatenates `diff` fields reports an empty patch as "no changes".
- `/merge_requests/:iid/changes` is the legacy shape; use `/diffs` (paginated) instead. Compare API: `compare_timeout: true` means `diffs` may be incomplete while `commits` is complete.
- Newly created MRs populate `diff_refs`/`changes_count` asynchronously; diffs fetched right after creation can be empty.
- The client hard-cuts results at 20 000 chars keeping the head, so a 300 KB file or big diff arrives as one file's partial content with no per-file structure.

**How to avoid:**
- Format the diff tool output per file with a header (`path`, `new_file/deleted_file/renamed_file`, flags) and a per-file body cap; when `collapsed`/`too_large`/`overflow` set, say so explicitly ("diff omitted by GitLab: too_large") and how to fetch (raw file at ref).
- Global output budget (e.g. 15 000 chars) smaller than the client's 20 000, with a footer stating what was cut and how to page (`page`, `path` filter).
- File read tool: `start_line`/`end_line` (or `offset`/`limit`), byte-size guard before decoding, and a message with total size/lines when truncated. Use the metadata (`size`) from `HEAD`/GET before pulling multi-MB bodies. Blobs over 10 MB have a 5 req/min limit on the file endpoint.
- Retry once after a short delay if a just-created MR returns empty diffs, or document it in the tool description.

**Warning signs:**
Tool says "0 changed files" for an MR the UI shows changes for; model repeatedly re-requests the same content; the `…[truncated N chars]` marker from the agent appears in results.

**Phase to address:** P3 (diff), P2 (file read)

---

### Pitfall 11: Base64, binary files, and line endings in read and write

**What goes wrong:**
- GET file returns `content` base64-encoded (`encoding: "base64"`); forgetting to decode, or decoding binary as UTF-8 (crash or garbage into the LLM context). UTF-8 BOM and CP1251/UTF-16 text also appear.
- On write, `content` is text unless `encoding: "base64"` is passed; sending Windows CRLF content for an LF file (or vice versa) rewrites every line, producing whole-file diffs and destroying blame. LLMs typically emit `\n`, so updating a CRLF file with the LLM's full-file text converts endings silently.
- Very large writes: the docs state requests over 20 MB are rate limited (3 per 30 s) and over 300 MB rejected; content sent as query string hits URL length limits (414).
- Python `open()` default text mode on Windows translates newlines and uses cp1251 for files you read locally in tests.

**How to avoid:**
- Decode base64 to bytes, try strict UTF-8 (`utf-8-sig` if BOM), otherwise return "binary file, N bytes, not shown" rather than mojibake; also detect NUL bytes.
- Send bodies as JSON (`json=`), never in the query string.
- For updates, detect the file's dominant line ending when reading and re-apply it (or offer `preserve_line_endings` default true); or document that the tool writes content exactly as given and read back with the read tool. Keep a test with a CRLF fixture.
- Impose a content size cap on write tools (for example 1 MB) with an explicit error; this is a sanity limit, not the read-only/confirmation restriction ruled out in PROJECT.md.
- Use `last_commit_id` (returned by GET file) on update/delete for optimistic concurrency, or at least surface the 400 conflict clearly.

**Warning signs:**
Diffs where every line changed; `UnicodeDecodeError` on images or `.exe`; `?content=...` 414 errors.

**Phase to address:** P2 (decode), P4 (encode, line endings, limits)

---

### Pitfall 12: Merge Request semantics (iid vs id, async mergeability, merge failures)

**What goes wrong:**
- Endpoints take the project-scoped **`iid`** (the `!42` number), not the global `id`. Mixing them yields 404 or the wrong MR.
- Mergeability is computed asynchronously. `merge_status` is deprecated (15.6); use `detailed_merge_status` (e.g. `checking`, `preparing`, `mergeable`, `ci_must_pass`, `draft_status`, `not_approved`, `discussions_not_resolved`, `conflict`). Merging right after creation often fails while the status is still `checking`. `with_merge_status_recheck=true` requests recalculation but is not guaranteed.
- Merge failure codes: 405 (cannot merge: draft, failed pipeline requirement, unresolved discussions, not approved, etc.), 406/422 ("Branch cannot be merged", e.g. conflicts), 409 (`sha` does not match HEAD), 401 (no permission), 400 (`SHA must be provided when merging`, when the group/instance "require SHA" setting is on; introduced in GitLab 19.2). Each needs an actionable message; raw "405 Method Not Allowed" is useless to the model.
- Create MR: 409 when an open MR already exists for the source branch; `target_project_id` needed for forks; draft via title prefix `Draft:` (the `wip` param/field is deprecated, `draft` replaced it in 19.0). Setting `should_remove_source_branch`/`squash` is destructive; default them to false.
- Comments: `POST /merge_requests/:iid/notes` with body in JSON; the notes list is newest-first and includes system notes by default (see Pitfall 7).
- Do not retry POST/PUT on timeout: duplicates comments and MRs (Pitfall 13).

**How to avoid:**
- The merge tool first GETs the MR (with recheck) and reports `detailed_merge_status`; if `checking`, wait briefly (2-3 short polls, total under ~10 s) and then decide. Offer an optional `sha` parameter and pass it through; pass `auto_merge` (not the deprecated `merge_when_pipeline_succeeds`) if exposed.
- Return the MR `web_url` and `iid` in every MR tool result so the model uses the right identifier.
- Prefer non-deprecated fields (`detailed_merge_status`, `draft`, `merge_user`, `references`) in output projections; do not rely on removed/deprecated fields.

**Warning signs:**
404 on MR operations with a value copied from `id`; "Method Not Allowed" from merge with no explanation; duplicate MR/comment after a timeout.

**Phase to address:** P3

---

### Pitfall 13: Rate limits, 429 handling, and unsafe retries

**What goes wrong:**
- GitLab.com currently limits authenticated API traffic to 2 000 requests/minute per user; `429` responses carry `Retry-After`, and all responses have `RateLimit-*` headers. A proposed tier-aware scheme (Free: 100/min burst and 5 000/hour sustained) is documented as *proposed, not in effect*, with brownout announcements to come, so a Free-plan personal account should plan for lower limits later. Repository Files API additionally has per-IP-and-path limits (500/min), a 5/min limit for blobs over 10 MB, and 3 per 30 s for writes over 20 MB.
- An LLM agent loops: one tree call, then a file call per file, plus per-MR lookups. This is the realistic way to hit limits, not human use.
- Naive retry loops sleep 60 s (exceeding the client's 30 s call timeout, so the agent reports a timeout and the server keeps sleeping), retry non-idempotent writes, or hammer the API without backoff.

**How to avoid:**
- One HTTP client with explicit timeouts (connect ~5 s, read ~20 s; the httpx default of 5 s is too short for some endpoints, no timeout is worse) and a total deadline per tool call of ~25 s.
- Retry only idempotent GETs, at most 1-2 times, honoring `Retry-After` only if it fits the remaining budget; otherwise return an error containing "rate limited, retry in N s".
- Never auto-retry POST/PUT/DELETE (create MR, comment, commit, merge).
- Expose `RateLimit-Remaining` in stderr logs (no secrets), not in tool output.
- Keep tools coarse enough to avoid N+1 (e.g. include the default branch in project list results; return commit stats only when asked).

**Warning signs:**
Bursts of 429 in the log during agent tasks; agent-side "timed out after 30s" while the server is sleeping in a retry.

**Phase to address:** P1 (HTTP core), P3/P4 (no-retry rule verified)

---

### Pitfall 14: PAT scopes and error-mapping (`api` vs `read_api`, 401/403/404 confusion)

**What goes wrong:**
- `read_api` allows reading (including repository files and MRs) but not writes; a read-only PAT makes every write tool fail with 403 `insufficient_scope`. `write_repository` **does not support API authentication** (Git-over-HTTP only), so it cannot power the Files/Commits/Branches/MR REST calls; writes need `api`. `read_repository` covers file reading (Git-over-HTTP or Repository Files API) but not MR listing.
- Expired PAT: default maximum lifetime is 365 days (up to 400 with the admin flag); tokens expire at midnight UTC on the expiry date, so the server suddenly starts returning 401.
- 404 is returned for both nonexistent resources and resources the token cannot see (private project); 403 for authenticated-but-forbidden (for example a Reporter role attempting to push, or push to a protected branch); 401 for invalid/expired/revoked token. Bodies vary: `{"message": "401 Unauthorized"}`, `{"message": {"base": [...]}}` (dict or list), `{"error": "insufficient_scope", "error_description": "...", "scope": "api"}`.
- Pushing with the Files/Commits API to a protected default branch fails (400/403 "You are not allowed to push into this branch"), which is the normal state of `main` on most repos.

**How to avoid:**
- Document required scopes in the README and in tool descriptions (reads: `read_api`; writes: `api`).
- Single `map_gitlab_error(status, body)` that handles all three body shapes and returns messages of the form "401: token invalid/expired/revoked", "403: token lacks scope `api`, or role/branch protection forbids this", "404: project or resource not found, or token cannot see it (check path and token access)", "429: rate limited, retry in N s", "5xx: GitLab server error, try again later", plus network failure/timeout/DNS classification. Never raise raw exceptions out of a tool handler; return `isError: true` results (the client treats `isError` correctly and formats text).
- Test each mapping with recorded fixtures plus one real 403 (use a read_api token against a write tool) and one real 404.

**Warning signs:**
Model says "project doesn't exist" for a private project; write tool 403 with no explanation; failures appear exactly on a token's expiry date.

**Phase to address:** P1 (mapper), P4 (write-specific messages), P5 (real 403/404 checks)

---

### Pitfall 15: Code search assumptions (tier and scope)

**What goes wrong:**
`GET /search?scope=blobs` (global) and `/groups/:id/search?scope=blobs` require Premium/Ultimate with advanced search or exact code search enabled; a Free account gets an error/empty result. Project-scoped `GET /projects/:id/search?scope=blobs` works with basic search on every tier (verified in the current API docs). Basic search is substring-oriented, returns `data` (a snippet), `path`, `startline`, `ref` (default branch unless `ref` given), and supports `filename:`/`path:`/`extension:` filters. Advanced search caps at 10 000 results (page x per_page).

**How to avoid:**
- Implement only project-scoped code search. Require the `project` argument; if the user wants a cross-project search, say it is not supported in Mini.
- Return `path:startline` plus snippet so the model can follow up with the file tool; pass `ref` when provided.
- Verify on the actual account in P5 (tier and behavior may differ from docs).

**Warning signs:**
400/403/empty results from a group-level search that works on the docs' example; search results reflecting only the default branch.

**Phase to address:** P2

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Return raw GitLab JSON to the LLM | Zero mapping code | 5-20x more tokens (MR objects have ~70 fields), hits the 20 000-char cut, leaks noise like avatar URLs | Never; project to a small field set per tool |
| One giant `gitlab_request(method, path, params)` tool | Tiny tool count | LLM must know API internals and path encoding; unsafe writes | Never for this project (Core Value is a clear, compact toolset) |
| Hand-rolled JSON-RPC over stdio instead of SDK | Avoids SDK quirks (CRLF) | Protocol drift, missing capabilities, more bugs | Only if Go without an official SDK is chosen and the surface stays tiny; verify Go SDK maturity in stack research |
| Mocked-HTTP tests only | Fast, offline | Encoding, pagination, limits and scope bugs pass unseen | As unit layer only; P5 real-gitlab.com run is mandatory |
| Global mutable "current project" state | Shorter tool calls | Stale/incorrect target for write tools | Never for write tools |
| Skip default-branch lookup (hardcode `main`) | One fewer call | Wrong on `master` repos, 404/400 on `ref` | Never; cache `default_branch` per project |
| Unbounded auto-pagination inside a tool | Complete-looking data | Timeouts (30 s), rate limits, huge output | Never; cap pages and return a next-page hint |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| GitLab REST | `/projects/group/sub/proj` with raw slashes; double-encoding | Encode once, `%2F`, keep encoded until the socket (Pitfall 3) |
| GitLab REST | Assuming `per_page` is unlimited or default is large | Default 20, max 100; use `X-Next-Page` |
| GitLab REST | Passing `null` for booleans | Only send `true`/`false` (docs: null treated as false, but do not send null) |
| GitLab REST | Encoding ISO 8601 dates with `+` | `+` must be `%2B` in query strings (for `since`/`until`/`created_after` filters); use `Z` timestamps |
| GitLab REST | `follow_redirects` off, or on blindly | Renamed/moved projects return redirects (`Location`); handle explicitly and only follow same-host redirects, because a custom `PRIVATE-TOKEN` header may be forwarded on cross-host redirects by some clients (MEDIUM confidence; httpx strips only `Authorization`) |
| GitLab REST | Using deprecated fields/endpoints (`merge_status`, `/changes`, `wip`, `merged_by`, `merge_when_pipeline_succeeds`) | Use `detailed_merge_status`, `/diffs`, `draft`, `merge_user`, `auto_merge` |
| GitLab Commits API | Multi-file change as several Files API calls | One commit via `POST /repository/commits` with `actions[]` for atomicity (start_branch to create the branch in the same call). Never expose `force=true` (it rewrites the branch history) |
| GitLab Branches API | Creating a branch by committing with unknown `branch` and no `start_branch` | Use `POST /repository/branches?branch=..&ref=..` or pass `start_branch` |
| MCP SDK (Python) | Mixing sync `requests` inside async handlers (blocks the event loop, stalls other calls and the shutdown) | `httpx.AsyncClient` (or run blocking code in a thread executor) |
| MCP client (agent) | Returning huge structured content or non-text content | Text content only; the client flattens text and drops other types |
| MCP protocol | Raising protocol-level errors for expected failures | Return `isError: true` with a text message so the LLM can react |
| MCP protocol | Using tool annotations/features the client ignores | Encode read-only/destructive hints in the description text as well |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Recursive tree of a big repo | Multi-page fetch, timeout, huge output | Non-recursive by default, item cap, `path` filter | Repos with >1 000 files |
| N+1 requests (fetch each file/commit/MR detail in a loop) | Slow calls, 429 | Coarse tools, return `stats` only when requested, single `/diffs` page per call | An agent task touching >30 files |
| Fetching whole file to check existence | Wasted bandwidth, big-blob limit (5/min over 10 MB) | `HEAD` on the files endpoint (returns `X-Gitlab-*` metadata) | Large files |
| Cold-start heavy imports/onefile extraction | Connect timeout (10 s) | Lazy imports, no startup network calls, measure cold start | After reboot or AV scan |
| Blocking HTTP in async loop | All calls stall during one slow request; shutdown hangs | Async client with timeouts | First concurrent tool calls |
| Search results paging by offset | Repeated identical pages when repo changes | Small page sizes, stop on empty page | Large result sets |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Token in logs, exceptions, or query string (Pitfall 6) | Token leaks into chat history, UI, temp files | Header-only auth, central redaction, no debug HTTP logging |
| Over-scoped PAT (`api` with Owner rights everywhere) since writes are unrestricted by design | An LLM mistake can push/merge/delete on any project the user owns | Per the decision in PROJECT.md there is no read-only mode or confirmations; compensate outside the server: dedicated PAT with short expiry, and for tests a sandbox project/group. Document this in README |
| Exposing destructive endpoints by accident (`force=true` commits, branch/project delete, protected-branch changes) | Irrecoverable history rewrite | Keep them out of the tool set; Mini scope already excludes them; review the tool list against this in P4 |
| Following user-supplied URLs (a "project" argument that is a URL to another host) and sending the token there | Token exfiltration to arbitrary host (prompt injection from repo content) | Pin the API base URL to config (`https://gitlab.com/api/v4`); parse only the path part of user-supplied URLs; never use user-supplied hosts |
| Prompt injection via repository content, MR descriptions, comments | Model is steered to write/merge | Tools return content as data; the merge/write tools' descriptions should say "only when the user asked"; no auto-chaining inside the server |
| Path traversal in `file_path` (`../`) | Unexpected file targets (GitLab rejects, but as opaque 400) | Validate and reject `..` and absolute paths up front |
| Commit author spoofing parameters (`author_name`/`author_email`) exposed to LLM | Misleading history | Do not expose them; let GitLab use the token's user |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Vague tool descriptions ("gets file") | Model guesses parameters, wrong project/ref | State parameter formats and examples in one or two sentences; name defaults ("ref defaults to the project's default branch") |
| Error text like "Request failed" | Model retries blindly | Include status, cause, and next step (Pitfall 14) |
| Success messages without identifiers | Model cannot follow up | Return `web_url`, `iid`, commit `id`/`short_id`, branch name in every write result |
| Output as raw JSON dumps for lists | Costly, hard to scan | Compact text lines or trimmed JSON with stable keys |
| Too many overlapping tools (5 ways to read a file) | Model picks poorly; 128-tool cap | One tool per concept, about 20-25 total |
| Hidden truncation | Model concludes with incomplete data | Always say "truncated", how much, and how to continue |

## "Looks Done But Isn't" Checklist

- [ ] **Handshake:** works from an interactive shell but not tested through the SDK `stdio_client` with a minimal `env` dict — verify `GITLAB_TOKEN` passed via `env`.
- [ ] **stdout purity:** no non-JSON bytes and no `\r\n` on stdout during a full session including error cases — verify with the binary-mode subprocess test.
- [ ] **Shutdown:** process exits within 2 s of stdin close and leaves no child processes — verify with process list after test.
- [ ] **Project identifiers:** subgroup path, numeric id, full URL, `.git` suffix — verify each against real gitlab.com.
- [ ] **File paths:** spaces, `#`, `+`, `%`, Cyrillic, nested, leading `/`, backslashes — verify read and write round-trip.
- [ ] **Pagination:** every list tool returns a next-page hint — verify on a repo with >100 entries.
- [ ] **Large output:** diff/file/tree over 20 000 chars returns a structured, self-limited result with a truncation note — verify with a real large file/MR.
- [ ] **Diff flags:** `collapsed`/`too_large`/`overflow` are surfaced, not turned into an empty diff.
- [ ] **Errors:** 401, 403, 404, 405/406/409/422 (merge), 429, 5xx, DNS failure, timeout all produce an `isError` result and the server keeps serving — verify each with real or recorded cases.
- [ ] **Write safety basics:** existing-file create and missing-file update give clear messages; no auto-retry on writes; no `force`.
- [ ] **Schemas:** `tools/list` contains no `$ref`/`anyOf`/`null`; tool names <= 30 chars; descriptions <= 900 chars — verify with the snapshot test.
- [ ] **Secrets:** fake token never appears in any output stream or tool result under induced errors.
- [ ] **Real run:** the "done" definition in PROJECT.md (automated tests plus manual run via stdio client on real gitlab.com, including at least one write and one MR flow on a sandbox project) is actually performed, including cleanup of test branches/MRs.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Stdout pollution found late | LOW | Grep for prints; route logging to stderr; add the purity test |
| CRLF on Windows with Python SDK | MEDIUM | Upgrade SDK if fixed, custom stream wrapper, or move to Go |
| Wrong path encoding baked into many tools | MEDIUM | Introduce the central builder and replace ad hoc f-strings; add encoding tests; grep for `f"/projects/` |
| Leaked token | HIGH | Revoke PAT in GitLab immediately, create a new one, purge logs/temp files/agent DB, add redaction |
| Schemas rejected by provider | LOW-MEDIUM | Flatten schemas, regenerate snapshot, coerce input server-side |
| Duplicate MRs/comments from retries | LOW | Delete duplicates via UI; remove write retries |
| Bad commit pushed by an LLM | MEDIUM | Revert commit via new MR/`git revert`; branch protection on the important branches (GitLab side) |
| Token expired mid-use | LOW | Create new PAT; improve 401 message to mention expiry |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1 stdout pollution | P1 | Binary stdout purity test over a full session |
| 2 Windows CRLF/encoding | P1 | No `\r\n` in stdout; Cyrillic round-trip test |
| 3 project id encoding | P1 (helper), P2-P4 (use) | Real calls on subgroup/numeric/URL inputs |
| 4 file path / ref handling | P2, P4 | Real files with special names; create-vs-update error messages |
| 5 token not reaching server | P1, P5 | Run through SDK `stdio_client` with env dict |
| 6 secret leakage | P1 | Fake-token redaction test across all outputs |
| 7 pagination truncation | P1 (helper), P2, P3 | Repo/MR with >100 items shows next-page hints |
| 8 process lifecycle | P1, P5 | Exit <= 2 s after stdin close; no orphans; cold start < 3 s |
| 9 schema compatibility | P1 conventions, P5 snapshot | `tools/list` snapshot test; agent-side `_mcp_parameters` run |
| 10 large diffs/files | P2, P3 | Large real MR and file; flags surfaced; output < budget |
| 11 base64/binary/line endings | P2, P4 | CRLF fixture, binary file read, non-UTF-8 read |
| 12 MR semantics | P3 | Create MR, comment, merge (sandbox); merge-refused paths produce clear text |
| 13 rate limits/retries | P1 (client), P3/P4 | Simulated 429 with `Retry-After`; no retries on POST/PUT |
| 14 scopes/error mapping | P1, P4, P5 | Real 401/403/404 cases with wrong-scope tokens |
| 15 code search tier | P2, P5 | Project-scoped blobs search on the real account |

## Phase-Specific Research Flags

| Phase | Needs deeper research? | Why |
|-------|------------------------|-----|
| P1 | Yes, small | Language choice interacts with Pitfalls 2 and 8 (Go SDK maturity vs Python SDK CRLF); decide before coding |
| P2 | No | Standard REST patterns; only search tier needs a live check |
| P3 | Light | Exact `detailed_merge_status` values and merge preconditions for a Free-tier project without pipelines/approvals |
| P4 | Light | Behavior of Files vs Commits API on protected default branch; whether sandbox project allows direct push |
| P5 | No | Test design follows from the checklist above |

## Sources

- GitLab REST API basics (namespaced paths, pagination, headers): https://docs.gitlab.com/api/rest/ (HIGH)
- Repository Files API (encoding, base64, size and rate limits, 400 causes): https://docs.gitlab.com/api/repository_files/ (HIGH)
- Merge Requests API (iid, `detailed_merge_status`, merge status codes, diff endpoints, deprecations): https://docs.gitlab.com/api/merge_requests/ and raw source `doc/api/merge_requests.md` in gitlab-org/gitlab master (HIGH)
- Commits API (`actions[]`, `start_branch`, `force`, `last_commit_id`): raw source `doc/api/commits.md` (HIGH)
- Repositories API (tree pagination, compare truncation): https://docs.gitlab.com/api/repositories/ (HIGH)
- Search API (blobs scope tier requirements, project-level basic search): raw source `doc/api/search.md` (HIGH for text; live tier behavior on the account is unverified, MEDIUM)
- Token scopes (`api`, `read_api`, `read_repository`, `write_repository` without API auth): raw source `doc/security/tokens/access_token_scopes.md`; PAT expiry: https://docs.gitlab.com/user/profile/personal_access_tokens/ (HIGH)
- GitLab.com rate limits (current 2 000/min authenticated API; proposed tier limits not in effect; Files API per-path limit): raw source `doc/user/gitlab_com/rate_limits.md` (HIGH for current numbers; the proposed values are explicitly not enforced yet)
- GitLab.com diff limits (200 KB patch, 3 000 files, 100 000 lines): https://docs.gitlab.com/user/gitlab_com/ (MEDIUM, summarized fetch)
- MCP stdio transport rules (stdout purity, newline-delimited, stderr for logs): https://modelcontextprotocol.io/specification/2025-06-18/basic/transports (HIGH)
- Python SDK CRLF on Windows: https://github.com/modelcontextprotocol/python-sdk/issues/2433 and local check of `C:\Python\Python313\Lib\site-packages\mcp\server\stdio.py` line 49 (HIGH that 1.30.0 lacks `newline=""`; fix status upstream unverified)
- Python SDK client env whitelist and shutdown sequence: https://github.com/modelcontextprotocol/python-sdk/blob/main/src/mcp/client/stdio.py and local `mcp/client/stdio/__init__.py` (HIGH)
- Client constraints (timeouts, 20 000 char cut, name/description/schema handling, stderr capture): `C:\Projects\AiAdventAgentV2\shared\config.py`, `agent\mcp_tools.py`, `agent\mcp_client.py` (HIGH, read directly)
- stdout-corruption bug pattern in real projects (esp-idf #19087, whatsapp-mcp #43, mcpsignals #22): found via web search (MEDIUM, illustrative)
- Local check: `httpx` preserves `%2F` in request paths; `urllib.parse.quote` needs `safe=""` (HIGH, executed)
- Unverified/LOW: Windows venv launcher and PyInstaller onefile child-process orphaning; `h11` echoing the token in `Illegal header value` errors; `httpx` forwarding custom headers on cross-host redirects

---
*Pitfalls research for: stdio MCP server on GitLab REST API v4 (gitlab.com)*
*Researched: 2026-09-24*
