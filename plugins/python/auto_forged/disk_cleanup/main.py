import sys
import json
import subprocess
def run_cmd(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        return result.stdout.strip() + ('\n' + result.stderr.strip() if result.stderr.strip() else '')
    except Exception as e:
        return f'Error: {str(e)}'
data = json.loads(sys.stdin.read() or '{}')
params = data.get('parameters', data)
paths = params.get('paths', ['/storage/emulated/0/Download'])
patterns = params.get('patterns', ['*.tmp', '*.old'])
log_dir = params.get('log_dir', '~/.gorkbot/logs')
output_lines = ['=== Disk Cleanup Started ===']
for p in paths:
    for pat in patterns:
        cmd = f"find {p} -maxdepth 2 -type f -name '{pat}' -delete 2>/dev/null || true"
        out = run_cmd(cmd)
        output_lines.append(f'Cleaned {pat} in {p}: {out}')
log_cmd = f"rm -rf {log_dir}/*.old {log_dir}/*.log.* 2>/dev/null || true"
output_lines.append('Log cleanup: ' + run_cmd(log_cmd))
output_lines.append('Package clean: ' + run_cmd('pkg clean 2>/dev/null || apt-get autoclean -y 2>/dev/null || true'))
df_out = run_cmd('df -h /storage/emulated')
output_lines.append('Disk usage:\n' + df_out)
result = {
    "success": True,
    "output": '\n'.join(output_lines),
    "error": ""
}
print(json.dumps(result))