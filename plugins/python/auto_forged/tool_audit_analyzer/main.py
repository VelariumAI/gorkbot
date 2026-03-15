import sys
import json
import re

def parse_audit_log(input_data):
    """Parse tool audit log data and extract performance metrics."""
    results = []
    
    # Handle various input formats
    if isinstance(input_data, dict):
        # Single tool entry
        if 'tool' in input_data:
            results.append(parse_tool_entry(input_data))
        # Multiple tools in a list
        elif 'tools' in input_data:
            for tool in input_data['tools']:
                results.append(parse_tool_entry(tool))
        else:
            results.append(parse_tool_entry(input_data))
    elif isinstance(input_data, list):
        for item in input_data:
            results.append(parse_tool_entry(item))
    
    return results

def parse_tool_entry(entry):
    """Parse a single tool entry to extract metrics."""
    tool_name = entry.get('tool', 'unknown')
    calls = entry.get('calls', entry.get('usage', 0))
    success_rate = entry.get('success_rate', entry.get('success', 0))
    total = entry.get('total', 0)
    error = entry.get('error', '')
    
    # Calculate failures from success rate if available
    if isinstance(success_rate, str) and '%' in success_rate:
        success_rate = float(success_rate.replace('%', ''))
    
    failures = 0
    if calls and success_rate:
        failures = int(calls * (100 - success_rate) / 100) if success_rate <= 100 else 0
    
    return {
        'tool': tool_name,
        'calls': calls,
        'success_rate': success_rate,
        'failures': failures,
        'total_metric': total,
        'error': error
    }

def main():
    try:
        # Read JSON from stdin
        input_data = sys.stdin.read().strip()
        
        if not input_data:
            print(json.dumps({
                'success': False,
                'output': '',
                'error': 'No input data provided'
            }))
            return
        
        # Parse input JSON
        try:
            data = json.loads(input_data)
        except json.JSONDecodeError:
            # Try to extract tool info from raw text
            data = {'raw_input': input_data}
        
        # Analyze the audit log
        analysis = parse_audit_log(data)
        
        # Format output
        output = {
            'success': True,
            'output': json.dumps({
                'analyzed_tools': analysis,
                'summary': {
                    'total_tools': len(analysis),
                    'total_calls': sum(a['calls'] for a in analysis),
                    'total_failures': sum(a['failures'] for a in analysis)
                }
            }),
            'error': ''
        }
        
        print(json.dumps(output))
        
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()
