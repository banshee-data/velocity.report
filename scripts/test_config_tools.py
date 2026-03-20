from __future__ import annotations

import importlib.machinery
import importlib.util
import os
import runpy
import sys
from collections import OrderedDict
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]


def load_script_module(name: str, relative_path: str):
    path = ROOT / relative_path
    loader = importlib.machinery.SourceFileLoader(name, str(path))
    spec = importlib.util.spec_from_loader(name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    loader.exec_module(module)
    return module


def run_script(path: Path, argv: list[str], cwd: Path) -> int:
    old_argv = sys.argv[:]
    old_cwd = Path.cwd()
    try:
        os.chdir(cwd)
        sys.argv = [str(path), *argv]
        with pytest.raises(SystemExit) as exc_info:
            runpy.run_path(str(path), run_name="__main__")
        return int(exc_info.value.code)
    finally:
        os.chdir(old_cwd)
        sys.argv = old_argv


def test_config_order_sync_helpers_and_targets(tmp_path: Path) -> None:
    mod = load_script_module("config_order_sync_test", "scripts/config-order-sync")

    list_json = tmp_path / "list.json"
    list_json.write_text("[1, 2]\n", encoding="utf-8")
    with pytest.raises(ValueError, match="top-level JSON object"):
        mod.load_json_object(list_json)

    invalid_md = tmp_path / "invalid.md"
    invalid_md.write_text("```json\n{bad}\n```\n", encoding="utf-8")
    with pytest.raises(ValueError, match="invalid JSON block"):
        mod.parse_markdown_json_blocks(
            invalid_md.read_text(encoding="utf-8"), invalid_md
        )

    empty_md = tmp_path / "empty.md"
    empty_md.write_text("# no json here\n", encoding="utf-8")
    with pytest.raises(ValueError, match="no parseable"):
        mod.parse_markdown_json_blocks(empty_md.read_text(encoding="utf-8"), empty_md)

    assert (
        mod.normalize_source_path("relative.json", tmp_path)
        == (tmp_path / "relative.json").resolve()
    )
    with pytest.raises(ValueError, match="PATH:STRUCT"):
        mod.parse_go_source("broken")

    go_source = tmp_path / "types.go"
    go_source.write_text(
        "\n".join(
            [
                "package test",
                "type Example struct {",
                '    Alpha int `json:"alpha"`',
                '    Skip int `json:"-"`',
                '    Beta int `json:"beta"`',
                '    Dup int `json:"alpha"`',
                "}",
            ]
        )
        + "\n",
        encoding="utf-8",
    )
    assert mod.parse_go_source(f"{go_source}:Example") == (str(go_source), "Example")
    assert mod.extract_keys_from_go_struct(go_source, "Example") == ["alpha", "beta"]
    with pytest.raises(ValueError, match="struct Missing not found"):
        mod.extract_keys_from_go_struct(go_source, "Missing")

    canonical = OrderedDict(
        [
            ("_comment", "keep"),
            ("version", 1),
            ("l3", OrderedDict([("alpha", 1), ("beta", 2)])),
        ]
    )
    existing = OrderedDict(
        [
            ("_comment", "keep"),
            ("l3", OrderedDict([("beta", 22)])),
            ("version", 1),
            ("extra", True),
        ]
    )

    assert mod.flatten_leaf_paths(canonical) == ["version", "l3.alpha", "l3.beta"]
    missing, extra, order_ok = mod.nested_key_status(existing, canonical)
    assert missing == ["l3.alpha"]
    assert extra == ["extra"]
    assert order_ok is False

    reordered, still_missing = mod.reorder_object(
        existing, canonical, fill_missing=True
    )
    assert still_missing == []
    assert list(reordered.keys()) == ["_comment", "version", "l3", "extra"]
    assert reordered["l3"]["alpha"] == 1
    assert reordered["l3"]["beta"] == 22

    markdown = tmp_path / "target.md"
    markdown.write_text(
        "\n".join(
            [
                "# Example",
                "```json",
                '{"unrelated":{"value":1}}',
                "```",
                "```json",
                '{"l3":{"beta":22},"version":1}',
                "```",
            ]
        )
        + "\n",
        encoding="utf-8",
    )
    blocks = mod.parse_markdown_json_blocks(
        markdown.read_text(encoding="utf-8"), markdown
    )
    best = mod.choose_best_markdown_block(blocks, canonical)
    assert list(best.obj.keys()) == ["l3", "version"]

    target_json = tmp_path / "target.json"
    target_json.write_text(
        '{"l3":{"beta":22},"version":1,"extra":true}\n', encoding="utf-8"
    )
    ok, changed, msg = mod.apply_json_target(target_json, canonical, check_only=True)
    assert (ok, changed) == (False, False)
    assert "missing keys" in msg
    ok, changed, msg = mod.apply_json_target(target_json, canonical, check_only=False)
    assert (ok, changed) == (True, True)
    assert msg.startswith("[SYNC]")
    synced = mod.load_json_object(target_json)
    assert list(synced["l3"].keys()) == ["alpha", "beta"]

    ok, changed, msg = mod.apply_markdown_target(markdown, canonical, check_only=False)
    assert (ok, changed) == (True, True)
    assert msg.startswith("[SYNC]")
    synced_blocks = mod.parse_markdown_json_blocks(
        markdown.read_text(encoding="utf-8"), markdown
    )
    assert list(synced_blocks[1].obj["l3"].keys()) == ["alpha", "beta"]


def test_config_order_sync_main_and___main__(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    mod = load_script_module("config_order_sync_main_test", "scripts/config-order-sync")

    config_dir = tmp_path / "config"
    config_dir.mkdir()
    canonical_json = config_dir / "canonical.json"
    canonical_json.write_text(
        '{"version":1,"l3":{"alpha":1,"beta":2}}\n', encoding="utf-8"
    )
    target_json = config_dir / "tuning.sample.json"
    target_json.write_text('{"l3":{"beta":2},"version":1}\n', encoding="utf-8")
    target_md = config_dir / "README.md"
    target_md.write_text(
        '```json\n{"l3":{"beta":2},"version":1}\n```\n', encoding="utf-8"
    )
    invalid_md = config_dir / "NOTES.md"
    invalid_md.write_text("# missing json block\n", encoding="utf-8")

    monkeypatch.chdir(tmp_path)

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-json",
            "config/canonical.json",
            "--check",
        ],
    )
    assert mod.main() == 2

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-json",
            "config/canonical.json",
            "--md-target",
            "config/NOTES.md",
        ],
    )
    assert mod.main() == 1

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-json",
            "config/canonical.json",
            "--discover",
            "--check",
        ],
    )
    assert mod.main() == 1

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-json",
            "config/canonical.json",
            "--discover",
        ],
    )
    assert mod.main() == 0
    assert "updated 2 file(s)" in capsys.readouterr().out

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-json",
            "config/canonical.json",
            "--discover",
            "--check",
        ],
    )
    assert mod.main() == 0
    assert "config key order check passed" in capsys.readouterr().out

    flat_go = tmp_path / "flat.go"
    flat_go.write_text(
        "\n".join(
            [
                "package test",
                "type Flat struct {",
                '    Alpha int `json:"alpha"`',
                '    Beta int `json:"beta"`',
                "}",
            ]
        )
        + "\n",
        encoding="utf-8",
    )
    flat_target = config_dir / "flat.json"
    flat_target.write_text('{"beta":2,"alpha":1}\n', encoding="utf-8")

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-go-struct",
            f"{flat_go}:Flat",
            "--json-target",
            "config/flat.json",
        ],
    )
    assert mod.main() == 0

    canonical_md = config_dir / "canonical.md"
    canonical_md.write_text(
        '```json\n{"version":1,"l3":{"alpha":1,"beta":2}}\n```\n', encoding="utf-8"
    )
    md_target = config_dir / "from-md.json"
    md_target.write_text('{"l3":{"beta":2,"alpha":1},"version":1}\n', encoding="utf-8")
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "config-order-sync",
            "--main-md",
            "config/canonical.md",
            "--json-target",
            "config/from-md.json",
            "--check",
        ],
    )
    assert mod.main() == 1

    exit_code = run_script(
        ROOT / "scripts/config-order-sync",
        ["--main-json", "config/canonical.json", "--discover", "--check"],
        tmp_path,
    )
    assert exit_code == 0


def test_readme_maths_helpers_and_main(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    mod = load_script_module("readme_maths_check_test", "scripts/readme-maths-check")

    assert (
        mod.is_valid_key("l3.ema_baseline_v1.noise_relative", allow_private=False)
        is True
    )
    assert mod.is_valid_key("_private", allow_private=False) is False
    assert mod.is_valid_key("_private", allow_private=True) is True
    assert mod.is_valid_key("Bad-Key", allow_private=True) is False
    assert mod.unique_preserve_order(["a", "b", "a"]) == ["a", "b"]

    maths_md = tmp_path / "README.maths.md"
    maths_md.write_text("- `version`\n- `l3.alpha`\n- `_private`\n", encoding="utf-8")
    assert mod.extract_maths_keys(maths_md) == ["version", "l3.alpha", "_private"]

    empty_maths = tmp_path / "EMPTY.maths.md"
    empty_maths.write_text("plain text\n", encoding="utf-8")
    with pytest.raises(ValueError, match="no config paths found"):
        mod.extract_maths_keys(empty_maths)

    assert mod.flatten_leaf_paths({"version": 1, "_note": "x", "l3": {"alpha": 1}}) == [
        "version",
        "l3.alpha",
    ]

    bad_doc = tmp_path / "bad.md"
    bad_doc.write_text("```json\n{bad}\n```\n", encoding="utf-8")
    with pytest.raises(ValueError, match="invalid JSON block"):
        mod.extract_json_blocks(bad_doc)

    docs_dir = tmp_path / "docs"
    docs_dir.mkdir()
    one = docs_dir / "one.md"
    two = docs_dir / "two.md"
    one.write_text("x", encoding="utf-8")
    two.write_text("y", encoding="utf-8")
    expanded = mod.expand_globs(tmp_path, ["docs/*.md", str(two)])
    assert expanded == [one.resolve(), two.resolve()]

    assert mod.compare_key_sets("match", ["a", "b"], ["b", "a"]) is True
    assert mod.compare_key_sets("mismatch", ["a"], ["b"]) is False
    compare_output = capsys.readouterr().out
    assert "[FAIL] mismatch: key mismatch" in compare_output

    config_dir = tmp_path / "config"
    config_dir.mkdir()
    maths_path = config_dir / "README.maths.md"
    maths_path.write_text("- `version`\n- `l3.alpha`\n", encoding="utf-8")
    readme_path = config_dir / "README.md"
    readme_path.write_text(
        '```json\n{"version":1,"l3":{"alpha":1}}\n```\n', encoding="utf-8"
    )
    extra_doc = docs_dir / "extra.md"
    extra_doc.write_text(
        '```json\n{"version":1,"l3":{"alpha":1}}\n```\n', encoding="utf-8"
    )
    tuning_json = config_dir / "tuning.defaults.json"
    tuning_json.write_text('{"version":1,"l3":{"alpha":1}}\n', encoding="utf-8")

    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "readme-maths-check",
            "--doc-glob",
            "docs/extra.md",
        ],
    )
    assert mod.main() == 0
    assert "README maths source checks passed" in capsys.readouterr().out

    no_tuning_root = tmp_path / "no-tuning"
    (no_tuning_root / "config").mkdir(parents=True)
    (no_tuning_root / "config/README.maths.md").write_text(
        "- `version`\n", encoding="utf-8"
    )
    (no_tuning_root / "config/README.md").write_text(
        '```json\n{"version":1}\n```\n', encoding="utf-8"
    )
    monkeypatch.chdir(no_tuning_root)
    monkeypatch.setattr(sys, "argv", ["readme-maths-check"])
    assert mod.main() == 1

    missing_required_root = tmp_path / "missing-required"
    (missing_required_root / "config").mkdir(parents=True)
    (missing_required_root / "config/README.maths.md").write_text(
        "- `version`\n", encoding="utf-8"
    )
    (missing_required_root / "config/README.md").write_text(
        '```json\n{"l3":{"alpha":1}}\n```\n', encoding="utf-8"
    )
    (missing_required_root / "config/tuning.defaults.json").write_text(
        '{"version":1}\n', encoding="utf-8"
    )
    monkeypatch.chdir(missing_required_root)
    monkeypatch.setattr(sys, "argv", ["readme-maths-check"])
    assert mod.main() == 1

    top_level_list_root = tmp_path / "top-level-list"
    (top_level_list_root / "config").mkdir(parents=True)
    (top_level_list_root / "config/README.maths.md").write_text(
        "- `version`\n", encoding="utf-8"
    )
    (top_level_list_root / "config/README.md").write_text(
        '```json\n{"version":1}\n```\n', encoding="utf-8"
    )
    (top_level_list_root / "config/tuning.list.json").write_text(
        "[1, 2]\n", encoding="utf-8"
    )
    monkeypatch.chdir(top_level_list_root)
    monkeypatch.setattr(
        sys,
        "argv",
        ["readme-maths-check", "--tuning-json-glob", "config/tuning.list.json"],
    )
    assert mod.main() == 1

    invalid_json_root = tmp_path / "invalid-json"
    (invalid_json_root / "config").mkdir(parents=True)
    (invalid_json_root / "config/README.maths.md").write_text(
        "- `version`\n", encoding="utf-8"
    )
    (invalid_json_root / "config/README.md").write_text(
        '```json\n{"version":1}\n```\n', encoding="utf-8"
    )
    (invalid_json_root / "config/tuning.invalid.json").write_text(
        "{bad}\n", encoding="utf-8"
    )
    monkeypatch.chdir(invalid_json_root)
    monkeypatch.setattr(
        sys,
        "argv",
        ["readme-maths-check", "--tuning-json-glob", "config/tuning.invalid.json"],
    )
    assert mod.main() == 1

    exit_code = run_script(ROOT / "scripts/readme-maths-check", [], tmp_path)
    assert exit_code == 0
