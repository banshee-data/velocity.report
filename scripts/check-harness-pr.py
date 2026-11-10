#!/usr/bin/env python3
"""Pass CI revision metadata as argument values to the offline run-note guard."""

import os
import subprocess

args = ["go", "run", "./cmd/tools/experiments", "check-notes"]
base = os.environ.get("HARNESS_BASE", "")
if base and set(base) != {"0"}:
    args += ["-base", base]
if os.environ.get("HARNESS_HEAD", "").startswith("harness/"):
    args += ["-harness-branch"]
raise SystemExit(subprocess.call(args))
