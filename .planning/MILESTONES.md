# Milestones

## v1.0 MVP (Shipped: 2026-09-25)

**Phases completed:** 4 phases, 20 plans, 26 tasks

**Delivered:** compact Go stdio MCP server (`gitlab-mcp.exe`, 20 tools) verified on real gitlab.com and documented for AiAdventAgentV2; UAT against the running agent passed (04-HUMAN-UAT.md).

**Stats:** ~11.5k lines of Go, ~1.2k of Python (smoke/live clients); 2026-09-24 → 2026-09-26.

**Known deferred items at close:** 2 (see STATE.md Deferred Items); backlog 999.3–999.5 remains open.

**Key accomplishments:**

- Go stdio MCP server (go-sdk v1.8.0 + client-go/v2 v2.64.0) with a `whoami` tool, fail-fast config, token-redacting logger, stdout guard, and a real-binary test harness including a Python mcp 1.30.0 smoke client.
- Read-only retries (GET/HEAD, max 2, Retry-After aware), writes never repeated, every GitLab failure class mapped to a short Russian isError text, and single-encoding project/file paths locked by exact-wire tests.
- Reusable in-memory MCP session helper, an automatic schema/name/description guard for every tool, panic and redaction tests for `safe`, and a raw-byte stdout purity test on the real binary.
- Two read-only project tools plus a shared rune-based output budget and page footer, verified through the in-memory MCP session against a fake GitLab.
- `list_repository_tree` browses a repository at any path/ref with explicit default-branch resolution, pagination and a 15 000-rune output budget.
- `get_file_contents` reads any text file whole, by inclusive line range, or head-truncated with an explicit footer, returns a marker for binary files, and all five tools are now proven through the real binary and the Python mcp 1.30.0 client.
- list_branches and create_branch over client-go v2 with write-aware Russian error wording (real HTTP status, "Результат записи неизвестен" on ambiguous failures), request-body capture in the fake GitLab, and a 7-tool real-binary plus Python mcp 1.30.0 smoke.
- list_commits and get_commit over the real binary, with a diff renderer that names the reason for every missing patch (too_large, collapsed, rename, mode, empty/binary) and keeps every file heading inside the 15 000-rune budget.
- compare_refs compares two branches, tags or commits: header "from → to", up to 20 commit lines, files paged on the server side and rendered by the shared diff renderer, with explicit compare_timeout, same-ref and out-of-range-page messages.
- commit_files makes one commit with several create/update/delete/move actions through a hand-written nested actions[] schema, sends the POST exactly once, and reports exact per-file +/- from a follow-up diff read.
- create_or_update_file reads the file on the target branch to choose create or update, sends last_commit_id with updates so concurrent edits are rejected, and commits through the same single POST path as commit_files without ever retrying.
- list_merge_requests (opened by default, filters) and get_merge_request (one request, explained detailed_merge_status) with a shared status-advice table, pinned at 14 tools through the real binary and Python mcp 1.30.0
- get_merge_request_diffs (typed /diffs, same renderer and budget as get_commit, explicit reason for every empty patch, overflow line) and list_merge_request_notes (newest first, [system] one-liners, user bodies cut at 1000 runes), pinned at 16 tools through the real binary and Python mcp 1.30.0
- create_merge_request (compare pre-check, Draft: title prefix, one POST, 409 reported as "открытый MR уже есть: !N") plus a status-aware write error layer with op-specific "outcome unknown" hints, pinned at 17 tools through the real binary and Python mcp 1.30.0 with loopback-only smoke writes
- create_merge_request_note (MR pre-read for the link, one unretried POST, body sent unchanged) and update_merge_request (only non-empty fields, close/reopen, Draft: prefix without blanking), pinned at 19 tools through the real binary and Python mcp 1.30.0
- merge_merge_request with a GET pre-check that reuses the shared status table, one unretried PUT without sha, op-keyed Russian explanations for every documented merge refusal, a stateful create-to-merged flow test, and 20 tools pinned through the real binary and Python mcp 1.30.0
- commit_files (create/update) and create_or_update_file now refuse missing or empty `content` before any request to GitLab, with the schema kept free of null/anyOf and the rule documented in descriptions.
- Opt-in `scripts/live.py` drives the real `gitlab-mcp.exe` over mcp==1.30.0 stdio through read, branch, commit, MR, note, merge and real error probes on gitlab.com, cleans up its own objects via REST, and writes a redacted LIVE-RUN.md; proven hermetically only (no real run, no tokens).
- Full read -> branch -> commit -> MR -> note -> merge -> cleanup scenario plus real error cases ran green against AlekseyYakovlev/sanbox with a freshly rebuilt, dependency-free 11.4 MB exe; transcript committed as LIVE-RUN.md.

---
