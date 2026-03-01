#!/usr/bin/env python3
"""
Scrapling Spider Tool - Full-featured web crawler

Uses Scrapling's Spider framework for large-scale crawling with
concurrency, pause/resume, and proxy rotation support.
"""

import sys
import os
import json
import tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


# We need to define a spider class dynamically
SPIDER_CODE = '''
import asyncio
from scrapling.spiders import Spider, Response
from scrapling.fetchers import FetcherSession, AsyncStealthySession, AsyncDynamicSession

class CrawlSpider(Spider):
    name = "gorkbot_crawl"
    start_urls = {start_urls}
    concurrent_requests = {concurrency}
    max_pages = {max_pages}
    parse_selector = "{parse_selector}"
    items = []

    async def parse(self, response: Response):
        # Extract items if selector provided
        if self.parse_selector:
            for elem in response.css(self.parse_selector):
                self.items{{
                    ".append(html": str(elem),
                    "text": elem.text(strip=True)
                }})

        # Follow links (internal only by default)
        for link in response.css("a::attr(href)").getall():
            if self.follow_external or link.startswith("/") or response.url in link:
                yield response.follow(link)

        # Respect max_pages
        if hasattr(self, 'pages_crawled') and self.pages_crawled >= self.max_pages:
            return

        yield from []

    async def onFinished(self):
        return self.items
'''


@tool(description="Full-featured web crawler with concurrency")
def crawl(
    urls: list,
    max_pages: int = 10,
    concurrency: int = 5,
    parse_selector: str = "",
    output: str = "",
    crawldir: str = "",
    session_type: str = "fast",
    follow_external: bool = False
) -> ToolResult:
    """
    Run a web crawling operation with Scrapling's Spider framework.

    Args:
        urls: List of starting URLs
        max_pages: Maximum pages to crawl
        concurrency: Number of concurrent requests
        parse_selector: CSS selector for items to extract
        output: Output file path (JSON)
        crawldir: Directory for pause/resume checkpoints
        session_type: fast, stealth, or dynamic
        follow_external: Allow following external links

    Returns:
        ToolResult with crawled data
    """
    try:
        from scrapling.spiders import Spider, Response

        # Prepare spider code
        spider_code = SPIDER_CODE.format(
            start_urls=urls,
            max_pages=max_pages,
            concurrency=concurrency,
            parse_selector=parse_selector,
            follow_external=follow_external
        )

        # Create a temporary spider file
        fd, spider_file = tempfile.mkstemp(suffix=".py")
        os.write(fd, spider_code.encode())
        os.close(fd)

        try:
            # Import and run the spider
            import importlib.util
            spec = importlib.util.spec_from_file_location("gorkbot_spider", spider_file)
            module = importlib.util.module_from_spec(spec)

            # Add scrapling imports to module
            exec("""
from scrapling.spiders import Spider, Response
from scrapling.fetchers import FetcherSession
            """, module.__dict__)

            spec.loader.exec_module(module)

            # Create spider instance
            spider = module.CrawlSpider()

            # Set crawldir if provided
            if crawldir:
                spider.crawldir = crawldir

            # Run the spider
            result = spider.start()

            # Get items
            items = result.items

            if not items:
                return ToolResult(
                    success=True,
                    output="No items found",
                    data={
                        "urls_crawled": result.stats.get('pages_crawled', 0),
                        "items_extracted": 0
                    }
                )

            # Prepare output
            output_data = {
                "stats": {
                    "pages_crawled": result.stats.get('pages_crawled', 0),
                    "items_extracted": len(items),
                    "start_urls": urls
                },
                "items": items[:100]  # Limit output
            }

            output_json = json.dumps(output_data, indent=2)

            # Save to file if requested
            if output:
                with open(output, "w") as f:
                    f.write(output_json)

            return ToolResult(
                success=True,
                output=output_json[:100000],
                data=output_data
            )

        finally:
            # Clean up temp file
            if os.path.exists(spider_file):
                os.unlink(spider_file)

    except ImportError as e:
        return ToolResult(
            success=False,
            error=f"Scrapling not installed: {e}. Run: pip install scrapling[fetchers]"
        )
    except Exception as e:
        return ToolResult(
            success=False,
            error=f"Crawl failed: {str(e)}"
        )


if __name__ == "__main__":
    run()
