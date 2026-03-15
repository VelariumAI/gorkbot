import sys
import json
import subprocess

def main():
    try:
        data = json.load(sys.stdin)
        
        # Get parameters with defaults
        mode = data.get('mode', 'sync')  # 'sync' or 'full'
        
        if mode == 'sync':
            # Safer: just flush buffers (~500MB gain)
            cmd = ['sync']
            description = 'Flushing disk buffers'
        elif mode == 'full':
            # Aggressive: drop all caches (~1GB gain, requires write to /proc)
            cmd = 'sync && echo 1 > /proc/sys/vm/drop_caches'
            description = 'Flushing buffers and dropping page cache'
        else:
            print(json.dumps({
                'success': False,
                'output': '',
                'error': f'Invalid mode: {mode}. Use "sync" or "full"'
            }))
            return
        
        if mode == 'sync':
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            output = result.stdout + result.stderr
            success = result.returncode == 0
        else:
            # Use shell for the full mode with proc write
            result = subprocess.run(
                cmd, 
                shell=True, 
                capture_output=True, 
                text=True, 
                timeout=30
            )
            output = result.stdout + result.stderr
            success = result.returncode == 0
        
        if not success:
            # Check for OOM kill pattern
            if 'killed' in output.lower() or 'signal' in output.lower():
                error_msg = 'OOM killed - system under memory pressure, try lighter sync mode'
            else:
                error_msg = output
            print(json.dumps({
                'success': False,
                'output': description,
                'error': error_msg
            }))
        else:
            print(json.dumps({
                'success': True,
                'output': f'{description} - completed successfully',
                'error': ''
            }))
            
    except subprocess.TimeoutExpired:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': 'Command timed out'
        }))
    except json.JSONDecodeError:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': 'Invalid JSON input'
        }))
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()
