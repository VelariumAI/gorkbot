import sys
import json
import os
from datetime import datetime

def main():
    try:
        data = json.loads(sys.stdin.read())
        
        # Extract failure data from input
        failures = data.get('failures', [])
        tool_stats = data.get('tool_stats', {})
        
        # Pattern detection and skill generation
        skills = []
        
        # Analyze failure patterns
        failure_types = {}
        for f in failures:
            ftype = f.get('type', 'unknown')
            tool = f.get('tool', 'unknown')
            key = f"{tool}:{ftype}"
            failure_types[key] = failure_types.get(key, 0) + 1
        
        # Generate skill for bash sanitizer rejects
        if any('bash:sanitizer' in k.lower() for k in failure_types):
            skills.append({
                'name': 'bash-sanitizerreject',
                'description': 'Auto-strips control characters from bash commands; retries with sanitized input',
                'content': '''# SKILL: bash-sanitizerreject
# Fixes: SanitizerRejects on bash commands with control characters

## Trigger
- Any bash command containing control characters (ASCII 0-31 except tab/newline)
- ToolFailure or SanitizerReject on bash operations

## Action
1. Strip control characters from command
2. Validate paths don't contain rejected patterns
3. Retry with sanitized input

## Code
```python
import re
def sanitize_bash(cmd):
    # Remove control chars except tab/newline
    return re.sub(r'[\x00-\x08\x0b\x0c\x0e-\x1f]', '', cmd)
```
'''
            })
        
        # Generate skill for MCP transport retries
        if any('mcp' in k.lower() for k in failure_types):
            skills.append({
                'name': 'mcp_transport_retry',
                'description': 'Implements retry logic for MCP transport closures',
                'content': '''# SKILL: mcp_transport_retry
# Fixes: MCP transport closure errors, timeouts

## Trigger
- MCP transport errors
- Connection failures to external APIs

## Action
1. Implement exponential backoff (1s, 2s, 4s, max 3 retries)
2. Check connectivity before call
3. Fallback to cached data if available

## Code
```python
import time
def mcp_retry(func, max_retries=3):
    for i in range(max_retries):
        try:
            return func()
        except TransportError:
            if i == max_retries - 1: raise
            time.sleep(2 ** i)
```
'''
            })
        
        # Generate skill for web fetch failures
        if any('web' in k.lower() for k in failure_types):
            skills.append({
                'name': 'web_fetch_fallback',
                'description': 'Handles web fetch failures with fallback to cached/heuristic data',
                'content': '''# SKILL: web_fetch_fallback
# Fixes: Empty results from web searches, network timeouts

## Trigger
- web_search returns empty results
- web_fetch timeout or connection error

## Action
1. Retry with shorter query
2. Fallback to heuristic/synthetic response
3. Log for pattern analysis

## Code
```python
def web_fetch_safe(query, fallback=None):
    try:
        result = web_search(query)
        return result if result else fallback
    except TimeoutError:
        return fallback
```
'''
            })
        
        # Build output
        output = {
            'success': True,
            'output': {
                'generated_skills': len(skills),
                'skills': skills,
                'failure_analysis': failure_types,
                'recommendation': f"Generate {len(skills)} SKILL.md files to address {len(failures)} failures"
            },
            'error': ''
        }
        
        print(json.dumps(output, indent=2))
        
    except json.JSONDecodeError as e:
        print(json.dumps({'success': False, 'output': '', 'error': f'Invalid JSON: {str(e)}'}))
    except Exception as e:
        print(json.dumps({'success': False, 'output': '', 'error': str(e)}))

if __name__ == '__main__':
    main()
