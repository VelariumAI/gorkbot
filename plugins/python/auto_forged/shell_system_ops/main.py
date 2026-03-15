import sys
import json
import subprocess
import os
import re

def escape_bash_params(param):
    """Escape parameters to prevent injection - MEL heuristic"""
    if not param:
        return ""
    # Use single quotes and escape existing single quotes
    return param.replace("'", "'\\''")

def read_system_file(filepath):
    """Safely read a system file with fallback for permission issues"""
    try:
        with open(filepath, 'r') as f:
            return f.read(), None
    except PermissionError:
        # Try with chmod for Termux environments
        try:
            os.chmod(filepath, 0o644)
            with open(filepath, 'r') as f:
                return f.read(), None
        except Exception as e:
            return None, f"Permission denied: {str(e)}"
    except FileNotFoundError:
        return None, f"File not found: {filepath}"
    except Exception as e:
        return None, f"Error reading {filepath}: {str(e)}"

def run_bash_command(command, retries=2):
    """Run bash command with retry logic for transient errors (MEL heuristic)"""
    # Escape {{params}} pattern to prevent injection
    command = re.sub(r'\{\{([^}]+)\}\}', lambda m: escape_bash_params(m.group(1)), command)
    
    for attempt in range(retries):
        try:
            result = subprocess.run(
                command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=30
            )
            
            # Handle EPIPE and transient errors
            if result.returncode == 0:
                return result.stdout, None
            elif "EPIPE" in str(result.stderr) and attempt < retries - 1:
                continue  # Retry on EPIPE (MEL heuristic)
            else:
                return None, f"Command failed: {result.stderr}"
                
        except subprocess.TimeoutExpired:
            if attempt < retries - 1:
                continue
            return None, "Command timed out"
        except Exception as e:
            return None, f"Execution error: {str(e)}"
    
    return None, "Max retries exceeded"

def list_processes():
    """List running processes"""
    try:
        result = subprocess.run(
            ["ps", "aux"],
            capture_output=True,
            text=True,
            timeout=10
        )
        return result.stdout, None
    except Exception as e:
        return None, f"Error listing processes: {str(e)}"

def get_system_info():
    """Gather basic system information"""
    info = {}
    
    # Read /proc/version
    version, err = read_system_file("/proc/version")
    if version:
        info["kernel"] = version.strip()
    
    # Get PATH
    info["path"] = os.environ.get("PATH", "")
    
    # Get current user
    info["user"] = os.environ.get("USER", "unknown")
    
    # Platform info
    info["platform"] = os.environ.get("PLATFORM", sys.platform)
    
    return json.dumps(info, indent=2), None

def main():
    try:
        # Read JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        action = input_data.get("action", "run")
        command = input_data.get("command", "")
        filepath = input_data.get("filepath", "")
        
        output = ""
        error = ""
        
        if action == "run":
            output, error = run_bash_command(command)
        elif action == "read_file":
            output, error = read_system_file(filepath)
        elif action == "list_processes":
            output, error = list_processes()
        elif action == "system_info":
            output, error = get_system_info()
        elif action == "echo_env":
            # Safe way to echo environment variable
            var_name = escape_bash_params(input_data.get("var", "PATH"))
            output, error = run_bash_command(f'echo ${var_name}')
        else:
            error = f"Unknown action: {action}"
        
        # Build response
        if error:
            result = {"success": False, "output": "", "error": error}
        else:
            result = {"success": True, "output": output, "error": ""}
            
    except json.JSONDecodeError as e:
        result = {"success": False, "output": "", "error": f"Invalid JSON input: {str(e)}"}
    except Exception as e:
        result = {"success": False, "output": "", "error": f"Unexpected error: {str(e)}"}
    
    print(json.dumps(result))

if __name__ == "__main__":
    main()