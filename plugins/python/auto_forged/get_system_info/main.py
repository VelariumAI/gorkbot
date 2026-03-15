import sys
import json
import subprocess

data = json.load(sys.stdin)
params = data.get('parameters', {}) if isinstance(data, dict) else {}

commands = [
    'uname -a',
    'getprop ro.build.version.release',
    'getprop ro.product.model',
    'free -h',
    'df -h /data',
    'df -h'
]

output_lines = ['System Diagnostics:']
for cmd in commands:
    try:
        result = subprocess.getoutput(cmd)
        output_lines.append(f'$ {cmd}')
        output_lines.append(result)
        output_lines.append('---')
    except Exception as e:
        output_lines.append(f'Error running {cmd}: {str(e)}')

summary = '\n'.join(output_lines)

print(json.dumps({
    'success': True,
    'output': summary,
    'error': ''
}, indent=2))