#!/usr/bin/env python3
"""
gorkbot_termux — Full-scope Termux MCP server.

Covers:
  • Termux:API (all 20+ commands)   battery, camera, clipboard, contacts,
                                    dialog, fingerprint, IR, location,
                                    media, microphone, NFC, notification,
                                    sensors, share, SMS, storage, telephony,
                                    torch, TTS, USB, vibrate, volume,
                                    wallpaper, WiFi
  • Package management              pkg, pip, npm, go, cargo, gem
  • Development tools               bash/zsh/fish, make, cmake, git, compilers
  • Process / service control       ps, kill, termux-services, boot scripts
  • Networking                      curl, wget, nc, ssh, dig, nmap
  • File operations (enhanced)      find, archive, checksum, hex dump
  • Crypto / security               openssl, gpg, age, totp
  • Bus bridge                      call Android tools via bidirectional bus

Bidirectional bus: connects to gorkbot_android via Unix socket.
"""

import base64
import hashlib
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path
from typing import Optional

from fastmcp import FastMCP

# ── Bus integration ──────────────────────────────────────────────────────────
sys.path.insert(0, str(Path(__file__).parent))
from bus import Bus

_bus = Bus("termux")

def _reg_termux_bus_tools():
    """Register tools that Android can call on us via the bus."""
    _bus.register_tool("pkg_list",        lambda a: pkg_list(a.get("filter", "")))
    _bus.register_tool("pkg_install",     lambda a: pkg_install(a.get("packages",""), a.get("confirm", False)))
    _bus.register_tool("pip_list",        lambda a: pip_list(a.get("filter", "")))
    _bus.register_tool("run_bash",        lambda a: run_bash(a.get("script",""), a.get("timeout", 30)))
    _bus.register_tool("termux_battery",  lambda _: api_battery())
    _bus.register_tool("termux_wifi",     lambda _: api_wifi_info())
    _bus.register_tool("termux_location", lambda _: api_location())
    _bus.register_tool("port_list",       lambda _: port_check_all())
    _bus.register_tool("process_list",    lambda a: process_list(a.get("filter","")))
    _bus.register_tool("env_vars",        lambda a: env_vars(a.get("filter","")))
    _bus.register_tool("ping",            lambda _: "pong from termux")

_reg_termux_bus_tools()
_bus.on_event("battery_low",       lambda d: print(f"[bus-event] Battery low: {d}", file=sys.stderr))
_bus.on_event("apk_installed",     lambda d: print(f"[bus-event] APK installed: {d}", file=sys.stderr))
_bus.on_event("storage_full",      lambda d: print(f"[bus-event] Storage alert: {d}", file=sys.stderr))
_bus.start()

mcp = FastMCP("gorkbot-termux")

# ── Helpers ──────────────────────────────────────────────────────────────────

PREFIX = Path(os.environ.get("PREFIX", "/data/data/com.termux/files/usr"))

def _run(cmd: list[str], timeout: int = 30, cwd: Optional[str] = None,
         input_data: Optional[str] = None, env_extra: Optional[dict] = None) -> str:
    env = {**os.environ}
    if env_extra:
        env.update(env_extra)
    try:
        r = subprocess.run(
            cmd, capture_output=True, text=True,
            timeout=timeout, cwd=cwd, input=input_data, env=env,
        )
        out = r.stdout.strip()
        err = r.stderr.strip()
        if r.returncode != 0 and err and not out:
            return f"[exit {r.returncode}] {err[:1000]}"
        if r.returncode != 0 and err:
            out += f"\n[stderr] {err[:400]}"
        return out or f"(exit {r.returncode}, no output)"
    except subprocess.TimeoutExpired:
        return f"[timeout after {timeout}s]"
    except FileNotFoundError:
        return f"[not found: {cmd[0]}]"
    except Exception as e:
        return f"[error: {e}]"

def _tapi(cmd_args: list[str], timeout: int = 15) -> str:
    """Run a termux-* API command and return parsed or raw output."""
    cmd = [f"termux-{cmd_args[0]}"] + cmd_args[1:]
    return _run(cmd, timeout=timeout)


# ════════════════════════════════════════════════════════════════════════════
# TERMUX:API — DEVICE SENSORS & HARDWARE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def api_battery() -> str:
    """Battery status: percentage, health, plugged state, temperature."""
    return _tapi(["battery-status"])


@mcp.tool()
def api_camera_info() -> str:
    """List available cameras and their capabilities."""
    return _tapi(["camera-info"])


@mcp.tool()
def api_camera_photo(output_path: str = "/sdcard/photo.jpg",
                     camera_id: int = 0) -> str:
    """Take a photo. output_path: where to save. camera_id: 0=back, 1=front."""
    return _tapi(["camera-photo", "-c", str(camera_id), output_path])


@mcp.tool()
def api_location(provider: str = "gps", request: str = "once") -> str:
    """Get current GPS/network location. provider: gps|network|passive."""
    return _tapi(["location", "-p", provider, "-r", request], timeout=30)


@mcp.tool()
def api_sensor_list() -> str:
    """List all hardware sensors available on this device."""
    return _tapi(["sensor", "--list"])


@mcp.tool()
def api_sensor_read(sensors: str = "all", duration_ms: int = 500) -> str:
    """
    Read sensor values. sensors: comma-separated names or 'all'.
    duration_ms: how long to collect data (ms).
    """
    args = ["sensor", "-s", sensors, "-d", str(duration_ms), "-n", "1"]
    return _tapi(args, timeout=max(10, duration_ms // 1000 + 5))


@mcp.tool()
def api_torch(state: bool = True) -> str:
    """Turn flashlight on (True) or off (False)."""
    return _tapi(["torch", "on" if state else "off"])


@mcp.tool()
def api_vibrate(duration_ms: int = 500, force: bool = False) -> str:
    """Trigger device vibration. duration_ms: vibration length."""
    args = ["vibrate", "-d", str(duration_ms)]
    if force: args.append("-f")
    return _tapi(args)


@mcp.tool()
def api_volume(stream: str = "music", volume: int = -1) -> str:
    """
    Get or set volume. stream: music|call|ring|alarm|notification|system.
    volume: 0-15, -1 to query current.
    """
    if volume < 0:
        return _tapi(["volume"])
    return _tapi(["volume", "-s", stream, "-v", str(volume)])


@mcp.tool()
def api_infrared_frequencies() -> str:
    """List supported infrared carrier frequencies (IR blaster devices only)."""
    return _tapi(["infrared-frequencies"])


@mcp.tool()
def api_infrared_transmit(pattern: str, frequency: int = 38400) -> str:
    """Transmit an IR pattern. pattern: comma-separated pulse durations in µs."""
    return _tapi(["infrared-transmit", "-f", str(frequency), "-p", pattern])


# ════════════════════════════════════════════════════════════════════════════
# TERMUX:API — COMMUNICATION & INTERACTION
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def api_clipboard_get() -> str:
    """Read current clipboard content."""
    return _tapi(["clipboard-get"])


@mcp.tool()
def api_clipboard_set(text: str) -> str:
    """Set clipboard content to text."""
    r = subprocess.run(["termux-clipboard-set"], input=text,
                       capture_output=True, text=True, timeout=10)
    return "Set" if r.returncode == 0 else r.stderr.strip()


@mcp.tool()
def api_tts(text: str, language: str = "en", pitch: float = 1.0,
            rate: float = 1.0, stream: str = "MUSIC") -> str:
    """Text-to-speech. language: BCP 47 code. stream: MUSIC|RING|ALARM."""
    return _tapi(["tts-speak", "-l", language, "-p", str(pitch),
                  "-r", str(rate), "-s", stream, text], timeout=60)


@mcp.tool()
def api_tts_stop() -> str:
    """Stop any ongoing TTS playback."""
    return _tapi(["tts-stop"])


@mcp.tool()
def api_notification(title: str, content: str, notification_id: int = 1,
                     priority: str = "default",
                     action: str = "", ongoing: bool = False) -> str:
    """
    Show an Android notification. priority: min|low|default|high|max.
    action: shell command to run when notification is tapped.
    """
    args = ["notification", "-t", title, "-c", content,
            "--id", str(notification_id), "--priority", priority]
    if action:   args += ["--action", action]
    if ongoing:  args += ["--ongoing"]
    return _tapi(args)


@mcp.tool()
def api_notification_remove(notification_id: int) -> str:
    """Remove a notification by its ID."""
    return _tapi(["notification-remove", str(notification_id)])


@mcp.tool()
def api_notification_list() -> str:
    """List current Android notifications."""
    return _tapi(["notification-list"])


@mcp.tool()
def api_dialog(title: str = "", widget: str = "confirm",
               hint: str = "", values: str = "") -> str:
    """
    Show an interactive dialog to the user and return their input.
    widget: confirm|checkbox|counter|date|radio|sheet|spinner|speech|text|time.
    values: comma-separated options for radio/checkbox/sheet/spinner.
    """
    args = ["dialog", "-t", title, "-w", widget]
    if hint:   args += ["-i", hint]
    if values: args += ["-v", values]
    return _tapi(args, timeout=120)


@mcp.tool()
def api_share(file_path: str, action: str = "send",
              content_type: str = "", title: str = "") -> str:
    """Share a file via Android share sheet. action: send|view|edit."""
    args = ["share", "-a", action]
    if content_type: args += ["-t", content_type]
    if title:        args += ["-n", title]
    args.append(file_path)
    return _tapi(args)


@mcp.tool()
def api_open_url(url: str) -> str:
    """Open a URL in the default Android browser/app."""
    return _tapi(["open", url])


# ════════════════════════════════════════════════════════════════════════════
# TERMUX:API — COMMUNICATION (SMS, CONTACTS, TELEPHONY)
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def api_sms_list(sms_type: str = "inbox", offset: int = 0,
                 limit: int = 10) -> str:
    """
    List SMS messages. sms_type: inbox|sent|draft|outbox|all.
    Returns JSON array of messages.
    """
    return _tapi(["sms-list", "-t", sms_type, "-o", str(offset),
                  "-l", str(limit)], timeout=20)


@mcp.tool()
def api_sms_send(number: str, text: str) -> str:
    """Send an SMS message. number: phone number. text: message body."""
    return _tapi(["sms-send", "-n", number, text], timeout=30)


@mcp.tool()
def api_contacts_list(filter_str: str = "") -> str:
    """List device contacts. filter_str narrows by name."""
    out = _tapi(["contact-list"], timeout=20)
    if filter_str:
        try:
            contacts = json.loads(out)
            filtered = [c for c in contacts
                        if filter_str.lower() in json.dumps(c).lower()]
            return json.dumps(filtered, indent=2)
        except json.JSONDecodeError:
            return "\n".join(l for l in out.splitlines()
                             if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def api_telephony_info() -> str:
    """Device telephony info: IMEI (if available), carrier, network type."""
    return _tapi(["telephony-deviceinfo"])


@mcp.tool()
def api_telephony_call(number: str) -> str:
    """Initiate a phone call to number."""
    return _tapi(["telephony-call", number])


@mcp.tool()
def api_microphone_record(output_file: str = "/sdcard/recording.mp3",
                           duration_s: int = 5,
                           encoder: str = "aac") -> str:
    """
    Record from microphone. encoder: aac|amr_wb|amr_nb|pcm|opus.
    duration_s: 0 = record until stop.
    """
    args = ["microphone-record", "-f", output_file, "-e", encoder]
    if duration_s > 0: args += ["-l", str(duration_s)]
    return _tapi(args, timeout=duration_s + 10)


@mcp.tool()
def api_media_player(action: str = "info", file_path: str = "") -> str:
    """
    Control media player. action: play|pause|stop|info|next|prev.
    file_path: required for action=play.
    """
    args = ["media-player", action]
    if file_path: args.append(file_path)
    return _tapi(args)


@mcp.tool()
def api_wallpaper(file_path: str, lockscreen: bool = False) -> str:
    """Set the wallpaper from a local image file."""
    args = ["wallpaper", file_path]
    if lockscreen: args.append("-l")
    return _tapi(args)


@mcp.tool()
def api_wifi_info() -> str:
    """Current WiFi connection info: SSID, BSSID, IP, link speed, RSSI."""
    return _tapi(["wifi-connectioninfo"])


@mcp.tool()
def api_wifi_scan() -> str:
    """Scan for nearby WiFi networks and return results."""
    return _tapi(["wifi-scaninfo"], timeout=20)


@mcp.tool()
def api_wifi_enable(enable: bool = True) -> str:
    """Enable or disable WiFi."""
    return _tapi(["wifi-enable", "true" if enable else "false"])


@mcp.tool()
def api_nfc_read() -> str:
    """Wait for an NFC tag and return its content."""
    return _tapi(["nfc"], timeout=30)


@mcp.tool()
def api_fingerprint() -> str:
    """Prompt for fingerprint authentication and return result."""
    return _tapi(["fingerprint"], timeout=30)


@mcp.tool()
def api_usb_list() -> str:
    """List connected USB devices."""
    return _tapi(["usb"])


@mcp.tool()
def api_storage_get(storage_type: str = "downloads") -> str:
    """
    Get a file from Android storage picker.
    storage_type: downloads|pictures|music|ringtones|videos.
    """
    return _tapi(["storage-get", "-t", storage_type], timeout=60)


# ════════════════════════════════════════════════════════════════════════════
# PACKAGE MANAGEMENT
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def pkg_list(filter_str: str = "") -> str:
    """List installed Termux packages (with versions)."""
    out = _run(["dpkg-query", "-W", "-f=${Package} ${Version} ${Status}\n"])
    if out.startswith("[not found"):
        out = _run(["pkg", "list-installed"])
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No packages matching '{filter_str}'"
    return out


@mcp.tool()
def pkg_search(query: str) -> str:
    """Search Termux repos for packages matching query."""
    return _run(["pkg", "search", query])


@mcp.tool()
def pkg_show(package: str) -> str:
    """Show detailed info (description, size, deps) for a package."""
    return _run(["pkg", "show", package])


@mcp.tool()
def pkg_files(package: str) -> str:
    """List all files installed by a package."""
    return _run(["dpkg", "-L", package])


@mcp.tool()
def pkg_depends(package: str) -> str:
    """Show package dependency tree."""
    return _run(["apt-cache", "depends", package])


@mcp.tool()
def pkg_install(packages: str, confirm: bool = False) -> str:
    """Install Termux packages. confirm=True required to proceed."""
    if not confirm:
        return f"Would install: {packages}\nSet confirm=true to proceed."
    return _run(["pkg", "install", "-y"] + packages.split(), timeout=180)


@mcp.tool()
def pkg_remove(packages: str, confirm: bool = False) -> str:
    """Remove Termux packages. confirm=True required."""
    if not confirm:
        return f"Would remove: {packages}\nSet confirm=true to proceed."
    result = _run(["pkg", "uninstall", "-y"] + packages.split(), timeout=120)
    _bus.emit("package_removed", {"packages": packages})
    return result


@mcp.tool()
def pkg_upgrade(confirm: bool = False) -> str:
    """Upgrade all installed Termux packages."""
    if not confirm:
        return "Would upgrade all packages. Set confirm=true to proceed."
    return _run(["pkg", "upgrade", "-y"], timeout=600)


# ════════════════════════════════════════════════════════════════════════════
# PYTHON ECOSYSTEM
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def pip_list(filter_str: str = "") -> str:
    """List installed Python packages."""
    out = _run(["pip", "list", "--format=columns"])
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No packages matching '{filter_str}'"
    return out


@mcp.tool()
def pip_install(packages: str, confirm: bool = False,
                upgrade: bool = False) -> str:
    """pip install packages. confirm=True required."""
    if not confirm:
        return f"Would pip install: {packages}. Set confirm=true."
    flags = ["--upgrade"] if upgrade else []
    return _run(["pip", "install"] + flags + packages.split(), timeout=120)


@mcp.tool()
def pip_uninstall(packages: str, confirm: bool = False) -> str:
    """pip uninstall packages. confirm=True required."""
    if not confirm:
        return f"Would uninstall: {packages}. Set confirm=true."
    return _run(["pip", "uninstall", "-y"] + packages.split())


@mcp.tool()
def run_python(code: str, timeout: int = 30) -> str:
    """Execute Python code and return stdout/stderr."""
    return _run(["python3", "-c", code], timeout=timeout)


@mcp.tool()
def run_python_file(file_path: str, args: str = "",
                    timeout: int = 60) -> str:
    """Run a Python script file with optional args."""
    p = Path(file_path).expanduser()
    if not p.exists(): return f"Not found: {file_path}"
    cmd = ["python3", str(p)] + (args.split() if args else [])
    return _run(cmd, timeout=timeout)


# ════════════════════════════════════════════════════════════════════════════
# NODE / NPM
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def npm_list(global_pkgs: bool = False, filter_str: str = "") -> str:
    """List npm packages. global_pkgs=True for global installs."""
    cmd = ["npm", "list", "--depth=0"]
    if global_pkgs: cmd.append("-g")
    out = _run(cmd)
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No packages matching '{filter_str}'"
    return out


@mcp.tool()
def npm_install(packages: str, global_install: bool = False,
                confirm: bool = False, cwd: str = "") -> str:
    """npm install packages. confirm=True required."""
    if not confirm:
        return f"Would npm install: {packages}. Set confirm=true."
    cmd = ["npm", "install"] + (["-g"] if global_install else []) + packages.split()
    return _run(cmd, timeout=180, cwd=cwd or None)


@mcp.tool()
def npm_run(script: str, cwd: str = "") -> str:
    """Run an npm script from package.json."""
    return _run(["npm", "run", script], timeout=120, cwd=cwd or None)


@mcp.tool()
def run_node(code: str, timeout: int = 30) -> str:
    """Execute JavaScript code in Node.js and return output."""
    return _run(["node", "-e", code], timeout=timeout)


@mcp.tool()
def run_node_file(file_path: str, args: str = "", timeout: int = 60) -> str:
    """Run a Node.js script file with optional args."""
    p = Path(file_path).expanduser()
    if not p.exists(): return f"Not found: {file_path}"
    cmd = ["node", str(p)] + (args.split() if args else [])
    return _run(cmd, timeout=timeout)


# ════════════════════════════════════════════════════════════════════════════
# DEVELOPMENT TOOLS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def go_build(package: str = "./...", output: str = "", cwd: str = "",
             extra_flags: str = "") -> str:
    """go build. package: Go package path. output: binary output path."""
    cmd = ["go", "build"]
    if output: cmd += ["-o", output]
    if extra_flags: cmd += extra_flags.split()
    cmd.append(package)
    return _run(cmd, timeout=180, cwd=cwd or None)


@mcp.tool()
def go_test(package: str = "./...", verbose: bool = False,
            cwd: str = "", timeout_s: int = 120) -> str:
    """go test. package: package path or './...' for all."""
    cmd = ["go", "test"] + (["-v"] if verbose else []) + [package]
    return _run(cmd, timeout=timeout_s, cwd=cwd or None)


@mcp.tool()
def go_run(file_path: str, args: str = "", cwd: str = "") -> str:
    """go run a Go source file."""
    p = Path(file_path).expanduser()
    if not p.exists(): return f"Not found: {file_path}"
    cmd = ["go", "run", str(p)] + (args.split() if args else [])
    return _run(cmd, timeout=60, cwd=cwd or None)


@mcp.tool()
def make_run(target: str = "", cwd: str = "", env_str: str = "") -> str:
    """Run make with an optional target and working directory."""
    cmd = ["make"] + ([target] if target else [])
    env_extra = dict(kv.split("=", 1) for kv in env_str.split() if "=" in kv)
    return _run(cmd, timeout=300, cwd=cwd or None,
                env_extra=env_extra or None)


@mcp.tool()
def git_status(repo_path: str = "") -> str:
    """git status with branch info."""
    cwd = repo_path or str(Path.home())
    out = _run(["git", "status", "-sb"], cwd=cwd)
    log = _run(["git", "log", "--oneline", "-10"], cwd=cwd)
    return f"## Status\n{out}\n\n## Recent commits\n{log}"


@mcp.tool()
def git_log(repo_path: str = "", limit: int = 20, author: str = "") -> str:
    """git log with optional author filter."""
    cwd  = repo_path or str(Path.home())
    cmd  = ["git", "log", "--oneline", f"-{limit}"]
    if author: cmd += ["--author", author]
    return _run(cmd, cwd=cwd)


@mcp.tool()
def git_diff(repo_path: str = "", staged: bool = False) -> str:
    """git diff (unstaged by default, staged=True for --cached)."""
    cwd = repo_path or str(Path.home())
    cmd = ["git", "diff"] + (["--cached"] if staged else [])
    return _run(cmd, cwd=cwd)[:6000]


@mcp.tool()
def cargo_build(cwd: str = "", release: bool = False,
                features: str = "") -> str:
    """Build a Rust project with cargo."""
    cmd = ["cargo", "build"] + (["--release"] if release else [])
    if features: cmd += ["--features", features]
    return _run(cmd, timeout=300, cwd=cwd or None)


# ════════════════════════════════════════════════════════════════════════════
# SHELL / SCRIPT EXECUTION
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def run_bash(script: str, timeout: int = 60, cwd: str = "") -> str:
    """Execute a bash script and return stdout/stderr."""
    return _run(["bash", "-c", script], timeout=timeout, cwd=cwd or None)


@mcp.tool()
def run_in_shell(command: str, shell: str = "bash",
                 timeout: int = 30, cwd: str = "") -> str:
    """Execute in a specific shell: bash, zsh, fish, sh, dash."""
    return _run([shell, "-c", command], timeout=timeout, cwd=cwd or None)


@mcp.tool()
def which_command(name: str) -> str:
    """Find the full path of a command."""
    return _run(["which", name])


@mcp.tool()
def env_vars(filter_str: str = "") -> str:
    """List all environment variables, optionally filtered."""
    env_str = "\n".join(f"{k}={v}" for k, v in sorted(os.environ.items()))
    if filter_str:
        lines = [l for l in env_str.splitlines()
                 if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No env vars matching '{filter_str}'"
    return env_str


# ════════════════════════════════════════════════════════════════════════════
# PROCESS MANAGEMENT
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def process_list(filter_str: str = "", sort_by: str = "pid") -> str:
    """List running processes. sort_by: pid|cpu|mem|name."""
    flags = {"pid": "-o pid,ppid,user,args",
             "cpu": "-o pid,pcpu,user,args --sort=-pcpu",
             "mem": "-o pid,pmem,vsz,rss,user,args --sort=-pmem",
             "name": "-o pid,user,args --sort=args"}.get(sort_by, "-o pid,ppid,user,args")
    out = _run(["ps", "aux"])
    if filter_str:
        header = out.splitlines()[0] if out.splitlines() else ""
        matches = [l for l in out.splitlines()[1:]
                   if filter_str.lower() in l.lower()]
        return header + "\n" + "\n".join(matches) if matches else f"No processes matching '{filter_str}'"
    return out


@mcp.tool()
def process_kill(pid: int, signal: str = "TERM",
                 confirm: bool = False) -> str:
    """Kill a process by PID. signal: TERM|KILL|HUP|INT|USR1. confirm=True required."""
    if not confirm:
        return f"Would send SIG{signal} to PID {pid}. Set confirm=true."
    return _run(["kill", f"-{signal}", str(pid)])


@mcp.tool()
def process_info(pid: int) -> str:
    """Detailed info for a process: status, maps, file descriptors count."""
    base = Path(f"/proc/{pid}")
    if not base.exists():
        return f"Process {pid} not found"
    parts = {}
    for f in ("status", "cmdline", "cwd"):
        try:
            content = (base / f).read_bytes()
            parts[f] = content.replace(b"\x00", b" ").decode(errors="replace")[:500]
        except Exception:
            pass
    try:
        fd_count = len(list((base / "fd").iterdir()))
        parts["fd_count"] = str(fd_count)
    except Exception:
        pass
    return json.dumps(parts, indent=2)


@mcp.tool()
def port_check_all() -> str:
    """List all listening TCP/UDP ports."""
    out = _run(["ss", "-tlnup"])
    if out.startswith("[not found"):
        out = _run(["netstat", "-tlnup"])
    return out


@mcp.tool()
def port_check(port: int, host: str = "127.0.0.1") -> str:
    """Check if a specific port is open/listening."""
    import socket as sock
    try:
        with sock.create_connection((host, port), timeout=3):
            return f"Port {port} on {host}: OPEN"
    except ConnectionRefusedError:
        return f"Port {port} on {host}: CLOSED"
    except Exception as e:
        return f"Port {port} on {host}: ERROR ({e})"


# ════════════════════════════════════════════════════════════════════════════
# TERMUX SERVICES & BOOT
# ════════════════════════════════════════════════════════════════════════════

_SERVICES_DIR = Path.home() / ".termux" / "boot"
_SV_DIR       = PREFIX / "var" / "service"

@mcp.tool()
def termux_service_list() -> str:
    """List services managed by termux-services (runit/sv)."""
    if not _SV_DIR.exists():
        return "termux-services not installed. Run: pkg install termux-services"
    services = []
    for svc in _SV_DIR.iterdir():
        if svc.is_dir():
            down = (svc / "down").exists()
            services.append(f"{'[running]' if not down else '[down]   '} {svc.name}")
    return "\n".join(sorted(services)) or "No services found"


@mcp.tool()
def termux_service_start(name: str) -> str:
    """Start a termux-service by name."""
    return _run(["sv", "start", name])


@mcp.tool()
def termux_service_stop(name: str) -> str:
    """Stop a termux-service by name."""
    return _run(["sv", "stop", name])


@mcp.tool()
def termux_service_status(name: str) -> str:
    """Check status of a termux-service."""
    return _run(["sv", "status", name])


@mcp.tool()
def termux_boot_list() -> str:
    """List scripts that run on Termux boot."""
    boot_dir = Path.home() / ".termux" / "boot"
    if not boot_dir.exists():
        return "No boot directory (~/.termux/boot)"
    scripts = sorted(boot_dir.iterdir())
    if not scripts:
        return "No boot scripts"
    lines = []
    for s in scripts:
        size = s.stat().st_size
        lines.append(f"{s.name:30s} {size:6} bytes")
    return "\n".join(lines)


@mcp.tool()
def termux_boot_add(name: str, content: str) -> str:
    """Add a startup script to ~/.termux/boot/. Overwrites existing."""
    boot_dir = Path.home() / ".termux" / "boot"
    boot_dir.mkdir(parents=True, exist_ok=True)
    script = boot_dir / name
    script.write_text(content)
    script.chmod(0o755)
    return f"Boot script '{name}' written ({len(content)} bytes)"


@mcp.tool()
def termux_boot_remove(name: str, confirm: bool = False) -> str:
    """Remove a boot script. confirm=True required."""
    if not confirm:
        return f"Would remove boot script '{name}'. Set confirm=true."
    script = Path.home() / ".termux" / "boot" / name
    if not script.exists():
        return f"Boot script '{name}' not found"
    script.unlink()
    return f"Removed boot script '{name}'"


# ════════════════════════════════════════════════════════════════════════════
# NETWORKING
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def curl_request(url: str, method: str = "GET", headers: str = "",
                 body: str = "", timeout: int = 15,
                 follow_redirects: bool = True) -> str:
    """Full HTTP request via curl. headers: 'Key: Value' per line."""
    cmd = ["curl", "-s", "-w", "\n---STATUS:%{http_code}---",
           "-X", method.upper(), "--max-time", str(timeout)]
    if follow_redirects: cmd.append("-L")
    for h in headers.strip().splitlines():
        if h.strip(): cmd += ["-H", h.strip()]
    if body: cmd += ["--data", body]
    cmd.append(url)
    return _run(cmd, timeout=timeout + 5)[:5000]


@mcp.tool()
def wget_download(url: str, output_path: str = "",
                  timeout: int = 60) -> str:
    """Download a URL with wget."""
    cmd = ["wget", "-q", "--show-progress", "--timeout", str(timeout)]
    if output_path: cmd += ["-O", output_path]
    cmd.append(url)
    return _run(cmd, timeout=timeout + 10)


@mcp.tool()
def nc_test(host: str, port: int, timeout: int = 5) -> str:
    """Test TCP connectivity to host:port using nc (netcat)."""
    return _run(["nc", "-zv", "-w", str(timeout), host, str(port)], timeout=timeout + 2)


@mcp.tool()
def dns_lookup(hostname: str, record_type: str = "A",
               server: str = "") -> str:
    """DNS lookup using dig. record_type: A|AAAA|MX|TXT|NS|CNAME|SOA."""
    cmd = ["dig", f"@{server}" if server else "", hostname, record_type,
           "+short"]
    return _run([c for c in cmd if c], timeout=10)


@mcp.tool()
def ssh_keygen(key_type: str = "ed25519", bits: int = 0,
               comment: str = "", output: str = "") -> str:
    """Generate an SSH key pair."""
    out = output or str(Path.home() / ".ssh" / f"id_{key_type}")
    cmd = ["ssh-keygen", "-t", key_type, "-N", "", "-f", out, "-q"]
    if bits: cmd += ["-b", str(bits)]
    if comment: cmd += ["-C", comment]
    return _run(cmd)


@mcp.tool()
def network_interfaces() -> str:
    """All network interfaces with IPs and stats."""
    addrs  = _run(["ip", "-br", "addr"])
    routes = _run(["ip", "route"])
    stats  = _run(["ip", "-s", "link"])
    return f"## Addresses\n{addrs}\n\n## Routes\n{routes}\n\n## Stats (head)\n{stats[:2000]}"


# ════════════════════════════════════════════════════════════════════════════
# FILE OPERATIONS (ENHANCED)
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def find_files(directory: str = "~", pattern: str = "*",
               file_type: str = "f", max_results: int = 50,
               newer_than: str = "", min_size: str = "") -> str:
    """
    Advanced file search. file_type: f=file,d=dir,l=symlink.
    newer_than: filename to compare mtime. min_size: e.g. '+1M'.
    """
    base = Path(directory).expanduser()
    if not base.exists(): return f"Dir not found: {directory}"
    cmd = ["find", str(base), "-name", pattern, f"-type", file_type]
    if newer_than: cmd += ["-newer", newer_than]
    if min_size:   cmd += ["-size",  min_size]
    cmd += ["-maxdepth", "10"]
    out = _run(cmd, timeout=30)
    lines = out.splitlines()[:max_results]
    suffix = f"\n(+{len(out.splitlines())-max_results} more)" if len(out.splitlines()) > max_results else ""
    return "\n".join(lines) + suffix


@mcp.tool()
def archive_create(output_path: str, paths: str,
                   fmt: str = "tar.gz") -> str:
    """
    Create an archive. fmt: tar.gz|tar.bz2|tar.xz|zip|7z.
    paths: space-separated list of files/dirs to include.
    """
    items = paths.split()
    if fmt in ("tar.gz", "tgz"):
        cmd = ["tar", "-czf", output_path] + items
    elif fmt in ("tar.bz2", "tbz"):
        cmd = ["tar", "-cjf", output_path] + items
    elif fmt == "tar.xz":
        cmd = ["tar", "-cJf", output_path] + items
    elif fmt == "zip":
        cmd = ["zip", "-r", output_path] + items
    elif fmt == "7z":
        cmd = ["7z", "a", output_path] + items
    else:
        return f"Unknown format: {fmt}"
    return _run(cmd, timeout=120)


@mcp.tool()
def archive_extract(archive_path: str, output_dir: str = "") -> str:
    """Extract any archive (tar, zip, 7z, gz, bz2, xz)."""
    p = Path(archive_path).expanduser()
    if not p.exists(): return f"Not found: {archive_path}"
    out = output_dir or str(p.parent / p.stem.rstrip(".tar"))
    Path(out).mkdir(parents=True, exist_ok=True)
    name = p.name.lower()
    if   ".tar" in name:   cmd = ["tar", "-xf", str(p), "-C", out]
    elif name.endswith(".zip"):  cmd = ["unzip", str(p), "-d", out]
    elif name.endswith(".7z"):   cmd = ["7z", "x", str(p), f"-o{out}"]
    elif name.endswith(".gz"):   cmd = ["gunzip", "-k", str(p)]
    else:                        cmd = ["7z", "x", str(p), f"-o{out}"]
    return _run(cmd, timeout=120)


@mcp.tool()
def checksum(path: str, algorithm: str = "sha256") -> str:
    """Compute file checksum. algorithm: md5|sha1|sha256|sha512."""
    p = Path(path).expanduser()
    if not p.exists(): return f"Not found: {path}"
    algo_map = {"md5": hashlib.md5, "sha1": hashlib.sha1,
                "sha256": hashlib.sha256, "sha512": hashlib.sha512}
    h = algo_map.get(algorithm, hashlib.sha256)()
    with p.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return f"{algorithm.upper()}: {h.hexdigest()}  {p}"


@mcp.tool()
def hex_dump(path: str, offset: int = 0, length: int = 256) -> str:
    """Hex dump of a file section. offset: byte offset. length: bytes to read."""
    p = Path(path).expanduser()
    if not p.exists(): return f"Not found: {path}"
    out = _run(["xxd", "-s", str(offset), "-l", str(length), str(p)])
    if out.startswith("[not found"):
        with p.open("rb") as f:
            f.seek(offset)
            data = f.read(length)
        rows = []
        for i in range(0, len(data), 16):
            chunk = data[i:i+16]
            hex_part  = " ".join(f"{b:02x}" for b in chunk)
            ascii_part = "".join(chr(b) if 32 <= b < 127 else "." for b in chunk)
            rows.append(f"{offset+i:08x}  {hex_part:<47}  |{ascii_part}|")
        out = "\n".join(rows)
    return out


@mcp.tool()
def file_type(path: str) -> str:
    """Identify file type using the file command."""
    p = Path(path).expanduser()
    if not p.exists(): return f"Not found: {path}"
    return _run(["file", str(p)])


# ════════════════════════════════════════════════════════════════════════════
# CRYPTO & SECURITY
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def openssl_hash(data: str, algorithm: str = "sha256") -> str:
    """Hash a string with openssl dgst. algorithm: md5|sha1|sha256|sha512."""
    return _run(["openssl", "dgst", f"-{algorithm}"], input_data=data)


@mcp.tool()
def openssl_gen_key(key_type: str = "rsa", bits: int = 2048,
                    output: str = "") -> str:
    """Generate a private key. key_type: rsa|ec|ed25519."""
    if key_type == "rsa":
        cmd = ["openssl", "genrsa", str(bits)]
    elif key_type == "ec":
        cmd = ["openssl", "ecparam", "-genkey", "-name", "prime256v1"]
    else:
        cmd = ["openssl", "genpkey", "-algorithm", key_type]
    out = _run(cmd)
    if output:
        Path(output).write_text(out)
        return f"Key written to {output}"
    return out


@mcp.tool()
def openssl_cert_info(path_or_url: str) -> str:
    """Show certificate details from a PEM file or HTTPS URL."""
    if path_or_url.startswith("https://") or path_or_url.startswith("http://"):
        host = re.sub(r"https?://", "", path_or_url).split("/")[0]
        return _run(["openssl", "s_client", "-connect", f"{host}:443",
                     "-servername", host, "-showcerts"],
                    input_data="", timeout=15)[:3000]
    p = Path(path_or_url).expanduser()
    if not p.exists(): return f"Not found: {path_or_url}"
    return _run(["openssl", "x509", "-in", str(p), "-text", "-noout"])


@mcp.tool()
def totp_generate(secret: str, digits: int = 6,
                  period: int = 30) -> str:
    """Generate a TOTP code from a base32 secret (RFC 6238)."""
    try:
        import hmac as _hmac, struct as _struct
        secret_bytes = base64.b32decode(secret.upper().replace(" ", ""))
        counter = int(time.time()) // period
        msg  = _struct.pack(">Q", counter)
        h    = _hmac.new(secret_bytes, msg, hashlib.sha1).digest()
        off  = h[-1] & 0x0F
        code = (_struct.unpack(">I", h[off:off+4])[0] & 0x7FFFFFFF) % (10 ** digits)
        remaining = period - (int(time.time()) % period)
        return f"{code:0{digits}d}  (valid for {remaining}s)"
    except Exception as e:
        return f"TOTP error: {e}"


@mcp.tool()
def gpg_list_keys(public: bool = True) -> str:
    """List GPG keys in the local keyring."""
    flag = "--list-keys" if public else "--list-secret-keys"
    return _run(["gpg", flag, "--with-fingerprint"])


# ════════════════════════════════════════════════════════════════════════════
# BUS BRIDGE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def bus_call_android(tool: str, args_json: str = "{}") -> str:
    """Call a tool on the Android MCP server via the bidirectional bus.
    tool: name registered on the Android side. args_json: JSON object string."""
    if not _bus.is_connected():
        return "Bus not connected to Android peer. Is gorkbot_android running?"
    try:
        args = json.loads(args_json)
        return _bus.call_remote(tool, args, timeout=45)
    except json.JSONDecodeError as e:
        return f"Invalid args JSON: {e}"
    except Exception as e:
        return f"Bus call error: {e}"


@mcp.tool()
def bus_emit_event(event_name: str, data_json: str = "{}") -> str:
    """Emit an event to the Android peer via the bus (fire-and-forget)."""
    try:
        data = json.loads(data_json)
        _bus.emit(event_name, data)
        return f"Event '{event_name}' emitted to Android"
    except Exception as e:
        return f"Error: {e}"


@mcp.tool()
def bus_status() -> str:
    """Show bidirectional bus connection status and recent events."""
    connected   = _bus.is_connected()
    local_tools = _bus.list_local_tools()
    recent_evts = _bus.recent_events(10)
    lines = [
        f"**Bus status**: {'🟢 Connected' if connected else '🔴 Disconnected'}",
        f"**Role**: {_bus.role}",
        f"**Local tools exposed**: {', '.join(local_tools)}",
        "\n**Recent events (last 10)**:",
    ]
    for e in recent_evts:
        lines.append(f"  [{e['ts']}] {e['src']}/{e['name']}: {e['data']}")
    if not recent_evts:
        lines.append("  (none)")
    return "\n".join(lines)


# ════════════════════════════════════════════════════════════════════════════
# QUICK STATUS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def termux_quick_status() -> str:
    """Single-call Termux environment snapshot."""
    return json.dumps({
        "prefix":         str(PREFIX),
        "python":         _run(["python3", "--version"]),
        "node":           _run(["node",    "--version"]),
        "go":             _run(["go",      "version"]),
        "rust":           _run(["rustc",   "--version"]),
        "git":            _run(["git",     "--version"]),
        "shell":          os.environ.get("SHELL", "unknown"),
        "home":           str(Path.home()),
        "storage_mounted": Path("/storage/emulated/0").exists(),
        "disk_free":      _run(["df", "-h", str(Path.home())]),
        "uptime":         _run(["uptime"]),
        "bus_connected":  _bus.is_connected(),
    }, indent=2)


if __name__ == "__main__":
    mcp.run()
