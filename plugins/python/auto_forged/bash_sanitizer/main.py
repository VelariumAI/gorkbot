import sys
import json
import re

data = json.load(sys.stdin)
command = data.get('command', '').strip()

if not command:
    print(json.dumps({'success': False, 'output': '', 'error': 'No command provided'}))
    sys.exit(0)

# Dangerous patterns that should be rejected
DANGEROUS_PATTERNS = [
    r'rm\s+-rf\s+(/|~|/data|/system)',
    r'rm\s+-rf\s+\*',
    r'\.{2,}/',  # path traversal
    r'(>\s*|>>|/dev/null\s*<)\s*/(etc|root|proc|sys)',
    r'chmod\s+-R\s+777\s+/', 
]

# Allowed base paths for this environment
ALLOWED_BASES = ['/storage/emulated', '/sdcard', '~/.gorkbot', '$HOME', '/data/local/tmp', '~/']

is_dangerous = any(re.search(pat, command, re.IGNORECASE) for pat in DANGEROUS_PATTERNS)

if is_dangerous:
    print(json.dumps({'success': False, 'output': '', 'error': 'Dangerous path escape or injection detected'}))
    sys.exit(0)

# Fix common /tmp escapes (Termux often restricts /tmp)
sanitized = re.sub(r'(?i)/tmp(?=/|\\s|$)', '/data/local/tmp', command)

# Ensure command targets allowed areas if it uses absolute paths
if re.search(r'^\s*(rm|du|ls|cat|find)\b', sanitized) and '/' in sanitized:
    if not any(base in sanitized for base in ALLOWED_BASES):
        print(json.dumps({'success': False, 'output': '', 'error': 'Command targets disallowed path'}))
        sys.exit(0)

result = {
    'success': True,
    'output': f'Sanitized command: {sanitized}',
    'error': '',
    'original': command,
    'sanitized': sanitized
}
print(json.dumps(result))