from __future__ import annotations

import importlib.machinery
import importlib.util
from pathlib import Path
import sys
from urllib.error import HTTPError


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "spider-docs-404s.py"


def load_module():
    loader = importlib.machinery.SourceFileLoader("spider_docs_404s", str(SCRIPT))
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[loader.name] = module
    spec.loader.exec_module(module)
    return module


class FakeResponse:
    def __init__(self, status: int, body: bytes) -> None:
        self.status = status
        self.body = body

    def read(self) -> bytes:
        return self.body

    def close(self) -> None:
        pass


class FakeOpener:
    def __init__(self, pages: dict[str, tuple[int, bytes]]) -> None:
        self.pages = pages

    def open(self, request, timeout: int):
        status, body = self.pages[request.full_url]
        if status == 404:
            raise HTTPError(request.full_url, status, "Not Found", {}, None)
        return FakeResponse(status, body)


def test_crawl_reports_unique_404s_grouped_by_parent_route():
    spider = load_module()
    root = "http://velocity.local/docs/"
    pages = {
        root: (
            200,
            b'<a href="good/">good</a><a href="dead-one/">dead</a>'
            b'<a href="bad/dead-two/">dead</a><a href="https://example.com/">external</a>',
        ),
        "http://velocity.local/docs/good/": (
            200,
            b'<a href="missing/#fragment">missing</a><a href="/app/">app</a>',
        ),
        "http://velocity.local/docs/dead-one/": (404, b""),
        "http://velocity.local/docs/bad/dead-two/": (404, b""),
        "http://velocity.local/docs/good/missing/": (404, b""),
    }

    result = spider.crawl(root, FakeOpener(pages))

    assert result.pages_checked == 5
    assert result.internal_links_found == 4
    assert result.not_found == (
        "http://velocity.local/docs/bad/dead-two/",
        "http://velocity.local/docs/dead-one/",
        "http://velocity.local/docs/good/missing/",
    )
    assert spider.parent_route(result.not_found[0]) == "/docs/bad/"
    assert spider.parent_route(result.not_found[1]) == "/docs/"
    assert spider.parent_route(result.not_found[2]) == "/docs/good/"


def test_internal_docs_url_rejects_other_origins_and_non_docs_paths():
    spider = load_module()
    root = "http://velocity.local/docs/"

    assert (
        spider.internal_docs_url("nested/#fragment", root, root)
        == "http://velocity.local/docs/nested/"
    )
    assert spider.internal_docs_url("/app/", root, root) is None
    assert spider.internal_docs_url("https://example.com/docs/", root, root) is None
