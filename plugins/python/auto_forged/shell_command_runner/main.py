import sys
import json
import os
import subprocess
import re

def run_shell_command(command, timeout=30):
    try:
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        return {
            'success': result.returncode == 0,
            'output': result.stdout,
            'error': result.stderr if result.returncode != 0 else '',
            'returncode': result.returncode
        }
    except subprocess.TimeoutExpired:
        return {'success': False, 'output': '', 'error': 'Command timed out', 'returncode': -1}
    except Exception as e:
        return {'success': False, 'output': '', 'error': str(e), 'returncode': -1}

def read_file(path, encoding='utf-8'):
    try:
        path = os.path.expanduser(path)
        with open(path, 'r', encoding=encoding) as f:
            content = f.read()
        return {'success': True, 'output': content, 'error': ''}
    except FileNotFoundError:
        return {'success': False, 'output': '', 'error': f'File not found: {path}'}
    except Exception as e:
        return {'success': False, 'output': '', 'error': str(e)}

def write_file(path, content, encoding='utf-8'):
    try:
        path = os.path.expanduser(path)
        with open(path, 'w', encoding=encoding) as f:
            f.write(content)
        return {'success': True, 'output': f'Written to {path}', 'error': ''}
    except Exception as e:
        return {'success': False, 'output': '', 'error': str(e)}

def get_system_load():
    try:
        with open('/proc/loadavg', 'r') as f:
            load = f.read().strip().split()
        return {
            'success': True,
            'output': json.dumps({
                'load_1min': load[0],
                'load_5min': load[1],
                'load_15min': load[2],
                'running_processes': load[3]
            }),
            'error': ''
        }
    except Exception as e:
        return {'success': False, 'output': '', 'error': str(e)}

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({'success': False, 'output': '', 'error': 'Invalid JSON input'}))
        return

    action = input_data.get('action', '')
    
    if action == 'run_command':
        command = input_data.get('command', '')
        if not command:
            print(json.dumps({'success': False, 'output': '', 'error': 'No command provided'}))
            return
        result = run_shell_command(command, input_data.get('timeout', 30))
        print(json.dumps(result))
    
    elif action == 'read_file':
        path = input_data.get('path', '')
        if not path:
            print(json.dumps({'success': False, 'output': '', 'error': 'No path provided'}))
            return
        result = read_file(path, input_data.get('encoding', 'utf-8'))
        print(json.dumps(result))
    
    elif action == 'write_file':
        path = input_data.get('path', '')
        content = input_data.get('content', '')
        if not path:
            print(json.dumps({'success': False, 'output': '', 'error': 'No path provided'}))
            return
        result = write_file(path, content, input_data.get('encoding', 'utf-8'))
        print(json.dumps(result))
    
    elif action == 'system_load':
        result = get_system_load()
        print(json.dumps(result))
    
    else:
        print(json.dumps({'success': False, 'output': '', 'error': f'Unknown action: {action}'}))

if __name__ == '__main__':
    main()