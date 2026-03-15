import sys
import json
import os
import subprocess
import re

def read_input():
    try:
        return json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        return {}

def cleanup_path(path, backup_hint=True):
    """Remove files or directories at the given path."""
    if not path or not os.path.exists(path):
        return False, f"Path does not exist: {path}"
    
    # Safety check - prevent accidental system deletion
    protected_paths = ['/', '/data', '/data/data', '/system', '/vendor']
    if any(path.startswith(p) and p != path for p in protected_paths):
        return False, f"Cannot delete protected path: {path}"
    
    try:
        cmd = ['rm', '-rf', path]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return True, f"Successfully removed: {path}"
        else:
            return False, result.stderr
    except Exception as e:
        return False, str(e)

def set_env_variable(key, value, target_file='~/.bashrc'):
    """Add or update an environment variable in the specified file."""
    target_file = os.path.expanduser(target_file)
    export_line = f"export {key}={value}"
    
    # Check if the variable already exists
    existing_pattern = f"^export {key}="
    
    try:
        with open(target_file, 'r') as f:
            lines = f.readlines()
        
        updated = False
        new_lines = []
        for line in lines:
            if re.match(existing_pattern, line):
                new_lines.append(export_line + '\n')
                updated = True
            else:
                new_lines.append(line)
        
        if not updated:
            new_lines.append(export_line + '\n')
        
        with open(target_file, 'w') as f:
            f.writelines(new_lines)
        
        return True, f"Set {key} in {target_file}"
    except Exception as e:
        return False, str(e)

def source_bashrc():
    """Reload bashrc by sourcing it."""
    bashrc_path = os.path.expanduser('~/.bashrc')
    if not os.path.exists(bashrc_path):
        return False, "~/.bashrc not found"
    
    try:
        # Source bashrc in a subshell
        result = subprocess.run(
            ['bash', '-c', 'source ~/.bashrc'],
            capture_output=True,
            text=True,
            timeout=30
        )
        if result.returncode == 0:
            return True, "Successfully sourced ~/.bashrc"
        else:
            return False, result.stderr
    except Exception as e:
        return False, str(e)

def execute_bash_command(command):
    """Execute an arbitrary bash command."""
    if not command:
        return False, "No command provided"
    
    # Basic safety check
    dangerous_patterns = ['rm -rf /', 'mkfs', 'dd if=', ':(){:|:&};:']
    for pattern in dangerous_patterns:
        if pattern in command:
            return False, f"Command blocked for safety: {pattern}"
    
    try:
        result = subprocess.run(
            ['bash', '-c', command],
            capture_output=True,
            text=True,
            timeout=60
        )
        output = result.stdout if result.returncode == 0 else result.stderr
        return result.returncode == 0, output
    except Exception as e:
        return False, str(e)

def main():
    input_data = read_input()
    
    operation = input_data.get('operation', '')
    
    if operation == 'cleanup':
        path = input_data.get('path', '')
        success, output = cleanup_path(path)
    
    elif operation == 'set_env':
        key = input_data.get('key', '')
        value = input_data.get('value', '')
        target = input_data.get('target', '~/.bashrc')
        if not key or not value:
            success, output = False, "Missing key or value"
        else:
            success, output = set_env_variable(key, value, target)
    
    elif operation == 'source':
        success, output = source_bashrc()
    
    elif operation == 'execute':
        command = input_data.get('command', '')
        success, output = execute_bash_command(command)
    
    else:
        success = False
        output = f"Unknown operation: {operation}. Supported: cleanup, set_env, source, execute"
    
    result = {
        "success": success,
        "output": output,
        "error": "" if success else output
    }
    
    print(json.dumps(result))

if __name__ == '__main__':
    main()
