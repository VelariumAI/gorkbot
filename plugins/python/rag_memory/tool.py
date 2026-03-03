#!/usr/bin/env python3
"""
RAG Memory Plugin — ChromaDB Knowledge Microservice

Provides semantic vector search over stored engrams using:
  - ChromaDB as the persistent vector store
  - all-MiniLM-L6-v2 (sentence-transformers) for embeddings

Actions: store | search | stats | purge

Communication: JSON over stdin/stdout (Gorkbot bridge protocol)
"""

import json
import os
import sys
import uuid
import time
from dataclasses import dataclass
from typing import Any, Dict, List, Optional

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

MAX_ENGRAMS = 10_000
COLLECTION_NAME = "gorkbot_engrams"
PERSIST_DIR = os.path.join(
    os.environ.get("GORKBOT_CONFIG_DIR", os.path.expanduser("~/.config/gorkbot")),
    "rag_memory",
)

# ---------------------------------------------------------------------------
# Minimal ToolResult (self-contained — no gorkbot_bridge import needed)
# ---------------------------------------------------------------------------

@dataclass
class ToolResult:
    success: bool
    output: str = ""
    error: str = ""
    data: Optional[Dict[str, Any]] = None

    def to_dict(self) -> Dict[str, Any]:
        result: Dict[str, Any] = {"success": self.success, "output": self.output}
        if self.error:
            result["error"] = self.error
        if self.data:
            result["data"] = self.data
        return result


# ---------------------------------------------------------------------------
# Module-level singletons (lazy-loaded)
# ---------------------------------------------------------------------------

_model: Any = None       # SentenceTransformer — loaded on first encode
_client: Any = None      # chromadb.PersistentClient
_collection: Any = None  # chroma Collection


def _get_model() -> Any:
    """Load the embedding model once; reuse on subsequent calls."""
    global _model
    if _model is None:
        from sentence_transformers import SentenceTransformer  # type: ignore
        _model = SentenceTransformer("all-MiniLM-L6-v2")
    return _model


def _get_collection() -> Any:
    """Initialise ChromaDB client and collection once; reuse on subsequent calls."""
    global _client, _collection
    if _collection is None:
        import chromadb  # type: ignore
        os.makedirs(PERSIST_DIR, exist_ok=True)
        _client = chromadb.PersistentClient(path=PERSIST_DIR)
        _collection = _client.get_or_create_collection(
            name=COLLECTION_NAME,
            metadata={"hnsw:space": "cosine"},
        )
    return _collection


# ---------------------------------------------------------------------------
# Rolling window — keeps engram count below MAX_ENGRAMS
# ---------------------------------------------------------------------------

def _rolling_window(col: Any) -> int:
    """Delete oldest 10 % of engrams when the collection is at capacity.

    Returns the number of engrams deleted.
    """
    count = col.count()
    if count < MAX_ENGRAMS:
        return 0

    # Fetch all ids + timestamps so we can sort by age
    all_items = col.get(include=["metadatas"])
    ids: List[str] = all_items["ids"]
    metas: List[Dict] = all_items["metadatas"]

    # Pair ids with their timestamps (default 0 if missing)
    pairs = [(ids[i], float(metas[i].get("ts", 0))) for i in range(len(ids))]
    pairs.sort(key=lambda x: x[1])  # oldest first

    n_delete = max(1, count // 10)
    to_delete = [p[0] for p in pairs[:n_delete]]
    col.delete(ids=to_delete)
    return len(to_delete)


# ---------------------------------------------------------------------------
# Actions
# ---------------------------------------------------------------------------

def store(content: str, metadata_json: str) -> ToolResult:
    """Embed *content* and persist it in ChromaDB."""
    if not content.strip():
        return ToolResult(success=False, error="content must not be empty")

    # Parse optional user metadata
    try:
        user_meta: Dict[str, Any] = json.loads(metadata_json) if metadata_json.strip() else {}
    except json.JSONDecodeError as exc:
        return ToolResult(success=False, error=f"Invalid metadata JSON: {exc}")

    model = _get_model()
    col = _get_collection()

    # Compute embedding
    vec: List[float] = model.encode(content).tolist()

    # Enforce rolling window before inserting
    _rolling_window(col)

    engram_id = str(uuid.uuid4())
    meta: Dict[str, Any] = {"ts": time.time(), "source": "gorkbot", **user_meta}

    col.add(
        ids=[engram_id],
        embeddings=[vec],
        documents=[content],
        metadatas=[meta],
    )

    return ToolResult(
        success=True,
        output=f"Stored engram {engram_id[:8]}...",
        data={"id": engram_id},
    )


def search(query: str, n_results: int, min_score: float) -> ToolResult:
    """Semantic search over stored engrams."""
    if not query.strip():
        return ToolResult(success=False, error="query must not be empty")

    model = _get_model()
    col = _get_collection()

    total = col.count()
    if total == 0:
        return ToolResult(success=True, output="No engrams stored yet.", data={"results": []})

    # Clamp n_results to what's available
    effective_n = min(n_results, total)

    vec: List[float] = model.encode(query).tolist()
    raw = col.query(
        query_embeddings=[vec],
        n_results=effective_n,
        include=["documents", "metadatas", "distances"],
    )

    docs: List[str] = raw["documents"][0]
    metas: List[Dict] = raw["metadatas"][0]
    distances: List[float] = raw["distances"][0]

    results = []
    lines: List[str] = []
    for i, (doc, meta, dist) in enumerate(zip(docs, metas, distances)):
        score = 1.0 - dist  # cosine distance → similarity
        if score < min_score:
            continue
        results.append({"rank": i + 1, "score": round(score, 4), "content": doc, "metadata": meta})
        lines.append(f"[{i+1}] score={score:.4f}\n    {doc[:200]}")

    if not results:
        return ToolResult(
            success=True,
            output=f"No results above min_score={min_score}.",
            data={"results": []},
        )

    return ToolResult(
        success=True,
        output="\n".join(lines),
        data={"results": results, "total_engrams": total},
    )


def stats() -> ToolResult:
    """Return collection statistics without loading the embedding model."""
    col = _get_collection()
    count = col.count()

    # RAM estimate: count × 384 dims × 4 bytes/float + ~100 MB model base
    embedding_mb = (count * 384 * 4) / (1024 * 1024)
    model_base_mb = 100.0
    total_mb = embedding_mb + model_base_mb

    output = (
        f"RAG Memory Stats\n"
        f"  Engrams stored : {count:,} / {MAX_ENGRAMS:,}\n"
        f"  Persist dir    : {PERSIST_DIR}\n"
        f"  RAM estimate   : {total_mb:.1f} MB  "
        f"(embeddings {embedding_mb:.1f} MB + model ~{model_base_mb:.0f} MB)"
    )

    return ToolResult(
        success=True,
        output=output,
        data={
            "count": count,
            "max_engrams": MAX_ENGRAMS,
            "persist_dir": PERSIST_DIR,
            "estimated_ram_mb": round(total_mb, 1),
        },
    )


def purge() -> ToolResult:
    """Force a rolling-window pass, deleting oldest engrams if over capacity."""
    col = _get_collection()
    before = col.count()
    deleted = _rolling_window(col)
    after = col.count()

    if deleted == 0:
        return ToolResult(
            success=True,
            output=f"No purge needed — {before:,} engrams (below {MAX_ENGRAMS:,} limit).",
            data={"deleted": 0, "remaining": before},
        )

    return ToolResult(
        success=True,
        output=f"Purged {deleted:,} oldest engrams. Remaining: {after:,}.",
        data={"deleted": deleted, "remaining": after},
    )


# ---------------------------------------------------------------------------
# Entry point — JSON over stdin/stdout bridge
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    payload: Dict[str, Any] = {}
    try:
        raw_in = sys.stdin.read()
        payload = json.loads(raw_in)
    except Exception as exc:
        print(json.dumps({"success": False, "error": f"Failed to parse input: {exc}"}))
        sys.exit(1)

    params = payload.get("params", {})
    action = params.get("action", "")

    try:
        if action == "store":
            result = store(
                params.get("content", ""),
                params.get("metadata", "{}"),
            )
        elif action == "search":
            result = search(
                params.get("query", ""),
                int(params.get("n_results", 5)),
                float(params.get("min_score", 0.0)),
            )
        elif action == "stats":
            result = stats()
        elif action == "purge":
            result = purge()
        else:
            result = ToolResult(success=False, error=f"Unknown action: '{action}'. Use store | search | stats | purge.")
    except Exception as exc:
        result = ToolResult(success=False, error=str(exc))

    print(json.dumps(result.to_dict()))
