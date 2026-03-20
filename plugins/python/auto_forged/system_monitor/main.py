import sys
import json
import subprocess
from datetime import datetime

def run_bash(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return result.stdout.strip()
        else:
            return f"Error ({result.returncode}): {result.stderr.strip()}"
    except subprocess.TimeoutExpired:
        return "Command timed out"
    except Exception as e:
        return f"Exception: {str(e)}"

def main():
    input_data = json.load(sys.stdin)
    action = input_data.get("action", "monitor")
    paths = input_data.get("paths", ["/tmp/*", "~/.cache/*"])
    
    response = {"success": True, "output": {}, "error": ""}
    
    if action in ["monitor", "full"]:
        mem = run_bash("free -h")
        disk = run_bash("df -h")
        top = run_bash("top -b -n1 | head -20")
        timestamp = run_bash("date --iso-8601=seconds")
        
        response["output"] = {
            "memory": mem,
            "disk": disk,
            "processes": top,
            "timestamp": timestamp or datetime.utcnow().isoformat(),
            "summary": f"Memory: {mem.splitlines()[1] if mem else 'N/A'} | Disk: {disk.splitlines()[-1] if disk else 'N/A'}"
        }
    
    if action in ["cleanup", "clean"]:
        clean_cmd = f"pkg clean && rm -rf {' '.join(paths)}"
        cleanup_out = run_bash(clean_cmd)
        response["output"]["cleanup"] = cleanup_out
        response["output"]["cleaned_paths"] = paths
    
    if not response["output"]:
        response["success"] = False
        response["error"] = "No valid action specified"
    
    print(json.dumps(response, indent=2))

if __name__ == "__main__":
    main()