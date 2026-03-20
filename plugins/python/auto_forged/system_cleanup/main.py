import sys
import json
import subprocess
def run_cmd(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, executable='/bin/bash')
        return f"{cmd}\n{result.stdout}{result.stderr}"
    except Exception as e:
        return f"Error running {cmd}: {str(e)}"
data = json.load(sys.stdin)
params = data.get('parameters', {}) if isinstance(data, dict) else {}
targets = params.get('targets', ['package_cache', 'temp_files'])
safe_mode = params.get('safe_mode', True)
if not safe_mode:
    print(json.dumps({'success': False, 'output': '', 'error': 'safe_mode is required for security'}))
    sys.exit(1)
output_lines = ['=== System Cleanup Started ===']
cmds = []
if 'package_cache' in targets:
    cmds.extend(['pkg clean', 'apt autoremove -y'])
if 'temp_files' in targets:
    cmds.extend(['rm -rf /tmp/* 2>/dev/null || true', 'rm -rf ~/.cache/* 2>/dev/null || true'])
for cmd in cmds:
    output_lines.append(run_cmd(cmd))
output_lines.append('=== Cleanup Complete ===')
final_output = '\n'.join(output_lines)
print(json.dumps({'success': True, 'output': final_output, 'error': ''}))