from __future__ import annotations

import importlib.machinery
import importlib.util
import sys

ROOT = __import__("pathlib").Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "check-backlog-format.py"


def load_script_module(name: str):
    loader = importlib.machinery.SourceFileLoader(name, str(SCRIPT))
    spec = importlib.util.spec_from_loader(name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    loader.exec_module(module)
    return module


MOD = load_script_module("check_backlog_format_test")


def test_valid_backlog_line_passes() -> None:
    data = """
### v0.5.5 - Physical body geometry (055)
- **P0** Solid-body geometry acceptance: [plan](plans/test.md) `M` {math} `S-GEO`
- **P1** Another item: [plan](plans/test.md) `S` `S-OBS`
""".strip()
    assert MOD.validate(data) == []


def test_unknown_workstream_is_reported() -> None:
    data = """
### v0.5.5 - Physical body geometry (055)
- **P0** Example item: [plan](plans/test.md) `M` `S-BOGUS`
""".strip()
    assert MOD.validate(data) == ["line 2: unknown workstream S-BOGUS"]


def test_missing_size_is_reported() -> None:
    data = """
### v0.5.5 - Physical body geometry (055)
- **P0** Example item: [plan](plans/test.md) `S-GEO`
""".strip()
    assert MOD.validate(data) == ["line 2: missing original size before workstream"]


def test_per_release_items_cap_is_enforced() -> None:
    data = "\n".join(
        ["### v0.5.5 - Physical body geometry (055)"]
        + [
            f"- **P{i % 2}** Example item {i}: [plan](plans/test.md) `M` `S-GEO`"
            for i in range(13)
        ]
    )
    assert MOD.validate(data) == ["v0.5.5: 13 outstanding items, maximum 12"]


def test_line_without_priority_is_rejected() -> None:
    data = """
### v0.5.5 - Physical body geometry (055)
- Example item: [plan](plans/test.md) `M` `S-GEO`
""".strip()
    assert MOD.validate(data) == ["line 2: missing priority directly after optional PR"]
