import sys
import json
import os
import re
from pathlib import Path

def generate_skill_content(skill_type, occurrences):
    """Generate SKILL.md content based on failure pattern type."""
    skills = {
        "bash_sanitizer": {
            "filename": "SKILL-bash-sanitizerreject.md",
            "content": """# Bash Sanitizer Reject Fix

## Pattern
Auto-strips control characters from bash commands; retries with sanitized input.

## Trigger
- SanitizerRejects with bash commands containing control chars (\x00-\x1f, \x7f)
- Path validation failures

## Action
1. Strip ASCII control characters (0x00-0x1F, 0x7F) from command strings
2. Validate paths don't contain /../ traversal
3. Retry with sanitized input
4. Escape special shell characters

## Example
```python
import re
def sanitize_bash(cmd):
    # Remove control chars
    cleaned = re.sub(r'[\x00-\x1f\x7f]', '', cmd)
    # Escape pipes and redirects
    return cleaned.replace('|', '\\|').replace('>', '\\>')
```
"
        },
        "mcp_transport_retry": {
            "filename": "SKILL-mcp-transport-retry.md",
            "content": """# MCP Transport Retry Fix

## Pattern
Handles MCP transport closures and timeouts with exponential backoff.

## Trigger
- MCP transport closure errors
- Connection timeouts
- ToolFailures in MCP-dependent tools

## Action
1. Catch TransportError, TimeoutError
2. Implement exponential backoff (1s, 2s, 4s, max 30s)
3. Add retry counter (max 3 attempts)
4. Fallback to cached data if available

## Example
```python
import time
def mcp_retry(fn, max_retries=3):
    for attempt in range(max_retries):
        try:
            return fn()
        except (TransportError, TimeoutError) as e:
            if attempt == max_retries - 1:
                raise
            wait = 2 ** attempt
            time.sleep(wait)
```
"
        },
        "web_fetch_retry": {
            "filename": "SKILL-web-fetch-retry.md",
            "content": """# Web Fetch Retry Fix

## Pattern
Handles web fetch failures with user-agent rotation and retries.

## Trigger
- Empty results from web_search
- HTTP 429/503 errors
- Network timeouts

## Action
1. Rotate User-Agent header
2. Add delay between requests (2s minimum)
3. Retry up to 3 times with different approach
4. Fallback to cached/heuristic results

## Example
```python
user_agents = [
    'Mozilla/5.0 (Linux; Android 14)',
    'Mozilla/5.0 (Windows NT 10.0)'
]
def smart_fetch(url, retries=3):
    for i in range(retries):
        headers = {'User-Agent': user_agents[i % len(user_agents)]}
        try:
            return requests.get(url, headers=headers, timeout=10)
        except:
            time.sleep(2)
    return None
```
"
        },
        "path_validation": {
            "filename": "SKILL-path-validation.md",
            "content": """# Path Validation Fix

## Pattern
Validates file paths to prevent sanitizer rejections.

## Trigger
- Paths with control characters
- Traversal attempts (/../)
- Invalid path separators

## Action
1. Reject paths with control characters
2. Block traversal attempts
3. Resolve and validate real path
4. Use safe path joiners

## Example
```python
def safe_path(path):
    # Block traversal
    if '..' in Path(path).parts:
        raise ValueError('Traversal blocked')
    # Remove control chars
    return re.sub(r'[\\x00-\\x1f\\x7f]', '', path)
```
"
        }
    }
    return skills.get(skill_type, skills["bash_sanitizer"])

def analyze_traces(traces):
    """Analyze failure traces and identify patterns."""
    patterns = {
        "bash_sanitizer": 0,
        "mcp_transport_retry": 0,
        "web_fetch_retry": 0,
        "path_validation": 0
    }
    
    for trace in traces:
        error_type = trace.get("type", "").lower()
        tool = trace.get("tool", "").lower()
        error_msg = trace.get("error", "").lower()
        
        if "sanitizer" in error_type or "sanitizer" in error_msg:
            if "bash" in tool or "control" in error_msg:
                patterns["bash_sanitizer"] += 1
            else:
                patterns["path_validation"] += 1
        
        if "mcp" in tool or "transport" in error_msg or "timeout" in error_msg:
            patterns["mcp_transport_retry"] += 1
        
        if "web" in tool or "fetch" in error_msg or "search" in tool:
            patterns["web_fetch_retry"] += 1
    
    return patterns

def main():
    try:
        data = json.loads(sys.stdin.read())
        
        # Get traces and output directory
        traces = data.get("traces", [])
        output_dir = data.get("output_dir", os.path.expanduser("~/.config/gorkbot/sense/skills/"))
        min_evidence = data.get("min_evidence", 3)
        
        # Create output directory
        os.makedirs(output_dir, exist_ok=True)
        
        # Analyze patterns
        patterns = analyze_traces(traces)
        
        # Generate skills for patterns meeting threshold
        generated = []
        for skill_type, count in patterns.items():
            if count >= min_evidence:
                skill = generate_skill_content(skill_type, count)
                filepath = os.path.join(output_dir, skill["filename"])
                with open(filepath, "w") as f:
                    f.write(skill["content"])
                generated.append({"skill": skill_type, "occurrences": count, "file": filepath})
        
        output = f"Generated {len(generated)} SKILL.md files from {len(traces)} traces:\n"
        for g in generated:
            output += f"- {g['file']} ({g['occurrences']} occurrences)\n"
        
        print(json.dumps({
            "success": True,
            "output": output.strip(),
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
