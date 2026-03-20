# Fix: Error 403 access_denied

## 🔍 What This Error Means

The OAuth request reached Google successfully, but access was **denied**. This is different from invalid_scope - your scopes are valid, but you're not authorized to use them.

## 🎯 Most Common Causes

### 1. App is in Testing Mode + You're Not a Test User ⭐ MOST LIKELY

**Check if this is the issue:**
1. Go to [Google Cloud Console](https://console.cloud.google.com)
2. Navigate to: **APIs & Services** → **OAuth consent screen**
3. Look at "Publishing status"

**If it says "Testing":**

You MUST add your email as a test user:

1. On the OAuth consent screen page
2. Scroll down to **"Test users"**
3. Click **"+ ADD USERS"**
4. Enter your Google account email (the one you're trying to sign in with)
5. Click **"SAVE"**
6. Try logging in again: `./gorkbot.sh login`

### 2. Cloud Platform Scope Requires Additional Setup

The `cloud-platform` scope gives broad access. Google may require:

**Option A: Add yourself as test user** (see above)

**Option B: Use more specific scopes instead:**

Let's use Generative AI specific scopes:

```bash
export GOOGLE_OAUTH_SCOPE="https://www.googleapis.com/auth/generative-language"
./gorkbot.sh login
```

But first, make sure this scope is added to your OAuth consent screen:
1. OAuth consent screen → EDIT APP
2. Scopes → ADD OR REMOVE SCOPES
3. Manually add scopes: `https://www.googleapis.com/auth/generative-language`
4. UPDATE → SAVE AND CONTINUE

### 3. You Clicked "Deny" Instead of "Allow"

If you accidentally clicked "Deny":

1. Try logging in again: `./gorkbot.sh login`
2. When the browser opens, click **"Allow"** this time
3. Grant all requested permissions

### 4. Organization/Workspace Restrictions

If using a Google Workspace account (company/school email):

- Your organization may block OAuth apps
- Try with a **personal Gmail account** instead
- Or ask your IT admin to allowlist the app

### 5. App Not Published

If your app is in Testing mode and you haven't added yourself as a test user, you'll get access denied.

**Quick fix: Publish your app**

1. OAuth consent screen → **"PUBLISH APP"**
2. Click **"CONFIRM"**

⚠️ Note: If you publish, you may see "This app isn't verified" - users can still continue with "Advanced" → "Go to app (unsafe)"

## 🔧 Step-by-Step Fix

### Solution 1: Add Yourself as Test User (Recommended)

```bash
# 1. Go to Google Cloud Console
open https://console.cloud.google.com

# 2. Navigate to: APIs & Services → OAuth consent screen

# 3. Scroll to "Test users" section

# 4. Click "+ ADD USERS"

# 5. Enter your Gmail address (the one you're signing in with)

# 6. Click "SAVE"

# 7. Try again
./gorkbot.sh login
```

### Solution 2: Reduce Scope Requirements

Instead of `cloud-platform`, use minimal scopes:

Edit `pkg/auth/oauth.go` to use minimal scopes:
```go
// Minimal scopes for Gemini API
defaultScopes := []string{
    "https://www.googleapis.com/auth/generative-language",
}
```

Then rebuild and try:
```bash
make build
./gorkbot.sh login
```

### Solution 3: Publish the App

1. OAuth consent screen → **"PUBLISH APP"**
2. Click **"CONFIRM"**
3. Try login again

Users will see "This app isn't verified" but can continue with:
- Click **"Advanced"**
- Click **"Go to Gorkbot (unsafe)"**
- Click **"Allow"**

## 🐛 Debugging Steps

### Check 1: Verify OAuth Consent Screen Setup

```
✓ App name: Set
✓ User support email: Set
✓ Developer contact: Set
✓ Scopes: Added (at least one scope)
✓ Test users: YOUR EMAIL ADDED ⭐
```

### Check 2: Verify Your Email

Make sure you're using the **exact same email** for:
- Test user in OAuth consent screen
- Sign-in in the browser

### Check 3: Check Publishing Status

OAuth consent screen should show:
- **Testing** (requires test users) ← Add yourself!
- **In production** (anyone can use) ← No restrictions

### Check 4: Try Different Account

```bash
# Try with a different Google account
./gorkbot.sh login

# When browser opens, use a different Gmail account
```

## 🎯 Quick Checklist

Run through this checklist:

- [ ] OAuth consent screen is configured
- [ ] At least one scope is added
- [ ] **Your email is added as a test user** ⭐
- [ ] You clicked "Allow" (not "Deny")
- [ ] Using personal Gmail (not Workspace if restricted)
- [ ] API is enabled (Generative Language API)
- [ ] Tried with a fresh browser/incognito window

## 🔄 If All Else Fails

### Nuclear Option: Start Fresh

1. **Delete OAuth client:**
   - Credentials → Delete your OAuth client

2. **Create new OAuth client:**
   - CREATE CREDENTIALS → OAuth client ID
   - Type: Desktop app
   - Name: Gorkbot v2

3. **Configure consent screen:**
   - Add yourself as test user
   - Add minimal scopes

4. **Update .env:**
   ```bash
   GOOGLE_CLIENT_ID=new_client_id_here
   GOOGLE_CLIENT_SECRET=new_client_secret_here
   ```

5. **Try again:**
   ```bash
   ./gorkbot.sh login
   ```

## ✅ Expected Success Flow

When everything is working, you should see:

```
1. Browser opens to Google sign-in
2. You sign in with your Google account
3. Page shows: "Gorkbot wants to access your Google Account"
4. You click "Allow"
5. Browser shows: "Authorization Successful!"
6. Terminal shows: "✓ Token saved"
```

## 📞 Still Having Issues?

Check these:

1. **Browser console** - Open DevTools (F12) and check for errors

2. **Incognito mode** - Try in incognito/private browsing

3. **Different browser** - Try Chrome, Firefox, or Safari

4. **Network logs** - Check if corporate firewall is blocking

5. **Google account status** - Make sure your Google account is active

## 🎯 Most Likely Solution

**90% of the time, this is the fix:**

1. Go to OAuth consent screen
2. Add your email as a **Test user**
3. Try `./gorkbot.sh login` again
4. Click **"Allow"** when prompted

That should do it! 🎉
