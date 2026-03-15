import sys
import json
import subprocess
import os
import shutil

data = json.load(sys.stdin)
category = data.get('category', 'pictures')
target_name = data.get('target_name', 'picked_file')
ext = data.get('extension', 'jpg')
output_path = f'./{target_name}.{ext}'

try:
    result = subprocess.run(['termux-storage-get', category], capture_output=True, text=True, timeout=120)
    stdout = result.stdout.strip()
    if os.path.exists(stdout):
        shutil.copy(stdout, output_path)
        msg = f'Successfully imported file to sandbox: {output_path}'
        success = True
        err = ''
    else:
        msg = f'Picker executed. Raw output: {stdout}. File may be available in sandbox as {output_path}'
        success = True
        err = ''
except subprocess.TimeoutExpired:
    success = False
    msg = ''
    err = 'Timeout waiting for storage picker'
except subprocess.CalledProcessError as e:
    success = False
    msg = ''
    err = f'Command error: {str(e)}'
except Exception as e:
    success = False
    msg = ''
    err = str(e)

print(json.dumps({
    'success': success,
    'output': msg,
    'error': err,
    'file_path': output_path if success else ''
}))