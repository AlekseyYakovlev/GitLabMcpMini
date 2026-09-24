# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.30.0"]
# ///
"""Live run of gitlab-mcp on real gitlab.com, using the same MCP SDK as the agent.

WRITES to the project given by --project (branches live/<ts>-*, files under
live-run/<ts>/, one merged and one closed merge request). Point it at a
throwaway sandbox project only.

The run spawns the real binary over stdio exactly like the agent does (one
session per token, token passed through the child env), walks
read -> branch -> commit -> merge request -> note -> merge, probes the real
error wording (duplicates, 404, empty content, Draft merge, invalid token,
read-only token), then removes its own branches and merge requests through the
REST API outside the server. The result is written as a redacted transcript.

Environment:
    GITLAB_TOKEN           required, scope api
    GITLAB_TOKEN_READONLY  optional, scope read_api (without it the read-only
                           step is SKIPPED and criterion 2 is PARTIAL)

Exit codes: 0 all steps passed, 1 a step failed or the cleanup is incomplete,
2 usage or environment error (nothing was sent, nothing was written).

Usage:
    uv run scripts/live.py --project GROUP/PROJECT [--exe PATH] [--out PATH]
"""

import argparse
import asyncio
import hashlib
import json
import os
import re
import secrets
import string
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from contextlib import asynccontextmanager
from dataclasses import dataclass
from datetime import datetime, timezone
from importlib.metadata import version as pkg_version
from pathlib import Path

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

API_BASE = "https://gitlab.com/api/v4"
CALL_TIMEOUT = 30  # the agent's MCP_TOOL_CALL_TIMEOUT
CONNECT_TIMEOUT = 10  # the agent's MCP_CONNECT_TIMEOUT
RESULT_MAX = 20000  # the agent's MCP_TOOL_RESULT_MAX_CHARS
EXPECTED_TOOL_COUNT = 20
PENDING = {"preparing", "checking", "unchecked"}
STATUS_RE = re.compile(r"detailed_merge_status: (\w+)")
SCOPES_RE = re.compile(r"scopes \[([^\]]*)\]")
IDENTITY_RE = re.compile(r"^@\S+ \(.*\), id \d+.*$", re.M)
IDENTITY_MARK = "[identity redacted] PASS: пользователь токена получен"
GLPAT_RE = re.compile(r"glpat-[A-Za-z0-9_\-]{20,}")
WRITE_TOOLS = {
    "create_branch",
    "commit_files",
    "create_or_update_file",
    "create_merge_request",
    "update_merge_request",
    "merge_merge_request",
    "create_merge_request_note",
}
MR_SOURCE_TOOLS = {"create_merge_request", "update_merge_request"}
CLEANUP_TITLE = "cleanup (вне сервера)"

# Every secret known to this run; redact() masks all of them everywhere.
SECRETS: set[str] = set()


class LiveFailure(Exception):
    def __init__(self, message: str, recorded: bool = False):
        super().__init__(message)
        self.recorded = recorded


@dataclass
class Step:
    n: int
    title: str
    tool: str
    args: dict
    text: str
    status: str  # PASS | FAIL | SKIPPED
    note: str = ""
    crit: int = 1
    ro: bool = False


class Ctx:
    def __init__(self, project: str, token: str, ro_token: str, exe: str, ts: str):
        self.project = project
        self.token = token
        self.ro_token = ro_token
        self.exe = exe
        self.ts = ts
        self.DEFAULT: str | None = None
        self.tracked_branches: list[str] = []
        self.tracked_mrs: list[int] = []
        self.steps: list[Step] = []
        self.secrets = SECRETS
        self.cleanup_log: list[str] = []
        self.crit = 1
        self.merged_iid: int | None = None
        self.error_block_started = False
        self.crit2_complete = False
        self.protocol = "?"
        self.exe_abs = ""
        self.exe_size = 0
        self.exe_sha = ""
        self.stale_note = ""
        self.go_info: list[str] = []
        self.mcp_version = "?"
        self.started = datetime.now(timezone.utc)

    def record(self, title, tool, args, text, status, note="", ro=False) -> Step:
        step = Step(len(self.steps) + 1, title, tool, args, text, status, note, self.crit, ro)
        self.steps.append(step)
        line = f"[{step.n}] {status} {title}"
        if note:
            line += f" — {note}"
        out(line)
        return step

    @property
    def prefix(self) -> str:
        return f"live/{self.ts}-"


def redact(s: str) -> str:
    for secret in sorted(SECRETS, key=len, reverse=True):
        if secret:
            s = s.replace(secret, "[REDACTED]")
    return s


def redact_identity(text: str) -> str:
    """Hide the whoami identity line; the scopes line stays visible."""
    return IDENTITY_RE.sub(IDENTITY_MARK, text)


def out(s: str = "") -> None:
    print(redact(s))


def err(s: str) -> None:
    print(redact(s), file=sys.stderr)


def leaf_exceptions(exc: BaseException) -> list[BaseException]:
    """Unwrap (nested) exception groups raised by the anyio task groups."""
    if isinstance(exc, BaseExceptionGroup):
        leaves: list[BaseException] = []
        for sub in exc.exceptions:
            leaves.extend(leaf_exceptions(sub))
        return leaves
    return [exc]


def result_text(result) -> str:
    return "\n".join(c.text for c in result.content if getattr(c, "type", "") == "text")


def default_exe() -> str:
    root = Path(__file__).resolve().parent.parent
    name = "gitlab-mcp.exe" if os.name == "nt" else "gitlab-mcp"
    return str(root / name)


def default_out() -> str:
    root = Path(__file__).resolve().parent.parent
    return str(root / ".planning" / "phases" / "04-live-verification-and-delivery" / "LIVE-RUN.md")


def parse_scopes(text: str) -> list[str] | None:
    """Scopes of the token from the whoami text, or None when unavailable."""
    m = SCOPES_RE.search(text)
    if not m:
        return None
    return [s.strip() for s in m.group(1).split(",") if s.strip()]


# --------------------------------------------------------------------------
# preflight (no network)
# --------------------------------------------------------------------------


def newest_source_mtime(root: Path) -> float:
    newest = 0.0
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if not d.startswith(".") and d != "node_modules"]
        for name in filenames:
            if name.endswith(".go") or name == "go.mod":
                try:
                    newest = max(newest, os.path.getmtime(os.path.join(dirpath, name)))
                except OSError:
                    pass
    return newest


def preflight(ctx: Ctx) -> None:
    exe = os.path.abspath(ctx.exe)
    ctx.exe_abs = exe
    ctx.exe_size = os.path.getsize(exe)
    digest = hashlib.sha256()
    with open(exe, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            digest.update(chunk)
    ctx.exe_sha = digest.hexdigest()

    root = Path(__file__).resolve().parent.parent
    newest = newest_source_mtime(root)
    if newest and os.path.getmtime(exe) < newest:
        ctx.stale_note = "WARN: exe старее исходников (*.go/go.mod); пересоберите перед прогоном"
    else:
        ctx.stale_note = "exe не старее исходников"

    wanted = ("CGO_ENABLED", "-trimpath", "modelcontextprotocol/go-sdk", "client-go")
    try:
        proc = subprocess.run(
            ["go", "version", "-m", exe], capture_output=True, text=True, timeout=30, encoding="utf-8", errors="replace"
        )
        ctx.go_info = [ln.strip() for ln in proc.stdout.splitlines() if any(w in ln for w in wanted)]
        if not ctx.go_info:
            ctx.go_info = ["go version -m: нет нужных строк (код возврата %d)" % proc.returncode]
    except (FileNotFoundError, subprocess.TimeoutExpired, OSError) as exc:
        ctx.go_info = [f"go not found ({type(exc).__name__})"]

    try:
        ctx.mcp_version = pkg_version("mcp")
    except Exception:  # noqa: BLE001 - the version is informational only
        ctx.mcp_version = "?"


# --------------------------------------------------------------------------
# session and guarded call helper
# --------------------------------------------------------------------------


@asynccontextmanager
async def open_session(exe: str, token: str):
    """One fresh server process per token; the parent env is not inherited."""
    params = StdioServerParameters(command=exe, args=[], env={"GITLAB_TOKEN": token})
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            init = await asyncio.wait_for(session.initialize(), CONNECT_TIMEOUT)
            yield session, init


def check_write_target(ctx: Ctx, name: str, args: dict) -> None:
    """Refuse a write that could touch the default branch, before it is sent.

    Guards exactly two arguments: `branch` (every write tool) and
    `source_branch` (MR create/update). target_branch == DEFAULT is allowed on
    purpose: merging this run's own MR into the default branch is the intended
    change. Tools addressed by iid only act on MRs this run created.
    """
    if name not in WRITE_TOOLS:
        return
    checked = ["branch"]
    if name in MR_SOURCE_TOOLS:
        checked.append("source_branch")
    for key in checked:
        if key not in args:
            continue
        value = args[key]
        if value == ctx.DEFAULT or not isinstance(value, str) or not value.startswith(ctx.prefix):
            raise LiveFailure(
                f"write-target guard: {name} {key}={value!r} не начинается с {ctx.prefix!r} "
                f"или совпадает с веткой по умолчанию {ctx.DEFAULT!r}; вызов не отправлен"
            )


async def raw_call(ctx: Ctx, session, name: str, args: dict) -> tuple[str, bool]:
    check_write_target(ctx, name, args)
    result = await asyncio.wait_for(session.call_tool(name, arguments=args), CALL_TIMEOUT)
    return result_text(result), bool(result.isError)


def display_text(name: str, text: str) -> str:
    return redact(redact_identity(text) if name == "whoami" else text)


def show(name: str, args: dict, text: str) -> None:
    print(redact(f"--- {name} {json.dumps(args, ensure_ascii=False)}"))
    print(display_text(name, text))


async def call(
    ctx: Ctx,
    session,
    name: str,
    args: dict,
    *,
    expect_error: bool = False,
    check=None,
    expect: str = "",
    hint: str = "",
    title: str | None = None,
    fatal: bool = True,
    ro: bool = False,
) -> str:
    """Call one tool, record the step, fail the chain when fatal."""
    title = title or name
    problems: list[str] = []
    try:
        text, is_err = await raw_call(ctx, session, name, args)
    except asyncio.TimeoutError:
        text, is_err = f"таймаут {CALL_TIMEOUT} с", False
        problems.append(f"таймаут вызова {CALL_TIMEOUT} с")
    else:
        if is_err != expect_error:
            problems.append(f"isError={is_err}, ожидалось {expect_error}")
        if len(text) > RESULT_MAX:
            problems.append(f"ответ {len(text)} символов > {RESULT_MAX}")
        if any(s and s in text for s in SECRETS):
            problems.append("токен присутствует в ответе сервера")
        if is_err == expect_error and check is not None and not check(text):
            problems.append("проверка текста ответа не прошла" + (f" (ожидалось: {expect})" if expect else ""))
    show(name, args, text)
    stored = display_text(name, text)
    if problems:
        note = "; ".join(problems) + (f". {hint}" if hint else "")
        step = ctx.record(title, name, args, stored, "FAIL", note, ro)
        if fatal:
            raise LiveFailure(f"шаг {step.n} ({title}) не прошёл: {note}", recorded=True)
    else:
        ctx.record(title, name, args, stored, "PASS", expect, ro)
    return text


def last_ok(ctx: Ctx) -> bool:
    return bool(ctx.steps) and ctx.steps[-1].status == "PASS"


def skip(ctx: Ctx, title: str, reason: str, ro: bool = False) -> None:
    ctx.record(title, "-", {}, "", "SKIPPED", reason, ro)


async def wait_mergeable(ctx: Ctx, session, iid: int, want: str = "mergeable", fatal: bool = True) -> bool:
    """Poll get_merge_request while GitLab is still checking, then judge the status."""
    args = {"project": ctx.project, "iid": iid}
    tries, pause = 20, 3.0
    text, is_err, status, polls = "", False, "?", 0
    for polls in range(1, tries + 1):
        text, is_err = await raw_call(ctx, session, "get_merge_request", args)
        show("get_merge_request", args, text)
        m = STATUS_RE.search(text)
        status = m.group(1) if m else "?"
        if is_err or status not in PENDING:
            break
        await asyncio.sleep(pause)
    ok = (not is_err) and status == want
    note = f"detailed_merge_status={status}, ожидалось {want}, опросов: {polls}"
    step = ctx.record(f"ожидание статуса {want} для MR !{iid}", "get_merge_request", args, redact(text), "PASS" if ok else "FAIL", note)
    if not ok and fatal:
        raise LiveFailure(f"шаг {step.n}: MR !{iid} в статусе {status}, ожидалось {want}: {note}", recorded=True)
    return ok


# --------------------------------------------------------------------------
# main scenario (criterion 1)
# --------------------------------------------------------------------------


async def main_scenario(ctx: Ctx, session) -> None:
    p, ts = ctx.project, ctx.ts
    main_branch = f"{ctx.prefix}main"
    run_dir = f"live-run/{ts}"

    # 1 whoami: the identity line is hidden by call(); the token needs scope api.
    await call(
        ctx, session, "whoami", {},
        check=lambda t: (parse_scopes(t) is None) or ("api" in parse_scopes(t)),
        expect="scopes содержит api (либо scopes unavailable)",
        title="1 whoami",
    )
    if parse_scopes(ctx.steps[-1].text) is None:
        ctx.steps[-1].note += "; WARN: scopes unavailable, права токена не проверены"

    # 2 get_project: the default branch feeds every later step.
    found = {}

    def chk_project(t: str) -> bool:
        m = re.search(r"default_branch: (\S+)", t)
        if m:
            found["default"] = m.group(1)
        return m is not None

    await call(ctx, session, "get_project", {"project": p}, check=chk_project,
               expect="есть строка default_branch", title="2 get_project")
    ctx.DEFAULT = found["default"]
    default = ctx.DEFAULT

    # 3-4 read the sandbox sentinel.
    await call(ctx, session, "list_repository_tree", {"project": p},
               check=lambda t: "README.md" in t, expect="в корне есть README.md", title="3 list_repository_tree")
    await call(ctx, session, "get_file_contents", {"project": p, "path": "README.md", "ref": default},
               hint=f"в sandbox в ветке {default} нет README.md (D-05)", title="4 get_file_contents README.md")

    # 5 branch: registered for cleanup before the call.
    ctx.tracked_branches.append(main_branch)
    await call(ctx, session, "create_branch", {"project": p, "branch": main_branch, "ref": default},
               check=lambda t: main_branch in t, expect="ответ называет новую ветку", title="5 create_branch")

    # 6 one commit with three new files.
    sha = {}

    def chk_commit(t: str) -> bool:
        m = re.search(r"коммит ([0-9a-f]{7,40})", t)
        if m:
            sha["v"] = m.group(1)
        creates = [ln for ln in t.splitlines() if ln.startswith("create ")]
        return t.startswith("коммит ") and len(creates) == 3 and m is not None

    files = [f"{run_dir}/a.md", f"{run_dir}/b.md", f"{run_dir}/notes.md"]
    await call(
        ctx, session, "commit_files",
        {
            "project": p, "branch": main_branch, "commit_message": f"live {ts}: add files",
            "actions": [
                {"action": "create", "file_path": f, "content": f"# {f}\nlive run {ts}\n"} for f in files
            ],
        },
        check=chk_commit, expect="коммит с тремя строками create", title="6 commit_files",
    )

    # 7 single-file tool: update a file of this run, then create another.
    await call(
        ctx, session, "create_or_update_file",
        {"project": p, "path": f"{run_dir}/notes.md", "content": f"# notes\nlive run {ts}\nupdated\n",
         "branch": main_branch, "commit_message": f"live {ts}: update notes"},
        check=lambda t: t.startswith("updated"), expect="ответ начинается с updated", title="7a create_or_update_file (update)",
    )
    await call(
        ctx, session, "create_or_update_file",
        {"project": p, "path": f"{run_dir}/c.md", "content": f"# c\nlive run {ts}\n",
         "branch": main_branch, "commit_message": f"live {ts}: add c"},
        check=lambda t: t.startswith("created"), expect="ответ начинается с created", title="7b create_or_update_file (create)",
    )

    # 8-9 read the commit and compare the branch with the default.
    await call(ctx, session, "get_commit", {"project": p, "sha": sha["v"]},
               check=lambda t: f"{run_dir}/a.md" in t, expect="в коммите виден a.md", title="8 get_commit")
    await call(ctx, session, "compare_refs", {"project": p, "from": default, "to": main_branch},
               check=lambda t: "c.md" in t, expect="в сравнении виден c.md", title="9 compare_refs")

    # 10 merge request.
    mr = {}

    def chk_mr(t: str) -> bool:
        m = re.search(r"MR !(\d+) создан", t)
        if m:
            mr["iid"] = int(m.group(1))
        return m is not None

    await call(
        ctx, session, "create_merge_request",
        {"project": p, "source_branch": main_branch, "target_branch": default,
         "title": f"live {ts}: gitlab-mcp live run", "description": f"Создан scripts/live.py, прогон {ts}."},
        check=chk_mr, expect="«MR !N создан»", title="10 create_merge_request",
    )
    iid = mr["iid"]
    ctx.tracked_mrs.append(iid)

    # 11-13 note, notes, diffs.
    body = f"live {ts}: note"
    await call(ctx, session, "create_merge_request_note", {"project": p, "iid": iid, "body": body},
               check=lambda t: re.search(rf"комментарий #\d+ добавлен к MR !{iid}", t) is not None,
               expect=f"«комментарий #ID добавлен к MR !{iid}»", title="11 create_merge_request_note")
    await call(ctx, session, "list_merge_request_notes", {"project": p, "iid": iid},
               check=lambda t: body in t, expect="комментарий виден в списке", title="12 list_merge_request_notes")
    await call(ctx, session, "get_merge_request_diffs", {"project": p, "iid": iid},
               check=lambda t: "a.md" in t, expect="в diff виден a.md", title="13 get_merge_request_diffs")

    # 14 wait for the mergeability check, 15 merge.
    await wait_mergeable(ctx, session, iid, want="mergeable")

    def chk_merge(t: str) -> bool:
        return f"MR !{iid} влит" in t and any(k in t for k in ("merge commit:", "squash commit:", "head sha (fast-forward):"))

    await call(ctx, session, "merge_merge_request", {"project": p, "iid": iid, "remove_source_branch": True},
               check=chk_merge, expect=f"«MR !{iid} влит» и SHA коммита", title="15 merge_merge_request")
    ctx.merged_iid = iid

    # 16 the merge request is merged (retry: the state can lag briefly).
    text = ""
    for attempt in range(3):
        text, is_err = await raw_call(ctx, session, "get_merge_request", {"project": p, "iid": iid})
        show("get_merge_request", {"project": p, "iid": iid}, text)
        if not is_err and "merged" in text:
            break
        await asyncio.sleep(2)
    ok = "merged" in text
    step = ctx.record("16 get_merge_request после merge", "get_merge_request", {"project": p, "iid": iid},
                      redact(text), "PASS" if ok else "FAIL", f"попыток: {attempt + 1}")
    if not ok:
        raise LiveFailure(f"шаг {step.n}: MR !{iid} не в состоянии merged", recorded=True)

    # 17 the merged file is now in the default branch.
    await call(ctx, session, "get_file_contents", {"project": p, "path": f"{run_dir}/c.md", "ref": default},
               check=lambda t: ts in t, expect="c.md в ветке по умолчанию содержит ts", title="17 get_file_contents (после merge)")

    # 18 the source branch removal is asynchronous: poll, warn instead of failing.
    listed = ""
    for _ in range(7):
        listed, is_err = await raw_call(ctx, session, "list_branches", {"project": p, "search": main_branch})
        if not is_err and main_branch not in listed:
            break
        await asyncio.sleep(3)
    show("list_branches", {"project": p, "search": main_branch}, listed)
    gone = main_branch not in listed
    ctx.record("18 list_branches: ветка-источник удалена", "list_branches", {"project": p, "search": main_branch},
               redact(listed), "PASS",
               "ветка удалена" if gone else "WARN: ветка ещё не удалена (удаление асинхронное), cleanup удалит")


# --------------------------------------------------------------------------
# error block (criterion 2)
# --------------------------------------------------------------------------


async def error_block(ctx: Ctx, session) -> None:
    ctx.crit = 2
    ctx.error_block_started = True
    p, ts, default = ctx.project, ctx.ts, ctx.DEFAULT
    run_dir = f"live-run/{ts}"
    draft = f"{ctx.prefix}draft"

    # E1 duplicate branch.
    ctx.tracked_branches.append(draft)
    await call(ctx, session, "create_branch", {"project": p, "branch": draft, "ref": default},
               check=lambda t: draft in t, expect="ветка создана", title="E1a create_branch (для Draft MR)", fatal=False)
    have_branch = last_ok(ctx)
    await call(ctx, session, "create_branch", {"project": p, "branch": draft, "ref": default},
               expect_error=True, check=lambda t: "ветка уже существует" in t,
               expect="isError, «ветка уже существует»", title="E1b create_branch (дубль)", fatal=False)

    # E2 a diff for the Draft MR.
    committed = False
    if have_branch:
        await call(
            ctx, session, "commit_files",
            {"project": p, "branch": draft, "commit_message": f"live {ts}: draft",
             "actions": [{"action": "create", "file_path": f"{run_dir}/draft.md", "content": f"draft {ts}\n"}]},
            check=lambda t: t.startswith("коммит "), expect="коммит создан", title="E2 commit_files (diff для Draft MR)",
            fatal=False,
        )
        committed = last_ok(ctx)
    else:
        skip(ctx, "E2 commit_files (diff для Draft MR)", "ветка Draft не создана (E1a)")

    # E3 empty content is rejected before any request (999.1 live).
    if have_branch:
        await call(
            ctx, session, "commit_files",
            {"project": p, "branch": draft, "commit_message": f"live {ts}: empty",
             "actions": [{"action": "create", "file_path": f"{run_dir}/empty.md"}]},
            expect_error=True, check=lambda t: "непустой content" in t,
            expect="isError, «непустой content»", title="E3 commit_files (create без content)", fatal=False,
        )
    else:
        skip(ctx, "E3 commit_files (create без content)", "ветка Draft не создана (E1a)")

    # E4-E6 Draft MR: merge refused, duplicate refused, then closed.
    draft_iid = {}

    def chk_draft_mr(t: str) -> bool:
        m = re.search(r"MR !(\d+) создан", t)
        if m:
            draft_iid["v"] = int(m.group(1))
        return m is not None

    if committed:
        await call(
            ctx, session, "create_merge_request",
            {"project": p, "source_branch": draft, "target_branch": default, "title": f"live {ts}: draft", "draft": True},
            check=chk_draft_mr, expect="«MR !N создан»", title="E4a create_merge_request (draft)", fatal=False,
        )
    else:
        skip(ctx, "E4a create_merge_request (draft)", "нет коммита в ветке Draft (E1a/E2)")
    if "v" in draft_iid:
        d = draft_iid["v"]
        ctx.tracked_mrs.append(d)
        await wait_mergeable(ctx, session, d, want="draft_status", fatal=False)
        await call(ctx, session, "merge_merge_request", {"project": p, "iid": d},
                   expect_error=True, check=lambda t: "draft_status" in t or "Draft" in t,
                   expect="isError, причина draft_status/Draft", title="E4b merge_merge_request (Draft)", fatal=False)
        await call(
            ctx, session, "create_merge_request",
            {"project": p, "source_branch": draft, "target_branch": default, "title": f"live {ts}: draft dup"},
            expect_error=True, check=lambda t: t.startswith(f"открытый MR уже есть: !{d}"),
            expect=f"isError, «открытый MR уже есть: !{d}»", title="E5 create_merge_request (дубль)", fatal=False,
        )
        await call(ctx, session, "update_merge_request", {"project": p, "iid": d, "state_event": "close"},
                   check=lambda t: True, expect="MR закрыт", title="E6 update_merge_request (close)", fatal=False)
    else:
        for title in ("E4b merge_merge_request (Draft)", "E5 create_merge_request (дубль)", "E6 update_merge_request (close)"):
            skip(ctx, title, "Draft MR не создан (E4a)")

    # E7-E9 not found: file, ref, project.
    await call(ctx, session, "get_file_contents", {"project": p, "path": f"{run_dir}/no-such-file.md", "ref": default},
               expect_error=True, check=lambda t: t.startswith("404: не найдено"),
               expect="isError, «404: не найдено»", title="E7 get_file_contents (нет файла)", fatal=False)
    await call(ctx, session, "get_file_contents", {"project": p, "path": "README.md", "ref": f"{ctx.prefix}no-such-branch"},
               expect_error=True, check=lambda t: "404" in t,
               expect="isError, 404", title="E8 get_file_contents (нет ветки)", fatal=False)
    namespace = p.rsplit("/", 1)[0] + "/" if "/" in p else ""
    await call(ctx, session, "get_project", {"project": f"{namespace}gitlab-mcp-live-missing-{ts}"},
               expect_error=True, check=lambda t: t.startswith("404: не найдено"),
               expect="isError, «404: не найдено»", title="E9 get_project (нет проекта)", fatal=False)

    # E10 merge of an already merged MR.
    if ctx.merged_iid is not None:
        await call(ctx, session, "merge_merge_request", {"project": p, "iid": ctx.merged_iid},
                   expect_error=True,
                   check=lambda t: any(k in t for k in ("MR уже влит", "not_open", "MR не открыт")),
                   expect="isError, MR уже влит / not_open", title="E10 merge_merge_request (уже влит)", fatal=False)
    else:
        skip(ctx, "E10 merge_merge_request (уже влит)", "основной сценарий не дошёл до merge")


async def invalid_token_session(ctx: Ctx) -> None:
    """Session B: a well-formed but invalid token must give a clear 401 without echo."""
    dummy = "glpat-" + "".join(secrets.choice(string.ascii_letters) for _ in range(20))
    SECRETS.add(dummy)
    async with open_session(ctx.exe, dummy) as (session, _init):
        await call(
            ctx, session, "whoami", {}, expect_error=True,
            check=lambda t: t.startswith("401: GitLab отклонил токен") and dummy not in t,
            expect="isError, «401: GitLab отклонил токен», токен не в тексте",
            title="B whoami (невалидный токен)", fatal=False,
        )


async def readonly_token_session(ctx: Ctx) -> None:
    """Session C: a read_api token can read but its write gives a clear 403."""
    title = "C запись read-only токеном"
    if not ctx.ro_token:
        skip(ctx, title, "GITLAB_TOKEN_READONLY не задан — критерий 2 PARTIAL", ro=True)
        return
    p = ctx.project
    async with open_session(ctx.exe, ctx.ro_token) as (session, _init):
        await call(ctx, session, "whoami", {}, expect="токен read-only без scope api", ro=True,
                   title="C1 whoami (read-only токен)", fatal=False)
        scopes = parse_scopes(ctx.steps[-1].text)
        if scopes is not None and "api" in scopes:
            ctx.steps[-1].status = "FAIL"
            ctx.steps[-1].note = "read-only токен имеет scope api; запись не выполнена"
            out(f"[{ctx.steps[-1].n}] FAIL read-only токен имеет scope api")
            return
        await call(ctx, session, "list_branches", {"project": p}, expect="чтение read-only токеном работает",
                   ro=True, title="C2 list_branches (read-only токен)", fatal=False)
        ro_branch = f"{ctx.prefix}ro"
        ctx.tracked_branches.append(ro_branch)
        await call(
            ctx, session, "create_branch", {"project": p, "branch": ro_branch, "ref": ctx.DEFAULT},
            expect_error=True,
            check=lambda t: t.startswith("403: запись отклонена") and "scope `api`" in t,
            expect="isError, «403: запись отклонена», подсказка про scope `api`", ro=True,
            title="C3 create_branch (read-only токен)", fatal=False,
        )


async def run(ctx: Ctx) -> None:
    async with open_session(ctx.exe, ctx.token) as (session, init):
        ctx.protocol = str(init.protocolVersion)
        tools = (await session.list_tools()).tools
        good = len(tools) == EXPECTED_TOOL_COUNT
        step = ctx.record("0 tools/list", "list_tools", {}, ", ".join(sorted(t.name for t in tools)),
                          "PASS" if good else "FAIL", f"инструментов: {len(tools)}, ожидалось {EXPECTED_TOOL_COUNT}")
        if not good:
            raise LiveFailure(f"шаг {step.n}: неверное число инструментов", recorded=True)
        await main_scenario(ctx, session)
        await error_block(ctx, session)
    await invalid_token_session(ctx)
    await readonly_token_session(ctx)
    ctx.crit2_complete = True


# --------------------------------------------------------------------------
# cleanup (outside the server, REST + PRIVATE-TOKEN)
# --------------------------------------------------------------------------


def rest(ctx: Ctx, method: str, path: str, data: dict | None = None) -> tuple[int, str]:
    body = json.dumps(data).encode("utf-8") if data is not None else None
    headers = {"PRIVATE-TOKEN": ctx.token, "User-Agent": "gitlab-mcp-live/1.0"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(API_BASE + path, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        try:
            return exc.code, exc.read().decode("utf-8", "replace")
        except Exception:  # noqa: BLE001
            return exc.code, ""
    except (urllib.error.URLError, OSError) as exc:
        return 0, str(exc)


def cleanup(ctx: Ctx) -> None:
    """Close this run's open MRs and delete this run's branches; nothing else."""
    log = ctx.cleanup_log
    if not ctx.tracked_branches:
        log.append("нечего чистить: ветки не создавались")
        return
    proj = urllib.parse.quote(ctx.project, safe="")
    for branch in ctx.tracked_branches:
        if not branch.startswith(ctx.prefix) or branch == ctx.DEFAULT:
            log.append(f"cleanup INCOMPLETE: отказ трогать ветку {branch!r} (не {ctx.prefix}* или ветка по умолчанию)")
            continue
        qb = urllib.parse.quote(branch, safe="")
        try:
            status, body = rest(ctx, "GET", f"/projects/{proj}/merge_requests?state=opened&source_branch={qb}")
            if status == 200:
                for item in json.loads(body):
                    iid = item.get("iid")
                    s, _ = rest(ctx, "PUT", f"/projects/{proj}/merge_requests/{iid}", {"state_event": "close"})
                    if s == 200:
                        log.append(f"MR !{iid} ({branch}): закрыт")
                    else:
                        log.append(f"cleanup INCOMPLETE: MR !{iid} ({branch}) не закрыт, HTTP {s}")
            else:
                log.append(f"cleanup INCOMPLETE: список открытых MR ветки {branch} недоступен, HTTP {status}")
            status, _ = rest(ctx, "DELETE", f"/projects/{proj}/repository/branches/{qb}")
            if status == 204:
                log.append(f"ветка {branch}: удалена")
            elif status == 404:
                log.append(f"ветка {branch}: уже отсутствует (404)")
            else:
                log.append(f"cleanup INCOMPLETE: ветка {branch} не удалена, HTTP {status}")
        except Exception as exc:  # noqa: BLE001 - one object must not abort the rest
            log.append(f"cleanup INCOMPLETE: ветка {branch}: {type(exc).__name__}: {exc}")


# --------------------------------------------------------------------------
# transcript
# --------------------------------------------------------------------------


def crit_status(ctx: Ctx, crit: int) -> str:
    steps = [s for s in ctx.steps if s.crit == crit]
    if not steps:
        return "FAIL (не выполнялся)"
    if any(s.status == "FAIL" or (s.status == "SKIPPED" and not s.ro) for s in steps):
        return "FAIL"
    if crit == 1:
        return "PASS" if ctx.merged_iid is not None else "FAIL (merge не достигнут)"
    if not ctx.crit2_complete:
        return "FAIL (не завершён)"
    if any(s.status == "SKIPPED" for s in steps):
        return "PARTIAL"
    return "PASS"


def cleanup_incomplete(ctx: Ctx) -> bool:
    return any(line.startswith("cleanup INCOMPLETE") for line in ctx.cleanup_log)


def has_failures(ctx: Ctx) -> bool:
    if any(s.status == "FAIL" or (s.status == "SKIPPED" and not s.ro) for s in ctx.steps):
        return True
    return not ctx.steps or cleanup_incomplete(ctx)


def build_transcript(ctx: Ctx) -> str:
    def fence(text: str, lang: str) -> list[str]:
        return [f"````{lang}", text, "````"]

    build_ok = any("CGO_ENABLED=0" in ln for ln in ctx.go_info)
    lines = [
        "# LIVE-RUN: живой прогон gitlab-mcp на gitlab.com",
        "",
        f"- Дата (UTC): {ctx.started.strftime('%Y-%m-%dT%H:%M:%SZ')}",
        f"- Проект: {ctx.project}",
        f"- Метка прогона: {ctx.ts}",
        f"- exe: {ctx.exe_abs}",
        f"- Размер: {ctx.exe_size} байт",
        f"- SHA-256: {ctx.exe_sha}",
        f"- Актуальность: {ctx.stale_note}",
        f"- mcp client: {ctx.mcp_version}",
        f"- protocolVersion: {ctx.protocol}",
        "",
        "go version -m (выдержка):",
        "",
        *fence("\n".join(ctx.go_info), "text"),
        "",
        "## Критерии",
        "",
        "| Критерий | Статус |",
        "|----------|--------|",
        f"| 1. Основной сценарий: чтение, ветка, коммит, MR, комментарий, merge, очистка | {crit_status(ctx, 1)} |",
        f"| 2. Реальные ошибки: дубли, 404, пустой content, Draft, 401, read-only 403 | {crit_status(ctx, 2)} |",
        f"| Сборка без внешних зависимостей (CGO_ENABLED=0), размер {ctx.exe_size} байт | {'PASS' if build_ok else 'не подтверждена'} |",
        f"| Очистка | {'INCOMPLETE' if cleanup_incomplete(ctx) else 'PASS'} |",
        "",
        "## Шаги",
        "",
    ]
    for step in ctx.steps:
        lines += [f"### {step.n}. {step.title} — {step.status}", "", f"- tool: `{step.tool}`"]
        if step.note:
            lines.append(f"- примечание: {step.note}")
        lines.append("")
        lines += fence(json.dumps(step.args, ensure_ascii=False, indent=2), "json")
        if step.text:
            lines.append("")
            lines += fence(step.text, "text")
        lines.append("")
    lines += [f"## {CLEANUP_TITLE}", ""]
    lines += [f"- {entry}" for entry in ctx.cleanup_log] or ["- (пусто)"]
    lines.append("")
    return "\n".join(lines)


def write_transcript(ctx: Ctx, out_path: str) -> str:
    doc = redact(build_transcript(ctx))
    for secret in SECRETS:
        assert not secret or secret not in doc, "токен попал в транскрипт"
    assert not GLPAT_RE.search(doc), "в транскрипте есть строка, похожая на токен glpat-"
    assert not re.search(r"^@\S+ \(.*\), id \d+", doc, re.M), "в транскрипте осталась строка идентичности whoami"
    target = Path(out_path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(doc, encoding="utf-8")
    return str(target)


# --------------------------------------------------------------------------
# entry point
# --------------------------------------------------------------------------


def main() -> int:
    # Tool texts are Russian; a piped stdout on Windows defaults to a legacy code page.
    for stream in (sys.stdout, sys.stderr):
        stream.reconfigure(encoding="utf-8", errors="replace")

    parser = argparse.ArgumentParser(
        description=(
            "gitlab-mcp live run on real gitlab.com (mcp==1.30.0). WRITES to --project. / "
            "Живой прогон на gitlab.com: пишет в проект --project."
        )
    )
    parser.add_argument("--project", required=True, help="sandbox project GROUP/PROJECT to write to (required, no default)")
    parser.add_argument("--exe", default=default_exe(), help="path to gitlab-mcp binary")
    parser.add_argument("--out", default=default_out(), help="path of the LIVE-RUN.md transcript")
    args = parser.parse_args()

    project = args.project.strip()
    if not project or any(ch.isspace() for ch in project):
        err("--project должен быть непустым путём GROUP/PROJECT / --project must be a non-empty GROUP/PROJECT")
        return 2
    token = os.environ.get("GITLAB_TOKEN", "").strip()
    if not token:
        err("GITLAB_TOKEN не задан (нужен токен со scope api) / GITLAB_TOKEN is not set")
        return 2
    if not os.path.isfile(args.exe):
        err(f"exe не найден: {args.exe} (соберите его или укажите --exe) / binary not found")
        return 2
    ro_token = os.environ.get("GITLAB_TOKEN_READONLY", "").strip()

    SECRETS.add(token)
    if ro_token:
        SECRETS.add(ro_token)
    ts = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    ctx = Ctx(project, token, ro_token, args.exe, ts)
    preflight(ctx)

    out(f"ЗАПИСЬ в проект {project} на gitlab.com (ветки {ctx.prefix}*)")
    out(f"exe: {ctx.exe_abs}, {ctx.exe_size} байт, sha256 {ctx.exe_sha}")
    if ctx.stale_note.startswith("WARN"):
        out(ctx.stale_note)

    transcript_ok = True
    transcript_path = args.out
    try:
        try:
            asyncio.run(run(ctx))
        except Exception as exc:  # noqa: BLE001 - report any client-side failure as a live failure
            for leaf in leaf_exceptions(exc):
                message = str(leaf) if isinstance(leaf, LiveFailure) else f"{type(leaf).__name__}: {leaf}"
                err(f"LIVE FAIL: {message}")
                if not (isinstance(leaf, LiveFailure) and leaf.recorded):
                    ctx.record("прерывание сценария", "-", {}, redact(message), "FAIL", "исключение вне проверки шага")
    finally:
        cleanup(ctx)
        for entry in ctx.cleanup_log:
            out(f"{CLEANUP_TITLE}: {entry}")
        try:
            transcript_path = write_transcript(ctx, args.out)
        except AssertionError as exc:
            transcript_ok = False
            err(f"LIVE FAIL: транскрипт не записан: {exc}")

    failed = has_failures(ctx) or not transcript_ok
    out(f"критерий 1: {crit_status(ctx, 1)}; критерий 2: {crit_status(ctx, 2)}")
    if transcript_ok:
        out(f"транскрипт: {transcript_path}")
    out("LIVE FAIL" if failed else "LIVE OK")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
