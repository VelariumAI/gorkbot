import sys
import json
import subprocess

def get_battery_level():
    """Get battery percentage on Termux."""
    try:
        result = subprocess.run(
            ['termux-battery-status'],
            capture_output=True,
            text=True,
            timeout=10
        )
        if result.returncode == 0:
            data = json.loads(result.stdout)
            return int(data.get('percentage', 0))
    except Exception:
        pass
    return -1

def toggle_wifi(enable):
    """Toggle WiFi on or off."""
    cmd = ['termux-wifi-enable', 'true' if enable else 'false']
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
        return result.returncode == 0
    except Exception:
        return False

def send_notification(message):
    """Send a notification in Termux."""
    try:
        subprocess.run(
            ['termux-notification', '-t', 'Battery Manager', '-c', message],
            capture_output=True,
            timeout=10
        )
        return True
    except Exception:
        return False

def main():
    input_data = json.load(sys.stdin)
    
    battery_threshold = input_data.get('threshold', 20)
    
    battery_level = get_battery_level()
    
    if battery_level < 0:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': 'Failed to get battery status'
        }))
        return
    
    output = f"Battery level: {battery_level}%"
    
    if battery_level < battery_threshold:
        wifi_off = toggle_wifi(False)
        if wifi_off:
            message = 'Low battery - WiFi saved'
            send_notification(message)
            output += f", WiFi turned off, notification sent: {message}"
        else:
            output += ", failed to toggle WiFi"
    else:
        output += ", battery OK - no action needed"
    
    print(json.dumps({
        'success': True,
        'output': output,
        'error': ''
    }))

if __name__ == '__main__':
    main()
