# Gorkbot Provider Guide

**Version:** 4.7.0

This document covers all AI providers supported by Gorkbot — model IDs, capability classes, API setup, dynamic model discovery, hot-swapping, and the adaptive routing system.

---

## Table of Contents

1. [Supported Providers](#1-supported-providers)
2. [Model Selection Logic](#2-model-selection-logic)
3. [xAI (Grok)](#3-xai-grok)
4. [Google Gemini](#4-google-gemini)
5. [Anthropic (Claude)](#5-anthropic-claude)
6. [OpenAI](#6-openai)
7. [MiniMax](#7-minimax)
8. [Moonshot](#8-moonshot)
9. [OpenRouter](#9-openrouter)
10. [Hot-Swapping Models at Runtime](#10-hot-swapping-models-at-runtime)
11. [Dynamic Model Discovery](#11-dynamic-model-discovery)
12. [Cloud Brains Tab](#12-cloud-brains-tab)
13. [Adaptive Routing and Feedback](#13-adaptive-routing-and-feedback)
14. [Dual-Model Orchestration](#14-dual-model-orchestration)
15. [Provider Failover Cascade](#15-provider-failover-cascade)
16. [Extended Thinking](#16-extended-thinking)

---

## 1. Supported Providers

| Provider | ID | Primary Use | API Key Env Var | Package |
|----------|----|------------|----------------|---------|
| xAI | `xai` | Primary agent (Grok) | `XAI_API_KEY` | `pkg/ai/grok.go` |
| Google | `google` | Specialist consultant (Gemini) | `GEMINI_API_KEY` | `pkg/ai/gemini.go` |
| Anthropic | `anthropic` | Alternative primary or specialist | `ANTHROPIC_API_KEY` | `pkg/ai/anthropic.go` |
| OpenAI | `openai` | Alternative primary or specialist | `OPENAI_API_KEY` | `pkg/ai/openai_provider.go` |
| MiniMax | `minimax` | Alternative provider | `MINIMAX_API_KEY` | `pkg/ai/minimax.go` |
| Moonshot | `moonshot` | Alternative provider | `MOONSHOT_API_KEY` | `pkg/ai/moonshot.go` |
| OpenRouter | `openrouter` | Gateway to 400+ models | `OPENROUTER_API_KEY` | `pkg/ai/openrouter.go` |

At least one provider with a valid key is required. Gorkbot works with any subset of providers.

---

## 2. Model Selection Logic

Gorkbot selects primary and specialist models through a layered decision process:

```
Priority 1: Environment variable overrides (highest)
  GORKBOT_PRIMARY_MODEL=grok-3-mini ./gorkbot.sh
  → Skips all dynamic selection; uses the named model directly.

Priority 2: Persisted app state (app_state.json)
  Restored after dynamic selection completes.
  If the saved model is no longer available, falls back to dynamic selection.

Priority 3: Dynamic selection (Router.SelectSystemModels)
  → Queries all registered providers for live model lists.
  → Ranks models by capability, cost, and provider priority.
  → Returns SystemConfiguration{PrimaryModel, SpecialistModel}.

Priority 4: Defaults
  Primary:    xAI Grok (latest available model)
  Specialist: Google Gemini (latest available model)
```

### Capability Classes

Dynamic selection and `spawn_sub_agent` use capability classes to route tasks to the best model:

| Class | Description | Example Models |
|-------|-------------|----------------|
| `General` | Balanced all-round | grok-3, gemini-2.0-flash |
| `Reasoning` | Deep thinking, planning | grok-3, claude-opus-4-6, o3 |
| `Speed` | Low latency, simple tasks | grok-3-mini, gemini-flash, gpt-4o-mini |
| `Coding` | Code generation and review | grok-3, claude-sonnet-4-6 |

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

> Model availability depends on your xAI plan. Run `/mcp status` or check [console.x.ai](https://console.x.ai) for current availability.

### Special Features

- **Native function calling** — xAI's structured `tool_calls` API is fully implemented. When Grok is primary, tool requests come as structured JSON rather than parsed text, improving reliability and reducing hallucinated tool calls.
- **Streaming** — Real-time token streaming via SSE.
- **Usage reporting** — `GrokProvider.GetLastUsage()` returns `TokenUsage{PromptTokens, CompletionTokens}` for billing.
- **Extended thinking** — `grok-3-mini` supports `reasoning_effort` parameter for extended internal reasoning.
- **x_pull tool** — Fetch content from X (Twitter) posts using the xAI API.
- **Vision** — `grok-2-vision-1212` processes images sent as base64 data URIs; used by the `vision_screen`, `vision_file`, `vision_ocr`, and related tools.

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
| `gemini-2.0-flash` | Fast, efficient (recommended default specialist) | 1M tokens |
| `gemini-2.0-flash-thinking-exp` | Extended reasoning mode | 1M tokens |
| `gemini-1.5-pro` | Previous generation pro | 2M tokens |
| `gemini-1.5-flash` | Previous generation fast | 1M tokens |

> Gemini model availability varies by region and plan. Check [aistudio.google.com](https://aistudio.google.com) for the current list.

### Special Features

- **Verbose thoughts** — Enable with `--verbose-thoughts` to display Gemini's chain-of-thought reasoning in distinctive consultant boxes in the TUI.
- **Large context** — Gemini supports up to 2M tokens of context, making it excellent for whole-codebase analysis.
- **Role mapping** — `"assistant"` maps to `"model"` in Gemini's API; system messages are included as user messages with special formatting.
- **Streaming** — SSE-based real-time token streaming.

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
| `claude-opus-4-6` | Most capable (Opus family) | 200k tokens |
| `claude-sonnet-4-6` | Balanced performance (Sonnet family) | 200k tokens |
| `claude-haiku-4-5-20251001` | Fast and efficient (Haiku family) | 200k tokens |

> The discovery system fetches the current model list from `GET /v1/models`. Actual available models depend on your Anthropic plan.

### Special Features

- **Extended thinking** — Claude Sonnet 3.7+ and Claude Opus 4+ support extended thinking blocks. Enable with `/think <budget>` (token budget) or set `ThinkingBudget` in the orchestrator. Thinking tokens are rendered in a dedicated TUI panel when a `ThinkingCallback` is wired.
- **SSE streaming** — Real-time token streaming.
- **Anthropic headers** — Requests include `x-api-key` and `anthropic-version: 2023-06-01` headers.
- **Tool tagging** — Models supporting extended thinking are tagged `SupportsThinking=true` in the model selection UI.

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

> `o1`/`o3`/`o4` series are tagged `SupportsThinking=true`. Non-chat models (embeddings, TTS, whisper, dall-e, babbage) are filtered out during discovery.

### Special Features

- **OpenAI-compatible** — Uses the standard chat completions API. Compatible with any OpenAI-compatible endpoint (e.g., local LLM servers, other providers) by changing the base URL.
- **o-series detection** — `o1`, `o3`, `o4` prefix detection tags models as reasoning-capable.

### Getting an OpenAI API Key

1. Go to [platform.openai.com](https://platform.openai.com)
2. Navigate to **API Keys**
3. Create a new key (starts with `sk-`)

---

## 7. MiniMax

**Package:** `pkg/ai/minimax.go`
**API Base:** `https://api.minimax.io/anthropic/v1` (Anthropic-compatible endpoint)
**Key env var:** `MINIMAX_API_KEY`

MiniMax exposes an Anthropic-compatible API. `MiniMaxProvider` wraps `AnthropicProvider` with a custom base URL and uses an OpenAI-compatible model listing endpoint with a static fallback list.

### Getting a MiniMax API Key

Register at [minimax.io](https://minimax.io) and generate an API key from the developer console.

---

## 8. Moonshot

**Package:** `pkg/ai/moonshot.go`
**API Base:** `https://api.moonshot.cn/v1` (OpenAI-compatible)
**Key env var:** `MOONSHOT_API_KEY`

### Available Models

| Model ID | Context |
|----------|---------|
| `moonshot-v1-8k` | 8k tokens |
| `moonshot-v1-32k` | 32k tokens |
| `moonshot-v1-128k` | 128k tokens |

Moonshot uses an OpenAI-compatible API. `MoonshotProvider` is a standalone implementation that targets the Moonshot API base URL.

### Getting a Moonshot API Key

Register at [platform.moonshot.cn](https://platform.moonshot.cn) and generate an API key from the console.

---

## 9. OpenRouter

**Package:** `pkg/ai/openrouter.go`
**API Base:** `https://openrouter.ai/api/v1`
**Key env var:** `OPENROUTER_API_KEY`

OpenRouter is a gateway that provides access to 400+ models from multiple providers (Anthropic, OpenAI, Google, Meta, Mistral, etc.) via a single API key, using provider-prefixed model IDs like `anthropic/claude-opus-4-6`.

### Special Features

- **Single key for 400+ models** — pay-as-you-go across all major providers without needing separate accounts.
- **Model ID format** — `<provider>/<model>` (e.g., `openai/gpt-4o`, `meta-llama/llama-3.1-70b-instruct`).
- **Last-resort failover** — OpenRouter serves as the final fallback in the provider cascade.
- **Referer header** — Requests include `HTTP-Referer: https://gorkbot.ai` and `X-Title: Gorkbot` headers per OpenRouter's requirements.

### Getting an OpenRouter API Key

1. Go to [openrouter.ai](https://openrouter.ai)
2. Create an account
3. Navigate to **Keys** and create a new key

---

## 10. Hot-Swapping Models at Runtime

Switch providers and models without restarting Gorkbot.

### Via TUI Model Selection (`Ctrl+T`)

1. Press `Ctrl+T` to open the dual-pane model selection
2. Use `Tab` to switch between Primary and Specialist panes
3. Navigate with `Up`/`Down` arrow keys
4. Press `Enter` to select a model
5. Press `r` to refresh model lists from all providers
6. Press `p` to cycle provider filter
7. Press `k` to add or update an API key

### Via `/model` Command

```
/model                              # show current primary and specialist
/model primary grok-3-mini          # switch primary to grok-3-mini
/model consultant claude-sonnet-4-6 # switch specialist to Claude Sonnet
/model consultant auto              # enable auto-select specialist per task
```

The `MODEL_SWITCH_PRIMARY:<provider>:<modelID>` and `MODEL_SWITCH_SECONDARY:<provider>:<modelID>` signals are processed by the TUI and call `Orchestrator.SetPrimary`/`SetSecondary` respectively.

### Via `/key` Command

```
/key xai xai-your-new-key
/key google AIza-your-new-key
/key anthropic sk-ant-your-key
/key openai sk-your-key
/key moonshot your-key
/key openrouter sk-or-your-key
/key status                         # show status for all providers
/key validate xai                   # validate a specific key with a live ping
```

### Via Environment Variable Override

Set model overrides before launch to bypass all selection logic:

```bash
GORKBOT_PRIMARY_MODEL=claude-sonnet-4-6 \
GORKBOT_CONSULTANT_MODEL=grok-3 \
./gorkbot.sh
```

---

## 11. Dynamic Model Discovery

The `pkg/discovery.Manager` polls all configured providers for live model lists on startup and every 30 minutes.

### How It Works

```
discovery.Manager.Start(ctx)
  → background goroutine: poll every 30 minutes
  → fetchXAIModels()       → GET https://api.x.ai/v1/models
  → fetchGeminiModels()    → GET https://generativelanguage.googleapis.com/v1beta/models
  → fetchAnthropicModels() → GET https://api.anthropic.com/v1/models
  → fetchOpenAIModels()    → GET https://api.openai.com/v1/models
  → fetchMiniMaxModels()   → static list + API query
  → classify each model → CapabilityClass
  → send DiscoveryUpdateMsg to TUI (updates Cloud Brains tab)
```

Discovery uses `NewManagerWithKeys(keyGetter, logger)` where `KeyGetter` is an interface satisfied by `pkg/providers.KeyStore`. This avoids an import cycle between `pkg/discovery` and `pkg/providers`.

### Model Classification

Models are classified by matching IDs against keyword patterns:

| Keyword patterns | CapabilityClass |
|-----------------|----------------|
| `reasoning`, `thinking`, `o1`, `o3`, `o4`, `opus` | `Reasoning` |
| `flash`, `mini`, `haiku`, `fast`, `turbo`, `lite` | `Speed` |
| `code`, `coder`, `dev` | `Coding` |
| (default) | `General` |

---

## 12. Cloud Brains Tab

The Cloud Brains tab (`Ctrl+D`) provides a live view of the discovery system:

- **Left panel** — discovered models grouped by provider and capability class
- **Right panel** — hierarchical agent delegation tree (sub-agents and their tasks)

This tab updates automatically when `discovery.Manager` completes a poll cycle.

---

## 13. Adaptive Routing and Feedback

`pkg/router` provides a feedback-driven model routing system:

```
/rate <1-5>
  → FeedbackManager.RecordOutcome(category, provider, model, score)
  → Persisted to ~/.config/gorkbot/feedback.jsonl

FeedbackManager.SuggestModel(category)
  → Returns the provider/model with the highest average score for this category
  → Logged before each task (suggestion loop closure)
```

Intent categories matched by the ARC system (from `pkg/adaptive`): `auto`, `deep`, `quick`, `visual`, `research`, `security`, `code`, `creative`, `data`, `plan`.

The feedback system is separate from ARC routing: ARC routes based on prompt structure and compute budget; the feedback router learns from explicit user satisfaction ratings.

---

## 14. Dual-Model Orchestration

Gorkbot uses a two-model architecture:

1. **Primary** — handles all conversational turns and tool execution; streaming tokens appear in real time in the TUI
2. **Specialist (Consultant)** — consulted for complex queries; response appears in a distinct bordered box in the TUI

The consultant is triggered when:
- The user includes `COMPLEX` or `REFRESH` keywords in the message
- The orchestrator's complexity heuristic fires (message length + tool count threshold)
- The `/mode` is set to `Plan` or `Auto`

The specialist's advice is prepended to the user message context before the primary AI call, so the primary benefits from specialist reasoning without the consultant being in the main conversation thread.

### Changing the Specialist

```bash
# Use Anthropic Claude as consultant instead of Gemini
/model consultant claude-sonnet-4-6

# Use auto-selection (picks best model per task via ARC + discovery)
/model consultant auto
```

---

## 15. Provider Failover Cascade

When the primary provider returns an error (rate limit, outage, invalid key, context overflow), `internal/engine/fallback.go` automatically cycles through the cascade:

```
xAI → Google → Anthropic → MiniMax → OpenAI → OpenRouter
```

Behaviour:
- Each step tries the same request with the next available provider
- Providers disabled in `app_state.json` (via Settings → API Providers tab) are skipped
- If all providers fail, the error is returned to the user with a summary of all failures
- The cascade is transparent to the TUI — the user sees the response as if the primary succeeded

---

## 16. Extended Thinking

Extended thinking allows supported models to reason internally before producing their response. The internal reasoning is visible in the TUI in a dedicated collapsible panel.

### Supported Models

| Provider | Models | Parameter |
|----------|--------|-----------|
| Anthropic | claude-sonnet-3-7, claude-opus-4+ | `thinking` block in API |
| xAI | grok-3-mini | `reasoning_effort` parameter |
| OpenAI | o1, o3, o4-mini | Internal (no explicit parameter needed) |

### Enabling Extended Thinking

```
/think 8000      # enable with 8000-token thinking budget (Anthropic)
/think 0         # disable extended thinking
/think           # toggle on/off with last-used budget
```

Or set at startup:

```bash
# Via orchestrator (set in main.go from flag or env):
./gorkbot.sh --thinking-budget 8000  # (if flag is exposed)
```

The thinking budget controls how many tokens the model can spend on internal reasoning. Higher budgets produce more thorough reasoning but increase cost and latency.

When thinking is active, the TUI renders thinking-block tokens in a separate stream (separated from main output by sentinel characters `\x02`/`\x03` in the stream). The `ThinkingCallback` on the orchestrator routes these tokens to the dedicated panel.
