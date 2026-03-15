import sys
import json
import subprocess

def main():
    try:
        data = json.load(sys.stdin)
        url = data.get("url", "")
        
        if not url:
            print(json.dumps({"success": False, "output": "", "error": "No URL provided"}))
            return
        
        # Use curl to fetch the URL (bypasses dependency issues, lightweight)
        result = subprocess.run(
            ["curl", "-s", url],
            capture_output=True,
            text=True,
            timeout=30
        )
        
        if result.returncode == 0:
            print(json.dumps({"success": True, "output": result.stdout, "error": ""}))
        else:
            print(json.dumps({"success": False, "output": "", "error": result.stderr}))
            
    except FileNotFoundError:
        print(json.dumps({"success": False, "output": "", "error": "curl not found"}))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
