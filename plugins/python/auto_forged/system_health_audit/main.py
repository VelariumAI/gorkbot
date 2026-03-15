import sys
import json
import os
import subprocess

def get_disk_usage(path='/'):
    """Get disk usage for a given path."""
    try:
        stat = os.statvfs(path)
        total = stat.f_blocks * stat.f_frsize
        free = stat.f_bfree * stat.f_frsize
        used = total - free
        percent = (used / total) * 100 if total > 0 else 0
        return {
            'total': total,
            'used': used,
            'free': free,
            'percent': round(percent, 1)
        }
    except Exception as e:
        return {'error': str(e)}

def get_memory_info():
    """Get system memory information."""
    try:
        with open('/proc/meminfo', 'r') as f:
            lines = f.readlines()
        
        mem_info = {}
        for line in lines:
            if ':' in line:
                key, val = line.split(':', 1)
                mem_info[key.strip()] = val.strip()
        
        total_kb = int(mem_info.get('MemTotal', '0').split()[0])
        available_kb = int(mem_info.get('MemAvailable', '0').split()[0]) if 'MemAvailable' in mem_info else int(mem_info.get('MemFree', '0').split()[0])
        used_kb = total_kb - available_kb
        
        # Check for swap
        swap_total_kb = int(mem_info.get('SwapTotal', '0').split()[0])
        swap_free_kb = int(mem_info.get('SwapFree', '0').split()[0])
        swap_used_kb = swap_total_kb - swap_free_kb
        swap_percent = (swap_used_kb / swap_total_kb * 100) if swap_total_kb > 0 else 0
        
        return {
            'ram': {
                'total_kb': total_kb,
                'used_kb': used_kb,
                'available_kb': available_kb,
                'percent': round((used_kb / total_kb) * 100, 1) if total_kb > 0 else 0
            },
            'swap': {
                'total_kb': swap_total_kb,
                'used_kb': swap_used_kb,
                'free_kb': swap_free_kb,
                'percent': round(swap_percent, 1)
            }
        }
    except Exception as e:
        return {'error': str(e)}

def get_home_usage():
    """Get disk usage for home directory."""
    home = os.path.expanduser('~')
    try:
        result = subprocess.run(
            ['du', '-sh', home],
            capture_output=True,
            text=True,
            timeout=30
        )
        if result.returncode == 0:
            size = result.stdout.split()[0]
            return {'home_size': size, 'home_path': home}
    except Exception:
        pass
    return {'home_path': home}

def analyze_health(disk_info, mem_info, home_info):
    """Analyze system health and generate recommendations."""
    issues = []
    health_status = 'healthy'
    
    # Check disk usage
    if 'error' not in disk_info:
        if disk_info['percent'] >= 100:
            issues.append({
                'severity': 'CRITICAL',
                'subsystem': 'disk',
                'issue': f"Root filesystem at {disk_info['percent']}% capacity",
                'impact': 'Long-context AI operations will fail - temp file creation not possible',
                'recommendation': 'Free root filesystem space immediately or use /storage/emulated for temp operations'
            })
            health_status = 'critical'
        elif disk_info['percent'] >= 90:
            issues.append({
                'severity': 'HIGH',
                'subsystem': 'disk',
                'issue': f"Root filesystem at {disk_info['percent']}% capacity",
                'recommendation': 'Clean up temp files and caches'
            })
            health_status = 'degraded'
    
    # Check swap usage
    if 'error' not in mem_info and 'swap' in mem_info:
        swap = mem_info['swap']
        if swap['total_kb'] > 0:
            if swap['percent'] >= 80:
                issues.append({
                    'severity': 'HIGH',
                    'subsystem': 'swap',
                    'issue': f"Swap at {swap['percent']}% utilization",
                    'impact': 'Memory pressure causing I/O degradation',
                    'recommendation': 'Terminate idle processes and clear caches'
                })
                if health_status != 'critical':
                    health_status = 'degraded'
    
    # Check memory pressure
    if 'error' not in mem_info and 'ram' in mem_info:
        ram = mem_info['ram']
        if ram['percent'] >= 90:
            issues.append({
                'severity': 'MEDIUM',
                'subsystem': 'memory',
                'issue': f"RAM at {ram['percent']}% utilization",
                'recommendation': 'Consider reducing concurrent operations'
            })
    
    return {
        'status': health_status,
        'issues': issues
    }

def run_audit():
    """Run comprehensive system health audit."""
    try:
        # Gather system info
        disk_info = get_disk_usage('/')
        mem_info = get_memory_info()
        home_info = get_home_usage()
        
        # Analyze health
        health = analyze_health(disk_info, mem_info, home_info)
        
        # Build report
        report = {
            'timestamp': subprocess.run(['date', '+%Y-%m-%dT%H:%M:%SZ'], capture_output=True, text=True).stdout.strip(),
            'disk': disk_info,
            'memory': mem_info,
            'home_directory': home_info,
            'health_analysis': health
        }
        
        return {
            'success': True,
            'output': json.dumps(report, indent=2),
            'error': ''
        }
    except Exception as e:
        return {
            'success': False,
            'output': '',
            'error': str(e)
        }

if __name__ == '__main__':
    # Read input from stdin (for Gorkbot plugin compatibility)
    input_data = sys.stdin.read().strip()
    
    # Run the audit
    result = run_audit()
    
    # Output JSON result
    print(json.dumps(result))
