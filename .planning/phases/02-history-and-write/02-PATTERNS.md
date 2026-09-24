# Phase 2: history-and-write - Pattern Map

**Mapped:** 2026-09-24
**Files analyzed:** 17 (11 new/modified production and test-support files, 6 new test files)
**Analogs found:** 17 / 17 (all in-repo, Phase 1 code; no external analog needed)

All Phase 1 analogs live in `internal/tools/`, `internal/testutil/`, `scripts/`, `e2e/`. Module path is `gitlab-mcp` (imports: `gitlab-mcp/internal/glclient`, `gitlab "gitlab.com/gitlab-org/api/client-go/v2"`).

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/tools/history.go` (list_branches, list_commits, parseTime, line renderers) | tool handler | request-response, paginated list | `internal/tools/tree.go` + `projects.go` (`listProjects`) | exact |
| `internal/tools/commit.go` (get_commit, compare_refs) | tool handler | request-response, header + bounded body | `internal/tools/projects.go` (`getProject`) + `file.go` (header + Budget body) | role-match |
| `internal/tools/diff.go` (renderDiffFiles, emptyPatchReason) | utility (formatter) | transform | `internal/tools/format.go` (`Budget`, `composeList`) + `file.go` truncation footer | role-match |
| `internal/tools/branch.go` (create_branch) | tool handler | request-response, POST | `internal/tools/tree.go` (resolveRef + client call) | role-match (first write tool) |
| `internal/tools/write.go` (commitCore, commit_files, create_or_update_file, write limits) | tool handler | request-response, POST + prior GET | `internal/tools/file.go` (GetFile + base64 decode + isBinary) | role-match (first write tool) |
| `internal/tools/schema.go` (`commitFilesSchema()` explicit InputSchema) | config | transform | none for code; `schema_test.go` defines the rules it must satisfy | no analog (see below) |
| `internal/tools/errors.go` (modify: `withWrite`, write-aware branch of `toToolText`) | utility (error mapper) | transform | itself (`subjectError`/`withSubject`/`toToolText`) | exact (extend) |
| `internal/tools/register.go` (modify: +7 `AddTool`) | config | registration | itself | exact |
| `internal/testutil/fakegitlab.go` (modify: body capture, `Recorded()`) | test utility | request recording | itself | exact (extend) |
| `internal/tools/history_test.go`, `commit_test.go`, `diff_test.go`, `branch_test.go`, `write_test.go` | test | request-response over fake | `internal/tools/tree_test.go` | exact |
| `internal/tools/errors_test.go` (modify: write cases) | test | table-driven | itself (`TestToToolText`) | exact |
| `internal/tools/schema_test.go` (modify: golden test, `-update` flag; `TestToolSchemas` auto-covers new tools) | test | golden snapshot | itself (`TestToolSchemas`) | role-match |
| `internal/tools/testdata/tools_list.golden.json` | fixture | snapshot | none | no analog |
| `scripts/smoke.py` (modify: EXPECTED_TOOLS 5 -> 12, new calls, nested `actions`) | test tool (E2E client) | request-response | itself | exact (extend) |
| `e2e/python_smoke_test.go` (modify: fake routes for new endpoints) | test | E2E | itself | exact (extend) |
| `e2e/stdio_test.go` (check only: fixed tool-list assertions, per RESEARCH Wave 0) | test | E2E | itself | n/a (verify) |

## Pattern Assignments

### `internal/tools/history.go` (list_branches, list_commits) - tool handler, paginated list

**Analog:** `internal/tools/tree.go` (whole file, 98 lines) and `internal/tools/projects.go` lines 27-85.

**Imports pattern** (`tree.go` lines 3-11):
```go
import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"gitlab-mcp/internal/glclient"
)
```

**Description const + input struct pattern** (`tree.go` lines 13-30). Descriptions in Russian, <=900 runes; jsonschema tags in English, must NOT start with `word=`; plain (non-pointer) fields with `omitempty`; `page`/`per_page` copy verbatim:
```go
type TreeIn struct {
	Project   string `json:"project" jsonschema:"numeric project ID as a string (\"12345\") or full path group/subgroup/project"`
	Ref       string `json:"ref,omitempty" jsonschema:"branch, tag or commit SHA; default the project's default branch"`
	Page      int    `json:"page,omitempty" jsonschema:"page number, default 1"`
	PerPage   int    `json:"per_page,omitempty" jsonschema:"items per page, default 20, max 100"`
}
```
(`projects.go` line 36-38 also has `ProjectIn` - reuse/embed style for tools needing just `project`.)

**Core handler pattern** (`tree.go` lines 33-81): normalise -> ClampPaging -> resolveRef -> typed options -> client-go call with `gitlab.WithContext(ctx)` -> `withSubject` on error -> header + lines -> `composeList`:
```go
func listRepositoryTree(d Deps) func(ctx context.Context, in TreeIn) (string, error) {
	return func(ctx context.Context, in TreeIn) (string, error) {
		project, err := glclient.NormalizeProject(in.Project)
		if err != nil {
			return "", err
		}
		page, perPage := ClampPaging(in.Page, in.PerPage)
		ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
		if err != nil {
			return "", err
		}
		opts := &gitlab.ListTreeOptions{
			ListOptions: gitlab.ListOptions{Page: int64(page), PerPage: int64(perPage)},
			Ref:         gitlab.Ptr(ref),
		}
		if path != "" {
			opts.Path = gitlab.Ptr(path)   // optional filters only when non-empty
		}
		nodes, resp, err := d.GL.Repositories.ListTree(project, opts, gitlab.WithContext(ctx))
		if err != nil {
			return "", withSubject("путь или ref в дереве", err)
		}
		header := fmt.Sprintf("%s @ %s, путь: %s", project, refLabel(ref, isDefault), shownPath)
		if len(nodes) == 0 {
			return header + "\n(пусто)", nil
		}
		lines := make([]string, 0, len(nodes)+1)
		lines = append(lines, header)
		for _, n := range nodes { lines = append(lines, treeLine(n)) }
		return composeList(strings.Join(lines, "\n"), page, perPage, resp), nil
	}
}
```
Apply to `list_commits` (`Commits.ListCommits`, `ListCommitsOptions{ListOptions, RefName, Since, Until, Path, Author}`; `list_branches`: `Branches.ListBranches`, `ListBranchesOptions{ListOptions, Search}`, no ref so no `resolveRef`).

**Per-line renderer pattern** (`projects.go` lines 73-85): separate `xxxLine(*gitlab.T) string` func using `strings.Builder`/`fmt.Fprintf`, long free text through `oneLine(text, N)`:
```go
func projectLine(p *gitlab.Project) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %s", p.ID, p.PathWithNamespace)
	if desc := oneLine(p.Description, listProjectLineDescriptionRunes); desc != "" {
		fmt.Fprintf(&sb, " — %s", desc)
	}
	return sb.String()
}
```
Use for `branchLine` (name, `[default]`/`[protected]`/`[merged]`, short SHA, `oneLine(commit title)`; `Commit` may be nil) and `commitLine`. Empty list: return plain text like `"проекты не найдены"` (projects.go line 62). Note per RESEARCH Pitfall 6, `PageFooter` already falls back to `NextLink`; add a Link-only test.

**parseTime**: no in-repo analog; copy from RESEARCH.md "ISO 8601 parsing" (returns Russian error before any request, like the `..` rejection in `tree_test.go` lines 103-117).

---

### `internal/tools/commit.go` (get_commit, compare_refs) - header + bounded diff body

**Analog:** `internal/tools/projects.go` `getProject` (lines 88-128) for header assembly with `strings.Builder` + final `Budget` + `TruncatedFooter`; `internal/tools/file.go` lines 91-106 for a header followed by a budgeted body with a custom truncation footer.

**Header + budget pattern** (`projects.go` lines 108-126):
```go
var sb strings.Builder
fmt.Fprintf(&sb, "id: %d\n", p.ID)
...
out, truncated := Budget(sb.String(), OutputBudget)
if truncated {
	out += "\n" + TruncatedFooter
}
return out, nil
```

**Custom truncation footer pattern** (`file.go` lines 91-105):
```go
header := fmt.Sprintf("%s — строки %d-%d из %d, %d байт", label, from, to, total, f.Size)
body, truncated := Budget(strings.Join(lines, "\n"), OutputBudget)
var sb strings.Builder
sb.WriteString(header); sb.WriteString("\n"); sb.WriteString(body)
if truncated { fmt.Fprintf(&sb, "\n[файл обрезан на 15000 символах: ...]", ...) }
```

**compare_refs pagination** (server-side slice): no in-repo analog. Copy RESEARCH.md Pattern 5 (synthetic `&gitlab.Response{NextPage: ...}` fed to `PageFooter`). Composition order (format.go header comment lines 1-9): body -> `TruncatedFooter` if cut -> page footer, each on its own line. `get_commit` makes two calls (`Commits.GetCommit(pid, sha, nil)` + `Commits.GetCommitDiff(...)` with `ListOptions`), and takes `resp` from the diff call for `PageFooter`. Use `withSubject("коммит", err)` / `withSubject("ref", err)` on 404s.

Do not use `resolveRef` for `get_commit` (`sha` required) or `compare_refs` (`from`/`to` required).

---

### `internal/tools/diff.go` (renderDiffFiles) - formatter, transform

**Analog:** `internal/tools/format.go` `Budget` (lines 31-50), `composeList` (82-93), `oneLine` (97-107). Copy RESEARCH.md Pattern 4 skeleton for structure (headings for all files first, then patches share remaining budget, `patchCap = 2000` runes, switch order in `emptyPatchReason` matters: RenamedFile, then NewFile/DeletedFile, then mode change, then default). `Budget` is rune-based and cuts on a line boundary, so reuse it for the per-file cap; do not slice strings manually. Constants style: named `const` block (see `format.go` lines 19-29).

---

### `internal/tools/branch.go` (create_branch) - POST tool

**Analog:** `internal/tools/tree.go` for `resolveRef` usage; `internal/tools/ref.go` lines 14-39 (`resolveRef`, `refLabel`).

**resolveRef contract** (`ref.go` lines 19-31): explicit ref returns without any request; empty ref costs one `GetProject`; returns `errEmptyRepo` for empty repos:
```go
ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
```
Then `d.GL.Branches.CreateBranch(project, &gitlab.CreateBranchOptions{Branch: gitlab.Ptr(in.Branch), Ref: gitlab.Ptr(ref)}, gitlab.WithContext(ctx))` (RESEARCH Pattern requirement WRT-01). Error: wrap with `withWrite(opCreateBranch, "проект или ветка", err)` (new, see errors.go section), so 400 "already exists" -> "ветка уже существует". No retry wrapper (client-level GET-only retry already covers POST; verified in `glclient/client_test.go` per RESEARCH). Output: name, source ref via `refLabel(ref, isDefault)`, tip SHA, `web_url`.

Annotations: `&mcp.ToolAnnotations{DestructiveHint: &f}` with `f := false` (RESEARCH Pattern 7) - no existing analog for non-read-only tools.

---

### `internal/tools/write.go` (commitCore, commit_files, create_or_update_file)

**Analog:** `internal/tools/file.go` for GET file + base64 decode + binary check (lines 62-80):
```go
f, _, err := d.GL.RepositoryFiles.GetFile(project, path, &gitlab.GetFileOptions{Ref: gitlab.Ptr(ref)}, gitlab.WithContext(ctx))
if err != nil {
	return "", withSubject("файл", err)
}
var raw []byte
if f.Encoding == "base64" {
	raw, err = base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return "", fmt.Errorf("не удалось декодировать содержимое файла: %w", err)
	}
} else {
	raw = []byte(f.Content)
}
if isBinary(raw) { ... }
```
Refactor suggestion for the planner: extract the decode block (file.go lines 67-75) into a small shared helper `decodeFileContent(f *gitlab.File) ([]byte, error)` used by both `getFileContents` and `create_or_update_file`, or duplicate minimally; `isBinary` (file.go 113-138) is reused as is. `f.LastCommitID` from this GET goes into the update action (D-07).

**Create-vs-update decision**: `glclient.Classify(err).Kind == glclient.KindNotFound` (`glclient/errors.go` lines 61-101, `Kind*` consts 24-36) -> create; other error -> return `withSubject("файл", err)`. Do NOT use `withWrite` for the GET.

**Path normalisation for every action path** (`glclient/normalize.go` lines 42-52): `NormalizeRepoPath` returns `ErrDotDot` (Russian text reaches model as is); empty result means root, so for `file_path` reject empty explicitly, as `file.go` lines 53-55 does:
```go
if path == "" {
	return "", errors.New("не указан путь к файлу")
}
```

**Guards (D-04)**: return plain `errors.New`/`fmt.Errorf` with Russian text BEFORE any client call; `toToolText` passes `KindOther` detail through (`errors.go` line 78, test `errors_test.go` "other passes detail"). Validate in the handler (not via schema `enum`) so messages are Russian.

**commitCore**: copy RESEARCH.md Pattern 2 verbatim (shared by `commit_files` and `create_or_update_file`; `Content` pointer only when action needs it so empty content is still sent).

**Input structs**: flat, no pointers, for `create_or_update_file` (copy `FileIn` style, `file.go` lines 34-40). `CommitFilesIn`/`ActionIn` use plain fields with `json` tags but the schema is supplied explicitly (see schema.go); `[]ActionIn` default inference would emit `["null","array"]` and fail `walkSchema`.

**Success output**: header line + one line per file + `web_url`; content of `commit_files` response is `*gitlab.Commit` (`ShortID`, `Stats`, `WebURL`); per RESEARCH Assumption A1, only commit-level `+/-` totals are available, per-file lines show action + path only.

---

### `internal/tools/schema.go` (commitFilesSchema) - explicit InputSchema

**Analog:** none in code. Follow RESEARCH.md Pattern 1 (hand-written `map[string]any`, not `jsonschema.For`, to avoid a direct dependency on `jsonschema-go`; CLAUDE.md). Rules it must pass are encoded in `internal/tools/schema_test.go`:
- `schema["type"] == "object"` (lines 71-73)
- no `$ref`, `$defs`, `anyOf`, `oneOf`, `allOf` anywhere (`forbiddenSchemaKeys`, line 27; `walkSchema` lines 89-111 recurses into nested `items`)
- no array-valued `"type"` (lines 98-102)
- every TOP-LEVEL property has a non-empty `description` (lines 76-82)
- tool description 1..900 runes, name <=30 chars, `^[a-z][a-z0-9_]*$`, no `gitlab_` prefix (lines 16-23, 46-61)

Register with `mcp.Tool{Name: "commit_files", Description: ..., InputSchema: commitFilesSchema(), Annotations: ...}` (`register.go` style). Keep `required` as `[]any{...}` and `additionalProperties: false` at both levels.

---

### `internal/tools/errors.go` (modify) - write-aware mapper

**Analog:** itself, lines 17-79.

**Subject-wrapper pattern to copy for `withWrite`** (lines 17-34):
```go
type subjectError struct {
	subject string
	err     error
}
func (e *subjectError) Error() string { return e.err.Error() }
func (e *subjectError) Unwrap() error { return e.err }
func withSubject(subject string, err error) error {
	if err == nil {
		return nil
	}
	return &subjectError{subject: subject, err: err}
}
```
Add a `write bool` field (or sibling type) set by `withWrite(op, subject, err)`; extract via `errors.As` exactly as `toToolText` does (lines 39-43).

**Where to hook:** in `toToolText` (lines 38-79), after `e := glclient.Classify(err)` and BEFORE the generic `switch e.Kind`, add `if se != nil && se.write { if text, ok := writeText(e, se.op, se.subject); ok { return text } }`. The generic switch keeps existing texts for reads (existing tests in `errors_test.go` lines 27-53 must still pass unchanged). Matching by lowercase substring of `e.Detail` (never equality; Detail renders `{message: ...}` per RESEARCH Pattern 3). Reuse `capDetail(e.Detail)` (lines 84-90) for unmatched 400s. Table of cases and Russian texts: RESEARCH.md Pattern 3 (403 combined wording, 400 "not allowed to push", conflict "changed since you started editing", "you can only create or edit files when you are on a branch", already exists, invalid reference name, 401 + scope api, 5xx/timeout/network on write + "Результат записи неизвестен").

Tests: extend the table-driven `TestToToolText` (`errors_test.go` lines 17-70; fields `prefix`, `contains`, `notContains`, `exact`) with write cases built via `withWrite(opCommit, "проект или ветка", &glclient.Error{Kind: glclient.KindBadRequest, Status: 400, Detail: "{message: You are not allowed to push into this branch}"})`.

---

### `internal/tools/register.go` (modify)

**Analog:** itself, lines 8-38. Copy per tool; read tools keep `ReadOnlyHint: true`:
```go
mcp.AddTool(s, &mcp.Tool{
	Name:        "list_repository_tree",
	Description: listRepositoryTreeDescription,
	Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
}, safe(d, listRepositoryTree(d)))
```
Add `list_branches`, `list_commits`, `get_commit`, `compare_refs` (ReadOnlyHint), `create_branch` (DestructiveHint=&false), `commit_files` (explicit `InputSchema`), `create_or_update_file` (annotations at planner's discretion; leave `DestructiveHint` nil = spec default true). Import `mcp` only (already there). Handler contract: `func(ctx, in T) (string, error)` wrapped by `safe(d, ...)` (`safe.go` lines 33-62) - it recovers panics, applies 25 s timeout, converts errors via `toToolText`, redacts token last. Never write to stdout.

---

### `internal/testutil/fakegitlab.go` (modify) - body capture

**Analog:** itself. Current recorder (lines 43-61):
```go
func (f *FakeGitLab) serve(w http.ResponseWriter, r *http.Request) {
	path := r.RequestURI
	if i := strings.IndexByte(path, '?'); i >= 0 { path = path[:i] }
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.RequestURI)
	h := f.routes[routeKey(r.Method, path)]
	f.mu.Unlock()
	...
	h(w, r)
}
```
Extend: read `r.Body` with `io.ReadAll`, restore via `io.NopCloser(bytes.NewReader(b))`, append to a parallel slice; add `Recorded() []Recorded{Method, URI, Body}`; keep `Requests()` (lines 84-93) and `Reset()` (96-100, must also clear the new slice) unchanged in behaviour. Register-by-exact-raw-path model stays: `fake.JSON(method, rawPath, status, body, headers)` / `fake.Handle(...)` (lines 65-82). New routes needed use raw encoded paths, e.g. `POST /api/v4/projects/g%2Fp/repository/commits`, `.../repository/files/README%2Emd` (dot -> `%2E`), branch names in `:sha` as `feature%2Fx`. Unregistered routes return 404 by default, which hides missing routes: assert on `fake.Requests()`.

---

### New test files (`history_test.go`, `commit_test.go`, `diff_test.go`, `branch_test.go`, `write_test.go`)

**Analog:** `internal/tools/tree_test.go` (whole file) with helpers in `helpers_test.go`.

**Imports** (`tree_test.go` lines 3-11): `fmt`, `net/url`, `strings`, `testing`, `unicode/utf8`, `gitlab-mcp/internal/testutil`.

**Session + call pattern** (`tree_test.go` lines 15-28; helpers `newTestSession` / `callText` at `helpers_test.go` lines 22-65):
```go
fake := testutil.NewFakeGitLab(t)
fake.JSON("GET", "/api/v4/projects/g%2Fp", 200, treeProjectJSON, nil)
fake.JSON("GET", "/api/v4/projects/g%2Fp/repository/tree", 200, `[...]`, nil)
cs := newTestSession(t, fake)
text, isErr := callText(t, cs, "list_repository_tree", map[string]any{"project": "g/p"})
if isErr { t.Fatalf("unexpected tool error: %s", text) }
```
`callText` fails the test if the result contains the token, so token-leak coverage is automatic. Note `treeProjectJSON` const is declared in `tree_test.go` (package-shared); reuse it rather than redeclaring.

**Wire assertions** (lines 30-53, 88-92): assert `fake.Requests()` order/count, parse query with `url.ParseQuery`, and check optional params are absent (`if _, has := vals[k]; has`). For POSTs use the new `Recorded()` body and `json.Unmarshal` into a map (do not string-match key order).

**Error/guard tests** (lines 103-117): assert `isErr`, expected Russian substring, and `len(fake.Requests()) == 0` for pre-request guards (empty actions, >50 actions, >1 MiB, empty message, `..`, bad date). Use `requestsTo(fake, "GET /...")` (defined in another `_test.go` in the package; used at `tree_test.go` line 131) to count calls to an endpoint. Write-retry test: 503 on POST commits -> exactly 1 request and text contains "результат неизвестен".

**Budget test** (lines 167-192): 100 long items, assert `TruncatedFooter`, `utf8.RuneCountInString(text) < 15500`, footer suffix. Mirror for diff budget exhaustion.

---

### `internal/tools/schema_test.go` (modify) - golden snapshot

**Analog:** `TestToolSchemas` (lines 31-85) for the session + `cs.ListTools(ctx, nil)` pattern:
```go
cs := newTestSession(t, testutil.NewFakeGitLab(t))
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
list, err := cs.ListTools(ctx, nil)
```
Add `TestToolsListGolden`: `json.MarshalIndent(list.Tools, "", "  ")`, compare to `testdata/tools_list.golden.json`, `-update` flag (`flag.Bool`) rewrites it. `TestToolSchemas` needs NO change: it iterates every registered tool and walks nested `items`, so new tools (including `commit_files`) are covered automatically. It requires all seven new descriptions to be <=900 runes.

---

### `scripts/smoke.py` (modify)

**Analog:** itself. Changes:
- `EXPECTED_TOOLS` (lines 34-40): add the 7 names (12 total).
- Tool-call helper `call(name, arguments, expect_error=False)` (lines 121-131) - reuse for new calls; write calls include a nested `actions` argument, e.g. `await call("commit_files", {"project": project, "branch": "feature", "commit_message": "m", "actions": [{"action": "create", "file_path": "a.md", "content": "x"}]})`. This closes the STATE.md blocker: real `mcp` 1.30.0 client accepts/forwards the schema and `walk_schema` (lines 63-73) already checks the nested schema for forbidden keys and null types.
- Write calls must only run against the fake (hermetic mode, `--base-url` set); live mode (README "read-only calls only", lines 10-16, 171-174) must keep skipping them (guard with `if base_url:`).
- Token-leak check at the end (lines 150-151) applies to new results automatically since `call` appends to `texts`.

### `e2e/python_smoke_test.go` (modify)

**Analog:** itself, lines 18-29: register new fake routes with `fake.JSON(method, rawPath, status, body, headers)` for `GET .../repository/branches`, `.../repository/commits`, `.../repository/commits/<sha>`, `.../commits/<sha>/diff`, `.../repository/compare`, `POST .../repository/branches`, `POST .../repository/commits`, plus assertions on stdout like lines 44-52 (`strings.Contains(stdout.String(), ...)`). Check `e2e/stdio_test.go` for fixed tool lists/counts (RESEARCH Wave 0 note; not read in this mapping).

---

## Shared Patterns

### Handler shape and input normalisation
**Source:** `internal/tools/tree.go` lines 33-48; `internal/tools/file.go` lines 43-55
**Apply to:** all 7 new handlers
Handler is `func(ctx, in T) (string, error)`; first `glclient.NormalizeProject(in.Project)` (returns Russian errors for URLs/empty), then `glclient.NormalizeRepoPath` for every path-like field (also inside `actions[]`), then reject empty required path with `errors.New("не указан путь к файлу")`. Return the normaliser error unwrapped (`return "", err`).

### Error handling
**Source:** `internal/tools/errors.go` lines 27-34, 38-79; `internal/tools/safe.go` lines 52-58
**Apply to:** every client-go call
```go
if err != nil {
	return "", withSubject("проект", err)   // reads
}
// writes: return "", withWrite(op, subject, err)  (new; op = opCreateBranch | opCommit, rules keyed on op; texts get statusPrefix(e))
```
Never echo raw response bodies; `capDetail` caps text at 300 runes. Handlers never build `isError` results themselves: returning a non-nil error is what makes `isError: true`. Texts are Russian only.

### Output budget and pagination footers
**Source:** `internal/tools/format.go` lines 19-107
**Apply to:** list_branches, list_commits, get_commit, compare_refs (diff), all write-success outputs that could be long
`OutputBudget = 15000` runes, `Budget(body, limit)` (rune-based, cuts on line boundary), `ClampPaging(page, perPage)` (defaults 20, max 100), `PageFooter(page, perPage, resp)`, `composeList(body, page, perPage, resp)`, `oneLine(s, max)`. Order: body, `TruncatedFooter` if cut, then page footer. Never read `X-Total`.

### Default ref resolution
**Source:** `internal/tools/ref.go` lines 19-39
**Apply to:** `list_commits` (empty `ref`), `create_branch` (empty `ref`). NOT `create_or_update_file` (branch is required), `get_commit`, `compare_refs`.
```go
ref, isDefault, err := resolveRef(ctx, d, project, strings.TrimSpace(in.Ref))
...
refLabel(ref, isDefault)   // "main (ветка по умолчанию)"
```

### Tool schema constraints (FND-08, D-15 Phase 1)
**Source:** `internal/tools/schema_test.go` lines 16-27, 89-111; `tree.go` lines 23-30
**Apply to:** all input structs
Plain typed fields with `json:"x,omitempty"` for optional args, no pointer fields (would render `["null", ...]`), `jsonschema:"..."` descriptions on every top-level property, none starting with `word=`; description const <=900 runes, names <=30 chars, no prefix. Slice fields (`actions`) need the explicit schema (schema.go).

### Registration and annotations
**Source:** `internal/tools/register.go` lines 8-38
**Apply to:** all 7 new tools. `mcp.AddTool(s, &mcp.Tool{Name, Description, Annotations}, safe(d, handler(d)))`.

### Write-safety and no retry
**Source:** `internal/glclient/{client,retry,transport}.go` (client-level GET/HEAD-only retry policy, per CONTEXT and RESEARCH; not re-read here); `errors.go` mapper
**Apply to:** `create_branch`, `commit_files`, `create_or_update_file`
Do not add retry loops or pre-checks around POST calls; 5xx/timeout/network on a write must say the outcome is unknown; success text always includes the SHA so `get_commit` can confirm. The only permitted extra request is the GET in `create_or_update_file`.

### Test harness
**Source:** `internal/tools/helpers_test.go` lines 19-65; `internal/testutil/fakegitlab.go`
**Apply to:** all new tool tests. Mock at the HTTP boundary (fake server), not at the client interface; use in-memory MCP session so schema validation by go-sdk is exercised.

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/tools/schema.go` | config | transform | No tool has an explicit `InputSchema` yet; all Phase 1 tools rely on go-sdk inference from structs. Use RESEARCH.md Pattern 1 (executed spike) and `schema_test.go` as the acceptance gate. |
| `internal/tools/testdata/tools_list.golden.json` + `-update` flag | fixture | snapshot | No golden tests exist. Use RESEARCH.md "Golden tools/list snapshot" recipe. |
| write-side error mapping content | utility | transform | Mapper structure exists, write wording is new (RESEARCH.md Pattern 3 table). |
| Non-read-only `ToolAnnotations` (`DestructiveHint *bool`) | config | registration | All Phase 1 tools are `ReadOnlyHint: true`. RESEARCH Pattern 7. |
| Client-side pagination of `compare_refs.Diffs` (synthetic `gitlab.Response`) | transform | pagination | Only GitLab-driven pagination exists. RESEARCH Pattern 5. |

## Conventions

Convention derivation skipped (the shared deriver returned `no-readable-files` for the Go tree, so it does not cover Go sources). Hand-observed in `internal/**`, for the planner to match:

| Axis | Dominant | Share | Entropy | Status |
|------|----------|-------|---------|--------|
| File-name casing | lowercase single word, `.go`; tests `<name>_test.go` (e.g. `tree.go`, `tree_test.go`, `fakegitlab.go`) | ~100% of 22 files in `internal/tools` and `internal/testutil` | low | named contract |
| Identifier casing | Go standard: exported `PascalCase` (`TreeIn`, `OutputBudget`, `ClampPaging`), unexported `camelCase` (`treeLine`, `resolveRef`) | ~100% | low | named contract |
| Export style | Go package-level; tool handlers are unexported constructors `func name(d Deps) func(ctx, in T) (string, error)`, inputs `XxxIn` exported | ~100% | low | named contract |
| Import style | three groups separated by blank lines: stdlib, third-party (`gitlab "gitlab.com/gitlab-org/api/client-go/v2"`, mcp), module-local `gitlab-mcp/internal/...` | ~100% | low | named contract |

Additional local conventions: user-facing strings and tool descriptions in Russian, `jsonschema` tag text in English; comments in English; description constants named `<tool>Description`; one concern per file.

**Contested hotspots (author's choice):** none within this Go module. Note for the mapper's generic rule: the CJS<->SDK dual resolver split (`bin/lib/**` CJS vs `sdk/src/**` ESM) is a property of the GSD plugin repo, not of this project; there is no such split here, so match the directory's local style (Go, `internal/tools`).

## Metadata

**Analog search scope:** `internal/tools`, `internal/glclient`, `internal/testutil`, `scripts`, `e2e`, `cmd` (file list via `git ls-files`)
**Files read:** `register.go`, `tree.go`, `errors.go`, `format.go`, `ref.go`, `file.go`, `safe.go`, `projects.go`, `fakegitlab.go`, `tree_test.go`, `schema_test.go`, `helpers_test.go`, `errors_test.go` (lines 1-70), `glclient/errors.go`, `glclient/normalize.go`, `scripts/smoke.py`, `e2e/python_smoke_test.go`
**Not read (assumed per RESEARCH/CONTEXT):** `glclient/{client,retry,transport}.go`, `testutil/session.go`, `e2e/stdio_test.go`, `whoami.go`, `ref_test.go`, `file_test.go`
**Pattern extraction date:** 2026-09-24
