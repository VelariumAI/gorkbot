import sys
import json
import subprocess
import os
import tempfile

try:
    data = json.load(sys.stdin)
    command = data.get('command', '')
    log = data.get('log', False)
    timeout = data.get('timeout', 30)

    if not command:
        raise ValueError('No command provided')

    test_dir = '/tmp/gorkbot_test'
    if log:
        os.makedirs(test_dir, exist_ok=True)
        log_file = os.path.join(test_dir, 'bash_log.txt')

    result = subprocess.run(['bash', '-c', command], capture_output=True, text=True, timeout=timeout)

    output = result.stdout.strip()
    error = result.stderr.strip() if result.returncode != 0 else ''
    success = result.returncode == 0

    if log:
        with open(log_file, 'a') as f:
            f.write(f'Command: {command}\nSuccess: {success}\nOutput: {output}\nError: {error}\n---\n')

    print(json.dumps({
        'success': success,
        'output': output,
        'error': error
    }))

except subprocess.TimeoutExpired:
    print(json.dumps({
        'success': False,
        'output': '',
        'error': 'Command timed out'
    }))
except Exception as e:
    print(json.dumps({
        'success': False,
        'output': '',
        'error': str(e)
    }));
