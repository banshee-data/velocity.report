#!/usr/bin/env python3

import re
import sys


NODE_BLOCK_RE = re.compile(r"(?ms)^([A-Za-z0-9_]+)\s+\[label=<.*?>\];\s*")
GRAPH_OPEN_RE = re.compile(r"\A\s*digraph\s+[^{]+\{", re.MULTILINE)

CLUSTERS = (
    ("site", "SITE"),
    ("lidar", "LIDAR"),
    ("radar", "RADAR"),
)


def cluster_for(table_name: str) -> str:
    if table_name == "site" or table_name.startswith("site_"):
        return "site"
    if table_name.startswith("lidar_"):
        return "lidar"
    if table_name.startswith("radar_"):
        return "radar"
    return "other"


def node_weight(node_block: str) -> int:
    return node_block.count("<TR>")


def split_balanced_columns(tables):
    weighted = sorted(tables, key=lambda item: (-node_weight(item[1]), item[0]))
    total_weight = sum(node_weight(block) for _, block in weighted)
    states = {0: 0}

    for index, (_, block) in enumerate(weighted):
        weight = node_weight(block)
        bit = 1 << index
        next_states = dict(states)
        for subtotal, mask in states.items():
            candidate_total = subtotal + weight
            if candidate_total not in next_states:
                next_states[candidate_total] = mask | bit
        states = next_states

    target = total_weight / 2
    valid_sums = [subtotal for subtotal in states if 0 < subtotal < total_weight]
    if not valid_sums:
        midpoint = max(1, len(tables) // 2)
        return sorted(tables[:midpoint]), sorted(tables[midpoint:])

    best_sum = min(valid_sums, key=lambda subtotal: (abs(subtotal - target), subtotal))
    left_mask = states[best_sum]
    left = []
    right = []
    for index, table in enumerate(weighted):
        if left_mask & (1 << index):
            left.append(table)
        else:
            right.append(table)

    return sorted(left, key=lambda item: item[0]), sorted(right, key=lambda item: item[0])


def main() -> int:
    dot = sys.stdin.read()
    if not dot.strip():
        return 0

    open_match = GRAPH_OPEN_RE.search(dot)
    if open_match is None:
        sys.stderr.write("Error: expected a Graphviz digraph on stdin\n")
        return 1

    closing_index = dot.rfind("}")
    if closing_index == -1 or closing_index < open_match.end():
        sys.stderr.write("Error: expected a closing graph brace\n")
        return 1

    graph_open = open_match.group(0)
    body = dot[open_match.end() : closing_index]

    node_matches = list(NODE_BLOCK_RE.finditer(body))
    if not node_matches:
        sys.stdout.write(dot)
        return 0

    header = body[: node_matches[0].start()]
    grouped_nodes = {name: [] for name, _ in CLUSTERS}
    grouped_nodes["other"] = []

    tail_chunks = []
    cursor = node_matches[0].start()
    for match in node_matches:
        if cursor < match.start():
            tail_chunks.append(body[cursor : match.start()])
        table_name = match.group(1)
        grouped_nodes[cluster_for(table_name)].append((table_name, match.group(0)))
        cursor = match.end()
    tail_chunks.append(body[cursor:])

    tail_lines = []
    for chunk in tail_chunks:
        for line in chunk.splitlines():
            if line.strip():
                tail_lines.append(line)

    output_lines = [graph_open]
    if header.strip():
        output_lines.append(header.strip("\n"))

    for sortv, (cluster_name, label) in enumerate(CLUSTERS, start=1):
        tables = grouped_nodes[cluster_name]
        if not tables:
            continue
        output_lines.append(f'subgraph cluster_{cluster_name} {{')
        output_lines.append(
            '  graph ['
            f'label="{label}", '
            'labelloc="t", '
            'labeljust="l", '
            f'sortv={sortv}, '
            'style="rounded", '
            'color="#aaaaaa"'
            '];'
        )
        sorted_tables = sorted(tables, key=lambda item: item[0])
        if cluster_name == "lidar" and len(sorted_tables) > 4:
            left_tables, right_tables = split_balanced_columns(sorted_tables)
            output_lines.append("  subgraph cluster_lidar_left {")
            output_lines.append('    graph [style="invis"];')
            for _, block in left_tables:
                for line in block.splitlines():
                    if line.strip():
                        output_lines.append("    " + line)
            output_lines.append("  }")
            output_lines.append("  subgraph cluster_lidar_right {")
            output_lines.append('    graph [style="invis"];')
            for _, block in right_tables:
                for line in block.splitlines():
                    if line.strip():
                        output_lines.append("    " + line)
            output_lines.append("  }")
            if left_tables and right_tables:
                output_lines.append(
                    f'  {left_tables[0][0]} -> {right_tables[0][0]} '
                    '[style="invis", weight=100];'
                )
        else:
            for _, block in sorted_tables:
                for line in block.splitlines():
                    if line.strip():
                        output_lines.append("  " + line)
        output_lines.append("}")

    for _, block in sorted(grouped_nodes["other"], key=lambda item: item[0]):
        for line in block.splitlines():
            if line.strip():
                output_lines.append(line)

    output_lines.extend(tail_lines)
    output_lines.append("}")
    sys.stdout.write("\n".join(output_lines) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
