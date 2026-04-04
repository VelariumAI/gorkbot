package tools

import (
	"testing"
)

// TestRuleEngine_ParamMatching tests per-parameter rule matching (Task 5.4)
func TestRuleEngine_ParamMatching(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		params  map[string]interface{}
		want    bool
	}{
		{
			name:    "param path glob prefix",
			pattern: "write_file(path:/etc/*)",
			tool:    "write_file",
			params: map[string]interface{}{
				"path":    "/etc/passwd",
				"content": "data",
			},
			want: true,
		},
		{
			name:    "param path glob no match",
			pattern: "write_file(path:/etc/*)",
			tool:    "write_file",
			params: map[string]interface{}{
				"path":    "/home/user/file.txt",
				"content": "data",
			},
			want: false,
		},
		{
			name:    "param filename glob",
			pattern: "delete_file(filename:*.tmp)",
			tool:    "delete_file",
			params: map[string]interface{}{
				"filename": "cache.tmp",
				"force":    true,
			},
			want: true,
		},
		{
			name:    "param filename no match",
			pattern: "delete_file(filename:*.tmp)",
			tool:    "delete_file",
			params: map[string]interface{}{
				"filename": "important.txt",
				"force":    true,
			},
			want: false,
		},
		{
			name:    "param missing returns false",
			pattern: "write_file(path:/etc/*)",
			tool:    "write_file",
			params: map[string]interface{}{
				"content": "data",
			},
			want: false,
		},
		{
			name:    "param glob suffix",
			pattern: "bash(cmd:*rm*)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "rm -rf /tmp",
			},
			want: true,
		},
		{
			name:    "param glob contains",
			pattern: "bash(cmd:*dangerous*)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "some dangerous command",
			},
			want: true,
		},
		{
			name:    "param exact match",
			pattern: "bash(cmd:git status)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "git status",
			},
			want: true,
		},
		{
			name:    "param exact match case sensitive",
			pattern: "bash(cmd:git status)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "GIT STATUS",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRule(tt.pattern, tt.tool, tt.params)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, params) = %v, want %v", tt.pattern, tt.tool, got, tt.want)
			}
		})
	}
}

// TestMatchParamRule_ArrayParams tests per-parameter matching with array params
func TestMatchParamRule_ArrayParams(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		params  map[string]interface{}
		want    bool
	}{
		{
			name:    "array param matches one element",
			pattern: "process(paths:*.py)",
			tool:    "process",
			params: map[string]interface{}{
				"paths": []string{"script.py", "data.json", "config.yaml"},
			},
			want: true,
		},
		{
			name:    "array param no match",
			pattern: "process(paths:*.py)",
			tool:    "process",
			params: map[string]interface{}{
				"paths": []string{"data.json", "config.yaml"},
			},
			want: false,
		},
		{
			name:    "empty array param",
			pattern: "process(paths:*.py)",
			tool:    "process",
			params: map[string]interface{}{
				"paths": []string{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRule(tt.pattern, tt.tool, tt.params)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, params) = %v, want %v", tt.pattern, tt.tool, got, tt.want)
			}
		})
	}
}

// TestRuleEngine_BackwardCompat tests backward compatibility with old pattern syntax
func TestRuleEngine_BackwardCompat(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		params  map[string]interface{}
		want    bool
	}{
		{
			name:    "flat glob prefix (backward compat)",
			pattern: "bash(git *)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "git status",
			},
			want: true,
		},
		{
			name:    "domain match (backward compat)",
			pattern: "web_fetch(domain:github.com)",
			tool:    "web_fetch",
			params: map[string]interface{}{
				"url": "https://github.com/user/repo",
			},
			want: true,
		},
		{
			name:    "no pattern matches any call",
			pattern: "write_file",
			tool:    "write_file",
			params: map[string]interface{}{
				"path":    "/etc/passwd",
				"content": "any",
			},
			want: true,
		},
		{
			name:    "tool name must match exactly",
			pattern: "bash(git *)",
			tool:    "sh",
			params: map[string]interface{}{
				"cmd": "git status",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRule(tt.pattern, tt.tool, tt.params)
			if got != tt.want {
				t.Errorf("matchRule(%q, %q, params) = %v, want %v", tt.pattern, tt.tool, got, tt.want)
			}
		})
	}
}

// TestRuleEngine_RealWorldScenarios tests realistic permission scenarios
func TestRuleEngine_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		params  map[string]interface{}
		want    bool
		desc    string
	}{
		{
			name:    "deny tmp files",
			pattern: "delete_file(path:/tmp/*)",
			tool:    "delete_file",
			params: map[string]interface{}{
				"path": "/tmp/cache/file.txt",
			},
			want: true,
			desc: "allow deletion of /tmp files",
		},
		{
			name:    "deny system files",
			pattern: "write_file(path:/etc/*)",
			tool:    "write_file",
			params: map[string]interface{}{
				"path": "/etc/shadow",
			},
			want: true,
			desc: "block writing to /etc",
		},
		{
			name:    "allow home dir",
			pattern: "write_file(path:~/documents/*)",
			tool:    "write_file",
			params: map[string]interface{}{
				"path": "~/documents/report.pdf",
			},
			want: true,
			desc: "allow writing to home directory",
		},
		{
			name:    "restrict dangerous commands",
			pattern: "bash(cmd:*rm -rf*)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "rm -rf /var",
			},
			want: true,
			desc: "catch dangerous rm patterns",
		},
		{
			name:    "allow safe git operations",
			pattern: "bash(cmd:git *)",
			tool:    "bash",
			params: map[string]interface{}{
				"cmd": "git commit -m 'test'",
			},
			want: true,
			desc: "allow git commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchRule(tt.pattern, tt.tool, tt.params)
			if got != tt.want {
				t.Errorf("%s: matchRule(%q, %q, params) = %v, want %v",
					tt.desc, tt.pattern, tt.tool, got, tt.want)
			}
		})
	}
}

// BenchmarkMatchParamRule benchmarks per-parameter rule matching
func BenchmarkMatchParamRule(b *testing.B) {
	pattern := "write_file(path:/etc/*)"
	tool := "write_file"
	params := map[string]interface{}{
		"path":    "/etc/passwd",
		"content": "test data",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchRule(pattern, tool, params)
	}
}

// BenchmarkMatchRuleBackwardCompat benchmarks backward compatible flat matching
func BenchmarkMatchRuleBackwardCompat(b *testing.B) {
	pattern := "bash(git *)"
	tool := "bash"
	params := map[string]interface{}{
		"cmd": "git status",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchRule(pattern, tool, params)
	}
}
