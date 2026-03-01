#!/usr/bin/env python3
"""
Scrapling Extract Tool - CLI wrapper for easy URL extraction

Wraps Scrapling's extract command for quick scraping without code.
"""

import sys
import os
import subprocess
import tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


@tool(description="Extract content from URLs using Scrapling CLI")
def extract(
    url: str,
    output: str = "",
    selector: str = "",
    impersonate: str = "chrome",
    fetch_mode: str = "get",
    headless: bool = True,
    solve_cloudflare: bool = False,
    timeout: int = 30
) -> ToolResult:
    """
    Use Scrapling's extract command for quick URL content extraction.

    Args:
        url: Target URL
        output: Output file (optional - returns content if not set)
        selector: CSS selector for targeted extraction
        impersonate: Browser to impersonate
        fetch_mode: get, fetch, or stealthy-fetch
        headless: Run headless
        solve_cloudflare: Solve Cloudflare challenges
        timeout: Timeout in seconds

    Returns:
        ToolResult with extracted content
    """
    try:
        # Build the command
        cmd = ["scrapling", "extract", fetch_mode, url]

        # Determine output path
        if output:
            output_path = output
        else:
            # Create temp file
            fd, output_path = tempfile.mkstemp(suffix=".md")
            os.close(fd)

        cmd.extend(["-o", output_path])

        # Add optional flags
        if selector:
            cmd.extend(["--css-selector", selector])

        if impersonate:
            cmd.extend(["--impersonate", impersonate])

        if not headless:
            cmd.append("--no-headless")

        if solve_cloudflare:
            cmd.append("--solve-cloudflare")

        # Execute command
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout
        )

        if result.returncode != 0:
            return ToolResult(
                success=False,
                error=f"Scrapling extract failed: {result.stderr}"
            )

        # Read output
        if os.path.exists(output_path):
            with open(output_path, "r", encoding="utf-8") as f:
                content = f.read()

            # Clean up temp file
            if not output:
                os.unlink(output_path)

            return ToolResult(
                success=True,
                output=content[:100000],
                data={
                    "url": url,
                    "output_file": output if output else "temp",
                    "content_length": len(content)
                }
            )
        else:
            return ToolResult(
                success=False,
                error="Output file not created"
            )

    except FileNotFoundError:
        return ToolResult(
            success=False,
            error="Scrapling CLI not found. Install with: pip install scrapling[shell]"
        )
    except subprocess.TimeoutExpired:
        return ToolResult(
            success=False,
            error=f"Extract timed out after {timeout} seconds"
        )
    except Exception as e:
        return ToolResult(
            success=False,
            error=f"Extract failed: {str(e)}"
        )


if __name__ == "__main__":
    run()
