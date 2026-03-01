# Gorkbot Provider Guide

**Version:** 3.4.0

This document covers all five AI providers supported by Gorkbot — model IDs, capability classes, API setup, dynamic model discovery, hot-swapping, and the adaptive routing system.

---

## Table of Contents

1. [Supported Providers](#1-supported-providers)
2. [Model Selection Logic](#2-model-selection-logic)
3. [xAI (Grok)](#3-xai-grok)
4. [Google Gemini](#4-google-gemini)
5. [Anthropic (Claude)](#5-anthropic-claude)
6. [OpenAI](#6-openai)
7. [MiniMax](#7-minimax)
8. [Hot-Swapping Models at Runtime](#8-hot-swapping-models-at-runtime)
9. [Dynamic Model Discovery](#9-dynamic-model-discovery)
10. [Cloud Brains Tab](#10-cloud-brains-tab)
11. [Adaptive Routing & Feedback](#11-adaptive-routing--feedback)
12. [Dual-Model Orchestration](#12-dual-model-orchestration)

---

## 1. Supported Providers

| Provider | ID | Primary Use | API Key Env Var |
|----------|----|------------|----------------|
| xAI | `xai` | Primary agent (Grok) | `XAI_API_KEY` |
| Google | `google` | Specialist consultant (Gemini) | `GEMINI_API_KEY` |
| Anthropic | `anthropic` | Alternative primary or specialist | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | Alternative primary or specialist | `OPENAI_API_KEY` |
| MiniMax | `minimax` | Alternative provider | `MINIMAX_API_KEY` |

At least one provider with a valid key is required. Gorkbot works with any subset of providers.

---

## 2. Model Selection Logic

Gorkbot selects primary and specialist models through a layered decision process:

```
Priority 1: Environment variable overrides
  GORKBOT_PRIMARY_MODEL=grok-3-mini ./gorkbot.sh
  → Skips all dynamic selection; uses the named model directly.

Priority 2: Persisted app state (app_state.json)
  Restored after dynamic selection completes.
  If the saved model is no longer available, falls back to dynamic selection.

Priority 3: Dynamic selection (Router.SelectSystemModels)
  → Queries all registered providers for live model lists.
  → Ranks models by capability, latency, and cost.
  → Returns SystemConfiguration{PrimaryModel, SpecialistModel}.

Priority 4: Defaults
  Primary:    baseGrok (default model from GrokProvider)
  Specialist: baseGemini (default model from GeminiProvider)
```

### Capability Classes

Dynamic selection and `spawn_sub_agent` use capability classes to route tasks to the best model:

| Class | Description | Examples |
|-------|-------------|---------|
| `General` | Balanced all-round | grok-3, gemini-2.0-flash |
| `Reasoning` | Deep thinking, planning | grok-3, claude-3-7-sonnet |
| `Speed` | Low latency, simple tasks | grok-3-mini, gemini-flash |
| `Coding` | Code generation and review | grok-3, claude-3-7-sonnet |

---

## 3. xAI (Grok)

**Package:** `pkg/ai/grok.go`
**API Base:** `https://api.x.ai/v1`
**Key env var:** `XAI_API_KEY`

### Available Models

| Model ID | Description | Context |
|----------|-------------|---------|
| `grok-3` | Most capable Grok model | 128k tokens |
| `grok-3-mini` | Fast, cost-efficient | 128k tokens |
| `grok-3-fast` | Optimized throughput | 128k tokens |
| `grok-2-vision-1212` | Vision-capable (image analysis) | 32k tokens |

> Model availability depends on your xAI plan. Check [console.x.ai](https://console.x.ai) for current model list.

### Special Features

- **Native function calling** — xAI's structured `tool_calls` API is fully implemented. When Grok is primary, tool requests come as structured JSON (not parsed text), improving reliability.
- **Streaming** — Real-time token streaming via SSE.
- **Usage reporting** — `GrokProvider.GetLastUsage()` returns `TokenUsage{PromptTokens, CompletionTokens}` for billing.
- **Thinking models** — Models supporting `reasoning_effort` are tagged `SupportsThinking=true` and displayed with a reasoning indicator in the model selection UI.
- **x_pull tool** — Fetch content from X (Twitter) posts using the xAI API.

### Vision

Gork Vision uses `grok-2-vision-1212` via the `vision_screen`, `vision_file`, `vision_ocr`, and related tools. Images are sent as base64 data URIs in the messages array.

### Getting an xAI API Key

1. Go to [console.x.ai](https://console.x.ai)
2. Sign in or create an account
3. Navigate to **API Keys**
4. Click **Create API Key**
5. Copy the key (starts with `xai-`)

---

## 4. Google Gemini

**Package:** `pkg/ai/gemini.go`
**API Base:** `https://generativelanguage.googleapis.com/v1beta`
**Key env var:** `GEMINI_API_KEY`

### Available Models

| Model ID | Description | Context |
|----------|-------------|---------|
| `gemini-2.0-flash` | Fast, efficient (recommended default) | 1M tokens |
| `gemini-2.0-flash-thinking-exp` | Extended reasoning mode | 1M tokens |
| `gemini-1.5-pro` | Previous generation pro | 2M tokens |
| `gemini-1.5-flash` | Previous generation fast | 1M tokens |

> Gemini model availability varies by region and plan. Check [aistudio.google.com](https://aistudio.google.com) for the current list.

### Special Features

- **Verbose thoughts** — Enable with `--verbose-thoughts` flag to display Gemini's chain-of-thought reasoning in consultant response boxes.
- **Thinking config** — Supported models have `reasoning_effort` enabled for extended internal reasoning before responding.
- **Large context** — Gemini supports up to 2M tokens of context, making it excellent for whole-codebase analysis.
- **Role mapping** — `"assistant"` maps to `"model"` in Gemini's API; `"system"` messages are included as user messages with special formatting.

### Getting a Gemini API Key

1. Go to [aistudio.google.com/apikey](https://aistudio.google.com/apikey)
2. Sign in with your Google account
3. Click **Create API Key**
4. Select or create a Google Cloud project
5. Copy the key (starts with `AIzaSy`)

---

## 5. Anthropic (Claude)

**Package:** `pkg/ai/anthropic.go`
**API Base:** `https://api.anthropic.com/v1`
**Key env var:** `ANTHROPIC_API_KEY`

### Available Models

| Model ID | Description | Context |
|----------|-------------|---------|
| `claude-opus-4-6` | Most capable (Opus) | 200k tokens |
| `claude-sonnet-4-6` | Balanced (Sonnet) | 200k tokens |
| `claude-haiku-4-5-20251001` | Fast (Haiku) | 200k tokens |

> Model IDs include dates in some cases. The discovery system fetches the current list from `GET /v1/models`.

### Special Features

- **SSE streaming** — Real-time token streaming.
- **Anthropic headers** — Requests include `x-api-key` and `anthropic-version: 2023-06-01` headers.
- **Extended thinking** — Claude Sonnet 3.7+ supports extended thinking; models are tagged `SupportsThinking=true`.

### Getting an Anthropic API Key

1. Go to [console.anthropic.com](https://console.anthropic.com)
2. Create an account or sign in
3. Navigate to **API Keys**
4. Create a new key (starts with `sk-ant-`)

---

## 6. OpenAI

**Package:** `pkg/ai/openai_provider.go`
**API Base:** `https://api.openai.com/v1`
**Key env var:** `OPENAI_API_KEY`

### Available Models

| Model ID | Description | Context |
|----------|-------------|---------|
| `gpt-4o` | Latest GPT-4o | 128k tokens |
| `gpt-4o-mini` | Fast GPT-4o mini | 128k tokens |
| `o4-mini` | o-series reasoning | 128k tokens |
| `o3` | Advanced reasoning | 128k tokens |
| `o1` | First reasoning model | 128k tokens |

> `o1`/`o3`/`o4` series are tagged `SupportsThinking=true`. Non-chat models are filtered out during discovery.

### Special Features

- **OpenAI-compatible** — Uses the standard chat completions API; compatible with any OpenAI-compatible endpoint by changing the base URL.
- **o-series detection** — `o1`, `o3`, `o4` prefix detection tags models as reasoning-capable.
- **Model filtering** — `embeddings`, `tts`, `whisper`, `dall-e`, and `babbage` models are excluded from the chat model list.

### Getting an OpenAI API Key

1. Go to [platform.openai.com](https://platform.openai.com)
2. Navigate to **API Keys**
3. Create a new key (starts with `sk-`)

---

## 7. MiniMax

**Package:** `pkg/ai/minimax.go`
**API Base:** `https://api.minimax.io/anthropic/v1` (Anthropic-compatible endpoint)
**Key env var:** `MINIMAX_API_KEY`

MiniMax exposes an Anthropic-compatible API, so `MiniMaxProvider` wraps `AnthropicProvider` with a custom base URL. Model discovery uses an OpenAI-compatible listing endpoint with a static fallback.

### Getting a MiniMax API Key

Register at [minimax.io](https://minimax.io) and generate an API key from the developer console.

---

## 8. Hot-Swapping Models at Runtime

Switch providers and models without restarting Gorkbot.

### Via TUI Model Selection (`Ctrl+T`)

1. Press `Ctrl+T` to open the dual-pane model selection
2. Use `Tab` to switch between Primary and Specialist panes
3. Navigate with `↑`/`↓` or `k`/`j`
4. Press `Enter` to select
5. Press `r` to refresh model lists from all providers
6. Press `p` to cycle provider filter
7. Press `k` to add/update an API key

### Via `/model` Command

```
/model                              # show current models
/model primary grok-3-mini          # switch primary
/model consultant claude-sonnet-4-6 # switch specialist
/model consultant auto              # enable auto-select specialist
```

### Via `/key` Command

Set or update API keys at runtime:

```
/key xai xai-your-new-key
/key google AIza-your-new-key
/key anthropic sk-ant-your-key
/key openai sk-your-key
/key status                # show key status for all providers
/key validate xai          # validate xAI key
```

### Via Environment Variable Override

Set model overrides before launch to bypass all selection logic:

```bash
GORKBOT_PRIMARY_MODEL=claude-sonnet-4-6 \
GORKBOT_CONSULTANT_MODEL=grok-3 \
./gorkbot.sh
```

---

## 9. Dynamic Model Discovery

The `pkg/discovery.Manager` polls all configured providers for live model lists on startup and every 30 minutes.

### How It Works

```
discovery.Manager.Start(ctx)
  → goroutine: poll every 30 minutes
  → fetchXAIModels()     → GET https://api.x.ai/v1/models
  → fetchGeminiModels()  → GET https://generativelanguage.googleapis.com/v1beta/models
  → fetchAnthropicModels() → GET https://api.anthropic.com/v1/models
  → fetchOpenAIModels()  → GET https://api.openai.com/v1/models
  → fetchMiniMaxModels() → static list + API query
  → classify each model by ID keywords → CapabilityClass
  → send DiscoveryUpdateMsg to TUI
```

### Model Classification

Models are classified by matching their IDs against keyword patterns:

| Keyword patterns | CapabilityClass |
|-----------------|----------------|
| `reasoning`, `thinking`, `o1`, `o3`, `o4`, `opus` | `Reasoning` |
| `mini`, `haiku`, `flash`, `fast`, `nano` | `Speed` |
| `code`, `codex`, `starcoder`, `deepsek` | `Coding` |
| (everything else) | `General` |

### BestForCap

`Manager.BestForCap(class)` returns the highest-ranked available model for a capability class. Used by `spawn_sub_agent` to automatically select the best model for the delegated task type.

---

## 10. Cloud Brains Tab

**Shortcut:** `Ctrl+D`

The Cloud Brains tab shows:

**Left panel — Discovered Models:**
- All discovered models grouped by provider and capability class
- Availability status (requires valid API key)
- Whether each model supports thinking/reasoning

**Right panel — Agent Tree:**
- Hierarchical view of active sub-agent delegations
- Shows which models are handling which sub-tasks
- Updated in real-time via `DiscoveryUpdateMsg`

---

## 11. Adaptive Routing & Feedback

The adaptive routing system learns from user feedback which models perform best for different task categories.

### Rating Responses

```
/rate 5    # excellent
/rate 3    # average
/rate 1    # poor
```

Each rating is recorded to `~/.config/gorkbot/feedback.jsonl` by `router.FeedbackManager`:

```json
{"timestamp":"2025-11-15T10:30:00Z","model":"grok-3","task_category":"code_review","score":5.0}
{"timestamp":"2025-11-15T10:30:05Z","model":"gemini-2.0-flash","task_category":"architecture","score":4.0}
```

### How Routing Uses Feedback

Before each task, `router.AdaptiveRouter.SuggestModel(category)` queries the feedback store for the best-performing model in the task's category. This suggestion is logged and optionally used to override the default selection.

### Task Categories

Categories are inferred from the task prompt using keyword detection:

| Category | Keywords |
|----------|---------|
| `code_review` | review, audit, check, quality |
| `code_generation` | write, create, implement, generate |
| `architecture` | design, architect, system, structure |
| `debugging` | bug, error, fix, debug |
| `research` | explain, what is, how does, research |
| `data_analysis` | data, analyze, csv, statistics |

---

## 12. Dual-Model Orchestration

Gorkbot's default configuration uses two models working together:

```
User prompt
    │
    ▼
Consultant needed? (isConsultant heuristic)
    │
    ├── No → Primary only
    │         Grok handles everything
    │
    └── Yes → Consultant first (Gemini)
              ├── Gemini provides architectural advice / analysis
              ├── Advice is appended to context
              └── Primary (Grok) generates the final response
                  incorporating the consultant's perspective
```

### Consultant Triggers

The `isConsultant` heuristic evaluates:

1. **Keyword triggers** — prompt contains `COMPLEX`, `REFRESH`, `ARCHITECTURE`, `DESIGN`
2. **Length threshold** — prompt longer than ~1000 characters
3. **ARC classification** — `WorkflowReasonVerify` classification routes to consultant

When triggered, the consultant's response appears in a bordered box in the TUI:

```
╭──────────────────────────────────────────────────╮
│  Specialist (Gemini)                             │
│                                                  │
│  Architectural recommendation: Use an event-     │
│  driven approach with CQRS for complex domain... │
╰──────────────────────────────────────────────────╯
```

### Auto Specialist Mode

Set `secondary_auto: true` in `app_state.json` or use `/model consultant auto` to let the ARC Router and discovery system pick the best specialist model per task, rather than always using the configured consultant.

### Using Only One Provider

Gorkbot works with a single provider. If only `XAI_API_KEY` is set:
- Primary: Grok
- Specialist: None (single-model mode; no consultant responses)

If only `GEMINI_API_KEY` is set:
- Primary: Gemini (acts as both primary and specialist)
- All tool calls routed through the text-parsing path (not native function calling)
