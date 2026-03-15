import sys
import json
import subprocess
import os

def main():
    try:
        # Read JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Extract command and optional parameters
        command = input_data.get('command', '')
        working_dir = input_data.get('working_dir', None)
        timeout = input_data.get('timeout', 30)
        
        if not command:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "No command provided"
            }))
            return
        
        # Set up environment
        env = os.environ.copy()
        env['TERM'] = 'xterm-256color'
        
        # Execute command
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=working_dir,
            env=env
        )
        
        output = result.stdout + result.stderr
        
        print(json.dumps({
            "success": result.returncode == 0,
            "output": output,
            "error": "" if result.returncode == 0 else f"Exit code: {result.returncode}"
        }))
        
    except subprocess.TimeoutExpired:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": "Command timed out"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
