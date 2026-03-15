import sys
import json
import subprocess

def main():
    try:
        # Read input JSON from stdin
        input_data = json.load(sys.stdin)
        
        # Extract parameters with defaults
        stream = input_data.get('stream', 'music')  # music, ring, alarm, notification, call, etc.
        level = input_data.get('level', 15)  # 0-15 scale for termux-volume
        
        # Validate level is in acceptable range
        if not isinstance(level, int) or level < 0 or level > 15:
            raise ValueError("Level must be an integer between 0 and 15")
        
        # Build and execute the termux-volume command
        cmd = ['termux-volume', stream, str(level)]
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=10
        )
        
        if result.returncode == 0:
            output_msg = f"Set {stream} stream to level {level}/15 (100%)"
            print(json.dumps({
                "success": True,
                "output": output_msg,
                "error": ""
            }))
        else:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": result.stderr or "Volume command failed"
            }))
            
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Invalid JSON input: {str(e)}"
        }))
    except ValueError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))
    except FileNotFoundError:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": "termux-volume command not found. Ensure Termux API is installed."
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Unexpected error: {str(e)}"
        }))

if __name__ == "__main__":
    main()
