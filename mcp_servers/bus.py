"""
bus.py — Bidirectional IPC bus for Gorkbot MCP servers.

Architecture
============
Two peers share a Unix domain socket.  The first peer to start binds the
socket and becomes the *listener*.  The second peer connects as a *client*.
Once the connection is established both sides are fully symmetric — either
can call tools on the other or broadcast events.

Protocol
--------
Newline-delimited JSON messages (framing via '\\n').

  call    {"id": str, "type": "call",   "src": str, "tool": str, "args": dict}
  result  {"id": str, "type": "result", "src": str, "result": str}
  error   {"id": str, "type": "error",  "src": str, "error": str}
  event   {           "type": "event",  "src": str, "name": str, "data": dict}
  ping    {           "type": "ping",   "src": str}
  pong    {           "type": "pong",   "src": str}

Usage
-----
    from bus import Bus

    bus = Bus("termux")                  # "android" on the other side
    bus.register_tool("pkg_count", lambda _: "42")
    bus.start()                          # connects / listens in background

    result = bus.call_remote("pkg_install", {"packages": "git"}, timeout=30)
    bus.emit("package_installed", {"name": "git"})
    bus.on_event("battery_low", lambda data: print("WARN battery:", data))

    bus.stop()
"""

from __future__ import annotations

import json
import logging
import os
import socket
import threading
import time
import uuid
from pathlib import Path
from typing import Any, Callable, Optional

log = logging.getLogger("gorkbot.bus")

_SOCKET_PATH = Path(os.environ.get(
    "GORKBOT_BUS_SOCK",
    Path.home() / ".config" / "gorkbot" / "mcp_bus.sock",
))
_CONNECT_TIMEOUT = 5.0      # seconds to wait for remote to come up
_CALL_TIMEOUT    = 60.0     # default RPC timeout
_PING_INTERVAL   = 15.0     # seconds between keepalive pings
_MAX_MSG         = 8 * 1024 * 1024   # 8 MB message size cap


class _Pending:
    """Tracks an in-flight call waiting for its result."""
    __slots__ = ("event", "result", "error")

    def __init__(self) -> None:
        self.event:  threading.Event = threading.Event()
        self.result: Optional[str]   = None
        self.error:  Optional[str]   = None


class Bus:
    """Bidirectional peer-to-peer IPC bus over a Unix domain socket."""

    # ── Construction ──────────────────────────────────────────────────────────

    def __init__(self, name: str, socket_path: Optional[Path] = None) -> None:
        self.name         = name
        self._path        = socket_path or _SOCKET_PATH
        self._tools:  dict[str, Callable[[dict], str]] = {}
        self._events: dict[str, list[Callable[[dict], None]]] = {}
        self._pending: dict[str, _Pending] = {}
        self._conn:   Optional[socket.socket] = None
        self._server: Optional[socket.socket] = None   # only on listener
        self._role   = "disconnected"   # "listener" | "client" | "disconnected"
        self._lock   = threading.Lock()
        self._running = False
        self._event_log: list[dict] = []   # recent events (cap 100)

    # ── Public API ────────────────────────────────────────────────────────────

    def register_tool(self, name: str, fn: Callable[[dict], str]) -> None:
        """Register a callable that the remote peer can invoke."""
        self._tools[name] = fn

    def on_event(self, event_name: str, handler: Callable[[dict], None]) -> None:
        """Subscribe to events emitted by the remote peer."""
        self._events.setdefault(event_name, []).append(handler)

    def start(self) -> None:
        """Connect to (or become) the bus listener. Non-blocking after init."""
        self._running = True
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._elect()
        threading.Thread(target=self._ping_loop, daemon=True, name=f"bus-ping-{self.name}").start()

    def stop(self) -> None:
        """Tear down the bus connection cleanly."""
        self._running = False
        if self._conn:
            try: self._conn.close()
            except Exception: pass
        if self._server:
            try: self._server.close()
            except Exception: pass
            try: self._path.unlink(missing_ok=True)
            except Exception: pass

    def call_remote(self, tool: str, args: dict | None = None,
                    timeout: float = _CALL_TIMEOUT) -> str:
        """
        Invoke a tool on the remote peer and return its result string.
        Raises RuntimeError if disconnected or timed out.
        """
        if self._conn is None:
            raise RuntimeError("Bus not connected to peer")
        call_id = str(uuid.uuid4())
        pend    = _Pending()
        with self._lock:
            self._pending[call_id] = pend
        self._send({"id": call_id, "type": "call",
                    "src": self.name, "tool": tool, "args": args or {}})
        if not pend.event.wait(timeout):
            with self._lock:
                self._pending.pop(call_id, None)
            raise RuntimeError(f"call_remote('{tool}') timed out after {timeout}s")
        if pend.error:
            raise RuntimeError(pend.error)
        return pend.result or ""

    def emit(self, event_name: str, data: dict | None = None) -> None:
        """Fire-and-forget event broadcast to the remote peer."""
        self._send({"type": "event", "src": self.name,
                    "name": event_name, "data": data or {}})

    def is_connected(self) -> bool:
        return self._conn is not None

    @property
    def role(self) -> str:
        return self._role

    def recent_events(self, limit: int = 20) -> list[dict]:
        return self._event_log[-limit:]

    def list_local_tools(self) -> list[str]:
        return list(self._tools.keys())

    # ── Server election ───────────────────────────────────────────────────────

    def _elect(self) -> None:
        """Try to become listener; fall back to client if socket already exists."""
        # Check for and clean up a stale socket file
        if self._path.exists():
            try:
                test_conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                test_conn.connect(str(self._path))
                test_conn.close()
            except ConnectionRefusedError:
                try: self._path.unlink()
                except Exception: pass
            except OSError:
                pass

        try:
            srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            srv.bind(str(self._path))
            srv.listen(1)
            self._server = srv
            self._role   = "listener"
            log.info("Bus[%s]: listening on %s", self.name, self._path)
            threading.Thread(target=self._accept_loop, daemon=True,
                             name=f"bus-accept-{self.name}").start()
        except OSError:
            # Socket already exists — try connecting
            self._role = "client"
            threading.Thread(target=self._connect_loop, daemon=True,
                             name=f"bus-connect-{self.name}").start()

    def _accept_loop(self) -> None:
        while self._running and self._server:
            try:
                self._server.settimeout(2.0)
                conn, _ = self._server.accept()
                with self._lock:
                    if self._conn:
                        try: self._conn.close()
                        except Exception: pass
                    self._conn = conn
                log.info("Bus[%s]: peer connected", self.name)
                threading.Thread(target=self._read_loop, args=(conn,),
                                 daemon=True, name=f"bus-read-{self.name}").start()
            except (socket.timeout, OSError):
                pass

    def _connect_loop(self) -> None:
        deadline = time.time() + _CONNECT_TIMEOUT
        while self._running and time.time() < deadline:
            try:
                conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                conn.connect(str(self._path))
                with self._lock:
                    self._conn = conn
                log.info("Bus[%s]: connected to peer", self.name)
                threading.Thread(target=self._read_loop, args=(conn,),
                                 daemon=True, name=f"bus-read-{self.name}").start()
                return
            except OSError:
                time.sleep(0.3)
        log.warning("Bus[%s]: could not connect to peer within %.1fs", self.name, _CONNECT_TIMEOUT)

    # ── I/O ───────────────────────────────────────────────────────────────────

    def _send(self, msg: dict) -> None:
        conn = self._conn
        if not conn:
            return
        try:
            raw = (json.dumps(msg) + "\n").encode()
            with self._lock:
                conn.sendall(raw)
        except Exception as e:
            log.warning("Bus[%s]: send error: %s", self.name, e)
            self._handle_disconnect()

    def _read_loop(self, conn: socket.socket) -> None:
        buf = b""
        while self._running:
            try:
                chunk = conn.recv(65536)
                if not chunk:
                    break
                buf += chunk
                if len(buf) > _MAX_MSG:
                    log.error("Bus[%s]: message too large, resetting", self.name)
                    buf = b""
                while b"\n" in buf:
                    line, buf = buf.split(b"\n", 1)
                    if line.strip():
                        self._dispatch(json.loads(line))
            except Exception as e:
                if self._running:
                    log.warning("Bus[%s]: read error: %s", self.name, e)
                break
        self._handle_disconnect()

    def _handle_disconnect(self) -> None:
        with self._lock:
            if self._conn:
                try: self._conn.close()
                except Exception: pass
                self._conn = None
        # Fail any pending calls
        for pend in list(self._pending.values()):
            pend.error = "Peer disconnected"
            pend.event.set()
        self._pending.clear()
        # Re-elect if we were the client (listener keeps socket open)
        if self._role == "client" and self._running:
            threading.Thread(target=self._connect_loop, daemon=True,
                             name=f"bus-reconnect-{self.name}").start()

    # ── Dispatch ──────────────────────────────────────────────────────────────

    def _dispatch(self, msg: dict) -> None:
        mtype = msg.get("type", "")
        if mtype == "call":
            threading.Thread(target=self._handle_call, args=(msg,),
                             daemon=True).start()
        elif mtype == "result":
            self._resolve(msg.get("id", ""), result=msg.get("result", ""))
        elif mtype == "error":
            self._resolve(msg.get("id", ""), error=msg.get("error", "unknown error"))
        elif mtype == "event":
            self._handle_event(msg)
        elif mtype == "ping":
            self._send({"type": "pong", "src": self.name})
        elif mtype == "pong":
            pass   # keepalive acknowledged

    def _handle_call(self, msg: dict) -> None:
        call_id = msg.get("id", "")
        tool    = msg.get("tool", "")
        args    = msg.get("args", {}) or {}
        fn      = self._tools.get(tool)
        if fn is None:
            self._send({"id": call_id, "type": "error", "src": self.name,
                        "error": f"unknown tool '{tool}'"})
            return
        try:
            result = fn(args)
            self._send({"id": call_id, "type": "result", "src": self.name,
                        "result": str(result)})
        except Exception as e:
            self._send({"id": call_id, "type": "error", "src": self.name,
                        "error": str(e)})

    def _handle_event(self, msg: dict) -> None:
        name = msg.get("name", "")
        data = msg.get("data", {})
        entry = {"ts": time.strftime("%H:%M:%S"), "name": name,
                 "src": msg.get("src", "?"), "data": data}
        self._event_log.append(entry)
        if len(self._event_log) > 100:
            self._event_log = self._event_log[-100:]
        for handler in self._events.get(name, []):
            try:
                handler(data)
            except Exception as e:
                log.warning("Bus[%s]: event handler error for '%s': %s", self.name, name, e)

    def _resolve(self, call_id: str, result: str = "", error: str = "") -> None:
        with self._lock:
            pend = self._pending.pop(call_id, None)
        if pend:
            pend.result = result
            pend.error  = error or None
            pend.event.set()

    # ── Keepalive ─────────────────────────────────────────────────────────────

    def _ping_loop(self) -> None:
        while self._running:
            time.sleep(_PING_INTERVAL)
            if self._conn:
                self._send({"type": "ping", "src": self.name})
