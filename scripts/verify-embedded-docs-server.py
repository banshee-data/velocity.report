#!/usr/bin/env python3
"""Verify every generated offline-docs page through an embedded binary."""

from __future__ import annotations

import argparse
import contextlib
import os
from html.parser import HTMLParser
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener


class Links(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.hrefs: list[tuple[str, bool]] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag != "a":
            return
        attributes = dict(attrs)
        href = attributes.get("href")
        if href:
            self.hrefs.append((href, "data-docs-internal" in attributes))


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *_: object, **__: object) -> None:
        return None


def page_url(site_root: Path, page: Path) -> str:
    relative = page.relative_to(site_root).as_posix()
    if relative == "index.html":
        return "/docs/"
    if relative.endswith("/index.html"):
        return f"/docs/{relative[: -len('index.html')]}"
    return f"/docs/{relative}"


def available_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.bind(("127.0.0.1", 0))
        return int(probe.getsockname()[1])


def request(opener: object, url: str) -> tuple[int, bytes]:
    response = opener.open(
        Request(url, headers={"User-Agent": "velocity-docs-check"}), timeout=5
    )
    with contextlib.closing(response):
        return response.status, response.read()


def wait_for_server(opener: object, root_url: str, process: subprocess.Popen[bytes]) -> None:
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"embedded docs server exited with {process.returncode}")
        try:
            status, body = request(opener, root_url)
            if status == 200 and b'class="shell"' in body:
                return
        except (HTTPError, URLError, TimeoutError):
            pass
        time.sleep(0.2)
    raise RuntimeError("embedded docs server did not become ready within 30 seconds")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--site-root", type=Path, default=Path("docs_html/_site"))
    args = parser.parse_args()
    binary = args.binary.resolve()
    site_root = args.site_root.resolve()
    if not binary.is_file():
        parser.error(f"binary does not exist: {binary}")
    if not site_root.is_dir():
        parser.error(f"offline docs site does not exist: {site_root}")

    pages = sorted(
        page
        for page in site_root.rglob("*.html")
        if b'name="velocity-docs-page" content="rendered"' in page.read_bytes()
    )
    if not pages:
        parser.error(f"offline docs site has no HTML pages: {site_root}")

    port = available_port()
    root_url = f"http://127.0.0.1:{port}/docs/"
    opener = build_opener(NoRedirect)
    with tempfile.TemporaryDirectory(prefix="velocity-docs-server-") as temporary:
        process = subprocess.Popen(
            [
                os.fspath(binary),
                "serve",
                "--disable-radar",
                "--listen",
                f"127.0.0.1:{port}",
                "--db-path",
                os.fspath(Path(temporary) / "sensor_data.db"),
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
        )
        try:
            wait_for_server(opener, root_url, process)
            failures: list[str] = []
            checked_links: set[str] = set()
            for page in pages:
                relative_url = page_url(site_root, page).removeprefix("/docs/")
                url = urljoin(root_url, relative_url)
                try:
                    status, body = request(opener, url)
                except (HTTPError, URLError, TimeoutError) as error:
                    failures.append(f"{page.relative_to(site_root)}: {error}")
                    continue
                if status != 200 or b'class="shell"' not in body:
                    failures.append(
                        f"{page.relative_to(site_root)}: status={status}, missing docs shell"
                    )
                    continue
                page_links = Links()
                page_links.feed(body.decode("utf-8", errors="replace"))
                for href, is_docs_internal in page_links.hrefs:
                    if not is_docs_internal:
                        continue
                    target = urljoin(url, href)
                    parsed = urlsplit(target)
                    if parsed.scheme not in {"http", "https"}:
                        continue
                    if parsed.netloc != urlsplit(root_url).netloc:
                        continue
                    if not parsed.path.startswith("/docs/"):
                        failures.append(
                            f"{page.relative_to(site_root)}: internal link escapes docs: {href}"
                        )
                        continue
                    if target in checked_links:
                        continue
                    checked_links.add(target)
                    try:
                        status, _ = request(opener, target)
                    except (HTTPError, URLError, TimeoutError) as error:
                        failures.append(
                            f"{page.relative_to(site_root)}: broken link {href}: {error}"
                        )
                    else:
                        if status != 200:
                            failures.append(
                                f"{page.relative_to(site_root)}: broken link {href}: status={status}"
                            )
            if failures:
                raise RuntimeError(
                    "embedded docs verification failed:\n  - " + "\n  - ".join(failures)
                )
            print(
                f"✓ Embedded docs server verified ({len(pages)} pages, "
                f"{len(checked_links)} internal links)"
            )
        finally:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


if __name__ == "__main__":
    main()
