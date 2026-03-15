import sys
import json
from pathlib import Path
import uuid
from datetime import datetime

data = json.load(sys.stdin)
action = data.get('action', 'queue')
queue_path = Path.home() / '.gorkbot_offline_queue.json'

if queue_path.exists():
    try:
        queue = json.loads(queue_path.read_text(encoding='utf-8'))
    except:
        queue = []
else:
    queue = []

result = {}
if action == 'queue':
    entry = {
        'id': str(uuid.uuid4()),
        'tool': data.get('tool', 'unknown'),
        'params': data.get('params', {}),
        'timestamp': datetime.now().isoformat(),
        'status': 'queued'
    }
    queue.append(entry)
    queue_path.write_text(json.dumps(queue, indent=2), encoding='utf-8')
    result = {'success': True, 'output': f"Queued {entry['tool']} (ID: {entry['id']})", 'error': ''}
elif action == 'list':
    result = {'success': True, 'output': json.dumps(queue, indent=2), 'error': ''}
elif action == 'clear':
    queue_path.write_text('[]', encoding='utf-8')
    result = {'success': True, 'output': 'Queue cleared', 'error': ''}
else:
    result = {'success': False, 'output': '', 'error': 'Unknown action'}

print(json.dumps(result))