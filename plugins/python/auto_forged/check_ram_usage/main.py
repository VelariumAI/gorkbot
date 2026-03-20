import sys
import json
from pathlib import Path

def get_memory_info():
    meminfo = {}
    try:
        with open('/proc/meminfo') as f:
            for line in f:
                if ':' in line:
                    key, val = line.split(':', 1)
                    meminfo[key.strip()] = int(val.strip().split()[0])
        
        total_kb = meminfo.get('MemTotal', 0)
        avail_kb = meminfo.get('MemAvailable', 0)
        swap_total_kb = meminfo.get('SwapTotal', 0)
        swap_free_kb = meminfo.get('SwapFree', 0)
        
        total_gb = round(total_kb / 1024 / 1024, 1)
        avail_gb = round(avail_kb / 1024 / 1024, 1)
        used_gb = round(total_gb - avail_gb, 1)
        used_pct = round((total_gb - avail_gb) / total_gb * 100, 1) if total_gb > 0 else 0
        
        swap_used_kb = swap_total_kb - swap_free_kb
        swap_used_pct = round(swap_used_kb / swap_total_kb * 100, 1) if swap_total_kb > 0 else 0
        
        notify = (avail_gb / total_gb < 0.5 if total_gb > 0 else False) or (swap_used_pct > 95)
        
        status = "critical" if notify else "normal"
        
        return {
            "total_gb": total_gb,
            "used_gb": used_gb,
            "available_gb": avail_gb,
            "used_percent": used_pct,
            "swap_used_percent": swap_used_pct,
            "notify": notify,
            "status": status
        }
    except Exception as e:
        raise RuntimeError(f"Failed to read memory info: {str(e)}")

def main():
    try:
        input_data = json.load(sys.stdin) if not sys.stdin.isatty() else {}
        threshold_ram = input_data.get("threshold_ram_available_pct", 50)
        threshold_swap = input_data.get("threshold_swap_used_pct", 95)
        
        info = get_memory_info()
        
        output_str = f"RAM: {info['used_percent']}% used ({info['used_gb']} GiB / {info['total_gb']} GiB), {info['available_gb']} GiB available. "
        output_str += f"Swap used: {info['swap_used_percent']}%"
        
        if info['notify']:
            output_str += f". NOTIFICATION ISSUED (RAM <{threshold_ram}% available or swap >{threshold_swap}%)"
        
        result = {
            "success": True,
            "output": output_str,
            "error": "",
            "data": info
        }
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"success": False, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()