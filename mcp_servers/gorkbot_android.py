#!/usr/bin/env python3
"""
gorkbot_android — Full-scope Android MCP server.

Covers:
  • APK analysis pipeline  (aapt2, strings, manifest, cert verification)
  • ADB device control     (shell, push/pull, install, logcat, forward)
  • Android package manager (pm list/info/grant/revoke/clear/path)
  • Activity manager       (am start/broadcast/force-stop)
  • Content providers      (content query/call)
  • System internals       (getprop, /proc, /sys, dumpsys)
  • Hardware / sensors     (battery, wifi, camera, thermal, GPS via sysfs)
  • Binary analysis        (readelf, strings, ELF/DEX headers)
  • Security               (SELinux, root check, APK cert, keystore)
  • Bus bridge             (call Termux tools via bidirectional bus)

Bidirectional bus: connects to gorkbot_termux via Unix socket.
"""

import json
import os
import re
import subprocess
import sys
import threading
from pathlib import Path
from typing import Optional

from fastmcp import FastMCP

# ── Bus integration ──────────────────────────────────────────────────────────
sys.path.insert(0, str(Path(__file__).parent))
from bus import Bus

_bus = Bus("android")

def _reg_android_bus_tools():
    """Register tools that Termux can call on us via the bus."""
    _bus.register_tool("device_props",   lambda _: android_properties())
    _bus.register_tool("battery_sys",    lambda _: battery_info_sys())
    _bus.register_tool("wifi_sysfs",     lambda _: wifi_info_sysfs())
    _bus.register_tool("apk_info_bus",   lambda a: apk_info(a.get("path", "")))
    _bus.register_tool("pm_list_pkgs",   lambda a: pm_list_packages(a.get("filter",""), a.get("flags","")))
    _bus.register_tool("adb_dev_list",   lambda _: adb_devices())
    _bus.register_tool("selinux",        lambda _: selinux_status())
    _bus.register_tool("ping",           lambda _: "pong from android")

_reg_android_bus_tools()
_bus.on_event("package_installed", lambda d: print(f"[bus-event] Termux installed: {d}", file=sys.stderr))
_bus.on_event("service_started",   lambda d: print(f"[bus-event] Termux service: {d}",   file=sys.stderr))
_bus.start()

mcp = FastMCP("gorkbot-android")

# ── Helpers ──────────────────────────────────────────────────────────────────

def _run(cmd: list[str], timeout: int = 30, cwd: Optional[str] = None,
         input_data: Optional[str] = None) -> str:
    try:
        r = subprocess.run(
            cmd, capture_output=True, text=True,
            timeout=timeout, cwd=cwd,
            input=input_data,
            env={**os.environ},
        )
        out = r.stdout.strip()
        err = r.stderr.strip()
        if r.returncode != 0 and err and not out:
            return f"[exit {r.returncode}] {err[:800]}"
        if r.returncode != 0 and err:
            out += f"\n[stderr] {err[:400]}"
        return out or f"(exit {r.returncode}, no output)"
    except subprocess.TimeoutExpired:
        return f"[timeout after {timeout}s]"
    except FileNotFoundError:
        return f"[not found: {cmd[0]}]"
    except Exception as e:
        return f"[error: {e}]"

def _prop(key: str) -> str:
    return _run(["getprop", key])

def _tool_present(name: str) -> bool:
    import shutil
    return shutil.which(name) is not None


# ════════════════════════════════════════════════════════════════════════════
# APK ANALYSIS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def apk_info(apk_path: str) -> str:
    """Full APK metadata: package name, version, SDK, activities, receivers.
    Uses aapt2 (preferred) or aapt."""
    p = Path(apk_path).expanduser()
    if not p.exists():
        return f"Not found: {apk_path}"
    for tool in ("aapt2", "aapt"):
        out = _run([tool, "dump", "badging", str(p)], timeout=30)
        if not out.startswith("[not found"):
            return out
    return "aapt/aapt2 not installed. Run: pkg install aapt2"


@mcp.tool()
def apk_permissions(apk_path: str) -> str:
    """List all permissions declared or used by an APK."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    for tool in ("aapt2", "aapt"):
        out = _run([tool, "dump", "permissions", str(p)], timeout=30)
        if not out.startswith("[not found"): return out
    return "aapt/aapt2 not available"


@mcp.tool()
def apk_manifest(apk_path: str) -> str:
    """Extract AndroidManifest.xml as human-readable text from an APK."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out = _run(["aapt2", "dump", "xmltree", str(p), "--file", "AndroidManifest.xml"], timeout=30)
    if out.startswith("[not found"):
        out = _run(["aapt", "dump", "xmltree", str(p), "AndroidManifest.xml"], timeout=30)
    return out[:8000]


@mcp.tool()
def apk_resources(apk_path: str, filter_str: str = "") -> str:
    """Dump APK resource table. filter_str narrows output by substring."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out = _run(["aapt2", "dump", "resources", str(p)], timeout=60)
    if out.startswith("[not found"):
        out = _run(["aapt", "dump", "resources", str(p)], timeout=60)
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines[:200]) or f"No resources matching '{filter_str}'"
    return out[:6000]


@mcp.tool()
def apk_strings(apk_path: str, filter_str: str = "", min_len: int = 6) -> str:
    """Extract printable strings from an APK binary (via strings command)."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out = _run(["strings", f"-n{min_len}", str(p)], timeout=30)
    if filter_str:
        matches = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(matches[:300]) or f"No strings matching '{filter_str}'"
    return "\n".join(out.splitlines()[:500])


@mcp.tool()
def apk_verify(apk_path: str) -> str:
    """Verify APK signature and print certificate details using apksigner."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out = _run(["apksigner", "verify", "--verbose", "--print-certs", str(p)], timeout=30)
    if out.startswith("[not found"):
        # Fall back to jarsigner
        out = _run(["jarsigner", "-verify", "-verbose", "-certs", str(p)], timeout=30)
    if out.startswith("[not found"):
        return "apksigner/jarsigner not installed. Run: pkg install apksigner"
    return out


@mcp.tool()
def apk_decompile(apk_path: str, output_dir: str = "", tool: str = "jadx") -> str:
    """Decompile APK to Java source. tool: 'jadx' (default) or 'apktool'."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out_dir = output_dir or str(p.parent / (p.stem + f"_{tool}"))
    if tool == "apktool":
        result = _run(["apktool", "d", "-o", out_dir, str(p)], timeout=300)
    else:
        result = _run(["jadx", "-d", out_dir, str(p)], timeout=300)
    if result.startswith("[not found"):
        return f"{tool} not installed. Install via pkg or download from GitHub."
    return f"Output: {out_dir}\n\n{result[:2000]}"


@mcp.tool()
def apk_rebuild(smali_dir: str, output_apk: str = "") -> str:
    """Rebuild a decompiled APK directory back to APK using apktool."""
    d = Path(smali_dir).expanduser()
    if not d.exists(): return f"Dir not found: {smali_dir}"
    out = output_apk or str(d.parent / (d.name + "_rebuilt.apk"))
    return _run(["apktool", "b", str(d), "-o", out], timeout=120)


@mcp.tool()
def apk_sign(apk_path: str, keystore: str, alias: str,
             ks_pass: str = "android", key_pass: str = "") -> str:
    """Sign an APK with apksigner. Defaults to debug keystore if keystore='debug'."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    if keystore == "debug":
        keystore = str(Path.home() / ".android" / "debug.keystore")
        alias    = alias or "androiddebugkey"
        ks_pass  = ks_pass or "android"
    cmd = ["apksigner", "sign",
           "--ks", keystore, "--ks-key-alias", alias,
           "--ks-pass", f"pass:{ks_pass}",
           "--key-pass", f"pass:{key_pass or ks_pass}",
           str(p)]
    return _run(cmd, timeout=60)


@mcp.tool()
def dex_analyze(apk_or_dex: str) -> str:
    """Analyze DEX classes from an APK or .dex file using dexdump."""
    p = Path(apk_or_dex).expanduser()
    if not p.exists(): return f"Not found: {apk_or_dex}"
    out = _run(["dexdump", "-f", str(p)], timeout=60)
    if out.startswith("[not found"):
        return "dexdump not installed. Run: pkg install dexdump"
    return out[:8000]


@mcp.tool()
def find_apks(search_dir: str = "~", max_results: int = 50) -> str:
    """Recursively find all .apk files under search_dir."""
    base = Path(search_dir).expanduser()
    if not base.exists(): return f"Dir not found: {search_dir}"
    apks = sorted(base.rglob("*.apk"), key=lambda p: p.stat().st_size, reverse=True)[:max_results]
    if not apks: return "No APK files found."
    lines = [f"{p.stat().st_size:>12,} B  {p}" for p in apks]
    return "\n".join(lines)


# ════════════════════════════════════════════════════════════════════════════
# ADB
# ════════════════════════════════════════════════════════════════════════════

def _adb(args: list[str], serial: str = "", timeout: int = 30) -> str:
    cmd = ["adb"]
    if serial: cmd += ["-s", serial]
    return _run(cmd + args, timeout=timeout)


@mcp.tool()
def adb_devices() -> str:
    """List connected ADB devices and their state."""
    return _run(["adb", "devices", "-l"])


@mcp.tool()
def adb_shell(command: str, serial: str = "", timeout: int = 30) -> str:
    """Execute a shell command on an ADB-connected device."""
    return _adb(["shell", command], serial=serial, timeout=timeout)


@mcp.tool()
def adb_push(local_path: str, remote_path: str, serial: str = "") -> str:
    """Push a local file to an ADB device."""
    return _adb(["push", local_path, remote_path], serial=serial, timeout=60)


@mcp.tool()
def adb_pull(remote_path: str, local_path: str = ".", serial: str = "") -> str:
    """Pull a file from an ADB device to local storage."""
    return _adb(["pull", remote_path, local_path], serial=serial, timeout=60)


@mcp.tool()
def adb_install(apk_path: str, serial: str = "", replace: bool = True) -> str:
    """Install an APK on an ADB device. replace=True allows replacing existing apps."""
    flags = ["-r"] if replace else []
    return _adb(["install"] + flags + [apk_path], serial=serial, timeout=120)


@mcp.tool()
def adb_uninstall(package: str, serial: str = "", keep_data: bool = False) -> str:
    """Uninstall a package from an ADB device."""
    flags = ["-k"] if keep_data else []
    return _adb(["uninstall"] + flags + [package], serial=serial, timeout=60)


@mcp.tool()
def adb_logcat(tag: str = "", level: str = "D", lines: int = 100,
               serial: str = "") -> str:
    """Capture recent logcat output. level: V/D/I/W/E/F."""
    cmd = ["logcat", "-d", f"-v", "time", f"*:{level}"]
    if tag:
        cmd = ["logcat", "-d", "-v", "time", f"{tag}:{level}", f"*:S"]
    out = _adb(cmd, serial=serial, timeout=15)
    return "\n".join(out.splitlines()[-lines:])


@mcp.tool()
def adb_logcat_grep(pattern: str, level: str = "E",
                    lines: int = 50, serial: str = "") -> str:
    """Search logcat for a regex pattern."""
    out = _adb(["logcat", "-d", "-v", "time", f"*:{level}"], serial=serial, timeout=15)
    try:
        rx = re.compile(pattern, re.IGNORECASE)
        matches = [l for l in out.splitlines() if rx.search(l)]
        return "\n".join(matches[-lines:]) or f"No matches for '{pattern}'"
    except re.error as e:
        return f"Invalid regex: {e}"


@mcp.tool()
def adb_screencap(output_path: str = "/sdcard/screen.png", serial: str = "") -> str:
    """Take a screenshot on an ADB device and optionally pull it."""
    result = _adb(["shell", "screencap", "-p", output_path], serial=serial)
    return f"Screenshot saved to {output_path} on device\n{result}"


@mcp.tool()
def adb_forward(local_port: int, remote_port: int, serial: str = "") -> str:
    """Forward a local TCP port to a remote port on the ADB device."""
    return _adb(["forward", f"tcp:{local_port}", f"tcp:{remote_port}"], serial=serial)


@mcp.tool()
def adb_bugreport(output_dir: str = "~", serial: str = "") -> str:
    """Generate a bug report from an ADB device (may take a minute)."""
    out_dir = Path(output_dir).expanduser()
    out_dir.mkdir(parents=True, exist_ok=True)
    return _adb(["bugreport", str(out_dir)], serial=serial, timeout=300)


# ════════════════════════════════════════════════════════════════════════════
# ANDROID PACKAGE MANAGER
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def pm_list_packages(filter_str: str = "",
                     flags: str = "-f") -> str:
    """
    List installed packages. flags examples: -f (with path), -3 (third-party),
    -s (system), -d (disabled), -e (enabled).
    """
    cmd = ["pm", "list", "packages"] + flags.split() + (["-f"] if "-f" not in flags else [])
    out = _run(["sh", "-c", " ".join(cmd)])
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No packages matching '{filter_str}'"
    return out


@mcp.tool()
def pm_package_info(package: str) -> str:
    """Detailed package info from pm dump (permissions, activities, services, receivers)."""
    return _run(["sh", "-c", f"pm dump {package}"], timeout=15)[:6000]


@mcp.tool()
def pm_path(package: str) -> str:
    """Get the filesystem path of an installed APK."""
    return _run(["sh", "-c", f"pm path {package}"])


@mcp.tool()
def pm_grant(package: str, permission: str) -> str:
    """Grant a runtime permission to a package."""
    return _run(["sh", "-c", f"pm grant {package} {permission}"])


@mcp.tool()
def pm_revoke(package: str, permission: str) -> str:
    """Revoke a runtime permission from a package."""
    return _run(["sh", "-c", f"pm revoke {package} {permission}"])


@mcp.tool()
def pm_clear_data(package: str) -> str:
    """Clear all data (cache, databases, prefs) for a package."""
    return _run(["sh", "-c", f"pm clear {package}"])


@mcp.tool()
def pm_disable(package: str) -> str:
    """Disable a package (requires root or system permissions)."""
    return _run(["sh", "-c", f"pm disable {package}"])


@mcp.tool()
def pm_enable(package: str) -> str:
    """Enable a previously disabled package."""
    return _run(["sh", "-c", f"pm enable {package}"])


# ════════════════════════════════════════════════════════════════════════════
# ACTIVITY MANAGER
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def am_start(component_or_action: str, extras: str = "",
             flags: str = "") -> str:
    """
    Start an Activity. component_or_action: 'com.pkg/.Activity' or intent action.
    extras: '-e key value -ei intkey 1 ...' style string.
    """
    cmd = f"am start {flags} {extras} {component_or_action}"
    return _run(["sh", "-c", cmd], timeout=15)


@mcp.tool()
def am_force_stop(package: str) -> str:
    """Force-stop all processes of a package."""
    return _run(["sh", "-c", f"am force-stop {package}"])


@mcp.tool()
def am_broadcast(action: str, package: str = "",
                 extras: str = "") -> str:
    """Send a broadcast intent. extras: '-e key value' style."""
    pkg_flag = f"-p {package}" if package else ""
    cmd = f"am broadcast -a {action} {pkg_flag} {extras}"
    return _run(["sh", "-c", cmd], timeout=15)


@mcp.tool()
def am_start_service(component: str, extras: str = "") -> str:
    """Start an Android service component."""
    return _run(["sh", "-c", f"am startservice {extras} {component}"], timeout=15)


@mcp.tool()
def am_stop_service(component: str) -> str:
    """Stop an Android service component."""
    return _run(["sh", "-c", f"am stopservice {component}"])


@mcp.tool()
def am_kill(package: str) -> str:
    """Kill background processes for a package (non-foreground only)."""
    return _run(["sh", "-c", f"am kill {package}"])


@mcp.tool()
def am_stack_list() -> str:
    """List current Activity task stack."""
    return _run(["sh", "-c", "am stack list"], timeout=10)


# ════════════════════════════════════════════════════════════════════════════
# CONTENT PROVIDERS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def content_query(uri: str, projection: str = "",
                  where: str = "", sort: str = "") -> str:
    """
    Query an Android content provider.
    uri: e.g. 'content://media/external/images/media'
    projection: comma-separated columns, empty = all.
    """
    cmd = f"content query --uri {uri}"
    if projection: cmd += f" --projection {projection}"
    if where:      cmd += f" --where '{where}'"
    if sort:       cmd += f" --sort '{sort}'"
    return _run(["sh", "-c", cmd], timeout=20)[:4000]


@mcp.tool()
def content_call(uri: str, method: str, arg: str = "") -> str:
    """Call a content provider method."""
    cmd = f"content call --uri {uri} --method {method}"
    if arg: cmd += f" --arg {arg}"
    return _run(["sh", "-c", cmd], timeout=15)


# ════════════════════════════════════════════════════════════════════════════
# SYSTEM INTERNALS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def android_properties(filter_str: str = "") -> str:
    """Dump Android system properties (getprop). filter_str narrows output."""
    out = _run(["getprop"])
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No props matching '{filter_str}'"
    return out


@mcp.tool()
def android_build_info() -> str:
    """Return structured Android build fingerprint and version info."""
    keys = [
        "ro.product.model", "ro.product.brand", "ro.product.manufacturer",
        "ro.build.version.release", "ro.build.version.sdk",
        "ro.build.version.codename", "ro.build.fingerprint",
        "ro.build.date", "ro.product.cpu.abi", "ro.product.cpu.abilist",
        "ro.build.type", "ro.debuggable", "ro.secure",
        "ro.build.tags", "persist.sys.language", "persist.sys.country",
    ]
    info = {k: _prop(k) for k in keys}
    return json.dumps(info, indent=2)


@mcp.tool()
def android_cpu_info() -> str:
    """Parsed CPU info from /proc/cpuinfo and lscpu."""
    out = _run(["cat", "/proc/cpuinfo"])
    lscpu = _run(["lscpu"])
    return f"## /proc/cpuinfo\n{out[:3000]}\n\n## lscpu\n{lscpu}"


@mcp.tool()
def android_memory_info() -> str:
    """Parsed memory info from /proc/meminfo."""
    return _run(["cat", "/proc/meminfo"])


@mcp.tool()
def dumpsys(service: str, args: str = "") -> str:
    """
    Run dumpsys for an Android service. Common services: battery, wifi,
    cpuinfo, meminfo, activity, package, window, input, display,
    connectivity, telephony.registry, sensorservice.
    """
    cmd = f"dumpsys {service} {args}"
    return _run(["sh", "-c", cmd], timeout=20)[:6000]


@mcp.tool()
def android_thermal() -> str:
    """Read thermal zone temperatures from /sys/class/thermal/."""
    zones = sorted(Path("/sys/class/thermal").glob("thermal_zone*"))
    lines = []
    for z in zones:
        try:
            typ  = (z / "type").read_text().strip()
            temp = int((z / "temp").read_text().strip()) / 1000
            lines.append(f"{z.name:20s} {typ:30s} {temp:.1f}°C")
        except Exception:
            pass
    return "\n".join(lines) or "No thermal zones found"


@mcp.tool()
def android_storage_volumes() -> str:
    """List mounted storage volumes from /proc/mounts."""
    out = _run(["cat", "/proc/mounts"])
    interesting = [l for l in out.splitlines()
                   if any(kw in l for kw in ("/sdcard", "/storage", "/mnt", "/data"))]
    return "\n".join(interesting) or out[:2000]


@mcp.tool()
def proc_status(pid: int) -> str:
    """Read /proc/<pid>/status for a process."""
    try:
        return Path(f"/proc/{pid}/status").read_text()
    except Exception as e:
        return f"Cannot read /proc/{pid}/status: {e}"


@mcp.tool()
def proc_maps(pid: int) -> str:
    """Read memory maps for a process from /proc/<pid>/maps."""
    try:
        return Path(f"/proc/{pid}/maps").read_text()[:8000]
    except Exception as e:
        return f"Cannot read /proc/{pid}/maps: {e}"


@mcp.tool()
def proc_environ(pid: int) -> str:
    """Read environment variables for a process (null-delimited → newlines)."""
    try:
        raw = Path(f"/proc/{pid}/environ").read_bytes()
        return raw.replace(b"\x00", b"\n").decode(errors="replace")[:4000]
    except Exception as e:
        return f"Cannot read /proc/{pid}/environ: {e}"


# ════════════════════════════════════════════════════════════════════════════
# HARDWARE / SENSORS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def battery_info_sys() -> str:
    """Battery info from /sys/class/power_supply and dumpsys battery."""
    lines = ["## dumpsys battery"]
    lines.append(_run(["sh", "-c", "dumpsys battery"], timeout=10)[:1000])
    psu = Path("/sys/class/power_supply")
    if psu.exists():
        lines.append("\n## /sys/class/power_supply")
        for supply in psu.iterdir():
            lines.append(f"\n### {supply.name}")
            for attr in ("capacity", "status", "health", "voltage_now",
                         "current_now", "temp", "technology", "cycle_count"):
                f = supply / attr
                if f.exists():
                    try:
                        lines.append(f"  {attr}: {f.read_text().strip()}")
                    except Exception:
                        pass
    return "\n".join(lines)


@mcp.tool()
def wifi_info_sysfs() -> str:
    """WiFi info from /sys/class/net and dumpsys wifi."""
    out = _run(["sh", "-c", "dumpsys wifi | head -100"], timeout=15)
    ifaces = _run(["ip", "link", "show"])
    return f"## ip link\n{ifaces}\n\n## dumpsys wifi (head)\n{out}"


@mcp.tool()
def camera_info_sys() -> str:
    """Camera hardware info from dumpsys media.camera and /sys."""
    out = _run(["sh", "-c", "dumpsys media.camera"], timeout=15)
    return out[:4000]


@mcp.tool()
def bluetooth_info() -> str:
    """Bluetooth status from dumpsys bluetooth_manager."""
    return _run(["sh", "-c", "dumpsys bluetooth_manager | head -80"], timeout=15)


@mcp.tool()
def sensor_list() -> str:
    """List sensors from dumpsys sensorservice."""
    return _run(["sh", "-c", "dumpsys sensorservice | head -100"], timeout=15)


@mcp.tool()
def display_info() -> str:
    """Display info: resolution, density, refresh rate from dumpsys window."""
    return _run(["sh", "-c", "dumpsys window | grep -E 'mDisplayInfo|mStable|mBase|displayId'"], timeout=10)


# ════════════════════════════════════════════════════════════════════════════
# SECURITY
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def selinux_status() -> str:
    """SELinux enforcement status and policy info."""
    enforcing = _run(["getenforce"])
    sestatus  = _run(["sestatus"])
    if sestatus.startswith("[not found"):
        sestatus = _run(["sh", "-c", "cat /sys/fs/selinux/enforce 2>/dev/null"])
    return f"getenforce: {enforcing}\n\n{sestatus}"


@mcp.tool()
def check_root() -> str:
    """Check if the device is rooted (su, Magisk, SuperSU detection)."""
    checks = {}
    checks["su_in_path"]    = _run(["which", "su"])
    checks["su_exists"]     = str(any(Path(p).exists() for p in
                                      ["/sbin/su","/system/bin/su","/system/xbin/su"]))
    checks["magisk_db"]     = str(Path("/data/adb/magisk.db").exists())
    checks["supersu"]       = str(Path("/system/xbin/daemonsu").exists())
    checks["debuggable"]    = _prop("ro.debuggable")
    checks["secure"]        = _prop("ro.secure")
    checks["build_type"]    = _prop("ro.build.type")
    return json.dumps(checks, indent=2)


@mcp.tool()
def apk_cert_info(apk_path: str) -> str:
    """Extract and display certificate info from an APK's signing block."""
    p = Path(apk_path).expanduser()
    if not p.exists(): return f"Not found: {apk_path}"
    out = _run(["apksigner", "verify", "--print-certs", "--verbose", str(p)], timeout=30)
    if out.startswith("[not found"):
        out = _run(["keytool", "-printcert", "-jarfile", str(p)], timeout=30)
    return out


@mcp.tool()
def keystore_list(keystore_path: str = "") -> str:
    """List certificates in a Java keystore. Empty path tries debug.keystore."""
    ks = keystore_path or str(Path.home() / ".android" / "debug.keystore")
    return _run(["keytool", "-list", "-v", "-keystore", ks,
                 "-storepass", "android"], timeout=15)


# ════════════════════════════════════════════════════════════════════════════
# BINARY ANALYSIS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def elf_info(binary_path: str) -> str:
    """ELF header, sections, and dynamic dependencies for a binary."""
    p = Path(binary_path).expanduser()
    if not p.exists(): return f"Not found: {binary_path}"
    header   = _run(["readelf", "-h", str(p)])
    sections = _run(["readelf", "-S", "--wide", str(p)])
    dynamic  = _run(["readelf", "-d", str(p)])
    symbols  = _run(["readelf", "--syms", "--wide", str(p)])[:2000]
    return f"## ELF Header\n{header}\n\n## Sections\n{sections}\n\n## Dynamic\n{dynamic}\n\n## Symbols (head)\n{symbols}"


@mcp.tool()
def strings_binary(path: str, min_len: int = 6,
                   filter_str: str = "") -> str:
    """Extract printable strings from a binary file."""
    p = Path(path).expanduser()
    if not p.exists(): return f"Not found: {path}"
    out = _run(["strings", f"-n{min_len}", str(p)], timeout=30)
    if filter_str:
        matches = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(matches[:500]) or f"No strings matching '{filter_str}'"
    return "\n".join(out.splitlines()[:500])


@mcp.tool()
def ldd_check(binary_path: str) -> str:
    """Show shared library dependencies. Uses readelf -d as ldd fallback."""
    p = Path(binary_path).expanduser()
    if not p.exists(): return f"Not found: {binary_path}"
    out = _run(["ldd", str(p)])
    if out.startswith("[not found"):
        out = _run(["readelf", "-d", str(p)])
    return out


@mcp.tool()
def objdump_disasm(binary_path: str, section: str = ".text",
                   max_lines: int = 200) -> str:
    """Disassemble a section of a binary using objdump."""
    p = Path(binary_path).expanduser()
    if not p.exists(): return f"Not found: {binary_path}"
    out = _run(["objdump", "-d", f"--section={section}", str(p)], timeout=30)
    return "\n".join(out.splitlines()[:max_lines])


# ════════════════════════════════════════════════════════════════════════════
# BUS BRIDGE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def bus_call_termux(tool: str, args_json: str = "{}") -> str:
    """Call a tool on the Termux MCP server via the bidirectional bus.
    tool: name registered on the Termux side. args_json: JSON object string."""
    if not _bus.is_connected():
        return "Bus not connected to Termux peer. Is gorkbot_termux running?"
    try:
        args = json.loads(args_json)
        return _bus.call_remote(tool, args, timeout=45)
    except json.JSONDecodeError as e:
        return f"Invalid args JSON: {e}"
    except Exception as e:
        return f"Bus call error: {e}"


@mcp.tool()
def bus_emit_event(event_name: str, data_json: str = "{}") -> str:
    """Emit an event to the Termux peer via the bus (fire-and-forget)."""
    try:
        data = json.loads(data_json)
        _bus.emit(event_name, data)
        return f"Event '{event_name}' emitted to Termux"
    except Exception as e:
        return f"Error: {e}"


@mcp.tool()
def bus_status() -> str:
    """Show bidirectional bus connection status and recent events."""
    connected  = _bus.is_connected()
    local_tools = _bus.list_local_tools()
    recent_evts = _bus.recent_events(10)
    lines = [
        f"**Bus status**: {'🟢 Connected' if connected else '🔴 Disconnected'}",
        f"**Role**: {_bus.role}",
        f"**Local tools exposed**: {', '.join(local_tools)}",
        f"\n**Recent events (last 10)**:",
    ]
    for e in recent_evts:
        lines.append(f"  [{e['ts']}] {e['src']}/{e['name']}: {e['data']}")
    if not recent_evts:
        lines.append("  (none)")
    return "\n".join(lines)


# ════════════════════════════════════════════════════════════════════════════
# GENERAL
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def android_quick_status() -> str:
    """Single-call snapshot: model, SDK, memory, battery, network, storage."""
    return json.dumps({
        "model":    _prop("ro.product.model"),
        "android":  _prop("ro.build.version.release"),
        "sdk":      _prop("ro.build.version.sdk"),
        "abi":      _prop("ro.product.cpu.abi"),
        "debuggable": _prop("ro.debuggable"),
        "mem_total_kb": _run(["sh", "-c", "grep MemTotal /proc/meminfo | awk '{print $2}'"]),
        "mem_free_kb":  _run(["sh", "-c", "grep MemAvailable /proc/meminfo | awk '{print $2}'"]),
        "battery_pct":  _run(["sh", "-c", "cat /sys/class/power_supply/*/capacity 2>/dev/null | head -1"]),
        "uptime":   _run(["uptime"]),
        "df_home":  _run(["df", "-h", str(Path.home())]),
    }, indent=2)


if __name__ == "__main__":
    mcp.run()
