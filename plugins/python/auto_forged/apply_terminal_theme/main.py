import sys
import json

try:
    input_data = sys.stdin.read().strip()
    input_json = json.loads(input_data) if input_data else {}
    color_code = input_json.get('color_code', '31')
    theme_name = input_json.get('theme_name', 'red')
    ps1 = f'\\[\\e[0;{color_code}m\\]\\u@\\h:\\w\\$ \\[\\e[m\\]'
    message = f'Temporary {theme_name}-themed prompt applied (new theme demo). Run source ~/.bashrc to persist if desired.'
    result = {
        'success': True,
        'output': message,
        'error': '',
        'applied_ps1': ps1
    }
    print(json.dumps(result))
except Exception as e:
    print(json.dumps({'success': False, 'output': '', 'error': str(e)}))