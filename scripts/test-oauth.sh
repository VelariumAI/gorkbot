#!/bin/bash
# Quick OAuth setup verification script

set -e

echo "🔍 Grokster OAuth Setup Verification"
echo "===================================="
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "   Please create a .env file with OAuth credentials"
    exit 1
fi

# Source .env
set -a
source <(grep -v '^#' .env | grep -v '^$')
set +a

# Check OAuth credentials
echo "📋 Checking OAuth configuration..."
echo ""

if [ -z "$GOOGLE_CLIENT_ID" ] || [ "$GOOGLE_CLIENT_ID" == "your_client_id_here.apps.googleusercontent.com" ]; then
    echo "❌ GOOGLE_CLIENT_ID is not configured"
    echo "   Please set it in your .env file"
    echo ""
    echo "   Get credentials from: https://console.cloud.google.com"
    echo "   See OAUTH_SETUP.md for detailed instructions"
    exit 1
else
    echo "✅ GOOGLE_CLIENT_ID: ${GOOGLE_CLIENT_ID:0:20}..."
fi

if [ -z "$GOOGLE_CLIENT_SECRET" ] || [ "$GOOGLE_CLIENT_SECRET" == "your_client_secret_here" ]; then
    echo "❌ GOOGLE_CLIENT_SECRET is not configured"
    echo "   Please set it in your .env file"
    exit 1
else
    echo "✅ GOOGLE_CLIENT_SECRET: ${GOOGLE_CLIENT_SECRET:0:10}..."
fi

echo ""
echo "📊 Current authentication status:"
echo ""

# Check if token already exists
TOKEN_PATH="${HOME}/.config/grokster/gemini_token.json"
if [ -f "$TOKEN_PATH" ]; then
    echo "✅ OAuth token found at: $TOKEN_PATH"
    echo ""
    echo "   Run './grokster.sh status' to check if it's still valid"
else
    echo "ℹ️  No OAuth token found (this is normal for first-time setup)"
    echo ""
    echo "   Run './grokster.sh login' to authenticate"
fi

echo ""
echo "🚀 Next steps:"
echo ""
echo "   1. Run: ./grokster.sh login"
echo "   2. Sign in with your Google account in the browser"
echo "   3. Run: ./grokster.sh status (to verify)"
echo "   4. Run: ./grokster.sh (to start the TUI)"
echo ""
echo "   See OAUTH_SETUP.md for detailed instructions"
echo ""
