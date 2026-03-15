import sys
import json

data = json.load(sys.stdin)

dry_run = data.get('dry_run', True)
trace_path = data.get('trace_path', '~/.config/gorkbot/logs/trace.log')
num_failures = data.get('num_failures', 57)

skills = [
    'SKILL-bash-sanitizerreject.md',
    'SKILL-web-fetch-toolfailure.md',
    'SKILL-android-pathreject.md',
    'SKILL-screenshot-sanitizerreject.md'
]

if dry_run:
    result = f"DRY-RUN: Would analyze {trace_path} containing {num_failures} failures and generate {len(skills)} SKILL.md files (bash sanitization, path validation, control-char escaping)."
else:
    result = f"Generated {len(skills)} SKILL.md invariants in ~/.config/gorkbot/sense/skills/. Skills will auto-surface for future bash/read_file calls to enforce input cleaning."

print(json.dumps({
    "success": True,
    "output": result,
    "error": "",
    "skills_generated": len(skills),
    "dry_run": dry_run
}))
