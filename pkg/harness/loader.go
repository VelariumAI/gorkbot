package harness

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultMaxHarnessDocumentBytes = 512 * 1024
	defaultMaxLoadedAssertions     = 256
)

type harnessDocument struct {
	Assertions []harnessAssertion `yaml:"assertions"`
}

type harnessAssertion struct {
	ID          string            `yaml:"id"`
	Scope       string            `yaml:"scope"`
	Severity    string            `yaml:"severity"`
	Type        string            `yaml:"type"`
	Condition   string            `yaml:"condition"`
	Message     string            `yaml:"message"`
	Remediation []string          `yaml:"remediation"`
	Owner       string            `yaml:"owner"`
	Version     string            `yaml:"version"`
	Metadata    map[string]string `yaml:"metadata"`
	Tests       harnessTests      `yaml:"tests"`
}

type harnessTests struct {
	Poisoned []string `yaml:"poisoned"`
	Golden   []string `yaml:"golden"`
}

// LoadAssertionsYAML loads assertions from a bounded YAML document.
func LoadAssertionsYAML(data []byte) ([]Assertion, error) {
	return loadAssertionsYAML(bytes.NewReader(data), defaultMaxHarnessDocumentBytes)
}

// LoadAssertionsYAMLReader loads assertions from a bounded reader.
func LoadAssertionsYAMLReader(r io.Reader, maxBytes int64) ([]Assertion, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxHarnessDocumentBytes
	}
	return loadAssertionsYAML(r, maxBytes)
}

func loadAssertionsYAML(r io.Reader, maxBytes int64) ([]Assertion, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrInvalidHarnessDocument)
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxHarnessDocumentBytes
	}

	limited := io.LimitReader(r, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: read failed: %v", ErrInvalidHarnessDocument, err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("%w: document exceeds %d bytes", ErrInvalidHarnessDocument, maxBytes)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)

	var doc harnessDocument
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: yaml parse failed: %v", ErrInvalidHarnessDocument, err)
	}
	if len(doc.Assertions) == 0 {
		return nil, fmt.Errorf("%w: no assertions", ErrInvalidHarnessDocument)
	}
	if len(doc.Assertions) > defaultMaxLoadedAssertions {
		return nil, fmt.Errorf("%w: %d > %d", ErrTooManyAssertions, len(doc.Assertions), defaultMaxLoadedAssertions)
	}

	out := make([]Assertion, 0, len(doc.Assertions))
	for i := range doc.Assertions {
		entry := doc.Assertions[i]
		a := Assertion{
			ID:          entry.ID,
			Scope:       entry.Scope,
			Severity:    Severity(strings.ToLower(strings.TrimSpace(entry.Severity))),
			Type:        AssertionType(strings.ToLower(strings.TrimSpace(entry.Type))),
			Condition:   entry.Condition,
			Message:     entry.Message,
			Remediation: entry.Remediation,
			Owner:       entry.Owner,
			Version:     entry.Version,
			Golden:      entry.Tests.Golden,
			Poisoned:    entry.Tests.Poisoned,
			Metadata:    entry.Metadata,
		}
		norm := a.Normalized()
		if err := norm.Validate(); err != nil {
			return nil, fmt.Errorf("%w: assertion %d: %v", ErrInvalidHarnessDocument, i, err)
		}
		out = append(out, norm)
	}
	return out, nil
}
