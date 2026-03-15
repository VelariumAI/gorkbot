package inline

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
	"github.com/velariumai/gorkbot/internal/engine"
	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/commands"
	"github.com/velariumai/gorkbot/pkg/hooks"
	"github.com/velariumai/gorkbot/pkg/tools"
)

// RunREPL starts the inline REPL mode.
func RunREPL(orch *engine.Orchestrator, cmdReg *commands.Registry, env *platform.EnvConfig) {
	fmt.Printf("Gorkbot v%s — Inline REPL mode\n", platform.Version)
	fmt.Println("Type /help for commands, or just type a prompt.")
	fmt.Println("Press Ctrl+C to interrupt or Ctrl+D to exit.")
	fmt.Println()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("gorkbot > "),
		HistoryFile:     env.ConfigDir + "/repl_history",
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing readline: %v\n", err)
		return
	}
	defer rl.Close()

	printer := NewPrinter()

	// Subscribe to hooks for consultation and background agents
	orch.Hooks.Subscribe(func(event hooks.Event, payload hooks.Payload) {
		switch event {
		case hooks.EventConsultantInvoked:
			printer.StartTool("consultation: stage 1 (Intent Classification)")
		case hooks.EventIntentDetected:
			printer.StopTool("consultation: stage 1 (Intent Classification)", true, 0)
			printer.StartTool("consultation: stage 2 (Semantic Search)")
		case hooks.EventPreCompaction:
			printer.StopTool("consultation: stage 2 (Semantic Search)", true, 0)
			printer.StartTool("consultation: stage 3 (Context Engrams)")
		case hooks.EventConsultantResponse:
			printer.StopTool("consultation: stage 4 (Specialist Query)", true, 0)
			printer.StartTool("consultation: stage 5 (Airlock Validation)")
		case hooks.EventBackgroundAgentStarted:
			agentID, _ := payload.Extra["agent_id"].(string)
			label, _ := payload.Extra["label"].(string)
			printer.StartTool(fmt.Sprintf("background agent: %s (%s)", label, agentID))
		case hooks.EventBackgroundAgentDone:
			agentID, _ := payload.Extra["agent_id"].(string)
			status, _ := payload.Extra["status"].(string)
			printer.StopTool(fmt.Sprintf("background agent: %s (%s)", "...", agentID), status == "done", 0)
		}
	})

	for {
		line, err := rl.Readline()
		if err != nil { // io.EOF, readline.ErrInterrupt
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			handleSlashCommand(input, cmdReg, printer)
			continue
		}

		// Execute task with streaming
		runTask(orch, input, printer)
	}
}

func handleSlashCommand(input string, cmdReg *commands.Registry, printer *Printer) {
	parts := strings.Fields(input)
	cmdName := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	// Command degradation for TUI-heavy commands
	switch cmdName {
	case "settings":
		fmt.Println("Settings overlay is not available in inline mode.")
		fmt.Println("Use /mcp config or edit your config files directly.")
		return
	case "dag":
		fmt.Println("DAG visualization is not available in inline mode.")
		return
	case "diagnostics":
		fmt.Println("Full diagnostics dashboard is not available in inline mode.")
		// Fall through to registry execution which might return a text report
	}

	result, err := cmdReg.Execute(cmdName, args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if result == "QUIT" {
		fmt.Println("Goodbye!")
		os.Exit(0)
	}

	if result != "" {
		// Use glamour to render markdown results from commands if they look like markdown
		if strings.Contains(result, "#") || strings.Contains(result, "|") || strings.Contains(result, "```") {
			rendered, _ := glamour.Render(result, "dark")
			fmt.Print(rendered)
		} else {
			fmt.Println(result)
		}
	}
}

func runTask(orch *engine.Orchestrator, prompt string, printer *Printer) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT to cancel the task
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			fmt.Println("\n[Interrupting...]")
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	streamCb := func(token string) {
		printer.WriteStream(StripAIArtifacts(token))
	}

	toolStartCb := func(name string, params map[string]interface{}) {
		printer.StartTool(name)
	}

	toolDoneCb := func(name string, result *tools.ToolResult) {
		elapsed := time.Duration(0)
		printer.StopTool(name, result.Success, elapsed)
	}

	interventionCb := func(severity engine.WatchdogSeverity, context string) engine.InterventionResponse {
		fmt.Printf("\n[WARNING] %s\n", context)
		fmt.Print("Allow execution? (y/N/session): ")

		// Use a dedicated reader to avoid conflicts with readline
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.ToLower(strings.TrimSpace(resp))

		switch resp {
		case "y", "yes":
			return engine.InterventionContinue
		case "session":
			return engine.InterventionAllowSession
		default:
			return engine.InterventionStop
		}
	}

	adviceCb := func(advice string) {
		printer.StopTool("consultation: stage 5 (Airlock Validation)", true, 0)
		fmt.Println()
		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")). // Blood Red
			Padding(0, 1)

		header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")).Render("EXPERT CONSULTANT ADVICE")
		fmt.Println(borderStyle.Render(header + "\n\n" + advice))
		fmt.Println()
	}

	fmt.Println() // Space before AI output
	err := orch.ExecuteTaskWithStreaming(ctx, prompt, streamCb, toolDoneCb, toolStartCb, interventionCb, adviceCb)
	if err != nil && err != context.Canceled {
		fmt.Printf("\nError: %v\n", err)
	}
	fmt.Println()
}

var tagRegex = regexp.MustCompile(`(?s)<thinking>.*?</thinking>|<thought>.*?</thought>`)

// StripAIArtifacts removes internal AI tags and stutter.
func StripAIArtifacts(token string) string {
	// Note: Simple regex won't work well for streaming tokens.
	// For a 100% complete implementation, we'd need a stateful buffer.
	// But for tokens like "\x02" (thinking start) and "\x03" (thinking end)
	// which Gorkbot uses internally, we can strip them easily.

	token = strings.ReplaceAll(token, "\x02", "")
	token = strings.ReplaceAll(token, "\x03", "")

	return token
}
