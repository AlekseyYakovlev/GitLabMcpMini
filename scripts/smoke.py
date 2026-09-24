# /// script
# requires-python = ">=3.10"
# dependencies = ["mcp==1.30.0"]
# ///
"""Smoke client for gitlab-mcp, using the same MCP SDK version as the agent.

Spawns the server over stdio exactly like the agent does (command + per-server
env), performs initialize and tools/list, checks every tool against the agent's
limits, and calls the tools. The token is read from GITLAB_TOKEN and is never
printed.

Usage:
    uv run scripts/smoke.py [--exe PATH] [--base-url URL]
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
FORBIDDEN_SCHEMA_KEYS = {"$ref", "$defs", "anyOf", "oneOf"}


class SmokeFailure(Exception):
    pass


def check(condition: bool, reason: str) -> None:
    if not condition:
        raise SmokeFailure(reason)


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


async def run(exe: str, base_url: str | None, token: str) -> None:
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
            check("whoami" in names, f"whoami missing from tools: {sorted(names)}")
            print(f"tools: {', '.join(sorted(names))}")

            result = await session.call_tool("whoami", {})
            text = result_text(result)
            texts.append(text)
            check(not result.isError, f"whoami returned isError: {text}")
            print(text)

            bad = await session.call_tool("whoami", {"bogus": 1})
            bad_text = result_text(bad)
            texts.append(bad_text)
            check(bad.isError, "whoami with an unknown argument did not return isError")
            print(bad_text)

    for text in texts:
        check(token not in text, "token appeared in a tool result")


def main() -> int:
    parser = argparse.ArgumentParser(description="gitlab-mcp smoke client (mcp==1.30.0)")
    parser.add_argument("--exe", default=default_exe(), help="path to gitlab-mcp binary")
    parser.add_argument("--base-url", default=None, help="GITLAB_URL override (fake GitLab)")
    args = parser.parse_args()

    token = os.environ.get("GITLAB_TOKEN", "").strip()
    if not token:
        print("GITLAB_TOKEN is not set in the environment", file=sys.stderr)
        return 2

    try:
        asyncio.run(run(args.exe, args.base_url, token))
    except SmokeFailure as exc:
        print(f"SMOKE FAIL: {exc}", file=sys.stderr)
        return 1
    except Exception as exc:  # noqa: BLE001 - report any client-side failure as a smoke failure
        message = str(exc).replace(token, "[REDACTED]")
        print(f"SMOKE FAIL: {type(exc).__name__}: {message}", file=sys.stderr)
        return 1

    print("SMOKE OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
