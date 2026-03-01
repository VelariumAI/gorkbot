// Package arc — categories.go
//
// Intent category classification for the ARC (Agentic Resource & Configuration)
// Router. Inspired by oh-my-opencode's category-based model-routing system,
// which maps task domains to the most capable (or most efficient) model for
// that domain rather than using a single model for everything.
//
// Design:
//   - IntentCategory is a semantic label for the nature of a request.
//   - ClassifyIntent() uses weighted keyword scoring to identify the category.
//   - The category is used by provider_routing.go to select the optimal model
//     (e.g. "visual" → Gemini, "security" → high-compute model, "quick" → Flash).
//   - All keyword lists are intentionally conservative: we'd rather return
//     CategoryAuto and let the normal workflow decide than misroute.
package arc

import (
	"strings"
)

// IntentCategory identifies the semantic domain of a user request so the
// router can select the most suitable AI provider and model variant.
type IntentCategory string

const (
	// CategoryAuto — no strong signal; use default routing logic.
	CategoryAuto IntentCategory = "auto"

	// CategoryDeep — complex reasoning, architectural decisions, hard debugging,
	// performance analysis. Route to the highest-capability model variant.
	CategoryDeep IntentCategory = "deep"

	// CategoryQuick — simple factual questions, syntax lookups, brief answers.
	// Route to the fastest/cheapest model.
	CategoryQuick IntentCategory = "quick"

	// CategoryVisual — frontend, UI/UX, CSS, design, image analysis.
	// Route to a vision-capable or design-specialised model.
	CategoryVisual IntentCategory = "visual"

	// CategoryResearch — documentation lookup, library comparison, OSS patterns,
	// web search. Web-tool-heavy execution path.
	CategoryResearch IntentCategory = "research"

	// CategorySecurity — penetration testing, CVE analysis, exploit research,
	// security auditing. Needs a model comfortable with offensive context.
	CategorySecurity IntentCategory = "security"

	// CategoryCode — pure implementation: writing functions, classes, tests,
	// refactoring, debugging single units. Code-focused context.
	CategoryCode IntentCategory = "code"

	// CategoryCreative — writing, documentation, README, blog posts, brainstorming.
	CategoryCreative IntentCategory = "creative"

	// CategoryData — data analysis, SQL, statistics, ML pipelines, visualisation.
	CategoryData IntentCategory = "data"

	// CategoryPlan — strategic planning, task decomposition, multi-step workflow
	// design. Engages plan_mode and the HITL guard.
	CategoryPlan IntentCategory = "plan"
)

// intentSignal pairs a keyword/phrase with an intent category and a weight.
type intentSignal struct {
	phrase   string
	category IntentCategory
	weight   float64
}

// signals is the master list of scored intent signals.
// Higher weight signals are more definitive; lower-weight signals contribute
// but don't dominate on their own.
var signals = []intentSignal{
	// ── CategoryDeep ─────────────────────────────────────────────────────────
	{"architect", CategoryDeep, 2.0},
	{"architecture", CategoryDeep, 2.0},
	{"design pattern", CategoryDeep, 2.0},
	{"trade-offs", CategoryDeep, 1.8},
	{"tradeoffs", CategoryDeep, 1.8},
	{"concurrency", CategoryDeep, 1.8},
	{"race condition", CategoryDeep, 2.5},
	{"deadlock", CategoryDeep, 2.5},
	{"performance bottleneck", CategoryDeep, 2.0},
	{"benchmark", CategoryDeep, 1.5},
	{"optimize algorithm", CategoryDeep, 1.8},
	{"complexity analysis", CategoryDeep, 2.0},
	{"deep dive", CategoryDeep, 1.5},
	{"explain internals", CategoryDeep, 1.8},
	{"how does it work internally", CategoryDeep, 2.0},
	{"compare approaches", CategoryDeep, 1.5},
	{"best approach", CategoryDeep, 1.3},
	{"refactor entire", CategoryDeep, 1.5},
	{"large-scale", CategoryDeep, 1.5},
	{"system design", CategoryDeep, 2.5},
	{"scalability", CategoryDeep, 1.5},
	{"distributed", CategoryDeep, 1.5},

	// ── CategoryQuick ────────────────────────────────────────────────────────
	{"what is the syntax", CategoryQuick, 2.0},
	{"what does this mean", CategoryQuick, 1.2},
	{"quick question", CategoryQuick, 2.5},
	{"just tell me", CategoryQuick, 1.8},
	{"one liner", CategoryQuick, 2.0},
	{"short answer", CategoryQuick, 2.0},
	{"tldr", CategoryQuick, 2.5},
	{"tl;dr", CategoryQuick, 2.5},
	{"briefly", CategoryQuick, 1.5},
	{"in one sentence", CategoryQuick, 2.0},
	{"what version", CategoryQuick, 1.5},

	// ── CategoryVisual ───────────────────────────────────────────────────────
	{"frontend", CategoryVisual, 1.5},
	{"ui component", CategoryVisual, 2.0},
	{"ux design", CategoryVisual, 2.0},
	{"css", CategoryVisual, 1.5},
	{"tailwind", CategoryVisual, 1.8},
	{"dark mode", CategoryVisual, 1.5},
	{"color palette", CategoryVisual, 2.0},
	{"animation", CategoryVisual, 1.5},
	{"responsive layout", CategoryVisual, 1.8},
	{"pixel perfect", CategoryVisual, 2.5},
	{"figma", CategoryVisual, 2.5},
	{"wireframe", CategoryVisual, 2.5},
	{"mockup", CategoryVisual, 2.0},
	{"screenshot", CategoryVisual, 1.5},
	{"image analysis", CategoryVisual, 2.0},
	{"look at this image", CategoryVisual, 2.5},
	{"design system", CategoryVisual, 2.0},
	{"storybook", CategoryVisual, 1.8},
	{"svg", CategoryVisual, 1.5},
	{"react component", CategoryVisual, 1.3},
	{"vue component", CategoryVisual, 1.3},
	{"svelte", CategoryVisual, 1.5},

	// ── CategoryResearch ─────────────────────────────────────────────────────
	{"look up", CategoryResearch, 1.5},
	{"find documentation", CategoryResearch, 2.0},
	{"official docs", CategoryResearch, 2.0},
	{"what does the docs say", CategoryResearch, 2.0},
	{"npm package", CategoryResearch, 1.8},
	{"crates.io", CategoryResearch, 2.0},
	{"pypi", CategoryResearch, 2.0},
	{"github stars", CategoryResearch, 1.5},
	{"compare libraries", CategoryResearch, 2.5},
	{"which library", CategoryResearch, 2.0},
	{"alternatives to", CategoryResearch, 2.0},
	{"search for", CategoryResearch, 1.2},
	{"latest version of", CategoryResearch, 1.8},
	{"changelog", CategoryResearch, 1.5},
	{"release notes", CategoryResearch, 1.5},
	{"api reference", CategoryResearch, 1.8},
	{"example code from", CategoryResearch, 1.5},

	// ── CategorySecurity ─────────────────────────────────────────────────────
	{"penetration test", CategorySecurity, 3.0},
	{"pentest", CategorySecurity, 3.0},
	{"vulnerability", CategorySecurity, 2.0},
	{"cve-", CategorySecurity, 3.0},
	{"exploit", CategorySecurity, 2.5},
	{"sql injection", CategorySecurity, 3.0},
	{"sqli", CategorySecurity, 3.0},
	{"xss", CategorySecurity, 2.5},
	{"csrf", CategorySecurity, 2.5},
	{"ssrf", CategorySecurity, 2.5},
	{"rce", CategorySecurity, 3.0},
	{"lfi", CategorySecurity, 2.5},
	{"rfi", CategorySecurity, 2.5},
	{"nmap", CategorySecurity, 2.5},
	{"nuclei", CategorySecurity, 2.5},
	{"burp suite", CategorySecurity, 3.0},
	{"metasploit", CategorySecurity, 3.0},
	{"payload", CategorySecurity, 1.8},
	{"bypass waf", CategorySecurity, 3.0},
	{"ctf challenge", CategorySecurity, 3.0},
	{"capture the flag", CategorySecurity, 3.0},
	{"reverse engineering", CategorySecurity, 2.5},
	{"decompile", CategorySecurity, 2.0},
	{"hash crack", CategorySecurity, 2.5},
	{"brute force", CategorySecurity, 2.0},
	{"privilege escalation", CategorySecurity, 3.0},
	{"lateral movement", CategorySecurity, 3.0},
	{"red team", CategorySecurity, 3.0},
	{"recon", CategorySecurity, 1.8},
	{"attack surface", CategorySecurity, 2.5},
	{"audit security", CategorySecurity, 2.0},
	{"tshark", CategorySecurity, 2.0},
	{"wireshark", CategorySecurity, 2.0},
	{"osint", CategorySecurity, 2.5},

	// ── CategoryCode ─────────────────────────────────────────────────────────
	{"implement the following", CategoryCode, 1.5},
	{"write a function", CategoryCode, 1.5},
	{"write a class", CategoryCode, 1.5},
	{"write unit tests", CategoryCode, 1.8},
	{"add a method", CategoryCode, 1.2},
	{"fix this bug", CategoryCode, 1.5},
	{"debug this", CategoryCode, 1.2},
	{"refactor this function", CategoryCode, 1.5},
	{"parse this", CategoryCode, 1.2},
	{"serialize", CategoryCode, 1.2},
	{"add error handling", CategoryCode, 1.2},
	{"complete this code", CategoryCode, 1.5},
	{"finish implementing", CategoryCode, 1.5},

	// ── CategoryCreative ─────────────────────────────────────────────────────
	{"write a blog post", CategoryCreative, 2.5},
	{"write documentation", CategoryCreative, 2.0},
	{"write a readme", CategoryCreative, 2.0},
	{"brainstorm", CategoryCreative, 2.0},
	{"generate ideas", CategoryCreative, 1.8},
	{"write an essay", CategoryCreative, 2.5},
	{"summarize this", CategoryCreative, 1.5},
	{"explain to a non-technical", CategoryCreative, 2.0},
	{"write a proposal", CategoryCreative, 2.0},
	{"draft an email", CategoryCreative, 2.5},
	{"create a changelog", CategoryCreative, 1.8},
	{"meeting notes", CategoryCreative, 2.0},

	// ── CategoryData ─────────────────────────────────────────────────────────
	{"sql query", CategoryData, 2.0},
	{"database schema", CategoryData, 2.0},
	{"pandas dataframe", CategoryData, 2.5},
	{"numpy array", CategoryData, 2.0},
	{"matplotlib", CategoryData, 1.8},
	{"seaborn", CategoryData, 1.8},
	{"statistical analysis", CategoryData, 2.5},
	{"linear regression", CategoryData, 2.5},
	{"machine learning model", CategoryData, 2.0},
	{"train a model", CategoryData, 2.0},
	{"dataset", CategoryData, 1.3},
	{"data pipeline", CategoryData, 2.0},
	{"etl", CategoryData, 2.5},
	{"aggregate this data", CategoryData, 1.8},
	{"pivot table", CategoryData, 2.5},
	{"visualize the data", CategoryData, 1.8},
	{"plot this", CategoryData, 1.5},

	// ── CategoryPlan ─────────────────────────────────────────────────────────
	{"create a plan", CategoryPlan, 2.5},
	{"plan how to", CategoryPlan, 2.5},
	{"step by step plan", CategoryPlan, 2.5},
	{"break down the task", CategoryPlan, 2.0},
	{"task decomposition", CategoryPlan, 2.5},
	{"roadmap", CategoryPlan, 2.0},
	{"what are the steps to", CategoryPlan, 1.8},
	{"plan mode", CategoryPlan, 3.0},
	{"project plan", CategoryPlan, 2.0},
	{"sprint plan", CategoryPlan, 2.0},
	{"milestone", CategoryPlan, 1.5},
}

// ClassifyIntent scores a prompt against all intent signals and returns the
// category with the highest score, or CategoryAuto if no category clears the
// minimum confidence threshold.
func ClassifyIntent(prompt string) IntentCategory {
	lower := strings.ToLower(prompt)
	scores := make(map[IntentCategory]float64, 10)

	for _, sig := range signals {
		if strings.Contains(lower, sig.phrase) {
			scores[sig.category] += sig.weight
		}
	}

	// Find the highest-scoring category.
	best := CategoryAuto
	bestScore := 0.0
	for cat, score := range scores {
		if score > bestScore {
			bestScore = score
			best = cat
		}
	}

	// Require a minimum confidence threshold to avoid random misclassification.
	const minThreshold = 2.0
	if bestScore < minThreshold {
		return CategoryAuto
	}

	return best
}

// CategoryLabel returns a human-readable description of a category.
func CategoryLabel(c IntentCategory) string {
	switch c {
	case CategoryDeep:
		return "Deep Reasoning"
	case CategoryQuick:
		return "Quick Answer"
	case CategoryVisual:
		return "Visual/UI/UX"
	case CategoryResearch:
		return "Research & Docs"
	case CategorySecurity:
		return "Security/Pentest"
	case CategoryCode:
		return "Code Implementation"
	case CategoryCreative:
		return "Creative Writing"
	case CategoryData:
		return "Data Analysis"
	case CategoryPlan:
		return "Strategic Planning"
	default:
		return "Auto"
	}
}

// CategoryEmoji returns a terminal-friendly icon for display.
func CategoryEmoji(c IntentCategory) string {
	switch c {
	case CategoryDeep:
		return "🧠"
	case CategoryQuick:
		return "⚡"
	case CategoryVisual:
		return "🎨"
	case CategoryResearch:
		return "🔍"
	case CategorySecurity:
		return "🛡"
	case CategoryCode:
		return "💻"
	case CategoryCreative:
		return "✍"
	case CategoryData:
		return "📊"
	case CategoryPlan:
		return "📋"
	default:
		return "🤖"
	}
}
