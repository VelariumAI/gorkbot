import sys
import json
import os
import subprocess

def main():
    try:
        # Read input JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        action = input_data.get('action', 'run_bash')
        params = input_data.get('params', {})
        
        result = {
            'success': True,
            'output': '',
            'error': ''
        }
        
        if action == 'run_bash':
            command = params.get('command', 'echo $PATH')
            timeout = params.get('timeout', 30)
            
            try:
                proc = subprocess.run(
                    command,
                    shell=True,
                    capture_output=True,
                    text=True,
                    timeout=timeout
                )
                result['output'] = proc.stdout
                if proc.stderr:
                    result['error'] = proc.stderr
            except subprocess.TimeoutExpired:
                result['success'] = False
                result['error'] = 'Command timed out'
            except Exception as e:
                result['success'] = False
                result['error'] = str(e)
                
        elif action == 'read_file':
            filepath = params.get('path', '/proc/version')
            try:
                with open(filepath, 'r') as f:
                    result['output'] = f.read()
            except FileNotFoundError:
                result['success'] = False
                result['error'] = f'File not found: {filepath}'
            except PermissionError:
                result['success'] = False
                result['error'] = f'Permission denied: {filepath}'
            except Exception as e:
                result['success'] = False
                result['error'] = str(e)
                
        elif action == 'get_env':
            var = params.get('variable', 'PATH')
            result['output'] = os.environ.get(var, '')
            
        else:
            result['success'] = False
            result['error'] = f'Unknown action: {action}'
            
    except json.JSONDecodeError as e:
        result = {
            'success': False,
            'output': '',
            'error': f'Invalid JSON input: {str(e)}'
        }
    except Exception as e:
        result = {
            'success': False,
            'output': '',
            'error': str(e)
        }
    
    print(json.dumps(result))

if __name__ == '__main__':
    main()
