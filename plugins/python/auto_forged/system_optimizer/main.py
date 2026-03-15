import sys
import json
import subprocess
import os

def run_command(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        return result.stdout.strip(), result.stderr.strip(), result.returncode
    except subprocess.TimeoutExpired:
        return '', 'Command timed out', 1
    except Exception as e:
        return '', str(e), 1

def get_disk_usage(path='~/.cache', top_n=5):
    expanded_path = os.path.expanduser(path)
    cmd = f'du -sh {expanded_path}/* 2>/dev/null | sort -hr | head -n {top_n}'
    output, _, rc = run_command(cmd)
    return output if rc == 0 else 'N/A'

def drop_caches(level=1):
    if os.geteuid() != 0 and not os.path.exists('/proc/sys/vm/drop_caches'):
        return 'Permission denied or not available (need root/privileged access)'
    sync_cmd = 'sync'
    drop_cmd = f'echo {level} > /proc/sys/vm/drop_caches'
    full_cmd = f'{sync_cmd} && {drop_cmd}'
    output, err, rc = run_command(full_cmd)
    if rc != 0:
        return f'Failed: {err}'
    levels = {1: 'pagecache', 2: 'dentries and inodes', 3: 'pagecache, dentries and inodes'}
    return f'Dropped {levels.get(level, 'all caches')}'

def get_memory_info():
    try:
        with open('/proc/meminfo', 'r') as f:
            lines = f.readlines()
        mem_info = {}
        for line in lines:
            if ':' in line:
                key, val = line.split(':', 1)
                mem_info[key.strip()] = val.strip()
        return mem_info
    except Exception as e:
        return {'error': str(e)}

def analyze_memory():
    mem = get_memory_info()
    if 'error' in mem:
        return 'Unable to read memory info'
    try:
        total_kb = int(mem.get('MemTotal', '0').split()[0])
        available_kb = int(mem.get('MemAvailable', '0').split()[0])
        free_kb = int(mem.get('MemFree', '0').split()[0])
        buffers_kb = int(mem.get('Buffers', '0').split()[0])
        cached_kb = int(mem.get('Cached', '0').split()[0])
        
        total_gb = total_kb / 1024 / 1024
        available_gb = available_kb / 1024 / 1024
        used_gb = (total_kb - available_kb) / 1024 / 1024
        usage_pct = ((total_kb - available_kb) / total_kb) * 100
        
        return (f"Total: {total_gb:.1f}GB, Available: {available_gb:.1f}GB, "
                f"Used: {used_gb:.1f}GB ({usage_pct:.1f}%)")
    except Exception as e:
        return f'Error parsing memory: {e}'

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        action = input_data.get('action', 'status')
        path = input_data.get('path', '~/.cache')
        cache_level = input_data.get('cache_level', 1)
        
        output = ''
        success = True
        
        if action == 'drop_caches':
            output = drop_caches(cache_level)
            success = 'Failed' not in output and 'Permission' not in output
        elif action == 'disk_usage':
            output = get_disk_usage(path)
        elif action == 'memory_info':
            output = analyze_memory()
        elif action == 'sync':
            output, err, rc = run_command('sync')
            success = rc == 0
            if err:
                output = f'{output} | Error: {err}'
        elif action == 'status':
            output = f"Memory: {analyze_memory()}\nDisk usage ({path}):\n{get_disk_usage(path)}"
        else:
            output = f'Unknown action: {action}'
            success = False
        
        print(json.dumps({
            'success': success,
            'output': output,
            'error': '' if success else 'Operation failed'
        }))
    except json.JSONDecodeError:
        print(json.dumps({'success': False, 'output': '', 'error': 'Invalid JSON input'}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
