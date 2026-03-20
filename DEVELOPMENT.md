# Development Guide

Setup, build, and develop Gorkbot.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Project Structure](#project-structure)
3. [Development Workflow](#development-workflow)
4. [Building](#building)
5. [Testing](#testing)
6. [Code Style](#code-style)
7. [Debugging](#debugging)
8. [Common Tasks](#common-tasks)

---

## Quick Start

### Prerequisites

```bash
# Go 1.25.0+
go version

# Git
git --version

# Make (optional, for Makefile targets)
make --version
```

### Setup

```bash
# Clone
git clone https://github.com/velariumai/gorkbot.git
cd gorkbot

# Verify environment
go version
go mod download
go mod tidy

# Build
make build

# Verify
./bin/gorkbot --version
```

---

## Project Structure

```
gorkbot/
├── cmd/
│   └── gorkbot/         Main CLI (81KB main.go)
├── internal/            Non-exported packages
│   ├── engine/          Orchestrator (34 files)
│   ├── tui/             Terminal UI (34 files)
│   ├── xskill/          Continual learning
│   ├── platform/        Environment detection
│   ├── llm/             LLM bridge
│   ├── webui/           Web UI (Gin)
│   └── inline/          REPL
├── pkg/                 Exported libraries (46 packages)
│   ├── ai/              AI providers (7 implementations)
│   ├── tools/           Tool registry & execution
│   ├── sense/           Stability layer
│   ├── spark/           Reasoning daemon
│   ├── sre/             Step-wise reasoning
│   ├── adaptive/        ARC router + MEL learning
│   ├── cache/           Provider caching
│   ├── hitl/            Human-in-the-loop
│   ├── persist/         SQLite persistence
│   ├── channels/        Discord/Telegram
│   ├── mcp/             Model Context Protocol
│   └── ... (36+ packages)
├── docs/                Documentation
├── examples/            Example projects
├── Makefile             Build automation
├── go.mod               Dependencies
├── go.sum               Dependency hashes
├── ARCHITECTURE.md      System architecture
└── README.md            Main documentation
```

---

## Development Workflow

### Create Feature Branch

```bash
# Update main
git fetch upstream
git checkout main
git merge upstream/main

# Create feature branch
git checkout -b feature/your-feature-name

# Make changes
# ... edit files ...

# Commit with conventional commits
git add .
git commit -m "feat(package): description of change"
```

### Before Submitting PR

```bash
# Update from upstream
git fetch upstream
git rebase upstream/main

# Format code
gofmt -w ./cmd ./internal ./pkg

# Run tests
go test ./...

# Run linter
go vet ./...

# Build
make build

# Verify
./bin/gorkbot --version
```

### Push & Create PR

```bash
# Push to your fork
git push origin feature/your-feature-name

# Create PR on GitHub
# https://github.com/velariumai/gorkbot/compare
```

---

## Building

### Build Targets

```bash
# Host OS (outputs to ./bin/gorkbot)
make build

# Specific platforms
make build-linux       # Linux x86_64
make build-macos       # macOS universal
make build-windows     # Windows x86_64
make build-android     # Android arm64

# All platforms
make build-all

# Clean build artifacts
make clean

# Install to system
make install
```

### Build Flags

```bash
# Optimized build (smaller binary)
CGO_ENABLED=0 go build \
  -ldflags="-s -w" \
  -o bin/gorkbot \
  ./cmd/gorkbot/

# With version info
go build \
  -ldflags="-X main.Version=1.2.0 -X main.BuildTime=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  -o bin/gorkbot \
  ./cmd/gorkbot/

# For specific platform
GOOS=linux GOARCH=amd64 go build -o bin/gorkbot-linux ./cmd/gorkbot/
GOOS=darwin GOARCH=arm64 go build -o bin/gorkbot-mac-arm ./cmd/gorkbot/
GOOS=windows GOARCH=amd64 go build -o bin/gorkbot.exe ./cmd/gorkbot/
```

---

## Testing

### Run Tests

```bash
# All tests
go test ./...

# With verbose output
go test -v ./...

# Specific package
go test -v ./pkg/tools

# With coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. -benchmem ./pkg/tools
```

### Write Tests

```go
// Table-driven test pattern
func TestToolRegistry_Execute(t *testing.T) {
    tests := []struct {
        name    string
        toolName string
        params  map[string]interface{}
        want    string
        wantErr bool
    }{
        {"valid bash", "bash", map[string]interface{}{"command": "echo hello"}, "hello", false},
        {"invalid tool", "nonexistent", map[string]interface{}{}, "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

---

## Code Style

### Format Code

```bash
# Format all files
gofmt -w ./cmd ./internal ./pkg

# Check formatting
gofmt -l ./

# Use go fmt explicitly
go fmt ./...
```

### Naming Conventions

```go
// Package names: short, lowercase, no underscores
package tools

// Exported types: CamelCase
type Registry struct {
    tools map[string]Tool
}

// Unexported fields/vars: camelCase
var defaultTimeout = 30 * time.Second

// Functions: CamelCase
func (r *Registry) Execute(ctx context.Context, name string) error {
    return nil
}

// Constants: UPPER_SNAKE_CASE (optionally)
const MaxRetries = 3
```

### Comments

```go
// Package comments for exported packages
// Package tools provides AI tool registry and execution.
package tools

// Function comments for exported functions
// Execute runs the specified tool and returns its result.
// It validates permissions and logs execution to audit trail.
func (r *Registry) Execute(ctx context.Context, name string) (*ToolResult, error) {
    // Implementation
}

// Unexported helpers don't require comments
// unless logic is complex
func (r *registry) findTool(name string) Tool {
    return r.tools[name]
}
```

---

## Debugging

### Enable Debug Logging

```bash
# Log to stderr
./gorkbot -debug-mcp

# More verbose
./gorkbot -watchdog

# With extended thinking visible
./gorkbot -verbose-thoughts
```

### Check Logs

```bash
# Structured logs (JSON)
cat ~/.config/gorkbot/gorkbot.json | jq '.'

# Filter by level
cat ~/.config/gorkbot/gorkbot.json | jq 'select(.level=="ERROR")'

# SENSE traces
cat ~/.config/gorkbot/trace/$(date +%Y-%m-%d).jsonl | jq '.'

# Filter by event kind
cat ~/.config/gorkbot/trace/$(date +%Y-%m-%d).jsonl | jq 'select(.kind=="tool_failure")'
```

### Use Debugger

```bash
# With Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug binary
dlv debug ./cmd/gorkbot/main.go

# In Delve CLI:
# (dlv) break main.main
# (dlv) continue
# (dlv) next
# (dlv) print variable
# (dlv) quit
```

---

## Common Development Tasks

### Add a New Tool

1. **Implement Tool interface** in `pkg/tools/`:

```go
type MyTool struct {
    // fields
}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something cool" }
func (t *MyTool) Category() ToolCategory { return CategoryFile }

func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
    // Implementation
    return &ToolResult{
        Success: true,
        Output: "result",
    }, nil
}
```

2. **Register in registry** in `pkg/tools/defaults.go`:

```go
func RegisterDefaultTools(r *Registry) {
    r.Register(NewMyTool())
}
```

3. **Write tests** in `pkg/tools/my_tool_test.go`

4. **Document in TOOL_REFERENCE.md**

---

### Add a New AI Provider

1. **Implement AIProvider interface** in `pkg/ai/`:

```go
type MyProvider struct {
    apiKey string
    model  string
}

func (p *MyProvider) Generate(ctx context.Context, prompt string) (string, error) {
    // API call
    return response, nil
}

func (p *MyProvider) StreamWithHistory(ctx context.Context, history *ConversationHistory, out io.Writer) error {
    // Streaming implementation
    return nil
}

// ... other methods
```

2. **Register in provider discovery** in `pkg/discovery/`

3. **Add caching strategy** in `pkg/cache/`

4. **Write tests** in `pkg/ai/my_provider_test.go`

---

### Modify Database Schema

1. **Add migration** in `pkg/persist/sqlite.go`:

```go
func (s *Store) migrateV7() error {
    _, err := s.db.Exec(`
        ALTER TABLE conversations ADD COLUMN new_field TEXT;
    `)
    return err
}
```

2. **Increment version** in migrate() method

3. **Test migration** - `go test -v ./pkg/persist`

---

### Add New TUI Command

1. **Add handler** in `pkg/commands/registry.go`:

```go
func (oa *OrchestratorAdapter) HandleMyCommand(arg string) string {
    // Implementation
    return "response"
}
```

2. **Register command** in TUI model

3. **Add help text** in help command

---

## Running Integration Tests

```bash
# Set up test environment
export XAI_API_KEY=test-key
export GEMINI_API_KEY=test-key

# Run integration tests
go test -tags=integration ./...

# Or specific test
go test -v -run TestIntegration ./pkg/ai
```

---

## Performance Profiling

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./pkg/tools
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof ./pkg/tools
go tool pprof mem.prof

# In pprof shell:
# (pprof) top      # Top memory consumers
# (pprof) list foo # Show function foo
# (pprof) web      # Generate graph (requires graphviz)
```

---

## Release Process

```bash
# Update version
# - Update pkg/version/version.go
# - Update CHANGELOG.md
# - Update README.md

# Commit
git add .
git commit -m "chore(release): bump to 1.2.1"

# Tag
git tag -a v1.2.1 -m "Release 1.2.1"

# Push
git push origin main
git push origin v1.2.1

# GitHub Actions will build and release binaries
```

---

**For more details, see [ARCHITECTURE.md](ARCHITECTURE.md) and [CONTRIBUTING.md](CONTRIBUTING.md).**

