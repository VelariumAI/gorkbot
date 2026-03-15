import sys
import json
import re
from collections import defaultdict

def analyze_bash_patterns(audit_text):
    """Extract bash tool usage patterns and success metrics from audit report."""
    results = {
        "bash_tool_stats": {},
        "low_success_tools": [],
        "total_tool_calls": 0,
        "successful_calls": 0,
        "overall_success_rate": 0.0
    }
    
    # Pattern: tool_name (calls, success%) or tool_name %
    tool_pattern = re.compile(r'`?(\w+)`?\s*\((\d+)\s*calls?,\s*(\d+)%?\s*success\)'
    )
    
    # Find all tool call patterns
    tool_matches = tool_pattern.findall(audit_text)
    
    for tool_name, calls, success in tool_matches:
        call_count = int(calls)
        success_rate = int(success)
        results["bash_tool_stats"][tool_name] = {
            "calls": call_count,
            "success_rate": success_rate,
            "failures": call_count - int(call_count * success_rate / 100)
        }
        results["total_tool_calls"] += call_count
        results["successful_calls"] += int(call_count * success_rate / 100)
        
        if success_rate < 75:
            results["low_success_tools"].append({
                "tool": tool_name,
                "success_rate": success_rate,
                "calls": call_count
            })
    
    if results["total_tool_calls"] > 0:
        results["overall_success_rate"] = round(
            results["successful_calls"] / results["total_tool_calls"] * 100, 2
        )
    
    # Sort low success tools by severity (lowest rate first)
    results["low_success_tools"].sort(key=lambda x: x["success_rate"])
    
    return results

def main():
    try:
        input_data = json.loads(sys.stdin.read())
        audit_text = input_data.get("input_md", "") + input_data.get("text", "")
        
        if not audit_text:
            raise ValueError("No input text provided")
        
        analysis = analyze_bash_patterns(audit_text)
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(analysis, indent=2),
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
