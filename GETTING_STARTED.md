# Getting Started with Gorkbot

## 🚀 Quick Start (2 Minutes)

### Step 1: Run Setup Wizard

```bash
./gorkbot.sh setup
```

The wizard will walk you through configuring your API keys:

```
╔════════════════════════════════════════════════════════════╗
║          Welcome to Gorkbot Setup Wizard! 🚀             ║
╚════════════════════════════════════════════════════════════╝

This wizard will help you configure your API keys.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Step 1: xAI API Key (for Grok)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📍 Get your xAI API key from:
   https://console.x.ai/

📋 Paste your xAI API key: _
```

### Step 2: Get Your API Keys

#### For Grok (xAI):

1. Go to **https://console.x.ai/**
2. Sign in to x.AI console
3. Click **"API Keys"**
4. Click **"Create API Key"**
5. Copy the key and paste it into the wizard

#### For Gemini (Google):

1. Go to **https://aistudio.google.com/apikey**
2. Sign in with your Google account
3. Click **"Create API Key"**
4. Select **"Create API key in new project"** (or use existing)
5. Copy the key and paste it into the wizard

### Step 3: Start Chatting!

```bash
# Interactive TUI mode
./gorkbot.sh

# Or one-shot mode
./gorkbot.sh -p "What is the meaning of life?"
```

## 📊 Check Configuration

See what's configured:

```bash
./gorkbot.sh status
```

Output:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Gorkbot Configuration Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🤖 Grok (xAI):
   ✓ Configured (xai-xxxx...xxxx)

💎 Gemini (Google):
   ✓ Configured (AIza...xxxx)

✓ All systems ready!

Run: ./gorkbot.sh
```

## 🎮 Usage

### Interactive TUI Mode

```bash
./gorkbot.sh
```

Features:
- Beautiful terminal UI
- Markdown rendering
- Code syntax highlighting
- Scrollable history
- Multi-line input (Alt+Enter)
- Slash commands

### One-Shot Mode

```bash
./gorkbot.sh -p "Your question here"
```

Perfect for:
- Quick queries
- Scripting
- CI/CD pipelines

### Advanced Options

```bash
# Enable verbose consultant thinking
./gorkbot.sh -verbose-thoughts

# Set timeout
./gorkbot.sh -p "Complex query" -timeout 120s

# Enable debug watchdog
./gorkbot.sh -watchdog
```

## 🔧 Re-configuring

Need to change your API keys?

```bash
# Run setup again
./gorkbot.sh setup

# It will detect existing keys and ask if you want to keep them
```

## 📁 Configuration File

Your keys are stored in:
```
.env
```

**Important:**
- ✅ This file is gitignored (won't be committed)
- ❌ Never share this file
- ❌ Never commit to git

Manual edit:
```bash
nano .env
```

```env
# Gorkbot Configuration

# xAI API Key (for Grok)
XAI_API_KEY=xai-xxxxxxxxxxxxx

# Google API Key (for Gemini)
GEMINI_API_KEY=AIzaxxxxxxxxxxxxxxx
```

## 🎯 TUI Features

### Slash Commands

Type these in the TUI:

- `/help` - Show all commands
- `/clear` - Clear conversation
- `/model <name>` - Switch AI model
- `/theme <dark|light>` - Change theme
- `/version` - Show version
- `/quit` - Exit

### Keyboard Shortcuts

- **Enter** - Send message
- **Alt+Enter** - New line (multi-line input)
- **Ctrl+C** - Quit
- **PgUp/PgDn** - Scroll conversation
- **Esc** - Cancel generation

### Consultant Mode

When you ask complex questions or use keywords like "COMPLEX" or "REFRESH", Gorkbot will consult Gemini:

```
You: COMPLEX: Design a microservices architecture

╭──────────────────────────────────────────╮
│ 💎 Consultant (Gemini)                  │
│                                          │
│ Here's my architectural recommendation: │
│ - Use API Gateway pattern               │
│ - Event-driven communication            │
│ - CQRS for complex domains              │
╰──────────────────────────────────────────╯

Grok: Based on Gemini's advice, here's a
complete implementation plan...
```

## 🐛 Troubleshooting

### "XAI_API_KEY is not set"

Run setup:
```bash
./gorkbot.sh setup
```

### "GEMINI_API_KEY is not set"

Run setup:
```bash
./gorkbot.sh setup
```

### Keys not loading

Make sure you're using the wrapper script:
```bash
./gorkbot.sh  # ✓ Loads .env
./bin/gorkbot # ✗ Doesn't load .env
```

Or export manually:
```bash
export XAI_API_KEY="your-key"
export GEMINI_API_KEY="your-key"
./bin/gorkbot
```

### API Errors

**Invalid API Key:**
- Make sure you copied the full key
- Check for extra spaces
- Regenerate key from console

**Rate Limit:**
- Wait a few minutes
- Upgrade your API plan
- Use different key

**Network Errors:**
- Check your internet connection
- Try again in a few seconds
- Check API status pages

## 💡 Tips

1. **Multi-line prompts:** Use Alt+Enter for complex questions
2. **Scroll history:** PgUp/PgDn to review conversation
3. **Quick exit:** Ctrl+C or `/quit`
4. **Theme preference:** `/theme light` for bright environments
5. **Complex queries:** Use "COMPLEX:" prefix to engage Gemini

## 📚 Next Steps

- ✅ Configure API keys (setup wizard)
- ✅ Start the TUI (`./gorkbot.sh`)
- ✅ Try slash commands (`/help`)
- ✅ Ask complex questions
- ✅ Explore the features

## 🔗 Resources

- **xAI Console:** https://console.x.ai/
- **Google AI Studio:** https://aistudio.google.com/apikey
- **Grok Documentation:** https://docs.x.ai/
- **Gemini Documentation:** https://ai.google.dev/

## 🎉 That's It!

You're ready to use Gorkbot. Happy chatting! 🚀

Need help? Run:
```bash
./gorkbot.sh help
```
