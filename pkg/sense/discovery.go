package sense

// discovery.go — SENSE Discovery Layer
//
// Implements /self schema: builds a machine-readable JSON Discovery Document
// of all registered Gorkbot tools and CLI flags.
//
// The Discovery Document is the authoritative source of truth for:
//   - External tooling that needs to enumerate available capabilities
//   - The /self check analyzer (validates tool names against real registry)
//   - Operator dashboards and monitoring integrations
//   - The evolutionary pipeline (maps hallucinated names to real alternatives)
//
// The document format is stable and versioned.  Adding new tools does not
// change the schema version; breaking schema changes increment MajorVersion.

import (
	"encoding/json"
	"fmt"
	"time"
)

// DiscoveryVersion is the semantic version of the discovery document schema.
const DiscoveryVersion = "1.0.0"

// SENSEVersion is the semantic version of the SENSE middleware layer.
//
// History:
//   1.0.0 — initial release: InputSanitizer, SENSETracer, TraceAnalyzer, SkillEvolver,
//            Discovery, AgeMem (3-tier STM/LTM), Engrams, LIE reward model,
//            Stabilizer (4-dim quality critic), Compression (4-stage pipeline)
//   1.1.0 — refinements across discovery, input_sanitizer, lie, skill_evolver,
//            trace_analyzer, tracer; sense_self_tools updated
//   1.2.0 — performance overhaul: tracer sync→async 512-slot buffered channel + drainLoop;
//            InputSanitizer atomic counters + package-level keyword slices (no alloc hot-path)
//   1.3.0 — cognitive consolidation (v4.5.0): AgeMem, Engrams, LIE, Stabilizer updated;
//            sense_hitl engine integration; sense_evolve MCP plugin added
//   1.4.0 — context scanning: InputSanitizer.ScanContextContent for brain files,
//            GORKBOT.md, and skill content (19 injection patterns + base64 + HTML entity decode)
//   1.5.0 — privilege & parsing (v4.7.0): Module 1 (env probe UID/IsRoot/escalation in
//            system prompt), Module 2 (EAL — privileged_execute auto-escalation router),
//            Module 4 (UPE — structured_bash with heuristic JSON/tabular/keyvalue parser)
//   1.6.0 — sandbox toggle (v4.8.0): runtime /sandbox command to bypass or restore the
//            InputSanitizer without restart; AppState-persisted SandboxEnabled preference
//   1.7.0 — SPARK TII/IDL/FRC/MotivationalCore/ResearchModule (v4.9.0)
//   1.7.1 — path sandbox expanded allowlist (v4.9.1): InputSanitizer now permits
//            $HOME, /tmp, /var/tmp, /sdcard, /storage, /data/local/tmp, $TMPDIR
//            in addition to CWD — prevents false rejections of screenshot/OCR/storage ops
//   1.8.0 — companion augmentation layer (v5.0.0): IngressFilter (token pruning + sentence
//            dedup before ARC routing), IngressGuard (Jaccard semantic evasion protection),
//            MELValidator (BLAKE2b bloom filter + entropy gate + injection scan for VectorStore),
//            VectorProjector (RAM-aware 128/256/full dim reduction wrapping Embedder),
//            CacheAdvisor (multi-provider cache: Anthropic model-aware floors, Gemini
//            cachedContents lifecycle, Grok x-grok-conv-id sticky routing, OpenAI structural
//            optimizer, MiniMax/OpenRouter passthrough, Moonshot best-effort, app-layer LRU)
//   1.9.0 — TUI status line redesign (v5.2.0): single authoritative "G ▶ " status line with
//            SRE phase labeling, token counting, grounding status, and proper lifecycle tracking;
//            eliminates duplicate thinking/reasoning messages and ensures honest status updates
//            only after LLM work actually begins (no premature labeling)
const SENSEVersion = "1.9.0"

// ToolDescriptor is the minimal, JSON-safe description of one registered tool.
// It mirrors the Tool interface without exposing Go types externally.
type ToolDescriptor struct {
	// Name is the unique tool identifier (normalised, lowercase-underscore).
	Name string `json:"name"`
	// Description is the human-readable tool description.
	Description string `json:"description"`
	// Category is the tool's category string.
	Category string `json:"category"`
	// Parameters is the raw JSON Schema object for this tool's parameters.
	Parameters json.RawMessage `json:"parameters"`
	// RequiresPermission indicates whether the tool prompts for user approval.
	RequiresPermission bool `json:"requires_permission"`
	// DefaultPermission is the default permission level ("always", "once", etc).
	DefaultPermission string `json:"default_permission"`
	// OutputFormat is the format of the tool's output ("text", "json", etc).
	OutputFormat string `json:"output_format"`
}

// FlagDescriptor describes a single CLI flag.
type FlagDescriptor struct {
	// Name is the flag name without the leading "--".
	Name string `json:"name"`
	// Type is the Go type string ("bool", "string", "int", etc).
	Type string `json:"type"`
	// Default is the string representation of the default value.
	Default string `json:"default"`
	// Description is the flag's help text.
	Description string `json:"description"`
}

// DiscoveryDoc is the top-level machine-readable discovery document.
type DiscoveryDoc struct {
	// SchemaVersion identifies the discovery document format version.
	SchemaVersion string `json:"schema_version"`
	// GeneratedAt is the ISO-8601 timestamp of document generation.
	GeneratedAt string `json:"generated_at"`
	// Application is the name of the application.
	Application string `json:"application"`
	// ToolCount is the total number of registered tools.
	ToolCount int `json:"tool_count"`
	// CategoryCounts maps category names to tool counts.
	CategoryCounts map[string]int `json:"category_counts"`
	// Tools is the full list of tool descriptors, sorted by name.
	Tools []ToolDescriptor `json:"tools"`
	// Flags is the list of known CLI flags.
	Flags []FlagDescriptor `json:"flags"`
	// SENSEVersion is the SENSE middleware version.
	SENSEVersion string `json:"sense_version"`
}

// ToolRegistryView is the read-only interface the DiscoveryBuilder requires
// from the tool registry.  It is defined here (not in pkg/tools) to avoid
// import cycles.
type ToolRegistryView interface {
	// ListAll returns all registered tool descriptors.
	ListAll() []ToolDescriptor
}

// DiscoveryBuilder constructs DiscoveryDoc instances from a live tool registry.
type DiscoveryBuilder struct {
	view ToolRegistryView
}

// NewDiscoveryBuilder creates a builder backed by the given registry view.
func NewDiscoveryBuilder(view ToolRegistryView) *DiscoveryBuilder {
	return &DiscoveryBuilder{view: view}
}

// Build constructs and returns a DiscoveryDoc with the current tool set and
// the provided CLI flags.
func (b *DiscoveryBuilder) Build(flags []FlagDescriptor) *DiscoveryDoc {
	tools := b.view.ListAll()

	catCounts := make(map[string]int)
	for _, t := range tools {
		catCounts[t.Category]++
	}

	return &DiscoveryDoc{
		SchemaVersion:  DiscoveryVersion,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Application:    "Gorkbot",
		ToolCount:      len(tools),
		CategoryCounts: catCounts,
		Tools:          tools,
		Flags:          flags,
		SENSEVersion:   SENSEVersion,
	}
}

// MarshalPretty serialises the DiscoveryDoc to indented JSON.
func (d *DiscoveryDoc) MarshalPretty() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// MarshalCompact serialises the DiscoveryDoc to compact JSON.
func (d *DiscoveryDoc) MarshalCompact() ([]byte, error) {
	return json.Marshal(d)
}

// Summary returns a human-readable Markdown summary of the discovery document.
func (d *DiscoveryDoc) Summary() string {
	var sb = new(summaryBuilder)
	sb.h1("SENSE Discovery Document")
	sb.line(fmt.Sprintf("**Schema Version:** %s  ", d.SchemaVersion))
	sb.line(fmt.Sprintf("**Generated:** %s  ", d.GeneratedAt))
	sb.line(fmt.Sprintf("**Application:** %s  ", d.Application))
	sb.line(fmt.Sprintf("**Total Tools:** %d  ", d.ToolCount))
	sb.line(fmt.Sprintf("**CLI Flags:** %d  ", len(d.Flags)))
	sb.line("")

	sb.h2("Tool Categories")
	sb.line("| Category | Count |")
	sb.line("|----------|-------|")
	for cat, count := range d.CategoryCounts {
		sb.line(fmt.Sprintf("| %s | %d |", cat, count))
	}
	sb.line("")

	sb.h2("All Tools")
	sb.line("| Name | Category | Description |")
	sb.line("|------|----------|-------------|")
	for _, t := range d.Tools {
		desc := t.Description
		if len(desc) > 80 {
			desc = desc[:77] + "…"
		}
		sb.line(fmt.Sprintf("| `%s` | %s | %s |", t.Name, t.Category, desc))
	}
	sb.line("")

	if len(d.Flags) > 0 {
		sb.h2("CLI Flags")
		sb.line("| Flag | Type | Default | Description |")
		sb.line("|------|------|---------|-------------|")
		for _, f := range d.Flags {
			sb.line(fmt.Sprintf("| `--%s` | %s | `%s` | %s |",
				f.Name, f.Type, f.Default, f.Description))
		}
	}

	return sb.String()
}

// summaryBuilder is a tiny string builder helper.
type summaryBuilder struct {
	lines []string
}

func (s *summaryBuilder) h1(t string)   { s.lines = append(s.lines, "# "+t, "") }
func (s *summaryBuilder) h2(t string)   { s.lines = append(s.lines, "## "+t, "") }
func (s *summaryBuilder) line(l string) { s.lines = append(s.lines, l) }
func (s *summaryBuilder) String() string {
	result := ""
	for _, l := range s.lines {
		result += l + "\n"
	}
	return result
}

// KnownCLIFlags returns the set of well-known Gorkbot CLI flags as
// FlagDescriptors.  This is the authoritative list for the discovery document.
func KnownCLIFlags() []FlagDescriptor {
	return []FlagDescriptor{
		{Name: "p", Type: "string", Default: "", Description: "One-shot prompt: send a single message and exit"},
		{Name: "stdin", Type: "bool", Default: "false", Description: "Read prompt from stdin (one-shot mode)"},
		{Name: "output", Type: "string", Default: "", Description: "Write one-shot response to file instead of stdout"},
		{Name: "allow-tools", Type: "string", Default: "", Description: "Comma-separated list of tools allowed in one-shot mode"},
		{Name: "deny-tools", Type: "string", Default: "", Description: "Comma-separated list of tools denied in one-shot mode"},
		{Name: "timeout", Type: "duration", Default: "60s", Description: "Timeout for the operation"},
		{Name: "trace", Type: "bool", Default: "false", Description: "Enable execution trace logging to LogDir/traces/"},
		{Name: "watchdog", Type: "bool", Default: "false", Description: "Enable orchestrator state watchdog / debug logging"},
		{Name: "verbose-thoughts", Type: "bool", Default: "false", Description: "Show consultant (Gemini) verbose thinking in TUI"},
		{Name: "model", Type: "string", Default: "", Description: "Override the primary AI model ID"},
		{Name: "provider", Type: "string", Default: "", Description: "Override the primary AI provider"},
		{Name: "dry-run", Type: "bool", Default: "false", Description: "Enable SENSE dry-run mode: validate tools without executing"},
		{Name: "describe", Type: "bool", Default: "false", Description: "Output the machine-readable JSON schema of the CLI"},
		{Name: "output-format", Type: "string", Default: "text", Description: "Format for one-shot mode output (e.g. 'json' or 'text')"},
		{Name: "no-tui", Type: "bool", Default: "false", Description: "Disable TUI; use plain text I/O"},
		{Name: "log-level", Type: "string", Default: "info", Description: "Logging verbosity: debug | info | warn | error"},
		{Name: "config-dir", Type: "string", Default: "", Description: "Override the configuration directory path"},
		{Name: "version", Type: "bool", Default: "false", Description: "Print version information and exit"},
	}
}
