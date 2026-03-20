import sys
import json
from datetime import datetime

def analyze_tool_stats(tool_stats):
    """Analyze tool usage statistics and calculate health metrics."""
    results = []
    total_calls = 0
    total_failures = 0
    
    for tool, stats in tool_stats.items():
        calls = stats.get('calls', 0)
        success_rate = stats.get('success_rate', 100.0)
        avg_time = stats.get('avg_time_ms', 0)
        failures = int(calls * (100 - success_rate) / 100)
        
        total_calls += calls
        total_failures += failures
        
        results.append({
            'tool': tool,
            'calls': calls,
            'success_rate': success_rate,
            'avg_time_ms': avg_time,
            'failures': failures,
            'health': 'healthy' if success_rate >= 90 else 'degraded' if success_rate >= 70 else 'critical'
        })
    
    overall_health = 'healthy'
    if total_calls > 0:
        overall_rate = (total_calls - total_failures) / total_calls * 100
        if overall_rate < 70:
            overall_health = 'critical'
        elif overall_rate < 90:
            overall_health = 'degraded'
    
    return results, overall_health, total_calls, total_failures

def analyze_sanitizer_rejects(rejects):
    """Analyze sanitizer rejection patterns."""
    patterns = {}
    for reject in rejects:
        tool = reject.get('tool', 'unknown')
        reason = reject.get('reason', 'unknown')
        key = f"{tool}:{reason}"
        patterns[key] = patterns.get(key, 0) + 1
    
    return sorted(patterns.items(), key=lambda x: x[1], reverse=True)

def calculate_impact_reduction(patterns, total_rejects):
    """Calculate potential impact reduction from skill-based fixes."""
    skill_applicable = 0
    for pattern, count in patterns:
        if 'bash' in pattern or 'screenshot' in pattern or 'write_file' in pattern:
            skill_applicable += count
    
    if total_rejects > 0:
        return round(skill_applicable / total_rejects * 100, 1)
    return 0.0

def generate_recommendations(health, rejects, patterns):
    """Generate recovery recommendations based on analysis."""
    recommendations = []
    
    if health == 'critical':
        recommendations.append("URGENT: Critical system health - investigate tool failures immediately")
    elif health == 'degraded':
        recommendations.append("WARNING: System degraded - review failing tools")
    
    if rejects:
        recommendations.append(f"Found {len(rejects)} sanitizer rejections - apply path/control char sanitization")
        
        # Pattern-specific recommendations
        for pattern, count in patterns[:3]:
            if 'bash' in pattern:
                recommendations.append(f"High priority: {pattern} ({count} occurrences) - use structured_bash or sanitize control chars")
            elif 'screenshot' in pattern or 'write_file' in pattern:
                recommendations.append(f"High priority: {pattern} ({count} occurrences) - fix path sandboxing issues")
    
    if not recommendations:
        recommendations.append("System healthy - no immediate action required")
    
    return recommendations

def main():
    try:
        # Read JSON input from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Extract relevant data
        tool_stats = input_data.get('tool_stats', {})
        sanitizer_rejects = input_data.get('sanitizer_rejects', [])
        context_usage = input_data.get('context_usage', 0)
        
        # Analyze tool statistics
        tool_results, health, total_calls, total_failures = analyze_tool_stats(tool_stats)
        
        # Analyze sanitizer patterns
        reject_patterns = analyze_sanitizer_rejects(sanitizer_rejects)
        
        # Calculate impact reduction potential
        impact_reduction = calculate_impact_reduction(reject_patterns, len(sanitizer_rejects))
        
        # Generate recommendations
        recommendations = generate_recommendations(health, sanitizer_rejects, reject_patterns)
        
        # Build output
        output = {
            'timestamp': datetime.now().isoformat(),
            'health_status': health,
            'tool_analysis': tool_results,
            'total_calls': total_calls,
            'total_failures': total_failures,
            'sanitizer_rejects': len(sanitizer_rejects),
            'reject_patterns': reject_patterns[:5],
            'impact_reduction_potential': impact_reduction,
            'context_usage_percent': context_usage,
            'recommendations': recommendations
        }
        
        print(json.dumps({
            'success': True,
            'output': json.dumps(output, indent=2),
            'error': ''
        }))
        
    except json.JSONDecodeError as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': f'Invalid JSON input: {str(e)}'
        }))
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': f'Plugin error: {str(e)}'
        }))

if __name__ == '__main__':
    main()
