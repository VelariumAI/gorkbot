import sys
import json
import re

def parse_audit_report(markdown_text):
    """Parse Gorkbot self-audit markdown report and extract structured data."""
    result = {
        "system_health": "UNKNOWN",
        "platform": "",
        "version": "",
        "generated": "",
        "metrics": {},
        "tools": {
            "total_registered": 0,
            "categories": 0,
            "working": [],
            "issues": []
        },
        "security": {
            "network_secure": False,
            "privilege_escalation": False,
            "hitl_guard": False
        },
        "memory": {
            "agemem_percent": 0,
            "mel_heuristics": 0,
            "goal_ledger": "UNKNOWN"
        },
        "cci": {
            "documented": [],
            "missing": []
        },
        "consultation": "UNKNOWN",
        "findings": {
            "critical": [],
            "medium": [],
            "low": []
        },
        "recommendations": []
    }
    
    lines = markdown_text.split('\n')
    
    # Extract version and platform
    version_match = re.search(r'Gorkbot v([\d.]+)', markdown_text)
    if version_match:
        result["version"] = version_match.group(1)
    
    platform_match = re.search(r'Platform:\s*(.+?)(?:\n|$)', markdown_text)
    if platform_match:
        result["platform"] = platform_match.group(1).strip()
    
    generated_match = re.search(r'Generated:\s*(.+?)(?:\n|$)', markdown_text)
    if generated_match:
        result["generated"] = generated_match.group(1).strip()
    
    # Extract system health
    health_match = re.search(r'Overall Assessment:\s*\*\*(.+?)\*\*', markdown_text)
    if health_match:
        result["system_health"] = health_match.group(1).strip()
    
    # Extract tool inventory metrics
    tools_total = re.search(r'Total Tools Registered\s*\|\s*(\d+)', markdown_text)
    if tools_total:
        result["tools"]["total_registered"] = int(tools_total.group(1))
    
    categories = re.search(r'Categories\s*\|\s*(\d+)', markdown_text)
    if categories:
        result["tools"]["categories"] = int(categories.group(1))
    
    # Extract working tools
    working_tools = re.findall(r'`(\w+)`\s*\|\s*\w+\s*\|\s*(?:✅|WORKING)', markdown_text)
    result["tools"]["working"] = list(set(working_tools))
    
    # Extract tool issues
    tool_issues = re.findall(r'\*\s*\*\*([^\*]+)\*\*\s*\|\s*(\w+)\s*\|\s*(.+)', markdown_text)
    for issue in tool_issues:
        result["tools"]["issues"].append({
            "tool": issue[0].strip(),
            "severity": issue[1].strip(),
            "description": issue[2].strip()
        })
    
    # Extract security findings
    if '✅ SECURE' in markdown_text or 'No exposed network' in markdown_text:
        result["security"]["network_secure"] = True
    if 'No SUID' in markdown_text:
        result["security"]["privilege_escalation"] = True
    if 'HITL' in markdown_text and 'ENABLED' in markdown_text:
        result["security"]["hitl_guard"] = True
    
    # Extract memory metrics
    agemem_match = re.search(r'AgeMem\s*\|\s*\w+\s*\|\s*STM:[^\(]*\(([\d.]+)%\)', markdown_text)
    if agemem_match:
        result["memory"]["agemem_percent"] = float(agemem_match.group(1))
    
    mel_match = re.search(r'MEL VectorStore\s*\|\s*\w+\s*\|\s*(\d+)\s*heuristics', markdown_text)
    if mel_match:
        result["memory"]["mel_heuristics"] = int(mel_match.group(1))
    
    # Extract CCI documentation status
    documented = re.findall(r'(\w+)\s*\|\s*✅\s*\|', markdown_text)
    result["cci"]["documented"] = documented
    
    missing_cci = re.findall(r'Missing Documentation:\s*(.+)', markdown_text)
    if missing_cci:
        missing_list = re.findall(r'(\w+)', missing_cci[0])
        result["cci"]["missing"] = missing_list
    
    # Extract consultation status
    if 'VERIFIED WORKING' in markdown_text or 'CONFIRMED WORKING' in markdown_text:
        result["consultation"] = "VERIFIED"
    elif '✅ VERIFIED' in markdown_text:
        result["consultation"] = "VERIFIED"
    
    # Extract findings by severity
    critical_section = re.search(r'### Critical Findings\s*\n(.*?)(?=###|##\s*Recommendations|$)', markdown_text, re.DOTALL)
    if critical_section:
        critical_findings = re.findall(r'\|\s*\d+\s*\|\s*(.+?)\s*\|', critical_section.group(1))
        result["findings"]["critical"] = [f.strip() for f in critical_findings if f.strip()]
    
    medium_section = re.search(r'### Medium Findings\s*\n(.*?)(?=###|##\s*Recommendations|$)', markdown_text, re.DOTALL)
    if medium_section:
        medium_findings = re.findall(r'\|\s*\d+\s*\|\s*(.+?)\s*\|', medium_section.group(1))
        result["findings"]["medium"] = [f.strip() for f in medium_findings if f.strip()]
    
    low_section = re.search(r'### Low Findings\s*\n(.*?)(.*?)(?=###|$)', markdown_text, re.DOTALL)
    if low_section:
        low_findings = re.findall(r'\|\s*\d+\s*\|\s*(.+?)\s*\|', low_section.group(1))
        result["findings"]["low"] = [f.strip() for f in low_findings if f.strip()]
    
    # Extract recommendations
    recommendations = re.findall(r'\d+\.\s*\*\*(.+?)\*\*:\s*(.+?)(?=\n\d|\n\*|$)', markdown_text, re.DOTALL)
    for rec in recommendations:
        result["recommendations"].append({
            "action": rec[0].strip(),
            "details": rec[1].strip()
        })
    
    return result

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        
        if 'input_md' not in input_data:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "Missing 'input_md' field in input JSON"
            }))
            return
        
        markdown_text = input_data['input_md']
        parsed = parse_audit_report(markdown_text)
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(parsed, indent=2),
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
