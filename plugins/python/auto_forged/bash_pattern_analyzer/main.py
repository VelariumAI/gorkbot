import sys
import json
import re
from collections import Counter, defaultdict

def analyze_bash_patterns(input_data):
    """Analyze bash command patterns from input data."""
    
    # Extract tool usage information from markdown/text input
    results = {
        "total_bash_calls": 0,
        "success_rate": 0.0,
        "failure_rate": 0.0,
        "pattern_detected": False,
        "recommendations": [],
        "risk_level": "LOW"
    }
    
    # Convert input to string if needed
    if isinstance(input_data, dict):
        text = json.dumps(input_data)
    else:
        text = str(input_data)
    
    # Detect bash tool usage patterns
    bash_patterns = [
        r'"tool":\s*"bash"',
        r'tool.*bash',
        r'bash.*\d+.*calls',
        r'bash.*\d+%.*success',
    ]
    
    found_patterns = []
    for pattern in bash_patterns:
        matches = re.findall(pattern, text, re.IGNORECASE)
        if matches:
            found_patterns.extend(matches)
    
    # Extract success rate if present
    success_rate_match = re.search(r'bash.*?(\d+)%.*?success', text, re.IGNORECASE)
    if success_rate_match:
        results["success_rate"] = float(success_rate_match.group(1))
        results["failure_rate"] = 100.0 - results["success_rate"]
    
    # Extract total calls if present
    calls_match = re.search(r'bash.*?(\d+)\s+calls', text, re.IGNORECASE)
    if calls_match:
        results["total_bash_calls"] = int(calls_match.group(1))
    
    # Determine if pattern is concerning
    if results["success_rate"] > 0 and results["success_rate"] < 75:
        results["pattern_detected"] = True
        results["risk_level"] = "MEDIUM"
        results["recommendations"].append("bash tool has low success rate - review failed invocations")
    
    if results["total_bash_calls"] > 100:
        results["recommendations"].append("High bash usage detected - consider optimizing with specialized tools")
    
    if results["failure_rate"] > 30:
        results["risk_level"] = "HIGH"
        results["recommendations"].append("Critical: bash failure rate exceeds 30% - immediate review needed")
    
    # Check for repeated bash patterns (potential loop or excessive use)
    bash_repeated = re.findall(r'("tool":\s*"bash")', text)
    if len(bash_repeated) > 3:
        results["pattern_detected"] = True
        results["recommendations"].append(f"Detected {len(bash_repeated)} repeated bash tool calls - possible inefficient pattern")
    
    return results

def main():
    try:
        # Read JSON from stdin
        input_str = sys.stdin.read()
        if not input_str.strip():
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "No input data provided"
            }))
            return
        
        # Parse input (try JSON first, fallback to raw text)
        try:
            input_data = json.loads(input_str)
        except json.JSONDecodeError:
            input_data = input_str
        
        # Analyze patterns
        analysis = analyze_bash_patterns(input_data)
        
        # Build output
        output = {
            "pattern_analysis": analysis,
            "summary": f"Bash tool: {analysis['total_bash_calls']} calls, {analysis['success_rate']}% success rate"
        }
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(output),
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
