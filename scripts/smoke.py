# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.30.0"]
# ///
"""Smoke client for gitlab-mcp, using the same MCP SDK version as the agent.

Spawns the server over stdio exactly like the agent does (command + per-server
env), performs initialize and tools/list, checks every tool against the agent's
limits, and calls the tools. The token is read from GITLAB_TOKEN and is never
printed.

Two modes:
    hermetic  --base-url points at a fake GitLab (used by the Go e2e tests).
    live      no --base-url: read-only calls against gitlab.com with the real
              GITLAB_TOKEN; --project is required and the first list_projects
              page is printed so default_branch can be checked by eye.

Usage:
    uv run scripts/smoke.py [--exe PATH] [--base-url URL] [--project P] [--file F]
"""

import argparse
import asyncio
import os
import sys
from pathlib import Path

from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

EXPECTED_PROTOCOL = "2025-11-25"
MAX_TOOL_NAME = 30
MAX_TOOL_DESCRIPTION = 900
EXPECTED_TOOLS = {
    "whoami",
    "list_projects",
    "get_project",
    "list_repository_tree",
    "get_file_contents",
}
FORBIDDEN_SCHEMA_KEYS = {"$ref", "$defs", "anyOf", "oneOf"}


class SmokeFailure(Exception):
    pass


def check(condition: bool, reason: str) -> None:
    if not condition:
        raise SmokeFailure(reason)


def leaf_exceptions(exc: BaseException) -> list[BaseException]:
    """Unwrap (nested) exception groups raised by the anyio task groups."""
    if isinstance(exc, BaseExceptionGroup):
        leaves: list[BaseException] = []
        for sub in exc.exceptions:
            leaves.extend(leaf_exceptions(sub))
        return leaves
    return [exc]


def walk_schema(node, path: str) -> None:
    """Reject schema constructs the agent's LLM tooling handles poorly."""
    if isinstance(node, dict):
        for key, value in node.items():
            check(key not in FORBIDDEN_SCHEMA_KEYS, f"{path}: forbidden schema key {key!r}")
            if key == "type" and isinstance(value, list):
                check("null" not in value, f"{path}: 'null' inside type list {value!r}")
            walk_schema(value, f"{path}/{key}")
    elif isinstance(node, list):
        for i, value in enumerate(node):
            walk_schema(value, f"{path}[{i}]")


def result_text(result) -> str:
    return "\n".join(c.text for c in result.content if getattr(c, "type", "") == "text")


def default_exe() -> str:
    root = Path(__file__).resolve().parent.parent
    name = "gitlab-mcp.exe" if os.name == "nt" else "gitlab-mcp"
    return str(root / name)


async def run(exe: str, base_url: str | None, token: str, project: str, file_path: str) -> None:
    env = {"GITLAB_TOKEN": token}
    if base_url:
        env["GITLAB_URL"] = base_url
    params = StdioServerParameters(command=exe, args=[], env=env)

    texts: list[str] = []

    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            init = await session.initialize()
            check(
                str(init.protocolVersion) == EXPECTED_PROTOCOL,
                f"protocolVersion is {init.protocolVersion!r}, want {EXPECTED_PROTOCOL!r}",
            )

            tools = (await session.list_tools()).tools
            check(len(tools) > 0, "tools/list returned no tools")
            names = {t.name for t in tools}
            for tool in tools:
                check(len(tool.name) <= MAX_TOOL_NAME, f"tool name too long: {tool.name}")
                desc = tool.description or ""
                check(
                    len(desc) <= MAX_TOOL_DESCRIPTION,
                    f"{tool.name}: description is {len(desc)} chars, max {MAX_TOOL_DESCRIPTION}",
                )
                schema = tool.inputSchema
                check(schema.get("type") == "object", f"{tool.name}: inputSchema type is not object")
                walk_schema(schema, tool.name)
            check(
                names == EXPECTED_TOOLS,
                f"tool set is {sorted(names)}, want {sorted(EXPECTED_TOOLS)}",
            )
            print(f"tools: {', '.join(sorted(names))}")

            async def call(name: str, arguments: dict, expect_error: bool = False) -> str:
                result = await session.call_tool(name, arguments)
                text = result_text(result)
                texts.append(text)
                if expect_error:
                    check(result.isError, f"{name} {arguments} did not return isError: {text}")
                else:
                    check(not result.isError, f"{name} {arguments} returned isError: {text}")
                print(f"--- {name} {arguments}")
                print(text)
                return text

            await call("whoami", {})
            await call("whoami", {"bogus": 1}, expect_error=True)

            await call("list_projects", {"per_page": 5})
            await call("get_project", {"project": project})
            await call("list_repository_tree", {"project": project, "per_page": 1})
            await call(
                "get_file_contents",
                {"project": project, "path": file_path, "end_line": 20},
            )
            missing = await call(
                "get_project",
                {"project": "no-such-group-xyz/no-such-project"},
                expect_error=True,
            )
            check("404" in missing, f"missing project error does not mention 404: {missing}")

    for text in texts:
        check(token not in text, "token appeared in a tool result")


def main() -> int:
    # Tool texts are Russian; a piped stdout on Windows defaults to a legacy code page.
    for stream in (sys.stdout, sys.stderr):
        stream.reconfigure(encoding="utf-8", errors="replace")

    parser = argparse.ArgumentParser(description="gitlab-mcp smoke client (mcp==1.30.0)")
    parser.add_argument("--exe", default=default_exe(), help="path to gitlab-mcp binary")
    parser.add_argument("--base-url", default=None, help="GITLAB_URL override (fake GitLab)")
    parser.add_argument("--project", default=None, help="project ID or path to read (required in live mode)")
    parser.add_argument("--file", default="README.md", help="file path to read from the project")
    args = parser.parse_args()

    token = os.environ.get("GITLAB_TOKEN", "").strip()
    if not token:
        print("GITLAB_TOKEN is not set in the environment", file=sys.stderr)
        return 2

    # Without --base-url the script talks to the real gitlab.com (read-only
    # calls only), so the project to read must be chosen explicitly.
    if args.base_url is None and not args.project:
        print("live mode (no --base-url) requires --project GROUP/PROJECT", file=sys.stderr)
        return 2
    project = args.project or "g/p"

    try:
        asyncio.run(run(args.exe, args.base_url, token, project, args.file))
    except Exception as exc:  # noqa: BLE001 - report any client-side failure as a smoke failure
        for leaf in leaf_exceptions(exc):
            message = f"{type(leaf).__name__}: {leaf}".replace(token, "[REDACTED]")
            if isinstance(leaf, SmokeFailure):
                message = str(leaf)
            print(f"SMOKE FAIL: {message}", file=sys.stderr)
        return 1

    print("SMOKE OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
