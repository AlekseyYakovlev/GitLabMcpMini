# Walking Skeleton — GitLabMcpMini

**Phase:** 1
**Generated:** 2026-09-24

## Capability Proven End-to-End

The real `gitlab-mcp.exe` is spawned over stdio by a Python `mcp` 1.30.0 client (the same SDK and launch path as AiAdventAgentV2), completes `initialize` and `tools/list` without any network I/O, and answers a `whoami` tool call with one real GitLab REST call (`GET /api/v4/user`) made through `client-go/v2`, with the token never appearing in stdout, stderr or tool text.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language / runtime | Go (`go 1.25.0` in go.mod, local toolchain 1.27.1), `CGO_ENABLED=0`, one `gitlab-mcp.exe` | D-12; agent must start it without Node/npx; single static binary |
| MCP framework | `github.com/modelcontextprotocol/go-sdk` v1.8.0, generic `mcp.AddTool` with struct-inferred flat schemas | D-12; spike-verified against Python `mcp` 1.30.0 (protocol 2025-11-25) |
| Transport | `mcp.IOTransport{Reader: os.Stdin, Writer: realStdout}` with `os.Stdout = os.Stderr` guard | FND-02: stray `fmt.Print*` from any dependency lands on stderr; exit 0 on stdin EOF |
| GitLab client ("data layer") | `gitlab.com/gitlab-org/api/client-go/v2` v2.64.0, built once in `internal/glclient` with a GET/HEAD-only retry policy, custom Retry-After backoff, no-op limiter, http timeouts, 8 MiB body cap | D-01, D-02, D-03; there is no database — GitLab REST v4 is the only data source |
| Auth | Personal Access Token from `GITLAB_TOKEN` (trimmed), sent only as `PRIVATE-TOKEN` header by client-go; fail fast with exit 1 when missing | D-13, FND-03; single user |
| Base URL | `GITLAB_URL` optional, default `https://gitlab.com`; only `https://` or `http://` to loopback accepted | D-13 test seam; prevents sending the token to arbitrary hosts |
| Tool handler contract | Every handler is `func(ctx, in T) (string, error)` wrapped by `tools.safe[T]`: recover, 25 s deadline, error to `isError` text via `toToolText`, redaction as the last step | FND-05, FND-07, D-04, D-15; go-sdk v1.8.0 does not recover panics |
| Output shape | Compact text, one line per item, one-line footer for pagination/truncation, 15 000-rune budget | D-06, D-07, D-08 |
| Logging | `log/slog` text handler on stderr, level from optional `LOG_LEVEL` (default warn), redacting `ReplaceAttr` | D-04, D-12 |
| Tool naming | No prefix (`whoami`, `list_projects`, ...), ≤30 chars, descriptions ≤900, flat schemas, plain (non-pointer) optional fields with `omitempty` | D-05, D-15, FND-08 |
| Directory layout | `cmd/gitlab-mcp` (composition root), `internal/{config,logging,glclient,tools,server,testutil}`, `e2e/` (real-binary tests), `scripts/smoke.py` (PEP 723) | Claude's discretion per CONTEXT; `glclient` owns every client-go-facing policy reused by Phases 2-4 |
| Test harness | Tier 1 `httptest` fake GitLab (`internal/testutil.FakeGitLab`, asserts raw `RequestURI`); Tier 2 in-memory MCP session (`mcp.NewInMemoryTransports`); Tier 3 real binary via `mcp.CommandTransport` + raw-pipe stdout purity; Tier 4 Python `mcp==1.30.0` smoke via `uv run` | QA-01, QA-02; no `-race` (no cgo on this box) |
| "Deployment" | Local build: `go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp`; local full-stack run: `uv run scripts/smoke.py --exe gitlab-mcp.exe` (live with a real `GITLAB_TOKEN`) or `--base-url <fake>` | Agent integration and README are Phase 4 (QA-04) |

## Stack Touched in Phase 1

- [ ] Project scaffold (go.mod with pinned go-sdk v1.8.0 and client-go/v2 v2.64.0, `go build`, `go vet`, `gofmt`, `go test`) — Plan 01
- [ ] Protocol entry — `initialize` + `tools/list` over stdio with zero GitLab requests — Plan 01
- [ ] Data read — one real GitLab REST read (`GET /api/v4/user` via `Users.CurrentUser`) — Plan 01; project/tree/file reads — Plans 04-06
- [ ] Client interaction — the Python `mcp` 1.30.0 client calls `whoami` and gets text back — Plan 01
- [ ] Local full-stack run — `uv run scripts/smoke.py` against the built binary — Plan 01 (extended in Plan 06)
- [ ] (No write path in Phase 1: repository writes are Phase 2 by roadmap; the write-safe retry policy is still installed in Plan 02 so Phase 2 inherits it)

## Out of Scope (Deferred to Later Slices)

- Branches, commits, compare, repository writes (`create_branch`, `commit_files`, `create_or_update_file`) — Phase 2
- Merge Requests (list/get/diffs/notes/create/update/merge) — Phase 3
- Live gitlab.com acceptance run, README with agent config snippet — Phase 4
- Code search, URL-as-project input, structured output, JSON-number project IDs — v2 / not planned
- Default-branch cache (zero state chosen for Phase 1; one extra `GET /projects/:id` when `ref` is empty)

## Subsequent Slice Plan

Each later phase adds tools on top of this skeleton without altering its decisions (same `glclient.New`, `tools.safe`, `toToolText`, `format.go` budget/footers, `FakeGitLab`, in-memory session helper, e2e binary harness, smoke script):

- Phase 2: agent explores history (branches, commits, compare) and writes to the repository (branch, multi-file commit, single file)
- Phase 3: agent drives a Merge Request from review to merge
- Phase 4: author runs the full live scenario on gitlab.com and gets `gitlab-mcp.exe` plus README ready for AiAdventAgentV2
