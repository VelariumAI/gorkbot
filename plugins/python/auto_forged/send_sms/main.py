import sys
import json
import subprocess

data = json.load(sys.stdin)
number = data.get('number') or data.get('phone')
message = data.get('message') or data.get('body')

if not number or not message:
    print(json.dumps({'success': False, 'output': '', 'error': 'Missing number or message'}))
    sys.exit(1)

success = False
output = ''
error = ''

# Try termux-sms-send (direct send if package installed)
try:
    result = subprocess.run(['termux-sms-send', '-n', number, message],
                            capture_output=True, text=True, timeout=15)
    if result.returncode == 0:
        success = True
        output = 'SMS sent successfully via termux-sms-send'
    else:
        error = result.stderr.strip() or result.stdout.strip()
except Exception as e:
    error = str(e)

# Fallback to am intent (opens SMS composer - no extra packages needed)
if not success:
    try:
        intent_cmd = [
            'am', 'start',
            '-a', 'android.intent.action.SENDTO',
            '-d', f'sms:{number}',
            '--es', 'sms_body', message
        ]
        result = subprocess.run(intent_cmd, capture_output=True, text=True, timeout=15)
        if result.returncode == 0:
            success = True
            output = 'SMS composer opened via Android intent'
        else:
            error = (error or '') + ' ' + (result.stderr.strip() or result.stdout.strip())
    except Exception as e:
        error = (error or '') + ' ' + str(e)

print(json.dumps({'success': success, 'output': output, 'error': error.strip()}))