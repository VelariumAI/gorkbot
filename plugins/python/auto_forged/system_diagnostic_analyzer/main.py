import sys
import json
import subprocess
import re
from pathlib import Path

def run_command(cmd):
    """Execute a bash command and return output."""
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
        return result.stdout + result.stderr
    except Exception as e:
        return f"Error: {str(e)}"

def get_disk_usage():
    """Get disk usage for all mounted filesystems."""
    output = run_command('df -h')
    return output

def get_memory_info():
    """Get memory and swap usage."""
    output = run_command('free -h')
    return output

def get_top_consumers():
    """Find top disk space consumers in home directory."""
    home = str(Path.home())
    output = run_command(f'du -sh {home}/* 2>/dev/null | sort -hr | head -20')
    return output

def analyze_disk_health(disk_output):
    """Analyze disk usage and identify critical filesystems."""
    critical = []
    warning = []
    lines = disk_output.strip().split('\n')
    
    for line in lines[1:]:  # Skip header
        parts = line.split()
        if len(parts) >= 5:
            mount = parts[5] if len(parts) > 5 else parts[4]
            use_pct = parts[4].replace('%', '')
            try:
                use_val = int(use_pct)
                if use_val >= 100:
                    critical.append({'mount': mount, 'usage': use_val})
                elif use_val >= 90:
                    warning.append({'mount': mount, 'usage': use_val})
            except ValueError:
                pass
    
    return {'critical': critical, 'warning': warning}

def analyze_memory_health(mem_output):
    """Analyze memory and swap usage."""
    swap_info = {'total': 0, 'used': 0, 'usage_pct': 0}
    lines = mem_output.strip().split('\n')
    
    for line in lines:
        if 'Swap' in line:
            parts = line.split()
            if len(parts) >= 3:
                try:
                    swap_info['total'] = int(parts[1])
                    swap_info['used'] = int(parts[2])
                    if swap_info['total'] > 0:
                        swap_info['usage_pct'] = round((swap_info['used'] / swap_info['total']) * 100, 1)
                except (ValueError, IndexError):
                    pass
    
    return swap_info

def generate_recommendations(disk_health, mem_health):
    """Generate actionable recommendations based on health analysis."""
    recommendations = []
    
    if disk_health['critical']:
        recommendations.append({
            'priority': 'P0-CRITICAL',
            'action': 'Free root filesystem space immediately',
            'commands': [
                'du -sh /*',
                'rm -rf ~/Downloads/*',
                'rm -rf ~/.cache/*',
                'rm -rf ~/pip/cache'
            ]
        })
    
    if mem_health.get('usage_pct', 0) > 80:
        recommendations.append({
            'priority': 'P0-CRITICAL',
            'action': 'Reduce swap pressure - clear caches and terminate idle processes',
            'commands': [
                'sync && echo 3 > /proc/sys/vm/drop_caches',
                'ps aux --sort=-%mem | head -10'
            ]
        })
    
    if disk_health['warning']:
        recommendations.append({
            'priority': 'P1-HIGH',
            'action': 'Monitor filesystem usage and plan cleanup',
            'commands': []
        })
    
    return recommendations

def perform_root_cause_analysis(disk_health, mem_health, top_consumers):
    """Perform root cause analysis for system failures."""
    causes = []
    
    if disk_health['critical']:
        causes.append({
            'issue': 'Filesystem capacity exhaustion',
            'impact': 'Temp file creation fails, long-context operations timeout',
            'confidence': 0.95
        })
    
    if mem_health.get('usage_pct', 0) > 80:
        causes.append({
            'issue': 'Swap thrashing',
            'impact': 'Severe I/O degradation, process timeouts',
            'confidence': 0.90
        })
    
    return causes

def run_full_diagnostic():
    """Run complete system diagnostic and return structured results."""
    results = {
        'disk_usage': get_disk_usage(),
        'memory_info': get_memory_info(),
        'top_consumers': get_top_consumers()
    }
    
    disk_health = analyze_disk_health(results['disk_usage'])
    mem_health = analyze_memory_health(results['memory_info'])
    
    analysis = {
        'disk_health': disk_health,
        'memory_health': mem_health,
        'recommendations': generate_recommendations(disk_health, mem_health),
        'root_causes': perform_root_cause_analysis(disk_health, mem_health, results['top_consumers'])
    }
    
    return {
        'diagnostic_data': results,
        'analysis': analysis
    }

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        
        action = input_data.get('action', 'diagnostic')
        
        if action == 'diagnostic':
            result = run_full_diagnostic()
            output = json.dumps(result, indent=2)
            print(json.dumps({'success': True, 'output': output, 'error': ''}))
        
        elif action == 'disk_check':
            disk_output = get_disk_usage()
            health = analyze_disk_health(disk_output)
            output = json.dumps({'disk_output': disk_output, 'health': health}, indent=2)
            print(json.dumps({'success': True, 'output': output, 'error': ''}))
        
        elif action == 'memory_check':
            mem_output = get_memory_info()
            health = analyze_memory_health(mem_output)
            output = json.dumps({'memory_output': mem_output, 'health': health}, indent=2)
            print(json.dumps({'success': True, 'output': output, 'error': ''}))
        
        elif action == 'top_consumers':
            consumers = get_top_consumers()
            print(json.dumps({'success': True, 'output': consumers, 'error': ''}))
        
        else:
            print(json.dumps({'success': False, 'output': '', 'error': f'Unknown action: {action}'}))
            
    except json.JSONDecodeError as e:
        print(json.dumps({'success': False, 'output': '', 'error': f'JSON decode error: {str(e)}'}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
