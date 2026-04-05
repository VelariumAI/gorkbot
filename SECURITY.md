# Security Policy

Gorkbot takes security seriously. This document outlines security practices, vulnerability reporting, and best practices.

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Supported Versions](#supported-versions)
3. [Reporting Vulnerabilities](#reporting-vulnerabilities)
4. [Security Features](#security-features)
5. [Best Practices](#best-practices)
6. [Known Limitations](#known-limitations)

---

## Security Overview

Gorkbot implements **defense-in-depth security** with multiple layers:

1. **Input Validation**: SENSE InputSanitizer validates all user inputs
2. **HITL Approval**: High-stakes operations require explicit approval
3. **Tool Permissions**: Per-tool approval levels (once/session/always/never)
4. **Audit Logging**: All tool executions logged to SQLite
5. **Encrypted Storage**: Optional encryption of API keys
6. **CSP Headers**: Web UI with strict Content Security Policy
7. **Shell Escaping**: All bash parameters properly quoted

---

## Supported Versions

| Version | Status | Support Until |
|---------|--------|-----------------|
| 1.2.0 | Current | 2026-12-31 |
| 1.1.x | Supported | 2026-06-30 |
| 1.0.x | Deprecated | 2025-12-31 |

**Policy**: Latest version receives all updates; previous version receives critical fixes only.

---

## Reporting Vulnerabilities

### **DO NOT** Open Public Issues

Security vulnerabilities must be reported **privately** before public disclosure.

### Report To

**Email**: velarium.ai@gmail.com

**Subject**: `[SECURITY] Vulnerability Report`

### Include

1. **Description**: Clear summary of vulnerability
2. **Type**: (e.g., XSS, SQL Injection, Authentication bypass)
3. **Severity**: (Critical/High/Medium/Low)
4. **Steps to reproduce**: Exact steps to trigger
5. **Affected versions**: Which versions are vulnerable
6. **Potential impact**: What could attackers do
7. **Suggested fix**: (Optional) How to fix

### Example

```
Subject: [SECURITY] XSS vulnerability in web UI

Description:
Gorkbot web UI is vulnerable to cross-site scripting (XSS) 
via the chat input field.

Type: Cross-Site Scripting (XSS)
Severity: High
Affected versions: 1.6.1-rc

Steps to reproduce:
1. Open web UI at localhost:8080
2. Paste in chat: <img src=x onerror="alert('XSS')">
3. See JavaScript executed

Impact:
- Attacker could steal session cookies
- Could redirect to phishing site
- Could log keypresses

Suggested fix:
Use html/template instead of raw HTML rendering
```

### Response Timeline

- **24 hours**: Acknowledgment of report
- **7 days**: Initial assessment
- **30 days**: Fix and security release
- **Public disclosure**: After patch released + 30 days

---

## Security Features

### 1. Input Sanitization (SENSE InputSanitizer)

All user inputs validated for:

- **Path traversal**: Validates file paths against whitelist
  - Allowed: `$HOME`, `/tmp`, `/var/tmp`, `/sdcard`, project directory
  - Blocked: `/../etc/passwd`, `/root/.ssh/id_rsa`

- **ANSI escape sequences**: Prevents terminal injection
  - Strips: `\x1b`, `\033` escape codes
  - Prevents: Terminal state manipulation

- **SQL patterns**: Basic SQL injection detection
  - Blocked: `'; DROP TABLE`, `1' OR '1'='1`

- **Shell metacharacters**: Quoted in bash execution
  - Bash params: `exec "$1"` with proper quoting
  - Prevents: Command injection via special chars

### 2. HITL Approval Gateway

High-stakes operations require explicit user approval:

- **Bash execution**: All shell commands
- **File deletion**: Destructive file operations
- **Git operations**: Push, reset, force operations
- **Package installation**: System package installation
- **HTTP requests**: Outbound web requests

**Risk Classification**: 4 levels
- **Low**: File reading, git status checks
- **Medium**: File modification, web fetches
- **High**: File deletion, git operations
- **Critical**: Bash execution, package installation

**Auto-approval** only if:
- Confidence score ≥ 85% AND past precedent ≥ 2 similar operations
- **Critical operations never auto-approved**

### 3. Tool Permissions System

```json
{
  "bash": "once",                  // Ask each time
  "delete_file": "never",           // Always blocked
  "read_file": "always",            // Always approved
  "web_fetch": "session",           // Approved for current session
  "git_push": "once"
}
```

**Levels**:
- `always`: Permanently approved
- `session`: Approved until session ends
- `once`: Ask every time (default)
- `never`: Permanently blocked

### 4. Audit Logging

All tool executions logged to SQLite:

```
gorkbot.db - tool_calls table:
- timestamp: When executed
- tool: Tool name
- params: Tool parameters (input)
- result: Tool result (output)
- duration: Execution time
- user: Session user
- status: Success/Failure
```

**Retention**: 10,000 records max, oldest pruned after 12 hours

### 5. API Key Security

**Storage Options**:

1. **Environment Variables** (default, unencrypted)
   ```bash
   export XAI_API_KEY=xai-xxx
   ```

2. **.env File** (unencrypted in plain text)
   ```bash
   XAI_API_KEY=xai-xxx
   ```

3. **Encrypted Storage** (optional)
   ```bash
   ./gorkbot setup
   # Choose "Encrypt API keys"
   ```

**.env File Security**:
- Should be in `.gitignore` (default)
- File permissions: 0600 (user read/write only)
- Never commit to version control
- Rotate keys regularly

### 6. Web UI Security (CSP Headers)

Strict Content Security Policy:

```
default-src 'self'
script-src 'self' https://cdnjs.cloudflare.com
style-src 'self' https://cdnjs.cloudflare.com
img-src 'self' data:
connect-src 'self'
object-src 'none'
frame-ancestors 'none'
```

**Additional Headers**:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`

### 7. Shell Escaping

All bash parameters properly quoted:

```go
// Secure: Use shell quoting
cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "exec \"$1\"", "sh", userPath)

// Insecure (DO NOT USE):
cmd := exec.CommandContext(ctx, "/bin/sh", userPath)  // Shell injection risk
```

---

## Best Practices

### For Users

1. **Rotate API Keys**: Change keys monthly or when rotated
   ```bash
   # Generate new key from provider website
   # Update in .env
   # Restart Gorkbot
   ```

2. **Use Encrypted Storage**: For sensitive environments
   ```bash
   ./gorkbot setup
   # Choose encryption option
   ```

3. **Review Audit Log**: Check what tools have been executed
   ```sql
   -- Query audit log
   sqlite3 ~/.config/gorkbot/gorkbot.db \
     "SELECT tool, params, result, timestamp FROM tool_calls LIMIT 10;"
   ```

4. **Limit Tool Access**: Disable unneeded tool categories
   ```
   /settings → Disable: security, bash
   ```

5. **Monitor SENSE Traces**: Check for suspicious activity
   ```bash
   # Daily traces
   cat ~/.config/gorkbot/trace/2026-03-20.jsonl | jq 'select(.kind == "hallucination")'
   ```

### For Operators

1. **Network Isolation**: Run in sandbox if untrusted input
   - Docker container with resource limits
   - Network namespace with restricted egress

2. **Update Regularly**: Keep Gorkbot updated
   ```bash
   git pull
   make build
   ```

3. **Monitor Resources**: Check for resource exhaustion
   ```bash
   # Monitor memory usage
   ps aux | grep gorkbot
   
   # Check database size
   ls -lh ~/.config/gorkbot/gorkbot.db
   ```

4. **Audit Trail**: Maintain audit logs
   ```bash
   # Backup audit database weekly
   cp ~/.config/gorkbot/gorkbot.db backups/gorkbot-$(date +%Y%m%d).db
   ```

5. **Secret Rotation**: Rotate API keys quarterly
   ```bash
   # Generate new keys from provider consoles
   # Update .env
   # Restart Gorkbot
   # Delete old keys from provider
   ```

---

## Known Limitations

### Not Protected Against

1. **Malicious AI Responses**: Gorkbot cannot validate that AI responses are truthful
   - Users should verify critical information independently
   - AI can hallucinate, generate malicious code

2. **Compromised API Keys**: If API key is leaked, attacker can impersonate user
   - Keep keys secret and rotate regularly
   - Never commit .env to version control

3. **Network-Level Attacks**: If network is compromised (e.g., DNS poisoning)
   - MitM attacks could intercept API calls
   - Use VPN for additional protection

4. **Physical Access**: If machine is physically compromised
   - Attacker could read memory or copy database
   - Use full-disk encryption for additional protection

5. **Zero-Days in Dependencies**: Unknown vulnerabilities in Go packages
   - Regularly update with `go get -u ./...`
   - Subscribe to Go security mailing list

---

## Dependency Security

### Scanning

```bash
# Check for known vulnerabilities
go list -json -m all | nancy sleuth

# Or using govulncheck
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

### Updates

```bash
# Check for updates
go list -u -m all

# Update all dependencies
go get -u ./...

# Update specific package
go get -u github.com/package/name

# Verify dependencies
go mod verify
```

---

## Reporting Non-Security Issues

- Use [GitHub Issues](https://github.com/velariumai/gorkbot/issues)
- Follow [Contributing Guide](CONTRIBUTING.md)
- Be specific with reproduction steps

---

## Contact

**Security Issues**: velarium.ai@gmail.com
**General Questions**: velarium.ai@gmail.com
**GitHub**: https://github.com/velariumai/gorkbot

---

**Last Updated**: April 5, 2026
**Version**: 1.6.1-rc
