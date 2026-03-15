import sys
import json
import subprocess
import re

def main():
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
        return

    url = data.get("url", "")
    if not url:
        print(json.dumps({"success": False, "output": "", "error": "Missing 'url' parameter"}))
        return

    # curl with redirects (-L), SSL skip (-k), silent (-s)
    cmd = ["curl", "-s", "-L", "-k", url]
    
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        html = result.stdout
        
        if not html:
            print(json.dumps({"success": True, "output": "(empty response)", "error": ""}))
            return
        
        # Strip HTML tags
        text = re.sub(r'<[^>]*>', '', html)
        # Remove blank lines
        lines = [line.strip() for line in text.split('\n') if line.strip()]
        clean = '\n'.join(lines[:100])  # Limit output
        
        print(json.dumps({"success": True, "output": clean, "error": ""}))
    except subprocess.TimeoutExpired:
        print(json.dumps({"success": False, "output": "", "error": "Request timed out"}))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
