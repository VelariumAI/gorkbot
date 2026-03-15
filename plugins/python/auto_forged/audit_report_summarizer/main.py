import sys
import json
import re

def extract_table_rows(md, section_start):
    """Extract markdown table rows between | markers"""
    rows = []
    in_table = False
    for line in md.split('\n'):
        if section_start in line:
            in_table = True
        if in_table and line.strip().startswith('|'):
            if '---' not in line:
                rows.append(line.strip())
        elif in_table and line.strip() == '':
            in_table = False
    return rows

def extract_metrics(md):
    """Extract key metrics from the audit report"""
    metrics = {}
    
    # Extract scores from Executive Summary
    score_pattern = r'\*\*(\w+)\*\*.*?(\d+/100|\d+%?)'
    for match in re.finditer(score_pattern, md):
        category = match.group(1)
        score = match.group(2)
        metrics[category] = score
    
    # Extract tool counts
    tools_match = re.search(r'Total Tools Registered.*?(\d+)', md)
    if tools_match:
        metrics['total_tools'] = int(tools_match.group(1))
    
    # Extract memory usage
    mem_match = re.search(r'STM: (\d+)/(\d+) tokens', md)
    if mem_match:
        used, total = mem_match.groups()
        metrics['agemem_usage'] = f"{int(used)/int(total)*100:.1f}%"
    
    # Extract MEL heuristics
    mel_match = re.search(r'(\d+) heuristics stored', md)
    if mel_match:
        metrics['mel_heuristics'] = int(mel_match.group(1))
    
    return metrics

def extract_findings(md):
    """Extract findings by severity"""
    findings = {'critical': [], 'medium': [], 'low': []}
    
    # Find severity sections
    sections = {
        'Critical': 'critical',
        'Medium': 'medium', 
        'Low': 'low'
    }
    
    current_severity = None
    for line in md.split('\n'):
        line = line.strip()
        for sev_name, sev_key in sections.items():
            if line.startswith(sev_name):
                current_severity = sev_key
        
        # Extract finding descriptions
        if current_severity and '|' in line and '---' not in line:
            parts = [p.strip() for p in line.split('|')]
            if len(parts) >= 3 and parts[1] and parts[1][0].isdigit():
                finding = parts[2] if len(parts) > 2 else parts[1]
                if finding and finding[0].isdigit():
                    findings[current_severity].append(finding)
    
    return findings

def extract_recommendations(md):
    """Extract actionable recommendations"""
    recs = []
    in_recs = False
    
    for line in md.split('\n'):
        if 'Recommendations' in line or 'Actions' in line:
            in_recs = True
            continue
        if in_recs:
            if line.strip().startswith('- ') or line.strip().startswith('1.'):
                rec = re.sub(r'^\s*[-*\d.]+\s*', '', line.strip())
                if rec and len(rec) > 5:
                    recs.append(rec)
            elif line.strip() == '' and len(recs) > 2:
                break
    
    return recs[:5]  # Top 5 recommendations

def summarize_audit_report(md):
    """Main function to summarize the audit report"""
    
    # Extract overall status
    health_match = re.search(r'Overall Assessment[:\s]*(\w+)', md, re.IGNORECASE)
    overall = health_match.group(1) if health_match else 'UNKNOWN'
    
    # Extract metrics
    metrics = extract_metrics(md)
    
    # Extract findings
    findings = extract_findings(md)
    
    # Extract recommendations
    recommendations = extract_recommendations(md)
    
    # Build summary
    summary = {
        'status': overall,
        'metrics': metrics,
        'findings': findings,
        'recommendations': recommendations
    }
    
    return summary

def main():
    try:
        # Read JSON from stdin
        input_data = json.load(sys.stdin)
        
        # Extract markdown from various possible fields
        md = ''
        if isinstance(input_data, dict):
            md = input_data.get('input_md', '') or input_data.get('content', '') or input_data.get('markdown', '')
        elif isinstance(input_data, str):
            md = input_data
        
        if not md:
            print(json.dumps({
                'success': False,
                'output': '',
                'error': 'No markdown content found in input'
            }))
            return
        
        # Process the markdown
        result = summarize_audit_report(md)
        
        # Output JSON result
        print(json.dumps({
            'success': True,
            'output': json.dumps(result, indent=2),
            'error': ''
        }))
        
    except Exception as e:
        print(json.dumps({
            'success': False,
            'output': '',
            'error': str(e)
        }))

if __name__ == '__main__':
    main()
