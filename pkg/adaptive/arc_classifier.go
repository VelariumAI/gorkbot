// Package arc implements the Agentic Resource & Configuration Router.
// It classifies incoming prompts and computes resource budgets based on
// query complexity and host platform capabilities.
package adaptive

import (
	"context"
	"strings"
	"sync"

	"github.com/velariumai/gorkbot/pkg/embeddings"
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

// ── Semantic classifier ────────────────────────────────────────────────────

// categoryExamples defines representative sentences for each workflow class.
// Multiple examples per class are averaged into a centroid vector for robust
// nearest-neighbour matching.
var categoryExamples = [workflowClassCount][]string{
	// WorkflowConversational
	{
		"hey, how are you doing today?",
		"thanks for the help, that was great",
		"good morning! what's new?",
		"that makes sense, thank you",
		"what do you think about this idea?",
	},
	// WorkflowFactual
	{
		"what is the capital of France?",
		"define the term idempotent",
		"list all environment variables currently set",
		"how many CPUs does this machine have?",
		"what version of Go is installed?",
	},
	// WorkflowAnalytical
	{
		"explain why this code is causing a memory leak",
		"debug this failing test and find the root cause",
		"analyze the performance bottleneck in this service",
		"compare the trade-offs between PostgreSQL and MongoDB",
		"review this pull request and identify issues",
	},
	// WorkflowAgentic
	{
		"implement a REST API with JWT authentication and rate limiting",
		"build and deploy this microservice to production",
		"set up a CI/CD pipeline for this repository",
		"refactor the entire authentication module to use OAuth2",
		"create an automated data-processing workflow from scratch",
	},
	// WorkflowCreative
	{
		"write a blog post about the future of distributed systems",
		"draft a professional email declining a meeting",
		"compose a short story about a robot learning to paint",
		"generate marketing copy for a new developer tool",
		"write documentation for this API endpoint",
	},
	// WorkflowSecurityCritical
	{
		"perform a penetration test on this web application",
		"find SQL injection vulnerabilities in this endpoint",
		"run an nmap scan to enumerate open ports",
		"exploit this CVE in the target environment",
		"enumerate subdomains and map the attack surface",
	},
}

// classVec pairs a workflow class with its averaged embedding centroid.
type classVec struct {
	wf  WorkflowType
	vec []float32
}

// SemanticClassifier classifies prompts using nearest-neighbour embedding
// similarity when an Embedder is available, or a minimal keyword heuristic
// when it is not.
type SemanticClassifier struct {
	mu       sync.RWMutex
	embedder embeddings.Embedder
	cached   []classVec // lazily computed centroids; nil until first use
}

// SetEmbedder wires an embedder. Thread-safe; may be called at any time.
func (c *SemanticClassifier) SetEmbedder(e embeddings.Embedder) {
	c.mu.Lock()
	c.embedder = e
	c.cached = nil // invalidate cached centroids when embedder changes
	c.mu.Unlock()
}

// EmbedderName returns the active embedder's name, or a fallback description.
func (c *SemanticClassifier) EmbedderName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.embedder != nil {
		return c.embedder.Name()
	}
	return "none (heuristic fallback)"
}

// ensureCentroids computes and caches per-class embedding centroids.
// Must be called with c.mu held for write, or under a lock that prevents
// concurrent embedding of the same examples.
func (c *SemanticClassifier) ensureCentroids(ctx context.Context) {
	if c.cached != nil {
		return
	}
	c.cached = make([]classVec, 0, workflowClassCount)
	for wf := WorkflowType(0); int(wf) < workflowClassCount; wf++ {
		exs := categoryExamples[int(wf)]
		var centroid []float32
		count := 0
		for _, ex := range exs {
			vec, err := c.embedder.Embed(ctx, ex)
			if err != nil {
				continue
			}
			if centroid == nil {
				centroid = make([]float32, len(vec))
			}
			for i, v := range vec {
				centroid[i] += v
			}
			count++
		}
		if count == 0 || centroid == nil {
			continue
		}
		// Average → centroid, then L2-normalise so cosine sim is just a dot product.
		inv := float32(1.0 / float64(count))
		for i := range centroid {
			centroid[i] *= inv
		}
		c.cached = append(c.cached, classVec{
			wf:  wf,
			vec: embeddings.L2Normalize(centroid),
		})
	}
}

// Classify returns the WorkflowType for the given prompt.
func (c *SemanticClassifier) Classify(prompt string) WorkflowType {
	wf, _ := c.ClassifyWithConfidence(prompt)
	return wf
}

// ClassifyWithConfidence returns the WorkflowType and a confidence score in
// [0.0, 1.0].  When an Embedder is available it uses cosine similarity to
// pre-computed class centroids; otherwise it falls back to a minimal
// keyword heuristic.
func (c *SemanticClassifier) ClassifyWithConfidence(prompt string) (WorkflowType, float64) {
	c.mu.RLock()
	emb := c.embedder
	c.mu.RUnlock()

	if emb != nil {
		return c.semanticClassify(prompt, emb)
	}
	return c.heuristicClassify(prompt)
}

// semanticClassify embeds the prompt and finds the nearest centroid.
func (c *SemanticClassifier) semanticClassify(prompt string, emb embeddings.Embedder) (WorkflowType, float64) {
	ctx := context.Background()

	// Ensure centroids are built (lazy, once per embedder).
	c.mu.Lock()
	c.ensureCentroids(ctx)
	cached := c.cached
	c.mu.Unlock()

	if len(cached) == 0 {
		return c.heuristicClassify(prompt)
	}

	vec, err := emb.Embed(ctx, prompt)
	if err != nil {
		return c.heuristicClassify(prompt)
	}

	// Cosine similarity to each centroid (vectors are already L2-normalised).
	best := WorkflowConversational
	bestSim := -2.0
	secondSim := -2.0
	for _, cv := range cached {
		sim := embeddings.CosineSimilarity(vec, cv.vec)
		if sim > bestSim {
			secondSim = bestSim
			bestSim = sim
			best = cv.wf
		} else if sim > secondSim {
			secondSim = sim
		}
	}

	// Confidence: normalised margin between top and second similarity.
	conf := 0.0
	if bestSim > secondSim && bestSim > 0 {
		conf = (bestSim - secondSim) / bestSim
		if conf > 1 {
			conf = 1
		}
		if conf < 0 {
			conf = 0
		}
	}
	return best, conf
}

// heuristicClassify is a minimal fallback used when no Embedder is wired.
// It covers security (hard to mistake), agentic multi-step, creative, and
// length-based analytical escalation — everything else is conversational/factual.
func (c *SemanticClassifier) heuristicClassify(prompt string) (WorkflowType, float64) {
	lower := strings.ToLower(prompt)

	// Security — high-signal tool names and techniques.
	for _, kw := range []string{
		"pentest", "exploit", "nmap", "sqlmap", "metasploit", "burp",
		"sql injection", "xss", "ssrf", "idor", "reverse shell",
		"shellcode", "buffer overflow", "privilege escalation", "red team",
		"cve-", "gobuster", "nuclei", "ffuf", "hydra", "hashcat",
	} {
		if strings.Contains(lower, kw) {
			return WorkflowSecurityCritical, 0.9
		}
	}

	// Agentic — action verbs with clear implementation intent.
	for _, kw := range []string{
		"implement", "build me", "build a", "deploy", "automate",
		"set up", "refactor", "migrate", "integrate", "from scratch",
		"step by step", "end to end", "pipeline",
	} {
		if strings.Contains(lower, kw) {
			return WorkflowAgentic, 0.7
		}
	}

	// Creative — writing / drafting intent.
	for _, kw := range []string{
		"write a", "write me", "write the", "draft ", "compose ",
		"blog post", "story ", "poem ", "email ",
	} {
		if strings.Contains(lower, kw) {
			return WorkflowCreative, 0.7
		}
	}

	// Length escalation: long prompts likely need analysis.
	switch {
	case len(prompt) > 600:
		return WorkflowAnalytical, 0.5
	case len(prompt) > 200:
		return WorkflowFactual, 0.3
	case len(prompt) < 60:
		return WorkflowConversational, 0.5
	}
	return WorkflowFactual, 0.2
}
