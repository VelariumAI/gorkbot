import sys
import json
import subprocess

data = json.load(sys.stdin)
path = data.get('path', '/')
depth = data.get('depth', 1)
action = data.get('action', 'scan')
sort = data.get('sort', False)

try:
    if action == 'df':
        cmd = ['df', '-h', path]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        output = result.stdout.strip()
    elif action == 'scan':
        cmd = ['du', '-sh', f'--max-depth={depth}', path]
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        output = result.stdout.strip()
        if sort:
            lines = output.split('\n')
            lines.sort(key=lambda x: int(x.split()[0][:-1]) if x.split()[0][-1].isdigit() else 0, reverse=True)
            output = '\n'.join(lines)
    else:
        output = 'Unknown action'
    print(json.dumps({'success': True, 'output': output, 'error': ''}))
except subprocess.CalledProcessError as e:
    print(json.dumps({'success': False, 'output': '', 'error': f'Command failed: {str(e)}'}))
except Exception as e:
    print(json.dumps({'success': False, 'output': '', 'error': str(e)}))