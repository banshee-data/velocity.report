"""Focused tests for the embedded offline-docs route verifier."""

from __future__ import annotations

import contextlib
import importlib.util
from pathlib import Path
import sys
from urllib.error import URLError

import pytest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "verify-embedded-docs-server.py"
SPEC = importlib.util.spec_from_file_location("verify_embedded_docs_server", SCRIPT)
assert SPEC and SPEC.loader
mod = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mod)


def test_links_marks_only_the_deliberate_app_surface(monkeypatch: pytest.MonkeyPatch) -> None:
    links = mod.Links()
    links.feed('<a href="docs/">docs</a><a href="/public_html/" data-docs-app-surface>home</a>')
    assert links.hrefs == [("docs/", False), ("/public_html/", True)]
    assert mod.NoRedirect().redirect_request() is None
    class Socket:
        def __enter__(self):
            return self

        def __exit__(self, *_args):
            return None

        def bind(self, _address):
            pass

        def getsockname(self):
            return ("127.0.0.1", 12345)

    monkeypatch.setattr(mod.socket, "socket", lambda *_: Socket())
    assert mod.available_port() == 12345


def test_page_url_handles_root_indexes_and_files(tmp_path: Path) -> None:
    assert mod.page_url(tmp_path, tmp_path / "index.html") == "/docs/"
    assert mod.page_url(tmp_path, tmp_path / "docs" / "index.html") == "/docs/docs/"
    assert mod.page_url(tmp_path, tmp_path / "asset.html") == "/docs/asset.html"


def test_request_closes_response() -> None:
    class Response:
        status = 200

        def read(self) -> bytes:
            return b"ok"

        def close(self) -> None:
            self.closed = True

    class Opener:
        def __init__(self) -> None:
            self.response = Response()

        def open(self, request, timeout):
            assert request.get_header("User-agent") == "velocity-docs-check"
            assert timeout == 5
            return self.response

    opener = Opener()
    assert mod.request(opener, "http://offline.local/docs/") == (200, b"ok")
    assert opener.response.closed is True


def test_wait_for_server_success_exit_and_timeout(monkeypatch: pytest.MonkeyPatch) -> None:
    class Process:
        def __init__(self, return_code=None) -> None:
            self.returncode = return_code

        def poll(self):
            return self.returncode

    monkeypatch.setattr(mod, "request", lambda *_: (200, b'<div class="shell">'))
    mod.wait_for_server(object(), "http://offline.local/docs/", Process())

    with pytest.raises(RuntimeError, match="exited with 7"):
        mod.wait_for_server(object(), "http://offline.local/docs/", Process(7))

    ticks = iter([0.0, 31.0])
    monkeypatch.setattr(mod.time, "monotonic", lambda: next(ticks))
    with pytest.raises(RuntimeError, match="did not become ready"):
        mod.wait_for_server(object(), "http://offline.local/docs/", Process())

    ticks = iter([0.0, 1.0, 31.0])
    monkeypatch.setattr(mod.time, "monotonic", lambda: next(ticks))
    monkeypatch.setattr(mod, "request", lambda *_: (_ for _ in ()).throw(URLError("down")))
    monkeypatch.setattr(mod.time, "sleep", lambda *_: None)
    with pytest.raises(RuntimeError, match="did not become ready"):
        mod.wait_for_server(object(), "http://offline.local/docs/", Process())


class _Process:
    def __init__(self, timeout: bool = False) -> None:
        self.returncode = None
        self.timeout = timeout

    def send_signal(self, _signal) -> None:
        pass

    def wait(self, timeout=None) -> None:
        if self.timeout:
            self.timeout = False
            raise mod.subprocess.TimeoutExpired("velocity", timeout)

    def kill(self) -> None:
        pass


def _run_main(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path, responses: list[object], process=None
) -> None:
    binary = tmp_path / "velocity"
    binary.write_text("binary")
    site = tmp_path / "site"
    site.mkdir()
    (site / "index.html").write_text(
        '<meta name="velocity-docs-page" content="rendered">'
    )
    monkeypatch.setattr(sys, "argv", ["verify", str(binary), "--site-root", str(site)])
    monkeypatch.setattr(mod, "available_port", lambda: 19001)
    monkeypatch.setattr(mod, "build_opener", lambda *_: object())
    monkeypatch.setattr(mod, "wait_for_server", lambda *_: None)
    monkeypatch.setattr(mod.subprocess, "Popen", lambda *_args, **_kwargs: process or _Process())

    def request(_opener, _url):
        response = responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response

    monkeypatch.setattr(mod, "request", request)
    mod.main()


def test_main_verifies_pages_and_all_same_origin_docs_links(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    page = b'<div class="shell"><a href="child/">child</a><a href="https://github.com/x">source</a><a href="/public_html/" data-docs-app-surface>home</a>'
    _run_main(monkeypatch, tmp_path, [(200, page), (200, b"")])
    assert "1 pages, 1 internal links" in capsys.readouterr().out


def test_main_collects_broken_link_failure_paths(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    page = b'<div class="shell"><a href="mailto:x">mail</a><a href="/outside/">outside</a><a href="bad/">bad</a><a href="bad/">again</a>'
    with pytest.raises(RuntimeError, match="broken docs link"):
        _run_main(monkeypatch, tmp_path, [(200, page), (404, b"")])


def test_main_collects_link_errors_and_kills_a_stuck_process(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    page = b'<div class="shell"><a href="bad/">bad</a>'
    with pytest.raises(RuntimeError, match="broken docs link"):
        _run_main(monkeypatch, tmp_path, [(200, page), URLError("down")], _Process(timeout=True))


def test_main_rejects_empty_rendered_site(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    binary = tmp_path / "velocity"
    binary.write_text("binary")
    site = tmp_path / "site"
    site.mkdir()
    monkeypatch.setattr(sys, "argv", ["verify", str(binary), "--site-root", str(site)])
    with pytest.raises(SystemExit):
        mod.main()


@pytest.mark.parametrize("response", [URLError("down"), (500, b"no shell"), (200, b"no shell")])
def test_main_reports_page_failures(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path, response: object
) -> None:
    with pytest.raises(RuntimeError, match="embedded docs verification failed"):
        _run_main(monkeypatch, tmp_path, [response])


@pytest.mark.parametrize("binary_exists,site_exists", [(False, True), (True, False)])
def test_main_rejects_missing_inputs(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
    binary_exists: bool,
    site_exists: bool,
) -> None:
    binary = tmp_path / "velocity"
    site = tmp_path / "site"
    if binary_exists:
        binary.write_text("binary")
    if site_exists:
        site.mkdir()
    monkeypatch.setattr(sys, "argv", ["verify", str(binary), "--site-root", str(site)])
    with pytest.raises(SystemExit):
        mod.main()
