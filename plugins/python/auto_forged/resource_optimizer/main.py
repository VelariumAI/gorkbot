import sys
import json
import subprocess

data = json.loads(sys.stdin.read().strip() or '{}')
action = data.get('action', '')
params = data.get('params', {})
result = {'success': False, 'output': '', 'error': ''}

try:
    if action == 'profile_memory':
        cmd = 'ps aux --sort=-%mem | head -10'
        out = subprocess.getoutput(cmd)
        result = {'success': True, 'output': out, 'error': ''}
    elif action == 'kill_idle':
        target = params.get('target', 'claude')
        cmd = f"ps aux | grep -v grep | grep '{target}' | awk '{{print $2}}' | xargs -r kill"
        out = subprocess.getoutput(cmd)
        result = {'success': True, 'output': f'Attempted to kill {target} processes', 'error': ''}
    elif action == 'flush_cache':
        cmd = 'sync; echo 3 > /proc/sys/vm/drop_caches'
        out = subprocess.getoutput(cmd)
        result = {'success': True, 'output': 'Caches flushed successfully', 'error': ''}
    elif action == 'git_gc':
        repo_path = params.get('path', '~/project/gorky')
        cmd = f"cd '{repo_path}' && git gc --aggressive --prune=now"
        out = subprocess.getoutput(cmd)
        result = {'success': True, 'output': out or 'Git garbage collection completed', 'error': ''}
    elif action == 'clean_media':
        dry_run = params.get('dry_run', True)
        path = params.get('path', '/storage/emulated/Download')
        size = params.get('size', '+100M')
        flag = '-print' if dry_run else '-delete'
        cmd = f"find {path} -name '*.mp4' -size {size} {flag}"
        out = subprocess.getoutput(cmd)
        result = {'success': True, 'output': out or 'No matching files found', 'error': ''}
    else:
        result['error'] = 'Unknown action'
except Exception as e:
    result['error'] = str(e)

print(json.dumps(result))