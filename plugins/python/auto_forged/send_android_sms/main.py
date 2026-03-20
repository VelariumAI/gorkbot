import sys
import json
import subprocess

data = json.load(sys.stdin)
phone = data.get('phone_number') or data.get('to')
message = data.get('message') or data.get('body')

if not phone or not message:
    print(json.dumps({'success': False, 'output': '', 'error': 'Missing phone_number/to or message/body'}))
    sys.exit(1)

cmd = [
    'am', 'start',
    '-a', 'android.intent.action.SENDTO',
    '-d', f'sms:{phone}',
    '--es', 'sms_body', message
]

try:
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
    output = (result.stdout + result.stderr).strip()
    success = result.returncode == 0 and 'Starting: Intent' in output
    print(json.dumps({
        'success': success,
        'output': output,
        'error': '' if success else f'Command failed with code {result.returncode}'
    }))
except Exception as e:
    print(json.dumps({'success': False, 'output': '', 'error': str(e)}))