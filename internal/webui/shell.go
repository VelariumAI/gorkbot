// Package webui — Phase 2: App Shell Layout
package webui

// Shell defines the unified app shell layout per GORKBOT_UI_DIRECTIVE section 20.2
// Layout Grid:
// ┌─────────────────────────────────────────────────────┐
// │ Top Context Bar (56-64px height)                    │
// ├────┬───────────────────────────────┬────────────────┤
// │    │                               │                │
// │ L  │    Main Canvas                │ Inspector      │
// │ e  │    (Active Workspace)          │ (320-420px)    │
// │ f  │                               │                │
// │ t  ├───────────────────────────────┤                │
// │    │ Bottom Composer (72-128px)    │                │
// │ R  │                               │                │
// │ a  │                               │                │
// │ i  │                               │                │
// │ l  │                               │                │
// │    │                               │                │
// │    │                               │                │
// │(72-│                               │                │
// │280 │                               │                │
// │px) │                               │                │
// │    │                               │                │
// └────┴───────────────────────────────┴────────────────┘

import (
	"fmt"
	"html/template"
)

// Shell represents the complete app shell layout structure.
type Shell struct {
	ActiveWorkspace string
	UserAgent       string // User identifier for display
	ModelName       string // Current model (e.g., "Grok 3")
	ProviderName    string // Current provider (e.g., "xAI")
	RunStatus       string // "running", "idle", "error"
}

// NewShell creates a new app shell with default configuration.
func NewShell() *Shell {
	return &Shell{
		ActiveWorkspace: "chat",
		UserAgent:       "User",
		ModelName:       "Grok 3",
		ProviderName:    "xAI",
		RunStatus:       "idle",
	}
}

// RenderHTML returns the shell HTML as a string (for direct HTTP responses).
func (sh *Shell) RenderHTML() string {
	html, _ := sh.Render()
	return string(html)
}

// Render generates the complete HTML shell layout using CSS Grid.
func (sh *Shell) Render() (template.HTML, error) {
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Gorkbot — Adaptive AI Orchestrator</title>
	<link rel="stylesheet" href="/static/css/shell.css">
	<link rel="stylesheet" href="/api/theme/tokens.css">
</head>
<body class="shell-layout">
	<!-- Top Context Bar (56-64px) -->
	<header class="topbar">
		<div class="topbar-left">
			<span class="topbar-model">%s</span>
			<span class="topbar-provider">%s</span>
		</div>
		<div class="topbar-center">
			<span class="topbar-session">Session • %s</span>
		</div>
		<div class="topbar-right">
			<span class="topbar-status status-%s">%s</span>
			<button class="topbar-palette" title="Command Palette (Ctrl+K)">⌘K</button>
		</div>
	</header>

	<div class="shell-container">
		<!-- Left Rail Navigation (72-280px, collapsible) -->
		<nav class="sidebar" id="sidebar">
			<div class="sidebar-header">
				<div class="sidebar-avatar">G</div>
			</div>

			<div class="sidebar-workspaces" id="workspaces-list">
				<!-- Workspace items loaded from /api/workspaces -->
				<a class="workspace-item active" href="#" data-workspace="chat">
					<span class="workspace-icon">💬</span>
					<span class="workspace-label">Chat</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="tasks">
					<span class="workspace-icon">✓</span>
					<span class="workspace-label">Tasks</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="tools">
					<span class="workspace-icon">⚙</span>
					<span class="workspace-label">Tools</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="agents">
					<span class="workspace-icon">🤖</span>
					<span class="workspace-label">Agents</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="memory">
					<span class="workspace-icon">🧠</span>
					<span class="workspace-label">Memory</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="analytics">
					<span class="workspace-icon">📊</span>
					<span class="workspace-label">Analytics</span>
				</a>
				<a class="workspace-item" href="#" data-workspace="settings">
					<span class="workspace-icon">⚡</span>
					<span class="workspace-label">Settings</span>
				</a>
			</div>

			<div class="sidebar-footer">
				<button class="sidebar-toggle" id="sidebar-toggle" title="Toggle Sidebar">
					<span class="toggle-arrow">◀</span>
				</button>
			</div>
		</nav>

		<!-- Main Canvas & Composer -->
		<div class="main-area">
			<!-- Canvas (active workspace) -->
			<main class="canvas" id="canvas">
				<div class="workspace-container">
					<!-- Workspace content loaded dynamically -->
					<div class="workspace-empty">Loading %s...</div>
				</div>
			</main>

			<!-- Bottom Composer (72-128px, expandable) -->
			<footer class="composer" id="composer">
				<div class="composer-content">
					<textarea class="composer-input" id="composer-input"
						placeholder="Type your message... (Shift+Enter for new line)"
						rows="2"></textarea>
					<div class="composer-actions">
						<button class="composer-btn composer-send" id="composer-send" title="Send (Ctrl+Enter)">
							Send
						</button>
						<button class="composer-btn composer-stop" id="composer-stop" style="display:none;" title="Stop Execution">
							Stop
						</button>
					</div>
				</div>
			</footer>
		</div>

		<!-- Right Inspector Panel (320-420px, persistent on 1200px+) -->
		<aside class="inspector" id="inspector">
			<div class="inspector-header">
				<h3>Details</h3>
				<button class="inspector-close" id="inspector-close" title="Close Inspector">✕</button>
			</div>

			<div class="inspector-tabs">
				<button class="inspector-tab active" data-tab="details" title="Run Details">Details</button>
				<button class="inspector-tab" data-tab="tools" title="Tools Used">Tools</button>
				<button class="inspector-tab" data-tab="memory" title="Memory Context">Memory</button>
				<button class="inspector-tab" data-tab="sources" title="Sources">Sources</button>
				<button class="inspector-tab" data-tab="artifacts" title="Generated Artifacts">Artifacts</button>
				<button class="inspector-tab" data-tab="diagnostics" title="Execution Trace">Diagnostics</button>
			</div>

			<div class="inspector-content">
				<div class="inspector-panel active" id="tab-details">
					<p class="inspector-label">Status</p>
					<p class="inspector-value">Ready</p>
					<p class="inspector-label">Model</p>
					<p class="inspector-value">%s</p>
					<p class="inspector-label">Provider</p>
					<p class="inspector-value">%s</p>
				</div>
				<div class="inspector-panel" id="tab-tools">
					<p class="inspector-empty">No tools executed</p>
				</div>
				<div class="inspector-panel" id="tab-memory">
					<p class="inspector-empty">No memory injected</p>
				</div>
				<div class="inspector-panel" id="tab-sources">
					<p class="inspector-empty">No external sources</p>
				</div>
				<div class="inspector-panel" id="tab-artifacts">
					<p class="inspector-empty">No artifacts generated</p>
				</div>
				<div class="inspector-panel" id="tab-diagnostics">
					<p class="inspector-empty">No execution trace</p>
				</div>
			</div>
		</aside>
	</div>

	<!-- App shell JavaScript -->
	<script src="/static/js/shell.js"></script>
</body>
</html>
	`, sh.ModelName, sh.ProviderName, sh.UserAgent, sh.RunStatus, sh.RunStatus, sh.ActiveWorkspace, sh.ModelName, sh.ProviderName)

	return template.HTML(html), nil
}
