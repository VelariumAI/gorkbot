import sys
import json
import subprocess
import os
import re

def run_bash(command):
    """Execute a bash command and return output."""
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
            "output": result.stdout.strip(),
            "error": result.stderr.strip() if result.returncode != 0 else ""
        }
    except subprocess.TimeoutExpired:
        return {"success": False, "output": "", "error": "Command timed out"}
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def read_file_safely(filepath):
    """Read a file safely, handling permission issues."""
    # Escape any template variables in filepath
    filepath = filepath.replace("{{params}}", "")
    
    # Security: only allow safe system paths
    allowed_paths = ["/proc/", "/sys/", "/etc/"]
    if not any(filepath.startswith(p) for p in allowed_paths):
        # Try home directory as fallback for Termux
        if filepath.startswith("~/"):
            filepath = os.path.expanduser(filepath)
        else:
            return {"success": False, "output": "", "error": "Path not in allowed system paths"}
    
    try:
        with open(filepath, 'r') as f:
            content = f.read()
        return {"success": True, "output": content.strip(), "error": ""}
    except PermissionError:
        # Try with elevated permissions or alternative path
        return {"success": False, "output": "", "error": f"Permission denied: {filepath}"}
    except FileNotFoundError:
        return {"success": False, "output": "", "error": f"File not found: {filepath}"}
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def main():
    try:
        # Read JSON input from stdin
        input_data = json.loads(sys.stdin.read())
        action = input_data.get("action", "bash")
        
        if action == "bash":
            command = input_data.get("command", "echo $PATH")
            # Escape template variables
            command = command.replace("{{params}}", "")
            result = run_bash(command)
            
        elif action == "read_file":
            filepath = input_data.get("filepath", "/proc/version")
            result = read_file_safely(filepath)
            
        elif action == "env":
            # Get environment variable
            var = input_data.get("var", "PATH")
            result = run_bash(f"echo ${var}")
            
        else:
            result = {"success": False, "output": "", "error": f"Unknown action: {action}"}
        
        print(json.dumps(result))
        
    except json.JSONDecodeError as e:
        print(json.dumps({"success": False, "output": "", "error": f"Invalid JSON input: {str(e)}"}))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
