import sys
import json
import os
import subprocess
import time
from datetime import datetime

def run_command(cmd, check=True):
    """Run a shell command and return output."""
    result = subprocess.run(
        cmd,
        shell=True,
        capture_output=True,
        text=True
    )
    if check and result.returncode != 0:
        raise Exception(result.stderr or f"Command failed: {cmd}")
    return result

def main():
    try:
        # Read input JSON from stdin
        input_data = json.load(sys.stdin)
        
        # Get filename or generate timestamp-based name
        filename = input_data.get('filename', f"screenshot_{datetime.now().strftime('%Y%m%d_%H%M%S')}.png")
        
        # Ensure .png extension
        if not filename.endswith('.png'):
            filename += '.png'
        
        output_path = input_data.get('output_path', f"~/storage/shared/Download/{filename}")
        temp_path = input_data.get('temp_path', f"/data/data/com.termux/files/home/{filename}")
        
        # Step 1: Setup storage access (one-time, idempotent)
        try:
            run_command("termux-setup-storage", check=False)
            time.sleep(1)  # Give user time to accept permission prompt
        except Exception as e:
            # Continue - may already be set up or will be handled by screenshot
            pass
        
        # Step 2: Take screenshot to temp location (-f is the correct termux-screenshot flag)
        result = run_command(f"termux-screenshot -f {temp_path}", check=False)

        # If screenshot failed, try alternative method
        if result.returncode != 0 or not os.path.exists(os.path.expanduser(temp_path.replace('~', '/data/data/com.termux/files/home'))):
            # Try with different path
            alt_temp = f"/sdcard/Download/{filename}"
            run_command(f"termux-screenshot -f {alt_temp}", check=False)
            time.sleep(1)
            temp_path = alt_temp
        
        # Step 3: Move to final output path if different
        final_path = os.path.expanduser(output_path.replace('~', '/data/data/com.termux/files/home'))
        temp_expanded = os.path.expanduser(temp_path.replace('~', '/data/data/com.termux/files/home'))
        
        if temp_expanded != final_path:
            # Ensure directory exists
            os.makedirs(os.path.dirname(final_path), exist_ok=True)
            # Move file
            run_command(f"mv {temp_expanded} {final_path}", check=False)
        
        # Verify file exists
        if os.path.exists(final_path):
            file_size = os.path.getsize(final_path)
            print(json.dumps({
                "success": True,
                "output": f"Screenshot saved to {final_path} ({file_size} bytes)",
                "error": ""
            }))
        else:
            # Check if it's in the original temp location
            if os.path.exists(temp_expanded):
                print(json.dumps({
                    "success": True,
                    "output": f"Screenshot saved to {temp_expanded}",
                    "error": ""
                }))
            else:
                print(json.dumps({
                    "success": False,
                    "output": "",
                    "error": "Screenshot file not found after capture"
                }))
                
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
