# Security Best Practices for Grokster

## 🔒 Handling OAuth Secrets

### ✅ What's Safe to Share

**OAuth Client ID** - This is PUBLIC:
```
200519342786-tcu9hra6g91o73n7r1fcr3nqiqi0lc6d.apps.googleusercontent.com
```

✅ Safe to:
- Embed in source code
- Share in documentation
- Commit to GitHub
- Show in browser URLs

Why? The Client ID just **identifies** your app - it doesn't grant access.

### ❌ What Must Stay Private

**OAuth Client Secret** - This is PRIVATE:
```
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  ⚠️ NEVER share this!
```

❌ **NEVER:**
- Embed in source code
- Commit to git (even private repos)
- Share in chat/email
- Include in screenshots
- Hardcode in applications

✅ **ONLY store in:**
- `.env` file (gitignored)
- Environment variables
- Secret managers (AWS Secrets Manager, etc.)
- Secure credential stores

## 🚨 What Happens if Secret is Exposed?

If someone gets your client secret, they can:

1. **Impersonate your app** - Make API calls pretending to be your application
2. **Use your quota** - Consume your API quota/billing
3. **Access user data** - If users authorized your app
4. **Bypass restrictions** - Circumvent rate limits you set
5. **Damage reputation** - Make malicious requests under your app's name

## 🔧 Immediate Response if Secret is Compromised

### Step 1: Regenerate Secret (NOW!)

1. **Go to** [Google Cloud Console](https://console.cloud.google.com)
2. **Navigate to:** APIs & Services → Credentials
3. **Find your OAuth client** in the list
4. **Click the name** to edit
5. **Click "RESET SECRET"** or create new credentials
6. **Copy the new secret**
7. **Update your `.env` file** with the new secret
8. **Delete old credentials** (if you created new ones)

### Step 2: Update Your Configuration

```bash
# Edit .env
nano .env

# Update the secret
GOOGLE_CLIENT_SECRET=NEW_SECRET_HERE

# Test it works
./grokster.sh login
```

### Step 3: If Already Committed to Git

If you **already committed** the secret to git:

```bash
# ⚠️ This rewrites git history - use with caution!

# Remove secret from ALL commits
git filter-branch --force --index-filter \
  "git rm --cached --ignore-unmatch pkg/auth/oauth.go" \
  --prune-empty --tag-name-filter cat -- --all

# Force push (if already pushed to GitHub)
git push origin --force --all
```

**Better approach:** Start a new repo:
```bash
# Backup current code
cp -r grokster grokster-backup

# Create fresh repo
cd grokster
rm -rf .git
git init
git add .
git commit -m "Initial commit (with secrets removed)"
```

### Step 4: Verify Protection

```bash
# Check .gitignore includes .env
cat .gitignore | grep ".env"

# Should output:
# .env
# .env.local
# .env.*.local

# Verify .env is not tracked
git status --ignored | grep ".env"

# Should show .env as ignored
```

## 📋 Current Configuration (Safe)

Your current setup is now **secure**:

### Code (Public - Safe to commit):
```go
// pkg/auth/oauth.go
const (
    DefaultClientID = "200519342786-...apps.googleusercontent.com"  // ✅ Public
    DefaultClientSecret = ""  // ✅ Empty (uses env var)
)
```

### Environment Variables (Private - NOT committed):
```bash
# .env (in .gitignore)
GOOGLE_CLIENT_ID=200519342786-...apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=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  # ⚠️ Private!
```

## 🛡️ Defense in Depth

### Layer 1: .gitignore

Prevents accidental commits:
```
.env
.env.local
*.secret
credentials.json
```

### Layer 2: Pre-commit Hook

Add to `.git/hooks/pre-commit`:
```bash
#!/bin/bash
if git diff --cached | grep -i "GOCSPX-\|client.*secret\|api.*key"; then
    echo "⚠️  ERROR: Detected potential secret in commit!"
    echo "Please remove secrets before committing."
    exit 1
fi
```

Make executable:
```bash
chmod +x .git/hooks/pre-commit
```

### Layer 3: Environment Variables Only

Never hardcode secrets:

❌ **Bad:**
```go
clientSecret := "GOCSPX-abc123"
```

✅ **Good:**
```go
clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
if clientSecret == "" {
    return fmt.Errorf("GOOGLE_CLIENT_SECRET not set")
}
```

### Layer 4: Secret Scanning

Use tools like:
- **git-secrets** - AWS tool to prevent committing secrets
- **gitleaks** - Scan repos for secrets
- **TruffleHog** - Find secrets in git history
- **GitHub secret scanning** - Automatic on public repos

Install git-secrets:
```bash
# macOS
brew install git-secrets

# Linux
git clone https://github.com/awslabs/git-secrets
cd git-secrets
make install

# Configure
cd /path/to/grokster
git secrets --install
git secrets --register-aws
git secrets --add 'GOCSPX-[0-9A-Za-z_-]+'
```

## 📊 OAuth Security Checklist

Before committing code:

- [ ] ✅ Client ID in code? (OK - it's public)
- [ ] ❌ Client Secret in code? (NEVER!)
- [ ] ✅ `.env` in `.gitignore`?
- [ ] ✅ Secrets only in environment variables?
- [ ] ✅ No secrets in comments/docs?
- [ ] ✅ No secrets in error messages?
- [ ] ✅ No secrets in logs?

## 🔍 How to Check if You're Safe

### Check 1: Search Code for Secrets

```bash
# Search for potential secrets
grep -r "GOCSPX-" . --exclude-dir=.env
grep -r "client.*secret.*=" . --exclude="*.md"

# Should return NOTHING (or only .env which is gitignored)
```

### Check 2: Verify .env is Ignored

```bash
git status --ignored | grep -E "\.env$"

# Should show:
# .env
```

### Check 3: Check Git History

```bash
# Search git history for secrets
git log -S "GOCSPX-" --all

# Should return NOTHING (or only this commit removing it)
```

## 📚 Additional Resources

- [OWASP API Security](https://owasp.org/www-project-api-security/)
- [Google OAuth Best Practices](https://developers.google.com/identity/protocols/oauth2/best-practices)
- [GitHub Secret Scanning](https://docs.github.com/en/code-security/secret-scanning)
- [12-Factor App: Config](https://12factor.net/config)

## ✅ Current Status

Your Grokster installation is now **secure**:

1. ✅ Client Secret removed from code
2. ✅ Secret stored in `.env` (gitignored)
3. ✅ `.gitignore` protecting secrets
4. ✅ Code uses environment variables
5. ✅ Not committed to git yet

**Keep it this way!** 🔒

## 🎯 Remember

> **Golden Rule:** If you wouldn't publish it on Twitter, don't put it in code!

Secrets belong in:
- ✅ Environment variables
- ✅ `.env` files (gitignored)
- ✅ Secret managers
- ✅ Encrypted storage

Secrets DO NOT belong in:
- ❌ Source code
- ❌ Git repositories
- ❌ Documentation
- ❌ Screenshots
- ❌ Error messages
- ❌ Log files
