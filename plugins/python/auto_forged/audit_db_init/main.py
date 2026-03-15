import sys
import json
import os
import sqlite3

def main():
    try:
        # Read JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Get database path (default: ~/.gorkbot/audit.db)
        db_path = input_data.get('db_path', os.path.expanduser('~/.gorkbot/audit.db'))
        
        # Ensure directory exists
        db_dir = os.path.dirname(db_path)
        if db_dir and not os.path.exists(db_dir):
            os.makedirs(db_dir, exist_ok=True)
        
        # Connect and create table
        conn = sqlite3.connect(db_path)
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
        
        print(json.dumps({
            "success": True,
            "output": f"Audit DB initialized: {db_path}",
            "error": ""
        }))
        
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
