#!/usr/bin/env python3
"""
Scrapling Search Tool - Web search by scraping search engines

Uses Scrapling to scrape search engine results from DuckDuckGo, Google, or Bing.
"""

import sys
import os
import urllib.parse

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


# Search engine URLs and selectors
ENGINES = {
    "duckduckgo": {
        "url": "https://html.duckduckgo.com/html/",
        "form": {"q": "{query}", "b": "{offset}"},
        "result_selector": ".result",
        "title_selector": ".result__title",
        "url_selector": ".result__url",
        "snippet_selector": ".result__snippet",
    },
    "google": {
        "url": "https://www.google.com/search",
        "params": {"q": "{query}", "num": "{num}", "hl": "{locale}"},
        "result_selector": "div.g",
        "title_selector": "h3",
        "url_selector": "a",
        "snippet_selector": "div.VwiC3b",
    },
    "bing": {
        "url": "https://www.bing.com/search",
        "params": {"q": "{query}", "count": "{num}"},
        "result_selector": ".b_algo",
        "title_selector": "h2 a",
        "url_selector": "h2 a",
        "snippet_selector": "p",
    },
}


@tool(description="Search the web using search engines via Scrapling")
def search(
    query: str,
    engine: str = "duckduckgo",
    num_results: int = 10,
    safe_search: bool = True,
    time_range: str = "",
    locale: str = "en-us"
) -> ToolResult:
    """
    Perform a web search by scraping search engine results.

    Args:
        query: Search query string
        engine: Search engine (duckduckgo, google, bing)
        num_results: Number of results to return
        safe_search: Enable safe search
        time_range: Time filter (d, w, m, y)
        locale: Locale for search

    Returns:
        ToolResult with search results
    """
    # Normalize engine name
    engine = engine.lower()
    if engine not in ENGINES:
        return ToolResult(
            success=False,
            error=f"Unknown engine: {engine}. Supported: {', '.join(ENGINES.keys())}"
        )

    try:
        from scrapling.fetchers import StealthyFetcher, StealthySession

        config = ENGINES[engine]

        # Build URL based on engine
        if engine == "duckduckgo":
            return search_duckduckgo(query, num_results, config)
        elif engine == "google":
            return search_google(query, num_results, locale, config)
        elif engine == "bing":
            return search_bing(query, num_results, config)

    except ImportError as e:
        return ToolResult(
            success=False,
            error=f"Scrapling not installed: {e}. Run: pip install scrapling[fetchers]"
        )
    except Exception as e:
        return ToolResult(
            success=False,
            error=f"Search failed: {str(e)}"
        )


def search_duckduckgo(query: str, num_results: int, config: dict) -> ToolResult:
    """Search using DuckDuckGo HTML version"""
    try:
        from scrapling.fetchers import FetcherSession

        # Calculate pages needed (10 results per page)
        pages = (num_results + 9) // 10

        all_results = []

        with FetcherSession(impersonate="chrome") as session:
            for page_num in range(pages):
                offset = page_num * 10

                # POST to DuckDuckGo HTML search
                data = {
                    "q": query,
                    "b": str(offset)
                }

                page = session.post(config["url"], data=data)

                # Extract results
                results = page.css(config["result_selector"])

                for result in results:
                    title_elem = result.css(config["title_selector"])
                    url_elem = result.css(config["url_selector"])
                    snippet_elem = result.css(config["snippet_selector"])

                    if not title_elem:
                        continue

                    title = title_elem[0].text(strip=True) if title_elem else ""

                    # Get URL
                    url = ""
                    if url_elem:
                        url = url_elem[0].attrib.get("href", "")
                        # Clean DuckDuckGo redirect
                        if "uddg=" in url:
                            import urllib.parse
                            parsed = urllib.parse.parse_qs(urllib.parse.urlparse(url).query)
                            url = parsed.get("uddg", [""])[0]

                    snippet = ""
                    if snippet_elem:
                        snippet = snippet_elem[0].text(strip=True)

                    if title and url:
                        all_results.append({
                            "title": title,
                            "url": url,
                            "snippet": snippet
                        })

                if len(all_results) >= num_results:
                    break

        # Trim to requested number
        all_results = all_results[:num_results]

        if not all_results:
            return ToolResult(
                success=True,
                output="No results found",
                data={"query": query, "engine": "duckduckgo", "count": 0}
            )

        # Format output
        output = f"Search results for '{query}' (DuckDuckGo):\n\n"
        for i, r in enumerate(all_results, 1):
            output += f"{i}. {r['title']}\n"
            output += f"   {r['url']}\n"
            if r['snippet']:
                output += f"   {r['snippet'][:200]}...\n"
            output += "\n"

        return ToolResult(
            success=True,
            output=output,
            data={
                "query": query,
                "engine": "duckduckgo",
                "count": len(all_results),
                "results": all_results
            }
        )

    except Exception as e:
        return ToolResult(success=False, error=f"DuckDuckGo search failed: {str(e)}")


def search_google(query: str, num_results: int, locale: str, config: dict) -> ToolResult:
    """Search using Google"""
    try:
        from scrapling.fetchers import StealthyFetcher

        # Build URL with parameters
        params = {
            "q": query,
            "num": min(num_results, 100),  # Google max is 100
            "hl": locale,
        }

        url = config["url"] + "&" + urllib.parse.urlencode(params)

        page = StealthyFetcher.fetch(url, headless=True, solve_cloudflare=False)

        results = page.css(config["result_selector"])
        all_results = []

        for result in results[:num_results]:
            title_elem = result.css(config["title_selector"])
            url_elem = result.css(config["url_selector"])
            snippet_elem = result.css(config["snippet_selector"])

            if not title_elem:
                continue

            title = title_elem[0].text(strip=True) if title_elem else ""

            url = ""
            if url_elem:
                url = url_elem[0].attrib.get("href", "")

            snippet = ""
            if snippet_elem:
                snippet = snippet_elem[0].text(strip=True)

            if title and url and url.startswith("http"):
                all_results.append({
                    "title": title,
                    "url": url,
                    "snippet": snippet
                })

        if not all_results:
            return ToolResult(
                success=True,
                output="No results found",
                data={"query": query, "engine": "google", "count": 0}
            )

        output = f"Search results for '{query}' (Google):\n\n"
        for i, r in enumerate(all_results, 1):
            output += f"{i}. {r['title']}\n"
            output += f"   {r['url']}\n"
            if r['snippet']:
                output += f"   {r['snippet'][:200]}...\n"
            output += "\n"

        return ToolResult(
            success=True,
            output=output,
            data={
                "query": query,
                "engine": "google",
                "count": len(all_results),
                "results": all_results
            }
        )

    except Exception as e:
        return ToolResult(success=False, error=f"Google search failed: {str(e)}")


def search_bing(query: str, num_results: int, config: dict) -> ToolResult:
    """Search using Bing"""
    try:
        from scrapling.fetchers import StealthyFetcher

        params = {
            "q": query,
            "count": min(num_results, 50),
        }

        url = config["url"] + "&" + urllib.parse.urlencode(params)

        page = StealthyFetcher.fetch(url, headless=True, solve_cloudflare=False)

        results = page.css(config["result_selector"])
        all_results = []

        for result in results[:num_results]:
            title_elem = result.css(config["title_selector"])
            snippet_elem = result.css(config["snippet_selector"])

            if not title_elem:
                continue

            title = title_elem[0].text(strip=True) if title_elem else ""
            url = title_elem[0].attrib.get("href", "") if title_elem else ""

            snippet = ""
            if snippet_elem:
                snippet = snippet_elem[0].text(strip=True)

            if title and url and url.startswith("http"):
                all_results.append({
                    "title": title,
                    "url": url,
                    "snippet": snippet
                })

        if not all_results:
            return ToolResult(
                success=True,
                output="No results found",
                data={"query": query, "engine": "bing", "count": 0}
            )

        output = f"Search results for '{query}' (Bing):\n\n"
        for i, r in enumerate(all_results, 1):
            output += f"{i}. {r['title']}\n"
            output += f"   {r['url']}\n"
            if r['snippet']:
                output += f"   {r['snippet'][:200]}...\n"
            output += "\n"

        return ToolResult(
            success=True,
            output=output,
            data={
                "query": query,
                "engine": "bing",
                "count": len(all_results),
                "results": all_results
            }
        )

    except Exception as e:
        return ToolResult(success=False, error=f"Bing search failed: {str(e)}")


if __name__ == "__main__":
    run()
