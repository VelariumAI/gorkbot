package puteradapter

import (
	"regexp"
	"strings"
)

var kvKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,255}$`)

// PuterKVKey is a validated key-value namespace artifact.
type PuterKVKey struct {
	value string
}

func (k PuterKVKey) String() string {
	return k.value
}

// ValidatePuterKVKey validates key syntax and enforces namespace constraints.
func ValidatePuterKVKey(raw string) (PuterKVKey, Decision) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" || containsControlRune(key) || !kvKeyPattern.MatchString(key) {
		return PuterKVKey{}, Decision{Allowed: false, ReasonCode: ReasonKVNamespaceBlocked}
	}
	return PuterKVKey{value: key}, Decision{Allowed: true, ReasonCode: ReasonAllowed}
}
