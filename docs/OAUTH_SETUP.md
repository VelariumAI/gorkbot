# Google OAuth Setup Guide

This guide walks you through setting up Google OAuth for Grokster so you can authenticate with your Google account instead of using API keys.

## 🎯 Overview

**What the OAuth flow does:**
1. Opens your browser to Google's sign-in page
2. You sign in with your Google account
3. Google redirects back to the app with an authorization code
4. App exchanges code for an access token
5. Token is saved locally for future use

## 📋 Prerequisites

- A Google account
- Access to [Google Cloud Console](https://console.cloud.google.com)
- Grokster installed and built

## 🔧 Step 1: Create OAuth Credentials

### 1.1 Go to Google Cloud Console

Visit: https://console.cloud.google.com

### 1.2 Create or Select a Project

- Click the project dropdown at the top
- Click **"NEW PROJECT"**
- Name it something like "Grokster" or "Personal AI CLI"
- Click **"CREATE"**

### 1.3 Enable the Generative Language API

1. In the left sidebar, go to **"APIs & Services"** → **"Library"**
2. Search for **"Generative Language API"**
3. Click on it
4. Click **"ENABLE"**

### 1.4 Create OAuth 2.0 Credentials

1. Go to **"APIs & Services"** → **"Credentials"**
2. Click **"+ CREATE CREDENTIALS"** at the top
3. Select **"OAuth client ID"**

4. **Configure OAuth Consent Screen** (if prompted):
   - User Type: **External**
   - Click **"CREATE"**

   Fill in the required fields:
   - App name: **Grokster**
   - User support email: *your email*
   - Developer contact: *your email*
   - Click **"SAVE AND CONTINUE"**

   Scopes:
   - Click **"ADD OR REMOVE SCOPES"**
   - Manually add: `https://www.googleapis.com/auth/generative-language`
   - Click **"UPDATE"**
   - Click **"SAVE AND CONTINUE"**

   Test users:
   - Click **"+ ADD USERS"**
   - Add your email address
   - Click **"SAVE AND CONTINUE"**

5. **Create the OAuth Client:**
   - Application type: **Desktop app**
   - Name: **Grokster CLI**
   - Click **"CREATE"**

6. **Download Credentials:**
   - A dialog will show your Client ID and Client Secret
   - **Copy both** - you'll need them in the next step
   - You can also download the JSON file

### 1.5 Configure Redirect URI

The OAuth flow uses `http://localhost:8085/callback` by default.

**Important:** Make sure to add this to your OAuth client:
1. Click on your OAuth client in the credentials list
2. Under "Authorized redirect URIs", click **"+ ADD URI"**
3. Add: `http://localhost:8085/callback`
4. Click **"SAVE"**

## 🔐 Step 2: Configure Grokster

### 2.1 Add Credentials to .env

Edit your `.env` file:

```bash
nano .env
# or
vi .env
```

Add your OAuth credentials:

```bash
# Google OAuth Credentials
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
```

**Example:**
```bash
GOOGLE_CLIENT_ID=123456789-abcdefghijklmnop.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-AbCdEfGhIjKlMnOpQrStUvWx
```

### 2.2 Verify .env is Loaded

The wrapper script `grokster.sh` automatically loads `.env`, so just use:

```bash
./grokster.sh login
```

Or, if running directly:

```bash
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
./bin/grokster login
```

## 🚀 Step 3: Run OAuth Login

### 3.1 Start the Login Flow

```bash
./grokster.sh login
```

You should see:

```
Starting Google OAuth flow...
Opening browser for authorization...
If the browser doesn't open, visit this URL:
https://accounts.google.com/o/oauth2/v2/auth?client_id=...

Started callback server on http://localhost:8085
Waiting for authorization...
```

### 3.2 Authenticate in Browser

Your browser should automatically open to Google's sign-in page:

1. **Sign in** to your Google account
2. **Review permissions** - Grokster is requesting access to Generative Language API
3. Click **"Allow"** or **"Continue"**

### 3.3 Success!

After authorization, you'll see in the browser:

```
✅ Authorization Successful!
You can now close this window and return to the terminal.
```

And in your terminal:

```
✓ Authorization successful!
✓ Token saved to ~/.config/grokster/gemini_token.json

You can now use Grokster with your Google account.
```

## 🔍 Step 4: Verify Authentication

Check your authentication status:

```bash
./grokster.sh status
```

You should see:

```
Authentication Status:
─────────────────────
  Method: Google OAuth
  Status: ✓ Valid
  Expires: 2026-02-15 10:30:00
  Fallback: API Key (configured)
```

## 🎮 Step 5: Test It!

Now run Grokster and it will use OAuth authentication:

```bash
# Interactive TUI
./grokster.sh

# One-shot prompt
./grokster.sh -p "Hello, Grokster!"
```

The app will automatically use your OAuth token instead of the API key.

## 🔧 Advanced Options

### Custom Port

If port 8085 is already in use, specify a different port:

```bash
./grokster.sh login --port 3000
```

**Important:** You must also add this redirect URI to your Google OAuth client:
- `http://localhost:3000/callback`

### Timeout

Increase timeout for slow connections:

```bash
./grokster.sh login --timeout 10m
```

### Refresh Token

OAuth tokens expire after 1 hour, but they auto-refresh automatically. If you need to manually refresh:

```bash
# Check if token is expired
./grokster.sh status

# The app will auto-refresh on next use
# Or force a new login:
./grokster.sh logout
./grokster.sh login
```

## 🐛 Troubleshooting

### Browser Doesn't Open

If the browser doesn't open automatically, copy the URL from the terminal and paste it into your browser manually.

**On Termux (Android):**
```bash
# Make sure termux-open-url is available
pkg install termux-tools
```

### "Credentials not configured" Error

Make sure your `.env` file contains:
```bash
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
```

And that you're using `./grokster.sh` (which loads `.env`) or have exported the variables.

### "Failed to start listener" Error

Port 8085 is already in use. Try a different port:
```bash
./grokster.sh login --port 9090
```

### "Invalid redirect URI" Error

You need to add the redirect URI to your OAuth client in Google Cloud Console:
1. Go to Credentials → Your OAuth Client
2. Add `http://localhost:8085/callback` to Authorized redirect URIs
3. Click SAVE

### "Access blocked: This app's request is invalid"

Your OAuth consent screen is not configured correctly:
1. Make sure you added yourself as a test user
2. Verify the scope `https://www.googleapis.com/auth/generative-language` is added
3. Make sure the app is in "Testing" mode (external users can be added)

### Token Expired

Tokens expire after 1 hour but should auto-refresh. If you get auth errors:

```bash
./grokster.sh logout
./grokster.sh login
```

## 🔒 Security Notes

### Token Storage

Tokens are stored at:
- **Linux/Mac/Termux**: `~/.config/grokster/gemini_token.json`
- **Windows**: `%APPDATA%/grokster/gemini_token.json`

File permissions are set to `0600` (owner read/write only).

### Client Secret

**Never commit your client secret to git!**

The `.gitignore` already excludes:
- `.env`
- `*_token.json`

### Revoking Access

To revoke Grokster's access to your Google account:
1. Go to https://myaccount.google.com/permissions
2. Find "Grokster" in the list
3. Click "Remove Access"

Then on your local machine:
```bash
./grokster.sh logout
```

## 📊 OAuth vs API Key

| Feature | OAuth | API Key |
|---------|-------|---------|
| Setup | One-time browser login | Copy/paste key |
| Expiration | Auto-refreshes (1 hour) | Never expires |
| Revocable | Yes (from Google account) | Manual deletion |
| Quota | Personal account quota | Project quota |
| Best for | Personal use | CI/CD, scripts |

## 🎯 Next Steps

1. ✅ OAuth is configured
2. ✅ Token is saved
3. Run the TUI: `./grokster.sh`
4. Try a complex query to see Gemini consultation
5. Enjoy your authenticated Grokster experience!

## 📚 Additional Resources

- [Google OAuth 2.0 Documentation](https://developers.google.com/identity/protocols/oauth2)
- [Generative Language API](https://ai.google.dev/)
- [OAuth Consent Screen Setup](https://support.google.com/cloud/answer/10311615)

---

**Need help?** Open an issue at https://github.com/taeddings/grokster/issues
