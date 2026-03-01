#!/usr/bin/env python3
"""
Scrapling Fetch Tool - Fast HTTP fetching with parsing

Uses Scrapling's Fetcher for fast HTTP requests with TLS fingerprinting
and powerful CSS/XPath selection capabilities.
"""

import sys
import os
import json

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


@tool(description="Fetch and parse web pages with Scrapling Fetcher")
def fetch(
    url: str,
    selector: str = "",
    method: str = "GET",
    impersonate: str = "chrome",
    headers: dict = None,
    timeout: int = 30,
    get_text: bool = False,
    get_all: bool = True
) -> ToolResult:
    """
    Fetch a URL and optionally extract elements using CSS/XPath selectors.

    Args:
        url: Target URL to fetch
        selector: CSS or XPath selector (optional - returns full page if empty)
        method: HTTP method (GET, POST, etc.)
        impersonate: Browser to impersonate for TLS fingerprint
        headers: Custom HTTP headers
        timeout: Request timeout in seconds
        get_text: If True, extract text content; if False, extract HTML
        get_all: If True, return all matches; if False, return first match

    Returns:
        ToolResult with extracted data
    """
    try:
        from scrapling.fetchers import Fetcher, FetcherSession

        headers = headers or {}

        with FetcherSession(impersonate=impersonate) as session:
            # Make the request
            page = session.get(url, headers=headers, timeout=timeout)

            if not selector:
                # No selector - return full page content
                content = page.html if not get_text else page.text
                return ToolResult(
                    success=True,
                    output=content[:100000],  # Limit output size
                    data={
                        "url": url,
                        "status_code": page.status,
                        "content_type": page.headers.get("content-type", "unknown"),
                        "content_length": len(content)
                    }
                )

            # Apply selector
            if selector.startswith("//") or selector.startswith("("):
                # XPath selector
                elements = page.xpath(selector)
            else:
                # CSS selector
                elements = page.css(selector)

            if not elements:
                return ToolResult(
                    success=True,
                    output="No elements found matching selector",
                    data={"url": url, "selector": selector, "count": 0}
                )

            # Extract data
            if get_all:
                results = []
                for elem in elements:
                    if get_text:
                        results.append(elem.text(strip=True))
                    else:
                        results.append(str(elem))
                output = "\n---\n".join(results[:100])  # Limit to 100 results
                return ToolResult(
                    success=True,
                    output=output,
                    data={
                        "url": url,
                        "selector": selector,
                        "count": len(results),
                        "results": results[:50]  # Include first 50 in data
                    }
                )
            else:
                # Single element
                elem = elements[0]
                content = elem.text(strip=True) if get_text else str(elem)
                return ToolResult(
                    success=True,
                    output=content[:100000],
                    data={
                        "url": url,
                        "selector": selector,
                        "count": 1,
                        "html": str(elem)[:1000] if not get_text else None
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
            error=f"Fetch failed: {str(e)}"
        )


if __name__ == "__main__":
    run()
