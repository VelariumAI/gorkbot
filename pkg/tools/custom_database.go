package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type SqliteQueryTool struct {
	BaseTool
}

func NewSqliteQueryTool() *SqliteQueryTool {
	return &SqliteQueryTool{
		BaseTool: BaseTool{
			name:               "sqlite_query",
			description:        "Executes a query against a local SQLite database.",
			category:           CategoryDatabase,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *SqliteQueryTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"db_path": map[string]interface{}{"type": "string", "description": "Path to the SQLite database file"},
			"query":   map[string]interface{}{"type": "string", "description": "SQL query to execute"},
		},
		"required": []string{"db_path", "query"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *SqliteQueryTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	dbPath, _ := params["db_path"].(string)
	query, _ := params["query"].(string)

	cmd := fmt.Sprintf("sqlite3 %s '%s'", dbPath, query)
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}

type PostgresConnectTool struct {
	BaseTool
}

func NewPostgresConnectTool() *PostgresConnectTool {
	return &PostgresConnectTool{
		BaseTool: BaseTool{
			name:               "postgres_query",
			description:        "Executes a query against a PostgreSQL database.",
			category:           CategoryDatabase,
			requiresPermission: true,
			defaultPermission:  PermissionSession,
		},
	}
}

func (t *PostgresConnectTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"conn_str": map[string]interface{}{"type": "string", "description": "PostgreSQL connection string"},
			"query":    map[string]interface{}{"type": "string", "description": "SQL query to execute"},
		},
		"required": []string{"conn_str", "query"},
	}
	b, _ := json.Marshal(schema)
	return b
}

func (t *PostgresConnectTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	connStr, _ := params["conn_str"].(string)
	query, _ := params["query"].(string)

	cmd := fmt.Sprintf("psql '%s' -c '%s'", connStr, query)
	bash := NewBashTool()
	return bash.Execute(ctx, map[string]interface{}{"command": cmd})
}
