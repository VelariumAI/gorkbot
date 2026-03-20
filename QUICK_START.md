# Quick Start Guide (5 Minutes)

Get Gorkbot running in under 5 minutes with this fast-track guide.

---

## Prerequisites

- **Go 1.25.0+** or **Docker**
- **API Keys** from at least one provider:
  - [xAI Grok Console](https://console.x.ai/)
  - [Google AI Studio](https://aistudio.google.com/apikey)
  - [Anthropic Console](https://console.anthropic.com/)
  - [OpenAI Platform](https://platform.openai.com/api-keys)

---

## Installation

### Option 1: Binary Download (Fastest)

```bash
# Download latest release
wget https://github.com/velariumai/gorkbot/releases/latest/download/gorkbot-linux-amd64
chmod +x gorkbot-linux-amd64

# Run with API key
XAI_API_KEY=xai-xxx ./gorkbot-linux-amd64
```

### Option 2: Build from Source

```bash
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot
make build

# Output: ./bin/gorkbot
```

### Option 3: Docker

```bash
docker run -it \
  -e XAI_API_KEY=xai-xxx \
  -e GEMINI_API_KEY=AIza-xxx \
  ghcr.io/velariumai/gorkbot:latest
```

---

## First Run

### Method A: Interactive Setup (Recommended)

```bash
./gorkbot setup
```

Follow the wizard to:
1. Enter API keys
2. Select primary model (default: Grok)
3. Configure preferences
4. Test provider connectivity

### Method B: Environment Variables

```bash
export XAI_API_KEY=xai-xxxxxxxxxxxxx
export GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx

./gorkbot
```

### Method C: .env File

Create `.env` in the project directory:

```bash
XAI_API_KEY=xai-xxxxxxxxxxxxx
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx
ANTHROPIC_API_KEY=sk-ant-xxxxxxxxxxxxx
```

Then run:

```bash
./gorkbot
```

---

## Basic Commands

### Interactive TUI

```bash
./gorkbot
```

**Keyboard Shortcuts:**
- `Ctrl+C` - Exit
- `Tab` - Switch between views
- `Ctrl+L` - Clear screen
- `Ctrl+U` - Clear input line
- `/help` - Show all commands

### One-Shot Queries

```bash
# Ask a single question
./gorkbot -p "What is quantum entanglement?"

# With extended thinking
./gorkbot -p "Explain machine learning" -thinking-budget 5000

# Use specific model
./gorkbot -p "Hello" -model grok-2-1212
```

### Common Commands in TUI

```
/help          Show all available commands
/clear         Clear conversation history
/model         List available models
/settings      Configure preferences
/version       Show version info
/theme         Change color theme
/compress      Compress conversation history
/export        Export conversation to file
/tools         List all available tools
/debug         Toggle debug mode
```

---

## Example Tasks

### 1. Write Code

```
You: "Write a Python script that calculates fibonacci numbers"

Gorkbot will:
- Generate code
- Offer to save to file (via bash tool)
- Explain the implementation
```

### 2. Analyze Files

```
You: "Analyze this Dockerfile: /path/to/Dockerfile"

Gorkbot will:
- Read the file (via read_file tool)
- Identify issues
- Suggest improvements
```

### 3. Git Operations

```
You: "What changes are uncommitted? Show the diff"

Gorkbot will:
- Check git status (via git_status tool)
- Display the diff (via git_diff tool)
- Recommend commit message
```

### 4. Web Research

```
You: "Fetch the latest news from HackerNews and summarize"

Gorkbot will:
- Fetch the page (via web_fetch tool)
- Parse the content
- Provide a summary
```

### 5. System Admin

```
You: "What processes are using the most CPU?"

Gorkbot will:
- List processes (via list_processes tool)
- Analyze resource usage
- Suggest optimization strategies
```

---

## Configuration

### Model Selection

Change your primary model in the TUI:

```
/model
```

Or edit `~/.config/gorkbot/app_state.json`:

```json
{
  "primary_provider": "google",
  "primary_model": "gemini-2-flash"
}
```

### Disable Tool Categories

Some tools might require approval. To disable a category:

```
/settings
```

Or edit `app_state.json`:

```json
{
  "disabled_categories": ["security", "bash"]
}
```

### Verbose Mode

To see all internal reasoning and system messages:

```
/verbose on
```

Or in `app_state.json`:

```json
{
  "verbose_mode": true
}
```

---

## Troubleshooting

### "API Key Invalid"

- ✅ Check key format (should start with `xai-`, `AIza-`, `sk-ant-`, etc.)
- ✅ Verify key hasn't expired
- ✅ Check for trailing whitespace
- ✅ Ensure environment variable is set correctly

### "Connection Refused"

- ✅ Verify internet connectivity: `ping google.com`
- ✅ Check firewall settings
- ✅ Try a different provider (fallback to Gemini, Claude, etc.)

### "Context Limit Exceeded"

- Run `/compress` to compact history
- Disable less important context: `/settings`
- Start a new session

### "Tool Execution Failed"

- Check tool syntax: `/tools`
- Read tool description: `/tool-info <name>`
- Enable debug mode: `-debug-mcp`
- Review audit log: `~/.config/gorkbot/gorkbot.db`

---

## Next Steps

- 📖 Read [GETTING_STARTED.md](GETTING_STARTED.md) for detailed setup
- 🏗️ Review [ARCHITECTURE.md](ARCHITECTURE.md) to understand the system
- 🔧 Explore [TOOL_REFERENCE.md](TOOL_REFERENCE.md) for all available tools
- ❓ Check [FAQ.md](FAQ.md) for common questions
- 💬 Join [GitHub Discussions](https://github.com/velariumai/gorkbot/discussions)

---

## Need Help?

- 🆘 **Troubleshooting**: See [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- ❓ **FAQ**: See [FAQ.md](FAQ.md)
- 📚 **Full Docs**: See [Documentation](#documentation) in README
- 🐛 **Report Issues**: [GitHub Issues](https://github.com/velariumai/gorkbot/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/velariumai/gorkbot/discussions)

---

**Happy coding! 🎉**
