# Sign in with Google - Quick Guide

## 🚀 Super Simple Setup (No Configuration Needed!)

Just run:

```bash
./gorkbot.sh login
```

That's it! Your browser will open and you'll sign in with your regular Google account.

## 🎯 How It Works

### Step 1: Run Login Command

```bash
./gorkbot.sh login
```

You'll see:

```
Using Gorkbot's public OAuth client
You'll be asked to sign in with your Google account

🔐 Signing in to Google...

Opening browser for authorization...
👤 Sign in with your Google account
✅ Grant access to Gorkbot
```

### Step 2: Sign In (Browser Opens Automatically)

Your browser opens to Google's sign-in page:

1. **Sign in** with your Google account (the one with Gemini access)
2. **Review permissions** - Gorkbot is requesting access to Generative Language API
3. **Click "Continue"** or **"Allow"**

### Step 3: Success!

Browser shows:
```
✅ Authorization Successful!
You can now close this window and return to the terminal.
```

Terminal shows:
```
✓ Authorization successful!
✓ Token saved to ~/.config/gorkbot/gemini_token.json

You can now use Gorkbot with your Google account.
```

### Step 4: Start Using Gorkbot!

```bash
./gorkbot.sh
```

The TUI launches and automatically uses your Google account for Gemini API access!

## 🔐 What's Happening Under the Hood?

### PKCE OAuth Flow

Gorkbot uses **PKCE (Proof Key for Code Exchange)** - a secure OAuth flow designed for public clients:

1. **No Client Secret** - Unlike traditional OAuth, PKCE doesn't require a secret embedded in the app
2. **Code Verifier** - Generates a random string on your machine
3. **Code Challenge** - Creates a SHA256 hash of the verifier
4. **Secure Exchange** - Google verifies the original verifier matches the challenge

```
Your Computer                  Google
     │                            │
     │  1. Generate verifier      │
     │  2. Create challenge       │
     │                            │
     ├──── Auth URL + challenge ──>
     │                            │
     │  <──── You sign in ────────┤
     │                            │
     ├──── Code + verifier ───────>
     │                            │
     │  3. Google verifies        │
     │     challenge matches      │
     │                            │
     │  <──── Access token ───────┤
     │                            │
     │  4. Token saved locally    │
     │                            │
```

### Security Benefits

- ✅ **No secrets in code** - PKCE doesn't require client secrets
- ✅ **CSRF protection** - Random state parameter prevents attacks
- ✅ **Localhost only** - Callback server binds to 127.0.0.1
- ✅ **Secure storage** - Token saved with 0600 permissions
- ✅ **Auto-refresh** - Tokens refresh automatically when expired

## 🎮 Commands

```bash
# Sign in with Google
./gorkbot.sh login

# Check authentication status
./gorkbot.sh status

# Sign out (remove token)
./gorkbot.sh logout

# Use Gorkbot (uses Google account automatically)
./gorkbot.sh
```

## 📊 Authentication Status

After signing in, check your status:

```bash
./gorkbot.sh status
```

Output:
```
Authentication Status:
─────────────────────
  Method: Google OAuth
  Status: ✓ Valid
  Expires: 2026-02-15 12:30:00
  Fallback: API Key (not configured)
```

## 🔄 Token Management

### Auto-Refresh

Tokens expire after 1 hour but **automatically refresh** when needed. You don't need to do anything!

### Manual Refresh

If you want to force a new login:

```bash
./gorkbot.sh logout
./gorkbot.sh login
```

### Token Location

Your OAuth token is stored at:
- **Linux/Termux**: `~/.config/gorkbot/gemini_token.json`
- **macOS**: `~/Library/Application Support/gorkbot/gemini_token.json`
- **Windows**: `%APPDATA%/gorkbot/gemini_token.json`

File permissions: `0600` (owner read/write only)

## ❓ FAQ

### Do I need a Gemini Pro plan?

For full API access, yes. Gemini API access requires:
- Gemini Advanced (paid plan), or
- Access to Google AI Studio with your account

Free tier users have limited quota.

### Can I use multiple Google accounts?

Yes! Just logout and login with a different account:

```bash
./gorkbot.sh logout
./gorkbot.sh login  # Sign in with different account
```

### Does this share my Google data?

No! Gorkbot only requests access to the **Generative Language API** scope:
- `https://www.googleapis.com/auth/generative-language`

This allows Gorkbot to:
- ✅ Use Gemini API on your behalf
- ✅ Use your account's quota

This does NOT give access to:
- ❌ Gmail
- ❌ Google Drive
- ❌ Calendar
- ❌ Any other Google services

### What if I don't trust Gorkbot's OAuth client?

You can create your own! See `OAUTH_SETUP.md` for instructions on using your own OAuth credentials.

Set in `.env`:
```bash
GOOGLE_CLIENT_ID=your-custom-client.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-secret  # Optional with PKCE
```

### Can I revoke access?

Yes! Go to:
- https://myaccount.google.com/permissions
- Find "Gorkbot" in the list
- Click "Remove Access"

Then locally:
```bash
./gorkbot.sh logout
```

## 🐛 Troubleshooting

### Browser doesn't open?

Copy the URL from the terminal and paste it into your browser manually.

### "Using Gorkbot's public OAuth client" but still asks for credentials?

The default client ID needs to be configured. For now, you may need to use your own OAuth credentials. See `OAUTH_SETUP.md`.

### Port already in use?

```bash
./gorkbot.sh login --port 3000
```

### "This app isn't verified"

If you see this warning:
1. Click "Advanced"
2. Click "Go to Gorkbot (unsafe)"

This happens because Gorkbot is in development/testing mode. Once published, this warning won't appear.

### "Access blocked: This app's request is invalid"

The OAuth client may not be configured correctly. Try:
1. Make sure you're using your Google account (not a workspace account with restrictions)
2. Check that you have Gemini API access
3. Try creating your own OAuth credentials (see `OAUTH_SETUP.md`)

## 🎉 That's It!

Three commands to remember:

```bash
./gorkbot.sh login   # Sign in with Google
./gorkbot.sh status  # Check status
./gorkbot.sh         # Start using Gorkbot!
```

No configuration files, no API keys to manage - just sign in and go! 🚀

---

**Note:** The default public OAuth client ID will be configured when Gorkbot is officially published. For now, you may need to use your own OAuth credentials. See `OAUTH_SETUP.md` for details.
