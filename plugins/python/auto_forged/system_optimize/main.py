import sys
import json
import subprocess
import os

def run_command(cmd):
    try:
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=60)
        return result.stdout + result.stderr, result.returncode
    except Exception as e:
        return str(e), 1

def main():
    try:
        data = json.loads(sys.stdin.read())
        action = data.get('action', 'help')
        
        output = ""
        success = True
        
        if action == 'sync':
            # Memory reclaim - flush buffers (safer than drop_caches)
            out, code = run_command('sync')
            output = f"Sync completed: {out}"
            success = code == 0
        
        elif action == 'drop_caches':
            # Full cache drop - requires root/sudo typically
            out, code = run_command('sync && echo 1 > /proc/sys/vm/drop_caches 2>/dev/null || sync')
            output = f"Cache drop attempted: {out}"
            success = code == 0
        
        elif action == 'disk_usage':
            # Analyze disk usage
            paths = data.get('paths', ['~/.cache', '~/.gorkbot/logs'])
            results = []
            for path in paths:
                expanded = os.path.expanduser(path)
                out, _ = run_command(f'du -sh {expanded} 2>/dev/null || echo "N/A"')
                results.append(out.strip())
            output = "\n".join(f"{paths[i]}: {results[i]}" for i in range(len(paths)))
        
        elif action == 'cleanup_cache':
            # Clean cache interactively
            out, code = run_command('du -sh ~/.cache/* ~/.gorkbot/logs/* 2>/dev/null | sort -hr | head -5')
            output = f"Large files:\n{out}"
        
        elif action == 'ollama_run':
            # Run ollama command
            prompt = data.get('prompt', 'Hello')
            model = data.get('model', 'llama3')
            out, code = run_command(f'ollama run {model} "{prompt}"')
            output = out
            success = code == 0
        
        elif action == 'ollama_pull':
            # Pull ollama model
            model = data.get('model', 'llama3:8b')
            out, code = run_command(f'ollama pull {model}')
            output = f"Pulled {model}: {out}"
            success = code == 0
        
        elif action == 'memory_info':
            # Quick memory check
            out, code = run_command('free -h')
            output = out
        
        elif action == 'help':
            output = "Actions: sync, drop_caches, disk_usage, cleanup_cache, ollama_run, ollama_pull, memory_info"
        
        else:
            output = f"Unknown action: {action}"
            success = False

        print(json.dumps({"success": success, "output": output, "error": ""}))
    
    except Exception as e:
        print(json.dumps({"success": false, "output": "", "error": str(e)}))

if __name__ == "__main__":
    main()
