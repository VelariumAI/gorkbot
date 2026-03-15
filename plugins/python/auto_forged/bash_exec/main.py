import sys
import json
import subprocess
import os

def main():
    try:
        # Read JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        command = input_data.get('command', '')
        
        if not command:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "No command provided"
            }))
            return
        
        # Get current working directory
        cwd = os.getcwd()
        
        # Execute the bash command
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            cwd=cwd
        )
        
        output = result.stdout
        error = result.stderr
        
        # If command was 'uptime', add load average info
        if 'uptime' in command:
            uptime_info = subprocess.run(
                "uptime",
                shell=True,
                capture_output=True,
                text=True
            )
            output = uptime_info.stdout
            error = uptime_info.stderr
        
        # If command was 'ls' or 'list', show directory contents
        if command in ['ls', 'ls -la', 'dir'] or 'ls' in command:
            list_info = subprocess.run(
                "ls -la" if 'la' in command else "ls",
                shell=True,
                capture_output=True,
                text=True
            )
            output = list_info.stdout
            error = list_info.stderr
        
        print(json.dumps({
            "success": result.returncode == 0,
            "output": output,
            "error": error
        }))
        
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
