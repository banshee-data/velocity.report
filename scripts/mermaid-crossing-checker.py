#!/usr/bin/env python3
"""Mermaid flowchart edge-crossing checker.

Parses a Mermaid flowchart block from a Markdown file, extracts the node
declaration order and all edges, then detects potential visual crossings
and suggests a corrected edge ordering.

Usage:
    python scripts/mermaid-crossing-checker.py [FILE]

If FILE is omitted it defaults to
    docs/lidar/architecture/lidar-data-layer-model.md

Layout model (how Dagre/Mermaid decides positions)
──────────────────────────────────────────────────
Mermaid delegates to Dagre, which implements the Sugiyama layered-graph
algorithm.  The steps that matter for crossing:

  1. **Rank assignment** — each node gets a vertical rank (layer).
     Subgraph membership and edge direction determine rank.

  2. **Initial ordering** — within each rank, nodes are ordered by their
     *source-file declaration order*.  This is the part we can control.

  3. **Crossing minimisation** — Dagre runs a barycenter sweep (usually
     2–4 passes) that tries to reorder nodes within a rank to reduce
     crossings.  It starts from the initial ordering and is sensitive to
     *edge declaration order*: earlier edges have more influence.

  4. **Coordinate assignment** — final X positions are computed.

Rules for crossing-free layout:

  • At every rank boundary, the left-to-right source positions should
    match the left-to-right target positions.  If source A is left of
    source B, their targets should also maintain that relative order —
    otherwise the two edges cross.

  • *Edge declaration order* should follow a consistent left-to-right
    sweep of sources.  Declare all edges from the leftmost source first,
    then the next source, etc.  Within a source, declare edges in the
    order their targets appear left-to-right.

  • *Node declaration order* within a subgraph should match the desired
    left-to-right visual position.

This script checks exactly these invariants and reports violations.
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


@dataclass
class Crossing:
    edge_a: Edge
    edge_b: Edge
    reason: str


# ──────────────────────────────────────────────────────────────────────
# Parser
# ──────────────────────────────────────────────────────────────────────

_NODE_DEF = re.compile(
    r"^\s+([A-Za-z_]\w*)\s*\[",
)

_EDGE = re.compile(
    r"^\s+([A-Za-z_]\w*)\s+(-->|-.->)\s+([A-Za-z_]\w*)",
)

_SUBGRAPH_START = re.compile(
    r"^\s+subgraph\s+(\S+)",
)

_SUBGRAPH_END = re.compile(
    r"^\s+end\s*$",
)

_DIRECTION = re.compile(
    r"^\s+direction\s+(TB|BT|LR|RL)",
)

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
    subgraph_stack: list[str] = []  # stack of subgraph names
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
            break  # only process the first mermaid block

        # Subgraph boundaries
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

        # Skip direction declarations
        if _DIRECTION.match(line):
            continue

        # Edge
        m = _EDGE.match(line)
        if m:
            src, arrow, dst = m.group(1), m.group(2), m.group(3)
            edges.append(Edge(src=src, dst=dst, line_no=i, dashed=(arrow == "-.->")))
            continue

        # Node definition
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
# Rank inference
# ──────────────────────────────────────────────────────────────────────


def infer_ranks(
    subgraphs: dict[str, Subgraph],
    node_to_subgraph: dict[str, str],
    global_node_order: list[str],
) -> dict[str, int]:
    """Approximate rank by subgraph declaration order.

    Nodes in the same subgraph share a rank.  Free-floating nodes get
    their own rank based on declaration position.
    """
    sg_order = list(subgraphs.keys())
    sg_rank = {name: i for i, name in enumerate(sg_order)}

    node_rank: dict[str, int] = {}
    free_rank = len(sg_order)

    for node in global_node_order:
        if node in node_to_subgraph:
            node_rank[node] = sg_rank[node_to_subgraph[node]]
        else:
            node_rank[node] = free_rank
            free_rank += 1

    return node_rank


# ──────────────────────────────────────────────────────────────────────
# Position index  (declaration order within a rank)
# ──────────────────────────────────────────────────────────────────────


def build_position_index(
    global_node_order: list[str],
    subgraphs: dict[str, Subgraph],
    node_to_subgraph: dict[str, str],
) -> dict[str, int]:
    """Return a mapping from node_id → left-to-right position within its
    rank (subgraph).  Lower numbers are further left."""
    pos: dict[str, int] = {}
    for sg in subgraphs.values():
        for i, node in enumerate(sg.nodes):
            pos[node] = i

    # Free-floating nodes: each alone in its rank, position 0
    for node in global_node_order:
        if node not in pos:
            pos[node] = 0

    return pos


# ──────────────────────────────────────────────────────────────────────
# Crossing detector
# ──────────────────────────────────────────────────────────────────────


def detect_crossings(
    edges: list[Edge],
    node_rank: dict[str, int],
    node_pos: dict[str, int],
    node_to_subgraph: dict[str, str],
) -> list[Crossing]:
    """Find pairs of edges that cross.

    Two edges cross when they connect the same pair of ranks and the
    source order is opposite to the target order.
    """
    crossings: list[Crossing] = []

    # Group edges by (src_rank, dst_rank) — only edges spanning the
    # same rank gap can cross each other.
    by_rank_pair: dict[tuple[int, int], list[Edge]] = defaultdict(list)
    for e in edges:
        if e.src not in node_rank or e.dst not in node_rank:
            continue
        key = (node_rank[e.src], node_rank[e.dst])
        by_rank_pair[key].append(e)

    for _rank_pair, group in by_rank_pair.items():
        if len(group) < 2:
            continue
        for i, ea in enumerate(group):
            for eb in group[i + 1 :]:
                src_a = node_pos.get(ea.src, 0)
                src_b = node_pos.get(eb.src, 0)
                dst_a = node_pos.get(ea.dst, 0)
                dst_b = node_pos.get(eb.dst, 0)

                # Same source or same target — no crossing from *these* two
                if ea.src == eb.src or ea.dst == eb.dst:
                    continue

                # Crossing: src order disagrees with dst order
                if (src_a - src_b) * (dst_a - dst_b) < 0:
                    sg_src = node_to_subgraph.get(ea.src, "(free)")
                    sg_dst = node_to_subgraph.get(ea.dst, "(free)")
                    crossings.append(
                        Crossing(
                            edge_a=ea,
                            edge_b=eb,
                            reason=(
                                f"{ea.src}(pos {src_a}) → {ea.dst}(pos {dst_a}) "
                                f"crosses "
                                f"{eb.src}(pos {src_b}) → {eb.dst}(pos {dst_b})  "
                                f"[src rank: {sg_src}, dst rank: {sg_dst}]"
                            ),
                        )
                    )

    return crossings


# ──────────────────────────────────────────────────────────────────────
# Edge-order linter
# ──────────────────────────────────────────────────────────────────────


def lint_edge_order(
    edges: list[Edge],
    node_pos: dict[str, int],
    node_to_subgraph: dict[str, str],
) -> list[str]:
    """Check that edges from the same source to the same target rank are
    declared in target position order (left-to-right)."""
    warnings: list[str] = []
    by_src: dict[str, list[Edge]] = defaultdict(list)
    for e in edges:
        by_src[e.src].append(e)

    for src, src_edges in by_src.items():
        if len(src_edges) < 2:
            continue
        # Only compare edges whose targets are in the same subgraph
        # (same rank), since cross-rank position comparison is meaningless.
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
                        f"  {ea}  declared before  {eb}  "
                        f"but target {ea.dst}(pos {pa}) is right of {eb.dst}(pos {pb}) — swap"
                    )

    return warnings


# ──────────────────────────────────────────────────────────────────────
# Suggested edge ordering
# ──────────────────────────────────────────────────────────────────────


def suggest_edge_order(
    edges: list[Edge],
    node_pos: dict[str, int],
    node_rank: dict[str, int],
) -> list[Edge]:
    """Return edges sorted to minimise crossings.

    Sort key: (src_rank, src_pos, dst_rank, dst_pos).
    This ensures a consistent left-to-right sweep at every rank
    boundary.
    """

    def sort_key(e: Edge) -> tuple[int, int, int, int, int]:
        return (
            node_rank.get(e.src, 999),
            node_pos.get(e.src, 0),
            node_rank.get(e.dst, 999),
            node_pos.get(e.dst, 0),
            0 if not e.dashed else 1,  # solid before dashed
        )

    return sorted(edges, key=sort_key)


# ──────────────────────────────────────────────────────────────────────
# Node-order suggestions
# ──────────────────────────────────────────────────────────────────────


def suggest_node_reorder(
    subgraphs: dict[str, Subgraph],
    edges: list[Edge],
    node_to_subgraph: dict[str, str],
    node_pos: dict[str, int],
    node_rank: dict[str, int],
) -> dict[str, list[str]]:
    """For each subgraph, suggest a node order that aligns with the
    majority of incoming/outgoing edge targets.

    Heuristic: order nodes by the median position of the nodes they
    connect to in *adjacent* ranks (parents above, children below).

    Sequential chains (A→B→C within the same subgraph) are detected and
    preserved as a unit, since reordering them would break pipeline flow.
    """
    suggestions: dict[str, list[str]] = {}

    for sg_name, sg in subgraphs.items():
        if len(sg.nodes) < 2:
            continue

        # Detect internal sequential chains: edges where both src and
        # dst are in this subgraph.  These define a fixed ordering
        # constraint.
        sg_set = set(sg.nodes)
        internal_next: dict[str, list[str]] = defaultdict(list)
        internal_prev: dict[str, list[str]] = defaultdict(list)
        for e in edges:
            if e.src in sg_set and e.dst in sg_set:
                internal_next[e.src].append(e.dst)
                internal_prev[e.dst].append(e.src)

        # Build chains through nodes with exactly one next and one prev.
        # Branch points (multiple nexts) or merge points (multiple prevs)
        # terminate the chain.
        chains: list[list[str]] = []
        visited: set[str] = set()
        # Find chain heads: no internal prev, or multiple prevs
        for node in sg.nodes:
            if node in visited:
                continue
            if len(internal_prev.get(node, [])) != 0:
                continue
            # Walk forward through single-successor nodes
            chain = [node]
            visited.add(node)
            cursor = node
            while len(internal_next.get(cursor, [])) == 1:
                nxt = internal_next[cursor][0]
                if nxt in visited:
                    break
                # Only chain through if the target has exactly one pred
                if len(internal_prev.get(nxt, [])) != 1:
                    break
                chain.append(nxt)
                visited.add(nxt)
                cursor = nxt
            chains.append(chain)

        # Add any isolated nodes (not in any chain)
        for node in sg.nodes:
            if node not in visited:
                chains.append([node])
                visited.add(node)

        if len(chains) <= 1:
            # Single chain or single node — no reordering possible
            continue

        # Collect external neighbour positions for each chain
        def chain_barycenter(chain: list[str]) -> float:
            positions: list[float] = []
            for node in chain:
                for e in edges:
                    if e.src == node and e.dst not in sg_set:
                        positions.append(node_pos.get(e.dst, 0))
                    if e.dst == node and e.src not in sg_set:
                        positions.append(node_pos.get(e.src, 0))
            if not positions:
                # Fall back to current position of first node
                return node_pos.get(chain[0], 0)
            return sum(positions) / len(positions)

        proposed_chains = sorted(chains, key=chain_barycenter)
        proposed = [node for chain in proposed_chains for node in chain]

        if proposed != sg.nodes:
            suggestions[sg_name] = proposed

    return suggestions


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

    node_rank = infer_ranks(subgraphs, node_to_subgraph, global_node_order)
    node_pos = build_position_index(global_node_order, subgraphs, node_to_subgraph)

    # ── Report ────────────────────────────────────────────────────────

    print(
        f"Parsed {len(global_node_order)} nodes, "
        f"{len(subgraphs)} subgraphs, {len(edges)} edges\n"
    )

    # 1. Show current node positions
    print("═══ Node positions (declaration order within rank) ═══")
    for sg_name, sg in subgraphs.items():
        if not sg.nodes:
            continue
        positions = "  ".join(f"{n}:{node_pos[n]}" for n in sg.nodes)
        print(f"  {sg_name:20s} │ {positions}")
    print()

    # 2. Detect crossings
    crossings = detect_crossings(edges, node_rank, node_pos, node_to_subgraph)
    if crossings:
        print(f"═══ {len(crossings)} potential crossing(s) detected ═══")
        for c in crossings:
            print(f"  ✗ {c.reason}")
            print(f"      {c.edge_a}")
            print(f"      {c.edge_b}")
        print()
    else:
        print("═══ No crossings detected ═══\n")

    # 3. Edge declaration order lint
    warnings = lint_edge_order(edges, node_pos, node_to_subgraph)
    if warnings:
        print(f"═══ {len(warnings)} edge-order warning(s) ═══")
        for w in warnings:
            print(w)
        print()

    # 4. Suggested node reorder
    node_suggestions = suggest_node_reorder(
        subgraphs,
        edges,
        node_to_subgraph,
        node_pos,
        node_rank,
    )
    if node_suggestions:
        print("═══ Suggested node reorder (within subgraph) ═══")
        for sg_name, proposed in node_suggestions.items():
            current = subgraphs[sg_name].nodes
            print(f"  {sg_name}:")
            print(f"    current:  {' → '.join(current)}")
            print(f"    proposed: {' → '.join(proposed)}")
        print()

    # 5. Suggested edge order
    suggested = suggest_edge_order(edges, node_pos, node_rank)
    if [e.line_no for e in suggested] != [e.line_no for e in edges]:
        print("═══ Suggested edge declaration order ═══")
        print("(sorted by src_rank, src_pos, dst_rank, dst_pos)\n")
        for e in suggested:
            arrow = "-.->" if e.dashed else " -->"
            print(f"    {e.src:6s} {arrow} {e.dst}")
        print()
    else:
        print("═══ Edge declaration order is already optimal ═══\n")

    # Exit code
    if crossings or warnings:
        sys.exit(1)
    print("All clear — no crossings or order warnings.")
    sys.exit(0)


if __name__ == "__main__":
    main()
