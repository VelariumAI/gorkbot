package harness

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultMaxAssertions = 256

type registryConfig struct {
	harnessID             string
	maxAssertions         int
	failClosedUnsupported bool
}

// Option configures registry behavior.
type Option func(*registryConfig)

func WithHarnessID(id string) Option {
	return func(cfg *registryConfig) {
		cfg.harnessID = truncateString(strings.TrimSpace(id), 128)
	}
}

func WithMaxAssertions(n int) Option {
	return func(cfg *registryConfig) {
		cfg.maxAssertions = n
	}
}

func WithFailClosedUnsupported(enabled bool) Option {
	return func(cfg *registryConfig) {
		cfg.failClosedUnsupported = enabled
	}
}

// Registry stores assertions and evaluates them deterministically.
type Registry struct {
	mu         sync.RWMutex
	byID       map[string]Assertion
	sortedIDs  []string
	harnessID  string
	maxEntries int
	failClosed bool
}

func NewRegistry(opts ...Option) *Registry {
	cfg := registryConfig{
		harnessID:             "harness.default",
		maxAssertions:         defaultMaxAssertions,
		failClosedUnsupported: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.maxAssertions <= 0 {
		cfg.maxAssertions = defaultMaxAssertions
	}
	if cfg.harnessID == "" {
		cfg.harnessID = "harness.default"
	}
	return &Registry{
		byID:       make(map[string]Assertion),
		harnessID:  cfg.harnessID,
		maxEntries: cfg.maxAssertions,
		failClosed: cfg.failClosedUnsupported,
	}
}

func (r *Registry) Register(assertion Assertion) error {
	return r.RegisterMany([]Assertion{assertion})
}

func (r *Registry) RegisterMany(assertions []Assertion) error {
	if len(assertions) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.byID)+len(assertions) > r.maxEntries {
		return fmt.Errorf("%w: %d > %d", ErrTooManyAssertions, len(r.byID)+len(assertions), r.maxEntries)
	}

	next := make(map[string]Assertion, len(r.byID)+len(assertions))
	for id, existing := range r.byID {
		next[id] = existing
	}

	for i := range assertions {
		norm := assertions[i].Normalized()
		if err := norm.Validate(); err != nil {
			return err
		}
		if _, exists := next[norm.ID]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateAssertion, norm.ID)
		}
		next[norm.ID] = norm
	}

	r.byID = next
	r.rebuildSortedIDsLocked()
	return nil
}

func (r *Registry) List() []Assertion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Assertion, 0, len(r.sortedIDs))
	for _, id := range r.sortedIDs {
		out = append(out, r.byID[id])
	}
	return out
}

func (r *Registry) Find(scope string) []Assertion {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.findLocked(scope)
}

func (r *Registry) Validate(ctx context.Context, artifact Artifact) Report {
	report := NewReport(r.harnessID, artifact.ID)
	started := report.StartedAt

	normArtifact := artifact.Normalized()
	if err := normArtifact.Validate(); err != nil {
		report.Status = StatusInvalid
		report.ErrorTrace = err.Error()
		report.Results = []Result{{
			Status:     statusForError(err),
			Severity:   SeverityHardFail,
			Message:    "artifact validation failed",
			ReasonCode: reasonCodeForError(err),
			Evidence:   []Evidence{{Kind: "error", Value: err.Error()}},
		}}
		report.FinishedAt = time.Now().UTC()
		report.Duration = report.FinishedAt.Sub(started)
		return report.Normalized()
	}

	assertions := r.selectAssertions(normArtifact)
	if len(assertions) == 0 {
		report.Status = StatusInconclusive
		report.FinishedAt = time.Now().UTC()
		report.Duration = report.FinishedAt.Sub(started)
		return report.Normalized()
	}

	report.Status = StatusPass
	for i := range assertions {
		if err := ctx.Err(); err != nil {
			report.Status = StatusInconclusive
			report.ErrorTrace = truncateString(err.Error(), maxAssertionMessageLen)
			break
		}
		result := evaluateAssertion(normArtifact, assertions[i])
		if result.Status == StatusUnsupported && r.failClosed {
			result.Status = StatusFail
			result.Severity = SeverityHardFail
			result.ReasonCode = "unsupported_assertion_fail_closed"
			result.Message = "unsupported assertion type in fail-closed mode"
		}
		report.Results = append(report.Results, result)
		report.Remediation = append(report.Remediation, result.Remediation...)
		report.Evidence = append(report.Evidence, result.Evidence...)
		report.Status = aggregateStatus(report.Status, result.Status)
	}

	report.FinishedAt = time.Now().UTC()
	report.Duration = report.FinishedAt.Sub(started)
	return report.Normalized()
}

func (r *Registry) selectAssertions(artifact Artifact) []Assertion {
	primary := artifact.PrimaryScope()
	kindScope := strings.ToLower(string(artifact.Kind))

	r.mu.RLock()
	defer r.mu.RUnlock()

	candidates := make([]Assertion, 0, len(r.sortedIDs))
	for _, id := range r.sortedIDs {
		a := r.byID[id]
		scope := strings.ToLower(strings.TrimSpace(a.Scope))
		if scope == "*" || scope == primary || scope == kindScope {
			candidates = append(candidates, a)
		}
	}
	return candidates
}

func (r *Registry) findLocked(scope string) []Assertion {
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" {
		return nil
	}
	out := make([]Assertion, 0, len(r.sortedIDs))
	for _, id := range r.sortedIDs {
		a := r.byID[id]
		if strings.EqualFold(a.Scope, s) {
			out = append(out, a)
		}
	}
	return out
}

func (r *Registry) rebuildSortedIDsLocked() {
	r.sortedIDs = r.sortedIDs[:0]
	for id := range r.byID {
		r.sortedIDs = append(r.sortedIDs, id)
	}
	sort.Strings(r.sortedIDs)
}

func aggregateStatus(current Status, next Status) Status {
	switch next {
	case StatusFail:
		return StatusFail
	case StatusInvalid:
		if current != StatusFail {
			return StatusInvalid
		}
	case StatusWarn:
		if current != StatusFail && current != StatusInvalid {
			return StatusWarn
		}
	case StatusUnsupported:
		if current == StatusPass || current == StatusInconclusive {
			return StatusUnsupported
		}
	}
	return current
}

func reasonCodeForError(err error) string {
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), ErrArtifactTooLarge.Error()):
		return "artifact_too_large"
	case strings.Contains(err.Error(), ErrInvalidArtifact.Error()):
		return "invalid_artifact"
	default:
		return "validation_error"
	}
}

func statusForError(err error) Status {
	if err == nil {
		return StatusPass
	}
	if strings.Contains(err.Error(), ErrArtifactTooLarge.Error()) {
		return StatusFail
	}
	return StatusInvalid
}
