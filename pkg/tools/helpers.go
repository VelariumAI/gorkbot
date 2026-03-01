package tools

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
)

// jsonStringEscape returns s as a JSON-encoded string literal (with surrounding quotes).
func jsonStringEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// sqlEscapeSingleQuote doubles single quotes for safe SQL string literal embedding.
func sqlEscapeSingleQuote(s string) string {
	out := ""
	for _, c := range s {
		if c == '\'' {
			out += "''"
		} else {
			out += string(c)
		}
	}
	return out
}

// isDigits returns true if s is non-empty and contains only ASCII digit characters.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// urlEncodeQuery percent-encodes s for safe use as a URL query parameter value.
func urlEncodeQuery(s string) string {
	return url.QueryEscape(s)
}

// gorkStateDB returns the path to the Gorkbot persistent state database.
// Resolves to ~/.gorkbot/state.db on all platforms.
func gorkStateDB() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gorkbot", "state.db")
}

// gorkStateDBInit returns the sqlite3 init SQL that ensures all required tables exist.
func gorkStateDBInit() string {
	return `CREATE TABLE IF NOT EXISTS state (key TEXT PRIMARY KEY, value TEXT, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);` +
		`CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY AUTOINCREMENT, level TEXT, message TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);` +
		`CREATE TABLE IF NOT EXISTS tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT, description TEXT, priority TEXT DEFAULT 'medium', status TEXT DEFAULT 'pending', due_date TEXT, recurring INTEGER DEFAULT 0, dependencies TEXT, subtasks TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);` +
		`CREATE TABLE IF NOT EXISTS calls (id INTEGER PRIMARY KEY AUTOINCREMENT, tool TEXT, params TEXT, status TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`
}
