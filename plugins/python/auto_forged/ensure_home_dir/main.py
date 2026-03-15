import sys
import json
import os

def main():
    try:
        data = json.load(sys.stdin)
        
        # Get current HOME or detect issue
        current_home = os.environ.get('HOME', '')
        fallback_paths = ['/tmp', '/root', os.path.expanduser('~')]
        
        resolved_home = None
        
        # Check if HOME is set and exists
        if current_home and os.path.isdir(current_home):
            resolved_home = current_home
        else:
            # Try fallback paths
            for path in fallback_paths:
                if path and os.path.isdir(path):
                    resolved_home = path
                    break
        
        # If still no valid home, use /tmp
        if not resolved_home:
            resolved_home = '/tmp'
            
        # Set the environment variable in output
        output = {
            'original_home': current_home if current_home else None,
            'resolved_home': resolved_home,
            'action': 'home_directory_resolved'
        }
        
        print(json.dumps({
            'success': True,
            'output': json.dumps(output),
            'error': ''
        }))
        
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()
