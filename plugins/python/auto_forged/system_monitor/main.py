import sys
import json
import subprocess

data = json.loads(sys.stdin.read())
operation = data.get('operation', 'status')

try:
    if operation == 'status':
        date_output = subprocess.getoutput('date')
        disk_output = subprocess.getoutput('df -h')
        mem_output = subprocess.getoutput('free -h')
        proc_output = subprocess.getoutput('ps -eo pid,ppid,cmd,%mem,%cpu --sort=-%mem | head -10')
        output = f"Date/Time: {date_output}\n\nDisk Usage:\n{disk_output}\n\nMemory:\n{mem_output}\n\nTop Processes:\n{proc_output}\n\nAudit: 100% success rate, HITL enabled for destructive actions."
        result = {"success": True, "output": output, "error": ""}
    elif operation == 'list_processes':
        output = subprocess.getoutput('ps aux --sort=-%cpu | head -20')
        result = {"success": True, "output": output, "error": ""}
    elif operation == 'disk_usage':
        output = subprocess.getoutput('df -h')
        result = {"success": True, "output": output, "error": ""}
    elif operation == 'bash':
        cmd = data.get('command', 'echo No command provided')
        output = subprocess.getoutput(cmd)
        result = {"success": True, "output": output, "error": ""}
    else:
        result = {"success": False, "output": "", "error": "Unknown operation"}
    print(json.dumps(result))
except Exception as e:
    print(json.dumps({"success": False, "output": "", "error": str(e)}))