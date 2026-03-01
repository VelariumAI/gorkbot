#!/usr/bin/env python3
"""
Clean Web Search - Suppresses verbose scrapling output

This provides a clean interface for web search that doesn't flood
the user with scraping activity details.
"""

import sys
import os
import io
import contextlib
import urllib.parse

# Redirect stderr to suppress scrapling's verbose output
stderr_capture = io.StringIO()

def suppress_stderr():
    """Context manager to suppress stderr during scraping"""
    return contextlib.redirect_stderr(stderr_capture)


def search_duckduckgo(query: str, num_results: int = 10) -> dict:
    """Search using DuckDuckGo HTML - quiet version"""
    try:
        with suppress_stderr():
            from scrapling.fetchers import FetcherSession

        with FetcherSession(impersonate="chrome") as session:
            # POST to DuckDuckGo HTML search
            page = session.post(
                'https://html.duckduckgo.com/html/',
                data={'q': query, 'b': '0'}
            )

            results = page.css('.result')
            all_results = []

            for result in results[:num_results]:
                title_elem = result.css('.result__title')
                url_elem = result.css('.result__url')
                snippet_elem = result.css('.result__snippet')

                if not title_elem:
                    continue

                title = title_elem[0].text(strip=True) if title_elem else ""

                # Get URL
                url = ""
                if url_elem:
                    url = url_elem[0].attrib.get("href", "")
                    if "uddg=" in url:
                        parsed = urllib.parse.parse_qs(urllib.parse.urlparse(url).query)
                        url = parsed.get("uddg", [""])[0]

                snippet = ""
                if snippet_elem:
                    snippet = snippet_elem[0].text(strip=True)

                if title and url:
                    all_results.append({
                        "title": title,
                        "url": url,
                        "snippet": snippet[:200] if snippet else ""
                    })

        return {
            "success": True,
            "results": all_results[:num_results],
            "query": query,
            "engine": "duckduckgo"
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


def search_google(query: str, num_results: int = 10) -> dict:
    """Search using Google - quiet version"""
    try:
        with suppress_stderr():
            from scrapling.fetchers import FetcherSession

        params = urllib.parse.urlencode({
            "q": query,
            "num": min(num_results, 100)
        })

        with FetcherSession(impersonate="chrome") as session:
            page = session.get(f"https://www.google.com/search?{params}")

            results = page.css('div.g')
            all_results = []

            for result in results[:num_results]:
                title_elem = result.css('h3')
                url_elem = result.css('a')
                snippet_elem = result.css('div.VwiC3b')

                if not title_elem:
                    continue

                title = title_elem[0].text(strip=True) if title_elem else ""
                url = url_elem[0].attrib.get("href", "") if url_elem else ""
                snippet = snippet_elem[0].text(strip=True) if snippet_elem else ""

                if title and url and url.startswith("http"):
                    all_results.append({
                        "title": title,
                        "url": url,
                        "snippet": snippet[:200] if snippet else ""
                    })

        return {
            "success": True,
            "results": all_results[:num_results],
            "query": query,
            "engine": "google"
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


def search_bing(query: str, num_results: int = 10) -> dict:
    """Search using Bing - quiet version"""
    try:
        with suppress_stderr():
            from scrapling.fetchers import FetcherSession

        params = urllib.parse.urlencode({
            "q": query,
            "count": min(num_results, 50)
        })

        with FetcherSession(impersonate="chrome") as session:
            page = session.get(f"https://www.bing.com/search?{params}")

            results = page.css('.b_algo')
            all_results = []

            for result in results[:num_results]:
                title_elem = result.css('h2 a')
                snippet_elem = result.css('p')

                if not title_elem:
                    continue

                title = title_elem[0].text(strip=True) if title_elem else ""
                url = title_elem[0].attrib.get("href", "") if title_elem else ""
                snippet = snippet_elem[0].text(strip=True) if snippet_elem else ""

                if title and url and url.startswith("http"):
                    all_results.append({
                        "title": title,
                        "url": url,
                        "snippet": snippet[:200] if snippet else ""
                    })

        return {
            "success": True,
            "results": all_results[:num_results],
            "query": query,
            "engine": "bing"
        }

    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


def main():
    """Main entry point - reads JSON from stdin, outputs JSON to stdout"""
    import json

    try:
        input_data = sys.stdin.read()
        if not input_data.strip():
            print(json.dumps({"success": False, "error": "No input"}))
            return

        request = json.loads(input_data)
        query = request.get("query", "")
        engine = request.get("engine", "duckduckgo").lower()
        num_results = request.get("num_results", 10)

        if not query:
            print(json.dumps({"success": False, "error": "No query provided"}))
            return

        if engine == "google":
            result = search_google(query, num_results)
        elif engine == "bing":
            result = search_bing(query, num_results)
        else:
            result = search_duckduckgo(query, num_results)

        # Format as clean output for display
        if result.get("success"):
            output = f"Search results for '{result['query']}' ({result['engine'].title()}):\n\n"
            for i, r in enumerate(result['results'], 1):
                output += f"{i}. {r['title']}\n"
                output += f"   {r['url']}\n"
                if r['snippet']:
                    output += f"   {r['snippet']}...\n"
                output += "\n"
            result["output"] = output

        print(json.dumps(result))

    except Exception as e:
        print(json.dumps({"success": False, "error": str(e)}))


if __name__ == "__main__":
    main()
