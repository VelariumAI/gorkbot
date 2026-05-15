package harness

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func evaluateAssertion(artifact Artifact, assertion Assertion) Result {
	result := Result{
		AssertionID: assertion.ID,
		Severity:    assertion.Severity,
		Message:     assertion.Message,
		Remediation: append([]string(nil), assertion.Remediation...),
		Status:      StatusPass,
	}
	if result.Message == "" {
		result.Message = fmt.Sprintf("assertion %s failed", assertion.ID)
	}

	switch assertion.Type {
	case AssertionTypeRegexForbid:
		re, err := regexp.Compile(assertion.Condition)
		if err != nil {
			return invalidResult(result, "invalid_regex", err.Error())
		}
		if re.MatchString(artifact.Content) {
			return failedResult(result, "regex_forbidden_match", "forbidden regex matched")
		}
		result.Evidence = []Evidence{{Kind: "regex", Value: "forbidden pattern not present"}}
		return result

	case AssertionTypeRegexRequire:
		re, err := regexp.Compile(assertion.Condition)
		if err != nil {
			return invalidResult(result, "invalid_regex", err.Error())
		}
		if !re.MatchString(artifact.Content) {
			return failedResult(result, "regex_required_missing", "required regex did not match")
		}
		result.Evidence = []Evidence{{Kind: "regex", Value: "required pattern present"}}
		return result

	case AssertionTypeStringForbid:
		if strings.Contains(artifact.Content, assertion.Condition) {
			return failedResult(result, "string_forbidden_match", "forbidden string present")
		}
		result.Evidence = []Evidence{{Kind: "string", Value: "forbidden string not present"}}
		return result

	case AssertionTypeStringRequire:
		if !strings.Contains(artifact.Content, assertion.Condition) {
			return failedResult(result, "string_required_missing", "required string missing")
		}
		result.Evidence = []Evidence{{Kind: "string", Value: "required string present"}}
		return result

	case AssertionTypeMaxLength:
		maxLen, err := strconv.Atoi(assertion.Condition)
		if err != nil || maxLen < 0 {
			return invalidResult(result, "invalid_max_length", "condition must be a non-negative integer")
		}
		if len(artifact.Content) > maxLen {
			return failedResult(result, "max_length_exceeded", fmt.Sprintf("content length %d exceeds max %d", len(artifact.Content), maxLen))
		}
		result.Evidence = []Evidence{{Kind: "length", Value: fmt.Sprintf("%d <= %d", len(artifact.Content), maxLen)}}
		return result

	case AssertionTypeRequiredMetadata:
		key := normalizeMetadataKey(assertion.Condition)
		if key == "" {
			return invalidResult(result, "invalid_metadata_key", "required metadata key missing")
		}
		meta := normalizedMetadata(artifact.Metadata)
		if _, ok := meta[key]; !ok {
			return failedResult(result, "required_metadata_missing", "required metadata key missing")
		}
		result.Evidence = []Evidence{{Kind: "metadata", Value: "required metadata key present"}}
		return result

	case AssertionTypeForbiddenMetadataKey:
		key := normalizeMetadataKey(assertion.Condition)
		if key == "" {
			return invalidResult(result, "invalid_metadata_key", "forbidden metadata key missing")
		}
		meta := normalizedMetadata(artifact.Metadata)
		if _, ok := meta[key]; ok {
			return failedResult(result, "forbidden_metadata_present", "forbidden metadata key present")
		}
		result.Evidence = []Evidence{{Kind: "metadata", Value: "forbidden metadata key not present"}}
		return result
	}

	result.Status = StatusUnsupported
	result.ReasonCode = "unsupported_assertion"
	result.Evidence = []Evidence{{Kind: "assertion_type", Value: string(assertion.Type)}}
	return result
}

// ValidateAssertionFixtures validates golden/poisoned fixtures for a single assertion.
func ValidateAssertionFixtures(assertion Assertion) error {
	norm := assertion.Normalized()
	if err := norm.Validate(); err != nil {
		return err
	}

	for i := range norm.Golden {
		artifact := Artifact{
			ID:      fmt.Sprintf("fixture-golden-%d", i),
			Kind:    ArtifactKindText,
			Content: norm.Golden[i],
		}
		result := evaluateAssertion(artifact, norm)
		if result.Status != StatusPass {
			return fmt.Errorf("%w: golden fixture %d did not pass (%s)", ErrInvalidAssertion, i, result.Status)
		}
	}

	for i := range norm.Poisoned {
		artifact := Artifact{
			ID:      fmt.Sprintf("fixture-poisoned-%d", i),
			Kind:    ArtifactKindText,
			Content: norm.Poisoned[i],
		}
		result := evaluateAssertion(artifact, norm)
		if result.Status == StatusPass {
			return fmt.Errorf("%w: poisoned fixture %d unexpectedly passed", ErrInvalidAssertion, i)
		}
		if norm.Severity == SeverityHardFail && result.Status != StatusFail {
			return fmt.Errorf("%w: poisoned fixture %d expected fail, got %s", ErrInvalidAssertion, i, result.Status)
		}
		if norm.Severity != SeverityHardFail && result.Status != StatusWarn {
			return fmt.Errorf("%w: poisoned fixture %d expected warn, got %s", ErrInvalidAssertion, i, result.Status)
		}
	}

	return nil
}

func invalidResult(base Result, reasonCode, detail string) Result {
	base.Status = StatusInvalid
	base.ReasonCode = reasonCode
	base.Evidence = []Evidence{{Kind: "error", Value: detail}}
	return base
}

func failedResult(base Result, reasonCode, detail string) Result {
	if base.Severity == SeverityHardFail {
		base.Status = StatusFail
	} else {
		base.Status = StatusWarn
	}
	base.ReasonCode = reasonCode
	base.Evidence = []Evidence{{Kind: "check", Value: detail}}
	return base
}

func normalizedMetadata(meta map[string]string) map[string]string {
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		key := normalizeMetadataKey(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}
