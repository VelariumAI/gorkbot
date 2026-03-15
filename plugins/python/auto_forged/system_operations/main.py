import sys
import json
import subprocess
import os

def run_command(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        return {
            "success": result.returncode == 0,
            "output": result.stdout.strip(),
            "error": result.stderr.strip() if result.returncode != 0 else ""
        }
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def system_monitor():
    cmd = "top -bn1 | head -20"
    return run_command(cmd)

def list_processes():
    cmd = "ps aux | head -50"
    return run_command(cmd)

def clear_cache():
    cache_path = os.path.expanduser("~/.cache")
    if os.path.exists(cache_path):
        cmd = f"rm -rf {cache_path}/*"
        return run_command(cmd)
    return {"success": False, "output": "", "error": "Cache directory not found"}

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        operation = input_data.get("operation", "")
        
        if operation == "system_monitor":
            result = system_monitor()
        elif operation == "list_processes":
            result = list_processes()
        elif operation == "clear_cache":
            result = clear_cache()
        else:
            result = {"success": False, "output": "", "error": f"Unknown operation: {operation}"}
        
        print(json.dumps(result))
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
