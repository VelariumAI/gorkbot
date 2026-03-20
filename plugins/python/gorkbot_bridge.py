#!/usr/bin/env python3
"""
Gorkbot Python Plugin Bridge

This is the standard interface for Python plugins to communicate with Gorkbot.
Each plugin should import this module and implement the execute() function.

Example manifest.json:
{
    "name": "my_tool",
    "version": "1.0.0",
    "description": "My custom tool",
    "author": "user",
    "entry_point": "bridge.py",
    "parameters": {
        "input": {
            "type": "string",
            "description": "Input text",
            "required": true
        }
    },
    "requires": ["requests"],
    "category": "web"
}

Example plugin.py:
    from gorkbot_bridge import tool, ToolResult

    @tool(description="My tool does X")
    def my_tool(input: str) -> ToolResult:
        return ToolResult(
            success=True,
            output=f"Processed: {input}"
        )

    if __name__ == "__main__":
        run()
"""

import json
import sys
import os
from typing import Any, Callable, Dict, Optional
from dataclasses import dataclass, asdict
from functools import wraps


@dataclass
class ToolResult:
    """Result returned by a tool execution"""
    success: bool
    output: str = ""
    error: str = ""
    data: Optional[Dict[str, Any]] = None

    def to_dict(self) -> Dict[str, Any]:
        result = {
            "success": self.success,
            "output": self.output,
        }
        if self.error:
            result["error"] = self.error
        if self.data:
            result["data"] = self.data
        return result


class GorkbotTool:
    """Decorator class for registering tools"""

    def __init__(
        self,
        name: Optional[str] = None,
        description: str = "",
        category: str = "custom"
    ):
        self.name = name
        self.description = description
        self.category = category
        self._func: Optional[Callable] = None

    def __call__(self, func: Callable) -> 'GorkbotTool':
        self._func = func
        # Auto-generate name from function if not provided
        if not self.name:
            self.name = func.__name__
        if not self.description and func.__doc__:
            self.description = func.__doc__.strip()
        return self

    def execute(self, params: Dict[str, Any]) -> ToolResult:
        """Execute the tool with given parameters"""
        if self._func is None:
            return ToolResult(
                success=False,
                error="Tool function not defined"
            )

        try:
            # Inspect function signature to handle parameters
            import inspect
            sig = inspect.signature(self._func)

            # Filter params to only those accepted by the function
            filtered_params = {}
            for param_name, param in sig.parameters.items():
                if param_name in params:
                    filtered_params[param_name] = params[param_name]
                elif param.default is inspect.Parameter.empty:
                    # Required parameter not provided
                    return ToolResult(
                        success=False,
                        error=f"Missing required parameter: {param_name}"
                    )

            result = self._func(**filtered_params)

            # If result is just a string, wrap it
            if isinstance(result, str):
                return ToolResult(success=True, output=result)

            # If result is already a ToolResult, return it
            if isinstance(result, ToolResult):
                return result

            # Otherwise, wrap it
            return ToolResult(success=True, output=str(result))

        except Exception as e:
            return ToolResult(success=False, error=str(e))


# Global registry of tools
_TOOL_REGISTRY: Dict[str, GorkbotTool] = {}


def tool(
    name: Optional[str] = None,
    description: str = "",
    category: str = "custom"
) -> GorkbotTool:
    """
    Decorator to register a function as a Gorkbot tool.

    Example:
        @tool(description="Say hello")
        def hello(name: str) -> ToolResult:
            return ToolResult(success=True, output=f"Hello, {name}!")
    """
    def decorator(func: Callable) -> GorkbotTool:
        t = GorkbotTool(name=name, description=description, category=category)
        t._func = func
        tool_name = name or func.__name__
        _TOOL_REGISTRY[tool_name] = t
        return t
    return decorator


def get_tool(name: str) -> Optional[GorkbotTool]:
    """Get a tool by name from the registry"""
    return _TOOL_REGISTRY.get(name)


def list_tools() -> Dict[str, Dict[str, str]]:
    """List all registered tools"""
    return {
        name: {
            "description": t.description,
            "category": t.category
        }
        for name, t in _TOOL_REGISTRY.items()
    }


def run():
    """
    Main entry point for Gorkbot communication.
    Reads JSON from stdin, executes the requested tool, writes JSON to stdout.
    """
    import io
    from contextlib import redirect_stdout

    # Keep a reference to the real stdout
    real_stdout = sys.stdout

    try:
        # Read input from stdin
        input_data = sys.stdin.read()
        if not input_data.strip():
            real_stdout.write(json.dumps({
                "success": False,
                "error": "No input received"
            }))
            sys.exit(1)

        request = json.loads(input_data)

        action = request.get("action", "execute")
        tool_name = request.get("tool")
        params = request.get("params", {})

        if action == "list":
            # Return list of available tools
            real_stdout.write(json.dumps({
                "success": True,
                "tools": list_tools()
            }))
            return

        if action == "execute":
            if not tool_name:
                real_stdout.write(json.dumps({
                    "success": False,
                    "error": "No tool specified"
                }))
                sys.exit(1)

            t = get_tool(tool_name)
            if not t:
                real_stdout.write(json.dumps({
                    "success": False,
                    "error": f"Tool not found: {tool_name}"
                }))
                sys.exit(1)

            # Redirect all stdout from the tool to stderr during execution
            # this prevents print statements in tools from corrupting the JSON output
            with redirect_stdout(sys.stderr):
                result = t.execute(params)
            
            real_stdout.write(json.dumps(result.to_dict()))
            return

        real_stdout.write(json.dumps({
            "success": False,
            "error": f"Unknown action: {action}"
        }))

    except json.JSONDecodeError as e:
        real_stdout.write(json.dumps({
            "success": False,
            "error": f"Invalid JSON: {e}"
        }))
        sys.exit(1)

    except Exception as e:
        real_stdout.write(json.dumps({
            "success": False,
            "error": f"Execution error: {e}"
        }))
        sys.exit(1)


# Convenience imports for plugin authors
__all__ = ['tool', 'ToolResult', 'GorkbotTool', 'run', 'get_tool', 'list_tools']
