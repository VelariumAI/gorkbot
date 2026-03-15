import sys
import json
import subprocess

def main():
    try:
        # Read input JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Extract message - default to test message if not provided
        message = input_data.get('message', 'Hello, this is a test of the text to speech system.')
        
        # Step 1: Set music stream volume to max (100)
        # Using termux-volume to set music stream (stream 3) to 100
        volume_cmd = ['termux-volume', 'music', '100']
        try:
            subprocess.run(volume_cmd, capture_output=True, check=True)
        except FileNotFoundError:
            # Fallback: try using termux-volume with different syntax
            try:
                subprocess.run(['termux-volume', '3', '100'], capture_output=True, check=True)
            except Exception as e:
                pass  # Continue even if volume setting fails
        
        # Step 2: Speak the message using termux-tts-speak
        speak_cmd = ['termux-tts-speak', message]
        result = subprocess.run(speak_cmd, capture_output=True, text=True)
        
        # Build output response
        output = {
            'success': result.returncode == 0,
            'output': f"TTS spoken: '{message}'" + (f"\n{result.stdout}" if result.stdout else ""),
            'error': result.stderr if result.stderr else ""
        }
        
        print(json.dumps(output))
        
    except json.JSONDecodeError as e:
        print(json.dumps({'success': False, 'output': '', 'error': f'Invalid JSON input: {str(e)}'}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
