# Python Plugin Integration for Gorkbot

This document describes how to add Python plugins to the Go-based Gorkbot system.

## Overview

The Python plugin system allows you to:
- Write tools in Python (leverage Python's extensive libraries)
- Use existing Python scripts without rewriting in Go
- Create custom AI/ML tools using Python's rich ecosystem
- Extend Gorkbot's capabilities without recompiling the binary

## Architecture

```
Gorkbot (Go)
    │
    ├── Tool Registry ───────────┐
    │                            │
    │                       ┌────▼──────────────────┐
    │                       │  Python Plugin Manager │
    │                       │  (pkg/python/)         │
    │                       │                        │
    │                       │  • Loads manifest.json │
    │                       │  • Wraps Python tools  │
    │                       │  • Handles JSON I/O    │
    │                       └────────────────────────┘
    │                                       │
    │                                       ▼
    │                              ┌──────────────────┐
    │                              │  Python Scripts  │
    │                              │  (plugins/*)     │
    │                              │                  │
    │                              │  • gorkbot_bridge│
    │                              │  • Your tools    │
    │                              └──────────────────┘
    │
    └── All tools executed uniformly
```

## Quick Start

### 1. Create Plugin Directory

```bash
mkdir -p plugins/python/my_tool
```

### 2. Create manifest.json

```json
{
    "name": "my_tool",
    "version": "1.0.0",
    "description": "My custom tool",
    "author": "you",
    "entry_point": "main.py",
    "parameters": {
        "input": {
            "type": "string",
            "description": "Input parameter",
            "required": true
        }
    },
    "requires": ["requests", "numpy"],
    "category": "custom"
}
```

### 3. Create Python Tool

```python
#!/usr/bin/env python3
import sys
import os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run

@tool(description="My tool does something useful")
def my_tool(input: str) -> ToolResult:
    """Process the input and return a result"""
    # Your logic here
    return ToolResult(success=True, output=f"Processed: {input}")

if __name__ == "__main__":
    run()
```

### 4. Register in Gorkbot

Add to your tool registration:

```go
// In cmd/grokster/main.go or orchestrator setup
pluginDir := filepath.Join(configDir, "plugins", "python")
pyManager := python.NewManager(pluginDir, registry)

// Load and register all Python plugins
if err := pyManager.LoadAllPlugins(); err != nil {
    logger.Error("failed to load Python plugins", "error", err)
}
if err := pyManager.RegisterAll(); err != nil {
    logger.Error("failed to register Python plugins", "error", err)
}
```

## Plugin Manifest Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique tool name |
| `version` | string | Yes | Semantic version |
| `description` | string | Yes | Human-readable description |
| `author` | string | No | Plugin author |
| `entry_point` | string | Yes | Python file to execute |
| `parameters` | object | No | JSON schema for parameters |
| `requires` | array | No | pip dependencies |
| `category` | string | Yes | Tool category |

## Tool Bridge API

The `gorkbot_bridge.py` module provides:

### @tool Decorator

```python
from gorkbot_bridge import tool, ToolResult

@tool(name="my_tool", description="Does X")
def my_function(param1: str, param2: int = 10) -> ToolResult:
    return ToolResult(success=True, output="Result")
```

### ToolResult

```python
ToolResult(
    success=True,
    output="Human-readable output",
    error="Error message if failed",
    data={"key": "Structured data"}
)
```

### Available Functions

- `tool()` - Decorator to register functions as tools
- `ToolResult` - Return type for tool execution
- `run()` - Main entry point (call in `if __name__ == "__main__":`)
- `get_tool(name)` - Get a tool by name
- `list_tools()` - List all registered tools

## Parameter Types

Parameters follow JSON Schema:

```json
{
    "name": {
        "type": "string",
        "description": "What this parameter does",
        "required": true,
        "default": "default value"
    },
    "count": {
        "type": "integer",
        "description": "A number",
        "required": false,
        "default": "5"
    },
    "enabled": {
        "type": "boolean",
        "description": "A flag",
        "required": false,
        "default": "true"
    }
}
```

Supported types: `string`, `integer`, `number`, `boolean`, `array`, `object`

## Examples

See `plugins/python/` for complete examples:

1. **example_weather** - Basic tool with parameters
2. **example_ai_analysis** - ML/NLP tool using textblob and nltk

### Example: Weather Tool

```python
@tool(description="Get weather for a city")
def get_weather(city: str, units: str = "celsius") -> ToolResult:
    # Call weather API, process data
    return ToolResult(
        success=True,
        output=f"Weather in {city}: 22°C, Sunny",
        data={"temp": 22, "condition": "Sunny"}
    )
```

### Example: AI Analysis Tool

```python
@tool(description="Sentiment analysis")
def sentiment(text: str) -> ToolResult:
    from textblob import TextBlob
    blob = TextBlob(text)

    return ToolResult(
        success=True,
        output=f"Sentiment: {blob.sentiment.polarity}",
        data={"polarity": blob.sentiment.polarity}
    )
```

## Integration with Go System

### Initialize the Manager

```go
import "github.com/velariumai/gorkbot/pkg/python"

// In your setup code
configDir := platform.GorkbotConfigDir()
pluginDir := filepath.Join(configDir, "plugins", "python")

// Ensure plugin directory exists
os.MkdirAll(pluginDir, 0755)

pyManager := python.NewManager(pluginDir, registry)
pyManager.SetPythonCmd("python3")  // or "python" on Windows

// Load plugins
if err := pyManager.LoadAllPlugins(); err != nil {
    logger.Warn("Python plugins not loaded", "error", err)
}

// Register with tool registry
if err := pyManager.RegisterAll(); err != nil {
    logger.Warn("Python plugins not registered", "error", err)
}
```

### Configuration Options

```go
// Set custom Python command
pyManager.SetPythonCmd("/usr/bin/python3.11")

// Enable/disable plugin system
pyManager.Enable(true)  // or false to disable

// Check status
if pyManager.IsEnabled() {
    plugins := pyManager.GetLoadedPlugins()
    logger.Info("Loaded plugins", "count", len(plugins))
}
```

## Execution Flow

1. **User invokes tool** → Gorkbot orchestrator
2. **Tool registry** looks up tool by name
3. **If Python tool** → `PythonTool.Execute()` called
4. **JSON request** written to Python's stdin
5. **Python bridge** parses request, calls decorated function
6. **Result** serialized to JSON, written to stdout
7. **Go wrapper** parses JSON, returns `*tools.ToolResult`

```
┌──────────┐    JSON     ┌──────────────┐    call    ┌─────────────┐
│   Go     │ ──────────► │  bridge.py   │ ─────────► │ @tool func  │
│ Execute  │             │   (parse)    │            │  (user)     │
│          │ ◄────────── │              │ ◄───────── │             │
└──────────┘    JSON     └──────────────┘    JSON     └─────────────┘
```

## Security Considerations

1. **Permission System**: Python tools require explicit user permission
2. **Sandboxing**: Consider using Python subprocess with limited permissions
3. **Dependencies**: Auto-install pip packages (can be disabled)
4. **Timeout**: Default 5-minute timeout on Python execution
5. **Input Validation**: Parameters validated via JSON schema

## Performance Notes

- **Subprocess overhead**: Each invocation spawns a Python process
- **Cold start**: First call may be slower (imports, dependency check)
- **Caching**: Consider caching Python results for repeated calls
- **Parallel execution**: Multiple Python tools can run concurrently

## Future Enhancements

- **Embedded Python**: Use cgo to embed Python interpreter (faster)
- **Persistent Process**: Reuse Python process across calls
- **IPC**: Use named pipes for better communication
- **Type Stubs**: Generate Go types from Python signatures

## Troubleshooting

### "Python not found"
```bash
# Check Python installation
which python3
python3 --version

# Update path in Gorkbot
pyManager.SetPythonCmd("/full/path/to/python3")
```

### "Module not found"
```bash
# Manually install dependencies
pip install -r plugins/python/your_plugin/requirements.txt

# Or let the plugin manager handle it (requires pip)
```

### "Tool not found"
- Check `manifest.json` exists in plugin directory
- Verify `entry_point` file exists
- Ensure plugin is in correct directory
- Check Gorkbot logs for loading errors

### "Permission denied"
- Make Python scripts executable: `chmod +x *.py`
- Or run Gorkbot with appropriate permissions
