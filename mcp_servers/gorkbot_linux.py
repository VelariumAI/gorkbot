#!/usr/bin/env python3
"""
gorkbot_linux — Comprehensive, distro-adaptive Linux MCP server.

Auto-detects: Ubuntu/Debian (apt), Fedora/RHEL/CentOS (dnf/yum),
              Arch/Manjaro (pacman), openSUSE (zypper), Alpine (apk),
              NixOS (nix).

Covers:
  • Package management    (distro-adaptive, snap, flatpak)
  • systemd               (units, journals, timers, sockets, cgroups)
  • Processes             (list, kill, tree, lsof, strace, perf)
  • Storage               (lsblk, smart, LVM, RAID, BTRFS/ZFS)
  • Networking            (ip, ss, iptables/nftables/firewalld/ufw, DNS)
  • Containers            (docker, podman, LXC, virsh)
  • Hardware              (CPU, RAM, PCI, USB, DMI, sensors, GPU)
  • Kernel                (modules, sysctl, dmesg, kallsyms)
  • Security              (SELinux, AppArmor, sudoers, auditd, fail2ban)
  • Users & cron          (accounts, login history, crontab, at)
  • Performance           (cpu, mem, io, network, perf stat)
  • Build / dev tools     (make, cmake, gcc, clang, rust, go, python)
  • Debugging             (gdb, strace, ltrace, valgrind)
"""

import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Optional

try:
    from fastmcp import FastMCP
except ImportError:
    print("fastmcp not installed. Run: pip install fastmcp", file=sys.stderr)
    sys.exit(1)

mcp = FastMCP("gorkbot-linux")


# ── Helpers ──────────────────────────────────────────────────────────────────

def _run(cmd: list[str], timeout: int = 30, cwd: Optional[str] = None,
         input_data: Optional[str] = None, root: bool = False) -> str:
    if root and os.geteuid() != 0:
        cmd = ["sudo", "-n"] + cmd
    try:
        r = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout,
            cwd=cwd, input=input_data, env={**os.environ},
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
        return f"[not found: {cmd[0] if not root else cmd[2]}]"
    except Exception as e:
        return f"[error: {e}]"


def _present(cmd: str) -> bool:
    return shutil.which(cmd) is not None


# ── Distro detection ──────────────────────────────────────────────────────────

def _detect_distro() -> dict:
    info: dict = {"id": "unknown", "like": "", "pm": "unknown", "version": ""}
    try:
        for line in Path("/etc/os-release").read_text().splitlines():
            if line.startswith("ID="):
                info["id"] = line.split("=",1)[1].strip('"').lower()
            elif line.startswith("ID_LIKE="):
                info["like"] = line.split("=",1)[1].strip('"').lower()
            elif line.startswith("VERSION_ID="):
                info["version"] = line.split("=",1)[1].strip('"')
    except Exception:
        pass
    # Determine package manager
    for pm, check in [("apt","apt"),("dnf","dnf"),("yum","yum"),
                      ("pacman","pacman"),("zypper","zypper"),
                      ("apk","apk"),("nix","nix-env"),("emerge","emerge")]:
        if _present(check):
            info["pm"] = pm
            break
    return info

_DISTRO = _detect_distro()


def _pkg_cmd(action: str, packages: list[str] | None = None) -> list[str]:
    """Return the package manager command for an action."""
    pm = _DISTRO["pm"]
    if pm == "apt":
        cmds = {"install": ["apt-get", "install", "-y"],
                "remove":  ["apt-get", "remove", "-y"],
                "search":  ["apt-cache", "search"],
                "info":    ["apt-cache", "show"],
                "list":    ["dpkg-query", "-W", "-f=${Package} ${Version}\n"],
                "upgrade": ["apt-get", "upgrade", "-y"],
                "update":  ["apt-get", "update"]}
    elif pm in ("dnf", "yum"):
        cmds = {"install": [pm, "install", "-y"],
                "remove":  [pm, "remove", "-y"],
                "search":  [pm, "search"],
                "info":    [pm, "info"],
                "list":    [pm, "list", "installed"],
                "upgrade": [pm, "upgrade", "-y"],
                "update":  [pm, "check-update"]}
    elif pm == "pacman":
        cmds = {"install": ["pacman", "-S", "--noconfirm"],
                "remove":  ["pacman", "-R", "--noconfirm"],
                "search":  ["pacman", "-Ss"],
                "info":    ["pacman", "-Si"],
                "list":    ["pacman", "-Q"],
                "upgrade": ["pacman", "-Syu", "--noconfirm"],
                "update":  ["pacman", "-Sy"]}
    elif pm == "zypper":
        cmds = {"install": ["zypper", "install", "-y"],
                "remove":  ["zypper", "remove", "-y"],
                "search":  ["zypper", "search"],
                "info":    ["zypper", "info"],
                "list":    ["zypper", "packages", "--installed-only"],
                "upgrade": ["zypper", "update", "-y"],
                "update":  ["zypper", "refresh"]}
    elif pm == "apk":
        cmds = {"install": ["apk", "add"],
                "remove":  ["apk", "del"],
                "search":  ["apk", "search"],
                "info":    ["apk", "info"],
                "list":    ["apk", "info", "-v"],
                "upgrade": ["apk", "upgrade"],
                "update":  ["apk", "update"]}
    else:
        return [pm, action] + (packages or [])
    cmd = cmds.get(action, [pm, action])
    if packages and action in ("install", "remove", "search", "info"):
        cmd = cmd + packages
    return cmd


# ════════════════════════════════════════════════════════════════════════════
# PACKAGE MANAGEMENT
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def distro_info() -> str:
    """Return detected Linux distribution and package manager."""
    return json.dumps(_DISTRO, indent=2)


@mcp.tool()
def pkg_list_installed(filter_str: str = "") -> str:
    """List installed packages (distro-adaptive)."""
    out = _run(_pkg_cmd("list"), timeout=30)
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines) or f"No packages matching '{filter_str}'"
    return out[:8000]


@mcp.tool()
def pkg_search(query: str) -> str:
    """Search available packages (distro-adaptive)."""
    return _run(_pkg_cmd("search", [query]), timeout=30)[:4000]


@mcp.tool()
def pkg_info(package: str) -> str:
    """Show package details: description, version, dependencies."""
    return _run(_pkg_cmd("info", [package]), timeout=15)


@mcp.tool()
def pkg_install(packages: str, confirm: bool = False) -> str:
    """Install packages (distro-adaptive). confirm=True required."""
    if not confirm:
        return f"Would install: {packages} (via {_DISTRO['pm']}). Set confirm=true."
    cmd = _pkg_cmd("install", packages.split())
    return _run(cmd, timeout=300, root=True)


@mcp.tool()
def pkg_remove(packages: str, confirm: bool = False) -> str:
    """Remove packages (distro-adaptive). confirm=True required."""
    if not confirm:
        return f"Would remove: {packages}. Set confirm=true."
    return _run(_pkg_cmd("remove", packages.split()), timeout=120, root=True)


@mcp.tool()
def pkg_upgrade(confirm: bool = False) -> str:
    """Upgrade all system packages. confirm=True required."""
    if not confirm:
        return f"Would upgrade all packages via {_DISTRO['pm']}. Set confirm=true."
    _run(_pkg_cmd("update"), timeout=60, root=True)
    return _run(_pkg_cmd("upgrade"), timeout=600, root=True)


@mcp.tool()
def snap_list(filter_str: str = "") -> str:
    """List installed snap packages."""
    out = _run(["snap", "list"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def snap_install(name: str, channel: str = "stable",
                 classic: bool = False, confirm: bool = False) -> str:
    """Install a snap package. confirm=True required."""
    if not confirm: return f"Would snap install '{name}'. Set confirm=true."
    cmd = ["snap", "install", name, f"--channel={channel}"]
    if classic: cmd.append("--classic")
    return _run(cmd, timeout=120, root=True)


@mcp.tool()
def flatpak_list(filter_str: str = "") -> str:
    """List installed flatpak applications."""
    out = _run(["flatpak", "list"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def flatpak_install(ref: str, confirm: bool = False) -> str:
    """Install a flatpak app. ref: e.g. 'flathub org.gimp.GIMP'. confirm=True required."""
    if not confirm: return f"Would install flatpak '{ref}'. Set confirm=true."
    parts = ref.split()
    if len(parts) == 2:
        return _run(["flatpak", "install", "-y", parts[0], parts[1]], timeout=300)
    return _run(["flatpak", "install", "-y", ref], timeout=300)


# ════════════════════════════════════════════════════════════════════════════
# SYSTEMD
# ════════════════════════════════════════════════════════════════════════════

def _sysd(args: list[str], root: bool = False, timeout: int = 15) -> str:
    cmd = ["systemctl"] + args
    return _run(cmd, timeout=timeout, root=root)


@mcp.tool()
def service_status(name: str) -> str:
    """Detailed systemd service status (state, PID, memory, recent logs)."""
    status = _sysd(["status", name, "--no-pager", "-l"])
    return status


@mcp.tool()
def service_start(name: str, confirm: bool = False) -> str:
    """Start a systemd service. confirm=True required."""
    if not confirm: return f"Would start '{name}'. Set confirm=true."
    return _sysd(["start", name], root=True)


@mcp.tool()
def service_stop(name: str, confirm: bool = False) -> str:
    """Stop a systemd service. confirm=True required."""
    if not confirm: return f"Would stop '{name}'. Set confirm=true."
    return _sysd(["stop", name], root=True)


@mcp.tool()
def service_restart(name: str, confirm: bool = False) -> str:
    """Restart a systemd service. confirm=True required."""
    if not confirm: return f"Would restart '{name}'. Set confirm=true."
    return _sysd(["restart", name], root=True)


@mcp.tool()
def service_enable(name: str, confirm: bool = False) -> str:
    """Enable a systemd service to start at boot. confirm=True required."""
    if not confirm: return f"Would enable '{name}'. Set confirm=true."
    return _sysd(["enable", "--now", name], root=True)


@mcp.tool()
def service_disable(name: str, confirm: bool = False) -> str:
    """Disable a systemd service from starting at boot. confirm=True required."""
    if not confirm: return f"Would disable '{name}'. Set confirm=true."
    return _sysd(["disable", "--now", name], root=True)


@mcp.tool()
def service_list(state: str = "all", unit_type: str = "service") -> str:
    """
    List systemd units. state: all|running|failed|inactive.
    unit_type: service|socket|timer|mount|target.
    """
    args = ["list-units", f"--type={unit_type}", "--no-pager", "--no-legend"]
    if state == "failed":
        args = ["--failed", "--no-pager", "--no-legend"]
    elif state != "all":
        args += [f"--state={state}"]
    return _sysd(args)[:6000]


@mcp.tool()
def service_logs(name: str, lines: int = 50, since: str = "",
                 until: str = "", priority: str = "") -> str:
    """
    Read systemd journal for a service. priority: emerg|alert|crit|err|warning|notice|info|debug.
    since/until: '2h ago', 'yesterday', '2024-01-15 12:00' etc.
    """
    cmd = ["journalctl", "-u", name, "-n", str(lines), "--no-pager"]
    if since:    cmd += ["--since", since]
    if until:    cmd += ["--until", until]
    if priority: cmd += [f"-p {priority}"]
    return _run(cmd)


@mcp.tool()
def timer_list() -> str:
    """List all systemd timers and their next activation time."""
    return _sysd(["list-timers", "--all", "--no-pager"])


@mcp.tool()
def socket_list() -> str:
    """List systemd socket units and their listen addresses."""
    return _sysd(["list-sockets", "--no-pager"])


@mcp.tool()
def cgroup_info(name: str = "") -> str:
    """Show cgroup hierarchy or specific cgroup details."""
    if name:
        return _run(["systemctl", "show", name, "--property=ControlGroup,MemoryCurrent,CPUUsageNSec"])
    return _run(["systemd-cgls", "--no-pager"])[:4000]


# ════════════════════════════════════════════════════════════════════════════
# JOURNAL / LOGS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def journal_query(unit: str = "", since: str = "1h ago", until: str = "",
                  priority: str = "warning", grep: str = "",
                  limit: int = 100) -> str:
    """
    Query systemd journal. priority: emerg|alert|crit|err|warning|notice|info|debug.
    grep: regex pattern to filter messages.
    """
    cmd = ["journalctl", "--no-pager", "-n", str(limit)]
    if unit:     cmd += ["-u", unit]
    if since:    cmd += ["--since", since]
    if until:    cmd += ["--until", until]
    if priority: cmd += [f"-p", priority]
    if grep:     cmd += ["--grep", grep]
    return _run(cmd)[:6000]


@mcp.tool()
def journal_boot_list() -> str:
    """List available journal boot sessions."""
    return _run(["journalctl", "--list-boots", "--no-pager"])


@mcp.tool()
def dmesg_tail(lines: int = 50, level: str = "",
               filter_str: str = "") -> str:
    """
    Read kernel ring buffer. level: emerg|alert|crit|err|warn|notice|info|debug.
    """
    cmd = ["dmesg", "--time-format=iso", "-H"]
    if level: cmd += [f"--level={level}"]
    out = _run(cmd)
    if filter_str:
        matches = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(matches[-lines:])
    return "\n".join(out.splitlines()[-lines:])


@mcp.tool()
def log_tail(log_file: str, lines: int = 50, grep: str = "") -> str:
    """Tail a log file from /var/log/ or any path."""
    p = Path(log_file)
    if not p.is_absolute():
        p = Path("/var/log") / log_file
    if not p.exists():
        return f"Log not found: {p}"
    cmd = ["tail", "-n", str(lines), str(p)]
    out = _run(cmd)
    if grep:
        matches = [l for l in out.splitlines() if grep.lower() in l.lower()]
        return "\n".join(matches) or f"No lines matching '{grep}'"
    return out


# ════════════════════════════════════════════════════════════════════════════
# PROCESSES
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def process_list(filter_str: str = "", sort_by: str = "cpu",
                 full_cmd: bool = True) -> str:
    """List processes. sort_by: cpu|mem|pid|name. filter_str: name substring."""
    flags = {"cpu": "--sort=-%cpu", "mem": "--sort=-%mem",
             "pid": "--sort=pid",   "name": "--sort=comm"}.get(sort_by, "--sort=-%cpu")
    fmt   = "pid,ppid,user,%cpu,%mem,vsz,rss,stat,comm" + (",args" if full_cmd else "")
    out   = _run(["ps", "axo", fmt, flags, "--no-header"])
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines[:100]) or f"No processes matching '{filter_str}'"
    return "\n".join(out.splitlines()[:100])


@mcp.tool()
def process_tree(pid: int = 0) -> str:
    """Show process tree. pid=0 for full tree."""
    if _present("pstree"):
        cmd = ["pstree", "-p", "-a"] + ([str(pid)] if pid else [])
        return _run(cmd)[:6000]
    return _run(["ps", "axjf"])[:6000]


@mcp.tool()
def process_kill(pid: int, signal: str = "TERM",
                 confirm: bool = False) -> str:
    """Kill a process. signal: TERM|KILL|HUP|INT|USR1|USR2. confirm=True required."""
    if not confirm:
        return f"Would send SIG{signal} to PID {pid}. Set confirm=true."
    return _run(["kill", f"-{signal}", str(pid)])


@mcp.tool()
def process_info(pid: int) -> str:
    """Detailed process info from /proc/<pid>."""
    base = Path(f"/proc/{pid}")
    if not base.exists(): return f"Process {pid} not found"
    result: dict = {}
    for fname in ("status", "cmdline", "comm", "cwd", "exe"):
        try:
            data = (base / fname).read_bytes()
            result[fname] = data.replace(b"\x00", b" ").decode(errors="replace").strip()[:500]
        except Exception:
            pass
    try: result["fd_count"] = len(list((base / "fd").iterdir()))
    except Exception: pass
    try: result["threads"] = len(list((base / "task").iterdir()))
    except Exception: pass
    return json.dumps(result, indent=2)


@mcp.tool()
def process_open_files(pid: int = 0, filter_str: str = "") -> str:
    """List open files for a process (or all processes if pid=0) using lsof."""
    cmd = ["lsof"] + (["-p", str(pid)] if pid else [])
    out = _run(cmd, timeout=20)
    if filter_str:
        lines = [l for l in out.splitlines() if filter_str.lower() in l.lower()]
        return "\n".join(lines[:200]) or f"No matches for '{filter_str}'"
    return "\n".join(out.splitlines()[:200])


@mcp.tool()
def process_maps(pid: int) -> str:
    """Memory maps for a process from /proc/<pid>/maps."""
    try:
        return Path(f"/proc/{pid}/maps").read_text()[:6000]
    except Exception as e:
        return f"Cannot read maps: {e}"


@mcp.tool()
def strace_run(command: str, duration: int = 5,
               filter_syscalls: str = "", confirm: bool = False) -> str:
    """
    Trace system calls of a command using strace.
    filter_syscalls: comma-separated syscall names (e.g. 'open,read,write').
    confirm=True required.
    """
    if not confirm: return f"Would strace: {command}. Set confirm=true."
    cmd = ["strace", "-T", "-f", "-e", "trace=all"]
    if filter_syscalls: cmd = ["strace", "-T", "-f", "-e", f"trace={filter_syscalls}"]
    cmd += ["-c", "--"] + command.split()
    return _run(cmd, timeout=duration + 10)


# ════════════════════════════════════════════════════════════════════════════
# STORAGE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def disk_list() -> str:
    """List block devices with filesystem info (lsblk)."""
    return _run(["lsblk", "-o", "NAME,TYPE,SIZE,FSTYPE,LABEL,MOUNTPOINT,MODEL", "--no-legend"])


@mcp.tool()
def disk_usage() -> str:
    """Filesystem disk usage (df -h) and inode usage (df -i)."""
    space  = _run(["df", "-h", "--output=source,size,used,avail,pcent,target"])
    inodes = _run(["df", "-i", "--output=source,iused,ifree,ipcent,target"])
    return f"## Space\n{space}\n\n## Inodes\n{inodes}"


@mcp.tool()
def disk_smart(device: str) -> str:
    """SMART health info for a disk. device: /dev/sda, /dev/nvme0 etc."""
    return _run(["smartctl", "-a", device], root=True)[:4000]


@mcp.tool()
def mount_list() -> str:
    """All mounted filesystems with options."""
    return _run(["findmnt", "--real", "--output=TARGET,SOURCE,FSTYPE,SIZE,OPTIONS"])


@mcp.tool()
def lvm_info() -> str:
    """LVM physical volumes, volume groups, and logical volumes."""
    if not _present("pvs"): return "LVM tools not installed (install lvm2)"
    pvs = _run(["pvs"], root=True)
    vgs = _run(["vgs"], root=True)
    lvs = _run(["lvs"], root=True)
    return f"## Physical Volumes\n{pvs}\n\n## Volume Groups\n{vgs}\n\n## Logical Volumes\n{lvs}"


@mcp.tool()
def raid_status() -> str:
    """Software RAID status from mdadm."""
    if not _present("mdadm"): return "mdadm not installed"
    return _run(["mdadm", "--detail", "--scan", "--verbose"], root=True)


# ════════════════════════════════════════════════════════════════════════════
# NETWORKING
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def net_info() -> str:
    """Network interfaces, routes, and neighbor table."""
    addrs   = _run(["ip", "-br", "addr"])
    routes  = _run(["ip", "route"])
    neigh   = _run(["ip", "neigh"])
    return f"## Addresses\n{addrs}\n\n## Routes\n{routes}\n\n## ARP/Neighbors\n{neigh}"


@mcp.tool()
def connections(state: str = "", local_port: int = 0,
                process: bool = True) -> str:
    """TCP/UDP connections via ss. state: LISTEN|ESTABLISHED|TIME-WAIT etc."""
    cmd = ["ss", "-tulnp" if process else "-tuln"]
    if state:      cmd.append(f"state {state}")
    if local_port: cmd.append(f"sport = :{local_port}")
    return _run(cmd)


@mcp.tool()
def iptables_list(table: str = "filter", chain: str = "") -> str:
    """List iptables rules. table: filter|nat|mangle|raw."""
    cmd = ["iptables", "-t", table, "-L", "-n", "-v", "--line-numbers"]
    if chain: cmd.append(chain)
    return _run(cmd, root=True)


@mcp.tool()
def nftables_list() -> str:
    """List nftables ruleset."""
    return _run(["nft", "list", "ruleset"], root=True)


@mcp.tool()
def firewalld_status(zone: str = "") -> str:
    """firewalld zone status. zone: empty for active zones."""
    if not _present("firewall-cmd"): return "firewalld not installed"
    cmd = ["firewall-cmd", "--state"] if not zone else ["firewall-cmd", f"--zone={zone}", "--list-all"]
    return _run(cmd)


@mcp.tool()
def ufw_status() -> str:
    """UFW (Uncomplicated Firewall) status and rules."""
    if not _present("ufw"): return "ufw not installed"
    return _run(["ufw", "status", "numbered"], root=True)


@mcp.tool()
def dns_query(hostname: str, record_type: str = "A",
              server: str = "") -> str:
    """DNS lookup with dig. record_type: A|AAAA|MX|TXT|NS|CNAME|SOA|PTR."""
    cmd = ["dig"]
    if server: cmd.append(f"@{server}")
    cmd += [hostname, record_type, "+short"]
    return _run(cmd, timeout=10)


@mcp.tool()
def bandwidth_usage(interface: str = "", duration: int = 3) -> str:
    """Network bandwidth usage snapshot (ifstat or ip -s link)."""
    if _present("ifstat"):
        return _run(["ifstat", "-i", interface or "all", str(duration), "1"], timeout=duration + 5)
    return _run(["ip", "-s", "link", "show"] + ([interface] if interface else []))


# ════════════════════════════════════════════════════════════════════════════
# CONTAINERS / VIRTUALIZATION
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def docker_ps(all_containers: bool = True) -> str:
    """List Docker containers."""
    cmd = ["docker", "ps", "--format", "table {{.ID}}\\t{{.Image}}\\t{{.Status}}\\t{{.Names}}\\t{{.Ports}}"]
    if all_containers: cmd.append("-a")
    return _run(cmd)


@mcp.tool()
def docker_images(filter_str: str = "") -> str:
    """List Docker images."""
    out = _run(["docker", "images"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def docker_logs(container: str, tail: int = 50,
                since: str = "", timestamps: bool = True) -> str:
    """Fetch Docker container logs."""
    cmd = ["docker", "logs", f"--tail={tail}"]
    if timestamps: cmd.append("-t")
    if since:      cmd += ["--since", since]
    cmd.append(container)
    return _run(cmd, timeout=15)


@mcp.tool()
def docker_exec(container: str, command: str,
                confirm: bool = False) -> str:
    """Execute a command in a running Docker container. confirm=True required."""
    if not confirm: return f"Would exec in '{container}': {command}. Set confirm=true."
    return _run(["docker", "exec", container, "sh", "-c", command], timeout=30)


@mcp.tool()
def docker_stats() -> str:
    """Docker container resource usage (non-streaming snapshot)."""
    return _run(["docker", "stats", "--no-stream",
                 "--format", "table {{.Name}}\\t{{.CPUPerc}}\\t{{.MemUsage}}\\t{{.NetIO}}\\t{{.BlockIO}}"])


@mcp.tool()
def docker_inspect(container_or_image: str) -> str:
    """Inspect a Docker container or image (JSON config)."""
    return _run(["docker", "inspect", container_or_image])[:6000]


@mcp.tool()
def podman_ps(all_containers: bool = True) -> str:
    """List Podman containers."""
    cmd = ["podman", "ps"]
    if all_containers: cmd.append("-a")
    return _run(cmd)


@mcp.tool()
def virsh_list() -> str:
    """List libvirt/KVM virtual machines."""
    if not _present("virsh"): return "virsh not installed (install libvirt)"
    return _run(["virsh", "list", "--all"])


@mcp.tool()
def namespace_list() -> str:
    """List Linux namespaces (lsns)."""
    return _run(["lsns"], root=True)


# ════════════════════════════════════════════════════════════════════════════
# HARDWARE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def cpu_info() -> str:
    """CPU details: model, cores, cache, frequency scaling."""
    cpuinfo = _run(["cat", "/proc/cpuinfo"])[:3000]
    lscpu   = _run(["lscpu"])
    freq    = _run(["cat", "/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"])
    return f"## lscpu\n{lscpu}\n\n## Governor: {freq}\n\n## /proc/cpuinfo (head)\n{cpuinfo}"


@mcp.tool()
def memory_info() -> str:
    """Detailed memory info: total, free, buffers, swap, huge pages."""
    meminfo = _run(["cat", "/proc/meminfo"])
    free    = _run(["free", "-h"])
    return f"## free -h\n{free}\n\n## /proc/meminfo\n{meminfo}"


@mcp.tool()
def pci_devices(filter_str: str = "") -> str:
    """List PCI devices (lspci). filter_str narrows output."""
    out = _run(["lspci", "-vvv"] if filter_str else ["lspci"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out[:6000]


@mcp.tool()
def usb_devices(filter_str: str = "") -> str:
    """List USB devices (lsusb). filter_str narrows output."""
    out = _run(["lsusb"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def dmi_info(dmi_type: str = "") -> str:
    """DMI/SMBIOS hardware info (dmidecode). dmi_type: bios|system|baseboard|chassis|processor|memory."""
    cmd = ["dmidecode"] + (["-t", dmi_type] if dmi_type else [])
    return _run(cmd, root=True, timeout=10)[:6000]


@mcp.tool()
def gpu_info() -> str:
    """GPU info: lspci + nvidia-smi or rocm-smi if available."""
    pci  = _run(["lspci", "-vnn"])
    gpus = [l for l in pci.splitlines() if re.search(r"VGA|3D|Display|NVIDIA|AMD|Intel", l, re.I)]
    lines = ["## GPU devices (lspci)"] + gpus
    if _present("nvidia-smi"):
        lines += ["\n## nvidia-smi", _run(["nvidia-smi"])]
    elif _present("rocm-smi"):
        lines += ["\n## rocm-smi", _run(["rocm-smi"])]
    return "\n".join(lines)


@mcp.tool()
def temperature_sensors() -> str:
    """Hardware temperatures from lm-sensors or /sys/class/thermal."""
    if _present("sensors"):
        return _run(["sensors"])
    # fallback to /sys
    zones = sorted(Path("/sys/class/thermal").glob("thermal_zone*"))
    lines = []
    for z in zones:
        try:
            typ  = (z / "type").read_text().strip()
            temp = int((z / "temp").read_text().strip()) / 1000
            lines.append(f"{typ:35s} {temp:.1f}°C")
        except Exception:
            pass
    return "\n".join(lines) or "No thermal data"


# ════════════════════════════════════════════════════════════════════════════
# KERNEL
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def kernel_modules(filter_str: str = "") -> str:
    """List loaded kernel modules."""
    out = _run(["lsmod"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out


@mcp.tool()
def module_info(name: str) -> str:
    """Show detailed info for a kernel module."""
    return _run(["modinfo", name])


@mcp.tool()
def module_load(name: str, params: str = "",
                confirm: bool = False) -> str:
    """Load a kernel module. confirm=True required."""
    if not confirm: return f"Would modprobe '{name}'. Set confirm=true."
    cmd = ["modprobe", name] + (params.split() if params else [])
    return _run(cmd, root=True)


@mcp.tool()
def sysctl_get(key: str = "") -> str:
    """Read sysctl kernel parameters. key empty = all parameters."""
    cmd = ["sysctl", key] if key else ["sysctl", "-a"]
    return _run(cmd, root=(not key))[:6000]


@mcp.tool()
def sysctl_set(key: str, value: str, confirm: bool = False) -> str:
    """Set a sysctl parameter. confirm=True required. NOT persistent across reboots."""
    if not confirm: return f"Would set {key}={value}. Set confirm=true."
    return _run(["sysctl", "-w", f"{key}={value}"], root=True)


# ════════════════════════════════════════════════════════════════════════════
# SECURITY
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def selinux_status() -> str:
    """SELinux status, mode, and policy info."""
    if not _present("getenforce"): return "SELinux not available on this system"
    enforcing = _run(["getenforce"])
    sestatus  = _run(["sestatus"])
    return f"Mode: {enforcing}\n\n{sestatus}"


@mcp.tool()
def selinux_booleans(filter_str: str = "") -> str:
    """List SELinux booleans. filter_str narrows by name."""
    out = _run(["getsebool", "-a"])
    if filter_str:
        return "\n".join(l for l in out.splitlines() if filter_str.lower() in l.lower())
    return out[:6000]


@mcp.tool()
def apparmor_status() -> str:
    """AppArmor status and profile list."""
    if not _present("aa-status"): return "AppArmor not available"
    return _run(["aa-status"], root=True)


@mcp.tool()
def fail2ban_status(jail: str = "") -> str:
    """fail2ban status. jail: specific jail name or empty for overview."""
    if not _present("fail2ban-client"): return "fail2ban not installed"
    if jail:
        return _run(["fail2ban-client", "status", jail], root=True)
    return _run(["fail2ban-client", "status"], root=True)


@mcp.tool()
def suid_sgid_files(directory: str = "/") -> str:
    """Find SUID/SGID files in a directory."""
    return _run(["find", directory, "-perm", "/6000", "-type", "f",
                 "-ls"], timeout=30, root=True)[:4000]


@mcp.tool()
def audit_log_query(event_type: str = "", uid: int = -1,
                    result: str = "", limit: int = 50) -> str:
    """Query auditd log (ausearch). event_type: LOGIN|USER_AUTH|SYSCALL etc."""
    if not _present("ausearch"): return "auditd not installed"
    cmd = ["ausearch", "-i", "-n", str(limit)]
    if event_type: cmd += ["-m", event_type]
    if uid >= 0:   cmd += ["-ua", str(uid)]
    if result:     cmd += ["--success", result]
    return _run(cmd, root=True)[:6000]


# ════════════════════════════════════════════════════════════════════════════
# USERS & CRON
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def user_list() -> str:
    """List local user accounts from /etc/passwd."""
    lines = []
    for line in Path("/etc/passwd").read_text().splitlines():
        parts = line.split(":")
        if len(parts) >= 7 and int(parts[2]) >= 1000:   # real users
            lines.append(f"{parts[0]:20s} UID={parts[2]} HOME={parts[5]} SHELL={parts[6]}")
    return "\n".join(lines) or "No user accounts found"


@mcp.tool()
def user_info(username: str) -> str:
    """User account info: groups, last login, password aging."""
    groups   = _run(["groups", username])
    login    = _run(["lastlog", "-u", username])
    aging    = _run(["chage", "-l", username], root=True)
    return f"Groups: {groups}\n\nLast login:\n{login}\n\nPassword aging:\n{aging}"


@mcp.tool()
def user_login_history(username: str = "", limit: int = 20) -> str:
    """Recent login history (last command)."""
    cmd = ["last", "-n", str(limit)]
    if username: cmd.append(username)
    return _run(cmd)


@mcp.tool()
def cron_list(user: str = "") -> str:
    """List crontab entries for a user (or current user)."""
    cmd = ["crontab", "-l"]
    if user: cmd = ["crontab", "-l", "-u", user]
    user_cron = _run(cmd)
    system_cron = _run(["cat", "/etc/crontab"])
    cron_d = "\n".join(
        f"--- {f.name} ---\n{f.read_text()[:500]}"
        for f in sorted(Path("/etc/cron.d").iterdir())
        if f.is_file()
    ) if Path("/etc/cron.d").exists() else ""
    return f"## User crontab\n{user_cron}\n\n## /etc/crontab\n{system_cron}\n\n## /etc/cron.d\n{cron_d[:2000]}"


@mcp.tool()
def cron_add(entry: str, confirm: bool = False) -> str:
    """Add a cron entry. entry: '0 2 * * * /path/to/script'. confirm=True required."""
    if not confirm: return f"Would add cron: {entry!r}. Set confirm=true."
    current = subprocess.run(["crontab", "-l"], capture_output=True, text=True).stdout
    new_cron = current.rstrip("\n") + f"\n{entry}\n"
    return _run(["crontab", "-"], input_data=new_cron)


# ════════════════════════════════════════════════════════════════════════════
# PERFORMANCE
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def cpu_usage(duration: int = 3, interval: int = 1) -> str:
    """CPU usage snapshot. Uses mpstat if available, else /proc/stat diff."""
    if _present("mpstat"):
        return _run(["mpstat", str(interval), str(duration)], timeout=duration + 5)
    return _run(["vmstat", str(interval), str(duration)], timeout=duration + 5)


@mcp.tool()
def io_stats(duration: int = 3, interval: int = 1) -> str:
    """Disk I/O statistics (iostat)."""
    if _present("iostat"):
        return _run(["iostat", "-x", str(interval), str(duration)], timeout=duration + 5)
    return _run(["cat", "/proc/diskstats"])[:2000]


@mcp.tool()
def vmstat_run(count: int = 5, interval: int = 1) -> str:
    """Virtual memory statistics (vmstat)."""
    return _run(["vmstat", str(interval), str(count)], timeout=count * interval + 5)


@mcp.tool()
def perf_stat(command: str, duration: int = 5,
              confirm: bool = False) -> str:
    """Profile a command with perf stat. confirm=True required."""
    if not confirm: return f"Would perf stat: {command}. Set confirm=true."
    if not _present("perf"): return "perf not installed (install linux-perf or linux-tools)"
    cmd = ["perf", "stat", "-a", "--"] + command.split()
    return _run(cmd, timeout=duration + 10, root=True)


# ════════════════════════════════════════════════════════════════════════════
# BUILD / DEV TOOLS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def make_run(target: str = "", cwd: str = "",
             jobs: int = 0, env_str: str = "") -> str:
    """Run make. jobs: parallel jobs (0 = auto). env_str: 'KEY=val KEY2=val2'."""
    cmd = ["make"] + ([f"-j{jobs}"] if jobs else [f"-j{os.cpu_count()}"]) + ([target] if target else [])
    env_extra = dict(kv.split("=", 1) for kv in env_str.split() if "=" in kv)
    env = {**os.environ, **env_extra}
    try:
        r = subprocess.run(cmd, capture_output=True, text=True,
                           timeout=300, cwd=cwd or None, env=env)
        return (r.stdout + r.stderr).strip()[:6000]
    except subprocess.TimeoutExpired:
        return "[timeout]"
    except Exception as e:
        return f"[error: {e}]"


@mcp.tool()
def go_build(package: str = "./...", output: str = "",
             cwd: str = "", tags: str = "") -> str:
    """go build. tags: space-separated build tags."""
    cmd = ["go", "build"]
    if output: cmd += ["-o", output]
    if tags:   cmd += ["-tags", tags]
    cmd.append(package)
    return _run(cmd, timeout=180, cwd=cwd or None)


@mcp.tool()
def rust_build(cwd: str = "", release: bool = False,
               features: str = "") -> str:
    """Build a Rust project with cargo."""
    cmd = ["cargo", "build"] + (["--release"] if release else [])
    if features: cmd += ["--features", features]
    return _run(cmd, timeout=300, cwd=cwd or None)


@mcp.tool()
def gcc_compile(source: str, output: str = "a.out",
                flags: str = "-O2 -Wall") -> str:
    """Compile a C/C++ file with gcc/g++. flags: compiler flags."""
    compiler = "g++" if source.endswith((".cpp", ".cc", ".cxx")) else "gcc"
    cmd = [compiler] + flags.split() + [source, "-o", output]
    return _run(cmd, timeout=60)


@mcp.tool()
def cmake_configure(src_dir: str, build_dir: str,
                    options: str = "") -> str:
    """Configure a CMake project. options: '-DCMAKE_BUILD_TYPE=Release ...'"""
    Path(build_dir).mkdir(parents=True, exist_ok=True)
    cmd = ["cmake", src_dir] + (options.split() if options else [])
    return _run(cmd, timeout=60, cwd=build_dir)


# ════════════════════════════════════════════════════════════════════════════
# DEBUGGING
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def gdb_attach(pid: int, commands: str = "bt\nquit",
               timeout: int = 30, confirm: bool = False) -> str:
    """
    Attach gdb to a running process and run commands. confirm=True required.
    commands: newline-separated gdb commands (default: backtrace then quit).
    """
    if not confirm: return f"Would gdb attach to PID {pid}. Set confirm=true."
    if not _present("gdb"): return "gdb not installed"
    cmd_seq = "\n".join(commands.splitlines()) + "\n"
    return _run(["gdb", "-q", "-n", "-p", str(pid),
                 "-ex", "set pagination off",
                 "-ex", commands.replace("\n", " -ex "),
                 "-ex", "quit"], timeout=timeout, root=True)


@mcp.tool()
def valgrind_run(command: str, tool: str = "memcheck",
                 timeout: int = 60, confirm: bool = False) -> str:
    """
    Run a command under valgrind. tool: memcheck|callgrind|helgrind|massif.
    confirm=True required.
    """
    if not confirm: return f"Would valgrind ({tool}): {command}. Set confirm=true."
    if not _present("valgrind"): return "valgrind not installed"
    return _run(["valgrind", f"--tool={tool}", "--error-exitcode=1"]
                + command.split(), timeout=timeout)


@mcp.tool()
def ldd_check(binary_path: str) -> str:
    """Show shared library dependencies for a binary."""
    p = Path(binary_path).expanduser()
    if not p.exists(): return f"Not found: {binary_path}"
    return _run(["ldd", str(p)])


# ════════════════════════════════════════════════════════════════════════════
# QUICK STATUS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def linux_quick_status() -> str:
    """Single-call Linux system snapshot: kernel, distro, load, memory, disk, network."""
    return json.dumps({
        "hostname":    _run(["hostname", "-f"]),
        "kernel":      _run(["uname", "-r"]),
        "distro":      _DISTRO,
        "uptime":      _run(["uptime", "-p"]),
        "load":        _run(["cat", "/proc/loadavg"]),
        "cpu_count":   str(os.cpu_count()),
        "mem_total":   _run(["awk", "/MemTotal/{print $2}", "/proc/meminfo"]),
        "mem_free":    _run(["awk", "/MemAvailable/{print $2}", "/proc/meminfo"]),
        "disk_root":   _run(["df", "-h", "/"]),
        "ip_addrs":    _run(["ip", "-br", "addr"]),
        "systemd":     _present("systemctl"),
        "docker":      _present("docker"),
        "python":      _run(["python3", "--version"]),
        "go":          _run(["go", "version"]),
    }, indent=2)


if __name__ == "__main__":
    mcp.run()
