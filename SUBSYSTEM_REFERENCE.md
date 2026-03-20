# Subsystem Reference Guide

Detailed guide to Gorkbot's advanced reasoning and learning subsystems.

---

## Table of Contents

1. [SENSE](#sense-stability--enlightened-stabilization)
2. [SPARK](#spark-self-propelling-autonomous-reasoning)
3. [SRE](#sre-step-wise-reasoning-engine)
4. [XSKILL](#xskill-continual-learning)
5. [ARC Router](#arc-router-adaptive-reasoning-classifier)
6. [MEL](#mel-multi-evidential-learning)
7. [CCI](#cci-codified-context-infrastructure)

---

## SENSE (Stability & Enlightened Stabilization)

### Version: 1.9.0

**Purpose**: Stabilize input/output and detect anomalies

### Components

#### InputSanitizer
Validates all user inputs before processing:

**Checks**:
- Path traversal protection (whitelist-based)
- ANSI escape code filtering
- SQL injection patterns
- Shell metacharacter validation

**Usage**:
```go
inputSanitizer.SanitizeParams(params)  // Raises error if invalid
```

#### LIE (Language Interrogation Engine)
Detects neural hallucinations in AI responses:

**Detection Methods**:
- Tool reference validation (does tool exist?)
- Fact contradiction (conflicts with prior context?)
- Fabrication signals (stats out of bounds?)

**Returns**: Confidence score (0-1) + explanation

#### Stabilizer
Smooths metrics using exponential moving average:

**Tracks**:
- Token latency
- Error rates
- Tool success rates

**Alpha**: 0.95 (configurable decay)

#### Compressor
Automatic context compression when token limit nears:

**Stages**:
1. Truncate verbose tool outputs (>500 chars)
2. Compress older turns with SENSE heuristics
3. Merge similar conversation pairs

**Trigger**: 85% of model context limit

#### AgeMem (Age-based Memory)
Decays memories by access frequency:

**Fields per memory**:
- Created timestamp
- Last accessed
- Frecency score (frequency + recency)

**Decay**: Older, less-accessed memories weighted lower

#### EngramStore
Persistent episodic memories (facts + decisions):

**Structure**: ~20-line markdown files per engram
**Indexed by**: Semantic hash + timestamp
**Loaded**: Session start if recent + high confidence

#### TraceAnalyzer
Real-time JSONL trace parsing for anomaly detection:

**Events Tracked**:
- tool_success, tool_failure
- hallucination, context_overflow
- provider_error, sanitizer_reject

**Metrics**:
- Error rate trend
- Hallucination frequency
- Provider SLA compliance

#### SENSETracer
Daily-rotated JSONL logging:

**Location**: `~/.config/gorkbot/trace/<YYYY-MM-DD>.jsonl`
**Buffer**: 512 entries (silent drop if exceeded)
**Event Types**: 63+ (tool execution, errors, provider issues, etc.)

#### OutputFilter
Category-based message suppression:

**Categories**:
- `ToolNarration`: "I will now execute..."
- `ToolStatus`: "Tool completed"
- `InternalReason`: "I chose bash because..."
- `DebugInfo`: "Context tokens: 5000"
- `SystemStatus`: "Context overflow detected"
- `CooldownNotice`: "Rate limited for 30s"

**Toggle**: `/verbose <on|off>`

### Configuration

```json
{
  "sandbox_enabled": true,
  "suppression_config": {
    "ToolNarration": true,
    "ToolStatus": true,
    "InternalReason": true,
    "DebugInfo": true
  }
}
```

---

## SPARK (Self-Propelling Autonomous Reasoning)

### Version: Integrated

**Purpose**: Autonomous 8-step self-improvement cycle

### 8-Step Cycle

**Triggered**: Post-task via `ObserveSuccess()`, non-blocking

#### 1. TII (Task Introspection Interface)
Analyze just-completed task:
- Time taken
- Tool usage pattern
- Success rate
- **Output**: Efficiency score (0-1, alpha=0.95 decay)

#### 2. IDL (Improvement Debt Ledger)
Track what could be better:
- "Why did tool Y fail?"
- Improvement records
- **Storage**: ~200 entries max, oldest pruned

#### 3. LIE Integration
Hallucination detection feedback:
- If hallucination found → boost suspicion for future
- Feeds into confidence scoring

#### 4. MotivationalCore
Compute drive/motivation metric:
- **Input**: LIE confidence, success rate, efficiency
- **Output**: 0-100 motivation score
- **Effect**: Feeds action selection heuristics

#### 5. ResearchModule
Formulate self-study objectives:
- LLM-generated: "Research why bash tools failed in domain X"
- **Max**: 10 concurrent objectives
- **Optional**: Requires LLMProvider wiring

#### 6. DiagnosisKernel
Analyze recent trace events:
- Pattern detection
- Root cause analysis
- **Output**: Recommendations (modify tool guards, etc.)

#### 7. Introspector
Self-reflection on limitations:
- "I made 5 errors; focus on safety"
- Feeds into next session's behavior
- **Factors**: TII + IDL + AgeMem + MotivationalCore + ResearchModule

#### 8. Phase Output
Generate directives for next session:
- ModifyToolGate: Tighten tool access
- InjectExpertMode: Enable reasoning
- etc.
- **Non-blocking**: Can be ignored

### Configuration

```go
type Config struct {
    ConfigDir          string        // Data directory
    TIIAlpha           float64       // Decay factor (0.95)
    MaxIDLEntries      int           // Max improvement records (200)
    DriveAlpha         float64       // Motivation decay (0.9)
    LLMObjectiveEnabled bool         // Enable LLM objectives
    ResearchObjectiveMax int         // Max concurrent objectives (10)
}
```

### Enabling SPARK

In main.go:
```go
orch.SPARK = spark.New(cfg, lie, analyzer, ageMem, aiProvider, logger)
```

---

## SRE (Step-wise Reasoning Engine)

### Version: Integrated

**Purpose**: Decompose complex queries into hypothesis → test → validate cycles

### 4-Phase Workflow

#### Phase 1: Ground
Extract world model from user prompt:
- Calls primary provider
- Structures facts
- Stores in AnchorLayer (priority 1.0)
- Committed to trace as `KindSREGrounding`

#### Phase 2: Hypothesis
Generate N candidate hypotheses:
- Config: `cfg.HypothesisTurns` (e.g., 3 hypotheses)
- Explores solution space

#### Phase 3: Test
Formulate test for each hypothesis:
- How to validate each?
- Configuration: `cfg.PruneTurns`

#### Phase 4: Validate
Confirm best hypothesis via sampling:
- Corrects any drift from grounding
- Triggers backtrack if deviation detected
- Cosine distance threshold: 0.42

### Sub-Components

#### AnchorLayer
Anchors extracted facts in AgeMem:
- Priority: 1.0 (highest)
- Prevents drift during reasoning
- Checked during CoS correction

#### GroundingExtractor
Extracts structured world model:
- Calls provider
- Returns `WorldModelState` struct

#### CoSEngine (Chain of Schedules)
Manages phase transitions:
- Config: `cfg.HypothesisTurns`, `cfg.PruneTurns`

#### CorrectionEngine
Detects deviation from original grounding:
- Triggers backtrack if distance > threshold (0.42)
- Records correction event to trace

#### EnsembleManager
Runs multiple trajectories in parallel:
- Merges results with voting
- Config: `cfg.EnsembleEnabled`

### Configuration

```go
type SREConfig struct {
    GroundingEnabled     bool
    CoSEnabled           bool
    EnsembleEnabled      bool
    HypothesisTurns      int     // N hypotheses (3)
    PruneTurns           int     // N to keep (2)
    CorrectionThresh     float64 // Distance threshold (0.42)
}
```

### Integration Point

```go
// Per-task
orch.SRE.Reset()                           // Reset phases
orch.SRE.Ground(ctx, prompt)               // Extract world model
// ...
orch.SRE.Validate(ctx, finalAnswer)        // Confirm coherence
```

---

## XSKILL (Continual Learning)

### Version: 1.0.0

**Purpose**: Accumulate tactical experiences, apply to future tasks

### Phase 1: Accumulation
Runs post-task, async (non-blocking):

**Captures**:
- Tactical patterns from task
- Environmental context
- Success/failure outcome

**Storage** (`~/.gorkbot/xskill_kb/`):
- `experiences.json`: Experience Bank
- `skills/<skillName>.md`: Domain-specific guides

**Structure**:
```json
{
    "id": "E1234",
    "timestamp": "2026-03-20T10:30:00Z",
    "skillName": "visual-logic",
    "task": "Identify objects in image",
    "outcome": "Success",
    "pattern": "Use OCR before semantic analysis",
    "tokens": 2500
}
```

### Phase 2: Inference
Runs pre-generation (before each LLM call):

**Process**:
1. Retrieve similar experiences (vector similarity)
2. Embed via injected LLMProvider
3. Inject context into system prompt
4. Modulate behavior based on match confidence

**Injection Example**:
```
"In the past, for visual-logic tasks, you often:
1. Extract text with OCR first
2. Then perform semantic analysis
This approach succeeded 92% of the time."
```

### Phase 3: Hot-Swap Embedder
Upgrade embedder at runtime:

```go
orch.UpgradeXSkillEmbedder(newEmbedder)  // Switch live, no restart
```

**Supports**:
- Anthropic embeddings
- OpenAI embeddings
- Native LLM (llamacpp)

### Configuration

```go
xskill.NewKnowledgeBase(
    "~/.gorkbot/xskill_kb",  // Base directory
    embeddingProvider,        // LLMProvider for embedding
)
```

---

## ARC Router (Adaptive Reasoning Classifier)

### Version: Integrated

**Purpose**: Route tasks to appropriate handler based on input features

### Classification Pipeline

```
Input prompt
  ↓
IngressFilter: Prune low-information content
  ↓
IngressGuard: Verify semantic preservation after pruning
  ↓
ARC Classifier:
  - Extract features (task type, complexity, domain, urgency)
  - TF-IDF vectorization
  - Historical decision scoring (BM25)
  - Compute classification confidence (0-1)
  - Classify action type (NORMAL/PLAN/AUTOEDIT/SECURITY)
  ↓
Confidence Guard:
  If confidence < 0.25 → Use RoutingTable fallback
  ↓
Action Dispatch:
  - NORMAL: Execute normally
  - PLAN: Generate plan first
  - AUTOEDIT: Auto-modify without asking
  - SECURITY: Route to redteam-recon
```

### Sub-Components

#### IngressFilter
Prunes low-information content:
- Removes duplicates
- Strips redundant context
- Preserves critical facts

#### IngressGuard
Semantic preservation validation:
- Checks: Does filtered content maintain semantic meaning?
- Prevents classifier evasion attacks

#### ARC Classifier
Task classification engine:
- Feature extraction (20+ features)
- Historical learning (BM25 scoring)
- Confidence computation
- Action classification

#### RoutingTable
Regex-based fallback:
```
"security audit" → redteam-recon
"deploy to prod" → deploy-specialist
"profile performance" → perf-specialist
```

### Configuration

```go
type ARCBudget struct {
    TokensPerDecision int    // Cost tracking (100)
    ConfidenceGate float64   // Minimum confidence (0.25)
}
```

---

## MEL (Multi-Evidential Learning)

### Version: Integrated

**Purpose**: Learn from successes via semantic vector store

### Learning Pipeline

```
Success observed
  ↓
MELValidator: Prevent poisoning attacks
  - Semantic sanity check
  - Consistency check against prior knowledge
  ↓
VectorStore: SQLite + embedding
  - Embed: "(tool=bash, params={...}, result=success)"
  - Store vector in heuristic index
  - BM25 + TFIDF ranking
  ↓
MEL Heuristic: Pre-compute patterns
  - "bash + git_push + small_diff → high_success"
  - Pattern extraction
  ↓
MELProjector: Inject into prompts
  - Project prompts into vector space
  - Find similar past successes
  - Inject as "reasoning examples"
```

### Components

#### MELValidator
Validates learning records:
- Semantic coherence check
- Contradiction detection
- Poisoning filters
- **Prevents**: Noisy/incorrect patterns

#### VectorStore
Semantic index:
- SQLite backend
- Embedding-based retrieval
- BM25 + TFIDF ranking
- Configurable TTL

#### MEL Heuristic
Pattern pre-computation:
- Frequent tool combinations
- Success correlations
- Time-series analysis

#### MELProjector
Prompt enrichment:
- Vector embedding
- Similarity matching
- Context injection

### Configuration

```go
orch.MELValidator = adaptive.NewMELValidator(...)
orch.Intelligence.VectorStore = vectorstore.NewVectorStore(db)
```

---

## CCI (Codified Context Infrastructure)

### Version: Integrated

**Purpose**: 3-tier persistent context injection

### 3 Memory Tiers

| Tier | Scope | Priority | Persistence |
|------|-------|----------|-------------|
| **Hot** | Current session | Highest (1.0) | In-memory |
| **Specialist** | Domain-specific rules | Medium (0.7) | Disk (per-domain) |
| **Cold** | Historical patterns | Lower (0.4) | Disk (historical) |

### Components

#### Hot Memory
Current session facts:
- Real-time decisions
- In-memory, 100-entry max
- Highest priority in system prompt
- Example: "User prefers Python 3.11"

#### Specialist Memory
Domain-specific knowledge:
- Loaded per-domain (e.g., "golang", "rust")
- ~20-50 entries per domain
- Medium priority
- Example: "Go fmt before commit"

#### Cold Memory
Historical patterns:
- Lessons learned
- Persisted to disk
- Queried on demand
- Lower priority but foundational
- Example: "Bash expansion issues with paths containing spaces"

#### Truth Sentry
Drift detection:
- Monitors semantic consistency
- Alerts if Hot contradicts Specialist/Cold
- Prevents conflicting guidance
- Correction: Suppress conflicting advice

### Injection Point

In `PromptBuilder`:
1. Inject Hot context (priority 1.0)
2. Inject Specialist context (priority 0.7)
3. Inject Cold context (priority 0.4)
4. Consolidate duplicates

### Configuration

Files in `~/.config/gorkbot/cci/`:
- `hot.md`
- `specialist-<domain>.md`
- `cold.md`

---

**For implementation details, see [DEVELOPMENT.md](DEVELOPMENT.md) and [ARCHITECTURE.md](ARCHITECTURE.md).**

