#!/usr/bin/env python3
"""Mermaid flowchart edge-order linter.

Parses a Mermaid flowchart block from a Markdown file and reports
edge-declaration-order issues that may cause visual line crossings.

Usage:
    python scripts/mermaid-crossing-checker.py [FILE]

If FILE is omitted it defaults to
    docs/lidar/architecture/lidar-data-layer-model.md

What this checks
────────────────
Mermaid delegates layout to Dagre (Sugiyama algorithm).  Edge
declaration order biases the crossing-minimisation sweep: earlier
edges pull their target nodes leftward.

This script checks one actionable invariant:

  For edges sharing the SAME SOURCE and targeting the SAME SUBGRAPH,
  they should be declared in left-to-right target position order.

It does NOT suggest node reorderings or full edge re-sorts — those
changes are too disruptive and unreliable. Fix reported issues by
swapping the indicated edge declarations.
"""

from __future__ import annotations

import re
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path


# ──────────────────────────────────────────────────────────────────────
# Types
# ──────────────────────────────────────────────────────────────────────


@dataclass
class Edge:
    src: str
    dst: str
    line_no: int
    dashed: bool = False

    def __repr__(self) -> str:
        arrow = "-.->" if self.dashed else "-->"
        return f"L{self.line_no}: {self.src} {arrow} {self.dst}"


@dataclass
class Subgraph:
    name: str
    nodes: list[str] = field(default_factory=list)
    direction: str = "TB"


# ──────────────────────────────────────────────────────────────────────
# Parser
# ──────────────────────────────────────────────────────────────────────

_NODE_DEF = re.compile(r"^\s+([A-Za-z_]\w*)\s*\[")
_EDGE = re.compile(r"^\s+([A-Za-z_]\w*)\s+(-->|-.->)\s+([A-Za-z_]\w*)")
_SUBGRAPH_START = re.compile(r"^\s+subgraph\s+(\S+)")
_SUBGRAPH_END = re.compile(r"^\s+end\s*$")
_DIRECTION = re.compile(r"^\s+direction\s+(TB|BT|LR|RL)")
_MERMAID_START = re.compile(r"^```mermaid\s*$")
_MERMAID_END = re.compile(r"^```\s*$")


def parse_mermaid(lines: list[str]) -> tuple[
    list[str],  # global node order
    dict[str, Subgraph],  # subgraph name → Subgraph
    dict[str, str],  # node → subgraph name
    list[Edge],  # edges in declaration order
]:
    """Extract nodes, subgraphs, and edges from the *first* mermaid block."""
    in_mermaid = False
    subgraph_stack: list[str] = []
    global_node_order: list[str] = []
    node_to_subgraph: dict[str, str] = {}
    subgraphs: dict[str, Subgraph] = {}
    edges: list[Edge] = []

    for i, raw_line in enumerate(lines, 1):
        line = raw_line.rstrip()

        if not in_mermaid:
            if _MERMAID_START.match(line):
                in_mermaid = True
            continue

        if _MERMAID_END.match(line):
            break

        m = _SUBGRAPH_START.match(line)
        if m:
            sg_name = m.group(1)
            subgraphs[sg_name] = Subgraph(name=sg_name)
            subgraph_stack.append(sg_name)
            continue

        if _SUBGRAPH_END.match(line):
            if subgraph_stack:
                subgraph_stack.pop()
            continue

        m = _DIRECTION.match(line)
        if m:
            if subgraph_stack:
                subgraphs[subgraph_stack[-1]].direction = m.group(1)
            continue

        m = _EDGE.match(line)
        if m:
            src, arrow, dst = m.group(1), m.group(2), m.group(3)
            edges.append(Edge(src=src, dst=dst, line_no=i, dashed=(arrow == "-.->")))
            continue

        m = _NODE_DEF.match(line)
        if m:
            node_id = m.group(1)
            global_node_order.append(node_id)
            if subgraph_stack:
                current_sg = subgraph_stack[-1]
                subgraphs[current_sg].nodes.append(node_id)
                node_to_subgraph[node_id] = current_sg
            continue

    return global_node_order, subgraphs, node_to_subgraph, edges


# ──────────────────────────────────────────────────────────────────────
# Position index
# ──────────────────────────────────────────────────────────────────────


def build_position_index(
    global_node_order: list[str],
    subgraphs: dict[str, Subgraph],
    node_to_subgraph: dict[str, str],
) -> dict[str, int]:
    """node_id → left-to-right position within its subgraph."""
    pos: dict[str, int] = {}
    for sg in subgraphs.values():
        for i, node in enumerate(sg.nodes):
            pos[node] = i
    for node in global_node_order:
        if node not in pos:
            pos[node] = 0
    return pos


# ──────────────────────────────────────────────────────────────────────
# Edge-order linter  (the only check that matters)
# ──────────────────────────────────────────────────────────────────────


def lint_edge_order(
    edges: list[Edge],
    node_pos: dict[str, int],
    node_to_subgraph: dict[str, str],
) -> list[str]:
    """Check edges from the same source targeting the same subgraph are
    declared in left-to-right target position order."""
    warnings: list[str] = []
    by_src: dict[str, list[Edge]] = defaultdict(list)
    for e in edges:
        by_src[e.src].append(e)

    for src, src_edges in by_src.items():
        if len(src_edges) < 2:
            continue
        # Group by target subgraph
        by_target_sg: dict[str, list[Edge]] = defaultdict(list)
        for e in src_edges:
            sg = node_to_subgraph.get(e.dst, f"__free_{e.dst}")
            by_target_sg[sg].append(e)

        for sg, sg_edges in by_target_sg.items():
            if len(sg_edges) < 2:
                continue
            for i in range(len(sg_edges) - 1):
                ea = sg_edges[i]
                eb = sg_edges[i + 1]
                pa = node_pos.get(ea.dst, 0)
                pb = node_pos.get(eb.dst, 0)
                if pa > pb:
                    warnings.append(
                        f"  SWAP  {ea}  ↔  {eb}\n"
                        f"        target {ea.dst}(pos {pa}) is right of "
                        f"{eb.dst}(pos {pb})"
                    )

    return warnings


# ──────────────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────────────


def main() -> None:
    default = "docs/lidar/architecture/lidar-data-layer-model.md"
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(default)

    if not path.exists():
        print(f"File not found: {path}", file=sys.stderr)
        sys.exit(1)

    lines = path.read_text().splitlines()
    global_node_order, subgraphs, node_to_subgraph, edges = parse_mermaid(lines)

    if not edges:
        print("No Mermaid edges found in file.", file=sys.stderr)
        sys.exit(1)

    node_pos = build_position_index(global_node_order, subgraphs, node_to_subgraph)

    # ── Report ────────────────────────────────────────────────────────

    print(
        f"Parsed {len(global_node_order)} nodes, "
        f"{len(subgraphs)} subgraphs, {len(edges)} edges\n"
    )

    # Show current node positions
    print("═══ Node positions (declaration order) ═══")
    for sg_name, sg in subgraphs.items():
        if not sg.nodes:
            continue
        positions = "  ".join(f"{n}:{node_pos[n]}" for n in sg.nodes)
        print(f"  {sg_name:20s} │ {positions}")
    print()

    # Edge-order lint
    warnings = lint_edge_order(edges, node_pos, node_to_subgraph)
    if warnings:
        print(f"═══ {len(warnings)} edge-order issue(s) — swap these ═══")
        for w in warnings:
            print(w)
        print()
        sys.exit(1)
    else:
        print("All clear — no edge-order issues found.")
        sys.exit(0)


if __name__ == "__main__":
    main()
