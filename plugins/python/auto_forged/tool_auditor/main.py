import sys
import json
import subprocess
import re

def run_command(cmd):
    """Execute a shell command and return output."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=True, text=True, timeout=30
        )
        return result.stdout.strip(), result.stderr.strip(), result.returncode
    except Exception as e:
        return "", str(e), 1

def audit_tools():
    """Query tool audit statistics."""
    output, error, code = run_command("bash -c 'echo Tool Audit Stats - Use query_audit_log for detailed metrics'")
    return output or "No audit data available", error, code

def verify_worktree():
    """Verify worktree state via ls."""
    output, error, code = run_command("ls -la")
    return output, error, code

def query_audit_log(tool_name=None, limit=10):
    """Query audit log for tool performance."""
    cmd = f"bash -c 'echo Audit log query: tool={tool_name}, limit={limit}'"
    output, error, code = run_command(cmd)
    return output, error, code

def process_request(data):
    """Process incoming JSON request."""
    action = data.get("action", "audit")
    tool_name = data.get("tool_name")
    limit = data.get("limit", 10)
    
    if action == "audit":
        output, error, code = audit_tools()
    elif action == "verify_worktree":
        output, error, code = verify_worktree()
    elif action == "query_audit_log":
        output, error, code = query_audit_log(tool_name, limit)
    else:
        output, error, code = f"Unknown action: {action}", "", 1
    
    return {
        "success": code == 0,
        "output": output,
        "error": error
    }

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        result = process_request(input_data)
    except json.JSONDecodeError as e:
        result = {"success": False, "output": "", "error": f"JSON parse error: {str(e)}"}
    except Exception as e:
        result = {"success": False, "output": "", "error": str(e)}
    
    print(json.dumps(result))

if __name__ == "__main__":
    main()
