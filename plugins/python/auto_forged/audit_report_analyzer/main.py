import sys
import json
import re

def parse_audit_report(markdown_text):
    """Extract structured data from Gorkbot audit markdown report."""
    
    results = {
        "overall_health": "UNKNOWN",
        "tool_metrics": {
            "total_tools": 0,
            "categories": 0,
            "top_tools": []
        },
        "system_status": {
            "tooling": "UNKNOWN",
            "security": "UNKNOWN",
            "memory": "UNKNOWN",
            "cci": "UNKNOWN",
            "consultation": "UNKNOWN"
        },
        "findings": {
            "critical": [],
            "medium": [],
            "low": []
        },
        "recommendations": [],
        "missing_packages": [],
        "security_checks": {},
        "resource_usage": {}
    }
    
    lines = markdown_text.split('\n')
    
    # Extract overall health status
    health_match = re.search(r'\*\*Overall Assessment:\s*\*\*\s*(\w+)', markdown_text, re.IGNORECASE)
    if health_match:
        results["overall_health"] = health_match.group(1).strip()
    
    # Extract system status scores from Executive Summary table
    status_patterns = [
        (r'\*\*Tooling\s*\*\*\s*\|\s*\S+\s*\|\s*(\d+)/100', 'tooling'),
        (r'\*\*System Security\s*\*\*\s*\|\s*\S+\s*\|\s*(\d+)/100', 'security'),
        (r'\*\*Memory Systems\s*\*\*\s*\|\s*\S+\s*\|\s*(\d+)/100', 'memory'),
        (r'\*\*CCI Infrastructure\s*\*\*\s*\|\s*\S+\s*\|\s*(\d+)/100', 'cci'),
        (r'\*\*Consultation System\s*\*\*\s*\|\s*\S+\s*\|\s*(\d+)/100', 'consultation')
    ]
    
    for pattern, key in status_patterns:
        match = re.search(pattern, markdown_text)
        if match:
            results["system_status"][key] = match.group(1)
    
    # Extract tool inventory metrics
    tool_inventory = re.search(r'\*\*Total Tools Registered\s*\*\*\s*\|\s*(\d+)', markdown_text)
    if tool_inventory:
        results["tool_metrics"]["total_tools"] = int(tool_inventory.group(1))
    
    categories = re.search(r'\*\*Categories\s*\*\*\s*\|\s*(\d+)', markdown_text)
    if categories:
        results["tool_metrics"]["categories"] = int(categories.group(1))
    
    # Extract top executed tools
    top_tools_match = re.search(r'\*\*Top Executed\s*\*\*\s*\|(.+?)(?=\n\n|## )', markdown_text, re.DOTALL)
    if top_tools_match:
        tools_text = top_tools_match.group(1)
        for line in tools_text.split('\n'):
            tool_match = re.search(r'`(\w+)`\s*\(([^)]+)\)', line)
            if tool_match:
                name = tool_match.group(1)
                stats = tool_match.group(2)
                results["tool_metrics"]["top_tools"].append({
                    "name": name,
                    "stats": stats.strip()
                })
    
    # Extract findings by severity
    findings_sections = [
        (r'### \d+\.\d+ Critical Findings\s*\n(.+?)(?=\n###|\n## |\Z)', 'critical'),
        (r'### \d+\.\d+ Medium Findings\s*\n(.+?)(?=\n###|\n## |\Z)', 'medium'),
        (r'### \d+\.\d+ Low Findings\s*\n(.+?)(?=\n###|\n## |\Z)', 'low')
    ]
    
    for pattern, severity in findings_sections:
        match = re.search(pattern, markdown_text, re.DOTALL | re.IGNORECASE)
        if match:
            section = match.group(1)
            # Extract numbered findings
            finding_matches = re.findall(r'^\d+\s+\|\s+(.+?)\s*\|\s*\w+\s*\|', section, re.MULTILINE)
            for finding in finding_matches:
                results["findings"][severity].append(finding.strip())
    
    # Extract recommendations
    rec_match = re.search(r'### \d+ Recommendations\s*\n(.+?)(?=\n## |\Z)', markdown_text, re.DOTALL)
    if rec_match:
        rec_section = rec_match.group(1)
        # Extract action items
        action_items = re.findall(r'^\d+\.\s+\*\*([^\*]+)\*\*:\s*(.+?)(?=\n|\Z)', rec_section, re.MULTILINE)
        for action_type, description in action_items:
            results["recommendations"].append({
                "type": action_type.strip(),
                "description": description.strip()
            })
    
    # Extract missing packages
    pkg_matches = re.findall(r'pkg install (\w+(?: \w+)?)', markdown_text)
    results["missing_packages"] = list(set(pkg_matches))
    
    # Extract resource usage
    cpu_match = re.search(r'\*\*CPU Cores\s*\*\*\s*\|\s*(\d+)', markdown_text)
    if cpu_match:
        results["resource_usage"]["cpu_cores"] = int(cpu_match.group(1))
    
    mem_match = re.search(r'\*\*Memory\s*\|\s*(\d+ \w+)\((\d+)%', markdown_text)
    if mem_match:
        results["resource_usage"]["memory_total"] = mem_match.group(1)
        results["resource_usage"]["memory_percent"] = int(mem_match.group(2))
    
    disk_match = re.search(r'\*\*Disk.*?\|\s*(\d+ \w+)\((\d+)%', markdown_text)
    if disk_match:
        results["resource_usage"]["disk_total"] = disk_match.group(1)
        results["resource_usage"]["disk_percent"] = int(disk_match.group(2))
    
    return results

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        
        markdown_text = input_data.get('input_md', '')
        
        if not markdown_text:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "No input_md field provided"
            }))
            return
        
        results = parse_audit_report(markdown_text)
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(results, indent=2),
            "error": ""
        }))
        
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"JSON parse error: {str(e)}"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Processing error: {str(e)}"
        }))

if __name__ == "__main__":
    main()
