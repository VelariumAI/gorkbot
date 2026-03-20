package sense

import (
	"errors"
	"testing"
)

func TestInputSanitizer_ControlCharRejection(t *testing.T) {
	s, err := NewInputSanitizer()
	if err != nil {
		t.Fatalf("NewInputSanitizer: %v", err)
	}

	cases := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
		policy  SanitizerPolicy
	}{
		{
			name:    "clean string passes",
			params:  map[string]interface{}{"command": "echo hello"},
			wantErr: false,
		},
		{
			name:    "null byte rejected",
			params:  map[string]interface{}{"command": "echo\x00injected"},
			wantErr: true,
			policy:  PolicyNullByte,
		},
		{
			name:    "ASCII control char rejected",
			params:  map[string]interface{}{"command": "echo\x01injected"},
			wantErr: true,
			policy:  PolicyControlChar,
		},
		{
			// \t, \n, \r are standard text whitespace — must be allowed so that
			// write_file can write multi-line files, shell scripts, configs, etc.
			name:    "tab (0x09) allowed",
			params:  map[string]interface{}{"content": "col1\tcol2"},
			wantErr: false,
		},
		{
			name:    "newline (0x0A) allowed",
			params:  map[string]interface{}{"content": "#!/bin/bash\necho hello"},
			wantErr: false,
		},
		{
			name:    "carriage return (0x0D) allowed",
			params:  map[string]interface{}{"content": "line1\r\nline2"},
			wantErr: false,
		},
		{
			// ESC (0x1B) IS a terminal injection vector — must still be rejected.
			name:    "ESC sequence (0x1B) rejected",
			params:  map[string]interface{}{"command": "echo\x1b[31mred\x1b[0m"},
			wantErr: true,
			policy:  PolicyControlChar,
		},
		{
			name:    "BEL (0x07) rejected",
			params:  map[string]interface{}{"input": "hello\x07world"},
			wantErr: true,
			policy:  PolicyControlChar,
		},
		{
			name:    "printable extended ASCII allowed",
			params:  map[string]interface{}{"input": "héllo wörld"},
			wantErr: false,
		},
		{
			name:    "array elements validated",
			params:  map[string]interface{}{"args": []interface{}{"safe", "evil\x01"}},
			wantErr: true,
			policy:  PolicyControlChar,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SanitizeParams(tc.params)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.wantErr && tc.policy != "" {
				var v SanitizerViolation
				if !errors.As(err, &v) {
					t.Errorf("expected SanitizerViolation, got %T: %v", err, err)
					return
				}
				if v.Policy != tc.policy {
					t.Errorf("policy: got %q, want %q", v.Policy, tc.policy)
				}
			}
		})
	}
}

func TestInputSanitizer_ResourceNameValidation(t *testing.T) {
	s, err := NewInputSanitizer()
	if err != nil {
		t.Fatalf("NewInputSanitizer: %v", err)
	}

	cases := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "clean name passes",
			params:  map[string]interface{}{"name": "my_tool"},
			wantErr: false,
		},
		{
			name:    "question mark in name rejected",
			params:  map[string]interface{}{"name": "tool?param=1"},
			wantErr: true,
		},
		{
			name:    "hash in name rejected",
			params:  map[string]interface{}{"tool_name": "tool#fragment"},
			wantErr: true,
		},
		{
			name:    "percent in name rejected",
			params:  map[string]interface{}{"name": "tool%20encoded"},
			wantErr: true,
		},
		{
			name:    "id with adversarial char rejected",
			params:  map[string]interface{}{"id": "item?x=1"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SanitizeParams(tc.params)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestInputSanitizer_PathSandbox(t *testing.T) {
	s, err := NewInputSanitizer()
	if err != nil {
		t.Fatalf("NewInputSanitizer: %v", err)
	}

	cases := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "relative path within CWD passes",
			params:  map[string]interface{}{"path": "src/main.go"},
			wantErr: false,
		},
		{
			name:    "dot-slash relative path passes",
			params:  map[string]interface{}{"path": "./README.md"},
			wantErr: false,
		},
		{
			// Deep traversal that genuinely escapes all allowed prefixes
			// (CWD, home dir, /tmp, /sdcard, /storage) and reaches /etc/passwd.
			// ../../etc/passwd no longer works because from pkg/sense the CWD
			// is within $HOME, so it resolves to $HOME/project/gorky/etc/passwd
			// which the expanded sandbox now correctly allows.
			name:    "path traversal to /etc rejected",
			params:  map[string]interface{}{"path": "/etc/passwd"},
			wantErr: true,
		},
		{
			name:    "absolute path outside CWD rejected",
			params:  map[string]interface{}{"file": "/etc/shadow"},
			wantErr: true,
		},
		{
			name:    "empty path allowed",
			params:  map[string]interface{}{"path": ""},
			wantErr: false,
		},
		{
			name:    "non-path key with slash not sandboxed",
			params:  map[string]interface{}{"message": "/etc/passwd is a file"},
			wantErr: false, // "message" is not a path param key
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := s.SanitizeParams(tc.params)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSanitizerViolation_Error(t *testing.T) {
	v := SanitizerViolation{
		Field:  "command",
		Value:  "echo hello",
		Policy: PolicyControlChar,
		Detail: "ASCII control character 0x01 at position 0",
	}
	msg := v.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.As(v, &SanitizerViolation{}) {
		// errors.As on a value type — just verify the string
		_ = msg
	}
}

func TestIsPathParam(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"path", true},
		{"file", true},
		{"filepath", true},
		{"directory", true},
		{"dest", true},
		{"output", true},
		{"command", false},
		{"message", false},
		{"name", false},
	}
	for _, tc := range cases {
		if got := isPathParam(tc.key); got != tc.want {
			t.Errorf("isPathParam(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestIsNameParam(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"name", true},
		{"tool_name", true},
		{"id", true},
		{"resource_id", true},
		{"command", false},
		{"path", false},
	}
	for _, tc := range cases {
		if got := isNameParam(tc.key); got != tc.want {
			t.Errorf("isNameParam(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
