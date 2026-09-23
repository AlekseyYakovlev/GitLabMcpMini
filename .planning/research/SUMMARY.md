# Project Research Summary

**Project:** GitLabMcpMini
**Domain:** Compact stdio MCP server wrapping the GitLab REST API v4 (gitlab.com, PAT auth), used by a Python `mcp` 1.30.0 client (AiAdventAgentV2) on Windows 11
**Researched:** 2026-09-24
**Confidence:** MEDIUM-HIGH

## Executive Summary

This is a small, agent-facing API-wrapper MCP server: about 18-22 typed tools over GitLab REST v4, covering code reading, Merge Requests and repository writes. It is launched as a stdio subprocess with a PAT in `GITLAB_TOKEN`. Experts build these as thin layers: SDK-owned transport and protocol, a tool layer of thin handlers (input shaping, one or two client calls, formatting, error mapping), and an MCP-free REST client that owns auth, path encoding, pagination and retry. The product's value is not breadth. Competing servers range from 9 to 261 tools, and the research consistently says the small, understandable toolset with compact, bounded, LLM-shaped output is the point.

The recommendation is **Go, one `gitlab-mcp.exe`, official `modelcontextprotocol/go-sdk` v1.8.0**. A working spike verified this against the real Python `mcp` 1.30.0 client on this machine: the handshake negotiates 2025-11-25, schemas are valid, `isError` mapping works, and env passing works. Go removes two whole pitfall classes (Python SDK CRLF on Windows; interpreter/venv path fragility and slow imports) and matches how the agent already launches `filesystem.exe`. Build order follows the dependency chain: prove the stdio handshake and stdout purity first, then the GitLab client core, then read tools, then write and MR tools, then live hardening.

The main risks are client-imposed or GitLab-semantic, and all are preventable by design: stdout pollution; the agent's 20,000-char result cut and 30 s tool timeout; silent pagination/truncation; project and file-path URL encoding; MR semantics (`iid`, async `detailed_merge_status`, merge error codes); token scope (`api` needed for writes); never auto-retrying writes. Mitigation is a small set of shared helpers (project normalizer, request builder, error mapper, pagination footer, output budget) built in the first phases and reused by every tool. A real gitlab.com end-to-end run on a scratch project is the definition of done.

## Key Findings

### Recommended Stack

Go 1.25+ (`go 1.25` in `go.mod`; local toolchain 1.27.1), single static binary via `CGO_ENABLED=0` (~12 MB), pinned dependencies. Python is a legitimate second choice but loses on launch fragility (venv path), startup time (~0.4 s import vs <30 ms handshake), pywin32 plus ~20 transitive packages, and the `mcp` 2.x ecosystem split.

**Core technologies:**
- **Go + `github.com/modelcontextprotocol/go-sdk` v1.8.0** — MCP server (`mcp.NewServer`, generic `mcp.AddTool`, `StdioTransport`). Official, spike-verified against the 1.30.0 client, speaks 2025-11-25 and 2026-07-28.
- **`gitlab.com/gitlab-org/api/client-go/v2` v2.64.0 OR hand-rolled `net/http` client** — GitLab access; see Gaps (recommendation: hand-rolled).
- **stdlib (`net/http`, `log/slog`, `httptest`, `os`)** — env config, stderr-only logging, fake GitLab for tests. No cobra/viper/godotenv.
- **`mcp.NewInMemoryTransports()`** — in-process protocol tests.
- **Python `mcp==1.30.0` (client only, in `scripts/`)** — smoke client mirroring exactly what the agent does.

**Do not use:** `xanzy/go-gitlab` (deprecated); Python `mcp` 2.x for a server; pointer fields for optional args (schema becomes `["null","integer"]`); `fmt.Print*` to stdout; the `write_repository` scope (does not work for REST — writes need `api`); `go test -race` on this box (needs cgo/gcc).

### Expected Features

About 18 tools at v1, all mapping to PROJECT.md Active requirements.

**Must have (table stakes):**
- Foundation: project resolver (numeric id, `group/sub/proj` or URL), authenticated client, error mapping to `isError`, pagination helper with next-page hint, output truncation helper
- Read: `list_projects` (default `membership=true`), `get_project`, `list_repository_tree`, `get_file_contents`, `list_branches`, `list_commits`, `get_commit` (with diff), `search_code` (project-scoped only)
- MR: `list_merge_requests`, `get_merge_request`, `get_merge_request_diffs`, `list_merge_request_notes`, `create_merge_request`, `create_merge_request_note`, `merge_merge_request`
- Write: `create_branch`, `commit_files` (multi-file, `actions[]`, the primitive), `create_or_update_file` (thin wrapper)

**Should have (v1.x):** `update_merge_request`, `compare_refs`, `whoami` (`GET /user`, cheap token/scope check), tool annotations, URL-as-input, merge `sha` guard, compact field-whitelisted responses.

**Defer (v2+):** inline diff comments (needs `diff_refs`), `approve_merge_request`, `delete_branch`, structured output, two-step per-file MR diff tools.

**Never build:** issues/work items, pipelines/CI logs, wiki/releases/groups, repo create/fork, OAuth/HTTP transports, read-only mode (author decision), a generic `gitlab_request` tool, force-push or other destructive endpoints, global/group code search.

### Architecture Approach

Four strictly downward layers: SDK transport/protocol → tool layer (`internal/tools`) → MCP-free GitLab client (`internal/gitlab`) → stdlib HTTP. Config and stderr-only logging are cross-cutting. `main.go` is a composition root only; `server.New(client, logger)` returns the server without running it so tests can attach any transport; `GITLAB_URL` is the seam for hermetic subprocess tests against a fake GitLab. No caching, no state beyond config and the shared `http.Client`.

**Major components:**
1. **`internal/gitlab` client** — `PRIVATE-TOKEN` auth, single encoded-path request builder, pagination parsing (`X-Next-Page`), GET-only bounded retry, typed `*APIError`. Knows nothing about MCP.
2. **`internal/tools`** — one file per domain group, thin handlers, `toToolError`, `format.go` for truncation and compact rendering.
3. **`internal/server` + `cmd/gitlab-mcp/main.go`** — wiring, fail-fast config, stdin-EOF shutdown.
4. **Test harness** — `httptest` fake GitLab with request recorder, in-memory MCP sessions, real-binary stdio e2e with stdout-purity test, opt-in `live` smoke against gitlab.com.

**Key patterns:** two error channels (all GitLab/network failures → `isError: true` with an actionable sentence, process never crashes); pagination surfaced, not swallowed; output bounded and LLM-shaped; one project-reference normalizer; never retry POST/PUT/DELETE; mock the HTTP boundary, not the client interface.

### Critical Pitfalls

1. **Anything but JSON-RPC on stdout.** stderr-only logging from line one, no `.bat` launcher; binary-mode stdout-purity test over a full session (including error paths) in Phase 1, re-run at the end.
2. **Client limits.** The agent enforces a 10 s connect timeout, 30 s per-tool-call timeout, 20,000-char result cut (keeps head), tool-name cap of 64 chars including `mcp__<slug>__`, description cut at 1024 chars. Response: no network at startup, ~25 s per-call deadline, own output budget ~15,000 chars with important data first and an explicit "truncated, how to continue" footer; tool names ≤30 chars, descriptions ≤900 chars.
3. **Silent pagination/truncation and diff flags.** Every list tool takes `page`/`per_page` (default 20-50, cap 100) and prints `next_page`; recursive tree capped; MR diffs rendered per file and surface `collapsed`/`too_large`/`overflow` rather than an empty patch reading as "no changes".
4. **Path/ID encoding.** One `normalize_project` (numeric, path, URL, `.git`) and one repo-path normalizer (backslashes, leading `/`, reject `..`); assert the wire path in tests (Go's `URL.Path` re-decodes `%2F` — use `RawPath` or a string URL); test subgroups, spaces, `#`, `+`, `%`, Cyrillic, `feature/x` branches.
5. **Token and scope handling.** Token reaches the server only via the agent's `env` — test through SDK `stdio_client` with an env dict, not the shell; central `glpat-…` redaction; fail fast on missing token; writes need `api` scope; map 401/403/404 (404 also means "cannot see it") and all three error body shapes; never auto-retry writes; MR semantics: `iid` not `id`, `detailed_merge_status` polling, readable 405/406/409 messages, `squash`/`remove_source_branch` default false.

## Implications for Roadmap

Recommended 6 phases (1-2 can be merged at coarse granularity).

### Phase 1: Walking Skeleton and Stdio Harness
**Rationale:** Core Value is a correct MCP handshake over stdio against the agent-pinned client; highest protocol risk, cheapest to fix now; every later phase reuses the harness.
**Delivers:** `go.mod`; `config.Load(getenv)` (fail fast on missing token); stderr `slog` with token redaction; `server.New`; one no-network tool; `internal/testutil` (fake GitLab, in-memory session); subprocess stdio e2e + stdout-purity test (exit on stdin EOF within 2 s); Python 1.30.0 smoke script via `stdio_client` with `env`.
**Avoids:** Pitfalls 1, 5 (env path), 6 (redaction), 8 (lifecycle, cold start <3 s), 9 (schema/naming conventions: flat primitives, no `$ref`/`anyOf`/`null`).

### Phase 2: GitLab Client Core
**Rationale:** Every tool depends on it; encoding and error bugs here would be baked into every tool.
**Delivers:** `internal/gitlab` (`do()`, `PRIVATE-TOKEN`, single encoded-path builder, project and repo-path normalizers, `*APIError` for all body shapes, pagination parsing, GET-only bounded retry honoring `Retry-After` within 25 s, timeouts); `tools/errors.go` (`toToolError` with actionable 401/403/404/405/406/409/422/429/5xx/network messages); `tools/format.go` (output budget, truncation markers, next-page footer); full `httptest` coverage asserting wire paths.
**Avoids:** Pitfalls 3, 6, 7, 13, 14. Resolve hand-rolled vs `client-go` v2 via time-boxed spike and record the decision.

### Phase 3: Read Tools
**Rationale:** Sets output-shaping conventions later tools copy; read tools are the live verification fixture for writes.
**Delivers:** `list_projects`, `get_project`, `list_repository_tree`, `get_file_contents`, `list_branches`, `list_commits`, `get_commit`, `search_code` (project-scoped), `whoami`; `readOnlyHint` annotations.
**Avoids:** Pitfalls 4 (read side), 7, 10 (`start_line`/`end_line`, size guard), 11 (base64 decode, binary/NUL detection, BOM), 15 (project-scoped search only).

### Phase 4: Write Tools
**Rationale:** Built before MR create/merge because an MR needs a pushed branch with commits ahead of target; verified using the read tools.
**Delivers:** `create_branch`; `commit_files` (`POST /repository/commits` with `actions[]` and `start_branch`, never `force`); `create_or_update_file` wrapper; explicit "already exists / does not exist" messages; ~1 MB content cap; line-ending preservation; no retries on writes; `last_commit_id` surfaced.
**Avoids:** Pitfalls 4 (write side), 11, 13, 14 (protected default branch, scope messages). Nested `actions[]` schema needs a client-compatibility check.

### Phase 5: Merge Request Tools
**Rationale:** Read side first (list, get, diffs, notes), then mutating side (create, comment, merge); depends on client core and on Phase 4 for live tests.
**Delivers:** `list_merge_requests`, `get_merge_request`, `get_merge_request_diffs` (paginated `/diffs`, per-file, flags surfaced), `list_merge_request_notes` (system notes flagged), `create_merge_request`, `create_merge_request_note`, `merge_merge_request` (pre-GET `detailed_merge_status`, 2-3 short polls <10 s, optional `sha`, readable 405/406/409 and 400 "SHA must be provided").
**Avoids:** Pitfalls 7, 10 (diff flags, empty diffs right after creation), 12 (`iid` vs `id`, destructive options default false), 13.

### Phase 6: Hardening, Live Verification and Delivery
**Rationale:** Definition of done is automated tests plus a manual run through a stdio client on real gitlab.com; mocks alone hide encoding, scope, tier and drift bugs.
**Delivers:** `live`-tagged suite on a scratch project (tree/file read, branch, commit, MR, comment, merge, cleanup); real 401/403/404/429 checks (read-only token vs write tool); `tools/list` snapshot test; output caps tuned on real large files/MRs; special-name file round-trip; README with agent `command`/`env` snippet and required scopes; cold-start and orphan-process measurement; v1.x items as time allows.
**Avoids:** Pitfalls 5, 8, 9 (snapshot), 14, 15 (real tier check).

### Phase Ordering Rationale
- Critical path 1 → 2 → 3. Phases 4 and 5 can swap or parallelize after 3, but 4 before 5's create/merge eases live testing.
- Shared helpers land in Phases 1-2, so no tool phase re-solves Pitfalls 3, 6, 7, 13, 14.
- Protocol risk first, GitLab semantic risk second, breadth third, real-service validation last; harness exists from Phase 1.
- Keep the tool list at ~18-22; no issue/CI/generic-request tools for symmetry.

### Research Flags
Needs deeper research during planning:
- **Phase 2:** client-go v2 vs hand-rolled (time-boxed spike); confirm rate-limit numbers and `Retry-After` against current docs (sources disagree).
- **Phase 5 (light):** confirm `GET /merge_requests/:iid/diffs` as the paginated endpoint and its `collapsed`/`too_large` fields; exact `detailed_merge_status` values; merge preconditions on a Free-tier scratch project.
- **Phase 4 (light):** Files vs Commits API on a protected default branch; `last_commit_id`, `execute_filemode`; whether the scratch project allows direct push.
- **Phase 3 (light, live check):** project-scoped `scope=blobs` search on the author's Free-tier account.

Standard patterns (skip research-phase): Phase 1 (spike-verified), Phase 6.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH (language, SDK) / MEDIUM (GitLab client lib) | Go + go-sdk verified by first-hand spike against the real 1.30.0 client; `client-go` v2 only compile-verified |
| Features | MEDIUM-HIGH | Official GitLab MCP tool list and archived reference server fetched; community lists from READMEs; several REST details unverified |
| Architecture | MEDIUM-HIGH | MCP spec, Go SDK v1.8.0 and GitLab docs verified; a few SDK API details (stdout redirect, `IOTransport`) LOW |
| Pitfalls | MEDIUM-HIGH | GitLab/MCP facts from current official docs; client constraints read directly from the agent's source; Windows process-lifecycle items MEDIUM/LOW |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address
- **GitLab client choice.** STACK recommends `client-go` v2 with hand-rolled fallback; ARCHITECTURE designs a hand-rolled `internal/gitlab` (~300 lines). Synthesis recommends hand-rolled: ~18 endpoints only, learning goal, exact control of wire-path encoding / GET-only retry / 25 s budget / redaction, smaller dependency graph. Settle in a time-boxed spike at the start of Phase 2.
- **Rate-limit numbers disagree** (5,000/h + 100/min vs 2,000/min authenticated vs a 2026-10-19 change for Free/unauthenticated). Design does not depend on the number; re-check docs before README claims.
- **Tool name prefix.** ARCHITECTURE suggests `gitlab_`; the agent already namespaces as `mcp__<slug>__<tool>` (64-char cap), so the prefix is redundant. Recommend short unprefixed names unless a collision with `filesystem.exe` appears. Decide in Phase 1.
- **Nested `actions[]` schema** for `commit_files`: verify generated schema in the snapshot test and against the agent's parameter handling; fall back to a JSON-string parameter or per-action tools.
- **Unverified GitLab/SDK details:** `/diffs` vs `/changes`; single-file API verbs and `last_commit_id`; whether the agent's 1.30.x client forwards tool annotations/structured output; `HTTPS_PROXY` passthrough. Handle in the phases noted under Research Flags.
- **Missing-token behavior:** fail fast with stderr message (recommended) vs per-call `isError`. Pick in Phase 1 and test.
- **Go stdout safety:** redirecting `os.Stdout` after the transport captures it is LOW confidence; rely on lint-ban plus purity test.
- **User-side prerequisites:** a scratch gitlab.com project and a minimal-scope PAT (`api`) for the live tier.

## Sources

### Primary (HIGH confidence)
- Go module proxy and PyPI JSON API: go-sdk v1.8.0, client-go v2.64.0, `mcp` 1.30.0/2.2.0
- Context7 `/modelcontextprotocol/go-sdk` and pkg.go.dev v1.8.0
- MCP spec 2025-11-25 (transports, tools, error handling)
- GitLab REST docs (rest, repository_files, commits, merge_requests, repositories, search, token scopes, rate limits)
- Official GitLab MCP server tool reference; archived `modelcontextprotocol/servers` gitlab
- Local spike (Go server + Python `mcp` 1.30.0 client, Windows 11) and AiAdventAgentV2 source (`shared/config.py`, `agent/mcp_tools.py`, `agent/mcp_client.py`)

### Secondary (MEDIUM confidence)
- go-sdk / python-sdk release notes; zereight/gitlab-mcp README; kopfrechner/gitlab-mr-mcp; GitLab.com rate-limit blog and diff-limit docs; Python SDK CRLF issue #2433

### Tertiary (LOW confidence, needs validation)
- Windows venv launcher / PyInstaller onefile orphaning; `h11` echoing tokens in header errors; `httpx` cross-host header forwarding; `HTTPS_PROXY` passthrough; Go stdout-redirect trick

---
*Research completed: 2026-09-24*
*Ready for roadmap: yes*
