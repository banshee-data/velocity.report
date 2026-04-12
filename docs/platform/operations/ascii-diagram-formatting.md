# ASCII Diagram Formatting — Tooling Rejected

**Status:** Closed (no tooling adopted)

This note records the investigation into automated ASCII diagram formatting
and why it was abandoned. It exists so the question is not re-opened without
cause.

## What Was Tried

Two PyPI tools were installed and run against the repository:

| Tool | Version | Command |
|---|---|---|
| `ascfix` | latest | `ascfix --mode diagram docs/ ...` |
| `ascii-guard` | 2.3.0 | `ascii-guard lint .` / `ascii-guard fix .` |

Both tools are general-purpose ASCII art formatters designed for stand-alone,
single-level Unicode box-drawing structures.

## What Happened

### ascfix

Mangled arrow connectors: `────►` became `────  ►` (trailing space inserted
before arrowhead). Shifted box contents sideways in multi-column layouts.
Inserted spurious `↓` characters in flow diagrams. The changes were
consistently wrong and would have broken every diagram it touched.

### ascii-guard

With `--exclude-code-blocks` (the flag that skips content inside triple-backtick
fences): found **zero boxes**, because every diagram in the repo is inside a
code fence. The tool did nothing useful at all.

Without `--exclude-code-blocks`: found 221 boxes across 1,290 files and
reported 25 warnings (missing `┴` junction points). Running `ascii-guard fix`
produced corrupt output: spurious double-junctions (`┴┴`), inserted `│ ││ │`
sequences inside content lines, and broken nested-box structures. A dry-run on
`ARCHITECTURE.md` alone showed three boxes "fixed" into illegibility.

## Root Cause

Both tools model ASCII boxes as stand-alone, single-level structures. The
diagrams in this codebase are:

- **Deeply nested** — boxes within boxes, three to four levels deep.
- **Junction-rich** — inner `┼`, `┤`, `├` characters that mark column
  boundaries inside a table or partition wall, not box corners.
- **Mixed content** — arrow connectors (`──►`, `▶`), flow labels, and
  path text that do not form closed rectangular structures.
- **Inside code fences** — which forces `--exclude-code-blocks` on any
  tool that must not touch prose, making it a no-op.

The warnings reported (missing `┴` at column intersections) are intentional:
the bottom borders use plain `─` because the diagrams are not column-aligned
tables. The tool is wrong; the diagrams are correct.

## Decision

Do not adopt any automated ASCII diagram formatter for this repository.

The diagrams are hand-authored. They are also largely static — they change
only when the architecture changes, which is infrequent. The cost of a
tool that corrupts diagrams on every run far exceeds the value of any
cosmetic alignment it might provide.

If a diagram needs updating, edit it by hand or use a visual box-drawing
helper (e.g. `boxes`, a terminal-based editor plugin, or an online Unicode
box builder) and paste the result.

## Signals That Would Reopen This

- A tool that natively understands triple-backtick code fences **and** nested
  Unicode box structures (none known as of April 2026).
- A decision to migrate diagrams out of code fences into a dedicated diagram
  format (Mermaid, PlantUML, etc.) — at which point this whole class of tool
  becomes irrelevant anyway.
