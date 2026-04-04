package prompts

import (
	"fmt"
	"strings"
)

// ===== VARIANT 1: GENERIC =====
type GenericVariant struct{}

func NewGenericVariant() Variant {
	return &GenericVariant{}
}

func (v *GenericVariant) Name() string {
	return "generic"
}

func (v *GenericVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	// System prompt
	if ctx.SystemPrompt != "" {
		sb.WriteString("SYSTEM:\n")
		sb.WriteString(ctx.SystemPrompt)
		sb.WriteString("\n\n")
	}

	// Main instruction
	sb.WriteString("TASK:\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	// Available tools
	if len(ctx.Tools) > 0 {
		sb.WriteString("AVAILABLE TOOLS:\n")
		for _, tool := range ctx.Tools {
			sb.WriteString(fmt.Sprintf("- %s\n", tool))
		}
		sb.WriteString("\n")
	}

	// Conversation history
	if len(ctx.ConversationHistory) > 0 {
		sb.WriteString("CONVERSATION HISTORY:\n")
		for _, msg := range ctx.ConversationHistory {
			sb.WriteString(msg)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Please complete the task above using the available tools and information.")

	return sb.String()
}

func (v *GenericVariant) MaxTokens() int {
	return 4000
}

func (v *GenericVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M
}

func (v *GenericVariant) SupportsThinking() bool {
	return false
}

func (v *GenericVariant) SupportsVision() bool {
	return false
}

func (v *GenericVariant) Priority() int {
	return 5 // Medium priority
}

// ===== VARIANT 2: NEXTGEN =====
type NextGenVariant struct{}

func NewNextGenVariant() Variant {
	return &NextGenVariant{}
}

func (v *NextGenVariant) Name() string {
	return "nextgen"
}

func (v *NextGenVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("## Task\n\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	if len(ctx.Tools) > 0 {
		sb.WriteString("## Tools Available\n\n")
		for _, tool := range ctx.Tools {
			sb.WriteString(fmt.Sprintf("- `%s`\n", tool))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Analyze the task thoroughly\n")
	sb.WriteString("2. Break it into steps if needed\n")
	sb.WriteString("3. Use tools appropriately\n")
	sb.WriteString("4. Provide clear reasoning\n")

	return sb.String()
}

func (v *NextGenVariant) MaxTokens() int {
	return 8000
}

func (v *NextGenVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 1.2 // 20% more for longer output
}

func (v *NextGenVariant) SupportsThinking() bool {
	return false
}

func (v *NextGenVariant) SupportsVision() bool {
	return false
}

func (v *NextGenVariant) Priority() int {
	return 7 // High priority
}

// ===== VARIANT 3: GPT5 =====
type GPT5Variant struct{}

func NewGPT5Variant() Variant {
	return &GPT5Variant{}
}

func (v *GPT5Variant) Name() string {
	return "gpt5"
}

func (v *GPT5Variant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("You are a highly capable AI assistant. Your task:\n\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	if len(ctx.Tools) > 0 {
		sb.WriteString("Use these tools as needed:\n")
		for _, tool := range ctx.Tools {
			sb.WriteString(fmt.Sprintf("- {%s}\n", tool))
		}
	}

	sb.WriteString("\nRespond with clear, structured reasoning.")

	return sb.String()
}

func (v *GPT5Variant) MaxTokens() int {
	return 16000
}

func (v *GPT5Variant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M
}

func (v *GPT5Variant) SupportsThinking() bool {
	return false
}

func (v *GPT5Variant) SupportsVision() bool {
	return true
}

func (v *GPT5Variant) Priority() int {
	return 8 // Very high priority
}

// ===== VARIANT 4: GEMINI3 =====
type Gemini3Variant struct{}

func NewGemini3Variant() Variant {
	return &Gemini3Variant{}
}

func (v *Gemini3Variant) Name() string {
	return "gemini3"
}

func (v *Gemini3Variant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("Task: ")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	if len(ctx.Tools) > 0 {
		sb.WriteString("Available capabilities: ")
		sb.WriteString(strings.Join(ctx.Tools, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Please provide a comprehensive response.")

	return sb.String()
}

func (v *Gemini3Variant) MaxTokens() int {
	return 32000
}

func (v *Gemini3Variant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 0.8 // 20% cheaper
}

func (v *Gemini3Variant) SupportsThinking() bool {
	return false
}

func (v *Gemini3Variant) SupportsVision() bool {
	return true
}

func (v *Gemini3Variant) Priority() int {
	return 9 // Highest priority (most capable)
}

// ===== VARIANT 5: CLAUDE_THINKING =====
type ClaudeThinkingVariant struct{}

func NewClaudeThinkingVariant() Variant {
	return &ClaudeThinkingVariant{}
}

func (v *ClaudeThinkingVariant) Name() string {
	return "claude_thinking"
}

func (v *ClaudeThinkingVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("This task requires careful reasoning. Please think through it thoroughly.\n\n")
	sb.WriteString("TASK:\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	if ctx.ThinkingBudget > 0 {
		sb.WriteString(fmt.Sprintf("You have %d tokens for extended thinking.\n\n", ctx.ThinkingBudget))
	}

	if len(ctx.Tools) > 0 {
		sb.WriteString("TOOLS:\n")
		for _, tool := range ctx.Tools {
			sb.WriteString(fmt.Sprintf("- %s\n", tool))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Provide detailed reasoning and clear conclusions.")

	return sb.String()
}

func (v *ClaudeThinkingVariant) MaxTokens() int {
	return 16000
}

func (v *ClaudeThinkingVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 2 // 2x cost for extended thinking
}

func (v *ClaudeThinkingVariant) SupportsThinking() bool {
	return true
}

func (v *ClaudeThinkingVariant) SupportsVision() bool {
	return true
}

func (v *ClaudeThinkingVariant) Priority() int {
	return 10 // Highest priority (most comprehensive)
}

// ===== VARIANT 6: XS (LIGHTWEIGHT) =====
type XSVariant struct{}

func NewXSVariant() Variant {
	return &XSVariant{}
}

func (v *XSVariant) Name() string {
	return "xs"
}

func (v *XSVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	if len(ctx.Tools) > 0 {
		sb.WriteString("Tools: ")
		sb.WriteString(strings.Join(ctx.Tools, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (v *XSVariant) MaxTokens() int {
	return 2000
}

func (v *XSVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 0.5 // 50% cheaper
}

func (v *XSVariant) SupportsThinking() bool {
	return false
}

func (v *XSVariant) SupportsVision() bool {
	return false
}

func (v *XSVariant) Priority() int {
	return 3 // Low priority (limited capability)
}

// ===== VARIANT 7: VISION =====
type VisionVariant struct{}

func NewVisionVariant() Variant {
	return &VisionVariant{}
}

func (v *VisionVariant) Name() string {
	return "vision"
}

func (v *VisionVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("VISUAL TASK:\n\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	sb.WriteString("ANALYSIS APPROACH:\n")
	sb.WriteString("1. Analyze any visual content provided\n")
	sb.WriteString("2. Describe observations in detail\n")
	sb.WriteString("3. Answer specific questions\n")
	sb.WriteString("4. Suggest improvements if applicable\n\n")

	if len(ctx.Tools) > 0 {
		sb.WriteString("TOOLS: ")
		sb.WriteString(strings.Join(ctx.Tools, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (v *VisionVariant) MaxTokens() int {
	return 8000
}

func (v *VisionVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 1.5 // 50% more for vision
}

func (v *VisionVariant) SupportsThinking() bool {
	return false
}

func (v *VisionVariant) SupportsVision() bool {
	return true
}

func (v *VisionVariant) Priority() int {
	return 9 // Very high priority (specialized)
}

// ===== VARIANT 8: SPECIALIST =====
type SpecialistVariant struct{}

func NewSpecialistVariant() Variant {
	return &SpecialistVariant{}
}

func (v *SpecialistVariant) Name() string {
	return "specialist"
}

func (v *SpecialistVariant) Build(ctx *PromptContext) string {
	sb := strings.Builder{}

	sb.WriteString("COMPLEX TASK - EXPERT REQUIRED:\n\n")
	sb.WriteString(ctx.Task)
	sb.WriteString("\n\n")

	sb.WriteString("EXPERT INSTRUCTIONS:\n")
	sb.WriteString("- Take your time for deep analysis\n")
	sb.WriteString("- Consider all angles and implications\n")
	sb.WriteString("- Provide comprehensive solutions\n")
	sb.WriteString("- Document your reasoning process\n")
	sb.WriteString("- Suggest alternatives where applicable\n\n")

	if ctx.ThinkingBudget > 0 {
		sb.WriteString(fmt.Sprintf("Extended thinking budget: %d tokens\n\n", ctx.ThinkingBudget))
	}

	if len(ctx.Tools) > 0 {
		sb.WriteString("SPECIALIST TOOLS:\n")
		for _, tool := range ctx.Tools {
			sb.WriteString(fmt.Sprintf("+ %s\n", tool))
		}
	}

	return sb.String()
}

func (v *SpecialistVariant) MaxTokens() int {
	return 32000
}

func (v *SpecialistVariant) EstimateCost(tokens int, costPer1M float64) float64 {
	return float64(tokens) / 1e6 * costPer1M * 3 // 3x cost for specialist
}

func (v *SpecialistVariant) SupportsThinking() bool {
	return true
}

func (v *SpecialistVariant) SupportsVision() bool {
	return true
}

func (v *SpecialistVariant) Priority() int {
	return 10 // Highest priority (most comprehensive)
}
