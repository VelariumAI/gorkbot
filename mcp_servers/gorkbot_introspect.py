#!/usr/bin/env python3
"""
gorkbot_introspect — MCP server exposing Gorkbot's audit DB, vector store,
tool analytics, and session state as first-class MCP tools.

Lets Gork query its own operational history, error patterns, and memory
without needing to go through the bash tool or Go tooling.

Run via:  python3 gorkbot_introspect.py
Add to mcp.json:
  {"name": "gorkbot-introspect", "transport": "stdio",
   "command": "python3",
   "args": ["/data/data/com.termux/files/home/project/gorky/mcp_servers/gorkbot_introspect.py"]}
"""

import json
import os
import sqlite3
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any

from fastmcp import FastMCP

CONFIG_DIR = Path(os.environ.get(
    "GORKBOT_CONFIG_DIR",
    Path.home() / ".config" / "gorkbot"
))
AUDIT_DB   = CONFIG_DIR / "audit.db"
GORKBOT_DB = CONFIG_DIR / "gorkbot.db"
ANALYTICS  = CONFIG_DIR / "tool_analytics.json"

mcp = FastMCP("gorkbot-introspect")


# ── Helpers ─────────────────────────────────────────────────────────────────

def _audit_conn():
    c = sqlite3.connect(str(AUDIT_DB))
    c.row_factory = sqlite3.Row
    return c

def _gorkbot_conn():
    c = sqlite3.connect(str(GORKBOT_DB))
    c.row_factory = sqlite3.Row
    return c


# ── Audit tools ──────────────────────────────────────────────────────────────

@mcp.tool()
def audit_summary(limit: int = 25) -> str:
    """
    Return a summary of the most-used tools from the audit log,
    including success rates and average execution times.
    """
    if not AUDIT_DB.exists():
        return "Audit DB not found at " + str(AUDIT_DB)
    with _audit_conn() as c:
        rows = c.execute("""
            SELECT tool_name,
                   COUNT(*)                                        AS total,
                   SUM(CASE WHEN success THEN 1 ELSE 0 END)       AS ok,
                   AVG(execution_ms)                               AS avg_ms,
                   MAX(timestamp)                                  AS last_used
            FROM   tool_audit_log
            GROUP  BY tool_name
            ORDER  BY total DESC
            LIMIT  ?
        """, (limit,)).fetchall()
    if not rows:
        return "No audit records found."
    lines = ["| Tool | Total | OK | Fail | Avg ms | Last used |",
             "|------|-------|----|------|--------|-----------|"]
    for r in rows:
        fail = r["total"] - r["ok"]
        lines.append(
            f"| {r['tool_name']} | {r['total']} | {r['ok']} | {fail} "
            f"| {r['avg_ms']:.0f} | {r['last_used']} |"
        )
    return "\n".join(lines)


@mcp.tool()
def audit_errors(hours: int = 24, tool_filter: str = "") -> str:
    """
    List recent tool execution errors from the audit log.
    Optionally filter by tool name substring.
    """
    if not AUDIT_DB.exists():
        return "Audit DB not found."
    since = (datetime.utcnow() - timedelta(hours=hours)).strftime("%Y-%m-%d %H:%M:%S")
    with _audit_conn() as c:
        if tool_filter:
            rows = c.execute("""
                SELECT tool_name, error_category, raw_error, timestamp
                FROM   tool_audit_log
                WHERE  success = 0 AND timestamp >= ? AND tool_name LIKE ?
                ORDER  BY timestamp DESC LIMIT 50
            """, (since, f"%{tool_filter}%")).fetchall()
        else:
            rows = c.execute("""
                SELECT tool_name, error_category, raw_error, timestamp
                FROM   tool_audit_log
                WHERE  success = 0 AND timestamp >= ?
                ORDER  BY timestamp DESC LIMIT 50
            """, (since,)).fetchall()
    if not rows:
        return f"No errors in the last {hours}h" + (f" for '{tool_filter}'" if tool_filter else "") + "."
    lines = [f"## Tool Errors — last {hours}h\n"]
    for r in rows:
        lines.append(f"**{r['tool_name']}** [{r['error_category']}] @ {r['timestamp']}")
        if r["raw_error"]:
            lines.append(f"  > {r['raw_error'][:200]}")
    return "\n".join(lines)


@mcp.tool()
def audit_error_rate(hours: int = 24) -> str:
    """
    Return overall tool success/failure rate for the given time window.
    """
    if not AUDIT_DB.exists():
        return "Audit DB not found."
    since = (datetime.utcnow() - timedelta(hours=hours)).strftime("%Y-%m-%d %H:%M:%S")
    with _audit_conn() as c:
        row = c.execute("""
            SELECT COUNT(*) AS total,
                   SUM(CASE WHEN success THEN 1 ELSE 0 END) AS ok
            FROM   tool_audit_log
            WHERE  timestamp >= ?
        """, (since,)).fetchone()
    total, ok = row["total"], row["ok"]
    if total == 0:
        return f"No executions recorded in the last {hours}h."
    fail = total - ok
    rate = (fail / total) * 100
    return (f"**Last {hours}h:** {total} total executions — "
            f"{ok} OK, {fail} failed ({rate:.1f}% error rate)")


# ── Conversation / memory tools ──────────────────────────────────────────────

@mcp.tool()
def recent_turns(limit: int = 10, role_filter: str = "") -> str:
    """
    Return recent conversation turns from persistent storage.
    role_filter: 'user', 'assistant', 'system', or empty for all.
    """
    if not GORKBOT_DB.exists():
        return "Gorkbot DB not found."
    with _gorkbot_conn() as c:
        if role_filter:
            rows = c.execute("""
                SELECT role, substr(content, 1, 500) AS snippet, created_at
                FROM conversations WHERE role = ?
                ORDER BY created_at DESC LIMIT ?
            """, (role_filter, limit)).fetchall()
        else:
            rows = c.execute("""
                SELECT role, substr(content, 1, 500) AS snippet, created_at
                FROM conversations
                ORDER BY created_at DESC LIMIT ?
            """, (limit,)).fetchall()
    if not rows:
        return "No conversation history found."
    lines = [f"## Last {limit} turns\n"]
    for r in rows:
        lines.append(f"**[{r['role']}]** @ {r['created_at']}")
        lines.append(r["snippet"].replace("\n", " ") + "\n")
    return "\n".join(lines)


@mcp.tool()
def search_conversation(query: str, limit: int = 10) -> str:
    """
    Full-text search across conversation history using SQLite LIKE.
    """
    if not GORKBOT_DB.exists():
        return "Gorkbot DB not found."
    with _gorkbot_conn() as c:
        rows = c.execute("""
            SELECT role, substr(content, 1, 600) AS snippet, created_at
            FROM conversations
            WHERE content LIKE ?
            ORDER BY created_at DESC LIMIT ?
        """, (f"%{query}%", limit)).fetchall()
    if not rows:
        return f"No results for '{query}'."
    lines = [f"## Search results for '{query}'\n"]
    for r in rows:
        lines.append(f"**[{r['role']}]** @ {r['created_at']}")
        lines.append(r["snippet"].replace("\n", " ") + "\n")
    return "\n".join(lines)


@mcp.tool()
def list_cached_tools(limit: int = 20) -> str:
    """
    Show recently cached tool results from the tool result cache.
    """
    if not GORKBOT_DB.exists():
        return "Gorkbot DB not found."
    with _gorkbot_conn() as c:
        rows = c.execute("""
            SELECT tool_name, cache_key, substr(result, 1, 200) AS preview,
                   created_at, expires_at
            FROM cache_tool_results
            ORDER BY created_at DESC LIMIT ?
        """, (limit,)).fetchall()
    if not rows:
        return "No cached tool results."
    lines = ["## Cached tool results\n",
             "| Tool | Key | Preview | Expires |",
             "|------|-----|---------|---------|"]
    for r in rows:
        lines.append(
            f"| {r['tool_name']} | {r['cache_key'][:20]}… | "
            f"{r['preview'][:60].replace(chr(10),' ')}… | {r['expires_at']} |"
        )
    return "\n".join(lines)


# ── Analytics tools ──────────────────────────────────────────────────────────

@mcp.tool()
def tool_analytics_summary() -> str:
    """
    Return a summary from the in-memory tool analytics JSON file,
    including call counts, latencies, and category breakdown.
    """
    if not ANALYTICS.exists():
        return "Analytics file not found at " + str(ANALYTICS)
    with open(ANALYTICS) as f:
        data: dict[str, Any] = json.load(f)
    lines = ["## Tool Analytics\n"]
    # The analytics file structure depends on Gorkbot's analyticsManager
    # Try to be generic — just format top-level keys
    for key, val in list(data.items())[:30]:
        if isinstance(val, dict):
            lines.append(f"**{key}**: {json.dumps(val)[:200]}")
        else:
            lines.append(f"**{key}**: {val}")
    return "\n".join(lines)


# ── System state ─────────────────────────────────────────────────────────────

@mcp.tool()
def gorkbot_config_files() -> str:
    """
    List all files in ~/.config/gorkbot with sizes and modification times.
    """
    files = sorted(CONFIG_DIR.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True)
    lines = ["## Gorkbot config files\n",
             "| File | Size | Modified |",
             "|------|------|----------|"]
    for f in files:
        if f.is_file():
            st = f.stat()
            size = f"{st.st_size:,} B"
            mtime = datetime.fromtimestamp(st.st_mtime).strftime("%Y-%m-%d %H:%M")
            lines.append(f"| {f.name} | {size} | {mtime} |")
    return "\n".join(lines)


@mcp.tool()
def read_gorkbot_config(filename: str) -> str:
    """
    Read a specific Gorkbot config file by name (e.g. 'app_state.json',
    'tool_permissions.json', 'mcp.json'). Text files only.
    """
    target = CONFIG_DIR / filename
    if not target.exists():
        return f"File '{filename}' not found in config dir."
    if target.stat().st_size > 100_000:
        return f"File '{filename}' is too large to return inline (>{100_000} bytes)."
    return target.read_text(errors="replace")


if __name__ == "__main__":
    mcp.run()
