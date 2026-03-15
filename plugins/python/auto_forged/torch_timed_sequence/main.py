import sys
import json
import subprocess
import time

def run_torch(command):
    """Execute termux-torch command."""
    result = subprocess.run(
        ['termux-torch', command],
        capture_output=True,
        text=True
    )
    return result.returncode, result.stdout, result.stderr

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Invalid JSON input: {str(e)}"
        }))
        return

    # Extract parameters with defaults
    off_duration = input_data.get("off_duration", 0)  # seconds to stay off initially
    strobe_duration = input_data.get("strobe_duration", 10)  # total strobe time
    on_time = input_data.get("on_time", 0.5)  # seconds light stays on per cycle
    off_time = input_data.get("off_time", 0.5)  # seconds light stays off per cycle
    
    output_parts = []
    
    # Step 1: Turn off torch initially
    if off_duration > 0:
        code, stdout, stderr = run_torch("off")
        if code != 0:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": f"Failed to turn off torch: {stderr}"
            }))
            return
        output_parts.append(f"Turned off torch")
        time.sleep(off_duration)
    
    # Step 2: Strobe sequence
    cycle_time = on_time + off_time
    num_cycles = int(strobe_duration / cycle_time)
    
    for i in range(num_cycles):
        # Turn on
        code, stdout, stderr = run_torch("on")
        if code != 0:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": f"Strobe on failed at cycle {i+1}: {stderr}"
            }))
            return
        time.sleep(on_time)
        
        # Turn off
        code, stdout, stderr = run_torch("off")
        if code != 0:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": f"Strobe off failed at cycle {i+1}: {stderr}"
            }))
            return
        time.sleep(off_time)
    
    # Ensure torch ends in off state
    run_torch("off")
    
    output_msg = f"Torch sequence completed: {off_duration}s off, {strobe_duration}s strobe ({num_cycles} cycles of {on_time}s on/{off_time}s off)"
    output_parts.append(output_msg)
    
    print(json.dumps({
        "success": True,
        "output": "; ".join(output_parts),
        "error": ""
    }))

if __name__ == "__main__":
    main()
