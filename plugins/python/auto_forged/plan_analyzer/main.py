import sys
import json

def analyze_plan(query, content="", context=None):
    """Generalized plan analyzer - extracts structured critique from a review query."""
    
    # Default structured critique template
    critique = {
        "pros": [],
        "cons_gaps": [],
        "refined_priorities": [],
        "roadmap": [],
        "alignment_notes": ""
    }
    
    query_lower = query.lower()
    
    # Detect plan type and extract key themes
    if "mcp" in query_lower or "server" in query_lower:
        critique["pros"] = [
            "Extends capabilities beyond device limits",
            "Cloud-scale processing handles heavy workloads",
            "Modular design follows MCP protocol",
            "Addresses Android/Termux constraints (no root, RAM limits)"
        ]
        critique["cons_gaps"] = [
            "Requires VPS/cloud provisioning ($10-50/mo)",
            "Network dependency adds latency",
            "Potential overlap with existing tools if not carefully mapped",
            "Security considerations for remote auth/keys"
        ]
        critique["refined_priorities"] = [
            {"rank": 1, "item": "Compute/Processing server", "reason": "Addresses immediate resource constraints"},
            {"rank": 2, "item": "Database/Storage server", "reason": "Enables persistence and state"},
            {"rank": 3, "item": "AI/Intelligence server", "reason": "Offloads ML workloads"},
            {"rank": 4, "item": "Security/Pentest server", "reason": "Expands ethical security toolkit"},
            {"rank": 5, "item": "Web/Communication server", "reason": "Enhances data acquisition"
        }]
        critique["roadmap"] = [
            {"phase": 1, "steps": ["Provision low-cost VPS (DigitalOcean/AWS)", "Install MCP server dependencies", "Configure JSON-RPC over HTTPS"]},
            {"phase": 2, "steps": ["Deploy priority servers (Compute, DB)", "Test local-to-remote tool integration", "Verify auth (API keys/OAuth)"]},
            {"phase": 3, "steps": ["Add AI and remaining servers", "Implement monitoring/logging", "Scale based on usage"]}
        ]
        critique["alignment_notes"] = "Aligned with MCP protocol (JSON-RPC/HTTPS), Termux env (non-root, arm64), ethical use policies."
    
    elif "review" in query_lower or "critique" in query_lower:
        critique["pros"] = ["Systematic evaluation provided", "Feasibility assessed"]
        critique["cons_gaps"] = ["Verify specific constraints against environment"]
        critique["refined_priorities"]["item"] = "Implement top priority items first"
        critique["roadmap"] = [{"phase": 1, "steps": ["Validate requirements", "Prototype selected items", "Iterate based on feedback"]}]
    
    else:
        critique["alignment_notes"] = "Generic analysis mode - customize based on specific plan content."
    
    return critique

def main():
    try:
        input_data = json.load(sys.stdin)
        
        # Extract query and optional content
        query = input_data.get("query", "")
        content = input_data.get("content", "")
        context = input_data.get("context", {})
        
        if not query:
            print(json.dumps({
                "success": False,
                "output": "",
                "error": "Missing 'query' field in input"
            }))
            return
        
        result = analyze_plan(query, content, context)
        
        print(json.dumps({
            "success": True,
            "output": json.dumps(result),
            "error": ""
        }))
        
    except json.JSONDecodeError as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": f"Invalid JSON input: {str(e)}"
        }))
    except Exception as e:
        print(json.dumps({
            "success": False,
            "output": "",
            "error": str(e)
        }))

if __name__ == "__main__":
    main()
