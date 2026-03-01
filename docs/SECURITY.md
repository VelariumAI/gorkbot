# Gorkbot Security Guide

**Version:** 3.4.0

This document covers API key security, encrypted credential storage, the tool permission model, shell execution safety, and recommended security practices.

---

## Table of Contents

1. [API Key Security](#1-api-key-security)
2. [Encrypted Credential Storage](#2-encrypted-credential-storage)
3. [Tool Permission System](#3-tool-permission-system)
4. [Shell Execution Safety](#4-shell-execution-safety)
5. [Security Tool Category](#5-security-tool-category)
6. [Secret Scanning](#6-secret-scanning)
7. [Network Security](#7-network-security)
8. [Security Checklist](#8-security-checklist)

---

## 1. API Key Security

### What to Keep Secret

All five provider API keys must be kept private:

| Key | Prefix | Never share |
|-----|--------|-------------|
| xAI | `xai-` | Yes |
| Google Gemini | `AIzaSy` | Yes |
| Anthropic | `sk-ant-` | Yes |
| OpenAI | `sk-` | Yes |
| MiniMax | varies | Yes |

If any key is exposed, revoke it immediately in the respective provider's console and generate a new one.

### Safe Storage Locations

✅ **Acceptable:**
- `.env` file in the project root (gitignored by default)
- Shell profile (`~/.bashrc`, `~/.zshrc`) via `export KEY=value`
- OS keyring / secret manager
- `~/.config/gorkbot/api_keys.json` (0600 permissions, encrypted at rest)

❌ **Never:**
- Hardcoded in source code
- Committed to any git repository (public or private)
- Included in logs, error messages, or screenshots
- Sent in chat, email, or issue tracker comments

### Verifying .env is Gitignored

```bash
# Check that .env is in .gitignore
grep -E '^\.env$' .gitignore

# Verify git does not track .env
git status --ignored | grep '\.env'

# Search git history for accidental commits
git log -S 'xai-' --all
git log -S 'AIzaSy' --all
```

If a key was accidentally committed, rotate it immediately in the provider console, then use `git filter-branch` or `git filter-repo` to scrub the history.

---

## 2. Encrypted Credential Storage

### .env Value Encryption

Values in `.env` prefixed with `ENC_` are decrypted at startup. The encryption module (`pkg/security`) uses **AES-GCM** (256-bit) with a locally managed key.

Encryption can be triggered from within a session to protect keys at rest:

```
/key encrypt xai    # encrypt the current xAI key value in .env
```

Encrypted values look like:

```env
XAI_API_KEY=ENC_base64encodedciphertext==
```

### api_keys.json

**Path:** `~/.config/gorkbot/api_keys.json`
**Permissions:** 0600 (owner read/write only — created with `os.O_WRONLY|os.CREATE, 0600`)

Keys are stored after validation. If a key fails validation, it is stored with status `invalid` and not used for API calls.

### Pre-commit Hook (Recommended)

Add this to `.git/hooks/pre-commit` to prevent accidental key commits:

```bash
#!/bin/bash
set -e

patterns=(
    'xai-[A-Za-z0-9]'
    'AIzaSy[A-Za-z0-9]'
    'sk-ant-[A-Za-z0-9]'
    'sk-[A-Za-z0-9]{20}'
)

for pattern in "${patterns[@]}"; do
    if git diff --cached | grep -qE "$pattern"; then
        echo "ERROR: Detected potential API key in staged changes: $pattern"
        echo "Remove the secret before committing."
        exit 1
    fi
done
```

```bash
chmod +x .git/hooks/pre-commit
```

---

## 3. Tool Permission System

All tools that modify state, execute commands, or make network requests are gated by the permission system. See [PERMISSIONS_GUIDE.md](PERMISSIONS_GUIDE.md) for the full reference.

### Principle of Least Privilege

Default permissions are conservative:

| Risk Level | Default Permission | Examples |
|------------|-------------------|---------|
| Read-only, safe | `always` | `read_file`, `list_directory`, `git_status`, `system_info` |
| Modifying, limited impact | `session` | `web_fetch`, `grep_content` |
| Modifying, significant impact | `once` | `bash`, `write_file`, `git_commit`, `git_push` |
| Destructive | `once` | `delete_file`, `kill_process` |

### Security Tool Category

The security and pentesting tool categories (`security`, `pentest`) are **disabled by default**. Enable them explicitly via `/settings → Tool Groups` only when you need them for authorized testing.

### Rule Engine

For automation workflows where you want to pre-approve certain tools without interactive prompts, use the rule engine:

```
/rules add allow "read_*"           # allow all read tools
/rules add allow "git_status"       # allow git status specifically
/rules add deny "delete_*"          # block all delete tools
/rules add deny "kill_process"      # block kill_process
```

Rules use glob patterns and are evaluated before the standard permission check.

---

## 4. Shell Execution Safety

### Shell Escaping

Every bash tool call passes user-provided parameters through `shellescape()` before substituting them into command templates. This prevents shell injection attacks where a crafted parameter value could escape the intended command context.

```go
// pkg/tools/bash.go
func shellescape(s string) string {
    // Wraps the string in single quotes and escapes internal single quotes
    return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
```

### Execution Timeouts

Every tool call has a hard timeout (configurable per tool, default 30 seconds). This prevents runaway commands from hanging the orchestrator.

### Sandboxing Considerations

Gorkbot is not a container or VM — it runs tools in the same user context as the process. For maximum isolation:

- Run Gorkbot in a separate user account
- Use the `--allow-tools` flag in one-shot mode to whitelist only necessary tools
- Use `spawn_sub_agent` with `isolated=true` to run sub-agents in separate git worktrees

### Dynamic Tool Safety

Tools created via `create_tool` execute shell commands via the same `bash` execution path with the same escaping and timeout protections. User review of dynamically created tool commands is recommended before granting `always` permission.

---

## 5. Security Tool Category

Gorkbot includes a comprehensive pentesting and security assessment suite (30+ tools). These tools are:

- **Disabled by default** — must be explicitly enabled via `/settings`
- **Authorized use only** — intended for penetration testing, CTF challenges, authorized security research, and defensive analysis
- **Permission-gated** — all require at least `once` permission (prompt each call)

### Enabling Security Tools

```
/settings
→ Tool Groups
→ Enable: Security Recon, Security Pentest
```

Or via the `disabledCategories` field in `app_state.json` (remove the category from the list).

### Authorized Use Statement

These tools must only be used against systems you own or have explicit written authorization to test. Misuse may violate computer fraud laws. The Gorkbot project and Velarium AI accept no liability for unauthorized use.

---

## 6. Secret Scanning

### gitleaks (Recommended)

```bash
# Install
brew install gitleaks  # macOS
# or
go install github.com/gitleaks/gitleaks/v8@latest

# Scan repo
gitleaks detect --source . --verbose

# Scan git history
gitleaks detect --source . --log-opts="--all"
```

### git-secrets (AWS Tool)

```bash
git clone https://github.com/awslabs/git-secrets
cd git-secrets && make install

cd /path/to/gorkbot
git secrets --install
git secrets --register-aws
git secrets --add 'xai-[A-Za-z0-9]+'
git secrets --add 'AIzaSy[A-Za-z0-9]+'
git secrets --add 'sk-ant-[A-Za-z0-9]+'
```

### TruffleHog

```bash
trufflehog git file://. --only-verified
```

---

## 7. Network Security

### API Endpoints

All provider API calls use HTTPS. Gorkbot does not support plain HTTP for AI provider communication.

| Provider | Endpoint |
|----------|---------|
| xAI | `https://api.x.ai/v1` |
| Google Gemini | `https://generativelanguage.googleapis.com/v1beta` |
| Anthropic | `https://api.anthropic.com/v1` |
| OpenAI | `https://api.openai.com/v1` |
| MiniMax | `https://api.minimax.io/anthropic/v1` |

### A2A Gateway

The A2A HTTP gateway (`--a2a`) binds to `127.0.0.1` by default — loopback only, not accessible from the network. To expose it on the network (for multi-agent setups), use `--a2a-addr 0.0.0.0:18890` only on trusted networks and behind appropriate firewall rules.

### SSE Relay

The `--share` relay listens on a random localhost port and is accessible only within the local network. Do not expose it to the public internet without appropriate security measures (authentication, TLS).

### web_fetch and http_request Tools

These tools can make arbitrary HTTP/S requests. Ensure they are not misused for:
- SSRF (Server-Side Request Forgery) attacks against internal services
- Exfiltration of sensitive data

Use the `/rules` and `--deny-tools` mechanisms to restrict web tools in untrusted contexts.

---

## 8. Security Checklist

### API Keys

- [ ] `.env` is listed in `.gitignore`
- [ ] `git status --ignored` shows `.env` as ignored
- [ ] No API keys in source code or comments
- [ ] Keys rotated if previously exposed
- [ ] Pre-commit hook installed for key detection

### Permissions

- [ ] Security tool category disabled (default)
- [ ] Destructive tools (`delete_file`, `git_push`, `kill_process`) at `once` permission
- [ ] `bash` at `once` permission unless in fully trusted session
- [ ] `/permissions` audited for unexpected `always` grants

### Process

- [ ] Running as non-root user
- [ ] `--allow-tools` filter applied in automated/CI contexts
- [ ] One-shot mode used for scripted automation (no interactive permission prompts)
- [ ] Execution traces (`--trace`) not stored in world-readable locations

### Updates

- [ ] Gorkbot updated to latest version
- [ ] `go mod tidy` run to update dependencies
- [ ] Dependency vulnerabilities checked with `govulncheck ./...`
