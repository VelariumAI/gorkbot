import sys
import json
import subprocess
import os

def main():
    try:
        data = json.load(sys.stdin)
        action = data.get('action', '')
        result = ''
        
        if action == 'echo_path':
            # Safe: echo $PATH
            result = subprocess.check_output(['echo', os.environ.get('PATH', '')], text=True)
            
        elif action == 'read_file':
            # Safe: read system file
            filepath = data.get('path', '/proc/version')
            if filepath.startswith('/proc/') or filepath.startswith('/sys/'):
                with open(filepath, 'r') as f:
                    result = f.read()
            else:
                raise PermissionError(f'Safe mode: only /proc/* and /sys/* allowed')
                
        elif action == 'bash':
            # Safe: run a bash command with no shell features
            cmd = data.get('command', '')
            # Block dangerous characters
            if any(c in cmd for c in ['&&', '||', ';', '|', '>', '<', '`', '$(']):
                raise ValueError('Dangerous shell operators blocked')
            result = subprocess.check_output(cmd, shell=False, text=True, executable='/bin/sh')
            
        elif action == 'list_processes':
            # Safe: list running processes
            result = subprocess.check_output(['ps', 'aux'], text=True)
            
        elif action == 'system_info':
            # Safe: read multiple system files
            info = {}
            for fpath in ['/proc/version', '/proc/uptime', '/proc/meminfo']:
                try:
                    with open(fpath, 'r') as f:
                        info[fpath] = f.read().strip()
                except:
                    info[fpath] = 'unavailable'
            result = json.dumps(info, indent=2)
            
        else:
            result = f'Unknown action: {action}. Supported: echo_path, read_file, bash, list_processes, system_info'
        
        print(json.dumps({'success': True, 'output': str(result), 'error': ''}))
        
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
