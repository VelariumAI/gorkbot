import sys
import json
import re

def parse_tool_inventory(text):
    """Extract tool inventory metrics from markdown."""
    result = {}
    
    # Extract total tools registered
    match = re.search(r'\|\s*\*\*Total Tools Registered\s*\*\*\s*\|\s*(\d+)\s*\|', text)
    if match:
        result['total_tools'] = int(match.group(1))
    
    # Extract categories
    match = re.search(r'\|\s*\*\*Categories\s*\*\*\s*\|\s*(\d+)\s*\(', text)
    if match:
        result['categories'] = int(match.group(1))
    
    # Extract top executed tools
    top_tools = []
    match = re.search(r'\|\s*\*\*Top Executed\s*\*\*\s*\|\s*(.+?)\s*\|', text)
    if match:
        top_text = match.group(1)
        tool_matches = re.findall(r'(\w+)\s*\((\d+)\s*calls?,\s*(\d+)%\s*success\)', top_text)
        for name, calls, success in tool_matches:
            top_tools.append({
                'tool': name,
                'calls': int(calls),
                'success_rate': int(success)
            })
    result['top_executed'] = top_tools
    
    return result

def parse_tool_validation(text):
    """Extract tool validation results from markdown tables."""
    tools = []
    
    # Find tool validation table section
    table_match = re.search(r'\|\s*Tool Name\s*\|.*?\|\s*Notes\s*\|\s*(.+?)(?:\n\n|## |$)', text, re.DOTALL)
    if table_match:
        table_text = table_match.group(1)
        lines = table_text.strip().split('\n')
        for line in lines:
            # Skip separator lines
            if re.match(r'\|\s*[-:]+', line):
                continue
            
            # Parse table row: | tool | category | status | notes |
            cols = [c.strip() for c in line.split('|') if c.strip()]
            if len(cols) >= 3:
                tool_name = cols[0].replace('`', '')
                category = cols[1]
                status = cols[2]
                notes = cols[3] if len(cols) > 3 else ''
                
                # Determine if working
                is_working = '✅' in status or 'WORKING' in status or 'AVAILABLE' in status
                
                tools.append({
                    'name': tool_name,
                    'category': category,
                    'status': status.strip(),
                    'working': is_working,
                    'notes': notes
                })
    
    return tools

def parse_tool_issues(text):
    """Extract identified tool issues."""
    issues = []
    
    # Find tool issues table
    table_match = re.search(r'\|\s*Issue\s*\|.*?\|\s*Description\s*\|\s*(.+?)(?:\n\n|## |$)', text, re.DOTALL)
    if table_match:
        table_text = table_match.group(1)
        lines = table_text.strip().split('\n')
        for line in lines:
            if re.match(r'\|\s*[-:]+', line):
                continue
            
            cols = [c.strip() for c in line.split('|') if c.strip()]
            if len(cols) >= 3:
                issues.append({
                    'issue': cols[0],
                    'severity': cols[1],
                    'description': cols[2]
                })
    
    return issues

def parse_executive_summary(text):
    """Extract executive summary category scores."""
    summary = {}
    
    # Find executive summary table
    table_match = re.search(r'\|\s*Category\s*\|.*?\|\s*Score\s*\|\s*(.+?)(?:\n\n|## )', text, re.DOTALL)
    if table_match:
        table_text = table_match.group(1)
        lines = table_text.strip().split('\n')
        for line in lines:
            if re.match(r'\|\s*[-:]+', line):
                continue
            
            cols = [c.strip() for c in line.split('|') if c.strip()]
            if len(cols) >= 3:
                category = cols[0].replace('*', '')
                status = cols[1]
                score = cols[2]
                summary[category] = {
                    'status': status,
                    'score': score
                }
    
    return summary

def main():
    try:
        # Read JSON from stdin
        input_data = json.loads(sys.stdin.read())
        
        # Get markdown content from input
        markdown = input_data.get('input_md', '')
        
        if not markdown:
            print(json.dumps({
                'success': False,
                'output': '',
                'error': 'No input_md provided'
            }))
            return
        
        # Parse different sections
        result = {
            'executive_summary': parse_executive_summary(markdown),
            'tool_inventory': parse_tool_inventory(markdown),
            'tool_validation': parse_tool_validation(markdown),
            'tool_issues': parse_tool_issues(markdown)
        }
        
        # Calculate summary statistics
        total_validated = len(result['tool_validation'])
        working_count = sum(1 for t in result['tool_validation'] if t.get('working'))
        result['summary'] = {
            'total_tools_validated': total_validated,
            'working_tools': working_count,
            'success_rate': round(working_count / total_validated * 100, 1) if total_validated > 0 else 0
        }
        
        print(json.dumps({
            'success': True,
            'output': json.dumps(result, indent=2),
            'error': ''
        }))
        
    except json.JSONDecodeError as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': f'Invalid JSON input: {str(e)}'
        }))
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()
