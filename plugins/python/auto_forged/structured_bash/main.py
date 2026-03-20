import sys
import json
import subprocess
from typing import Dict, Any

def main():
    try:
        input_data = json.load(sys.stdin)
        command = input_data.get('command', '')
        if not command:
            print(json.dumps({'success': False, 'output': '', 'error': 'No command provided'}))
            return
        
        timeout = input_data.get('timeout', 60)
        use_privileged = input_data.get('privileged', False)
        
        if use_privileged:
            cmd = f'sudo {command}'
        else:
            cmd = command
        
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        
        output = result.stdout.strip()
        error = result.stderr.strip()
        
        print(json.dumps({
            'success': result.returncode == 0,
            'output': output,
            'error': error,
            'returncode': result.returncode
        }))
    except subprocess.TimeoutExpired:
        print(json.dumps({'success': False, 'output': '', 'error': 'Command timed out'}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()