import sys
import json
import subprocess
def run_cmd(cmd):
    try:
        result = subprocess.check_output(cmd, shell=True, text=True, timeout=15).strip()
        return result
    except Exception as e:
        return f"[Error] {str(e)}"
data = json.load(sys.stdin)
action = data.get("action", "full_demo")
output = {}
if action in ["full_demo", "system"]:
    output["ls_home"] = run_cmd("ls -la ~")
    output["disk_usage"] = run_cmd("df -h / | tail -1")
    output["memory"] = run_cmd("free -h | grep Mem")
    output["processes"] = run_cmd("ps aux --no-heading | head -8")
    output["date"] = run_cmd("date")
elif action == "android":
    output["screenshot"] = "Simulated capture to /tmp/demo.png (use adb in real env)"
    output["battery"] = "Simulated battery sensor read: 87%"
    output["file_check"] = run_cmd("ls -l /tmp/demo.png 2>/dev/null || echo 'file not created'")
else:
    output["status"] = "unknown action"
print(json.dumps({"success": True, "output": output, "error": ""}))