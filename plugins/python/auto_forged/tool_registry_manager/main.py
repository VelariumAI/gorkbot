import sys
import json
import os
import sqlite3
from pathlib import Path

def main():
    try:
        data = json.loads(sys.stdin.read())
        action = data.get('action', '')
        
        if action == 'init_audit':
            # Initialize the audit database
            audit_db = os.path.expanduser('~/.gorkbot/audit.db')
            os.makedirs(os.path.dirname(audit_db), exist_ok=True)
            
            conn = sqlite3.connect(audit_db)
            cursor = conn.cursor()
            cursor.execute('''
                CREATE TABLE IF NOT EXISTS tool_calls (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    tool_name TEXT NOT NULL,
                    params TEXT,
                    result TEXT,
                    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
                )
            ''')
            conn.commit()
            conn.close()
            
            return json.dumps({
                'success': True,
                'output': f'Audit database initialized at {audit_db}',
                'error': ''
            })
        
        elif action == 'log_tool_call':
            # Log a tool call to the audit database
            audit_db = os.path.expanduser('~/.gorkbot/audit.db')
            tool_name = data.get('tool_name', '')
            params = json.dumps(data.get('params', {}))
            result = data.get('result', '')
            
            if not os.path.exists(audit_db):
                return json.dumps({
                    'success': False,
                    'output': '',
                    'error': 'Audit database not initialized. Run init_audit first.'
                })
            
            conn = sqlite3.connect(audit_db)
            cursor = conn.cursor()
            cursor.execute(
                'INSERT INTO tool_calls (tool_name, params, result) VALUES (?, ?, ?)',
                (tool_name, params, result)
            )
            conn.commit()
            conn.close()
            
            return json.dumps({
                'success': True,
                'output': 'Tool call logged successfully',
                'error': ''
            })
        
        elif action == 'get_tool_history':
            # Retrieve tool call history
            audit_db = os.path.expanduser('~/.gorkbot/audit.db')
            limit = data.get('limit', 10)
            
            if not os.path.exists(audit_db):
                return json.dumps({
                    'success': False,
                    'output': '',
                    'error': 'Audit database not initialized'
                })
            
            conn = sqlite3.connect(audit_db)
            cursor = conn.cursor()
            cursor.execute(
                'SELECT id, tool_name, params, result, timestamp FROM tool_calls ORDER BY id DESC LIMIT ?',
                (limit,)
            )
            rows = cursor.fetchall()
            conn.close()
            
            history = [
                {'id': r[0], 'tool_name': r[1], 'params': r[2], 'result': r[3], 'timestamp': r[4]}
                for r in rows
            ]
            
            return json.dumps({
                'success': True,
                'output': json.dumps(history),
                'error': ''
            })
        
        elif action == 'register_tool':
            # Register a new tool dynamically
            tool_name = data.get('tool_name', '')
            tool_type = data.get('tool_type', 'bash')  # bash, http, etc.
            command = data.get('command', '')
            description = data.get('description', '')
            category = data.get('category', 'Custom')
            
            if not tool_name or not command:
                return json.dumps({
                    'success': False,
                    'output': '',
                    'error': 'Missing required fields: tool_name, command'
                })
            
            # Tool registry file path
            registry_path = os.path.expanduser('~/.gorkbot/tools_registry.json')
            os.makedirs(os.path.dirname(registry_path), exist_ok=True)
            
            # Load existing registry
            if os.path.exists(registry_path):
                with open(registry_path, 'r') as f:
                    registry = json.load(f)
            else:
                registry = {}
            
            # Add/update tool
            registry[tool_name] = {
                'type': tool_type,
                'command': command,
                'description': description,
                'category': category
            }
            
            with open(registry_path, 'w') as f:
                json.dump(registry, f, indent=2)
            
            return json.dumps({
                'success': True,
                'output': f'Tool {tool_name} registered successfully',
                'error': ''
            })
        
        elif action == 'list_tools':
            # List all registered tools
            registry_path = os.path.expanduser('~/.gorkbot/tools_registry.json')
            
            if not os.path.exists(registry_path):
                return json.dumps({
                    'success': True,
                    'output': '[]',
                    'error': ''
                })
            
            with open(registry_path, 'r') as f:
                registry = json.load(f)
            
            return json.dumps({
                'success': True,
                'output': json.dumps(registry),
                'error': ''
            })
        
        else:
            return json.dumps({
                'success': False,
                'output': '',
                'error': f'Unknown action: {action}'
            })
            
    except json.JSONDecodeError as e:
        return json.dumps({
            'success': False,
            'output': '',
            'error': f'Invalid JSON input: {str(e)}'
        })
    except Exception as e:
        return json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        })

if __name__ == '__main__':
    print(main())
