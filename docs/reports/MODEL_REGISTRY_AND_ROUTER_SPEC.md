# Technical Specification: Model Registry & Intelligent Router

**Date:** February 16, 2026
**Project:** Grokster CLI
**Status:** Draft / Proposal

---

## 1. Executive Summary

Current architecture relies on a hardcoded "Primary" (Grok) and "Consultant" (Gemini) relationship within the `Orchestrator`. While functional, this is brittle and limits scalability.

This specification outlines a modular **Model Registry** to serve as the single source of truth for available models, and an **Intelligent Router** to dynamically select the optimal model based on prompt complexity, context length, and user intent.

---

## 2. Component 1: The Model Registry (`pkg/registry`)

The Registry acts as the database of available intelligence. It decouples the *definition* of a model from its *instantiation*.

### 2.1 Core Structures

```go
package registry

type ModelID string

const (
    Grok3        ModelID = "grok-3"
    GeminiFlash  ModelID = "gemini-2.0-flash"
    GeminiPro    ModelID = "gemini-1.5-pro"
    // Future expansion
    ClaudeSonnet ModelID = "claude-3-5-sonnet"
    LocalLlama   ModelID = "llama-3-8b"
)

type Capabilities struct {
    ContextWindow int
    SupportsImages bool
    SupportsTools  bool
    Streaming      bool
}

type ModelProfile struct {
    ID           ModelID
    Provider     string // "xai", "google", "anthropic"
    Name         string // Display name
    Description  string
    Capabilities Capabilities
    CostTier     int // 1=Cheap, 2=Standard, 3=Premium
}
```

### 2.2 The Registry Interface

```go
type Registry interface {
    // Register a new provider/model pair
    Register(profile ModelProfile, factory func() ai.AIProvider)
    
    // Retrieve a specific model provider
    GetProvider(id ModelID) (ai.AIProvider, error)
    
    // List all available models (for CLI selection/menus)
    ListModels() []ModelProfile
}
```

---

## 3. Component 2: The Intelligent Router (`pkg/router`)

The Router is the decision engine. Instead of the Orchestrator hardcoding logic like `if len(prompt) > 1000`, the Router evaluates the request and returns a `RoutePlan`.

### 3.1 Routing Strategy

The router will assess three dimensions:

1.  **Context Load:**
    *   *Low (< 8k tokens):* Preferred for any model.
    *   *High (> 100k tokens):* Forces usage of High-Context models (Gemini Pro/Flash).

2.  **Complexity Heuristics:**
    *   *Simple:* "Hello", "What time is it?", "Fix this typo." -> **Fast Tier** (Gemini Flash / Grok-2).
    *   *Reasoning:* "Architect a system...", "Debug this race condition...", "Explain quantum physics." -> **Reasoning Tier** (Grok-3 / Gemini Pro).

3.  **Tool Necessity:**
    *   If specific tools (e.g., Vision/Image analysis) are required, filter for `SupportsImages: true`.

### 3.2 The Router Interface

```go
package router

type RouteRequest struct {
    Prompt      string
    HistorySize int // Approximate token count
    ToolsNeeded []string
}

type RouteDecision struct {
    PrimaryID    registry.ModelID
    Reasoning    string // Why this model was chosen
    FallbackID   registry.ModelID // If primary fails
}

type Router interface {
    // Analyze and decide
    Route(req RouteRequest) (*RouteDecision, error)
}
```

---

## 4. Architectural Data Flow

**Current Flow:**
`Main` -> `Orchestrator` -> `Primary (Grok)` [Maybe calls Consultant]

**Proposed Flow:**
1.  `Main` initializes `Registry` (loads API keys, registers providers).
2.  `Main` initializes `Router` (with default heuristics).
3.  `Orchestrator` receives user input.
4.  `Orchestrator` asks `Router`: "Who should handle this?"
    *   *Router:* "This is a 200k token file read. Use Gemini Pro."
5.  `Orchestrator` requests `Gemini Pro` instance from `Registry`.
6.  `Orchestrator` executes task.

---

## 5. Implementation Roadmap

### Phase 1: Refactoring `pkg/ai`
*   Standardize the `AIProvider` interface (completed).
*   Ensure all providers (Grok, Gemini) implement the full interface including `Ping` and metadata reporting.

### Phase 2: Build the Registry
*   Create `pkg/registry`.
*   Move hardcoded provider setup from `cmd/grokster/main.go` into a registry initialization function.

### Phase 3: Build the Router
*   Create `pkg/router`.
*   Move the logic currently inside `orchestrator.go` (keywords: "COMPLEX", "REFRESH", length checks) into the Router implementation.

### Phase 4: Integration
*   Inject `Registry` and `Router` into `Orchestrator`.
*   Update TUI to display *which* model was selected dynamically (e.g., "🤖 Routing to Gemini Pro [Reason: High Context]").

---

## 6. Benefits

1.  **Cost Efficiency:** Automatically route simple tasks to cheaper/faster models (Flash) and complex tasks to smarter ones (Grok-3).
2.  **Resilience:** If xAI API is down, the Router can automatically fallback to Google.
3.  **Extensibility:** Adding Local LLMs (Ollama) or Claude becomes a simple registration line, without touching the Orchestrator logic.
