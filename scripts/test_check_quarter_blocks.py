from __future__ import annotations

import importlib.machinery
import importlib.util
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "check-quarter-blocks.py"


def load_script_module(name: str, relative_path: str):
    path = ROOT / relative_path
    loader = importlib.machinery.SourceFileLoader(name, str(path))
    spec = importlib.util.spec_from_loader(name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    loader.exec_module(module)
    return module


def check_module():
    return load_script_module(
        "check_quarter_blocks_test", "scripts/check-quarter-blocks.py"
    )


# Quarter blocks are built from code points rather than pasted in, so this test
# file is itself free of them and is scanned like any other source file.
QUARTER_BLOCK = chr(0x2596)
LAST_QUARTER_BLOCK = chr(0x259F)

EM_DASH = "—"
EN_DASH = "–"
LEFT_QUOTE = "“"
RIGHT_QUOTE = "”"
FULL_BLOCK = "█"
LOWER_HALF_BLOCK = "▄"
LIGHT_SHADE = "░"
RIGHT_HALF_BLOCK = "▐"


def test_em_dash_is_not_a_quarter_block() -> None:
    """The regression this check was rewritten for.

    The shell implementation matched a literal bracket expression with grep,
    which outside a UTF-8 locale matches byte-wise. Every quarter block starts
    with the byte 0xE2, and so does the em dash, so any line containing one was
    reported. Comparing code points cannot make that mistake, and this pins it.
    """
    mod = check_module()
    line = f"// 0.5 fps, i.e. one frame every 2 seconds of pcap wall-clock {EM_DASH} 1/20 of"
    assert mod.scan_text(line) == []


def test_other_e2_lead_byte_characters_are_not_flagged() -> None:
    """The em dash is not the only U+2xxx character sharing that lead byte."""
    mod = check_module()
    for char in (EN_DASH, LEFT_QUOTE, RIGHT_QUOTE):
        assert mod.scan_text(f"text {char} more text") == [], f"flagged {char!r}"


def test_safe_block_characters_are_not_flagged() -> None:
    """Full, half and shade blocks render on the Pi console and are allowed.

    They sit just below the forbidden range, so an off-by-one on the lower
    bound would catch them.
    """
    mod = check_module()
    for char in (FULL_BLOCK, LOWER_HALF_BLOCK, LIGHT_SHADE, RIGHT_HALF_BLOCK):
        assert mod.scan_text(f"banner {char}{char}{char}") == [], f"flagged {char!r}"


def test_quarter_blocks_are_flagged_with_their_line_and_character() -> None:
    mod = check_module()
    text = "\n".join(
        [
            "clean line",
            f"offending {QUARTER_BLOCK} line",
            "clean again",
        ]
    )
    assert mod.scan_text(text) == [(2, QUARTER_BLOCK)]


def test_both_ends_of_the_range_are_flagged() -> None:
    """Inclusive bounds: an off-by-one at either end would slip a glyph past."""
    mod = check_module()
    assert mod.scan_text(QUARTER_BLOCK) == [(1, QUARTER_BLOCK)]
    assert mod.scan_text(LAST_QUARTER_BLOCK) == [(1, LAST_QUARTER_BLOCK)]
    assert mod.is_quarter_block(QUARTER_BLOCK)
    assert mod.is_quarter_block(LAST_QUARTER_BLOCK)
    # One either side of the range.
    assert not mod.is_quarter_block(chr(0x2595))
    assert not mod.is_quarter_block(chr(0x25A0))


def test_distinct_characters_on_one_line_are_each_reported_once() -> None:
    mod = check_module()
    line = f"{QUARTER_BLOCK}{QUARTER_BLOCK}{LAST_QUARTER_BLOCK}"
    assert mod.scan_text(line) == [(1, QUARTER_BLOCK), (1, LAST_QUARTER_BLOCK)]


def test_describe_names_the_character() -> None:
    mod = check_module()
    described = mod.describe(QUARTER_BLOCK)
    assert "U+2596" in described
    assert "QUADRANT" in described


def test_scan_file_tolerates_undecodable_bytes(tmp_path: Path) -> None:
    """A file that is not valid UTF-8 must not crash the lint."""
    mod = check_module()
    target = tmp_path / "binaryish.txt"
    target.write_bytes(b"\xff\xfe not utf-8 at all\n")
    assert mod.scan_file(target) == []


def test_scan_file_reads_a_real_file(tmp_path: Path) -> None:
    mod = check_module()
    target = tmp_path / "sample.md"
    target.write_text(f"first\nsecond {QUARTER_BLOCK}\n", encoding="utf-8")
    assert mod.scan_file(target) == [(2, QUARTER_BLOCK)]


def run_script(args: list[str], env_overrides: dict[str, str] | None = None):
    """Invoke the script as a subprocess, so exit codes are exercised."""
    import os

    env = dict(os.environ)
    if env_overrides is not None:
        env.update(env_overrides)
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args],
        capture_output=True,
        text=True,
        env=env,
        cwd=ROOT,
    )


def test_exit_code_is_nonzero_on_findings(tmp_path: Path) -> None:
    target = tmp_path / "bad.txt"
    target.write_text(f"boom {QUARTER_BLOCK}\n", encoding="utf-8")
    result = run_script([str(target)])
    assert result.returncode == 1
    assert "U+2596" in result.stdout


def test_report_mode_always_exits_zero(tmp_path: Path) -> None:
    target = tmp_path / "bad.txt"
    target.write_text(f"boom {QUARTER_BLOCK}\n", encoding="utf-8")
    result = run_script(["--report", str(target)])
    assert result.returncode == 0
    assert "U+2596" in result.stdout


def test_clean_file_exits_zero(tmp_path: Path) -> None:
    target = tmp_path / "good.md"
    target.write_text(
        f"an em dash {EM_DASH} and a block {FULL_BLOCK}\n", encoding="utf-8"
    )
    result = run_script([str(target)])
    assert result.returncode == 0, result.stdout


def test_result_is_identical_without_a_locale() -> None:
    """The whole point of the rewrite.

    The shell version disagreed with itself depending on the caller's locale.
    Running the real repository scan under an empty locale and under a UTF-8
    one must now produce the same output and the same exit code.
    """
    stripped = run_script([], env_overrides={"LC_ALL": "", "LANG": ""})
    utf8 = run_script(
        [], env_overrides={"LC_ALL": "en_US.UTF-8", "LANG": "en_US.UTF-8"}
    )
    assert stripped.returncode == utf8.returncode
    assert stripped.stdout == utf8.stdout


def test_the_repository_is_clean() -> None:
    """The check passes on the tree as committed, including this test file."""
    result = run_script([])
    assert result.returncode == 0, result.stdout
