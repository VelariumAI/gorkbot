import sys
import json
import os
import subprocess

data = json.load(sys.stdin)
log_file = data.get('log_file', os.path.expanduser('~/.gorkbot/logs/gorkbot.log'))
pid_file = data.get('pid_file', '/tmp/hitl_notifier.pid')
script_path = data.get('script_path', '/data/data/com.termux/files/home/project/gorky/hitl_monitor.sh')

bash_script = '''#!/bin/bash
# HITL Notifier: Real-time log tail + notify
LOG_FILE="''' + log_file + '''"
PID_FILE="''' + pid_file + '''"

if [ -f "$PID_FILE" ]; then
  old_pid=$(cat "$PID_FILE")
  if kill -0 "$old_pid" 2>/dev/null; then
    echo "HITL notifier already running (PID: $old_pid). Exiting."
    exit 1
  fi
fi
echo $$ > "$PID_FILE"

tail -f "$LOG_FILE" | grep --line-buffered "\[HITL\]" | while read -r line; do
  tool=$(echo "$line" | grep -o 'tool=[^ ]*' | cut -d= -f2 || echo "unknown")
  params=$(echo "$line" | grep -o 'params=.*' | sed 's/params=//' | cut -c1-100 || echo "N/A")
  termux-notification --title "HITL Approval Needed" --content "Tool: $tool | Params: $params | Approve in TUI now." || notification_send --title "HITL Approval Needed" --content "Tool: $tool | Params: $params";
  sleep 1
done'''

try:
    os.makedirs(os.path.dirname(script_path), exist_ok=True)
    with open(script_path, 'w') as f:
        f.write(bash_script)
    os.chmod(script_path, 0o755)
    
    subprocess.Popen(['nohup', 'bash', script_path], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setpgrp)
    result = {"success": true, "output": f"HITL monitor launched at {script_path}", "error": ""}
except Exception as e:
    result = {"success": false, "output": "", "error": str(e)}

print(json.dumps(result))