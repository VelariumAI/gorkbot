package harness

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxAssertionIDLen         = 128
	maxAssertionScopeLen      = 128
	maxAssertionMessageLen    = 256
	maxAssertionOwnerLen      = 64
	maxAssertionVersionLen    = 32
	maxAssertionConditionLen  = 2048
	maxAssertionRemediations  = 16
	maxAssertionFixtureValues = 32
	maxFixtureValueLen        = 4096
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityHardFail Severity = "hard_fail"
)

var validSeverities = map[Severity]struct{}{
	SeverityInfo:     {},
	SeverityWarn:     {},
	SeverityHardFail: {},
}

type AssertionType string

const (
	AssertionTypeRegexForbid          AssertionType = "regex_forbid"
	AssertionTypeRegexRequire         AssertionType = "regex_require"
	AssertionTypeStringForbid         AssertionType = "string_forbid"
	AssertionTypeStringRequire        AssertionType = "string_require"
	AssertionTypeMaxLength            AssertionType = "max_length"
	AssertionTypeRequiredMetadata     AssertionType = "required_metadata"
	AssertionTypeForbiddenMetadataKey AssertionType = "forbidden_metadata_key"
)

var validAssertionTypes = map[AssertionType]struct{}{
	AssertionTypeRegexForbid:          {},
	AssertionTypeRegexRequire:         {},
	AssertionTypeStringForbid:         {},
	AssertionTypeStringRequire:        {},
	AssertionTypeMaxLength:            {},
	AssertionTypeRequiredMetadata:     {},
	AssertionTypeForbiddenMetadataKey: {},
}

// Assertion defines a deterministic check that can be evaluated on an artifact.
type Assertion struct {
	ID          string            `json:"id"`
	Scope       string            `json:"scope"`
	Severity    Severity          `json:"severity"`
	Type        AssertionType     `json:"type"`
	Condition   string            `json:"condition"`
	Message     string            `json:"message,omitempty"`
	Remediation []string          `json:"remediation,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Version     string            `json:"version,omitempty"`
	Golden      []string          `json:"golden,omitempty"`
	Poisoned    []string          `json:"poisoned,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (a Assertion) Normalized() Assertion {
	out := a
	out.ID = truncateString(strings.TrimSpace(out.ID), maxAssertionIDLen)
	out.Scope = strings.ToLower(truncateString(strings.TrimSpace(out.Scope), maxAssertionScopeLen))
	out.Condition = truncateString(strings.TrimSpace(out.Condition), maxAssertionConditionLen)
	out.Message = truncateString(strings.TrimSpace(out.Message), maxAssertionMessageLen)
	out.Owner = truncateString(strings.TrimSpace(out.Owner), maxAssertionOwnerLen)
	out.Version = truncateString(strings.TrimSpace(out.Version), maxAssertionVersionLen)
	out.Severity = Severity(strings.ToLower(strings.TrimSpace(string(out.Severity))))
	out.Type = AssertionType(strings.ToLower(strings.TrimSpace(string(out.Type))))

	out.Remediation = boundStringList(out.Remediation, maxAssertionRemediations, maxAssertionMessageLen)
	out.Golden = boundStringList(out.Golden, maxAssertionFixtureValues, maxFixtureValueLen)
	out.Poisoned = boundStringList(out.Poisoned, maxAssertionFixtureValues, maxFixtureValueLen)
	out.Metadata = trace.BoundMetadata(out.Metadata)
	return out
}

func (a Assertion) Validate() error {
	norm := a.Normalized()
	if norm.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidAssertion)
	}
	if norm.Scope == "" {
		return fmt.Errorf("%w: scope is required", ErrInvalidAssertion)
	}
	if _, ok := validSeverities[norm.Severity]; !ok {
		return fmt.Errorf("%w: invalid severity %q", ErrInvalidAssertion, norm.Severity)
	}
	if _, ok := validAssertionTypes[norm.Type]; !ok {
		return fmt.Errorf("%w: %q", ErrUnsupportedAssertion, norm.Type)
	}
	if norm.Condition == "" {
		return fmt.Errorf("%w: condition is required", ErrInvalidAssertion)
	}
	if norm.Message == "" {
		norm.Message = fmt.Sprintf("assertion %s failed", norm.ID)
	}

	switch norm.Type {
	case AssertionTypeRegexForbid, AssertionTypeRegexRequire:
		if _, err := regexp.Compile(norm.Condition); err != nil {
			return fmt.Errorf("%w: invalid regex for %q: %v", ErrInvalidAssertion, norm.ID, err)
		}
	case AssertionTypeMaxLength:
		n, err := strconv.Atoi(norm.Condition)
		if err != nil || n < 0 {
			return fmt.Errorf("%w: max_length condition must be a non-negative integer", ErrInvalidAssertion)
		}
	case AssertionTypeRequiredMetadata, AssertionTypeForbiddenMetadataKey:
		if normalizeMetadataKey(norm.Condition) == "" {
			return fmt.Errorf("%w: metadata key condition is required", ErrInvalidAssertion)
		}
	}
	return nil
}

func normalizeMetadataKey(raw string) string {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	for strings.Contains(key, "__") {
		key = strings.ReplaceAll(key, "__", "_")
	}
	return strings.Trim(key, "_")
}
