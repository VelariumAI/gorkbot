import sys
import json
import os
import subprocess

data = json.load(sys.stdin)
log_file = data.get('log_file', os.path.expanduser('~/.gorkbot/logs/gorkbot.log'))
pid_file = data.get('pid_file', '/tmp/hitl_notifier.pid')
action = data.get('action', 'start')

if action == 'start':
    if os.path.exists(pid_file):
        try:
            with open(pid_file, 'r') as f:
                old_pid = int(f.read().strip())
            if os.path.exists(f'/proc/{old_pid}'):
                print(json.dumps({'success': False, 'output': '', 'error': 'Monitor already running'}))
                sys.exit(0)
        except:
            pass
    script_content = '''#!/bin/bash
LOG_FILE="''' + log_file + '''"
PID_FILE="''' + pid_file + '''"
if [ -f "$PID_FILE" ]; then
  old_pid=$(cat "$PID_FILE")
  if kill -0 "$old_pid" 2>/dev/null; then
    echo "Already running"
    exit 1
  fi
fi
echo $$ > "$PID_FILE"
tail -f "$LOG_FILE" | grep --line-buffered "\[HITL\]" | while read -r line; do
  tool=$(echo "$line" | grep -o 'tool=[^ ]*' | cut -d= -f2 || echo "unknown")
  params=$(echo "$line" | grep -o 'params=.*' | sed 's/params=//' | cut -c1-100 || echo "N/A")
  termux-notification --title "HITL Approval Needed" --content "Tool: $tool | Params: $params | Approve in TUI now." || notification_send --title "HITL Approval Needed" --content "Tool: $tool | Params: $params"
  sleep 1
done'''
    script_path = '/tmp/hitl_monitor.sh'
    with open(script_path, 'w') as f:
        f.write(script_content)
    os.chmod(script_path, 0o755)
    try:
        proc = subprocess.Popen(['nohup', 'bash', script_path], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setpgrp)
        print(json.dumps({'success': True, 'output': f'HITL monitor started (PID {proc.pid})', 'error': ''}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))
elif action == 'stop':
    if os.path.exists(pid_file):
        try:
            with open(pid_file, 'r') as f:
                pid = int(f.read().strip())
            os.kill(pid, 15)
            os.unlink(pid_file)
            print(json.dumps({'success': True, 'output': 'HITL monitor stopped', 'error': ''}))
        except Exception as e:
            print(json.dumps({'success': False, 'output': '', 'error': str(e)}))
    else:
        print(json.dumps({'success': False, 'output': '', 'error': 'No PID file found'}))
else:
    print(json.dumps({'success': False, 'output': '', 'error': 'Unknown action'}))