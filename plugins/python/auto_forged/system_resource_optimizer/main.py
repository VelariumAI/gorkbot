import sys
import json

data = json.loads(sys.stdin.read())
action = data.get('action', 'monitor').lower()
result = ''
if action == 'monitor' or action == 'system_monitor':
    result = 'Memory: 33% available (alert <50%), Storage: 20% available (<50%). Context 27% used. Everything nominal—no anomalies. RAM trend up from 35%. SPARK uses <100M.'
elif action == 'clear_cache' or action == 'clear_caches':
    result = 'Caches cleared successfully. Command: pkg clean && sync; echo 3 > /proc/sys/vm/drop_caches. Boosted available RAM by 200-500 MiB.'
elif action in ['identify_culprits', 'identify_large_dirs', 'du']:
    result = 'Largest directories identified (sorted): ~/project (est. largest), logs, /data/data/com.termux/files/*. Top 10 output simulated from: du -sh ~/* /data/data/com.termux/files/* 2>/dev/null | sort -hr | head -10'
elif 'bash' in action or 'command' in data:
    cmd = data.get('command', '')
    if 'drop_caches' in cmd:
        result = 'Cache drop executed. Available memory increased.'
    elif 'du -sh' in cmd:
        result = 'Storage breakdown: largest dirs listed in tabular format.'
    else:
        result = 'Structured bash command completed.'
else:
    result = 'System resource operation completed. Use actions: monitor, clear_cache, identify_culprits.'
print(json.dumps({'success': True, 'output': result, 'error': ''}))