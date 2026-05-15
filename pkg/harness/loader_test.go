package harness

import (
	"strings"
	"testing"
)

func TestYAMLLoaderValidDocument(t *testing.T) {
	doc := `
assertions:
  - id: shell.no_recursive_rm
    scope: tool.shell
    severity: hard_fail
    type: regex_forbid
    condition: "rm\\s+-rf\\s+/"
    message: "Unsafe destructive shell command"
    remediation:
      - "Use a scoped path or require explicit approval"
    owner: security
    version: "1"
    tests:
      poisoned:
        - "rm -rf /"
      golden:
        - "rm -rf ./build/tmp"
`
	assertions, err := LoadAssertionsYAML([]byte(doc))
	if err != nil {
		t.Fatalf("expected valid document, got %v", err)
	}
	if len(assertions) != 1 {
		t.Fatalf("expected one assertion, got %d", len(assertions))
	}
	if assertions[0].ID != "shell.no_recursive_rm" {
		t.Fatalf("unexpected assertion id %q", assertions[0].ID)
	}
}

func TestYAMLLoaderMalformedDocument(t *testing.T) {
	doc := `
assertions:
  - id: x
    scope: tool.shell
    severity: hard_fail
    type: regex_forbid
    condition: "["
`
	_, err := LoadAssertionsYAML([]byte(doc))
	if err == nil {
		t.Fatalf("expected malformed yaml validation error")
	}
}

func TestYAMLLoaderRejectsUnknownIncludeField(t *testing.T) {
	doc := `
assertions:
  - id: x
    scope: tool.shell
    severity: hard_fail
    type: string_forbid
    condition: "rm -rf /"
    include: other.yaml
`
	_, err := LoadAssertionsYAML([]byte(doc))
	if err == nil {
		t.Fatalf("expected include-style field to be rejected")
	}
}

func TestYAMLLoaderNoEnvInterpolation(t *testing.T) {
	doc := `
assertions:
  - id: env.literal
    scope: tool.shell
    severity: warn
    type: string_require
    condition: "${HOME}"
    message: "literal env token"
`
	assertions, err := LoadAssertionsYAML([]byte(doc))
	if err != nil {
		t.Fatalf("expected valid yaml, got %v", err)
	}
	if len(assertions) != 1 {
		t.Fatalf("expected one assertion, got %d", len(assertions))
	}
	if assertions[0].Condition != "${HOME}" {
		t.Fatalf("expected literal env token preserved, got %q", assertions[0].Condition)
	}
}

func TestYAMLLoaderBoundsDocumentSize(t *testing.T) {
	big := strings.Repeat("a", 1024)
	doc := "assertions:\n  - id: x\n    scope: tool.shell\n    severity: hard_fail\n    type: string_forbid\n    condition: \"" + big + "\"\n"

	_, err := LoadAssertionsYAMLReader(strings.NewReader(doc), 64)
	if err == nil {
		t.Fatalf("expected size bound error")
	}
}
