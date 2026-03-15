"""
NotebookLM Shaper — Gorkbot MCP Pre/Post Processor
====================================================
Invoked as a short-lived subprocess by the Go orchestrator (mcp_client.go).

stdin:  JSON object {"action": "shape"|"parse", "query": str, "result": str}
stdout: JSON object {"output": str, "error": str}
stderr: logging only

All output is flushed in the `finally` block to guarantee delivery even when
the parent process terminates before the child can flush.
"""

import sys
import json
import logging

logging.basicConfig(level=logging.INFO, stream=sys.stderr,
                    format="%(name)s [%(levelname)s] %(message)s")
logger = logging.getLogger("NotebookLM-Shaper")

# ── Shaping logic ──────────────────────────────────────────────────────────────

def shape_query(data: dict) -> str:
    """
    Refines a raw query into a well-formed question suitable for NotebookLM.
    Applied to the 'query' argument before the tool call is dispatched.
    """
    query = data.get("query", "")
    if not isinstance(query, str):
        query = str(query)
    query = query.strip()
    if not query:
        raise ValueError("Empty query provided to shaper.")

    # Enforce interrogative phrasing for better notebook retrieval.
    interrogatives = ("how", "what", "why", "when", "where", "which",
                      "describe", "list", "explain", "summarise", "summarize")
    if not any(query.lower().startswith(w) for w in interrogatives):
        query = f"Describe {query}"

    if not query.endswith("?"):
        query += "?"

    logger.debug("Shaped query: %r", query)
    return query


def parse_result(data: dict) -> str:
    """
    Post-processes a raw NotebookLM result for high-fidelity TUI output.
    Applied to the first text content block of the tool response.
    """
    result = data.get("result", "")
    if not isinstance(result, str):
        result = str(result)
    result = result.strip()
    if not result:
        return "⚠️ [NotebookLM]: No data retrieved for this query."

    # Strip redundant emoji/artefacts that may appear in raw LLM output.
    clean = result.replace("🔍", "").strip()
    return f"💡 [NotebookLM Intelligence]: {clean}"


# ── Entry point ────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    output = {"output": "", "error": ""}
    try:
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            # Empty input is a no-op; return empty output.
            sys.stdout.write(json.dumps(output))
            sys.exit(0)

        try:
            data = json.loads(raw_input)
        except json.JSONDecodeError as exc:
            output["error"] = f"Invalid JSON input: {exc}"
            sys.stdout.write(json.dumps(output))
            sys.exit(0)

        action = data.get("action", "")
        if action == "shape":
            output["output"] = shape_query(data)
        elif action == "parse":
            output["output"] = parse_result(data)
        else:
            output["error"] = f"Unknown action: {action!r}. Expected 'shape' or 'parse'."

    except Exception as exc:
        logger.exception("Shaper error")
        output["error"] = str(exc)
    finally:
        # Guaranteed flush even if the parent terminates before we exit.
        try:
            sys.stdout.write(json.dumps(output))
            sys.stdout.flush()
        except BrokenPipeError:
            pass
