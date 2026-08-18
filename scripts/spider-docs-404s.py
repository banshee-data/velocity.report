#!/usr/bin/env python3
"""Crawl a Velocity docs site and report same-origin /docs/ 404s by route."""

from __future__ import annotations

import argparse
import contextlib
from collections import defaultdict, deque
from dataclasses import dataclass
from html.parser import HTMLParser
import sys
from typing import Protocol
from urllib.error import HTTPError, URLError
from urllib.parse import urldefrag, urljoin, urlsplit, urlunsplit
from urllib.request import Request, build_opener


DEFAULT_ROOT_URL = "http://velocity.local/docs/"


class Links(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.hrefs: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag != "a":
            return
        href = dict(attrs).get("href")
        if href:
            self.hrefs.append(href)


class Response(Protocol):
    status: int

    def read(self) -> bytes: ...

    def close(self) -> None: ...


class Opener(Protocol):
    def open(self, request: Request, timeout: int) -> Response: ...


@dataclass(frozen=True)
class CrawlResult:
    pages_checked: int
    internal_links_found: int
    not_found: tuple[str, ...]
    request_errors: tuple[str, ...]


def docs_scope(root_url: str) -> tuple[str, str, str]:
    parsed = urlsplit(root_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError(f"docs root must be an absolute HTTP(S) URL: {root_url}")
    path = parsed.path.rstrip("/") + "/"
    if not path.startswith("/docs/"):
        raise ValueError(f"docs root must be under /docs/: {root_url}")
    return parsed.scheme, parsed.netloc, path


def internal_docs_url(href: str, current_url: str, root_url: str) -> str | None:
    scheme, netloc, scope_path = docs_scope(root_url)
    target, _fragment = urldefrag(urljoin(current_url, href))
    parsed = urlsplit(target)
    if parsed.scheme != scheme or parsed.netloc != netloc:
        return None
    if not parsed.path.startswith(scope_path):
        return None
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, parsed.query, ""))


def parent_route(url: str) -> str:
    """Return the route grouping for a URL, dropping its final path segment."""
    segments = [segment for segment in urlsplit(url).path.split("/") if segment]
    if len(segments) <= 1:
        return "/"
    return "/" + "/".join(segments[:-1]) + "/"


def fetch(opener: Opener, url: str) -> tuple[int, bytes]:
    try:
        response = opener.open(
            Request(url, headers={"User-Agent": "velocity-docs-spider"}), timeout=10
        )
    except HTTPError as error:
        return error.code, b""
    with contextlib.closing(response):
        return response.status, response.read()


def crawl(root_url: str, opener: Opener) -> CrawlResult:
    root_scheme, root_netloc, root_path = docs_scope(root_url)
    canonical_root = urlunsplit((root_scheme, root_netloc, root_path, "", ""))
    pending = deque([canonical_root])
    seen: set[str] = set()
    not_found: set[str] = set()
    request_errors: list[str] = []
    internal_links_found = 0

    while pending:
        url = pending.popleft()
        if url in seen:
            continue
        seen.add(url)
        try:
            status, body = fetch(opener, url)
        except (URLError, TimeoutError, OSError) as error:
            request_errors.append(f"{url}: {error}")
            continue
        if status == 404:
            not_found.add(url)
            continue
        if status != 200:
            request_errors.append(f"{url}: status={status}")
            continue

        links = Links()
        links.feed(body.decode("utf-8", errors="replace"))
        for href in links.hrefs:
            target = internal_docs_url(href, url, canonical_root)
            if target is None:
                continue
            internal_links_found += 1
            if target not in seen:
                pending.append(target)

    return CrawlResult(
        pages_checked=len(seen),
        internal_links_found=internal_links_found,
        not_found=tuple(sorted(not_found)),
        request_errors=tuple(sorted(request_errors)),
    )


def print_report(root_url: str, result: CrawlResult) -> None:
    print(f"Docs root: {root_url}")
    print(f"URLs checked: {result.pages_checked}")
    print(f"Internal docs links found: {result.internal_links_found}")
    print(f"404 URLs: {len(result.not_found)}")

    grouped: dict[str, list[str]] = defaultdict(list)
    for url in result.not_found:
        grouped[parent_route(url)].append(url)
    if grouped:
        print("\n404 URLs by parent route:")
        for route, urls in sorted(grouped.items()):
            print(f"  {len(urls):4d}  {route}")
            for url in urls:
                print(f"        {url}")

    if result.request_errors:
        print("\nRequest errors (not counted as 404s):", file=sys.stderr)
        for error in result.request_errors:
            print(f"  {error}", file=sys.stderr)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "root_url",
        nargs="?",
        default=DEFAULT_ROOT_URL,
        help=f"docs root to crawl (default: {DEFAULT_ROOT_URL})",
    )
    args = parser.parse_args()
    try:
        result = crawl(args.root_url, build_opener())
    except ValueError as error:
        parser.error(str(error))
    print_report(args.root_url, result)
    return 1 if result.not_found or result.request_errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
