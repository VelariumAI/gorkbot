package subagents

// secctx.go — Shared Security Context for red team agent coordination.
//
// SecurityContext is session-scoped (not persisted). All red team subagents
// share it via a context key injected by the orchestrator, enabling findings
// discovered by one agent (e.g., recon) to be available to subsequent agents
// (e.g., injection, XSS).

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// SecurityContextKey is the context key for injecting SecurityContext into tool execution.
type secCtxKeyType struct{}

var SecurityContextKey = secCtxKeyType{}

// FindingSeverity classifies the impact level of a discovered vulnerability.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "Critical"
	SeverityHigh     FindingSeverity = "High"
	SeverityMedium   FindingSeverity = "Medium"
	SeverityLow      FindingSeverity = "Low"
	SeverityInfo     FindingSeverity = "Info"
)

// Finding represents a discovered security issue.
type Finding struct {
	ID       string
	Type     string         // SQLi, XSS, IDOR, SSRF, RCE, AuthnBypass, etc.
	Severity FindingSeverity
	Detail   string
	FoundBy  string    // which agent discovered it
	At       time.Time
}

// SecurityContext is the shared mutable state for red team sessions.
type SecurityContext struct {
	mu        sync.RWMutex
	Target    string            // primary target (URL, IP, domain)
	Scope     []string          // in-scope hosts, paths, or CIDR ranges
	Findings  []Finding         // discovered vulnerabilities (accumulated)
	Recon     map[string]string // endpoint/host → tech notes
	SessionID string
}

// NewSecurityContext creates a fresh SecurityContext for a pentest session.
func NewSecurityContext(target, sessionID string) *SecurityContext {
	return &SecurityContext{
		Target:    target,
		Scope:     []string{},
		Findings:  []Finding{},
		Recon:     make(map[string]string),
		SessionID: sessionID,
	}
}

// AddFinding records a new vulnerability finding.
func (sc *SecurityContext) AddFinding(findType, severity, detail, foundBy string) string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	id := fmt.Sprintf("F%03d", len(sc.Findings)+1)
	sc.Findings = append(sc.Findings, Finding{
		ID:       id,
		Type:     findType,
		Severity: FindingSeverity(severity),
		Detail:   detail,
		FoundBy:  foundBy,
		At:       time.Now(),
	})
	return id
}

// GetFindings returns a copy of all findings, optionally filtered by type.
func (sc *SecurityContext) GetFindings(filterType string) []Finding {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if filterType == "" {
		out := make([]Finding, len(sc.Findings))
		copy(out, sc.Findings)
		return out
	}
	var out []Finding
	for _, f := range sc.Findings {
		if strings.EqualFold(f.Type, filterType) {
			out = append(out, f)
		}
	}
	return out
}

// SetRecon stores reconnaissance notes for a host or endpoint.
func (sc *SecurityContext) SetRecon(endpoint, notes string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.Recon[endpoint] = notes
}

// GetRecon returns all recon notes.
func (sc *SecurityContext) GetRecon() map[string]string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	out := make(map[string]string, len(sc.Recon))
	for k, v := range sc.Recon {
		out[k] = v
	}
	return out
}

// FormatBrief returns a compact summary for injection into agent system prompts.
// Returns "" if the context has no meaningful content.
func (sc *SecurityContext) FormatBrief() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	if sc.Target == "" && len(sc.Findings) == 0 && len(sc.Recon) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### Security Context (shared session)\n")
	if sc.Target != "" {
		sb.WriteString(fmt.Sprintf("**Target:** %s\n", sc.Target))
	}
	if len(sc.Scope) > 0 {
		sb.WriteString(fmt.Sprintf("**In-scope:** %s\n", strings.Join(sc.Scope, ", ")))
	}
	if len(sc.Recon) > 0 {
		sb.WriteString("**Recon:**\n")
		for ep, notes := range sc.Recon {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", ep, notes))
		}
	}
	if len(sc.Findings) > 0 {
		sb.WriteString(fmt.Sprintf("**Findings so far (%d):**\n", len(sc.Findings)))
		for _, f := range sc.Findings {
			sb.WriteString(fmt.Sprintf("  - [%s][%s] %s (%s)\n", f.ID, f.Severity, f.Type, f.FoundBy))
		}
	}
	return sb.String()
}

// FormatFull returns a detailed report of all findings for inclusion in final reports.
func (sc *SecurityContext) FormatFull() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Security Assessment — %s\n\n", sc.Target))
	sb.WriteString(fmt.Sprintf("Session: %s\n\n", sc.SessionID))

	if len(sc.Findings) == 0 {
		sb.WriteString("No findings recorded.\n")
		return sb.String()
	}

	// Group by severity
	order := []FindingSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo}
	for _, sev := range order {
		var group []Finding
		for _, f := range sc.Findings {
			if f.Severity == sev {
				group = append(group, f)
			}
		}
		if len(group) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n", sev))
		for _, f := range group {
			sb.WriteString(fmt.Sprintf("### [%s] %s\n", f.ID, f.Type))
			sb.WriteString(fmt.Sprintf("**Found by:** %s  **At:** %s\n\n", f.FoundBy, f.At.Format("15:04:05")))
			sb.WriteString(f.Detail + "\n\n")
		}
	}
	return sb.String()
}
