import sys
import json
import subprocess

def main():
    try:
        input_data = sys.stdin.read().strip()
        params = json.loads(input_data) if input_data else {}
        avail_threshold = params.get('available_threshold', 50)
        swap_threshold = params.get('swap_threshold', 90)
        result = subprocess.run(['free', '-m'], capture_output=True, text=True, check=True)
        lines = result.stdout.strip().split('\n')
        if len(lines) < 3:
            raise ValueError('Unexpected free output format')
        mem_parts = lines[1].split()
        swap_parts = lines[2].split()
        total = int(mem_parts[1])
        avail = int(mem_parts[6])
        used = int(mem_parts[2])
        avail_pct = round((avail / total) * 100)
        used_pct = round((used / total) * 100)
        swap_used_pct = 0
        if len(swap_parts) > 3 and int(swap_parts[1]) > 0:
            swap_used_pct = round((int(swap_parts[2]) / int(swap_parts[1])) * 100)
        status = f"RAM: {used_pct}% used ({used/1024:.1f}G/{total/1024:.1f}G), {avail/1024:.1f}G available ({avail_pct}%)"
        status += f". Swap: {swap_used_pct}%"
        notification = ""
        if avail_pct < avail_threshold:
            notification = f"Low available RAM ({avail_pct}% < {avail_threshold}% threshold)"
        if swap_used_pct > swap_threshold:
            if notification:
                notification += "; "
            notification += f"High swap usage ({swap_used_pct}% > {swap_threshold}% threshold)"
        if notification:
            status += f". NOTIFICATION: {notification}"
        print(json.dumps({"success": True, "output": status, "error": ""}))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
