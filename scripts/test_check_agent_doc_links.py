from __future__ import annotations

import importlib.machinery
import importlib.util
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "check-agent-doc-links.py"


def load_check():
    loader = importlib.machinery.SourceFileLoader(
        "check_agent_doc_links_test", str(SCRIPT)
    )
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[loader.name] = module
    loader.exec_module(module)
    return module


check = load_check()


def rules(text: str) -> list[str]:
    return [name for _, name, _ in check.scan_text(text)]


def test_each_agent_store_is_found():
    cases = {
        "See [the brief](https://claude.ai/artifact/AbC123).": "claude-artifact",
        "Viewer: https://claude.ai/code/artifact/0f1e2d3c": "claude-artifact",
        "Notes in https://claude.ai/chat/1234-abcd": "claude-chat",
        "Draft at https://chatgpt.com/canvas/shared/abc": "chatgpt",
        "From https://chat.openai.com/share/abc": "chatgpt",
        "Gem: https://gemini.google.com/app/abc": "gemini",
        "Copilot said https://github.com/copilot/c/abc": "copilot-chat",
        "Handover: https://chatgpt.com/space/page_268b6582": "chatgpt",
        "Exploration: https://claude.ai/design/p/98edced9": "claude-design",
    }
    for text, rule in cases.items():
        assert rules(text) == [rule], text


def test_repository_and_ordinary_links_pass():
    for text in (
        "See [the record](sf-city-map.md).",
        "PR https://github.com/banshee-data/velocity.report/pull/1085",
        "Docs at https://docs.anthropic.com/en/docs/claude-code",
        "A claude.ai account is needed to use the app.",
    ):
        assert rules(text) == [], text


def test_ignore_marker_and_code_fences_pass():
    text = "\n".join(
        (
            "Moved from https://claude.ai/artifact/AbC123 <!-- agent-doc-link: ignore -->",
            "```",
            "https://claude.ai/artifact/AbC123",
            "```",
        )
    )
    assert rules(text) == []


def test_repository_is_clean():
    result = subprocess.run(
        [sys.executable, str(SCRIPT)], cwd=ROOT, capture_output=True, text=True
    )
    assert result.returncode == 0, result.stdout


def test_nested_fences_keep_their_block():
    # A four-backtick block holding a three-backtick example: the example is code.
    nested = "\n".join(
        (
            "````markdown",
            "```",
            "https://claude.ai/artifact/AbC123",
            "```",
            "````",
            "After the block: https://claude.ai/artifact/XyZ789",
        )
    )
    assert check.scan_text(nested) == [(6, "claude-artifact", "claude.ai/artifact/")]


def test_other_fence_character_does_not_close_the_block():
    # A ~~~ line inside a backtick block is content, so the block still ends at its own fence.
    mixed = "\n".join(
        (
            "```",
            "~~~",
            "```",
            "Prose again: https://chatgpt.com/share/abc",
        )
    )
    assert rules(mixed) == ["chatgpt"]
