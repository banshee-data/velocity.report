#!/usr/bin/env python3
"""Write a tuning config with dotted-key overrides applied to the defaults."""

import json
import sys

base = json.load(open(sys.argv[1]))
out = sys.argv[2]
for arg in sys.argv[3:]:
    key, _, val = arg.partition("=")
    node = base
    parts = key.split(".")
    for p in parts[:-1]:
        node = node[p]
    cur = node[parts[-1]]
    if isinstance(cur, bool):
        node[parts[-1]] = val.lower() in ("1", "true", "yes")
    elif isinstance(cur, int) and not isinstance(cur, bool):
        node[parts[-1]] = (
            int(float(val)) if float(val) == int(float(val)) else float(val)
        )
    elif isinstance(cur, float):
        node[parts[-1]] = float(val)
    else:
        node[parts[-1]] = val
json.dump(base, open(out, "w"), indent=2)
