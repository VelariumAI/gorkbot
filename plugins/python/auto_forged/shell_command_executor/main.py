import sys
import json
import subprocess
import os
import shlex

def main():
    try:
        # Read JSON from stdin
        input_data = sys.stdin.read().strip()
        if not input_data:
            print(json.dumps({"success": False, "output": "", "error": "No input data provided"}))
            return
        
        try:
            data = json.loads(input_data)
        except json.JSONDecodeError as e:
            print(json.dumps({"success": False, "output": "", "error": f"Invalid JSON: {str(e)}"}))
            return
        
        # Extract command and parameters
        command = data.get("command", "")
        args = data.get("args", [])
        timeout = data.get("timeout", 30)
        
        if not command:
            print(json.dumps({"success": False, "output": "", "error": "No command specified"}))
            return
        
        # Safe list of allowed commands (whitelist approach)
        allowed_commands = {
            "echo", "cat", "pwd", "ls", "whoami", "date", "hostname",
            "uname", "id", "which", "env", "printenv", "readlink",
            "head", "tail", "wc", "cut", "sort", "uniq", "grep", "find"
        }
        
        # Validate command is in whitelist or is a read-only operation
        cmd_parts = command.split()
        base_cmd = cmd_parts[0] if cmd_parts else ""
        
        # For safety, only allow whitelisted commands or system file reads
        if base_cmd not in allowed_commands:
            # Check if it's reading a system file (like /proc/version)
            if not (base_cmd == "cat" and len(cmd_parts) > 1 and cmd_parts[1].startswith("/proc/")):
                print(json.dumps({"success": False, "output": "", "error": f"Command '{base_cmd}' not allowed. Use whitelisted commands only."}))
                return
        
        # Build full command with escaped arguments (MEL heuristic: Escape {{params}})
        full_cmd = [command]
        for arg in args:
            # Properly escape parameters to prevent injection
            full_cmd.append(shlex.quote(str(arg)))
        
        # Execute command safely
        try:
            result = subprocess.run(
                " ".join(full_cmd) if args else command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=timeout,
                env=os.environ.copy()
            )
            
            output = result.stdout.strip() if result.stdout else ""
            stderr = result.stderr.strip() if result.stderr else ""
            
            if result.returncode == 0:
                print(json.dumps({"success": True, "output": output, "error": ""}))
            else:
                print(json.dumps({"success": False, "output": output, "error": stderr or f"Command failed with exit code {result.returncode}"}))
                
        except subprocess.TimeoutExpired:
            print(json.dumps({"success": False, "output": "", "error": "Command timed out"}))
        except OSError as e:
            print(json.dumps({"success": False, "output": "", "error": f"OS error: {str(e)}"}))
    
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": f"Unexpected error: {str(e)}"}))

if __name__ == "__main__":
    main()
