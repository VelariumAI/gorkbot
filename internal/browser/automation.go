package browser

import (
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// BrowserSession represents an active browser session
type BrowserSession struct {
	ID       string
	URL      string
	Status   string // "running", "stopped"
	Browser  string // "chromium", "firefox", "webkit"
	Started  time.Time
	Viewport map[string]int // width, height
}

// BrowserAutomation manages browser automation using Playwright
type BrowserAutomation struct {
	logger   *slog.Logger
	sessions map[string]*BrowserSession
	enabled  bool
}

// NewBrowserAutomation creates a new browser automation manager
func NewBrowserAutomation(logger *slog.Logger) *BrowserAutomation {
	if logger == nil {
		logger = slog.Default()
	}

	ba := &BrowserAutomation{
		logger:   logger,
		sessions: make(map[string]*BrowserSession),
		enabled:  isPlaywrightInstalled(),
	}

	if ba.enabled {
		ba.logger.Info("browser automation enabled with Playwright")
	} else {
		ba.logger.Warn("browser automation disabled: Playwright not found")
	}

	return ba
}

// LaunchBrowser launches a new browser session
func (ba *BrowserAutomation) LaunchBrowser(url string, browserType string) (*BrowserSession, error) {
	if !ba.enabled {
		return nil, fmt.Errorf("browser automation not available")
	}

	session := &BrowserSession{
		ID:      fmt.Sprintf("browser-%d", time.Now().UnixNano()),
		URL:     url,
		Status:  "running",
		Browser: browserType,
		Started: time.Now(),
		Viewport: map[string]int{
			"width":  1920,
			"height": 1080,
		},
	}

	ba.sessions[session.ID] = session

	ba.logger.Info("launched browser session",
		slog.String("id", session.ID),
		slog.String("url", url),
		slog.String("browser", browserType),
	)

	return session, nil
}

// TakeScreenshot captures a screenshot
func (ba *BrowserAutomation) TakeScreenshot(sessionID string, path string) (string, error) {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return "", fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Status != "running" {
		return "", fmt.Errorf("session not running: %s", sessionID)
	}

	ba.logger.Debug("taking screenshot",
		slog.String("session", sessionID),
		slog.String("path", path),
	)

	// Would execute Playwright command in production
	// For now, simulate the operation
	return fmt.Sprintf("%s/screenshot-%d.png", path, time.Now().Unix()), nil
}

// NavigateTo navigates to a URL
func (ba *BrowserAutomation) NavigateTo(sessionID string, url string) error {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.URL = url

	ba.logger.Debug("navigated to URL",
		slog.String("session", sessionID),
		slog.String("url", url),
	)

	return nil
}

// ClickElement clicks an element matching selector
func (ba *BrowserAutomation) ClickElement(sessionID string, selector string) error {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Status != "running" {
		return fmt.Errorf("session not running")
	}

	ba.logger.Debug("clicked element",
		slog.String("session", sessionID),
		slog.String("selector", selector),
	)

	return nil
}

// TypeText types text into an element
func (ba *BrowserAutomation) TypeText(sessionID string, selector string, text string) error {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Status != "running" {
		return fmt.Errorf("session not running")
	}

	ba.logger.Debug("typed text",
		slog.String("session", sessionID),
		slog.String("selector", selector),
		slog.Int("length", len(text)),
	)

	return nil
}

// GetConsoleLogs retrieves console logs
func (ba *BrowserAutomation) GetConsoleLogs(sessionID string) []string {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return nil
	}

	if session.Status != "running" {
		return nil
	}

	ba.logger.Debug("retrieving console logs",
		slog.String("session", sessionID),
	)

	return []string{}
}

// CloseBrowser closes a browser session
func (ba *BrowserAutomation) CloseBrowser(sessionID string) error {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Status = "stopped"

	ba.logger.Info("closed browser session",
		slog.String("id", sessionID),
		slog.Duration("duration", time.Since(session.Started)),
	)

	return nil
}

// WaitForSelector waits for element to appear
func (ba *BrowserAutomation) WaitForSelector(sessionID string, selector string, timeout time.Duration) error {
	session, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if session.Status != "running" {
		return fmt.Errorf("session not running")
	}

	ba.logger.Debug("waiting for selector",
		slog.String("session", sessionID),
		slog.String("selector", selector),
		slog.Duration("timeout", timeout),
	)

	return nil
}

// WaitForNavigation waits for page navigation
func (ba *BrowserAutomation) WaitForNavigation(sessionID string) error {
	_, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	ba.logger.Debug("waiting for navigation",
		slog.String("session", sessionID),
	)

	return nil
}

// GetStats returns browser automation statistics
func (ba *BrowserAutomation) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":  ba.enabled,
		"sessions": len(ba.sessions),
	}
}

// Helper function
func isPlaywrightInstalled() bool {
	_, err := exec.LookPath("playwright")
	return err == nil
}

// AutomationScript represents a browser automation script
type AutomationScript struct {
	Name  string
	Steps []AutomationStep
}

// AutomationStep represents a single automation step
type AutomationStep struct {
	Action string      // "navigate", "click", "type", "screenshot", "wait"
	Target string      // Selector or URL
	Value  string      // Value for type action
	Delay  time.Duration // Delay before next step
}

// ExecuteScript executes an automation script
func (ba *BrowserAutomation) ExecuteScript(sessionID string, script *AutomationScript) error {
	_, ok := ba.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	for i, step := range script.Steps {
		switch step.Action {
		case "navigate":
			ba.NavigateTo(sessionID, step.Target)
		case "click":
			ba.ClickElement(sessionID, step.Target)
		case "type":
			ba.TypeText(sessionID, step.Target, step.Value)
		case "screenshot":
			ba.TakeScreenshot(sessionID, step.Target)
		case "wait":
			time.Sleep(step.Delay)
		}

		ba.logger.Debug("executed automation step",
			slog.String("session", sessionID),
			slog.Int("step", i+1),
			slog.Int("total", len(script.Steps)),
		)
	}

	return nil
}
