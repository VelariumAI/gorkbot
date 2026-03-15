"""
NotebookLM MCP Server — Gorkbot Integration
============================================
A fully-functional local notebook system backed by the Gemini AI API.

Notebooks are stored as JSON files in $GORKBOT_CONFIG_DIR/notebooks/.
Each notebook holds a list of "sources" (text snippets, web URLs, or Google
Drive documents) that form the knowledge base for AI-powered queries.

Environment variables (injected by the Go orchestrator):
  GEMINI_API_KEY          — Required for all AI operations.
  GOOGLE_ACCESS_TOKEN     — Optional; required only for Google Drive sources.
  GORKBOT_CONFIG_DIR      — Config directory for notebook storage.
                            Defaults to ~/.config/gorkbot if unset.

Auth-Required Protocol
----------------------
When a prerequisite credential is missing, tools return a structured error
text beginning with the sentinel [GORKBOT_AUTH_REQUIRED] followed by a JSON
blob.  The Go layer in orchestrator/mcp_client.go detects this prefix and
routes the payload to the TUI auth prompt system.

Blocking I/O
------------
All blocking network / genai calls (_fetch_url, _fetch_gdrive_file,
_query_gemini) are wrapped with asyncio.to_thread() so they run in a thread
pool and never stall the FastMCP event loop.

Tools exposed:
  auth_status         — Report configuration / credential state.
  list_notebooks      — List all local notebooks.
  create_notebook     — Create a new notebook with a title.
  delete_notebook     — Delete a notebook and all its sources.
  add_source          — Add a text, URL, or Google Drive source to a notebook.
  list_sources        — List sources attached to a notebook.
  remove_source       — Remove a specific source from a notebook.
  chat_with_notebook  — Ask a question answered from the notebook's sources.
"""

import sys
import os
import json
import uuid
import signal
import asyncio
import logging
import urllib.request
import urllib.error
from datetime import datetime, timezone
from pathlib import Path

# ── Bootstrap ─────────────────────────────────────────────────────────────────
# All logging goes to stderr so it never corrupts the JSON-RPC stdout stream.
logging.basicConfig(level=logging.INFO, stream=sys.stderr,
                    format="%(name)s [%(levelname)s] %(message)s")
logger = logging.getLogger("NotebookLM-MCP")

# Lazy-import google.genai — only used in chat_with_notebook.
# Avoids a hard crash at server start if the package isn't installed yet.
_genai = None


def _load_genai():
    """Lazily import and return the google.genai module (google-genai package)."""
    global _genai
    if _genai is not None:
        return _genai
    try:
        from google import genai  # type: ignore  # pip install google-genai
        _genai = genai
        return _genai
    except ImportError:
        return None


# ── FastMCP ────────────────────────────────────────────────────────────────────
from mcp.server.fastmcp import FastMCP  # type: ignore

mcp = FastMCP("NotebookLM")

# ── Configuration ─────────────────────────────────────────────────────────────

def _config_dir() -> Path:
    """Resolves the Gorkbot config directory from the environment."""
    env_dir = os.environ.get("GORKBOT_CONFIG_DIR", "")
    if env_dir:
        return Path(env_dir)
    xdg = os.environ.get("XDG_CONFIG_HOME", "")
    if xdg:
        return Path(xdg) / "gorkbot"
    return Path.home() / ".config" / "gorkbot"


def _notebooks_dir() -> Path:
    """Returns (and ensures) the notebooks storage directory."""
    d = _config_dir() / "notebooks"
    d.mkdir(parents=True, exist_ok=True)
    return d


def _gemini_api_key() -> str:
    return os.environ.get("GEMINI_API_KEY", "")


def _google_access_token() -> str:
    return os.environ.get("GOOGLE_ACCESS_TOKEN", "")


def _auth_required(auth_type: str, message: str) -> str:
    """Returns the sentinel string that the Go layer intercepts."""
    payload = json.dumps({"type": auth_type, "message": message})
    return f"[GORKBOT_AUTH_REQUIRED] {payload}"


# ── Notebook persistence helpers ───────────────────────────────────────────────

def _notebook_path(notebook_id: str) -> Path:
    return _notebooks_dir() / f"{notebook_id}.json"


def _load_notebook(notebook_id: str) -> dict | None:
    p = _notebook_path(notebook_id)
    if not p.exists():
        return None
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except Exception as exc:
        logger.error("Failed to load notebook %s: %s", notebook_id, exc)
        return None


def _save_notebook(nb: dict) -> None:
    nb["updated_at"] = _now()
    p = _notebook_path(nb["id"])
    p.write_text(json.dumps(nb, indent=2, ensure_ascii=False), encoding="utf-8")


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _all_notebooks() -> list[dict]:
    nbs = []
    for p in sorted(_notebooks_dir().glob("*.json")):
        try:
            nbs.append(json.loads(p.read_text(encoding="utf-8")))
        except Exception:
            pass
    return nbs


# ── Blocking I/O helpers (wrapped in asyncio.to_thread at call sites) ──────────

def _fetch_url(url: str, max_bytes: int = 256_000) -> str:
    """Fetches plain text from a URL. Returns empty string on failure."""
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "Gorkbot-NotebookLM/1.0"})
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read(max_bytes)
            charset = resp.headers.get_content_charset("utf-8") or "utf-8"
            text = raw.decode(charset, errors="replace")
            import re
            text = re.sub(r"<[^>]+>", " ", text)
            text = re.sub(r"\s{3,}", "\n\n", text)
            return text.strip()
    except Exception as exc:
        logger.warning("URL fetch failed for %s: %s", url, exc)
        return ""


def _fetch_gdrive_file(file_id: str, token: str) -> str:
    """Downloads a Google Drive file as plain text (requires OAuth token)."""
    export_url = (
        f"https://www.googleapis.com/drive/v3/files/{file_id}"
        f"/export?mimeType=text/plain"
    )
    download_url = (
        f"https://www.googleapis.com/drive/v3/files/{file_id}?alt=media"
    )
    headers = {"Authorization": f"Bearer {token}"}

    def _try(url: str) -> str:
        try:
            req = urllib.request.Request(url, headers=headers)
            with urllib.request.urlopen(req, timeout=30) as resp:
                return resp.read(512_000).decode("utf-8", errors="replace")
        except urllib.error.HTTPError as exc:
            logger.debug("GDrive fetch failed (%s) for %s: %d", url, file_id, exc.code)
            return ""
        except Exception as exc:
            logger.warning("GDrive fetch error for %s: %s", file_id, exc)
            return ""

    text = _try(export_url)
    if not text:
        text = _try(download_url)
    return text


def _query_gemini(context: str, query: str, api_key: str) -> str:
    """Sends a grounded query to Gemini and returns the response text."""
    genai = _load_genai()
    if genai is None:
        return (
            "⚠️  google-genai package not installed.\n"
            "Install it with:  pip install google-genai"
        )

    system_instruction = (
        "You are a precise research assistant. Answer the user's question "
        "using ONLY the provided source documents. If the documents do not "
        "contain enough information, say so clearly. Do not invent facts."
    )
    prompt = (
        f"{system_instruction}\n\n"
        f"Source documents:\n\n{context}\n\n"
        f"---\n\nQuestion: {query}\n\n"
        "Answer based strictly on the sources above:"
    )

    try:
        gclient = genai.Client(api_key=api_key)
        response = gclient.models.generate_content(
            model="gemini-2.0-flash",
            contents=prompt,
        )
        return response.text
    except Exception as exc:
        logger.error("Gemini query failed: %s", exc)
        return f"❌ Gemini API error: {exc}"


def _build_context(sources: list[dict]) -> str:
    """Concatenates source content into a single prompt context block."""
    parts = []
    for s in sources:
        title = s.get("title") or s.get("source_type", "Source")
        content = s.get("content", "")
        if content:
            parts.append(f"=== {title} ===\n{content}")
    return "\n\n".join(parts)


# ── MCP Tools ──────────────────────────────────────────────────────────────────

@mcp.tool()
async def auth_status() -> str:
    """
    Reports the current authentication and configuration status for the
    NotebookLM integration. Use this to diagnose missing credentials.
    """
    lines = ["**NotebookLM Auth Status**\n"]

    api_key = _gemini_api_key()
    if api_key:
        masked = api_key[:8] + "…" + api_key[-4:] if len(api_key) > 12 else "***"
        lines.append(f"✅ Gemini API Key: configured ({masked})")
    else:
        lines.append("❌ Gemini API Key: NOT SET  →  add GEMINI_API_KEY to your .env")

    token = _google_access_token()
    if token:
        lines.append("✅ Google OAuth Token: present (enables Google Drive sources)")
    else:
        lines.append("⚠️  Google OAuth Token: not set  →  run `/auth notebooklm login`")

    nb_count = len(list(_notebooks_dir().glob("*.json")))
    lines.append(f"\n📓 Notebooks stored: {nb_count}")
    lines.append(f"📁 Storage directory: {_notebooks_dir()}")

    return "\n".join(lines)


@mcp.tool()
async def list_notebooks() -> str:
    """
    Lists all notebooks in the local workspace with their IDs, titles,
    source counts, and last-updated timestamps.
    """
    nbs = _all_notebooks()
    if not nbs:
        return (
            "📓 No notebooks yet.\n\n"
            "Create one with: create_notebook(title=\"My Research\")"
        )

    lines = [f"📓 **{len(nbs)} Notebook(s):**\n"]
    for nb in nbs:
        src_count = len(nb.get("sources", []))
        updated = nb.get("updated_at", "unknown")[:10]
        lines.append(
            f"• **{nb['title']}**  (ID: `{nb['id']}`)  "
            f"— {src_count} source(s)  — updated {updated}"
        )
    return "\n".join(lines)


@mcp.tool()
async def create_notebook(title: str) -> str:
    """
    Creates a new, empty notebook.

    Args:
        title: The display name for the notebook (e.g. "Architecture Goals").

    Returns:
        The new notebook's ID, which is used in all subsequent operations.
    """
    if not title or not title.strip():
        return "❌ Title cannot be empty."

    nb_id = f"nb-{uuid.uuid4().hex[:8]}"
    nb = {
        "id": nb_id,
        "title": title.strip(),
        "sources": [],
        "created_at": _now(),
        "updated_at": _now(),
    }
    _save_notebook(nb)
    logger.info("Created notebook %s: %s", nb_id, title)
    return (
        f"✅ Notebook created.\n\n"
        f"• **Title:** {title}\n"
        f"• **ID:** `{nb_id}`\n\n"
        f"Add sources with: add_source(notebook_id=\"{nb_id}\", ...)"
    )


@mcp.tool()
async def delete_notebook(notebook_id: str) -> str:
    """
    Permanently deletes a notebook and all its sources.

    Args:
        notebook_id: The ID returned by create_notebook or list_notebooks.
    """
    nb = _load_notebook(notebook_id)
    if nb is None:
        return f"❌ Notebook not found: `{notebook_id}`"

    _notebook_path(notebook_id).unlink(missing_ok=True)
    logger.info("Deleted notebook %s", notebook_id)
    return f"🗑️  Deleted notebook **{nb['title']}** (`{notebook_id}`)."


@mcp.tool()
async def add_source(
    notebook_id: str,
    source_type: str,
    content: str,
    title: str = "",
) -> str:
    """
    Adds a knowledge source to a notebook.

    Args:
        notebook_id:  The target notebook's ID.
        source_type:  One of "text" | "url" | "gdrive_id".
                      • "text"     — paste raw text directly.
                      • "url"      — web page URL (fetched and cached).
                      • "gdrive_id"— Google Drive file ID (requires OAuth token).
        content:      For "text": the content itself.
                      For "url": the full URL (https://...).
                      For "gdrive_id": the file's Drive ID from the URL.
        title:        Optional human-readable label shown in source lists.
    """
    nb = _load_notebook(notebook_id)
    if nb is None:
        return f"❌ Notebook not found: `{notebook_id}`"

    source_type = source_type.strip().lower()
    src_id = f"src-{uuid.uuid4().hex[:8]}"
    cached_content = ""

    if source_type == "text":
        if not content.strip():
            return "❌ Text content cannot be empty."
        cached_content = content.strip()
        display_title = title or f"Text snippet ({len(cached_content)} chars)"

    elif source_type == "url":
        if not content.startswith(("http://", "https://")):
            return "❌ URL must start with http:// or https://"
        logger.info("Fetching URL source: %s", content)
        # Offload blocking HTTP I/O to a thread pool.
        cached_content = await asyncio.to_thread(_fetch_url, content)
        if not cached_content:
            return (
                f"⚠️  Could not fetch content from {content}\n"
                "The URL may be inaccessible. Try 'text' type and paste the content directly."
            )
        display_title = title or content

    elif source_type == "gdrive_id":
        token = _google_access_token()
        if not token:
            return _auth_required(
                "google_oauth",
                "Google Drive sources require OAuth authentication. "
                "Run `/auth notebooklm login` to authenticate with Google.",
            )
        logger.info("Fetching GDrive file: %s", content)
        # Offload blocking HTTP I/O to a thread pool.
        cached_content = await asyncio.to_thread(_fetch_gdrive_file, content, token)
        if not cached_content:
            return (
                f"⚠️  Could not fetch Drive file `{content}`.\n"
                "Ensure the file ID is correct and your account has read access."
            )
        display_title = title or f"Drive file ({content})"

    else:
        return f"❌ Unknown source_type: '{source_type}'. Use 'text', 'url', or 'gdrive_id'."

    source = {
        "id": src_id,
        "source_type": source_type,
        "title": display_title,
        "content": cached_content,
        "added_at": _now(),
        "reference": content if source_type in ("url", "gdrive_id") else "",
    }
    nb["sources"].append(source)
    _save_notebook(nb)

    word_count = len(cached_content.split())
    logger.info("Added source %s to notebook %s (%d words)", src_id, notebook_id, word_count)
    return (
        f"✅ Source added to **{nb['title']}**.\n\n"
        f"• **ID:** `{src_id}`\n"
        f"• **Type:** {source_type}\n"
        f"• **Title:** {display_title}\n"
        f"• **Content:** {word_count:,} words cached\n"
    )


@mcp.tool()
async def list_sources(notebook_id: str) -> str:
    """
    Lists all sources attached to a notebook.

    Args:
        notebook_id: The notebook's ID.
    """
    nb = _load_notebook(notebook_id)
    if nb is None:
        return f"❌ Notebook not found: `{notebook_id}`"

    sources = nb.get("sources", [])
    if not sources:
        return (
            f"📓 Notebook **{nb['title']}** has no sources yet.\n\n"
            "Add sources with: add_source(notebook_id=..., source_type=..., content=...)"
        )

    lines = [f"📚 Sources in **{nb['title']}** ({len(sources)} total):\n"]
    for s in sources:
        word_count = len(s.get("content", "").split())
        ref = f" — {s['reference']}" if s.get("reference") else ""
        lines.append(
            f"• [{s['source_type'].upper()}] **{s['title']}**  "
            f"(ID: `{s['id']}`, {word_count:,} words){ref}"
        )
    return "\n".join(lines)


@mcp.tool()
async def remove_source(notebook_id: str, source_id: str) -> str:
    """
    Removes a source from a notebook.

    Args:
        notebook_id: The notebook's ID.
        source_id:   The source's ID (from list_sources).
    """
    nb = _load_notebook(notebook_id)
    if nb is None:
        return f"❌ Notebook not found: `{notebook_id}`"

    before = len(nb["sources"])
    nb["sources"] = [s for s in nb["sources"] if s["id"] != source_id]
    if len(nb["sources"]) == before:
        return f"❌ Source `{source_id}` not found in notebook `{notebook_id}`."

    _save_notebook(nb)
    return f"🗑️  Removed source `{source_id}` from **{nb['title']}**."


@mcp.tool()
async def chat_with_notebook(notebook_id: str, query: str) -> str:
    """
    Queries a notebook using the Gemini AI, grounded strictly in the
    notebook's sources. The model will not invent facts beyond the sources.

    Args:
        notebook_id: The notebook to query.
        query:       Your question or research prompt.
    """
    api_key = _gemini_api_key()
    if not api_key:
        return _auth_required(
            "gemini_api_key",
            "NotebookLM AI queries require a Gemini API key. "
            "Add GEMINI_API_KEY=your_key to your .env file and restart Gorkbot.",
        )

    nb = _load_notebook(notebook_id)
    if nb is None:
        return f"❌ Notebook not found: `{notebook_id}`"

    sources = nb.get("sources", [])
    if not sources:
        return (
            f"⚠️  Notebook **{nb['title']}** has no sources.\n\n"
            "Add sources with add_source() before querying."
        )

    if not query.strip():
        return "❌ Query cannot be empty."

    context = _build_context(sources)
    total_words = len(context.split())
    logger.info(
        "Querying notebook %s (%d sources, %d words) for: %s",
        notebook_id, len(sources), total_words, query[:80],
    )

    # Offload the blocking Gemini API call to a thread pool so we never stall
    # the FastMCP asyncio event loop.
    answer = await asyncio.to_thread(_query_gemini, context, query, api_key)

    source_list = ", ".join(s["title"] for s in sources[:5])
    if len(sources) > 5:
        source_list += f" +{len(sources) - 5} more"

    return (
        f"💡 **{nb['title']}** — AI Response\n\n"
        f"{answer}\n\n"
        f"---\n*Sources: {source_list}*"
    )


# ── Entry point ────────────────────────────────────────────────────────────────

def _install_sigterm_handler() -> None:
    """Install a clean SIGTERM handler so the FastMCP loop shuts down gracefully."""
    def _on_sigterm(signum, frame):
        logger.info("SIGTERM received, shutting down.")
        sys.exit(0)

    try:
        signal.signal(signal.SIGTERM, _on_sigterm)
    except (OSError, ValueError):
        # signal handling unavailable in this context (e.g. non-main thread)
        pass


if __name__ == "__main__":
    _install_sigterm_handler()
    logger.info(
        "NotebookLM MCP server starting  |  notebooks_dir=%s  |  gemini_key=%s",
        _notebooks_dir(),
        "configured" if _gemini_api_key() else "MISSING",
    )
    try:
        mcp.run()
    except EOFError:
        logger.info("stdin closed, shutting down.")
        sys.exit(0)
    except KeyboardInterrupt:
        logger.info("Interrupted, shutting down.")
        sys.exit(0)
    except BrokenPipeError:
        # Parent process closed the pipe — clean exit, not an error.
        logger.info("Broken pipe, shutting down.")
        sys.exit(0)
    except Exception as exc:
        logger.critical("Critical server failure: %s", exc)
        sys.exit(1)
