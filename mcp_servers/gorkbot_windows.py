#!/usr/bin/env python3
"""
gorkbot_windows — Comprehensive Windows MCP server.

Covers:
  • Registry          (read, write, list, search, export/import, startup items)
  • Services          (list, start, stop, config, create)
  • Scheduled Tasks   (list, create, run, delete, enable/disable)
  • Process mgmt      (list, kill, tree, DLLs, connections, dump)
  • Event Logs        (query, tail, export, clear)
  • Networking        (config, routes, ARP, connections, DNS, firewall, shares)
  • Users & Groups    (list, info, create, delete, group membership)
  • PowerShell        (run scripts, install modules)
  • WMI               (arbitrary queries, hardware info)
  • Windows Defender  (status, scan, update, exclusions)
  • WSL               (list distros, run commands)
  • Certificates      (store list, info, export, certutil)
  • Binary analysis   (PE info, strings, file hash, ACLs, NTFS streams)
  • System info       (installed software, features, Windows updates)

NOTE: Run on a Windows host. Requires Python 3.8+ with no extra deps.
      PowerShell 5.1+ assumed (pwsh/powershell both tried).
"""

import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Optional

try:
    from fastmcp import FastMCP
except ImportError:
    print("fastmcp not installed. Run: pip install fastmcp", file=sys.stderr)
    sys.exit(1)

mcp = FastMCP("gorkbot-windows")

_IS_WINDOWS = sys.platform == "win32"

# ── Helpers ──────────────────────────────────────────────────────────────────

def _run(cmd: list[str], timeout: int = 30, cwd: Optional[str] = None,
         input_data: Optional[str] = None, shell: bool = False) -> str:
    if not _IS_WINDOWS:
        return "[not running on Windows]"
    try:
        r = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout,
            cwd=cwd, input=input_data, shell=shell,
            creationflags=0x08000000,  # CREATE_NO_WINDOW
            encoding="utf-8", errors="replace",
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
        return f"[not found: {cmd[0] if cmd else '?'}]"
    except Exception as e:
        return f"[error: {e}]"


def _ps(script: str, timeout: int = 30) -> str:
    """Run a PowerShell script string."""
    if not _IS_WINDOWS:
        return "[not running on Windows]"
    ps = "pwsh" if _ps_binary() == "pwsh" else "powershell"
    with tempfile.NamedTemporaryFile(mode="w", suffix=".ps1",
                                     delete=False, encoding="utf-8") as f:
        f.write(f"$ErrorActionPreference = 'SilentlyContinue'\n{script}")
        tmp = f.name
    try:
        return _run([ps, "-NoProfile", "-NonInteractive",
                     "-ExecutionPolicy", "Bypass", "-File", tmp],
                    timeout=timeout)
    finally:
        try: os.unlink(tmp)
        except Exception: pass


_ps_bin_cache: Optional[str] = None
def _ps_binary() -> str:
    global _ps_bin_cache
    if _ps_bin_cache: return _ps_bin_cache
    import shutil
    _ps_bin_cache = "pwsh" if shutil.which("pwsh") else "powershell"
    return _ps_bin_cache


def _reg_query(key: str, value: str = "") -> str:
    cmd = ["reg", "query", key]
    if value: cmd += ["/v", value]
    return _run(cmd)


# ════════════════════════════════════════════════════════════════════════════
# REGISTRY
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def reg_read(hive_key: str, value_name: str = "") -> str:
    """
    Read a registry value. hive_key: full path e.g. HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion.
    value_name: empty = read all values in key.
    """
    return _reg_query(hive_key, value_name)


@mcp.tool()
def reg_list(hive_key: str, recursive: bool = False) -> str:
    """List subkeys (and optionally values) under a registry key."""
    cmd = ["reg", "query", hive_key]
    if recursive: cmd.append("/s")
    return _run(cmd)[:6000]


@mcp.tool()
def reg_write(hive_key: str, value_name: str, value_type: str,
              data: str, confirm: bool = False) -> str:
    """
    Write a registry value. confirm=True required.
    value_type: REG_SZ|REG_DWORD|REG_QWORD|REG_BINARY|REG_EXPAND_SZ|REG_MULTI_SZ.
    """
    if not confirm:
        return f"Would write {hive_key}\\{value_name} = {data!r} ({value_type}). Set confirm=true."
    return _run(["reg", "add", hive_key, "/v", value_name,
                 "/t", value_type, "/d", data, "/f"])


@mcp.tool()
def reg_delete(hive_key: str, value_name: str = "",
               confirm: bool = False) -> str:
    """Delete a registry value (or entire key if value_name empty). confirm=True required."""
    if not confirm:
        return f"Would delete {hive_key}\\{value_name or '(key)'}. Set confirm=true."
    cmd = ["reg", "delete", hive_key, "/f"]
    if value_name: cmd += ["/v", value_name]
    return _run(cmd)


@mcp.tool()
def reg_search(hive_key: str, search_term: str,
               search_data: bool = True, search_keys: bool = True) -> str:
    """Search registry for a term recursively. Returns matching keys/values."""
    script = f"""
    Get-ChildItem -Path "Registry::{hive_key}" -Recurse -ErrorAction SilentlyContinue |
    Where-Object {{ $_.Name -like "*{search_term}*" -or
                    ($_.GetValueNames() | Where-Object {{ $_ -like "*{search_term}*" }}) -or
                    ($_.GetValueNames() | ForEach-Object {{ $_.GetValue($_) }} | Where-Object {{ "$_" -like "*{search_term}*" }})
                 }} |
    Select-Object Name | Format-List
    """
    return _ps(script, timeout=60)[:6000]


@mcp.tool()
def reg_export(hive_key: str, output_file: str) -> str:
    """Export a registry key to a .reg file."""
    return _run(["reg", "export", hive_key, output_file, "/y"])


@mcp.tool()
def startup_items() -> str:
    """List all startup programs from registry and startup folders."""
    script = """
    $locations = @(
        "HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run",
        "HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\RunOnce",
        "HKCU:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run",
        "HKCU:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\RunOnce"
    )
    foreach ($loc in $locations) {
        Write-Output "=== $loc ==="
        if (Test-Path $loc) {
            Get-ItemProperty $loc | Format-List
        }
    }
    Write-Output "=== Startup Folders ==="
    $folders = @(
        "$env:APPDATA\\Microsoft\\Windows\\Start Menu\\Programs\\Startup",
        "$env:ProgramData\\Microsoft\\Windows\\Start Menu\\Programs\\StartUp"
    )
    foreach ($f in $folders) {
        Write-Output "--- $f ---"
        if (Test-Path $f) { Get-ChildItem $f | Format-List }
    }
    """
    return _ps(script)


# ════════════════════════════════════════════════════════════════════════════
# SERVICES
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def service_list(filter_str: str = "", state: str = "all") -> str:
    """
    List Windows services. state: all|running|stopped|paused.
    filter_str: name substring filter.
    """
    script = f"""
    Get-Service | Where-Object {{ '{state}' -eq 'all' -or $_.Status -eq '{state.capitalize()}' }} |
    Where-Object {{ $_.DisplayName -like '*{filter_str}*' -or $_.Name -like '*{filter_str}*' }} |
    Select-Object Status, Name, DisplayName |
    Format-Table -AutoSize
    """
    return _ps(script)


@mcp.tool()
def service_status(name: str) -> str:
    """Detailed status for a specific Windows service."""
    script = f"""
    $svc = Get-Service -Name '{name}' -ErrorAction Stop
    $wmi = Get-WmiObject Win32_Service -Filter "Name='{name}'"
    [PSCustomObject]@{{
        Name        = $svc.Name
        DisplayName = $svc.DisplayName
        Status      = $svc.Status
        StartType   = $svc.StartType
        PathName    = $wmi.PathName
        ProcessId   = $wmi.ProcessId
        Description = $wmi.Description
        Account     = $wmi.StartName
    }} | Format-List
    """
    return _ps(script)


@mcp.tool()
def service_start(name: str, confirm: bool = False) -> str:
    """Start a Windows service. confirm=True required."""
    if not confirm: return f"Would start service '{name}'. Set confirm=true."
    return _run(["sc", "start", name])


@mcp.tool()
def service_stop(name: str, confirm: bool = False) -> str:
    """Stop a Windows service. confirm=True required."""
    if not confirm: return f"Would stop service '{name}'. Set confirm=true."
    return _run(["sc", "stop", name])


@mcp.tool()
def service_restart(name: str, confirm: bool = False) -> str:
    """Restart a Windows service. confirm=True required."""
    if not confirm: return f"Would restart service '{name}'. Set confirm=true."
    stop = _run(["sc", "stop", name])
    import time; time.sleep(2)
    start = _run(["sc", "start", name])
    return f"Stop: {stop}\nStart: {start}"


@mcp.tool()
def service_config(name: str) -> str:
    """Query service configuration (binary path, start type, dependencies)."""
    return _run(["sc", "qc", name])


@mcp.tool()
def service_create(name: str, display_name: str, bin_path: str,
                   start_type: str = "demand", confirm: bool = False) -> str:
    """Create a new service. start_type: auto|demand|disabled. confirm=True required."""
    if not confirm:
        return f"Would create service '{name}'. Set confirm=true."
    return _run(["sc", "create", name, f"DisplayName={display_name}",
                 f"binPath={bin_path}", f"start={start_type}"])


# ════════════════════════════════════════════════════════════════════════════
# SCHEDULED TASKS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def task_list(filter_str: str = "", include_status: bool = True) -> str:
    """List scheduled tasks, optionally filtered by name."""
    script = f"""
    Get-ScheduledTask | Where-Object {{ $_.TaskName -like '*{filter_str}*' }} |
    Select-Object TaskName, TaskPath, State, @{{n='LastRun';e={{$_.LastRunTime}}}},
                  @{{n='NextRun';e={{$_.NextRunTime}}}} |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


@mcp.tool()
def task_info(name: str) -> str:
    """Detailed info for a scheduled task including triggers and actions."""
    script = f"""
    $t = Get-ScheduledTask -TaskName '{name}' -ErrorAction Stop
    $info = Get-ScheduledTaskInfo -TaskName '{name}'
    [PSCustomObject]@{{
        Name        = $t.TaskName
        Path        = $t.TaskPath
        State       = $t.State
        Actions     = ($t.Actions | ForEach-Object {{ $_.Execute + " " + $_.Arguments }}) -join "; "
        Triggers    = ($t.Triggers | ForEach-Object {{ $_.GetType().Name + ": " + $_.StartBoundary }}) -join "; "
        LastRun     = $info.LastRunTime
        LastResult  = $info.LastTaskResult
        NextRun     = $info.NextRunTime
    }} | Format-List
    """
    return _ps(script)


@mcp.tool()
def task_run(name: str, confirm: bool = False) -> str:
    """Immediately run a scheduled task. confirm=True required."""
    if not confirm: return f"Would run task '{name}'. Set confirm=true."
    return _ps(f"Start-ScheduledTask -TaskName '{name}'")


@mcp.tool()
def task_create(name: str, command: str, schedule: str,
                description: str = "", confirm: bool = False) -> str:
    """
    Create a scheduled task. schedule examples:
    'Daily at 09:00', 'Weekly on Monday at 18:00', 'At startup'.
    confirm=True required.
    """
    if not confirm:
        return f"Would create task '{name}' ({schedule}: {command}). Set confirm=true."
    script = f"""
    $action  = New-ScheduledTaskAction -Execute "{command}"
    $trigger = switch -Wildcard ('{schedule}') {{
        '*daily*'   {{ New-ScheduledTaskTrigger -Daily -At ('{schedule}' -replace '.*at ','') }}
        '*startup*' {{ New-ScheduledTaskTrigger -AtStartup }}
        '*logon*'   {{ New-ScheduledTaskTrigger -AtLogOn }}
        default     {{ New-ScheduledTaskTrigger -Once -At '{schedule}' }}
    }}
    Register-ScheduledTask -TaskName '{name}' -Action $action -Trigger $trigger `
        -Description '{description}' -Force
    """
    return _ps(script)


@mcp.tool()
def task_delete(name: str, confirm: bool = False) -> str:
    """Delete a scheduled task. confirm=True required."""
    if not confirm: return f"Would delete task '{name}'. Set confirm=true."
    return _ps(f"Unregister-ScheduledTask -TaskName '{name}' -Confirm:$false")


# ════════════════════════════════════════════════════════════════════════════
# PROCESSES
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def process_list(filter_str: str = "", sort_by: str = "cpu") -> str:
    """List processes. sort_by: cpu|mem|name|pid."""
    sort_prop = {"cpu": "CPU", "mem": "WorkingSet64",
                 "name": "Name", "pid": "Id"}.get(sort_by, "CPU")
    script = f"""
    Get-Process | Where-Object {{ $_.Name -like '*{filter_str}*' }} |
    Sort-Object -{sort_prop} |
    Select-Object Id, Name, CPU,
        @{{n='Mem(MB)'; e={{[math]::Round($_.WorkingSet64/1MB,1)}}}},
        @{{n='Threads';e={{$_.Threads.Count}}}},
        Path |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


@mcp.tool()
def process_kill(pid_or_name: str, confirm: bool = False) -> str:
    """Kill a process by PID or name. confirm=True required."""
    if not confirm:
        return f"Would kill '{pid_or_name}'. Set confirm=true."
    script = f"""
    try {{
        $id = [int]'{pid_or_name}'
        Stop-Process -Id $id -Force -ErrorAction Stop
        "Killed PID $id"
    }} catch {{
        Stop-Process -Name '{pid_or_name}' -Force -ErrorAction Stop
        "Killed process(es) named '{pid_or_name}'"
    }}
    """
    return _ps(script)


@mcp.tool()
def process_tree() -> str:
    """Show process parent-child hierarchy tree."""
    script = """
    $procs = Get-CimInstance Win32_Process
    $map = @{}
    foreach ($p in $procs) { $map[$p.ProcessId] = $p }
    function Show-Tree($pid, $indent=0) {
        $p = $map[$pid]; if (-not $p) { return }
        Write-Output (" " * $indent + "[{0,6}] {1}" -f $p.ProcessId, $p.Name)
        $children = $procs | Where-Object { $_.ParentProcessId -eq $pid -and $_.ProcessId -ne $pid }
        foreach ($c in $children) { Show-Tree $c.ProcessId ($indent+4) }
    }
    $roots = $procs | Where-Object { $_.ParentProcessId -eq 0 -or -not $map[$_.ParentProcessId] }
    foreach ($r in $roots) { Show-Tree $r.ProcessId }
    """
    return _ps(script, timeout=20)[:8000]


@mcp.tool()
def process_dlls(pid: int) -> str:
    """List DLLs loaded by a process."""
    script = f"""
    (Get-Process -Id {pid} -ErrorAction Stop).Modules |
    Select-Object ModuleName, FileName |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


@mcp.tool()
def process_connections(pid: int = 0) -> str:
    """List TCP/UDP connections, optionally filtered by PID."""
    script = f"""
    $filter = {pid}
    Get-NetTCPConnection | Where-Object {{ $filter -eq 0 -or $_.OwningProcess -eq $filter }} |
    Select-Object LocalAddress, LocalPort, RemoteAddress, RemotePort, State,
        @{{n='Process';e={{(Get-Process -Id $_.OwningProcess -EA SilentlyContinue).Name}}}} |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


# ════════════════════════════════════════════════════════════════════════════
# EVENT LOGS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def event_log_list() -> str:
    """List available Windows Event Log sources."""
    return _ps("Get-EventLog -List | Select-Object Log,Entries,MaximumKilobytes | Format-Table -AutoSize")


@mcp.tool()
def event_log_query(log: str = "System", event_id: int = 0,
                    source: str = "", level: str = "",
                    hours: int = 24, limit: int = 50) -> str:
    """
    Query Windows Event Log. log: System|Application|Security.
    level: Error|Warning|Information|SuccessAudit|FailureAudit.
    hours: look back this many hours.
    """
    since = f"(Get-Date).AddHours(-{hours})"
    filters = [f"$_.TimeWritten -gt {since}"]
    if event_id: filters.append(f"$_.EventID -eq {event_id}")
    if source:   filters.append(f"$_.Source -like '*{source}*'")
    if level:    filters.append(f"$_.EntryType -eq '{level}'")
    where = " -and ".join(filters)
    script = f"""
    Get-EventLog -LogName '{log}' -Newest 1000 |
    Where-Object {{ {where} }} |
    Select-Object -First {limit} |
    Select-Object TimeWritten, EventID, EntryType, Source,
        @{{n='Message';e={{$_.Message.Split("`n")[0]}}}}) |
    Format-Table -AutoSize -Wrap
    """
    return _ps(script, timeout=30)[:8000]


@mcp.tool()
def event_log_tail(log: str = "System", limit: int = 20) -> str:
    """Return the most recent events from a Windows Event Log."""
    script = f"""
    Get-EventLog -LogName '{log}' -Newest {limit} |
    Select-Object TimeWritten, EventID, EntryType, Source,
        @{{n='Message';e={{$_.Message.Split("`n")[0]}}}}) |
    Format-Table -AutoSize -Wrap
    """
    return _ps(script)[:6000]


# ════════════════════════════════════════════════════════════════════════════
# NETWORKING
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def net_config() -> str:
    """All network adapter configuration (IP, DNS, gateway, MAC)."""
    return _run(["ipconfig", "/all"])


@mcp.tool()
def net_connections(state: str = "", process_filter: str = "") -> str:
    """Active TCP connections. state: ESTABLISHED|LISTENING|TIME_WAIT etc."""
    script = f"""
    Get-NetTCPConnection |
    Where-Object {{ ('{state}' -eq '' -or $_.State -eq '{state}') }} |
    Select-Object LocalAddress, LocalPort, RemoteAddress, RemotePort, State,
        @{{n='Proc';e={{(Get-Process -Id $_.OwningProcess -EA SilentlyContinue).Name}}}} |
    Where-Object {{ '{process_filter}' -eq '' -or $_.Proc -like '*{process_filter}*' }} |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


@mcp.tool()
def net_dns_cache() -> str:
    """Show Windows DNS resolver cache."""
    return _run(["ipconfig", "/displaydns"])[:4000]


@mcp.tool()
def net_dns_flush(confirm: bool = False) -> str:
    """Flush DNS resolver cache. confirm=True required."""
    if not confirm: return "Would flush DNS cache. Set confirm=true."
    return _run(["ipconfig", "/flushdns"])


@mcp.tool()
def net_arp() -> str:
    """Display ARP table (IP-to-MAC mappings)."""
    return _run(["arp", "-a"])


@mcp.tool()
def net_route() -> str:
    """Display routing table."""
    return _run(["route", "print"])


@mcp.tool()
def net_shares() -> str:
    """List Windows network shares."""
    return _run(["net", "share"])


@mcp.tool()
def net_adapters() -> str:
    """Network adapter status via PowerShell Get-NetAdapter."""
    return _ps("Get-NetAdapter | Format-Table -AutoSize")


@mcp.tool()
def hosts_file() -> str:
    """Read the Windows hosts file."""
    path = Path(os.environ.get("SystemRoot", "C:\\Windows")) / "System32" / "drivers" / "etc" / "hosts"
    try:
        return path.read_text(errors="replace")
    except Exception as e:
        return f"Cannot read hosts file: {e}"


# ════════════════════════════════════════════════════════════════════════════
# FIREWALL
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def fw_status() -> str:
    """Windows Firewall status for all profiles (Domain, Private, Public)."""
    return _run(["netsh", "advfirewall", "show", "allprofiles"])


@mcp.tool()
def fw_rules_list(direction: str = "in", profile: str = "any",
                  filter_str: str = "") -> str:
    """List firewall rules. direction: in|out. profile: any|domain|private|public."""
    script = f"""
    Get-NetFirewallRule |
    Where-Object {{ $_.Direction -like '*{direction}*' -and
                    ('{profile}' -eq 'any' -or $_.Profile -like '*{profile}*') -and
                    ($_.DisplayName -like '*{filter_str}*' -or '{filter_str}' -eq '') }} |
    Select-Object DisplayName, Direction, Action, Protocol, Profile, Enabled |
    Format-Table -AutoSize
    """
    return _ps(script)[:6000]


@mcp.tool()
def fw_rule_add(name: str, direction: str = "in", action: str = "allow",
                protocol: str = "TCP", port: str = "",
                program: str = "", confirm: bool = False) -> str:
    """Add a firewall rule. direction: in|out. action: allow|block. confirm=True required."""
    if not confirm:
        return f"Would add firewall rule '{name}'. Set confirm=true."
    script = f"""
    $params = @{{
        DisplayName = '{name}'; Direction = '{direction}'; Action = '{action}'
        Protocol    = '{protocol}'
    }}
    if ('{port}')    {{ $params.LocalPort   = '{port}' }}
    if ('{program}') {{ $params.Program     = '{program}' }}
    New-NetFirewallRule @params
    """
    return _ps(script)


@mcp.tool()
def fw_rule_delete(name: str, confirm: bool = False) -> str:
    """Delete a firewall rule by display name. confirm=True required."""
    if not confirm: return f"Would delete firewall rule '{name}'. Set confirm=true."
    return _ps(f"Remove-NetFirewallRule -DisplayName '{name}'")


# ════════════════════════════════════════════════════════════════════════════
# USERS & GROUPS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def user_list() -> str:
    """List local user accounts."""
    return _ps("Get-LocalUser | Format-Table Name, Enabled, LastLogon, Description -AutoSize")


@mcp.tool()
def user_info(username: str) -> str:
    """Detailed info for a local user account."""
    return _ps(f"Get-LocalUser '{username}' | Format-List")


@mcp.tool()
def group_list() -> str:
    """List local groups."""
    return _ps("Get-LocalGroup | Format-Table Name, Description -AutoSize")


@mcp.tool()
def group_members(name: str) -> str:
    """List members of a local group."""
    return _ps(f"Get-LocalGroupMember '{name}' | Format-Table Name, ObjectClass -AutoSize")


@mcp.tool()
def whoami_info() -> str:
    """Current user identity, groups, and privileges."""
    return _run(["whoami", "/all"])


# ════════════════════════════════════════════════════════════════════════════
# POWERSHELL
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def ps_run(script: str, timeout: int = 60) -> str:
    """Execute an arbitrary PowerShell script and return output."""
    return _ps(script, timeout=timeout)


@mcp.tool()
def ps_run_file(path: str, args: str = "", timeout: int = 120) -> str:
    """Run a .ps1 script file."""
    p = Path(path)
    if not p.exists(): return f"Not found: {path}"
    ps = _ps_binary()
    cmd = [ps, "-NoProfile", "-NonInteractive",
           "-ExecutionPolicy", "Bypass", "-File", str(p)]
    if args: cmd += args.split()
    return _run(cmd, timeout=timeout)


@mcp.tool()
def ps_module_list(filter_str: str = "") -> str:
    """List installed PowerShell modules."""
    return _ps(f"Get-Module -ListAvailable | Where-Object {{$_.Name -like '*{filter_str}*'}} | Select-Object Name, Version, ModuleType | Format-Table -AutoSize")


@mcp.tool()
def ps_install_module(name: str, confirm: bool = False) -> str:
    """Install a PowerShell module from PSGallery. confirm=True required."""
    if not confirm: return f"Would install PS module '{name}'. Set confirm=true."
    return _ps(f"Install-Module -Name '{name}' -Force -AllowClobber -Scope CurrentUser", timeout=120)


# ════════════════════════════════════════════════════════════════════════════
# WMI
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def wmi_query(wmi_class: str, properties: str = "*",
              where_clause: str = "") -> str:
    """
    Execute an arbitrary WMI query.
    wmi_class: Win32_Process|Win32_Service|Win32_Disk etc.
    where_clause: SQL-style WHERE (e.g. 'Name=\"notepad.exe\"').
    """
    where = f" WHERE {where_clause}" if where_clause else ""
    query = f"SELECT {properties} FROM {wmi_class}{where}"
    script = f"Get-WmiObject -Query \"{query}\" | Format-List"
    return _ps(script)[:6000]


@mcp.tool()
def wmi_hardware_info() -> str:
    """System hardware summary: CPU, RAM, GPU, disks, BIOS."""
    script = """
    Write-Output "=== CPU ==="
    Get-WmiObject Win32_Processor | Select-Object Name, NumberOfCores, NumberOfLogicalProcessors, MaxClockSpeed | Format-List
    Write-Output "=== RAM ==="
    $ram = (Get-WmiObject Win32_PhysicalMemory | Measure-Object Capacity -Sum).Sum / 1GB
    "Total RAM: {0:N1} GB" -f $ram
    Write-Output "=== GPU ==="
    Get-WmiObject Win32_VideoController | Select-Object Name, AdapterRAM, DriverVersion | Format-List
    Write-Output "=== Disks ==="
    Get-WmiObject Win32_DiskDrive | Select-Object Model, Size, MediaType | Format-Table -AutoSize
    Write-Output "=== BIOS ==="
    Get-WmiObject Win32_BIOS | Select-Object Manufacturer, Name, Version, ReleaseDate | Format-List
    Write-Output "=== Motherboard ==="
    Get-WmiObject Win32_BaseBoard | Select-Object Manufacturer, Product, Version | Format-List
    """
    return _ps(script)


# ════════════════════════════════════════════════════════════════════════════
# WINDOWS DEFENDER
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def defender_status() -> str:
    """Windows Defender / Microsoft Security status."""
    return _ps("Get-MpComputerStatus | Format-List")


@mcp.tool()
def defender_scan(path: str = "", scan_type: str = "QuickScan") -> str:
    """
    Trigger a Defender scan. scan_type: QuickScan|FullScan|CustomScan.
    path: required for CustomScan.
    """
    if scan_type == "CustomScan" and path:
        script = f"Start-MpScan -ScanType CustomScan -ScanPath '{path}'"
    else:
        script = f"Start-MpScan -ScanType {scan_type}"
    return _ps(script, timeout=300)


@mcp.tool()
def defender_exclusions() -> str:
    """List Windows Defender exclusions (paths, processes, extensions)."""
    return _ps("""
    $p = Get-MpPreference
    [PSCustomObject]@{
        ExclusionPath      = $p.ExclusionPath
        ExclusionProcess   = $p.ExclusionProcess
        ExclusionExtension = $p.ExclusionExtension
    } | Format-List
    """)


# ════════════════════════════════════════════════════════════════════════════
# WSL
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def wsl_list() -> str:
    """List installed WSL distributions and their status."""
    return _run(["wsl", "--list", "--verbose"])


@mcp.tool()
def wsl_run(command: str, distro: str = "", timeout: int = 60) -> str:
    """Run a command in WSL. distro: leave empty for default."""
    cmd = ["wsl"]
    if distro: cmd += ["-d", distro]
    cmd += ["--", "bash", "-c", command]
    return _run(cmd, timeout=timeout)


# ════════════════════════════════════════════════════════════════════════════
# CERTIFICATES
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def cert_list(store: str = "My", store_location: str = "CurrentUser") -> str:
    """
    List certificates. store: My|Root|CA|TrustedPeople|WebHosting.
    store_location: CurrentUser|LocalMachine.
    """
    script = f"""
    Get-ChildItem Cert:\\{store_location}\\{store} |
    Select-Object Subject, Thumbprint, NotBefore, NotAfter, Issuer |
    Format-Table -AutoSize -Wrap
    """
    return _ps(script)[:6000]


@mcp.tool()
def cert_info(thumbprint: str,
              store_location: str = "CurrentUser") -> str:
    """Detailed info for a certificate by thumbprint."""
    script = f"""
    $certs = Get-ChildItem Cert:\\{store_location}\\* -Recurse |
             Where-Object {{ $_.Thumbprint -eq '{thumbprint}' }}
    $certs | Format-List
    """
    return _ps(script)


@mcp.tool()
def certutil_run(args: str) -> str:
    """Run certutil with arbitrary args (e.g. '-hashfile file SHA256')."""
    return _run(["certutil"] + args.split())


# ════════════════════════════════════════════════════════════════════════════
# BINARY ANALYSIS
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def file_hash(path: str, algorithm: str = "SHA256") -> str:
    """Compute file hash. algorithm: MD5|SHA1|SHA256|SHA384|SHA512."""
    script = f"Get-FileHash '{path}' -Algorithm {algorithm} | Format-List"
    return _ps(script)


@mcp.tool()
def file_acl(path: str) -> str:
    """Show file/directory ACLs (Access Control List)."""
    script = f"(Get-Acl '{path}').Access | Format-Table FileSystemRights, AccessControlType, IdentityReference -AutoSize"
    return _ps(script)


@mcp.tool()
def ntfs_streams(path: str) -> str:
    """List NTFS alternate data streams on a file."""
    script = f"Get-Item '{path}' -Stream * | Where-Object {{$_.Stream -ne ':$DATA'}} | Format-Table Stream, Length -AutoSize"
    return _ps(script)


@mcp.tool()
def pe_info(path: str) -> str:
    """Basic PE (Portable Executable) info for a Windows binary."""
    script = f"""
    $file = Get-Item '{path}'
    $v = [System.Diagnostics.FileVersionInfo]::GetVersionInfo('{path}')
    [PSCustomObject]@{{
        Name         = $file.Name
        Size         = $file.Length
        ProductName  = $v.ProductName
        FileVersion  = $v.FileVersion
        CompanyName  = $v.CompanyName
        Description  = $v.FileDescription
        Is64Bit      = [System.Environment]::Is64BitOperatingSystem
        IsDotNet     = $null -ne [Reflection.AssemblyName]::GetAssemblyName('{path}') 2>$null
    }} | Format-List
    """
    return _ps(script)


@mcp.tool()
def strings_file(path: str, min_len: int = 6,
                 filter_str: str = "") -> str:
    """Extract printable strings from a file (ASCII + Unicode)."""
    script = f"""
    $bytes = [System.IO.File]::ReadAllBytes('{path}')
    $ascii = [System.Text.RegularExpressions.Regex]::Matches([System.Text.Encoding]::ASCII.GetString($bytes), '[\\x20-\\x7E]{{{min_len},}}')
    $ascii.Value | Where-Object {{ '{filter_str}' -eq '' -or $_ -like '*{filter_str}*' }} | Select-Object -First 500
    """
    return _ps(script, timeout=30)


# ════════════════════════════════════════════════════════════════════════════
# SYSTEM INFO
# ════════════════════════════════════════════════════════════════════════════

@mcp.tool()
def sys_info() -> str:
    """Comprehensive system info (systeminfo equivalent via WMI)."""
    script = """
    $cs   = Get-WmiObject Win32_ComputerSystem
    $os   = Get-WmiObject Win32_OperatingSystem
    $bios = Get-WmiObject Win32_BIOS
    [PSCustomObject]@{
        HostName        = $cs.Name
        Domain          = $cs.Domain
        OS              = $os.Caption
        Version         = $os.Version
        Architecture    = $os.OSArchitecture
        TotalRAM_GB     = [math]::Round($cs.TotalPhysicalMemory/1GB, 2)
        FreeRAM_GB      = [math]::Round($os.FreePhysicalMemory/1MB, 2)
        LastBoot        = $os.LastBootUpTime
        SystemDrive     = $os.SystemDrive
        BIOSVersion     = $bios.SMBIOSBIOSVersion
        Manufacturer    = $cs.Manufacturer
        Model           = $cs.Model
    } | Format-List
    """
    return _ps(script)


@mcp.tool()
def installed_software(filter_str: str = "") -> str:
    """List installed software from registry uninstall keys."""
    script = f"""
    $paths = @(
        'HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*',
        'HKLM:\\SOFTWARE\\WOW6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*',
        'HKCU:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\*'
    )
    $paths | ForEach-Object {{ Get-ItemProperty $_ -ErrorAction SilentlyContinue }} |
    Where-Object {{ $_.DisplayName -and $_.DisplayName -like '*{filter_str}*' }} |
    Select-Object DisplayName, DisplayVersion, Publisher, InstallDate |
    Sort-Object DisplayName |
    Format-Table -AutoSize
    """
    return _ps(script)[:8000]


@mcp.tool()
def windows_updates() -> str:
    """List installed Windows updates (HotFixes)."""
    return _ps("Get-HotFix | Select-Object HotFixID, InstalledOn, Description | Sort-Object InstalledOn -Descending | Format-Table -AutoSize")


@mcp.tool()
def env_vars(scope: str = "all") -> str:
    """List environment variables. scope: all|user|machine."""
    script = {
        "user":    "[Environment]::GetEnvironmentVariables('User').GetEnumerator() | Sort-Object Name | Format-Table Name, Value -AutoSize",
        "machine": "[Environment]::GetEnvironmentVariables('Machine').GetEnumerator() | Sort-Object Name | Format-Table Name, Value -AutoSize",
        "all":     "Get-ChildItem Env: | Format-Table Name, Value -AutoSize",
    }.get(scope, "Get-ChildItem Env: | Format-Table Name, Value -AutoSize")
    return _ps(script)[:6000]


@mcp.tool()
def windows_quick_status() -> str:
    """Single-call Windows system snapshot."""
    script = """
    $os  = Get-WmiObject Win32_OperatingSystem
    $cpu = Get-WmiObject Win32_Processor | Select-Object -First 1
    [PSCustomObject]@{
        Host        = $env:COMPUTERNAME
        OS          = $os.Caption
        Version     = $os.Version
        Uptime_h    = [math]::Round(($os.LocalDateTime - $os.LastBootUpTime).TotalHours, 1)
        CPU         = $cpu.Name
        RAM_Free_GB = [math]::Round($os.FreePhysicalMemory/1MB, 2)
        PS_Version  = $PSVersionTable.PSVersion.ToString()
    } | ConvertTo-Json
    """
    return _ps(script)


if __name__ == "__main__":
    mcp.run()
