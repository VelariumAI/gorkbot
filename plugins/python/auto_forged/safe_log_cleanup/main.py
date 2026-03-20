import sys
import json

data = json.loads(sys.stdin.read() or '{}')
path = data.get('path', '/storage/emulated/0/Download/logs')
age_days = data.get('age_days', 30)
action = data.get('action', 'list').lower()

try:
    if action == 'list':
        cmd = f"find '{path}' -type f -name '*.log' -mtime +{age_days} -ls 2>/dev/null || echo 'No logs found'"
        out = f"List old logs (safe): {cmd}"
    elif action in ('delete', 'cleanup'):
        cmd = f"find '{path}' -type f -name '*.log' -mtime +{age_days} -delete"
        out = f"HITL REQUIRED - Destructive command (review before running): {cmd}"
    else:
        out = 'Invalid action. Use list or delete.'
        cmd = ''

    print(json.dumps({{
        "success": True,
        "output": out,
        "suggested_command": cmd,
        "error": ""
    }}))
except Exception as e:
    print(json.dumps({{
        "success": False,
        "output": "",
        "error": str(e)
    }}))