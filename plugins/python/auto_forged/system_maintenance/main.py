import sys
import json
import os
import subprocess
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
        return result.stdout, result.stderr, result.returncode
    except Exception as e:
        return "", str(e), 1

def delete_files(paths, backup_warning=True):
    """Delete specified files or directories."""
    results = []
    for path in paths:
        if backup_warning and not path.startswith('/storage/emulated'):
            results.append(f"Skipped {path} - backup recommended before deletion")
            continue
        stdout, stderr, code = run_bash(f"rm -rf {path}")
        if code == 0:
            results.append(f"Deleted: {path}")
        else:
            results.append(f"Failed to delete {path}: {stderr}")
    return results

def clear_cache(cache_type):
    """Clear various types of caches."""
    if cache_type == "package_manager":
        stdout, stderr, code = run_bash("pm clear com.android.providers.downloads")
        return [f"Package manager cache cleared: {stdout}"] if code == 0 else [f"Failed: {stderr}"]
    elif cache_type == "termux":
        stdout, stderr, code = run_bash("rm -rf ~/cache/* ~/.cache/* 2>/dev/null")
        return ["Termux cache cleared"] if code == 0 else [f"Failed: {stderr}"]
    else:
        return [f"Unknown cache type: {cache_type}"]

def set_env_var(key, value):
    """Set environment variable in bashrc."""
    bashrc_path = os.path.expanduser("~/.bashrc")
    export_line = f"export {key}={value}"
    
    # Check if already exists
    with open(bashrc_path, "a") as f:
        f.write(f"\n{export_line}\n")
    
    return [f"Added {export_line} to ~/.bashrc"]

def reload_bashrc():
    """Reload bashrc environment."""
    stdout, stderr, code = run_bash("source ~/.bashrc")
    if code == 0:
        return ["Bashrc reloaded successfully"]
    return [f"Failed to reload bashrc: {stderr}"]

def restart_service(service_name):
    """Restart a service (like gorkbot)."""
    if service_name == "gorkbot":
        stdout, stderr, code = run_bash("pkill gorkbot && cd $(find /storage -name 'gorkbot.sh' 2>/dev/null | head -1) && ./gorkbot.sh")
        if code == 0:
            return ["Gorkbot restarted"]
        return [f"Failed to restart gorkbot: {stderr}"]
    return [f"Unknown service: {service_name}"]

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
        return
    
    operation = input_data.get("operation", "")
    params = input_data.get("params", {})
    
    results = []
    error = ""
    
    if operation == "delete_files":
        results = delete_files(params.get("paths", []), params.get("backup_warning", True))
    
    elif operation == "clear_cache":
        results = clear_cache(params.get("cache_type", ""))
    
    elif operation == "set_env":
        results = set_env_var(params.get("key", ""), params.get("value", ""))
    
    elif operation == "reload_bashrc":
        results = reload_bashrc()
    
    elif operation == "restart_service":
        results = restart_service(params.get("service_name", ""))
    
    elif operation == "full_cleanup":
        # Run all cleanup operations
        if "paths" in params:
            results.extend(delete_files(params["paths"], params.get("backup_warning", True)))
        if "cache_type" in params:
            results.extend(clear_cache(params["cache_type"]))
        results.extend(reload_bashrc())
    
    else:
        error = f"Unknown operation: {operation}"
    
    print(json.dumps({
        "success": error == "",
        "output": "\n".join(results),
        "error": error
    }))

if __name__ == "__main__":
    main()
