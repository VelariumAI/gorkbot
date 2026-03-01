package tools

// x_pull.go — X/Twitter API v2 tool with persistent token storage and robust error handling.
// Token is stored in state.db (key: x_bearer_token) so it is only entered once.
//
// Design notes:
//   - Validation uses /2/users/by/username/twitter (app-only bearer works; /2/users/me does not)
//   - All temp files use $TMPDIR (Termux has no /tmp)
//   - Shell commands avoid multi-line continuations for Termux compat
//   - When no token is stored the tool returns ACTION_REQUIRED so the orchestrator asks the user

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	xStateKey  = "x_bearer_token"
	xAPIBase   = "https://api.twitter.com/2"
	xMaxResult = 10
)

// XPullTool pulls live data from the X/Twitter API v2.
type XPullTool struct{ BaseTool }

func NewXPullTool() *XPullTool {
	return &XPullTool{BaseTool: BaseTool{
		name: "x_pull",
		description: "Pull live data from X/Twitter API v2 (search, user, timeline, tweet lookup, trends). " +
			"Requires a one-time setup: call with action=setup to store the bearer token. " +
			"All subsequent calls load the token automatically from local storage.",
		category:           CategoryWeb,
		requiresPermission: true,
		defaultPermission:  PermissionSession,
	}}
}

func (t *XPullTool) Parameters() json.RawMessage {
	s, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"setup", "search", "user", "timeline", "lookup", "trends", "status"},
				"description": "setup=store bearer token, status=check stored token, " +
					"search=recent tweets, user=profile, timeline=user's tweets, " +
					"lookup=tweet by ID, trends=trending topics",
			},
			"bearer_token": map[string]interface{}{
				"type":        "string",
				"description": "X API v2 Bearer Token. Only required for action=setup.",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query for action=search (X query syntax supported)",
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "X username without @ for action=user or action=timeline",
			},
			"tweet_id": map[string]interface{}{
				"type":        "string",
				"description": "Numeric tweet ID for action=lookup",
			},
			"woeid": map[string]interface{}{
				"type":        "string",
				"description": "Where On Earth ID for action=trends (default: 1 = worldwide)",
			},
			"max_results": map[string]interface{}{
				"type":        "integer",
				"description": "Max results (default 10, min 10, max 100) — X API requires ≥10 for search/timeline",
				"minimum":     10,
				"maximum":     100,
			},
		},
		"required": []string{"action"},
	})
	return s
}

func (t *XPullTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	action, _ := params["action"].(string)
	if action == "" {
		action = "status"
	}

	bash := NewBashTool()
	db := gorkStateDB()

	// Ensure state DB exists.
	if err := os.MkdirAll(filepath.Dir(db), 0700); err != nil {
		return xErr("cannot create state DB dir: " + err.Error()), nil
	}
	bash.Execute(ctx, map[string]interface{}{ //nolint:errcheck
		"command": fmt.Sprintf("sqlite3 %s %s 2>/dev/null; true", shellescape(db), shellescape(gorkStateDBInit())),
	})

	// Resolve temp dir — Termux uses $TMPDIR, not /tmp.
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = filepath.Join(os.Getenv("HOME"), ".tmp")
		os.MkdirAll(tmpDir, 0700) //nolint:errcheck
	}
	respFile := filepath.Join(tmpDir, "x_pull_resp.json")
	errFile := filepath.Join(tmpDir, "x_pull_err.txt")

	// ── setup ────────────────────────────────────────────────────────────────────
	if action == "setup" {
		token := strings.TrimSpace(params["bearer_token"].(string))
		if token == "" {
			return xNeedsToken(), nil
		}
		// Validate with an app-only-compatible endpoint before storing.
		msg, ok := xValidate(ctx, bash, token, respFile, errFile)
		if !ok {
			return xErr(fmt.Sprintf(
				"Token validation failed: %s\n\n"+
					"Make sure you copied the Bearer Token (not the API Key or Access Token).\n"+
					"Find it at: https://developer.x.com → your app → Keys and Tokens → Bearer Token",
				msg,
			)), nil
		}
		// Store.
		sql := fmt.Sprintf("INSERT OR REPLACE INTO state (key, value) VALUES ('%s', '%s');",
			sqlEscapeSingleQuote(xStateKey), sqlEscapeSingleQuote(token))
		res, _ := bash.Execute(ctx, map[string]interface{}{
			"command": fmt.Sprintf("sqlite3 %s %s", shellescape(db), shellescape(sql)),
		})
		if res != nil && res.Error != "" {
			return xErr("failed to store token in state DB: " + res.Error), nil
		}
		return &ToolResult{
			Output: fmt.Sprintf(
				"✅ Bearer token validated and stored (%s).\n%s\n\nYou're all set — x_pull will use this token automatically from now on.",
				xMask(token), msg,
			),
		}, nil
	}

	// ── status ───────────────────────────────────────────────────────────────────
	if action == "status" {
		token := xLoad(ctx, bash, db)
		if token == "" {
			return xNeedsToken(), nil
		}
		msg, ok := xValidate(ctx, bash, token, respFile, errFile)
		if !ok {
			return &ToolResult{
				Output: fmt.Sprintf(
					"⚠️  Stored token is no longer valid: %s\n\n"+
						"Run action=setup with a fresh token to update it.\n"+
						"Get a new token at: https://developer.x.com → your app → Keys and Tokens",
					msg,
				),
			}, nil
		}
		return &ToolResult{
			Output: fmt.Sprintf("✅ Stored token is valid (%s).\n%s", xMask(token), msg),
		}, nil
	}

	// ── All data actions require a token ─────────────────────────────────────────
	token := xLoad(ctx, bash, db)
	if override := strings.TrimSpace(func() string { s, _ := params["bearer_token"].(string); return s }()); override != "" {
		token = override
	}
	if token == "" {
		return xNeedsToken(), nil
	}

	maxResults := xMaxResult
	if v, ok := params["max_results"].(float64); ok && v >= 10 {
		maxResults = int(v)
		if maxResults > 100 {
			maxResults = 100
		}
	}

	switch action {

	case "search":
		query, _ := params["query"].(string)
		if strings.TrimSpace(query) == "" {
			return xErr("query is required for action=search"), nil
		}
		url := fmt.Sprintf(
			"%s/tweets/search/recent?query=%s&max_results=%d&tweet.fields=created_at,author_id,public_metrics,lang&expansions=author_id&user.fields=username,name",
			xAPIBase, urlEncodeQuery(strings.TrimSpace(query)), maxResults,
		)
		return xCall(ctx, bash, token, url, "Search results", respFile, errFile)

	case "user":
		username := strings.TrimPrefix(strings.TrimSpace(func() string { s, _ := params["username"].(string); return s }()), "@")
		if username == "" {
			return xErr("username is required for action=user"), nil
		}
		url := fmt.Sprintf("%s/users/by/username/%s?user.fields=name,username,description,public_metrics,verified,created_at",
			xAPIBase, urlEncodeQuery(username))
		return xCall(ctx, bash, token, url, "User profile: @"+username, respFile, errFile)

	case "timeline":
		username := strings.TrimPrefix(strings.TrimSpace(func() string { s, _ := params["username"].(string); return s }()), "@")
		if username == "" {
			return xErr("username is required for action=timeline"), nil
		}
		// Step 1: resolve user ID.
		idURL := fmt.Sprintf("%s/users/by/username/%s?user.fields=id", xAPIBase, urlEncodeQuery(username))
		idRes, _ := xCall(ctx, bash, token, idURL, "", respFile, errFile)
		if idRes.Error != "" {
			return idRes, nil
		}
		userID := xField(idRes.Output, "id")
		if userID == "" {
			return xErr(fmt.Sprintf("could not resolve user ID for @%s — does the account exist?", username)), nil
		}
		// Step 2: fetch timeline.
		tlURL := fmt.Sprintf("%s/users/%s/tweets?max_results=%d&tweet.fields=created_at,public_metrics,lang",
			xAPIBase, userID, maxResults)
		return xCall(ctx, bash, token, tlURL, "@"+username+" timeline", respFile, errFile)

	case "lookup":
		tweetID, _ := params["tweet_id"].(string)
		tweetID = strings.TrimSpace(tweetID)
		if tweetID == "" {
			return xErr("tweet_id is required for action=lookup"), nil
		}
		if !isDigits(tweetID) {
			return xErr("tweet_id must be numeric (the number in the tweet URL)"), nil
		}
		url := fmt.Sprintf("%s/tweets/%s?tweet.fields=created_at,author_id,public_metrics,lang,entities&expansions=author_id&user.fields=username,name",
			xAPIBase, tweetID)
		return xCall(ctx, bash, token, url, "Tweet "+tweetID, respFile, errFile)

	case "trends":
		woeid, _ := params["woeid"].(string)
		if woeid == "" {
			woeid = "1"
		}
		if !isDigits(woeid) {
			return xErr("woeid must be numeric (1=worldwide, 2459115=New York, 44418=London)"), nil
		}
		// Trends are v1.1 — v2 requires Pro tier.
		url := fmt.Sprintf("https://api.twitter.com/1.1/trends/place.json?id=%s", woeid)
		res, err := xCall(ctx, bash, token, url, "Trending topics", respFile, errFile)
		if err != nil {
			return res, err
		}
		if res.Error != "" {
			return &ToolResult{
				Output: "⚠️  Trends API error. Note: trending topics require X API Pro access.\n" + res.Output,
			}, nil
		}
		return res, nil

	default:
		return xErr(fmt.Sprintf("unknown action %q — valid: setup, status, search, user, timeline, lookup, trends", action)), nil
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// xLoad reads the bearer token from state.db. Returns "" if absent.
func xLoad(ctx context.Context, bash *BashTool, db string) string {
	sql := fmt.Sprintf("SELECT value FROM state WHERE key='%s' LIMIT 1;", sqlEscapeSingleQuote(xStateKey))
	res, err := bash.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("sqlite3 %s %s 2>/dev/null", shellescape(db), shellescape(sql)),
	})
	if err != nil || res == nil {
		return ""
	}
	return strings.TrimSpace(res.Output)
}

// xValidate checks a bearer token using /2/users/by/username/twitter
// (app-only bearer tokens work here; /2/users/me requires user-context OAuth).
func xValidate(ctx context.Context, bash *BashTool, token, respFile, errFile string) (string, bool) {
	// Use @twitter account — always exists, app-only bearer works.
	url := xAPIBase + "/users/by/username/twitter?user.fields=username,name"
	res, err := xCall(ctx, bash, token, url, "", respFile, errFile)
	if err != nil {
		return "network error: " + err.Error(), false
	}
	if res.Error != "" {
		return res.Error, false
	}
	name := xField(res.Output, "name")
	if name == "" {
		// Unexpected response body — still treat as success if HTTP was 200.
		return "token accepted by API", true
	}
	return "token accepted (test call: @twitter = " + name + ")", true
}

// xCall makes a GET request and returns formatted output with translated HTTP errors.
// Uses $TMPDIR for response buffering (Termux has no /tmp).
func xCall(ctx context.Context, bash *BashTool, token, url, label, respFile, errFile string) (*ToolResult, error) {
	// Build a single-line curl command — no backslash continuations for Termux compat.
	curlCmd := fmt.Sprintf(
		"curl -s -o %s -w '%%{http_code}' -H %s -H 'Accept: application/json' %s 2>%s",
		shellescape(respFile),
		shellescape("Authorization: Bearer "+token),
		shellescape(url),
		shellescape(errFile),
	)
	httpCodeRes, err := bash.Execute(ctx, map[string]interface{}{"command": curlCmd})
	if err != nil {
		return xErr("curl failed: " + err.Error()), nil
	}

	httpCode := strings.TrimSpace(httpCodeRes.Output)

	// If curl itself errored (no HTTP code produced).
	if httpCode == "" || httpCode == "000" {
		curlErr := ""
		if r, _ := bash.Execute(ctx, map[string]interface{}{
			"command": fmt.Sprintf("cat %s 2>/dev/null | head -3", shellescape(errFile)),
		}); r != nil {
			curlErr = strings.TrimSpace(r.Output)
		}
		msg := "network error — is internet available?"
		if curlErr != "" {
			msg += " curl says: " + curlErr
		}
		return xErr(msg), nil
	}

	// Read response body.
	bodyRes, _ := bash.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("cat %s 2>/dev/null", shellescape(respFile)),
	})
	body := ""
	if bodyRes != nil {
		body = strings.TrimSpace(bodyRes.Output)
	}

	// Pretty-print with jq if available, fall back to raw.
	prettyRes, _ := bash.Execute(ctx, map[string]interface{}{
		"command": fmt.Sprintf("echo %s | jq . 2>/dev/null || echo %s", shellescape(body), shellescape(body)),
	})
	pretty := body
	if prettyRes != nil && strings.TrimSpace(prettyRes.Output) != "" {
		pretty = strings.TrimSpace(prettyRes.Output)
	}

	switch httpCode {
	case "200", "201":
		prefix := ""
		if label != "" {
			prefix = "=== " + label + " ===\n"
		}
		return &ToolResult{Output: prefix + pretty}, nil
	case "401":
		return &ToolResult{
			Output: "❌ Authentication failed (401 Unauthorized).\n\n" +
				"The stored bearer token was rejected. To fix:\n" +
				"  1. Go to https://developer.x.com → your app → Keys and Tokens\n" +
				"  2. Regenerate the Bearer Token\n" +
				"  3. Call: x_pull(action=\"setup\", bearer_token=\"<new_token>\")",
			Error: "HTTP 401",
		}, nil
	case "403":
		return &ToolResult{
			Output: "❌ Permission denied (403 Forbidden).\n\n" +
				"Your app may not have access to this endpoint.\n" +
				"Check your X Developer app's access level (Basic/Pro/Enterprise).\n\nAPI response:\n" + pretty,
			Error: "HTTP 403",
		}, nil
	case "404":
		return &ToolResult{
			Output: "❌ Not found (404). Check the username or tweet ID.\n\nAPI response:\n" + pretty,
			Error:  "HTTP 404",
		}, nil
	case "429":
		return &ToolResult{
			Output: "❌ Rate limit exceeded (429). X API free tier: 500k tweets/month, 1 req/15 min for some endpoints.\n\nWait a moment and try again.",
			Error:  "HTTP 429",
		}, nil
	case "503":
		return &ToolResult{
			Output: "❌ X API temporarily unavailable (503). Try again in a moment.",
			Error:  "HTTP 503",
		}, nil
	default:
		return &ToolResult{
			Output: fmt.Sprintf("❌ Unexpected HTTP %s.\n\nAPI response:\n%s", httpCode, xTruncate(pretty, 400)),
			Error:  "HTTP " + httpCode,
		}, nil
	}
}

// xNeedsToken returns the standard "no token" response.
// The ACTION_REQUIRED prefix tells the AI to ask the user for the token.
func xNeedsToken() *ToolResult {
	return &ToolResult{
		Output: "ACTION_REQUIRED: No X/Twitter bearer token is stored.\n\n" +
			"Please ask the user to provide their X API Bearer Token:\n\n" +
			"  1. Go to https://developer.x.com/en/portal/dashboard\n" +
			"  2. Select or create an app\n" +
			"  3. Open Keys and Tokens\n" +
			"  4. Copy the Bearer Token (starts with AAAA...)\n\n" +
			"Once you have it, call: x_pull(action=\"setup\", bearer_token=\"<their_token>\")\n\n" +
			"The token will be saved locally and used automatically for all future calls.",
	}
}

// xErr returns a ToolResult flagged as an error.
func xErr(msg string) *ToolResult {
	return &ToolResult{Output: "❌ " + msg, Error: msg}
}

// xMask returns first 8 + last 4 chars of a token with *** in between.
func xMask(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:8] + "***" + token[len(token)-4:]
}

// xField extracts a JSON string field value from pretty-printed output.
func xField(jsonOut, field string) string {
	needle := `"` + field + `"`
	idx := strings.Index(jsonOut, needle)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimLeft(jsonOut[idx+len(needle):], " \t\r\n:")
	rest = strings.TrimLeft(rest, " \t\r\n")
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			return ""
		}
		return rest[1 : end+1]
	}
	end := strings.IndexAny(rest, ", \t\r\n}")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// xTruncate returns at most n runes of s.
func xTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
