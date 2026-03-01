// Package arc implements the Agentic Resource & Configuration Router.
// It classifies incoming prompts and computes resource budgets based on
// query complexity and host platform capabilities.
package arc

import (
	"strings"
)

// WorkflowType classifies the nature and complexity of an incoming request.
type WorkflowType int

const (
	// WorkflowConversational — chat, greeting, simple Q&A, acknowledgement.
	WorkflowConversational WorkflowType = iota
	// WorkflowFactual — lookup, definition, status check, enumeration.
	WorkflowFactual
	// WorkflowAnalytical — explain, compare, reason, review, debug analysis.
	WorkflowAnalytical
	// WorkflowAgentic — implement, build, execute multi-step, automate.
	WorkflowAgentic
	// WorkflowCreative — write, draft, generate content, design.
	WorkflowCreative
	// WorkflowSecurityCritical — pentest, exploit, scan, inject, enumerate.
	WorkflowSecurityCritical

	workflowClassCount = 6
)

// String returns the workflow name.
func (w WorkflowType) String() string {
	switch w {
	case WorkflowConversational:
		return "Conversational"
	case WorkflowFactual:
		return "Factual"
	case WorkflowAnalytical:
		return "Analytical"
	case WorkflowAgentic:
		return "Agentic"
	case WorkflowCreative:
		return "Creative"
	case WorkflowSecurityCritical:
		return "SecurityCritical"
	default:
		return "Unknown"
	}
}

// IsComplex returns true for workflow classes that require multi-step reasoning.
func (w WorkflowType) IsComplex() bool {
	return w == WorkflowAnalytical || w == WorkflowAgentic || w == WorkflowSecurityCritical
}

// QueryClassifier scores prompts and maps them to a WorkflowType using
// multi-class keyword scoring.
type QueryClassifier struct{}

// keyword scoring tables: each entry is (keyword, target class, weight)
type kwEntry struct {
	kw     string
	class  WorkflowType
	weight float64
}

var keywords = []kwEntry{
	// Conversational
	{"hello", WorkflowConversational, 3.0},
	{"hi ", WorkflowConversational, 3.0},
	{"hey ", WorkflowConversational, 3.0},
	{"thanks", WorkflowConversational, 2.0},
	{"thank you", WorkflowConversational, 2.0},
	{"good morning", WorkflowConversational, 3.0},
	{"good evening", WorkflowConversational, 3.0},
	{"how are you", WorkflowConversational, 3.0},
	{"what do you think", WorkflowConversational, 1.5},
	{"tell me about yourself", WorkflowConversational, 3.0},

	// Factual
	{"what is", WorkflowFactual, 2.0},
	{"what are", WorkflowFactual, 1.5},
	{"who is", WorkflowFactual, 2.0},
	{"when did", WorkflowFactual, 2.0},
	{"where is", WorkflowFactual, 2.0},
	{"define ", WorkflowFactual, 2.0},
	{"definition of", WorkflowFactual, 2.0},
	{"list all", WorkflowFactual, 1.5},
	{"show me", WorkflowFactual, 1.0},
	{"status of", WorkflowFactual, 1.5},
	{"version of", WorkflowFactual, 1.5},
	{"how many", WorkflowFactual, 1.5},
	{"what version", WorkflowFactual, 2.0},

	// Analytical
	{"explain", WorkflowAnalytical, 2.0},
	{"explain how", WorkflowAnalytical, 2.5},
	{"analyze", WorkflowAnalytical, 2.5},
	{"analyse", WorkflowAnalytical, 2.5},
	{"compare", WorkflowAnalytical, 2.0},
	{"review", WorkflowAnalytical, 2.0},
	{"evaluate", WorkflowAnalytical, 2.0},
	{"why does", WorkflowAnalytical, 2.0},
	{"debug ", WorkflowAnalytical, 2.0},
	{"diagnose", WorkflowAnalytical, 2.0},
	{"reason ", WorkflowAnalytical, 1.5},
	{"think about", WorkflowAnalytical, 1.5},
	{"what causes", WorkflowAnalytical, 2.0},
	{"how does", WorkflowAnalytical, 1.5},
	{"root cause", WorkflowAnalytical, 3.0},
	{"performance issue", WorkflowAnalytical, 2.0},

	// Agentic
	{"implement", WorkflowAgentic, 3.0},
	{"build ", WorkflowAgentic, 2.5},
	{"build me", WorkflowAgentic, 3.0},
	{"create a", WorkflowAgentic, 2.0},
	{"develop ", WorkflowAgentic, 2.5},
	{"execute ", WorkflowAgentic, 2.0},
	{"automate", WorkflowAgentic, 2.5},
	{"set up ", WorkflowAgentic, 2.0},
	{"deploy ", WorkflowAgentic, 2.5},
	{"configure ", WorkflowAgentic, 2.0},
	{"refactor ", WorkflowAgentic, 2.5},
	{"migrate ", WorkflowAgentic, 2.5},
	{"integrate ", WorkflowAgentic, 2.5},
	{"step by step", WorkflowAgentic, 2.0},
	{"pipeline", WorkflowAgentic, 2.0},
	{"workflow", WorkflowAgentic, 2.0},
	{"multi-step", WorkflowAgentic, 2.5},
	{"end to end", WorkflowAgentic, 2.5},
	{"from scratch", WorkflowAgentic, 2.0},

	// Creative
	{"write a", WorkflowCreative, 2.5},
	{"write the", WorkflowCreative, 2.0},
	{"draft ", WorkflowCreative, 2.5},
	{"generate a", WorkflowCreative, 2.0},
	{"compose ", WorkflowCreative, 2.0},
	{"design ", WorkflowCreative, 2.0},
	{"create content", WorkflowCreative, 2.5},
	{"write an", WorkflowCreative, 2.5},
	{"write me", WorkflowCreative, 2.0},
	{"story ", WorkflowCreative, 2.0},
	{"blog post", WorkflowCreative, 3.0},
	{"email ", WorkflowCreative, 1.5},

	// SecurityCritical
	{"pentest", WorkflowSecurityCritical, 4.0},
	{"penetration test", WorkflowSecurityCritical, 4.0},
	{"exploit", WorkflowSecurityCritical, 3.5},
	{"nmap ", WorkflowSecurityCritical, 4.0},
	{"nmap scan", WorkflowSecurityCritical, 4.0},
	{"sqlmap", WorkflowSecurityCritical, 4.0},
	{"sql injection", WorkflowSecurityCritical, 4.0},
	{"sqli", WorkflowSecurityCritical, 4.0},
	{"xss", WorkflowSecurityCritical, 3.5},
	{"cross-site scripting", WorkflowSecurityCritical, 4.0},
	{"burp suite", WorkflowSecurityCritical, 4.0},
	{"metasploit", WorkflowSecurityCritical, 4.0},
	{"hydra ", WorkflowSecurityCritical, 3.5},
	{"hashcat", WorkflowSecurityCritical, 3.5},
	{"gobuster", WorkflowSecurityCritical, 4.0},
	{"nuclei ", WorkflowSecurityCritical, 3.5},
	{"ffuf ", WorkflowSecurityCritical, 3.5},
	{"ssrf", WorkflowSecurityCritical, 4.0},
	{"idor", WorkflowSecurityCritical, 4.0},
	{"privilege escalation", WorkflowSecurityCritical, 4.0},
	{"red team", WorkflowSecurityCritical, 4.0},
	{"vulnerability scan", WorkflowSecurityCritical, 4.0},
	{"attack surface", WorkflowSecurityCritical, 4.0},
	{"payload ", WorkflowSecurityCritical, 3.0},
	{"shellcode", WorkflowSecurityCritical, 4.0},
	{"reverse shell", WorkflowSecurityCritical, 4.0},
	{"buffer overflow", WorkflowSecurityCritical, 4.0},
	{"cve-", WorkflowSecurityCritical, 3.5},
	{"bypass ", WorkflowSecurityCritical, 2.5},
	{"enumerate ", WorkflowSecurityCritical, 2.0},
	{"reconnaissance", WorkflowSecurityCritical, 3.5},
	{"subdomain ", WorkflowSecurityCritical, 2.5},
}

// Classify returns the WorkflowType for the given prompt using multi-class scoring.
func (c *QueryClassifier) Classify(prompt string) WorkflowType {
	lower := strings.ToLower(prompt)
	scores := make([]float64, workflowClassCount)

	// Keyword scoring across all 6 classes
	for _, kw := range keywords {
		if strings.Contains(lower, kw.kw) {
			scores[int(kw.class)] += kw.weight
		}
	}

	// Length heuristic: longer prompts imply higher complexity
	lenBoost := 0.0
	switch {
	case len(prompt) > 800:
		lenBoost = 2.0
	case len(prompt) > 400:
		lenBoost = 1.0
	case len(prompt) > 150:
		lenBoost = 0.5
	}
	// Length boost applies to analytical and agentic classes
	scores[int(WorkflowAnalytical)] += lenBoost * 0.5
	scores[int(WorkflowAgentic)] += lenBoost * 0.5

	// Find the highest-scoring class
	best := WorkflowConversational
	bestScore := scores[0]
	for i := 1; i < workflowClassCount; i++ {
		if scores[i] > bestScore {
			bestScore = scores[i]
			best = WorkflowType(i)
		}
	}

	// Tie-break: if scores are all zero or very low, classify short prompts as
	// conversational and longer ones as factual
	if bestScore < 0.5 {
		if len(prompt) < 50 {
			return WorkflowConversational
		}
		return WorkflowFactual
	}

	return best
}
