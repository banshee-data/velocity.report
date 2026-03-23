#!/usr/bin/env python3

import re
import sys
from collections import defaultdict, deque

NODE_BLOCK_RE = re.compile(r"(?ms)^([A-Za-z0-9_]+)\s+\[label=<.*?>\];\s*")
GRAPH_OPEN_RE = re.compile(r"\A\s*digraph\s+[^{]+\{", re.MULTILINE)
EDGE_PAIR_RE = re.compile(r"^([A-Za-z0-9_]+):[^\s]+ -> ([A-Za-z0-9_]+)(?::[^\s]+)?$")

CLUSTERS = (
    ("site", "SITE"),
    ("lidar", "LIDAR"),
    ("radar", "RADAR"),
)

# Dedicated LIDAR subgroups are driven by dependency connectivity to these roots.
# Any lidar table in the same dependency component as one of these tables is
# emitted into that subgroup. Remaining lidar tables go into a third "other"
# subgroup. Adjust these anchors if the schema grows new lidar domains.
LIDAR_SUBGROUP_ROOTS = (
    ("analysis_runs", "lidar_run_records"),
    ("tracks", "lidar_tracks"),
)
ROOTED_LIDAR_SUBGROUPS = {name for name, _ in LIDAR_SUBGROUP_ROOTS}
LIDAR_SUBGROUP_DISPLAY_ORDER = ("tracks", "analysis_runs", "other")
LIDAR_SECOND_ROW_SUBGROUP = "other"
LIDAR_SECOND_ROW_ALIGNMENT_SUBGROUP = "analysis_runs"

# Large lidar subgroups are packed into 2-4 inner columns so they stay wide
# without forcing explicit node coordinates. The target weight is a rough table
# height budget per inner column.
LIDAR_SUBGROUP_MIN_COLUMNS = 2
LIDAR_SUBGROUP_MAX_COLUMNS = 4
LIDAR_SUBGROUP_TARGET_WEIGHT = 36

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
        columns = list(
            reversed(
                split_level_groups_into_columns(
                    topo_levels(names, parents, children),
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

    parents = defaultdict(set)
    children = defaultdict(set)
    for line in tail_lines:
        edge = EDGE_PAIR_RE.match(line)
        if edge is None:
            continue
        child, parent = edge.groups()
        if cluster_for(child) != cluster_for(parent):
            continue
        parents[child].add(parent)
        children[parent].add(child)

    output_lines = [graph_open]
    if header.strip():
        output_lines.append(header.strip("\n"))
    if "newrank=" not in header:
        output_lines.append("newrank=true")

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

    output_lines.extend(tail_lines)
    output_lines.append("}")
    sys.stdout.write("\n".join(output_lines) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
