import sys
import json
import subprocess
import os

def run_bash(command, timeout=30):
    """Execute a bash command with error handling."""
    try:
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        return {
            "success": result.returncode == 0,
            "output": result.stdout.strip(),
            "error": result.stderr.strip() if result.returncode != 0 else "",
            "returncode": result.returncode
        }
    except subprocess.TimeoutExpired:
        return {"success": False, "output": "", "error": "Command timed out"}
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def read_system_file(filepath):
    """Read a system file with fallback paths for common permission issues."""
    paths_to_try = [filepath, os.path.expanduser(filepath)]
    
    for path in paths_to_try:
        try:
            with open(path, 'r') as f:
                content = f.read()
                return {"success": True, "output": content.strip(), "error": ""}
        except PermissionError:
            # Try with sudo/termux-specific paths
            alt_path = f"~/../usr{path}" if path.startswith("/") else path
            try:
                result = run_bash(f"cat {alt_path}")
                if result["success"]:
                    return result
            except:
                pass
            return {"success": False, "output": "", "error": f"Permission denied: {path}"}
        except FileNotFoundError:
            continue
        except Exception as e:
            return {"success": False, "output": "", "error": str(e)}
    
    return {"success": False, "output": "", "error": f"File not found: {filepath}"}

def execute_operation(op_type, params):
    """Execute the appropriate operation based on type."""
    if op_type == "bash":
        command = params.get("command", "")
        timeout = params.get("timeout", 30)
        return run_bash(command, timeout)
    
    elif op_type == "read_file":
        filepath = params.get("filepath", "")
        return read_system_file(filepath)
    
    elif op_type == "system_info":
        # Common system info commands
        info_type = params.get("type", "version")
        commands = {
            "version": "cat /proc/version",
            "uptime": "uptime",
            "memory": "cat /proc/meminfo",
            "cpuinfo": "cat /proc/cpuinfo"
        }
        cmd = commands.get(info_type, commands["version"])
        return run_bash(cmd)
    
    elif op_type == "env_check":
        # Check environment variables
        var = params.get("variable", "PATH")
        return run_bash(f"echo ${var}")
    
    else:
        return {"success": False, "output": "", "error": f"Unknown operation: {op_type}"}

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
        return
    
    op_type = input_data.get("operation", "bash")
    params = input_data.get("params", {})
    
    result = execute_operation(op_type, params)
    
    output = {
        "success": result.get("success", False),
        "output": result.get("output", ""),
        "error": result.get("error", "")
    }
    
    print(json.dumps(output))

if __name__ == "__main__":
    main()