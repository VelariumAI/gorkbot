#!/usr/bin/env python3
"""
Scrapling Stealth Tool - Anti-bot bypass and Cloudflare circumvention

Uses Scrapling's StealthyFetcher for bypassing Cloudflare, Turnstile,
and other anti-bot protections.
"""

import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


@tool(description="Stealthy fetch with Cloudflare and anti-bot bypass")
def stealth(
    url: str,
    selector: str = "",
    headless: bool = True,
    solve_cloudflare: bool = True,
    network_idle: bool = True,
    get_text: bool = False,
    get_all: bool = True,
    timeout: int = 60
) -> ToolResult:
    """
    Fetch URLs with stealth capabilities to bypass anti-bot protections.

    Args:
        url: Target URL to fetch
        selector: CSS or XPath selector (optional)
        headless: Run browser in headless mode
        solve_cloudflare: Attempt to solve Cloudflare challenges
        network_idle: Wait for network idle
        get_text: Extract text content only
        get_all: Return all matches
        timeout: Timeout in seconds

    Returns:
        ToolResult with extracted data
    """
    try:
        from scrapling.fetchers import StealthyFetcher

        # Build fetch options
        options = {
            "headless": headless,
            "solve_cloudflare": solve_cloudflare,
            "network_idle": network_idle,
            "timeout": timeout * 1000,  # Convert to milliseconds
        }

        # Fetch the page
        page = StealthyFetcher.fetch(url, **options)

        if not selector:
            # No selector - return full page content
            content = page.html if not get_text else page.text
            return ToolResult(
                success=True,
                output=content[:100000],
                data={
                    "url": url,
                    "status_code": page.status,
                    "solved_cloudflare": True
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
            error=f"Stealth fetch failed: {str(e)}"
        )


if __name__ == "__main__":
    run()
