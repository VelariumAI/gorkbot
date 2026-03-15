import sys
import json
import subprocess
import os

def main():
    try:
        # Read JSON from stdin
        input_data = json.load(sys.stdin)
        
        # Extract operation type and parameters
        operation = input_data.get('operation', 'execute')
        command = input_data.get('command') or input_data.get('cmd', '')
        filepath = input_data.get('filepath', '')
        timeout = input_data.get('timeout', 30)
        shell = input_data.get('shell', True)
        
        if operation == 'read_file':
            # Read a system file (like /proc/version)
            if not filepath:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": "No filepath provided for read_file operation"
                }))
                return
            
            try:
                # Security: restrict to safe system paths
                safe_paths = ['/proc/', '/sys/', '/etc/']
                if not any(filepath.startswith(p) for p in safe_paths):
                    # Allow relative paths in home or current dir
                    if not filepath.startswith('/'):
                        filepath = os.path.expanduser(filepath)
                    else:
                        print(json.dumps({
                            "success": False,
                            "output": "",
                            "error": " filepath must be in /proc/, /sys/, /etc/ or be a relative path"
                        }))
                        return
                
                with open(filepath, 'r') as f:
                    content = f.read()
                print(json.dumps({
                    "success": True,
                    "output": content,
                    "error": ""
                }))
            except PermissionError:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": f"Permission denied: {filepath}"
                }))
            except FileNotFoundError:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": f"File not found: {filepath}"
                }))
            except Exception as e:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": str(e)
                }))
                
        elif operation == 'execute' or operation == 'bash':
            # Execute shell command
            if not command:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": "No command provided"
                }))
                return
            
            try:
                result = subprocess.run(
                    command,
                    shell=shell,
                    capture_output=True,
                    text=True,
                    timeout=timeout,
                    env=os.environ.copy()
                )
                
                output = result.stdout
                if result.stderr:
                    output += "\n" + result.stderr
                
                print(json.dumps({
                    "success": result.returncode == 0,
                    "output": output,
                    "error": "" if result.returncode == 0 else f"Exit code: {result.returncode}"
                }))
                
            except subprocess.TimeoutExpired:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": f"Command timed out after {timeout} seconds"
                }))
            except Exception as e:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": str(e)
                }))
        
        elif operation == 'env':
            # Get environment variable
            var_name = input_data.get('var', 'PATH')
            value = os.environ.get(var_name, '')
            print(json.dumps({
                "success": True,
                "output": value,
                "error": ""
            }))
        
        else:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": f"Unknown operation: {operation}. Supported: execute, read_file, env"
            }))
    
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"JSON parse error: {str(e)}"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
