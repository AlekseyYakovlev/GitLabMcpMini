<!-- GSD:project-start source:PROJECT.md -->
## Project

**GitLabMcpMini**

Учебный MCP-сервер (stdio) с компактным набором основных инструментов для работы с репозиториями на gitlab.com: чтение кода, Merge Requests и запись в репозиторий. Сервер предназначен для личного использования автором и подключается к его собственному агенту (AiAdventAgentV2, Python, `mcp` SDK 1.30.x), который запускает MCP-серверы как stdio-подпроцессы.

**Core Value:** Небольшой, понятный и работающий набор GitLab-инструментов, которые корректно отрабатывают на реальном gitlab.com через стандартный MCP-протокол (stdio: initialize → tools/list → tools/call).

### Constraints

- **Transport**: stdio — агент подключает MCP-серверы только как подпроцессы со stdio
- **Runtime**: результат должен запускаться без Node/npx — нативный бинарник или Python-скрипт, запускаемый агентом
- **Auth**: Personal Access Token через переменную окружения — единственный пользователь, простейший рабочий вариант
- **Target**: только gitlab.com — заявленная цель, self-hosted не проверяется
- **Scope**: «Mini» — компактный набор инструментов, без расширения на остальные области GitLab
- **Compatibility**: совместимость с MCP SDK 1.30.x на стороне клиента (протокол, формат inputSchema)
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Verdict: Go
### Empirical verification (spike run 2026-09-24, HIGH confidence)
- `initialize` succeeds and negotiates `protocolVersion = 2025-11-25`.
- `tools/list` returns a valid `inputSchema` (`type: object`, `properties`, `required`, `additionalProperties: false`, `description` taken from the `jsonschema` struct tag).
- `tools/call` works; env vars passed via `StdioServerParameters(env=...)` reach the server (this is how `GITLAB_TOKEN` will arrive).
- A Go `error` return becomes `isError: true` with the message as text. It is not a protocol error, so the server does not crash.
- Unknown or invalid arguments are rejected with `isError: true` and a readable message.
- Binary size: 12 MB for the MCP-only spike. A `client-go` v2 spike also compiled (12.7 MB, 9.1 MB with `-s -w`).
## Recommended Stack
### Core Technologies
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.25+ in `go.mod` (`go 1.25`); local toolchain is 1.27.1 | Language / toolchain | `go-sdk` v1.8.0 requires `go 1.25.0`. Cross-compiles to a single `.exe` with `CGO_ENABLED=0`. Go is already installed locally. |
| `github.com/modelcontextprotocol/go-sdk` | **v1.8.0** (2026-09-04) | MCP server: `mcp.NewServer`, `mcp.AddTool`, `mcp.StdioTransport` | Official SDK, co-maintained with Google, and it has a compatibility promise. Supports spec 2025-11-25 and 2026-07-28. Generic `AddTool` infers the input schema from a struct and validates arguments. Verified against the agent's client (see the spike above). |
| `gitlab.com/gitlab-org/api/client-go/v2` | **v2.64.0** (2026-09-05) | Typed GitLab REST v4 client | Official successor of `xanzy/go-gitlab`, which is deprecated (last release v0.115.0, 2024-12). Covers every endpoint in scope (projects, repository tree/files/commits/branches, search, MRs, notes, diffs) with typed options. Builds a `Response` with pagination fields and handles rate-limit retries through go-retryablehttp. Compile-verified: `NewClient(token)` and `Projects.ListProjects(opts)` work in v2.64.0. |
| Go standard library: `net/http`, `log/slog`, `encoding/json`, `context`, `os` | (bundled) | Config from env, logging to stderr, HTTP timeouts | No config or logging framework is needed for a one-token, no-CLI-flags server. |
### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `testing` + `net/http/httptest` (stdlib) | bundled | Fake GitLab server for unit and integration tests | Always. Point client-go at `httptest.Server` with `gitlab.WithBaseURL(...)`. |
| `mcp.NewInMemoryTransports()` (in go-sdk v1.8.0, verified to exist) | bundled with SDK | In-process client to server tests over the real MCP protocol without spawning a process | Automated tests for `tools/list` and `tools/call`, including error mapping. |
| `github.com/google/jsonschema-go` | v0.4.3 (transitive, pulled in by go-sdk) | Schema inference from struct tags | Used implicitly. Use `jsonschema:"..."` tags for tool argument descriptions. Do not add it as a direct dependency. |
| Python `mcp` (client only, test tooling) | **==1.30.0** (same as the agent's `requirements.txt`) | Manual/E2E smoke client: `stdio_client` to `initialize` to `list_tools` to `call_tool` against real gitlab.com | For the "manual check through a stdio client" acceptance criterion. It mirrors exactly what the agent does. Keep it in a `scripts/` dir with its own venv, or run `uv run --with mcp==1.30.0 script.py`. It is not part of the server. |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| `go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe .` | Release build | `CGO_ENABLED=0` (the default here, since no gcc is installed). Point the agent's `command` at the resulting absolute path. |
| `go vet`, `gofmt` | Static checks | Built in. Add `golangci-lint` only if you want extra lint. It is not needed for a project this size. |
| `go test ./...` | Automated tests | Do **not** rely on `go test -race` on this machine. The race detector needs cgo and a C compiler on Windows. |
| MCP Inspector (`npx @modelcontextprotocol/inspector`) | Optional interactive poking | Requires Node/npx. That is acceptable as a dev-only tool (the "no Node" rule concerns the agent runtime), but the Python smoke script is the primary check because it uses the same SDK as the agent. |
## Installation
# Init (Go 1.25+)
# Core
# Build
# Smoke client (dev-only, separate venv)
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Go + go-sdk | **Python + `mcp` 1.30.x FastMCP** (`mcp.server.fastmcp`, decorators; python-gitlab 8.5.0 or httpx 0.28.1) | Fastest to prototype (roughly 150 lines) and the same language and SDK as the agent. Choose it if you never want a build step and are happy to hard-code a venv `python.exe` path in the agent's config. If chosen: pin `mcp>=1.30,<2`; python-gitlab is sync (`requests`), so wrap calls in `asyncio.to_thread` or use httpx directly; never `print()` to stdout. |
| `client-go/v2` | Hand-rolled `net/http` client (about 200 lines: `PRIVATE-TOKEN` header, JSON in/out, `Link`/`X-Next-Page` pagination, error mapping) | If `client-go` v2 friction appears (frequent major releases, large dependency graph with protobuf/CEL/keyring pulled in for features we do not use), or if the goal is to learn the REST API itself. It is a safe fallback and stays stdlib-only. Decide in phase 1 after wiring the first two tools. |
| `client-go/v2` | `client-go` v1.46.0 (`gitlab.com/gitlab-org/api/client-go`, 2026-03-01) | Only if a v2 signature blocks you. v1 is the previous major line. Prefer v2 for new code. |
| go-sdk (official) | `github.com/mark3labs/mcp-go` | Popular community SDK. Not needed now that the official SDK is at v1.x with a stability promise. Do not mix the two. |
| PAT in `GITLAB_TOKEN` | OAuth / GitLab's built-in MCP server (`/api/v4/mcp`) | GitLab's built-in server needs Premium/Ultimate, OAuth and HTTP transport. It is out of scope and not usable over stdio. |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `github.com/xanzy/go-gitlab` | Deprecated and archived. Last tag v0.115.0 (2024-12) exists only to point to the new home. | `gitlab.com/gitlab-org/api/client-go/v2` |
| Python `mcp` 2.x (2.2.0 is "latest" on PyPI) for the server | The agent pins 1.30.0. 2.x is a redesign (`MCPServer` naming, `mcp-types`, `httpx2`). Using 2.x on the server gains nothing and adds an untested pairing. `pip install mcp` without a pin would give you 2.x. | If Python: `mcp>=1.30,<2`. If Go: go-sdk v1.8.0. |
| `fmt.Println` / `log.Println` to stdout in Go | Stdout is the JSON-RPC channel, and any stray byte corrupts the stream and breaks the handshake. The Go `log` package writes to stderr by default, but `fmt.Print*` does not. | `slog` / `log` configured to `os.Stderr` only. The agent captures stderr to a temp file and shows its tail on connect errors. |
| Pointer fields (`*int`, `*string`) for optional tool arguments | The inferred schema becomes `"type": ["null","integer"]` (verified in the spike). LLMs handle this worse than a plain type. | Plain typed fields with `json:"x,omitempty"` (optional in the schema), plus `jsonschema:"description"` tags. |
| Node/npx-based servers (for example zereight/gitlab-mcp, yoda-digital/mcp-gitlab-server) | Node/npx is not used by the agent. They are also sprawling (86 tools, OAuth, HTTP transports), the opposite of "Mini". | Own compact Go server. Read them only for tool-naming ideas. |
| Cobra/viper/godotenv/config frameworks | Over-engineering: the only config is `GITLAB_TOKEN` and an optional `GITLAB_URL` (default `https://gitlab.com`). | `os.Getenv` in `main`, and fail fast to stderr with exit code 1 if the token is missing. |
| Hard-coded write-scope tokens like `write_repository` | `write_repository` only covers git push. The Repository Files/Commits/MR REST endpoints need the full **`api`** scope (`read_api` and `read_repository` are read-only). | Create the PAT with `api`. Document this in the README. A read-only smoke test can use `read_api`. |
| `go test -race` as a CI gate on this box | Needs cgo and gcc, which are not installed on the dev machine. | `go test ./...`, and `go vet`. |
## Stack Patterns by Variant
- `func(ctx, req *mcp.CallToolRequest, in InStruct) (*mcp.CallToolResult, any, error)`. Return `nil, nil, err` for failures.
- Because the SDK turns a returned `error` into `isError: true` with text, map GitLab failures inside the handler to short, actionable messages ("401: token invalid/expired", "403: token lacks `api` scope or no project access", "404: project/file/branch not found", "429: rate limited, retry after N s") instead of leaking raw HTTP bodies. This satisfies the "understandable errors without crashing the server" requirement.
- Set `mcp.ToolAnnotations` (`ReadOnlyHint`, `DestructiveHint`) on tools. It is cheap and helps clients (the agent currently ignores it, but it documents intent).
- Truncate at a documented cap and say so in the result text. LLM context is the real limit, not the SDK.
- Cap and default `per_page` (GitLab max is 100; default 20). Offset pagination headers (`X-Next-Page`, `X-Total`) are omitted for results over 10,000 records, so do not depend on `X-Total`.
- It must be URL-encoded in the path (`group%2Fproject`). client-go does this for string IDs (MEDIUM, confirm in the first tool test). A hand-rolled client must use `url.PathEscape`.
- gitlab.com allows about 2,000 authenticated requests per minute per user, and 429 responses carry `Retry-After`. Limits for Free/unauthenticated traffic tighten on 2026-10-19 (unauthenticated: 60 req/h per IP). A PAT keeps you in the authenticated bucket. Surface `Retry-After` in the error message.
## Version Compatibility
| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| go-sdk v1.8.0 server | Python `mcp` 1.30.0 client | **Verified by spike**: negotiates 2025-11-25; tools/list and tools/call work. |
| go-sdk v1.8.0 | Go >= 1.25.0 | `go.mod` requires `go 1.25.0`. Local Go 1.27.1 is fine. |
| client-go v2.64.0 | Go >= 1.25.0 | Same floor as the SDK, so no conflict. golang.org/x/oauth2 and x/time are shared transitive deps (MVS resolves them). |
| go-sdk v1.8.0 | agent upgrade to `mcp` 2.x (2026-07-28) | Server supports both revisions (v1.7.0+). Not tested here, taken from release notes (MEDIUM). |
| Python `mcp` 1.30.0 | `pywin32` on Windows | Only matters for the Python alternative or the smoke client. Already installed locally (312). |
| client-go major cadence | roughly every 6 months | Pin the exact version in `go.mod`. Read the migration guide before bumping majors. |
## Sources
- Go module proxy (`proxy.golang.org`) — go-sdk latest v1.8.0 (2026-09-04), `go.mod` requires go 1.25.0; client-go v1.46.0 (2026-03-01) and `client-go/v2` v2.64.0 (2026-09-05); xanzy/go-gitlab last v0.115.0. HIGH.
- PyPI JSON API — `mcp` 2.2.0 latest, 1.30.0 latest 1.x, dependency lists (pywin32 on Windows); python-gitlab 8.5.0 (2026-07-28, requests-based); httpx 0.28.1. HIGH.
- Context7 `/modelcontextprotocol/go-sdk` — `mcp.AddTool` behaviour (schema inference, argument validation, error to `IsError`), `StdioTransport`, quick start. HIGH.
- https://github.com/modelcontextprotocol/go-sdk/releases — v1.7.0 (protocol 2026-07-28, backward compatible with 2025-11-25), v1.8.0 hardening. MEDIUM (summarised by a fetch tool; dates cross-checked against the module proxy).
- https://github.com/modelcontextprotocol/python-sdk/releases — v2.x redesign, `FastMCP` to `MCPServer`, 1.x maintenance/security-only. MEDIUM.
- https://docs.gitlab.com/security/tokens/access_token_scopes/ and https://docs.gitlab.com/api/repository_files/ — PAT scopes, `api` needed for file create/update. HIGH.
- https://docs.gitlab.com/api/rest/ — pagination (per_page max 100, headers omitted over 10,000 records), URL-encoded project paths. HIGH.
- https://about.gitlab.com/blog/rate-limit-change-2026/ and https://docs.gitlab.com/user/gitlab_com/rate_limits/ — 60 req/h unauthenticated, 429/Retry-After, change dated 2026-10-19 (Free/unauthenticated). MEDIUM.
- https://docs.gitlab.com/user/model_context_protocol/mcp_server/ — GitLab's built-in MCP server (HTTP/OAuth, Premium/Ultimate). MEDIUM.
- Local spike (scratch, not in the repo) — Go server + Python `mcp` 1.30.0 client on Windows 11; import-time and startup measurements; client-go v2 compile and binary size. HIGH (first-hand).
- `C:\Projects\AiAdventAgentV2\requirements.txt`, `agent\mcp_client.py`, `agent\mcp_config.py`, `tests\fixtures\mcp_stdio_server.py` — how the client launches servers (command/args/env/cwd, stderr to temp file, connect timeout). HIGH.
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
