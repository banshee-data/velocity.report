#!/usr/bin/env python3
"""
Group raw Graphviz DOT output into family clusters for the schema ERD.

Configuration is loaded from erd-config.json (adjacent to this script).
Edit that file to change cluster definitions and lidar layout parameters.

Layout modes (--layout flag):
  full  — (default) clustered with lidar subgroups, balanced columns,
          and invisible alignment edges for a structured layout.
  auto  — tables clustered by family prefix only; Graphviz handles
          all routing within and between clusters.

The --report flag outputs a markdown crossing analysis instead of DOT.
An LLM or human can read the report, update erd-config.json, and re-run.
"""

import argparse
import json
import os
import re
import sys
from collections import defaultdict, deque

NODE_BLOCK_RE = re.compile(r"(?ms)^([A-Za-z0-9_]+)\s+\[label=<.*?>\];\s*")
GRAPH_OPEN_RE = re.compile(r"\A\s*digraph\s+[^{]+\{", re.MULTILINE)
EDGE_PAIR_RE = re.compile(r"^([A-Za-z0-9_]+):[^\s]+ -> ([A-Za-z0-9_]+)(?::[^\s]+)?$")

# ---------------------------------------------------------------------------
# Configuration — loaded from erd-config.json (see that file to edit)
# ---------------------------------------------------------------------------

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CONFIG_PATH = os.path.join(SCRIPT_DIR, "erd-config.json")


def _load_config():
    """Load ERD configuration from erd-config.json."""
    try:
        with open(CONFIG_PATH) as fh:
            cfg = json.load(fh)
    except FileNotFoundError:
        sys.stderr.write(f"Error: config not found: {CONFIG_PATH}\n")
        raise SystemExit(1)
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"Error: malformed JSON in {CONFIG_PATH}: {exc}\n")
        raise SystemExit(1)
    if "clusters" not in cfg:
        sys.stderr.write(f"Error: missing required key 'clusters' in {CONFIG_PATH}\n")
        raise SystemExit(1)
    return cfg


# Configuration is loaded lazily to avoid crashing at import time if
# erd-config.json is missing or invalid. Call _init_config() before using
# any of the derived constants below.
_config = None
CLUSTERS = ()
_lidar = {}
LIDAR_SUBGROUP_ROOTS = ()
ROOTED_LIDAR_SUBGROUPS = set()
LIDAR_SUBGROUP_DISPLAY_ORDER = ("tracks", "analysis_runs", "other")
LIDAR_SECOND_ROW_SUBGROUP = "other"
LIDAR_SECOND_ROW_ALIGNMENT_SUBGROUP = "analysis_runs"
LIDAR_SUBGROUP_MIN_COLUMNS = 2
LIDAR_SUBGROUP_MAX_COLUMNS = 4
LIDAR_SUBGROUP_TARGET_WEIGHT = 36
_layout_overrides = {}


def _init_config():
    """Initialise configuration-derived globals on first use."""
    global _config, CLUSTERS, _lidar
    global LIDAR_SUBGROUP_ROOTS, ROOTED_LIDAR_SUBGROUPS
    global LIDAR_SUBGROUP_DISPLAY_ORDER
    global LIDAR_SECOND_ROW_SUBGROUP, LIDAR_SECOND_ROW_ALIGNMENT_SUBGROUP
    global LIDAR_SUBGROUP_MIN_COLUMNS, LIDAR_SUBGROUP_MAX_COLUMNS
    global LIDAR_SUBGROUP_TARGET_WEIGHT, _layout_overrides

    if _config is not None:
        return

    _config = _load_config()

    CLUSTERS = tuple((c["name"], c["label"]) for c in _config.get("clusters", ()))

    _lidar = _config.get("lidar_subgroups", {}) or {}

    LIDAR_SUBGROUP_ROOTS = tuple(_lidar.get("roots", {}).items())
    ROOTED_LIDAR_SUBGROUPS = {name for name, _ in LIDAR_SUBGROUP_ROOTS}
    LIDAR_SUBGROUP_DISPLAY_ORDER = tuple(
        _lidar.get("display_order", ("tracks", "analysis_runs", "other"))
    )
    LIDAR_SECOND_ROW_SUBGROUP = _lidar.get("second_row_subgroup", "other")
    LIDAR_SECOND_ROW_ALIGNMENT_SUBGROUP = _lidar.get(
        "second_row_alignment_subgroup", "analysis_runs"
    )
    LIDAR_SUBGROUP_MIN_COLUMNS = _lidar.get("min_columns", 2)
    LIDAR_SUBGROUP_MAX_COLUMNS = _lidar.get("max_columns", 4)
    LIDAR_SUBGROUP_TARGET_WEIGHT = _lidar.get("target_weight", 36)

    _layout_overrides = _config.get("layout_overrides", {})


def cluster_for(table_name: str) -> str:
    _init_config()
    for name, _ in CLUSTERS:
        if table_name == name or table_name.startswith(name + "_"):
            return name
    return "other"


def node_weight(node_block: str) -> int:
    return node_block.count("<TR>")


def topo_order(component_names, parents, children):
    ordered_levels = topo_levels(component_names, parents, children)
    ordered = []
    for level_names in ordered_levels:
        ordered.extend(level_names)
    return ordered


def topo_levels(component_names, parents, children):
    in_degree = {
        name: len(
            [
                parent
                for parent in parents[name]
                if parent in component_names and parent != name
            ]
        )
        for name in component_names
    }
    levels = {name: 0 for name in component_names}
    queue = deque(sorted(name for name, degree in in_degree.items() if degree == 0))

    while queue:
        name = queue.popleft()
        for child in sorted(
            child
            for child in children[name]
            if child in component_names and child != name
        ):
            levels[child] = max(levels[child], levels[name] + 1)
            in_degree[child] -= 1
            if in_degree[child] == 0:
                queue.append(child)

    grouped_levels = defaultdict(list)
    for name in component_names:
        grouped_levels[levels[name]].append(name)

    ordered_levels = []
    for level in sorted(grouped_levels):
        ordered_levels.append(
            sorted(
                grouped_levels[level],
                key=lambda name: (-len(children[name]), -len(parents[name]), name),
            )
        )
    return ordered_levels


def dependency_components(tables, parents, children):
    remaining = {name for name, _ in tables}
    table_lookup = {name: block for name, block in tables}
    components = []

    while remaining:
        start = min(remaining)
        queue = [start]
        remaining.remove(start)
        component = []

        while queue:
            name = queue.pop()
            component.append(name)
            neighbors = (parents[name] | children[name]) & set(table_lookup)
            for neighbor in sorted(neighbors):
                if neighbor in remaining:
                    remaining.remove(neighbor)
                    queue.append(neighbor)

        ordered_names = topo_order(component, parents, children)
        weight = sum(node_weight(table_lookup[name]) for name in ordered_names)
        components.append(
            {
                "names": ordered_names,
                "weight": weight,
                "size": len(ordered_names),
                "root": ordered_names[0],
            }
        )

    components.sort(key=lambda item: (-item["size"], -item["weight"], item["root"]))
    return components


def split_lidar_subgroups(tables, parents, children):
    subgroups = {name: [] for name, _ in LIDAR_SUBGROUP_ROOTS}
    subgroups["other"] = []
    available_names = {name for name, _ in tables}

    components = dependency_components(tables, parents, children)
    for _, root_name in LIDAR_SUBGROUP_ROOTS:
        if root_name not in available_names:
            sys.stderr.write(
                f"Warning: lidar subgroup root '{root_name}' was not found; "
                "falling back to the 'other' subgroup where needed.\n"
            )
    for component in components:
        component_names = set(component["names"])
        target_subgroup = "other"
        for subgroup_name, root_name in LIDAR_SUBGROUP_ROOTS:
            if root_name in component_names:
                target_subgroup = subgroup_name
                break
        subgroups[target_subgroup].extend(component["names"])

    return subgroups


def lidar_subgroup_column_count(names, table_lookup):
    if len(names) <= 1:
        return 1

    total_weight = sum(node_weight(table_lookup[name]) for name in names)
    columns = round(total_weight / LIDAR_SUBGROUP_TARGET_WEIGHT)
    columns = max(LIDAR_SUBGROUP_MIN_COLUMNS, columns)
    columns = min(LIDAR_SUBGROUP_MAX_COLUMNS, len(names), columns)
    return max(1, columns)


def split_names_into_columns(names, table_lookup, column_count):
    column_count = max(1, min(column_count, len(names)))
    if column_count == 1:
        return [names]

    columns = balanced_partition(
        names,
        [node_weight(table_lookup[name]) for name in names],
        column_count,
    )
    return [column for column in columns if column]


def split_level_groups_into_columns(level_groups, table_lookup, column_count):
    filtered_levels = [level_names for level_names in level_groups if level_names]
    if not filtered_levels:
        return []

    total_tables = sum(len(lv) for lv in filtered_levels)
    column_count = max(1, min(column_count, total_tables))

    # When more columns are requested than topo levels exist, expand
    # large levels into single-table sub-levels so balanced_partition
    # has enough items to distribute.
    if column_count > len(filtered_levels):
        expanded = []
        for level_names in filtered_levels:
            if len(level_names) == 1:
                expanded.append(level_names)
            else:
                for name in level_names:
                    expanded.append([name])
        filtered_levels = expanded

    column_count = max(1, min(column_count, len(filtered_levels)))
    if column_count == len(filtered_levels):
        return filtered_levels

    columns = balanced_partition(
        filtered_levels,
        [
            sum(node_weight(table_lookup[name]) for name in level_names)
            for level_names in filtered_levels
        ],
        column_count,
    )
    return [
        [name for level_names in column_levels for name in level_names]
        for column_levels in columns
        if column_levels
    ]


def balanced_partition(items, weights, column_count):
    total_items = len(items)
    inf = float("inf")
    prefix = [0]
    for weight in weights:
        prefix.append(prefix[-1] + weight)

    best = [[inf] * (column_count + 1) for _ in range(total_items + 1)]
    split_at = [[-1] * (column_count + 1) for _ in range(total_items + 1)]
    best[0][0] = 0

    for item_count in range(1, total_items + 1):
        for columns_used in range(1, min(column_count, item_count) + 1):
            for previous_count in range(columns_used - 1, item_count):
                candidate = max(
                    best[previous_count][columns_used - 1],
                    prefix[item_count] - prefix[previous_count],
                )
                if candidate < best[item_count][columns_used]:
                    best[item_count][columns_used] = candidate
                    split_at[item_count][columns_used] = previous_count

    partitions = []
    item_count = total_items
    columns_used = column_count
    while columns_used > 0:
        previous_count = split_at[item_count][columns_used]
        partitions.append(items[previous_count:item_count])
        item_count = previous_count
        columns_used -= 1

    partitions.reverse()
    return partitions


def emit_table_nodes(output_lines, indent, names, table_lookup):
    for name in names:
        for line in table_lookup[name].splitlines():
            if line.strip():
                output_lines.append(indent + line)


def emit_lidar_subgroup(
    output_lines, subgroup_name, names, table_lookup, parents, children
):
    if not names:
        return None, None, []

    output_lines.append(f"  subgraph cluster_lidar_{subgroup_name} {{")
    output_lines.append('    graph [style="invis"];')

    column_count = lidar_subgroup_column_count(names, table_lookup)
    if subgroup_name in ROOTED_LIDAR_SUBGROUPS:
        levels = topo_levels(names, parents, children)
        levels = _apply_node_order_overrides(levels, f"lidar.{subgroup_name}")
        columns = list(
            reversed(
                split_level_groups_into_columns(
                    levels,
                    table_lookup,
                    column_count,
                )
            )
        )
    else:
        columns = split_names_into_columns(names, table_lookup, column_count)
    column_heads = []

    for index, column_names in enumerate(columns, start=1):
        if not column_names:
            continue
        column_heads.append(column_names[0])
        output_lines.append(f"    subgraph lidar_{subgroup_name}_col{index} {{")
        output_lines.append("      rank=same;")
        emit_table_nodes(output_lines, "      ", column_names, table_lookup)
        output_lines.append("    }")

    for left_name, right_name in zip(column_heads, column_heads[1:]):
        output_lines.append(
            f'    {left_name} -> {right_name} [style="invis", weight=50];'
        )

    output_lines.append("  }")
    if not column_heads:
        return None, None, []
    return column_heads[0], column_heads[-1], column_heads


# ---------------------------------------------------------------------------
# Layout report — crossing analysis and suggestions
# ---------------------------------------------------------------------------


def _apply_node_order_overrides(cluster_levels, cluster_name):
    """Apply node_order_overrides from config to cluster levels."""
    noo = _layout_overrides.get("node_order_overrides", {}).get(cluster_name, {})
    if not noo:
        return cluster_levels
    result = [list(level) for level in cluster_levels]
    for level_idx in range(len(result)):
        level_key = str(level_idx)
        if level_key in noo:
            override_order = noo[level_key]
            existing = set(result[level_idx])
            reordered = [t for t in override_order if t in existing]
            reordered.extend(t for t in result[level_idx] if t not in set(reordered))
            result[level_idx] = reordered
    return result


def _compute_positions(grouped_nodes, parents, children):
    """Assign (cluster_name, level, position) to each table."""
    positions = {}
    for cluster_name, _ in CLUSTERS:
        tables = grouped_nodes[cluster_name]
        if not tables:
            continue
        sorted_tables = sorted(tables, key=lambda item: item[0])
        levels = topo_levels([name for name, _ in sorted_tables], parents, children)
        levels = _apply_node_order_overrides(levels, cluster_name)
        for level_idx, level_names in enumerate(levels):
            for pos, name in enumerate(level_names):
                positions[name] = (cluster_name, level_idx, pos)
    for pos, (name, _) in enumerate(sorted(grouped_nodes["other"])):
        positions[name] = ("other", 0, pos)
    return positions


def _detect_crossings(positions, all_edges):
    """Detect edge crossings from the assigned positions.

    Returns (crossings, cross_cluster) where crossings is a list of
    crossing details and cross_cluster is a list of edges that span
    cluster boundaries.
    """
    layer_edges = defaultdict(list)
    cross_cluster = []
    for child, parent, _raw in all_edges:
        if child not in positions or parent not in positions:
            continue
        c_cluster, c_level, c_pos = positions[child]
        p_cluster, p_level, p_pos = positions[parent]
        if c_cluster != p_cluster:
            cross_cluster.append((child, parent, c_cluster, p_cluster))
            continue
        lo_level = min(c_level, p_level)
        hi_level = max(c_level, p_level)
        if c_level <= p_level:
            layer_edges[(c_cluster, lo_level, hi_level)].append(
                (c_pos, p_pos, child, parent)
            )
        else:
            layer_edges[(c_cluster, lo_level, hi_level)].append(
                (p_pos, c_pos, parent, child)
            )

    crossings = []
    for key, edge_list in sorted(layer_edges.items()):
        for i in range(len(edge_list)):
            for j in range(i + 1, len(edge_list)):
                a_lo, a_hi, a_from, a_to = edge_list[i]
                b_lo, b_hi, b_from, b_to = edge_list[j]
                if (a_lo < b_lo and a_hi > b_hi) or (a_lo > b_lo and a_hi < b_hi):
                    crossings.append(
                        {
                            "edge_a": (a_from, a_to),
                            "edge_b": (b_from, b_to),
                            "cluster": key[0],
                            "levels": f"{key[1]}\u2192{key[2]}",
                        }
                    )
    return crossings, cross_cluster


def _find_long_edges(positions, all_edges):
    """Find edges spanning more than one topological level."""
    seen = set()
    long = []
    for child, parent, _raw in all_edges:
        if (child, parent) in seen:
            continue
        seen.add((child, parent))
        if child not in positions or parent not in positions:
            continue
        c_cluster, c_level, _ = positions[child]
        p_cluster, p_level, _ = positions[parent]
        if c_cluster != p_cluster:
            continue
        span = abs(c_level - p_level)
        if span > 1:
            long.append((child, parent, c_cluster, span, c_level, p_level))
    long.sort(key=lambda x: (-x[3], x[2], x[0]))
    return long


def _emit_report(grouped_nodes, all_edges, positions):
    """Emit a markdown layout report to stdout."""
    crossings, cross_cluster = _detect_crossings(positions, all_edges)
    long_edges = _find_long_edges(positions, all_edges)

    total_tables = sum(len(v) for v in grouped_nodes.values())
    lines = [
        "# ERD Layout Report",
        "",
        f"**Config**: `{CONFIG_PATH}`  ",
        f"**Tables**: {total_tables}  ",
        f"**Foreign keys**: {len(all_edges)}  ",
        f"**Estimated crossings**: {len(crossings)}  ",
        f"**Cross-cluster edges**: {len(cross_cluster)}  ",
        f"**Long edges** (span > 1 level): {len(long_edges)}",
        "",
    ]

    # Cluster membership with positions
    lines.append("## Cluster Membership")
    lines.append("")
    for cluster_name, label in CLUSTERS:
        tables = grouped_nodes[cluster_name]
        lines.append(f"### {label} ({len(tables)} tables)")
        lines.append("")
        for name, _ in sorted(tables, key=lambda t: t[0]):
            pos_info = ""
            if name in positions:
                _, level, pos = positions[name]
                pos_info = f" \u2014 level {level}, position {pos}"
            lines.append(f"- `{name}`{pos_info}")
        lines.append("")
    other = grouped_nodes["other"]
    if other:
        lines.append(f"### Unclustered ({len(other)} tables)")
        lines.append("")
        for name, _ in sorted(other, key=lambda t: t[0]):
            lines.append(f"- `{name}`")
        lines.append("")

    # Cross-cluster edges
    lines.append("## Cross-Cluster Edges")
    lines.append("")
    if cross_cluster:
        lines.append(
            "Edges spanning cluster boundaries are routed around cluster boxes"
            " and are a primary source of visual tangling."
        )
        lines.append("")
        lines.append("| Child | Parent | From cluster | To cluster |")
        lines.append("|-------|--------|-------------|-----------|")
        for child, parent, c_cl, p_cl in cross_cluster:
            lines.append(f"| `{child}` | `{parent}` | {c_cl} | {p_cl} |")
        lines.append("")
    else:
        lines.append("No cross-cluster foreign keys found.")
        lines.append("")

    # Within-cluster crossings
    lines.append("## Within-Cluster Crossings")
    lines.append("")
    if crossings:
        lines.append(
            "Crossings estimated from topological node ordering."
            " Each row is a pair of edges whose paths must intersect"
            " given the current node positions."
        )
        lines.append("")
        by_cluster = defaultdict(list)
        for c in crossings:
            by_cluster[c["cluster"]].append(c)
        for cluster_name, label in CLUSTERS:
            cluster_crossings = by_cluster.get(cluster_name, [])
            lines.append(f"### {label} \u2014 {len(cluster_crossings)} crossings")
            lines.append("")
            if cluster_crossings:
                lines.append(
                    "| # | Edge A (child \u2192 parent)"
                    " | Edge B (child \u2192 parent) | Levels |"
                )
                lines.append("|---|---|---|---|")
                for idx, c in enumerate(cluster_crossings, 1):
                    ea = f"`{c['edge_a'][0]}` \u2192 `{c['edge_a'][1]}`"
                    eb = f"`{c['edge_b'][0]}` \u2192 `{c['edge_b'][1]}`"
                    lines.append(f"| {idx} | {ea} | {eb} | {c['levels']} |")
                lines.append("")
    else:
        lines.append("No within-cluster crossings detected.")
        lines.append("")

    # Long edges
    lines.append("## Long Edges")
    lines.append("")
    if long_edges:
        lines.append(
            "Edges spanning multiple topological levels increase layout"
            " complexity and may cause crossings not detected"
            " by the same-level-pair analysis above."
        )
        lines.append("")
        lines.append(
            "| Child \u2192 Parent | Cluster | Span" " | Child level | Parent level |"
        )
        lines.append("|---|---|---|---|---|")
        for child, parent, cluster, span, c_lev, p_lev in long_edges:
            lines.append(
                f"| `{child}` \u2192 `{parent}`"
                f" | {cluster} | {span} | {c_lev} | {p_lev} |"
            )
        lines.append("")
    else:
        lines.append("No long edges found.")
        lines.append("")

    # Suggestions
    lines.append("## Suggestions")
    lines.append("")
    lines.append(
        "Each suggestion maps to a key in `erd-config.json`"
        " under `layout_overrides`."
        " Edit the config and re-run `make schema-erd` to apply."
    )
    lines.append("")

    suggestion_idx = 0

    if crossings:
        reorder_targets = defaultdict(set)
        for c in crossings:
            cluster = c["cluster"]
            for edge_key in ("edge_a", "edge_b"):
                for table in c[edge_key]:
                    if table in positions and positions[table][0] == cluster:
                        reorder_targets[(cluster, positions[table][1])].add(table)

        for (cluster, level), tables in sorted(reorder_targets.items()):
            suggestion_idx += 1
            table_json = ", ".join(f'"{t}"' for t in sorted(tables))
            table_md = ", ".join(f"`{t}`" for t in sorted(tables))
            lines.append(
                f"### {suggestion_idx}."
                f" Reorder level {level} in `{cluster}` cluster"
            )
            lines.append("")
            lines.append(f"Tables involved in crossings: {table_md}")
            lines.append("")
            lines.append("```json")
            lines.append('"node_order_overrides": {')
            lines.append(f'  "{cluster}": {{ "{level}": [{table_json}] }}')
            lines.append("}")
            lines.append("```")
            lines.append("")

    for child, parent, _cluster, span, _c, _p in long_edges:
        suggestion_idx += 1
        lines.append(
            f"### {suggestion_idx}."
            f" Increase weight for `{child}` \u2192 `{parent}`"
            f" (span {span})"
        )
        lines.append("")
        lines.append("Higher weight pulls connected nodes closer together.")
        lines.append("")
        lines.append("```json")
        lines.append('"edge_weight_overrides": {')
        lines.append(f'  "{child} -> {parent}": 100')
        lines.append("}")
        lines.append("```")
        lines.append("")

    for child, parent, c_cl, p_cl in cross_cluster:
        suggestion_idx += 1
        lines.append(
            f"### {suggestion_idx}."
            f" Rank hint for cross-cluster"
            f" `{child}` \u2192 `{parent}`"
        )
        lines.append("")
        lines.append(f"Crosses from `{c_cl}` to `{p_cl}`.")
        lines.append("")
        lines.append("```json")
        lines.append(f'"rank_hints": [["{child}", "{parent}"]]')
        lines.append("```")
        lines.append("")

    if suggestion_idx == 0:
        lines.append(
            "No suggestions \u2014 the layout has no detected crossings"
            " or long edges."
        )
        lines.append("")

    # Current overrides
    overrides = _config.get("layout_overrides", {})
    lines.append("## Current `layout_overrides`")
    lines.append("")
    lines.append("```json")
    lines.append(json.dumps(overrides, indent=2))
    lines.append("```")
    lines.append("")

    sys.stdout.write("\n".join(lines) + "\n")
    return 0


def _emit_auto_layout(graph_open, header, grouped_nodes, tail_lines):
    """Emit DOT with family clusters only — no forced layout."""
    output_lines = [graph_open]
    if header.strip():
        output_lines.append(header.strip("\n"))

    for cluster_name, label in CLUSTERS:
        tables = grouped_nodes[cluster_name]
        if not tables:
            continue
        output_lines.append(f"subgraph cluster_{cluster_name} {{")
        output_lines.append(
            "  graph ["
            f'label="{label}", '
            'labelloc="t", '
            'labeljust="l", '
            'style="rounded", '
            'color="#aaaaaa"'
            "];"
        )
        for _, block in sorted(tables, key=lambda item: item[0]):
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


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Group DOT into family clusters for the schema ERD."
    )
    parser.add_argument(
        "--layout",
        choices=["full", "auto"],
        default="full",
        help="full: structured layout with subgroups; auto: family clusters only",
    )
    parser.add_argument(
        "--report",
        action="store_true",
        help="output a markdown layout report instead of DOT",
    )
    args = parser.parse_args()
    _init_config()

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

    all_edges = []
    parents = defaultdict(set)
    children = defaultdict(set)
    for line in tail_lines:
        edge = EDGE_PAIR_RE.match(line)
        if edge is None:
            continue
        child_name, parent_name = edge.groups()
        all_edges.append((child_name, parent_name, line))
        if cluster_for(child_name) != cluster_for(parent_name):
            continue
        parents[child_name].add(parent_name)
        children[parent_name].add(child_name)

    if args.report:
        positions = _compute_positions(grouped_nodes, parents, children)
        return _emit_report(grouped_nodes, all_edges, positions)

    if args.layout == "auto":
        return _emit_auto_layout(graph_open, header, grouped_nodes, tail_lines)

    output_lines = [graph_open]
    if header.strip():
        output_lines.append(header.strip("\n"))
    if "newrank=" not in header:
        output_lines.append("newrank=true")
    for attr_key, attr_val in _layout_overrides.get("graph_attributes", {}).items():
        if attr_key.startswith("_"):
            continue
        output_lines.append(f"{attr_key}={attr_val}")

    cluster_bounds = {}
    radar_component_left_anchors = []
    radar_component_right_anchors = []
    for cluster_name, label in CLUSTERS:
        tables = grouped_nodes[cluster_name]
        if not tables:
            continue
        output_lines.append(f"subgraph cluster_{cluster_name} {{")
        output_lines.append(
            "  graph ["
            f'label="{label}", '
            'labelloc="t", '
            'labeljust="l", '
            'style="rounded", '
            'color="#aaaaaa"'
            "];"
        )
        sorted_tables = sorted(tables, key=lambda item: item[0])
        cluster_levels = topo_levels(
            [name for name, _ in sorted_tables], parents, children
        )
        cluster_levels = _apply_node_order_overrides(cluster_levels, cluster_name)
        ordered_tables = [
            name for level_names in cluster_levels for name in level_names
        ]
        table_lookup = {name: block for name, block in sorted_tables}
        if cluster_levels:
            cluster_bounds[cluster_name] = (
                cluster_levels[0][0],
                cluster_levels[-1][0],
            )
        if cluster_name == "radar":
            for component in dependency_components(sorted_tables, parents, children):
                component_levels = topo_levels(component["names"], parents, children)
                radar_component_right_anchors.append(component_levels[0][0])
                radar_component_left_anchors.append(component_levels[-1][0])
        if cluster_name == "lidar" and len(sorted_tables) > 4:
            lidar_subgroups = split_lidar_subgroups(sorted_tables, parents, children)
            subgroup_heads = {}
            subgroup_bounds = []
            for subgroup_name in LIDAR_SUBGROUP_DISPLAY_ORDER:
                subgroup_start, subgroup_end, column_heads = emit_lidar_subgroup(
                    output_lines,
                    subgroup_name,
                    lidar_subgroups[subgroup_name],
                    table_lookup,
                    parents,
                    children,
                )
                subgroup_heads[subgroup_name] = column_heads
                if (
                    subgroup_name != LIDAR_SECOND_ROW_SUBGROUP
                    and subgroup_start
                    and subgroup_end
                ):
                    subgroup_bounds.append((subgroup_start, subgroup_end))
            for (_, left_end), (right_start, _) in zip(
                subgroup_bounds, subgroup_bounds[1:]
            ):
                output_lines.append(
                    f'  {left_end} -> {right_start} [style="invis", weight=50];'
                )
            aligned_row = subgroup_heads.get(LIDAR_SECOND_ROW_SUBGROUP, [])
            aligned_to_row = subgroup_heads.get(LIDAR_SECOND_ROW_ALIGNMENT_SUBGROUP, [])
            if aligned_row and len(aligned_row) == len(aligned_to_row):
                for aligned_to_name, aligned_name in zip(aligned_to_row, aligned_row):
                    output_lines.append(
                        f"  {{ rank=same; {aligned_to_name}; {aligned_name}; }}"
                    )
        else:
            for name in ordered_tables:
                for line in table_lookup[name].splitlines():
                    if line.strip():
                        output_lines.append("  " + line)
        output_lines.append("}")

    other_nodes = sorted(grouped_nodes["other"], key=lambda item: item[0])
    schema_migrations_name = next(
        (name for name, _ in other_nodes if name == "schema_migrations"),
        None,
    )
    if cluster_bounds.get("site"):
        site_rightmost, _ = cluster_bounds["site"]
        for radar_leftmost in radar_component_left_anchors:
            output_lines.append(
                f'{site_rightmost} -> {radar_leftmost} [style="invis", weight=50];'
            )
    if schema_migrations_name:
        for radar_rightmost in radar_component_right_anchors:
            output_lines.append(
                f'{radar_rightmost} -> {schema_migrations_name} [style="invis", weight=50];'
            )

    for _, block in other_nodes:
        for line in block.splitlines():
            if line.strip():
                output_lines.append(line)

    for hint in _layout_overrides.get("rank_hints", []):
        tables = " ".join(f"{t};" for t in hint)
        output_lines.append(f"{{ rank=same; {tables} }}")

    for ie in _layout_overrides.get("invisible_edges", []):
        w = ie.get("weight", 50)
        output_lines.append(f'{ie["from"]} -> {ie["to"]} [style="invis", weight={w}];')

    weight_overrides = _layout_overrides.get("edge_weight_overrides", {})
    for line in tail_lines:
        if weight_overrides:
            edge = EDGE_PAIR_RE.match(line.strip())
            if edge:
                key = f"{edge.group(1)} -> {edge.group(2)}"
                weight = weight_overrides.get(key)
                if weight is not None:
                    line = f"{line.rstrip()} [weight={weight}]"
        output_lines.append(line)

    output_lines.append("}")
    sys.stdout.write("\n".join(output_lines) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
