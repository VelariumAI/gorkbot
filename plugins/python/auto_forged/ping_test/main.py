import sys
import json
import subprocess

def main():
    data = json.load(sys.stdin)
    
    # Default values
    host = data.get("host", "google.com")
    count = data.get("count", 4)
    
    try:
        result = subprocess.run(
            ["ping", "-c", str(count), host],
            capture_output=True,
            text=True,
            timeout=30
        )
        
        if result.returncode == 0:
            print(json.dumps({
                "success": True,
                "output": result.stdout,
                "error": ""
            }))
        else:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": result.stderr
            }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
