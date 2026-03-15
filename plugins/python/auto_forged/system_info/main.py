import sys
import json
import os
import subprocess
from datetime import datetime

def run_command(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        return result.stdout.strip(), result.stderr.strip(), result.returncode
    except Exception as e:
        return "", str(e), 1

def get_disk_usage():
    output, _, code = run_command("df -h /data 2>/dev/null || df -h /")
    return output if code == 0 else "N/A"

def get_memory_info():
    output, _, code = run_command("free -m 2>/dev/null || cat /proc/meminfo | head -5")
    return output if code == 0 else "N/A"

def get_load_avg():
    output, _, code = run_command("uptime")
    return output if code == 0 else "N/A"

def get_env_vars(vars_list):
    results = {}
    for var in vars_list:
        results[var] = os.environ.get(var, "N/A")
    return results

def main():
    try:
        data = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
        return

    info_type = data.get("info_type", "all")
    
    result = {"timestamp": datetime.now().isoformat()}
    
    if info_type in ("all", "disk"):
        result["disk_usage"] = get_disk_usage()
    
    if info_type in ("all", "memory"):
        result["memory"] = get_memory_info()
    
    if info_type in ("all", "load"):
        result["load_avg"] = get_load_avg()
    
    if info_type in ("all", "env"):
        default_vars = ["PATH", "HOME", "USER", "ANDROID_ROOT", "ANDROID_DATA"]
        result["env_vars"] = get_env_vars(data.get("env_vars", default_vars))
    
    output_json = json.dumps(result, indent=2)
    print(json.dumps({"success": True, "output": output_json, "error": ""}))

if __name__ == "__main__":
    main()
