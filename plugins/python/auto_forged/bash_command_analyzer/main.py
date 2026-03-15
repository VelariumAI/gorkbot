import sys
import json

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        
        commands = input_data.get('commands', [])
        if not commands:
            # Default analytics from provided context
            tools = [
                {'name': 'read_file', 'calls': 72, 'success_rate': 100},
                {'name': 'bash', 'calls': 68, 'success_rate': 91},
                {'name': 'grep_content', 'calls': 42, 'success_rate': 86}
            ]
            analysis = {
                'total_calls': sum(t['calls'] for t in tools),
                'tools': tools,
                'avg_success_rate': round(sum(t['calls'] * t['success_rate'] for t in tools) / sum(t['calls'] for t in tools), 2),
                'high_performers': [t['name'] for t in tools if t['success_rate'] >= 90],
                'needs_attention': [t['name'] for t in tools if t['success_rate'] < 90]
            }
        else:
            # Process custom command data
            analysis = {
                'command_count': len(commands),
                'commands': commands,
                'analyzed': True
            }
        
        output = json.dumps({
            'success': True,
            'output': analysis,
            'error': ''
        })
        print(output)
        
    except json.JSONDecodeError as e:
        error_output = json.dumps({
            'success': False,
            'output': {},
            'error': f'JSON decode error: {str(e)}'
        })
        print(error_output)
    except Exception as e:
        error_output = json.dumps({
            'success': False,
            'output': {},
            'error': str(e)
        })
        print(error_output)

if __name__ == '__main__':
    main()
