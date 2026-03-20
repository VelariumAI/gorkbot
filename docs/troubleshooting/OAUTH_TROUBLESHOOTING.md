# OAuth Troubleshooting Guide

## Error: "Error 400: invalid_scope"

### What This Means

Google doesn't recognize the scope `https://www.googleapis.com/auth/generative-language` for your OAuth client. This happens because the scope needs to be explicitly added to your OAuth consent screen.

### Solution 1: Add Scope to OAuth Consent Screen ✅ RECOMMENDED

1. **Go to Google Cloud Console:**
   - https://console.cloud.google.com
   - Select your project

2. **Navigate to OAuth consent screen:**
   - Left sidebar → APIs & Services → OAuth consent screen

3. **Edit your app:**
   - Click "EDIT APP" button

4. **Add the scope:**
   - Click through to the "Scopes" step
   - Click "ADD OR REMOVE SCOPES"
   - Scroll to the bottom: "Manually add scopes"
   - Enter: `https://www.googleapis.com/auth/generative-language`
   - Click "ADD TO TABLE"
   - Click "UPDATE"
   - Click "SAVE AND CONTINUE"

5. **Test again:**
   ```bash
   ./gorkbot.sh login
   ```

### Solution 2: Use Cloud Platform Scope (Temporary Workaround)

If the specific Gemini scope doesn't work, use a broader scope:

```bash
export GOOGLE_OAUTH_SCOPE="https://www.googleapis.com/auth/cloud-platform"
./gorkbot.sh login
```

This requests full Google Cloud Platform access, which includes Gemini API.

**Add this scope to OAuth consent screen:**
1. Go to OAuth consent screen → Edit App → Scopes
2. Find "Google Cloud Platform" in the list (it's usually available)
3. Check the box
4. Save

### Solution 3: Enable the API First

Make sure the Generative Language API is enabled:

1. **Go to:** APIs & Services → Library
2. **Search:** "Generative Language API"
3. **Click** on it
4. **Click** "ENABLE"

Then add the scope to your OAuth consent screen (Solution 1).

### Solution 4: Use API Key Instead (Fallback)

If OAuth continues to have issues, use API keys:

1. **Get API key from:**
   - https://aistudio.google.com/apikey

2. **Add to .env:**
   ```bash
   GEMINI_API_KEY=your_api_key_here
   ```

3. **Run Gorkbot:**
   ```bash
   ./gorkbot.sh
   ```

## Other Common OAuth Errors

### Error 400: redirect_uri_mismatch

**Problem:** The redirect URI doesn't match what's configured in your OAuth client.

**Solution:**
1. Go to APIs & Services → Credentials
2. Click on your OAuth client
3. Add this redirect URI: `http://localhost:8085/callback`
4. Click SAVE
5. Try again

### Error 403: access_denied

**Problem:** User denied access, or app is not verified.

**Solution:**
- Make sure you click "Allow" when authorizing
- If you see "This app isn't verified", click "Advanced" → "Go to Gorkbot (unsafe)"
- Make sure you added yourself as a test user (if app is not published)

### Error: "This app isn't verified"

**Problem:** Your OAuth app hasn't been verified by Google.

**Solutions:**

**Option 1: Continue anyway (for testing):**
1. Click "Advanced"
2. Click "Go to Gorkbot (unsafe)"
3. Grant permissions

**Option 2: Add yourself as test user:**
1. OAuth consent screen → Test users
2. Add your email address
3. Try again

**Option 3: Publish app:**
1. OAuth consent screen → Click "PUBLISH APP"
2. Submit for verification (takes 3-7 days)

### Error: "Access blocked: This app's request is invalid"

**Problem:** OAuth consent screen not configured correctly.

**Solution:**
1. Check OAuth consent screen is complete:
   - App name: Set
   - User support email: Set
   - Scopes: Added
   - Test users: Added (if not published)

2. Make sure your email is added as a test user

3. Verify the API is enabled

### Browser doesn't open

**Problem:** `xdg-open`, `open`, or `termux-open-url` not available.

**Solution:**

**On Linux:**
```bash
sudo apt install xdg-utils
```

**On Termux:**
```bash
pkg install termux-tools
```

**Manual option:**
Copy the URL from the terminal and paste into your browser.

### Port already in use

**Problem:** Port 8085 is already in use.

**Solution:**
```bash
./gorkbot.sh login --port 3000
```

Then add `http://localhost:3000/callback` to your OAuth client redirect URIs.

### Token expired errors

**Problem:** OAuth token has expired.

**Solution:**

Tokens auto-refresh, but if you get errors:
```bash
./gorkbot.sh logout
./gorkbot.sh login
```

## Testing Your OAuth Setup

### Quick Test

```bash
# Check configuration
./test-oauth.sh

# Try login
./gorkbot.sh login

# Check status
./gorkbot.sh status

# Test API access
./gorkbot.sh -p "Hello"
```

### Debug Mode

Enable verbose OAuth debugging:

```bash
# Add to your shell
export GOOGLE_OAUTH_DEBUG=true
./gorkbot.sh login
```

This will show:
- OAuth URL being generated
- Scopes being requested
- Redirect URI
- Any errors

## Scope Reference

| Scope | Description | Use Case |
|-------|-------------|----------|
| `generative-language` | Gemini API access | ✅ Recommended |
| `cloud-platform` | Full GCP access | Broader access |
| `generative-ai` | AI/ML APIs | May not exist |

## Valid OAuth Client Types

| Type | Client Secret | Use Case |
|------|--------------|----------|
| **Desktop app** | Optional (PKCE) | ✅ CLI tools like Gorkbot |
| Web application | Required | Web apps |
| Mobile app | Not used (PKCE) | Mobile apps |
| Service account | JSON key | Backend services |

## Environment Variables

Override OAuth behavior:

```bash
# Custom client ID
export GOOGLE_CLIENT_ID="your-client.apps.googleusercontent.com"

# Custom client secret (optional with PKCE)
export GOOGLE_CLIENT_SECRET="your-secret"

# Override scope
export GOOGLE_OAUTH_SCOPE="https://www.googleapis.com/auth/cloud-platform"

# Custom port
./gorkbot.sh login --port 3000

# Timeout
./gorkbot.sh login --timeout 10m
```

## Still Having Issues?

### Check these:

1. ✅ **API Enabled?**
   - Generative Language API is enabled in Google Cloud

2. ✅ **Scope Added?**
   - Scope is in OAuth consent screen

3. ✅ **Redirect URI?**
   - `http://localhost:8085/callback` is in OAuth client

4. ✅ **Test User?**
   - Your email is added as test user (if not published)

5. ✅ **OAuth Client Type?**
   - Set to "Desktop app"

### Get Help

If you're still stuck:

1. **Check the full error message:**
   ```bash
   ./gorkbot.sh login 2>&1 | tee oauth-error.log
   ```

2. **Share the error log** (remove any secrets!)

3. **Try API key authentication:**
   ```bash
   export GEMINI_API_KEY="your-api-key"
   ./gorkbot.sh
   ```

## Success Checklist

When OAuth is working correctly, you should see:

```bash
$ ./gorkbot.sh login
Using Gorkbot's public OAuth client

🔐 Signing in to Google...
Opening browser for authorization...
👤 Sign in with your Google account
✅ Grant access to Gorkbot

✓ Authorization successful!
✓ Token saved to ~/.config/gorkbot/gemini_token.json

$ ./gorkbot.sh status
Authentication Status:
─────────────────────
  Method: Google OAuth
  Status: ✓ Valid
  Expires: 2026-02-15 10:30:00
```

Then you're all set! 🎉
