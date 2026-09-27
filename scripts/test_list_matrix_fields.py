from __future__ import annotations

import importlib.machinery
import importlib.util
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_list_matrix_fields():
    path = ROOT / "scripts" / "list-matrix-fields.py"
    loader = importlib.machinery.SourceFileLoader("list_matrix_fields_test", str(path))
    spec = importlib.util.spec_from_loader("list_matrix_fields_test", loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules["list_matrix_fields_test"] = module
    loader.exec_module(module)
    return module


def test_computed_struct_targets_resolve_to_fields() -> None:
    mod = load_list_matrix_fields()

    structs = mod.extract_computed_structs(ROOT)
    by_name = {struct.name: struct for struct in structs}

    assert by_name["RunStatistics"].file == "internal/lidar/l8analytics/summary.go"
    assert by_name["TrackAlignmentMetrics"].file == (
        "internal/lidar/l5tracks/tracking_metrics.go"
    )
    assert all(struct.fields for struct in structs)


def test_schema_inventory_includes_generated_fields_and_ignores_constraints(
    tmp_path,
) -> None:
    mod = load_list_matrix_fields()
    schema = tmp_path / "internal" / "db" / "schema.sql"
    schema.parent.mkdir(parents=True)
    schema.write_text(
        "CREATE TABLE example (id INTEGER PRIMARY KEY, payload TEXT, "
        "derived TEXT GENERATED ALWAYS AS (upper(payload)) STORED, "
        "CHECK (length(payload) > 0));"
    )

    tables = mod.extract_db_tables(tmp_path)

    assert [(table.name, table.columns) for table in tables] == [
        ("example", ["id", "payload", "derived"])
    ]


def test_matrix_covers_every_current_sqlite_table_and_field() -> None:
    mod = load_list_matrix_fields()
    tables = mod.extract_db_tables(ROOT)
    matrix = (ROOT / "data" / "structures" / "MATRIX.md").read_text()
    table_section = matrix.split("## 4. Database tables", 1)[1].split("## 5.", 1)[0]
    field_section = matrix.split("## 5. Database fields", 1)[1].split("## 6.", 1)[0]

    listed_tables = set(re.findall(r"\| `([^`]+)`\s*\|", table_section))
    listed_fields = set(re.findall(r"\| `([^`]+)`\s*\| `([^`]+)`\s*\|", field_section))
    schema_tables = {table.name for table in tables}
    schema_fields = {
        (table.name, column) for table in tables for column in table.columns
    }

    assert listed_tables == schema_tables
    assert listed_fields == schema_fields
    assert len(re.findall(r"\| `[^`]+`\s*\|", table_section)) == len(tables)
    assert len(re.findall(r"\| `[^`]+`\s*\| `[^`]+`\s*\|", field_section)) == len(
        schema_fields
    )


def test_updated_matrix_surface_counts_match_its_rows() -> None:
    matrix = (ROOT / "data" / "structures" / "MATRIX.md").read_text()
    summary = matrix.split("### Counts by surface", 1)[1].split("### Gap summary", 1)[0]

    for label, start, end, marks in [
        (
            "HTTP endpoints (LiDAR)",
            "## 2. HTTP API endpoints: LiDAR server",
            "## 3. gRPC",
            (4, 5, 6),
        ),
        ("DB tables", "## 4. Database tables", "## 5. Database fields", (None, 3, 4)),
    ]:
        section = matrix.split(start, 1)[1].split(end, 1)[0]
        rows = [
            line.split("|")
            for line in section.splitlines()
            if line.startswith("| ") and "`" in line
        ]
        lines = section.splitlines()
        header = next(i for i, line in enumerate(lines) if line.startswith("| Layer"))
        contiguous = []
        for line in lines[header + 2 :]:
            if not line.startswith("|"):
                break
            if "`" in line:
                contiguous.append(line)
        assert len(contiguous) == len(rows), label
        actual = [len(rows)] + [
            "-" if index is None else sum(row[index].strip() == "✅" for row in rows)
            for index in marks
        ]
        matching = next(
            line.split("|")
            for line in summary.splitlines()
            if line.startswith(f"| {label}")
        )
        declared = [int(matching[2].strip())] + [
            value if value == "-" else int(value)
            for value in (cell.strip() for cell in matching[3:6])
        ]
        assert declared == actual, label
