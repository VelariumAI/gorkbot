import sys
import json
import subprocess
from pathlib import Path

data = json.load(sys.stdin)
operation = data.get('operation', '')
args = data.get('args', {})

result = {'success': False, 'output': '', 'error': 'Unknown error'}

try:
    if operation == 'run_bash':
        cmd = args.get('command', '')
        if not cmd:
            result['error'] = 'No command provided'
        elif any(c in cmd for c in ['\n', '\r', ';', '&', '|', '`', '$(']):
            result['error'] = 'Control characters or dangerous sequences detected (MEL heuristic)'
        else:
            res = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=15)
            result = {'success': True, 'output': res.stdout.strip(), 'error': res.stderr.strip()}
    elif operation == 'read_file':
        path = args.get('path', '')
        if not path or ('..' in path) or path.startswith('/sdcard'):
            result['error'] = 'Path outside allowed sandbox'
        else:
            content = Path(path).read_text(encoding='utf-8')
            result = {'success': True, 'output': content, 'error': ''}
    elif operation == 'write_file':
        path = args.get('path', '')
        content = args.get('content', '')
        if not path or ('..' in path) or path.startswith('/sdcard'):
            result['error'] = 'Path outside allowed sandbox'
        else:
            Path(path).write_text(content, encoding='utf-8')
            result = {'success': True, 'output': f'Written to {path}', 'error': ''}
    elif operation == 'git_status':
        res = subprocess.run('git status', shell=True, capture_output=True, text=True, timeout=10, cwd=args.get('cwd', '.'))
        result = {'success': True, 'output': res.stdout.strip(), 'error': res.stderr.strip()}
    elif operation == 'grep_content':
        pattern = args.get('pattern', '')
        path = args.get('path', '.')
        res = subprocess.run(f'grep -r "{pattern}" {path}', shell=True, capture_output=True, text=True, timeout=10)
        result = {'success': True, 'output': res.stdout.strip(), 'error': res.stderr.strip()}
    else:
        result['error'] = 'Unsupported operation'
except Exception as e:
    result['error'] = str(e)

print(json.dumps(result))