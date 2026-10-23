"""image/Dockerfile.static-build is shell, not Make.

A RUN line goes to /bin/sh without Docker touching its dollars, so the
Makefile habit of writing $$ to mean $ turns $$? into the shell's PID with
a "?" after it. That once broke the Go build's timeout handler: it lost the
exit status and never printed its 45-minute message.
"""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCKERFILE = ROOT / "image" / "Dockerfile.static-build"


def test_no_makefile_dollar_escapes():
    hits = [
        f"{n}: {line.strip()}"
        for n, line in enumerate(DOCKERFILE.read_text().splitlines(), 1)
        if not line.lstrip().startswith("#") and re.search(r"\$\$[?A-Za-z_{]", line)
    ]
    assert hits == [], f"use $ not $$ in {DOCKERFILE.name}: {hits}"


def test_go_build_timeout_keeps_its_exit_status():
    text = DOCKERFILE.read_text()
    assert "status=$?;" in text
    assert 'exit "$status"' in text
