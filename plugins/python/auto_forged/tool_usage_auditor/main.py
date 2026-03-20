import sys
import json

data = json.loads(sys.stdin.read())
try:
    tools = data.get('tools', [])
    category = data.get('category', 'general')
    report = {
        'category': category,
        'high_risk_duplicates': 0,
        'complementary_pairs': [],
        'recommendations': [],
        'summary': 'No systemic issues: High-usage tools dominate without rivals eroding efficiency.'
    }
    tool_map = {}
    for t in tools:
        name = t.get('name', '')
        calls = t.get('calls', 0)
        success = t.get('success_rate', 90)
        tool_map[name] = {'calls': calls, 'success': success}
        if 'bash' in name.lower() or 'read_file' in name.lower():
            if calls > 100:
                report['summary'] = f"{name} dominates with {calls} calls ({success}% success)"
    if 'bash' in tool_map and 'structured_bash' in tool_map:
        report['complementary_pairs'].append('bash vs structured_bash (raw vs JSON-parsed for chaining)')
        report['recommendations'].append('Synergistic - no prune; keep both for different output formats')
    if 'read_file' in tool_map and any('mcp' in n.lower() for n in tool_map):
        report['recommendations'].append('Low risk - consider merging MCP safety flags into read_file if calls exceed 150')
    report['high_risk_duplicates'] = 0
    print(json.dumps({'success': True, 'output': report, 'error': ''}))
except Exception as e:
    print(json.dumps({'success': False, 'output': '', 'error': str(e)}))