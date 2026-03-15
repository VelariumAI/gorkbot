import sys
import json
import subprocess
import os

def main():
    try:
        input_data = json.load(sys.stdin)
        commands = input_data.get('commands', ["mkdir -p /tmp/gorkbot_test/", "echo 'Gorkbot self-test passed'"])
        log_dir = input_data.get('log_dir', '/tmp/gorkbot_test')

        os.makedirs(log_dir, exist_ok=True)

        results = []
        success_count = 0
        total = len(commands)

        for i, cmd in enumerate(commands):
            try:
                result = subprocess.run(
                    ['bash', '-c', cmd],
                    capture_output=True,
                    text=True,
                    timeout=30,
                    cwd=log_dir
                )
                output = result.stdout.strip()
                error = result.stderr.strip()
                success = result.returncode == 0
                if success:
                    success_count += 1

                log_file = os.path.join(log_dir, f'test_{i+1}.log')
                with open(log_file, 'w') as f:
                    f.write(f'Command: {cmd}\nReturn code: {result.returncode}\nOutput: {output}\nError: {error}\nSuccess: {success}\n')

                results.append({
                    'command': cmd,
                    'success': success,
                    'return_code': result.returncode,
                    'output': output,
                    'error': error,
                    'log_file': log_file
                })
            except subprocess.TimeoutExpired:
                results.append({'command': cmd, 'success': False, 'error': 'Timeout (30s)' })
            except Exception as e:
                results.append({'command': cmd, 'success': False, 'error': str(e)})

        success_rate = round((success_count / total * 100), 1) if total > 0 else 0.0

        summary = {
            'total_tests': total,
            'success_count': success_count,
            'success_rate_percent': success_rate,
            'log_directory': log_dir,
            'results': results
        }

        output_json = {
            'success': True,
            'output': json.dumps(summary),
            'error': ''
        }
        print(json.dumps(output_json))
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()