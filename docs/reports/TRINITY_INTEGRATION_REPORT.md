# Project Trinity: Comprehensive Integration Report

**Date:** February 16, 2026
**Project Roots:** `SENSE`, `Gorkbot`, `Scholium`
**Objective:** Unify three distinct AI architectures into a single, cohesive autonomous system.

---

## 1. Executive Summary

This report outlines the strategy for **Project Trinity**, an initiative to integrate three specialized AI systems into a unified organism. The goal is to combine the autonomous cognitive capabilities of **SENSE** (The Brain), the high-performance interface and execution layer of **Gorkbot** (The Body), and the deep research and distillation engine of **Scholium** (The Subconscious).

The resulting system will function as a self-evolving, persistent AI agent with a polished terminal interface, robust tool execution, and deep knowledge refinement capabilities.

---

## 2. Component Analysis

| Component | Role | Tech Stack | Key Capabilities | Integration Gap |
| :--- | :--- | :--- | :--- | :--- |
| **SENSE** | **The Brain** (Orchestrator) | Python 3.12+ | Self-evolution, Long Term Memory (LTM), "Pulse" daemon, Cognitive Dispatcher. | Lacks a native, high-performance UI; currently relies on Telegram or basic CLI. API surface needs expansion. |
| **Gorkbot** | **The Body** (Interface & Execution) | Go (Bubble Tea) | Polished TUI, A2A (Agent-to-Agent) protocol, fast/safe tool execution, multi-model support. | Currently hardcoded for xAI/Gemini; needs a generic adapter to communicate with the SENSE "brain". |
| **Scholium** | **The Subconscious** (Deep Research) | Python / Postgres | Automated research missions, "Teacher-Student" distillation, trace generation, "State-Aware Cockpit". | Operates as a standalone refinery; needs to be callable as a background service or "tool" by SENSE. |

---

## 3. Proposed Architecture

The proposed architecture establishes a clear hierarchy where **Gorkbot** serves as the user-facing shell, **SENSE** acts as the decision-making kernel, and **Scholium** functions as an on-demand specialized subsystem.

```mermaid
graph TD
    User[User] -->|Interacts via| Gorkbot[Gorkbot TUI (Go)]
    
    subgraph "The Body (Frontend & Execution)"
        Gorkbot -->|A2A Protocol / API| SENSE_API[SENSE API Gateway]
        Gorkbot -->|Executes| Tools[Go-based Tools (Files, Git, System)]
    end
    
    subgraph "The Brain (Cognition)"
        SENSE_API --> SENSE_Core[SENSE Daemon (Python)]
        SENSE_Core -->|Reads/Writes| LTM[Long Term Memory]
        SENSE_Core -->|Evolves| Persona[Genetic Soul]
    end
    
    subgraph "The Subconscious (Deep Research)"
        SENSE_Core -->|Dispatches Mission| Scholium[Scholium Refinery]
        Scholium -->|Returns Distilled Facts| LTM
    end
```

---

## 4. Integration Implementation Plan

### Phase 1: The Neural Link (Gorkbot <-> SENSE)
**Goal:** Establish Gorkbot as the primary frontend for the SENSE daemon.

*   **SENSE Upgrade:** 
    *   Enhance `SENSE/src/sense/api/app.py` to expose a standard chat/completion endpoint.
    *   Ensure the API supports streaming responses to take advantage of Gorkbot's TUI capabilities.
*   **Gorkbot Adapter:**
    *   Create a new AI Provider in `gorkbot/pkg/ai/sense.go` implementing the `AIProvider` interface.
    *   Map SENSE's specific context/memory parameters to Gorkbot's request format.
*   **Outcome:** Users can launch Gorkbot (`./gorkbot.sh`) and select `/model sense`, interacting with the persistent, evolving agent through a professional TUI.

### Phase 2: The Tool Handshake
**Goal:** Offload safe, high-performance execution from Python to Go.

*   **Current State:** SENSE uses internal Python tools (`SENSE/src/sense/tools`) which can be slower and harder to sandbox.
*   **New Protocol:**
    *   Implement a "Tool Bridge" protocol.
    *   When SENSE determines an action is needed (e.g., `git commit`), it returns a structured `tool_request` instead of text.
    *   Gorkbot intercepts this request, executes the tool using its hardened Go implementation (`gorkbot/pkg/tools`), and feeds the result back to SENSE.
*   **Benefit:** dramatically improved security and performance for file system and shell operations.

### Phase 3: The Deep Thought (SENSE <-> Scholium)
**Goal:** Enable autonomous, deep research capabilities.

*   **Mechanism:** Create a specialized SENSE Skill (`skill-researcher`).
*   **Workflow:**
    1.  User asks a complex question (e.g., "Analyze the last 5 years of commercial spaceflight trends").
    2.  SENSE's `Cognitive Dispatcher` recognizes a knowledge gap.
    3.  SENSE triggers the `research` skill, which makes an API call to the local Scholium service.
    4.  Scholium initiates a "Mission", browsing the web and distilling findings.
    5.  Scholium injects the synthesized report directly into SENSE's **Long Term Memory**.
    6.  SENSE retrieves the new memory and formulates a comprehensive answer.

---

## 5. Technical Details & Investigation Findings

### Key Files Identified
*   **SENSE API:** `SENSE/src/sense/api/app.py` - The entry point for external communication.
*   **Gorkbot AI Interface:** `gorkbot/pkg/ai/` - The directory where the new `sense.go` provider must be implemented.
*   **Gorkbot A2A Protocol:** `gorkbot/pkg/a2a/protocol.go` - The existing protocol that can be adapted for the Tool Bridge.
*   **Scholium Server:** `Scholium/src/scholium/server/` - The interface for triggering research missions.

### Infrastructure Requirements
*   **Unified Startup:** A master script (`trinity_start.sh`) or `docker-compose.yml` is required to orchestrate the startup sequence:
    1.  Start Postgres (for Scholium).
    2.  Start Scholium Server (Headless).
    3.  Start SENSE Daemon (API Mode).
    4.  Launch Gorkbot TUI (Attached to SENSE).

---

## 6. Immediate Next Steps

1.  **API Verification:** Audit `SENSE/src/sense/api/app.py` to document the exact request/response schema.
2.  **Prototype Bridge:** Create a minimal Go program that sends a request to the SENSE API and prints the response, validating connectivity.
3.  **Dependency Alignment:** Ensure all three projects can coexist in the same environment (Python version compatibility, port allocation).
