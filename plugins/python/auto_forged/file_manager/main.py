import sys
import json
import os
import subprocess
import re

def main():
    try:
        data = json.load(sys.stdin)
        action = data.get('action', '')
        
        if action == 'delete_files':
            paths = data.get('paths', [])
            backup = data.get('backup', False)
            deleted = []
            for path in paths:
                if backup:
                    # Move to .backup instead of direct delete
                    backup_dir = os.path.join(os.path.dirname(path), '.backup')
                    os.makedirs(backup_dir, exist_ok=True)
                    basename = os.path.basename(path)
                    dest = os.path.join(backup_dir, basename + '_' + str(os.path.getmtime(path)))
                    os.rename(path, dest)
                    deleted.append(f"Backed up: {path} -> {dest}")
                else:
                    subprocess.run(['rm', '-rf', path], check=True)
                    deleted.append(f"Deleted: {path}")
            print(json.dumps({'success': True, 'output': '\n'.join(deleted), 'error': ''}))
        
        elif action == 'append_bashrc':
            exports = data.get('exports', {})
            added = []
            for key, value in exports.items():
                line = f"export {key}={value}"
                with open(os.path.expanduser('~/.bashrc'), 'a') as f:
                    f.write('\n' + line)
                added.append(line)
            print(json.dumps({'success': True, 'output': 'Added to ~/.bashrc:\n' + '\n'.join(added), 'error': ''}))
        
        elif action == 'source_bashrc':
            result = subprocess.run(['bash', '-c', 'source ~/.bashrc'], capture_output=True, text=True)
            print(json.dumps({'success': result.returncode == 0, 'output': result.stdout, 'error': result.stderr}))
        
        elif action == 'clear_cache':
            package = data.get('package', '')
            if package:
                result = subprocess.run(['pm', 'clear', package], capture_output=True, text=True)
                print(json.dumps({'success': result.returncode == 0, 'output': f"Cleared cache for {package}", 'error': result.stderr}))
            else:
                print(json.dumps({'success': False, 'output': '', 'error': 'No package specified'}))
        
        elif action == 'restart_service':
            service_name = data.get('service_name', '')
            start_cmd = data.get('start_command', '')
            subprocess.run(['pkill', service_name], capture_output=True)
            if start_cmd:
                subprocess.Popen(start_cmd, shell=True)
            print(json.dumps({'success': True, 'output': f"Restarted {service_name}", 'error': ''}))
        
        else:
            print(json.dumps({'success': False, 'output': '', 'error': f'Unknown action: {action}'}))
    
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
