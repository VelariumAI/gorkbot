# Troubleshooting Guide

Solutions to common Gorkbot issues.

---

## Installation Issues

### "Go: command not found"

**Cause**: Go not installed or not in PATH

**Solution**:
```bash
# Check if Go is installed
which go

# If not, install it
# Ubuntu/Debian
sudo apt install golang-1.25

# macOS
brew install go@1.25

# Windows
# Download from https://golang.org/dl/

# Verify
go version
```

### "Failed to download dependencies"

**Cause**: Network issue or missing go.mod

**Solution**:
```bash
# Clear module cache
go clean -modcache

# Download again
go mod download

# Verify
go mod tidy

# Try again
make build
```

### "Permission denied" when running binary

**Cause**: Binary not executable

**Solution**:
```bash
chmod +x bin/gorkbot
./bin/gorkbot --version
```

---

## API Key Issues

### "Invalid API Key"

**Symptoms**: "Authentication failed", "Invalid credentials"

**Solutions**:

1. **Check key format**:
   ```bash
   echo $XAI_API_KEY
   # Should start with: xai-
   # Should be ~50 characters
   # No spaces or newlines
   ```

2. **Check for whitespace**:
   ```bash
   echo "$XAI_API_KEY" | od -c
   # Look for 0x20 (space) or 0x0a (newline)
   ```

3. **Regenerate key**:
   - Go to provider website
   - Delete old key
   - Create new key
   - Copy carefully (no spaces)

4. **Re-configure**:
   ```bash
   ./gorkbot setup
   # Re-enter key when prompted
   ```

5. **Check environment**:
   ```bash
   # Ensure .env is not being overridden
   unset XAI_API_KEY
   export XAI_API_KEY=xai-newkey
   ./gorkbot
   ```

### "API key not found"

**Cause**: Key not set in environment or .env

**Solution**:
```bash
# Option 1: Set environment variable
export XAI_API_KEY=xai-xxx
./gorkbot

# Option 2: Create .env file
cat > .env << 'EOF'
XAI_API_KEY=xai-xxx
GEMINI_API_KEY=AIza-xxx
EOF
./gorkbot

# Option 3: Run setup wizard
./gorkbot setup
```

### "Provider returned 401 Unauthorized"

**Cause**: API key expired or revoked

**Solution**:
1. Go to provider dashboard
2. Check if key is still active
3. If revoked, create new key
4. Update in .env or setup
5. Delete old key from provider

---

## Connection Issues

### "Connection refused"

**Cause**: Network connectivity problem

**Solutions**:

1. **Test internet**:
   ```bash
   ping google.com
   # Should get responses
   ```

2. **Test DNS**:
   ```bash
   nslookup api.x.ai
   # Should resolve to IP address
   ```

3. **Check firewall**:
   ```bash
   # May need to allow outbound HTTPS
   # Check firewall rules
   ```

4. **Try different provider**:
   ```
   /model
   # Select Gemini or Claude
   ```

5. **Try with proxy**:
   ```bash
   export HTTP_PROXY=http://proxy:8080
   export HTTPS_PROXY=https://proxy:8080
   ./gorkbot
   ```

### "Connection timeout"

**Cause**: Provider is slow or network is slow

**Solutions**:

1. **Increase timeout**:
   ```bash
   export GORKBOT_TIMEOUT=30
   ./gorkbot
   ```

2. **Try different provider**: May be faster
3. **Check network**: May be congested
4. **Try later**: Provider may be temporarily slow

### "TLS certificate error"

**Cause**: SSL/TLS certificate validation failure

**Solution**:
```bash
# Update system certificates
# Ubuntu/Debian
sudo apt install ca-certificates
sudo update-ca-certificates

# macOS
# Reinstall Python certificates
/Applications/Python\ 3.x/Install\ Certificates.command

# Windows
# Update certificates in settings
```

---

## Performance Issues

### "Gorkbot is running slowly"

**Causes & Solutions**:

1. **Large conversation history**:
   ```
   /compress       # Compact history
   /clear          # Clear conversation
   ```

2. **Provider is slow**:
   ```
   /model          # Try different provider
   /model          # Try newer model version
   ```

3. **Network is slow**:
   ```bash
   # Check internet speed
   ping -c 5 google.com
   
   # Check DNS
   nslookup api.x.ai
   ```

4. **System is low on memory**:
   ```bash
   # Check memory usage
   free -h
   
   # Close other apps
   ```

5. **Tool execution is slow**:
   ```bash
   # Enable debug logging
   ./gorkbot -debug-mcp
   
   # Check logs
   tail -f ~/.config/gorkbot/gorkbot.json
   ```

### "High memory usage"

**Solutions**:

1. **Compress history**:
   ```
   /compress
   ```

2. **Clear old conversations**:
   ```bash
   # Delete database and start fresh
   rm ~/.config/gorkbot/gorkbot.db
   ```

3. **Monitor memory**:
   ```bash
   # Watch memory usage
   top -p $(pgrep gorkbot)
   ```

4. **Check large files**:
   ```bash
   # Find large files
   du -sh ~/.config/gorkbot/*
   ```

---

## Tool Execution Issues

### "Tool execution failed"

**Solutions**:

1. **Check tool exists**:
   ```
   /tools          # List available tools
   /tool-info bash # Check specific tool
   ```

2. **Check tool syntax**:
   ```
   # Tool parameters should match description
   /tool-info <tool_name>
   ```

3. **Check permissions**:
   ```
   # May need approval for sensitive tools
   # Approve when prompted
   ```

4. **Enable debug logging**:
   ```bash
   ./gorkbot -debug-mcp
   ```

5. **Check audit log**:
   ```bash
   # View recent tool executions
   sqlite3 ~/.config/gorkbot/gorkbot.db \
     "SELECT tool, params, result FROM tool_calls LIMIT 5;"
   ```

### "Bash command failed"

**Solutions**:

1. **Test command directly**:
   ```bash
   # Try running command in shell
   # Does it work?
   bash -c "your command"
   ```

2. **Check path**:
   ```bash
   # File paths may need to be absolute
   pwd     # Get current directory
   ```

3. **Check permissions**:
   ```bash
   # May need sudo for system commands
   # Ask Gorkbot to use appropriate command
   ```

4. **Check syntax**:
   ```bash
   # Bash syntax must be correct
   # Test in shell first
   ```

### "File not found"

**Solutions**:

1. **Check path**:
   ```bash
   ls -la /path/to/file  # Does it exist?
   ```

2. **Use absolute path**:
   ```
   > Read the file /home/user/myfile.txt
   # Instead of relative path: ./myfile.txt
   ```

3. **Check permissions**:
   ```bash
   stat /path/to/file    # Can you read it?
   ```

---

## TUI Issues

### "TUI is unresponsive"

**Solutions**:

1. **Try pressing keys**:
   ```
   Ctrl+C      # Quit
   Ctrl+L      # Refresh screen
   ```

2. **Check for modal dialog**:
   ```
   # May be waiting for input
   Esc         # Close dialog
   ```

3. **Restart**:
   ```bash
   Ctrl+C      # Exit
   ./gorkbot   # Run again
   ```

4. **Check logs**:
   ```bash
   tail -f ~/.config/gorkbot/gorkbot.json
   ```

### "Text is not displaying correctly"

**Cause**: Terminal encoding issue

**Solutions**:

1. **Set locale**:
   ```bash
   export LANG=en_US.UTF-8
   export LC_ALL=en_US.UTF-8
   ./gorkbot
   ```

2. **Update terminal**: Use modern terminal (iTerm2, Windows Terminal, etc.)

3. **Check font**: Ensure terminal uses Unicode-compatible font

---

## Database Issues

### "Database is locked"

**Cause**: Another instance is running

**Solutions**:

1. **Check running processes**:
   ```bash
   ps aux | grep gorkbot
   ```

2. **Kill other instances**:
   ```bash
   killall gorkbot
   ```

3. **Wait for lock to release**:
   ```bash
   # Database lock times out after 5 seconds
   # Just wait and try again
   ```

### "Database is corrupted"

**Cause**: Unexpected shutdown or filesystem issue

**Solutions**:

1. **Check integrity**:
   ```bash
   sqlite3 ~/.config/gorkbot/gorkbot.db ".tables"
   ```

2. **Rebuild if corrupted**:
   ```bash
   # Backup old database
   cp ~/.config/gorkbot/gorkbot.db gorkbot.db.backup
   
   # Remove corrupted database
   rm ~/.config/gorkbot/gorkbot.db
   
   # Gorkbot will create new database on next run
   ./gorkbot
   ```

---

## Configuration Issues

### "Settings not saving"

**Cause**: File permissions or format issue

**Solutions**:

1. **Check file exists**:
   ```bash
   ls -la ~/.config/gorkbot/app_state.json
   ```

2. **Check permissions**:
   ```bash
   chmod 600 ~/.config/gorkbot/app_state.json
   ```

3. **Check format**:
   ```bash
   # Verify JSON is valid
   python3 -m json.tool ~/.config/gorkbot/app_state.json
   ```

4. **Reset settings**:
   ```bash
   rm ~/.config/gorkbot/app_state.json
   ./gorkbot setup
   ```

---

## Getting Help

If you can't fix the issue:

1. **Check logs**:
   ```bash
   cat ~/.config/gorkbot/gorkbot.json | jq 'select(.level=="ERROR")'
   ```

2. **Check SENSE traces**:
   ```bash
   cat ~/.config/gorkbot/trace/$(date +%Y-%m-%d).jsonl | jq 'select(.kind=="error")'
   ```

3. **Report issue**:
   - [GitHub Issues](https://github.com/velariumai/gorkbot/issues)
   - Include: error message, steps to reproduce, logs
   - Email: velarium.ai@gmail.com

---

**Can't find solution? Open an issue on GitHub or email us!**

