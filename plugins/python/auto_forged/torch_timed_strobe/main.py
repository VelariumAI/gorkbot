import sys
import json
import subprocess
import time

def main():
    try:
        data = json.load(sys.stdin)
        
        off_duration = data.get('off_duration', 0)
        strobe_duration = data.get('strobe_duration', 10)
        strobe_interval = data.get('strobe_interval', 0.5)
        
        # Turn off torch immediately
        subprocess.run(['termux-torch', 'off'], check=True, capture_output=True)
        
        # Wait for the off duration
        if off_duration > 0:
            time.sleep(off_duration)
        
        # Perform strobe pattern
        strobe_cycles = 0
        if strobe_duration > 0:
            cycles = int(strobe_duration / (strobe_interval * 2))
            for _ in range(cycles):
                subprocess.run(['termux-torch', 'on'], check=True, capture_output=True)
                time.sleep(strobe_interval)
                subprocess.run(['termux-torch', 'off'], check=True, capture_output=True)
                time.sleep(strobe_interval)
                strobe_cycles += 1
        
        output_msg = f"Torch sequence completed: {off_duration}s off, {strobe_duration}s strobe ({strobe_cycles} cycles at {strobe_interval}s interval)"
        
        print(json.dumps({
            "success": True,
            "output": output_msg,
            "error": ""
        }))
        
    except subprocess.CalledProcessError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"termux-torch command failed: {str(e)}"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
