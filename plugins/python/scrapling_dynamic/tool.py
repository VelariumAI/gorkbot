#!/usr/bin/env python3
"""
Scrapling Dynamic Tool - Full browser automation for JavaScript sites

Uses Scrapling's DynamicFetcher for complete browser control to handle
React, Vue, Angular, and other JavaScript-heavy frameworks.
"""

import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


@tool(description="Full browser automation for dynamic JS websites")
def dynamic(
    url: str,
    selector: str = "",
    headless: bool = True,
    network_idle: bool = True,
    wait_for: str = "",
    get_text: bool = False,
    get_all: bool = True,
    timeout: int = 60
) -> ToolResult:
    """
    Fetch pages using full browser automation for JavaScript-rendered content.

    Args:
        url: Target URL to fetch
        selector: CSS or XPath selector (optional)
        headless: Run browser in headless mode
        network_idle: Wait for network idle
        wait_for: Wait for specific selector before extracting
        get_text: Extract text content only
        get_all: Return all matches
        timeout: Timeout in seconds

    Returns:
        ToolResult with extracted data
    """
    try:
        from scrapling.fetchers import DynamicFetcher

        options = {
            "headless": headless,
            "network_idle": network_idle,
            "timeout": timeout * 1000,
        }

        # Fetch with dynamic browser
        page = DynamicFetcher.fetch(url, **options)

        # Wait for specific selector if provided
        if wait_for:
            try:
                page.wait_for(wait_for, timeout=timeout * 1000)
            except Exception:
                pass  # Continue even if wait fails

        if not selector:
            content = page.html if not get_text else page.text
            return ToolResult(
                success=True,
                output=content[:100000],
                data={
                    "url": url,
                    "status_code": page.status,
                    "browser": "chromium"
                }
            )

        # Apply selector
        if selector.startswith("//") or selector.startswith("("):
            elements = page.xpath(selector)
        else:
            elements = page.css(selector)

        if not elements:
            return ToolResult(
                success=True,
                output="No elements found",
                data={"url": url, "selector": selector, "count": 0}
            )

        if get_all:
            results = []
            for elem in elements:
                if get_text:
                    results.append(elem.text(strip=True))
                else:
                    results.append(str(elem))
            output = "\n---\n".join(results[:100])
            return ToolResult(
                success=True,
                output=output,
                data={
                    "url": url,
                    "selector": selector,
                    "count": len(results),
                    "results": results[:50]
                }
            )
        else:
            elem = elements[0]
            content = elem.text(strip=True) if get_text else str(elem)
            return ToolResult(
                success=True,
                output=content[:100000],
                data={
                    "url": url,
                    "selector": selector,
                    "count": 1
                }
            )

    except ImportError as e:
        return ToolResult(
            success=False,
            error=f"Scrapling not installed: {e}. Run: pip install scrapling[fetchers]"
        )
    except Exception as e:
        return ToolResult(
            success=False,
            error=f"Dynamic fetch failed: {str(e)}"
        )


if __name__ == "__main__":
    run()
