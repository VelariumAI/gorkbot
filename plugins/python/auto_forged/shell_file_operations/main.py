import sys
import json
import subprocess
import os
import re
from pathlib import Path

def run_shell_command(command, timeout=30):
    """Run a shell command and return structured output."""
    try:
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        return {
            "stdout": result.stdout,
            "stderr": result.stderr,
            "returncode": result.returncode,
            "success": result.returncode == 0
        }
    except subprocess.TimeoutExpired:
        return {
            "stdout": "",
            "stderr": "Command timed out",
            "returncode": -1,
            "success": False
        }
    except Exception as e:
        return {
            "stdout": "",
            "stderr": str(e),
            "returncode": -1,
            "success": False
        }

def read_file_content(path, encoding="utf-8"):
    """Read file content from specified path."""
    try:
        expanded_path = os.path.expanduser(path)
        with open(expanded_path, 'r', encoding=encoding) as f:
            content = f.read()
        return {"success": True, "content": content, "path": expanded_path}
    except FileNotFoundError:
        return {"success": False, "content": "", "error": "File not found"}
    except Exception as e:
        return {"success": False, "content": "", "error": str(e)}

def write_file_content(path, content, encoding="utf-8"):
    """Write content to specified path."""
    try:
        expanded_path = os.path.expanduser(path)
        os.makedirs(os.path.dirname(expanded_path), exist_ok=True)
        with open(expanded_path, 'w', encoding=encoding) as f:
            f.write(content)
        return {"success": True, "path": expanded_path}
    except Exception as e:
        return {"success": False, "error": str(e)}

def get_system_metrics():
    """Get CPU and load metrics."""
    metrics = {}
    
    # CPU usage
    cpu_result = run_shell_command("top -bn1 | grep 'Cpu(s)' | sed 's/.*, *\([0-9.]*\)%* id.*/\1/' | awk '{print 100 - $1}'")
    if cpu_result["success"] and cpu_result["stdout"].strip():
        try:
            metrics["cpu_percent"] = float(cpu_result["stdout"].strip())
        except ValueError:
            metrics["cpu_percent"] = 0.0
    
    # Load average
    load_result = run_shell_command("uptime | grep -o 'load average.*' | sed 's/load average: //'")
    if load_result["success"]:
        metrics["load_avg"] = load_result["stdout"].strip()
    
    # Memory usage
    mem_result = run_shell_command("free -m | grep Mem")
    if mem_result["success"] and mem_result["stdout"]:
        parts = mem_result["stdout"].split()
        if len(parts) >= 3:
            metrics["memory_total_mb"] = int(parts[1])
            metrics["memory_used_mb"] = int(parts[2])
    
    return metrics

def process_request(data):
    """Process incoming JSON request."""
    action = data.get("action", "unknown")
    params = data.get("parameters", {})
    
    result = {"success": True, "output": {}, "error": ""}
    
    if action == "shell" or action == "structured_bash":
        command = params.get("command", "")
        timeout = params.get("timeout", 30)
        shell_result = run_shell_command(command, timeout)
        result["output"] = shell_result
        
    elif action == "read_file":
        path = params.get("path", "")
        encoding = params.get("encoding", "utf-8")
        result["output"] = read_file_content(path, encoding)
        
    elif action == "write_file":
        path = params.get("path", "")
        content = params.get("content", "")
        encoding = params.get("encoding", "utf-8")
        result["output"] = write_file_content(path, content, encoding)
        
    elif action == "system_monitor" or action == "sensor_read":
        result["output"] = get_system_metrics()
        
    elif action == "disk_usage":
        path = params.get("path", "/")
        du_result = run_shell_command(f"df -h {path}")
        result["output"] = {"disk_usage": du_result["stdout"], "success": du_result["success"]}
        
    else:
        result["success"] = False
        result["error"] = f"Unknown action: {action}"
    
    return result

def main():
    """Main entry point - read JSON from stdin, process, write JSON to stdout."""
    try:
        input_data = json.loads(sys.stdin.read())
        output = process_request(input_data)
        print(json.dumps(output))
    except json.JSONDecodeError as e:
        print(json.dumps({"success": False, "output": {}, "error": f"JSON decode error: {str(e)}"}))
    except Exception as e:
        print(json.dumps({"success": False, "output": {}, "error": str(e)}))

if __name__ == "__main__":
    main()
