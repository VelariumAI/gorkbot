import sys
import json
import os
from datetime import datetime, timedelta
from pathlib import Path

def main():
    try:
        input_data = json.load(sys.stdin)
        target_path = os.path.expanduser(input_data.get('path', '~/.gorkbot/logs'))
        days_old = input_data.get('days_old', 7)
        action = input_data.get('action', 'scan')  # scan, list, cleanup
        dry_run = input_data.get('dry_run', True)
        pattern = input_data.get('pattern', '*.log')
        max_files = input_data.get('max_files', 50)

        path_obj = Path(target_path)
        if not path_obj.exists():
            print(json.dumps({'success': False, 'output': '', 'error': f'Path does not exist: {target_path}'}))
            return

        cutoff = datetime.now() - timedelta(days=days_old)
        matches = []
        total_size = 0

        for f in path_obj.rglob(pattern):
            if f.is_file():
                mtime = datetime.fromtimestamp(f.stat().st_mtime)
                if mtime < cutoff:
                    size = f.stat().st_size
                    matches.append({'path': str(f), 'age_days': (datetime.now() - mtime).days, 'size': size})
                    total_size += size
                    if len(matches) >= max_files:
                        break

        output = {
            'files_found': len(matches),
            'total_size_mb': round(total_size / (1024*1024), 2),
            'cutoff_days': days_old,
            'sample': matches[:10]
        }

        if action in ('cleanup', 'delete') and not dry_run:
            deleted = 0
            for item in matches:
                try:
                    Path(item['path']).unlink()
                    deleted += 1
                except Exception:
                    pass
            output['deleted'] = deleted
            output['message'] = f'Deleted {deleted} files (dry_run was False)'
        else:
            output['message'] = 'Dry run only. Set dry_run=false to execute (HITL recommended for destructive actions)'

        print(json.dumps({'success': True, 'output': output, 'error': ''}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()