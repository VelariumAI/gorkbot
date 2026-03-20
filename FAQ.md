# Frequently Asked Questions (FAQ)

Answers to common questions about Gorkbot.

---

## General Questions

### Q: What is Gorkbot?

Gorkbot is an AI orchestration platform that integrates multiple AI providers (Grok, Gemini, Claude, OpenAI) into a single intelligent interface with reasoning layers, tool execution, and cross-session memory.

### Q: How is Gorkbot different from ChatGPT?

| Feature | Gorkbot | ChatGPT |
|---------|---------|---------|
| **Multi-provider** | ✅ Yes | ❌ No (OpenAI only) |
| **Local execution** | ✅ Terminal + Web | ❌ Web only |
| **Tool execution** | ✅ 28+ tools | ❌ Limited |
| **Reasoning engines** | ✅ SPARK, SRE, etc. | ❌ Single model |
| **Memory** | ✅ Cross-session | ❌ Current session only |
| **Custom tools** | ✅ DIY tool creator | ❌ No |
| **Open source** | ✅ MIT License | ❌ Proprietary |

### Q: What providers are supported?

- ✅ **Grok (xAI)** - grok-2, grok-2-1212
- ✅ **Gemini (Google)** - gemini-2-flash, gemini-1.5-pro
- ✅ **Claude (Anthropic)** - claude-opus-4-6, claude-sonnet-4-6, claude-haiku-4-5
- ✅ **OpenAI (GPT)** - gpt-4-turbo, gpt-4o
- ✅ **OpenRouter** - 100+ models via unified API
- ✅ **Minimax** - Chinese LLM
- ✅ **Moonshot** - 200K context window
- ✅ **Any OpenAI-compatible API**

### Q: Is Gorkbot free?

Gorkbot itself is free (MIT license). However:
- API calls to providers are paid (varies by provider)
- Free tiers available: Gemini, Anthropic have free credits
- Pay-as-you-go: xAI, OpenAI charge per token

### Q: Can I run Gorkbot offline?

Partially:
- ✅ Local conversation history, tools, reasoning
- ❌ LLM calls require internet connection
- ⚠️ Native LLM support (llamacpp) coming soon

---

## Setup & Installation

### Q: How do I get API keys?

**Grok (xAI)**:
1. Go to https://console.x.ai/
2. Create account (email required)
3. Click "API Keys" → "Generate API Key"
4. Copy key (starts with `xai-`)

**Gemini (Google)**:
1. Go to https://aistudio.google.com/apikey
2. Click "Create API Key"
3. Free tier available!

**Claude (Anthropic)**:
1. Go to https://console.anthropic.com/
2. Create account
3. API Keys section → Create Key

**OpenAI**:
1. Go to https://platform.openai.com/api-keys
2. Create account/sign in
3. "$5 free credits" available

### Q: Do I need all API keys?

No! You need at least ONE:
- Grok: Recommended (default)
- Gemini: Free tier available
- Claude: Good alternative
- OpenAI: Optional

### Q: How do I set API keys?

**Option 1**: Interactive setup (recommended)
```bash
./gorkbot setup
```

**Option 2**: Environment variables
```bash
export XAI_API_KEY=xai-xxx
./gorkbot
```

**Option 3**: .env file
```bash
echo "XAI_API_KEY=xai-xxx" > .env
./gorkbot
```

### Q: Is my API key safe?

Yes:
- ✅ Keys never sent to Gorkbot servers
- ✅ Only sent to official provider APIs
- ✅ Stored in `.env` (gitignored by default)
- ✅ Can enable encryption: `./gorkbot setup`
- ✅ File permissions: 0600 (user only)

---

## Usage

### Q: How do I switch models?

In TUI:
```
/model
# Select from list
```

Or via command line:
```bash
./gorkbot -model grok-2-1212
```

Or in config (`~/.config/gorkbot/app_state.json`):
```json
{
  "primary_model": "grok-2"
}
```

### Q: How do I enable extended thinking?

```bash
./gorkbot -thinking-budget 5000
```

Or in TUI (if supported by model):
```
/settings → Thinking Budget: 5000
```

**Note**: Only Claude 3.5+ and Grok support extended thinking.

### Q: Can I use multiple providers?

Yes! Configure:
- **Primary**: Main model for responses
- **Secondary/Consultant**: For complex reasoning

Switch in TUI:
```
/model
```

Gorkbot automatically fails over if primary is unavailable.

### Q: What tools are available?

```
/tools
```

28+ tools including:
- Bash command execution
- File operations (read, write, list)
- Git operations
- Web fetching
- System info
- Security tools (nmap, nuclei)
- And more!

### Q: Can I create custom tools?

Yes! Use the `create_tool` meta-tool:
```
> Create a tool that pings a host

Gorkbot will:
1. Ask for tool name, description, parameters
2. Generate complete tool code
3. Save to ~/.config/gorkbot
4. Available immediately
```

---

## Performance & Optimization

### Q: Why is Gorkbot slow?

Possible causes:

1. **Network latency**: Provider API is slow
   - Try different provider: `/model`
   - Check internet: `ping google.com`

2. **Large history**: Conversation too long
   - Compress: `/compress`
   - Clear: `/clear`

3. **Tool execution**: Tool command is slow
   - Check logs: `~/.config/gorkbot/gorkbot.json`
   - Enable debug: `-debug-mcp`

4. **Context limits**: Token limit exceeded
   - Compress history: `/compress`
   - Start new session: `/clear`

### Q: How can I make Gorkbot faster?

1. **Use cache**: Enables by default
2. **Disable verbose**: `/verbose off`
3. **Compress regularly**: `/compress`
4. **Use Grok**: Generally faster than alternatives
5. **Upgrade providers**: Use newer models (grok-2 > grok-1)

### Q: How much do API calls cost?

Varies by provider:

| Provider | Cost | Free Tier |
|----------|------|-----------|
| Grok | ~$0.02/1K input tokens | $5 free |
| Gemini | Free - $0.075/1K | ✅ Free |
| Claude | ~$0.003/1K input | Free credits |
| GPT-4o | ~$0.015/1K input | $5 free |

**Typical session** (~5K tokens input, 10K output):
- Grok: ~$0.15-0.20
- Gemini (free): $0
- Claude: ~$0.02
- GPT-4o: ~$0.20

---

## Troubleshooting

### Q: "Invalid API Key" error

**Solutions**:
1. Check key format
   ```bash
   echo $XAI_API_KEY  # Should start with xai-
   ```
2. Check for whitespace
   ```bash
   echo "$XAI_API_KEY" | od -c  # Look for spaces
   ```
3. Regenerate key from provider website
4. Run setup again: `./gorkbot setup`

### Q: "Connection refused"

**Solutions**:
1. Check internet: `ping google.com`
2. Check firewall: May block API calls
3. Try VPN: Some networks block external APIs
4. Try different provider: `/model`

### Q: "Out of memory"

**Solutions**:
1. Compress history: `/compress`
2. Clear session: `/clear`
3. Close other apps
4. Increase system RAM

### Q: "Tool execution failed"

**Solutions**:
1. Check tool name: `/tools`
2. Check tool docs: `/tool-info <name>`
3. Enable debug: `-debug-mcp`
4. Check audit log: `gorkbot.db` tool_calls table

### Q: "Context limit exceeded"

**Solutions**:
1. Compress history: `/compress`
2. Export and archive old conversations
3. Use shorter prompts
4. Switch to model with larger context (Gemini 200K, Moonshot 200K)

---

## Advanced Questions

### Q: Can I deploy Gorkbot on a server?

Yes! Options:

1. **Docker**: Containerized deployment
   ```bash
   docker run -it ghcr.io/velariumai/gorkbot:latest
   ```

2. **Web UI**: Server mode on port 8080
   ```bash
   ./gorkbot -web -port 8080
   ```

3. **MCP Servers**: Integrate with other tools
   - 12+ supported servers
   - Configure in `mcp.json`

### Q: Can I use Gorkbot in CI/CD?

Yes! One-shot mode:

```bash
# In GitHub Actions
./gorkbot -p "Run tests and report" > output.txt
```

Or programmatically:
```go
// Use orchestrator package directly
orch.ExecuteTask(ctx, "Run pytest on changes")
```

### Q: How do I integrate with my project?

Options:

1. **GORKBOT.md**: Project-specific instructions
   - Automatically loaded
   - Injected into system prompt
   - Watched for changes

2. **Custom tools**: Create tools for your workflow
   - `create_tool` to generate
   - Available immediately

3. **MCP servers**: Connect to project tools
   - Git integration
   - Database queries
   - Custom APIs

### Q: Can I use Gorkbot programmatically?

Yes! Import the packages:

```go
import (
    "github.com/velariumai/gorkbot/internal/engine"
    "github.com/velariumai/gorkbot/pkg/ai"
    "github.com/velariumai/gorkbot/pkg/tools"
)

// Create orchestrator
orch, err := engine.NewOrchestrator(...)

// Execute task
result, err := orch.ExecuteTask(ctx, "What is 2+2?")
```

---

## Getting Help

- 📖 **Full docs**: [README.md](README.md)
- 🆘 **Troubleshooting**: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- 🐛 **Report bugs**: [GitHub Issues](https://github.com/velariumai/gorkbot/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/velariumai/gorkbot/discussions)
- 📧 **Email**: velarium.ai@gmail.com

---

**Still have questions? Open an issue or email us!**

