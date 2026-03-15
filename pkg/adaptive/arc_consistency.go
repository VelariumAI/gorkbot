package adaptive

import "strings"

// highRiskOps lists operation keywords that trigger a consistency check.
var highRiskOps = []string{
	"delete", "remove", "drop", "truncate", "format",
	"rm ", "rm -rf", "overwrite", "wipe", "kill",
	"shutdown", "reboot", "reset", "uninstall", "purge",
	"destroy", "nuke", "erase", "flush",
}

// synonymMap maps canonical destructive terms to their common synonyms,
// used to detect when the same operation is expressed differently.
var synonymMap = map[string][]string{
	"delete": {"remove", "erase", "drop", "wipe", "purge", "destroy"},
	"stop":   {"kill", "terminate", "halt", "shutdown"},
	"create": {"make", "build", "generate", "new", "add"},
	"update": {"modify", "change", "edit", "patch", "alter"},
	"reset":  {"revert", "restore", "undo", "rollback"},
}

// ReframedEvaluator checks whether a high-risk operation is consistent
// across synonym substitution. No LLM call required — pure Go.
type ReframedEvaluator struct{}

// IsHighRisk returns true if the prompt contains a destructive operation keyword.
func (e *ReframedEvaluator) IsHighRisk(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, op := range highRiskOps {
		if strings.Contains(lower, op) {
			return true
		}
	}
	return false
}

// CheckConsistency verifies the operation intent is unambiguous by checking
// that destructive synonyms are used consistently with their canonical form.
// Returns true when the intent is clear and consistent.
func (e *ReframedEvaluator) CheckConsistency(prompt string) bool {
	lower := strings.ToLower(prompt)
	for canonical, syns := range synonymMap {
		hasCanonical := strings.Contains(lower, canonical)
		for _, syn := range syns {
			if strings.Contains(lower, syn) && !hasCanonical {
				// Synonym found but canonical term not present — ambiguous intent.
				return false
			}
		}
	}
	return true
}
