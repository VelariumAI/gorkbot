# Gorkbot

Gorkbot is a governed AI operator/workstation framework in Go. It combines multi-provider LLM orchestration with controlled tool execution, VCSE correctness boundaries, a governed research egress gateway, and an optional Puter workspace adapter.

## Current Status

- Active development
- Go project
- Governed agent/operator framework
- Optional Puter adapter exists (live transport not included yet)
- VCSE integration baseline is implemented
- Apache-2.0 licensed

## What Gorkbot Is

Gorkbot is designed for assisted operation under bounded controls. It can research, operate, draft, and propose actions. Governance and approval logic classify and gate risky actions. VCSE and renderer-guard paths define correctness boundaries for final answers. Research egress is policy-controlled, and the Puter adapter is optional.

## Core Capabilities

### LLM orchestration

- Multi-provider orchestration through one operator runtime
- Interactive terminal flow and one-shot prompt execution

### Governed tool execution

- Tool routing with governance mode controls
- Policy and risk classification hooks for runtime decisions
- Approval runtime support for higher-risk actions

### VCSE correctness boundary

- VCSE client baseline includes proposal, ledger, and render verification endpoints
- Governance can fail closed in stricter modes when VCSE is unavailable

### Research Egress Gateway

- Governed outbound research path for external fetch/search requests
- Public `GET`/`HEAD` are bounded and policy-controlled
- Private/local/cloud metadata targets are blocked by default
- Credential material is blocked by default
- Redirects are revalidated before follow
- Mutating methods are blocked unless policy/approval allows them
- Fetched content is not automatically verified (`fetched != verified`)

### Puter Workspace Adapter

- Governed Puter workspace adapter is implemented
- Canonical API contract source is pinned to `VelariumAI/puter` at reviewed ref
- Deployment posture supports `local`, `self_hosted`, and `saas`
- Live Puter transport is not included yet
- Puter is optional and disabled by default
- Puter does not bypass governance or the Research Egress Gateway

### TUI and one-shot mode

- Interactive terminal UI for operator workflows
- One-shot mode for prompt execution in scripts/automation

## Architecture At A Glance

```text
User
  ↓
Gorkbot Orchestrator
  ├─ LLM Providers
  ├─ Tool Registry + Governance
  ├─ Research Egress Gateway
  ├─ VCSE Client / Renderer Guard
  └─ Puter Workspace Adapter
```

Operational model:
- Gorkbot researches, operates, drafts, and proposes.
- VCSE validates correctness boundaries.
- Renderer guard prevents overclaiming in correctness paths.
- Research Gateway controls web egress.
- Puter provides an optional workspace substrate.
- User approval remains required for risky actions.

## Quick Start

```bash
git clone https://github.com/VelariumAI/gorkbot.git
cd gorkbot
make setup
make build
./bin/gorkbot
./bin/gorkbot -p "Summarize this repository's architecture."
```

## Important Flags

- `--governance off|audit|fast|enforce|correctness`
- `--vcse-url`
- `--vcse-timeout`
- `--render-guard-on-unavailable`
- `--render-guard-timeout`
- `--research-egress off|audit|enforce`
- `--research-max-response-bytes`
- `--research-timeout`
- `--puter-workspace off|audit|enforce`
- `--puter-deployment local|self_hosted|saas`
- `--puter-endpoint`
- `--puter-root`
- `--puter-repo`
- `--puter-ref`

## Safety Model

- Default governance mode is `off`
- High-risk actions are policy-gated in governed modes
- Approval runtime is bounded by timeout and in-flight controls
- Research egress is controlled by explicit policy
- Renderer guard gates correctness-mode final answers
- Missing claim maps prevent verified rendering
- Unsupported segments prevent verified rendering
- Gorkbot does not self-certify truth
- Puter adapter is disabled by default

## Development

- Go version: `1.25.0` (from `go.mod`)
- Run tests:

```bash
go test ./...
go vet ./...
```

Promotion manifest note:
If you add or remove tracked files under managed paths, run:

```bash
bash scripts/generate_promotion_manifest.sh
bash scripts/check_promotion_manifest.sh
```

## Roadmap

- PR-006 dynamic tool/self-modification hardening
- Live Puter transport
- VCSE source/datum ingestion
- Puter UI/workstation integration
- Stronger claim/source lifecycle

## License

Gorkbot is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).

This license applies to this Gorkbot repository. External projects and integrations, including Puter, remain governed by their own licenses.
