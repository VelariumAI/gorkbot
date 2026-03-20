# OAuth Client Setup for Gorkbot Maintainers

## 📝 For Project Maintainers Only

This guide is for setting up the **official Gorkbot public OAuth client** that all users will use.

## 🎯 Goal

Create a public OAuth client that:
- Allows users to sign in with "Sign in with Google"
- Uses PKCE (no client secret needed)
- Doesn't require users to create their own OAuth app
- Is embedded in the Gorkbot codebase

## 🔧 Setup Steps

### 1. Create Google Cloud Project

1. Go to https://console.cloud.google.com
2. Create a new project: **"Gorkbot Official"**
3. Note the project ID

### 2. Enable API

1. Go to **APIs & Services** → **Library**
2. Search for **"Generative Language API"**
3. Click **ENABLE**

### 3. Configure OAuth Consent Screen

1. Go to **APIs & Services** → **OAuth consent screen**

2. **User Type:**
   - Select **External**
   - Click **CREATE**

3. **App Information:**
   - App name: `Gorkbot`
   - User support email: Your email
   - App logo: Upload Gorkbot logo (optional)
   - Application home page: `https://github.com/taeddings/gorkbot`
   - Privacy policy: Link to privacy policy (required for verification)
   - Terms of service: Link to terms (optional)

4. **Scopes:**
   - Click **ADD OR REMOVE SCOPES**
   - Manually add scope: `https://www.googleapis.com/auth/generative-language`
   - Click **UPDATE**
   - Click **SAVE AND CONTINUE**

5. **Test Users** (while in testing mode):
   - Add your email
   - Add any beta testers
   - Click **SAVE AND CONTINUE**

6. **Publish App** (when ready):
   - Click **PUBLISH APP**
   - Submit for verification (recommended)

### 4. Create OAuth Client ID

1. Go to **APIs & Services** → **Credentials**
2. Click **+ CREATE CREDENTIALS** → **OAuth client ID**

3. **Application type:**
   - Select **Desktop app**
   - Name: `Gorkbot CLI`

4. **Configure Redirect URIs:**

   Add all possible localhost ports users might use:
   ```
   http://localhost:8085/callback
   http://localhost:8080/callback
   http://localhost:3000/callback
   http://localhost:9090/callback
   ```

5. Click **CREATE**

6. **Copy the Client ID** - it will look like:
   ```
   123456789-abc123def456.apps.googleusercontent.com
   ```

7. **Download JSON** (optional) - keep this secure

### 5. Configure for PKCE

**Important:** For public clients using PKCE:

1. Click on your OAuth client in the credentials list
2. Under **Client type**, ensure it's set to **Public**
3. Leave **Client secret** field - we won't use it (PKCE doesn't need it)

Note: The client secret is shown but won't be embedded in the code. PKCE provides security without it.

### 6. Update Gorkbot Code

Edit `pkg/auth/oauth.go`:

```go
const (
    // Google OAuth scopes for Generative AI API
    GeminiScope = "https://www.googleapis.com/auth/generative-language"

    // Public OAuth Client ID for Gorkbot
    // Replace this with your actual client ID
    DefaultClientID = "YOUR_ACTUAL_CLIENT_ID.apps.googleusercontent.com"
)
```

Replace `YOUR_ACTUAL_CLIENT_ID` with the Client ID from step 4.

### 7. Test the Flow

```bash
# Build Gorkbot
make build

# Test login (should use the new default client ID)
./gorkbot.sh login

# Verify it works
./gorkbot.sh status
```

### 8. Commit and Push

```bash
git add pkg/auth/oauth.go
git commit -m "Configure official OAuth client ID"
git push
```

## 🔒 Security Considerations

### Is it safe to embed the Client ID?

**Yes!** The OAuth Client ID is **public** by design:
- ✅ It's shown in the browser URL during auth
- ✅ It's meant to identify your app
- ✅ It's visible in network traffic
- ✅ OAuth specs consider it non-sensitive

### What about the Client Secret?

**Don't embed it!** With PKCE, we don't use the client secret:
- ❌ Client secrets are for confidential clients (backend apps)
- ✅ PKCE uses code verifier/challenge instead
- ✅ More secure for public clients like CLI tools

### PKCE Security

PKCE provides security through:
1. **Code Verifier**: Random string generated on user's machine
2. **Code Challenge**: SHA256 hash sent to Google
3. **Verification**: Google ensures verifier matches challenge
4. **CSRF Protection**: State parameter prevents attacks

## 📊 OAuth Client Settings Summary

| Setting | Value |
|---------|-------|
| **Application type** | Desktop app |
| **Client type** | Public |
| **Redirect URIs** | `http://localhost:*/callback` |
| **Scopes** | `https://www.googleapis.com/auth/generative-language` |
| **PKCE** | Enabled (automatic for public clients) |
| **Client Secret** | Not used (PKCE flow) |

## 🚀 Publishing the App

### Testing Phase

While in testing mode:
- Only test users can authorize
- "Unverified app" warning appears
- Good for beta testing

### Verification

To remove the "unverified" warning:

1. **Submit for Verification:**
   - Go to OAuth consent screen
   - Click **PUBLISH APP**
   - Click **Prepare for verification**

2. **Required:**
   - Privacy policy URL
   - Terms of service URL (optional but recommended)
   - App homepage (GitHub repo)
   - YouTube demo video (optional)

3. **Verification takes 3-7 days**

4. **After verification:**
   - No "unverified" warning
   - Any Google user can authorize
   - Increased trust

### Production Checklist

- [ ] OAuth consent screen configured
- [ ] Privacy policy published
- [ ] Terms of service published (optional)
- [ ] Redirect URIs configured
- [ ] Client ID embedded in code
- [ ] Tested on all platforms (Linux, macOS, Windows, Termux)
- [ ] App submitted for verification
- [ ] Documentation updated

## 🔧 Maintenance

### Rotating Client ID

If you need to change the client ID:

1. Create new OAuth client in Google Cloud Console
2. Update `DefaultClientID` in `pkg/auth/oauth.go`
3. Release new version
4. Old tokens will still work (different client, same scope)

### Monitoring Usage

Check OAuth analytics:
1. Go to Google Cloud Console
2. **APIs & Services** → **Credentials**
3. Click on your OAuth client
4. View usage statistics

## 🐛 Common Issues

### "Redirect URI mismatch"

Add the redirect URI to your OAuth client:
- `http://localhost:PORT/callback`

### "Access blocked: This app's request is invalid"

- Make sure the scope is correctly configured
- Verify app is published (or user is in test users list)

### "This app isn't verified"

- Submit app for verification, or
- Users can click "Advanced" → "Go to Gorkbot (unsafe)"

## 📚 References

- [Google OAuth 2.0 for Mobile & Desktop Apps](https://developers.google.com/identity/protocols/oauth2/native-app)
- [PKCE RFC 7636](https://tools.ietf.org/html/rfc7636)
- [OAuth 2.0 for Native Apps - Best Practices](https://tools.ietf.org/html/rfc8252)

---

**After setup, users just run:** `./gorkbot.sh login` and sign in with their Google account!
