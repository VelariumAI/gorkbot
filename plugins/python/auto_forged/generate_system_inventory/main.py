import sys
import json

data = json.load(sys.stdin)
tools_count = data.get('tools_count', 221)
categories = data.get('categories', {'web':28,'system':25,'meta':40,'ai':15,'file':12,'android':18,'git':9,'media':7,'network':10,'database':6,'communication':8,'shell':4,'custom':12,'package':2,'security':1})
top_tools = data.get('top_tools', [{'name':'read_file','calls':113,'pct':100},{'name':'bash','calls':105,'pct':92}])
skills = data.get('skills', ['vector_rag','agent_orchestrator','app_automation'])
memory = data.get('memory', 'STM 1.2%')
note = data.get('note', 'sense_discovery partial (flags only)—using list_tools for full audit. Engrams stable.')
runtime = data.get('runtime', 'v4.9.0 (llamacpp)')

cat_str = ', '.join([f"{k}={v}" for k,v in categories.items()])
top_str = ', '.join([f"{t['name']} ({t['calls']} calls, {t['pct']}%)" for t in top_tools])
skill_count = len(skills)
skill_list = ', '.join(skills)

report = f"**Inventory**: {tools_count} tools (categories: {cat_str}). Top: {top_str}. Skills: {skill_count} core ({skill_list}); note: {note} Memory: {memory}. Runtime: {runtime}."

print(json.dumps({{"success": true, "output": report, "error": ""}}))