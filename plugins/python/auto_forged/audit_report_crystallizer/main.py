import sys
import json
import re

def parse_audit_report(markdown_text):
    """Parse Gorkbot audit report markdown and extract structured data."""
    
    result = {
        "overall_health": None,
        "version": None,
        "platform": None,
        "categories": {},
        "tools": {
            "total_registered": 0,
            "top_executed": [],
            "issues": []
        },
        "security": {
            "network_secure": False,
            "sudo_access": False,
            "suid_binaries": False
        },
        "memory": {
            "agemem_percent": 0,
            "mel_heuristics": 0
        },
        "cci": {
            "specialists": [],
            "missing_docs": []
        },
        "consultation": {
            "working": False,
            "provider": None
        },
        "resources": {
            "cpu_cores": 0,
            "memory_total_gb": 0,
            "memory_used_percent": 0,
            "disk_root_percent": 0
        },
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
    
    platform_match = re.search(r'Platform:\s*([^\n]+)', markdown_text)
    if platform_match:
        result["platform"] = platform_match.group(1).strip()
    
    # Extract overall health
    health_match = re.search(r'Overall Assessment:\s*(\w+)', markdown_text)
    if health_match:
        result["overall_health"] = health_match.group(1)
    
    # Extract category scores from Executive Summary table
    category_pattern = re.compile(r'\|\s*\*\*(\w+)\*\*\s*\|\s*[✅❌⚠️]+\s*\|\s*(\d+)/100\s*\|')
    for match in category_pattern.finditer(markdown_text):
        cat_name = match.group(1).lower().replace(" ", "_")
        result["categories"][cat_name] = int(match.group(2))
    
    # Extract tool inventory
    tools_match = re.search(r'\|\s*\*\*Total Tools Registered\s*\*\*\s*\|\s*(\d+)\s*\|', markdown_text)
    if tools_match:
        result["tools"]["total_registered"] = int(tools_match.group(1))
    
    # Extract top executed tools
    top_exec_match = re.search(r'\*\*Top Executed\s*\*\*\s*\|\s*([^\n]+)', markdown_text)
    if top_exec_match:
        top_tools = top_exec_match.group(1).split(',')
        for tool in top_tools:
            tool_match = re.match(r'(\w+)\s*\((\d+)\s*calls?,\s*(\d+)%\s*success\)', tool.strip())
            if tool_match:
                result["tools"]["top_executed"].append({
                    "name": tool_match.group(1),
                    "calls": int(tool_match.group(2)),
                    "success_rate": int(tool_match.group(3))
                })
    
    # Extract tool issues
    issue_pattern = re.compile(r'\|\s*\*\*([^\|]+)\*\*\s*\|\s*(\w+)\s*\|\s*([^\|]+)\|')
    for match in issue_pattern.finditer(markdown_text):
        if "Missing Dependencies" in match.group(1):
            result["tools"]["issues"].append({
                "type": "missing_dependency",
                "severity": match.group(2).lower(),
                "description": match.group(3).strip()
            })
    
    # Extract security status
    if re.search(r'Localhost Ports.*✅ SECURE', markdown_text):
        result["security"]["network_secure"] = True
    if re.search(r'sudo Access.*❌ DENIED', markdown_text):
        result["security"]["sudo_access"] = False
    elif re.search(r'sudo Access.*✅', markdown_text):
        result["security"]["sudo_access"] = True
    if re.search(r'SUID Binaries.*✅ NONE', markdown_text):
        result["security"]["suid_binaries"] = False
    
    # Extract memory info
    agemem_match = re.search(r'AgeMem.*?(\d+(?:\.\d+)?)%', markdown_text)
    if agemem_match:
        result["memory"]["agemem_percent"] = float(agemem_match.group(1))
    
    mel_match = re.search(r'MEL VectorStore.*?(\d+)\s*heuristics', markdown_text)
    if mel_match:
        result["memory"]["mel_heuristics"] = int(mel_match.group(1))
    
    # Extract CCI specialists
    specialist_pattern = re.compile(r'\|\s*(\w+[-\w]*)\s*\|\s*✅\s*AVAILABLE\s*\|')
    for match in specialist_pattern.finditer(markdown_text):
        result["cci"]["specialists"].append(match.group(1))
    
    # Extract missing CCI docs
    missing_docs_match = re.search(r'Missing Documentation:\s*([^\n]+)', markdown_text)
    if missing_docs_match:
        docs = missing_docs_match.group(1).split(',')
        result["cci"]["missing_docs"] = [d.strip() for d in docs]
    
    # Extract consultation status
    if re.search(r'Consultation.*VERIFIED.*WORKING', markdown_text):
        result["consultation"]["working"] = True
    
    provider_match = re.search(r'\|\s*\*\*Provider\s*\*\*\s*\|\s*(\w+[-\w]*)\s*\|', markdown_text)
    if provider_match:
        result["consultation"]["provider"] = provider_match.group(1)
    
    # Extract resources
    cpu_match = re.search(r'\|\s*\*\*CPU Cores\s*\*\*\s*\|\s*(\d+)\s*\|', markdown_text)
    if cpu_match:
        result["resources"]["cpu_cores"] = int(cpu_match.group(1))
    
    mem_total_match = re.search(r'\|\s*\*\*Memory Total\s*\*\*\s*\|\s*(\d+)\s*GB\s*\|', markdown_text)
    if mem_total_match:
        result["resources"]["memory_total_gb"] = int(mem_total_match.group(1))
    
    mem_used_match = re.search(r'\|\s*\*\*Memory.*?\*\*\s*\|\s*(\d+(?:\.\d+)?)\s*%\s*\|', markdown_text)
    if mem_used_match:
        result["resources"]["memory_used_percent"] = float(mem_used_match.group(1))
    
    disk_match = re.search(r'\|\s*\*\*Disk.*?\*\*\s*\|\s*(\d+(?:\.\d+)?)\s*%\s*(?:FULL)?', markdown_text)
    if disk_match:
        result["resources"]["disk_root_percent"] = float(disk_match.group(1))
    
    # Extract findings by severity
    critical_section = re.search(r'### Critical Findings\s*\|[^\|]+\|[^\|]+\|\s*\|\s*\d+\s*\|\s*([^\|]+)\|', markdown_text)
    if critical_section:
        finding = critical_section.group(1).strip()
        if finding:
            result["findings"]["critical"].append(finding)
    
    medium_section = re.search(r'### Medium Findings\s*\|[^\|]+\|[^\|]+\|\s*\|\s*\d+\s*\|\s*([^\|]+)\|', markdown_text)
    if medium_section:
        finding = medium_section.group(1).strip()
        if finding:
            result["findings"]["medium"].append(finding)
    
    low_section = re.search(r'### Low Findings\s*\|[^\|]+\|[^\|]+\|\s*\|\s*\d+\s*\|\s*([^\|]+)\|', markdown_text)
    if low_section:
        finding = low_section.group(1).strip()
        if finding:
            result["findings"]["low"].append(finding)
    
    # Extract recommendations
    rec_matches = re.findall(r'\d+\.\s*\*\*([^*]+)\*\*\s*-\s*([^*]+)', markdown_text)
    for rec in rec_matches:
        result["recommendations"].append({
            "action": rec[0].strip(),
            "detail": rec[1].strip()
        })
    
    return result

def main():
    try:
        input_data = sys.stdin.read()
        
        if not input_data.strip():
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "No input data provided"
            }))
            return
        
        # Try to parse as JSON first (in case wrapped)
        try:
            parsed = json.loads(input_data)
            if isinstance(parsed, dict) and 'input_md' in parsed:
                markdown_text = parsed['input_md']
            elif isinstance(parsed, dict) and 'markdown' in parsed:
                markdown_text = parsed['markdown']
            else:
                markdown_text = input_data
        except json.JSONDecodeError:
            markdown_text = input_data
        
        result = parse_audit_report(markdown_text)
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(result, indent=2),
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
