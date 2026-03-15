package tools

// NormalizeToolParams auto-corrects common LLM parameter naming mistakes
// before tool validation. This reduces failures when smaller models use
// slightly different parameter names.
func NormalizeToolParams(toolName string, params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return params
	}

	// Copy the map so we never mutate the original
	result := make(map[string]interface{}, len(params))
	for k, v := range params {
		result[k] = v
	}

	// Helper: rename alias → canonical if alias present but canonical absent
	rename := func(alias, canonical string) {
		if _, hasCanonical := result[canonical]; hasCanonical {
			return // never overwrite an existing canonical key
		}
		if val, hasAlias := result[alias]; hasAlias {
			result[canonical] = val
			delete(result, alias)
		}
	}

	// ---- Universal aliases (always applied) ----

	// "file" → "path" for file-related tools
	switch toolName {
	case "read_file", "write_file", "file_info", "parse_document", "delete_file",
		"edit_file", "multi_edit_file", "search_files":
		rename("file", "path")
	}

	// "directory" / "dir" → "path" for directory tools
	switch toolName {
	case "list_directory":
		rename("directory", "path")
		rename("dir", "path")
	}

	// "cmd" → "command" for all shell execution tools
	switch toolName {
	case "bash", "privileged_execute", "structured_bash":
		rename("cmd", "command")
	}

	// "text" → "content" for write_file
	switch toolName {
	case "write_file":
		rename("text", "content")
	}

	// "message" → "prompt" for consultation
	switch toolName {
	case "consultation", "consult_gemini":
		rename("message", "prompt")
	}

	// "lang" → "language" for execute_code
	switch toolName {
	case "execute_code":
		rename("lang", "language")
	}

	// "query" → "pattern" for grep/search tools
	switch toolName {
	case "grep_content", "search_files":
		rename("query", "pattern")
	}

	// "q" → "query" for web search tools
	switch toolName {
	case "web_search", "web_fetch", "http_request":
		rename("q", "query")
	}

	// Universal: "src" → "source", "dest" → "destination"
	rename("src", "source")
	rename("dest", "destination")

	return result
}

// NormalizeToolName normalizes common tool name variations to canonical names (exported).
func NormalizeToolName(name string) string {
	return normalizeToolName(name)
}

// normalizeToolName normalizes common tool name variations to canonical names.
func normalizeToolName(name string) string {
	switch name {
	case "grep":
		return "grep_content"
	case "search":
		return "search_files"
	case "ls":
		return "list_directory"
	case "cat":
		return "read_file"
	case "exec":
		return "bash"
	case "run":
		return "bash"
	case "shell":
		return "bash"
	// privileged_execute aliases
	case "privileged_bash", "sudo_bash", "sudo_exec", "priv_exec", "priv_bash":
		return "privileged_execute"
	// structured_bash aliases
	case "structured_exec", "structured_shell", "struct_bash", "parsed_bash":
		return "structured_bash"
	default:
		return name
	}
}
