import sys
import json
import subprocess
import os

def main():
    try:
        # Read input JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Extract parameters
        repo_path = input_data.get('repo_path', '.')
        count = input_data.get('count', 5)
        format_type = input_data.get('format', 'fuller')
        
        # Validate repo_path exists
        if not os.path.isdir(repo_path):
            print(json.dumps({
                "success": False,
                "output": "",
                "error": f"Repository path does not exist: {repo_path}"
            }))
            return
        
        # Build git log command
        cmd = ['git', 'log', f'--pretty={format_type}', f'-{count}']
        
        # Execute git log
        result = subprocess.run(
            cmd,
            cwd=repo_path,
            capture_output=True,
            text=True
        )
        
        if result.returncode != 0:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": result.stderr
            }))
            return
        
        # Success
        print(json.dumps({
            "success": True,
            "output": result.stdout,
            "error": ""
        }))
        
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Invalid JSON input: {str(e)}"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == '__main__':
    main()
