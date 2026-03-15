import sys
import json
import subprocess

def run_command(cmd):
    """Run a bash command and return success status."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=True, text=True, timeout=10
        )
        return result.returncode == 0, result.stdout, result.stderr
    except subprocess.TimeoutExpired:
        return False, "", "timeout"
    except Exception as e:
        return False, "", str(e)

def main():
    data = json.load(sys.stdin)
    
    # Tiered memory reclaim: aggressive -> safe -> minimal
    commands = [
        ("Drop all caches (aggressive)", "sync; echo 3 > /proc/sys/vm/drop_caches"),
        ("Drop pagecache only (safer)", "sync && echo 1 > /proc/sys/vm/drop_caches"),
        ("Sync only (minimal)", "sync")
    ]
    
    results = []
    success = False
    
    for label, cmd in commands:
        ok, stdout, stderr = run_command(cmd)
        results.append(f"{label}: {'OK' if ok else f'FAILED - {stderr}'}")
        if ok:
            success = True
            break
    
    output = " | ".join(results)
    
    print(json.dumps({
        "success": success,
        "output": output,
        "error": "" if success else "All reclaim methods failed"
    }))

if __name__ == "__main__":
    main()
