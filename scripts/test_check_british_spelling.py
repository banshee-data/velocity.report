from __future__ import annotations

import importlib.machinery
import importlib.util
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_check():
    path = ROOT / "scripts" / "check-british-spelling.py"
    loader = importlib.machinery.SourceFileLoader(
        "check_british_spelling_test", str(path)
    )
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[loader.name] = module
    loader.exec_module(module)
    return module


check = load_check()


def test_american_spelling_in_prose_is_reported(tmp_path):
    doc = tmp_path / "doc.md"
    doc.write_text("Driver behavior at the kerb.\n", encoding="utf-8")
    changes = check.process_file(str(doc))
    assert [new for _, _, new in changes] == ["Driver behaviour at the kerb."]


def test_a_marked_line_keeps_a_quoted_title(tmp_path):
    doc = tmp_path / "doc.md"
    doc.write_text(
        "_Riskier behavior in human drivers_ (2026) <!-- spelling-ignore -->\n",
        encoding="utf-8",
    )
    assert check.process_file(str(doc)) == []
