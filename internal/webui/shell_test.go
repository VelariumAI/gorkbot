package webui

import (
	"strings"
	"testing"
)

// TestShell_NewShell creates a new shell with default configuration.
func TestShell_NewShell(t *testing.T) {
	shell := NewShell()

	if shell == nil {
		t.Errorf("NewShell() returned nil")
	}
	if shell.ActiveWorkspace != "chat" {
		t.Errorf("expected ActiveWorkspace=chat, got %s", shell.ActiveWorkspace)
	}
	if shell.ModelName != "Grok 3" {
		t.Errorf("expected ModelName=Grok 3, got %s", shell.ModelName)
	}
	if shell.ProviderName != "xAI" {
		t.Errorf("expected ProviderName=xAI, got %s", shell.ProviderName)
	}
	if shell.RunStatus != "idle" {
		t.Errorf("expected RunStatus=idle, got %s", shell.RunStatus)
	}
}

// TestShell_RenderHTML renders the shell HTML structure.
func TestShell_RenderHTML(t *testing.T) {
	shell := NewShell()
	html, err := shell.Render()

	if err != nil {
		t.Errorf("Render() failed: %v", err)
	}

	htmlStr := string(html)
	if htmlStr == "" {
		t.Errorf("Render() returned empty HTML")
	}

	// Verify key shell components are present
	components := []string{
		"<!DOCTYPE html>",
		"<header class=\"topbar\">",
		"<nav class=\"sidebar\"",
		"<main class=\"canvas\"",
		"<footer class=\"composer\"",
		"<aside class=\"inspector\"",
		"/api/theme/tokens.css",
		"/static/js/shell.js",
	}

	for _, component := range components {
		if !strings.Contains(htmlStr, component) {
			t.Errorf("Render() missing expected component: %s", component)
		}
	}
}

// TestShell_ContainsWorkspaces verifies all 6 workspaces are in the layout.
func TestShell_ContainsWorkspaces(t *testing.T) {
	shell := NewShell()
	html, _ := shell.Render()
	htmlStr := string(html)

	workspaces := []string{"chat", "tasks", "tools", "agents", "memory", "analytics", "settings"}
	for _, ws := range workspaces {
		if !strings.Contains(htmlStr, `data-workspace="`+ws+`"`) {
			t.Errorf("Render() missing workspace: %s", ws)
		}
	}
}

// TestShell_ContainsInspectorTabs verifies inspector tabs are present.
func TestShell_ContainsInspectorTabs(t *testing.T) {
	shell := NewShell()
	html, _ := shell.Render()
	htmlStr := string(html)

	tabs := []string{"details", "tools", "memory", "sources", "artifacts", "diagnostics"}
	for _, tab := range tabs {
		if !strings.Contains(htmlStr, `data-tab="`+tab+`"`) {
			t.Errorf("Render() missing inspector tab: %s", tab)
		}
	}
}

// TestShell_TopbarDisplaysModel verifies model and provider in topbar.
func TestShell_TopbarDisplaysModel(t *testing.T) {
	shell := NewShell()
	shell.ModelName = "Custom Model"
	shell.ProviderName = "Custom Provider"

	html, _ := shell.Render()
	htmlStr := string(html)

	if !strings.Contains(htmlStr, "Custom Model") {
		t.Errorf("Topbar missing model name")
	}
	if !strings.Contains(htmlStr, "Custom Provider") {
		t.Errorf("Topbar missing provider name")
	}
}

// TestShell_StatusVariations verifies run status classes.
func TestShell_StatusVariations(t *testing.T) {
	statuses := []string{"running", "idle", "error"}
	for _, status := range statuses {
		shell := NewShell()
		shell.RunStatus = status

		html, _ := shell.Render()
		htmlStr := string(html)

		if !strings.Contains(htmlStr, `status-`+status) {
			t.Errorf("Render() missing status class for: %s", status)
		}
	}
}

// TestShell_GridLayout verifies CSS grid layout is declared.
func TestShell_GridLayout(t *testing.T) {
	shell := NewShell()
	html, _ := shell.Render()
	htmlStr := string(html)

	if !strings.Contains(htmlStr, "shell-layout") {
		t.Errorf("Render() missing shell-layout class")
	}
	if !strings.Contains(htmlStr, "shell-container") {
		t.Errorf("Render() missing shell-container class")
	}
}

// TestShell_KeyboardHints verifies user hints for keyboard shortcuts.
func TestShell_KeyboardHints(t *testing.T) {
	shell := NewShell()
	html, _ := shell.Render()
	htmlStr := string(html)

	hints := []string{
		"Ctrl+K",
		"Shift+Enter for new line",
		"Ctrl+Enter",
	}

	for _, hint := range hints {
		if !strings.Contains(htmlStr, hint) {
			t.Errorf("Render() missing keyboard hint: %s", hint)
		}
	}
}

// TestShell_ResponsiveDesign verifies responsive CSS is referenced.
func TestShell_ResponsiveDesign(t *testing.T) {
	shell := NewShell()
	html, _ := shell.Render()
	htmlStr := string(html)

	if !strings.Contains(htmlStr, "viewport") {
		t.Errorf("Render() missing viewport meta tag for responsive design")
	}
}
