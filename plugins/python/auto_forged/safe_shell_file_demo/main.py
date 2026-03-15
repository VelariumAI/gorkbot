import sys
import json
import subprocess
import os

def run_bash(command):
    try:
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=30
        )
        return {
            "success": result.returncode == 0,
            "output": result.stdout,
            "error": result.stderr if result.returncode != 0 else ""
        }
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def read_file(path):
    try:
        with open(path, 'r') as f:
            content = f.read()
        return {"success": True, "output": content, "error": ""}
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def get_system_info():
    info = {}
    # Try to read /proc/version
    version_result = read_file("/proc/version")
    info["kernel"] = version_result["output"] if version_result["success"] else "N/A"
    # Try to read /proc/cpuinfo
    cpu_result = read_file("/proc/cpuinfo")
    info["cpu"] = cpu_result["output"][:500] + "..." if cpu_result["success"] and len(cpu_result["output"]) > 500 else (cpu_result["output"] if cpu_result["success"] else "N/A")
    # Get PATH via bash
    path_result = run_bash("echo $PATH")
    info["path"] = path_result["output"] if path_result["success"] else "N/A"
    return {"success": True, "output": json.dumps(info, indent=2), "error": ""}

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        operation = input_data.get("operation", "")
        
        if operation == "run_bash":
            command = input_data.get("command", "echo $PATH")
            result = run_bash(command)
        elif operation == "read_file":
            path = input_data.get("path", "/proc/version")
            result = read_file(path)
        elif operation == "get_system_info":
            result = get_system_info()
        elif operation == "demo":
            # Demo 5 safe tools pattern
            results = []
            results.append({"tool": "bash echo $PATH", **run_bash("echo $PATH")})
            results.append({"tool": "read_file /proc/version", **read_file("/proc/version")})
            results.append({"tool": "bash uname -a", **run_bash("uname -a")})
            results.append({"tool": "read_file /proc/uptime", **read_file("/proc/uptime")})
            results.append({"tool": "bash date", **run_bash("date")})
            result = {"success": True, "output": json.dumps(results, indent=2), "error": ""}
        else:
            result = {"success": False, "output": "", "error": f"Unknown operation: {operation}"}
        
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
