# Contributing to Gorkbot

Thank you for your interest in contributing to Gorkbot! This guide explains how to participate in the project.

---

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [Development Setup](#development-setup)
4. [Making Changes](#making-changes)
5. [Testing](#testing)
6. [Commit Messages](#commit-messages)
7. [Pull Requests](#pull-requests)
8. [Reporting Issues](#reporting-issues)
9. [Feature Requests](#feature-requests)

---

## Code of Conduct

By participating in Gorkbot, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## Getting Started

### Prerequisites
- Go 1.25.0+
- Git
- Basic familiarity with Go
- GitHub account

### Fork & Clone

```bash
# Fork repository at https://github.com/velariumai/gorkbot

# Clone your fork
git clone https://github.com/YOUR_USERNAME/gorkbot.git
cd gorkbot

# Add upstream remote
git remote add upstream https://github.com/velariumai/gorkbot.git

# Create feature branch
git checkout -b feature/your-feature-name
```

---

## Development Setup

### Install Dependencies

```bash
# Download Go modules
go mod download

# Verify dependencies
go mod tidy
go mod verify
```

### Build & Test

```bash
# Build for your platform
make build

# Run tests
go test ./...

# Run linter
go vet ./...

# Check formatting
gofmt -l ./

# Format all files
gofmt -w ./cmd ./internal ./pkg
```

### Running Locally

```bash
# Set up API keys
export XAI_API_KEY=xai-xxx
export GEMINI_API_KEY=AIza-xxx

# Run from source
./bin/gorkbot

# Or with make
make run
```

---

## Making Changes

### Code Style

- Follow Go conventions (gofmt, go vet)
- Functions under 50 lines when possible
- Comment exported functions
- Use meaningful variable names
- Prefer interfaces for dependencies

### Project Structure

```
gorkbot/
├── cmd/           Entry points
├── internal/      Non-exported packages (TUI, engine)
├── pkg/           Exported libraries (tools, sense, spark, etc.)
├── docs/          Documentation
├── examples/      Example projects
└── Makefile       Build targets
```

### Naming Conventions

- **Packages**: lowercase, short (no underscores)
- **Functions**: CamelCase
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE
- **Interfaces**: NameReader, NameWriter (verb-based)

### Comments

```go
// Package tools provides AI tool execution and registry.
package tools

// Tool defines the interface for executable tools.
type Tool interface {
    Name() string
    Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
}

// registry holds all available tools.
type registry struct {
    tools map[string]Tool
}
```

### Error Handling

```go
// Good: Wrap errors with context
if err := os.Stat(path); err != nil {
    return fmt.Errorf("check path %q: %w", path, err)
}

// Avoid: Ignore errors
_ = os.Stat(path)  // Only if intentional and documented
```

---

## Testing

### Write Tests

```bash
# Test specific package
go test -v ./pkg/tools

# Test with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Files

- Place tests in `*_test.go` files
- Use table-driven tests for multiple cases
- Use `testify/assert` or `testify/require` for assertions
- Mock external dependencies

```go
// Example test
func TestToolRegistry_Execute(t *testing.T) {
    tests := []struct {
        name    string
        toolName string
        params  map[string]interface{}
        wantErr bool
    }{
        {"valid", "bash", map[string]interface{}{"command": "echo hello"}, false},
        {"invalid", "nonexistent", map[string]interface{}{}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            reg := NewRegistry()
            _, err := reg.Execute(context.Background(), tt.toolName, tt.params)
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Before Submitting

```bash
# Run all tests
go test ./...

# Run linter
go vet ./...

# Format code
gofmt -w ./cmd ./internal ./pkg

# Check no syntax errors
go build ./...
```

---

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/) format:

```
type(scope): description

optional body

optional footer
```

### Types

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation changes
- **style**: Code style changes (formatting, missing semicolons, etc.)
- **refactor**: Code refactoring (no feature changes)
- **perf**: Performance improvements
- **test**: Test additions or modifications
- **chore**: Build process, dependencies, etc.
- **security**: Security-related fixes

### Examples

```bash
# Feature
git commit -m "feat(tools): add sql_query tool for database operations"

# Fix
git commit -m "fix(tui): prevent viewport scroll freeze on large messages"

# Documentation
git commit -m "docs(api): clarify tool parameter validation rules"

# Breaking change
git commit -m "feat(cache)!: redesign cache key structure

BREAKING CHANGE: Old cache keys no longer supported. Run migration."

# With scope
git commit -m "fix(streaming): handle incomplete JSON blocks correctly"
```

---

## Pull Requests

### Before Submitting

1. **Update main branch**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Test everything**:
   ```bash
   go test ./...
   go vet ./...
   gofmt -w ./
   ```

3. **Write clear title**: "Add tool execution timeout feature"
4. **Write detailed description**: What, why, how
5. **Link related issues**: "Fixes #123"

### PR Checklist

- [ ] Tests added or updated
- [ ] Documentation updated
- [ ] All tests passing (`go test ./...`)
- [ ] Code formatted (`gofmt`)
- [ ] No linter warnings (`go vet`)
- [ ] Conventional commit messages
- [ ] PR title clearly describes changes

### PR Template

```markdown
## Description
Brief summary of changes

## Type of Change
- [ ] New feature
- [ ] Bug fix
- [ ] Documentation update
- [ ] Refactoring

## Related Issues
Fixes #123

## Testing
- [ ] Unit tests added
- [ ] Integration tests added
- [ ] Manual testing done

## Screenshots (if applicable)
<!-- Add screenshots for UI changes -->
```

### Review Process

- At least 1 approval required
- All CI checks must pass
- No unresolved conversations
- Author can merge after approval

---

## Reporting Issues

### Security Issues

**Do not** open public issues for security vulnerabilities.

Email: velarium.ai@gmail.com with:
- Description of vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if available)

### Bug Reports

Use [GitHub Issues](https://github.com/velariumai/gorkbot/issues/new/choose):

**Include**:
- **Title**: Concise summary
- **Description**: What happened, what should happen
- **Steps to reproduce**: Exact steps to trigger bug
- **Environment**: OS, Go version, Gorkbot version
- **Logs**: Error messages, SENSE traces if available
- **Screenshots**: For TUI bugs

```markdown
**Describe the bug**
Gorkbot crashes when...

**Steps to reproduce**
1. Run `./gorkbot`
2. Execute command `...`
3. See error

**Expected behavior**
Should display...

**Environment**
- OS: Ubuntu 20.04
- Go: 1.25.0
- Gorkbot: 1.2.0-beta

**Logs**
```
error message here
```
```

---

## Feature Requests

Use [GitHub Discussions](https://github.com/velariumai/gorkbot/discussions):

**Include**:
- **Use case**: Why you need this
- **Proposed solution**: How it should work
- **Alternatives**: Other approaches considered
- **Examples**: Sample usage

---

## Documentation Contributions

### Update Docs

```bash
# Documentation files
docs/*.md              # Guides and references
README.md              # Main overview
ARCHITECTURE.md        # System design
API_REFERENCE.md       # API documentation
```

### Documentation Style

- Clear and concise
- Use examples
- Include code snippets
- Link to related sections
- Update table of contents

---

## Getting Help

- 📖 [DEVELOPMENT.md](DEVELOPMENT.md) - Developer guide
- 🏗️ [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
- 💬 [GitHub Discussions](https://github.com/velariumai/gorkbot/discussions)
- 📧 Email: velarium.ai@gmail.com

---

## License

By contributing to Gorkbot, you agree that your contributions will be licensed under its MIT License.

---

**Thank you for contributing to Gorkbot! 🙏**

