from __future__ import annotations

import importlib.machinery
import importlib.util
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load_script_module(name: str, relative_path: str):
    path = ROOT / relative_path
    loader = importlib.machinery.SourceFileLoader(name, str(path))
    spec = importlib.util.spec_from_loader(name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    loader.exec_module(module)
    return module


def test_topological_sort_ignores_self_references() -> None:
    mod = load_script_module(
        "order_schema_tables_test", "scripts/order-schema-tables.py"
    )

    schema = """
    CREATE TABLE parent (
        id TEXT PRIMARY KEY,
        parent_id TEXT,
        FOREIGN KEY (parent_id) REFERENCES parent (id)
    );

    CREATE TABLE child (
        id TEXT PRIMARY KEY,
        parent_id TEXT NOT NULL,
        FOREIGN KEY (parent_id) REFERENCES parent (id)
    );
    """

    tables, deps = mod.parse_schema(schema)
    assert deps["parent"] == ["parent"]
    assert mod.topological_sort(tables, deps) == ["parent", "child"]


def test_reorder_schema_handles_current_schema() -> None:
    mod = load_script_module(
        "order_schema_tables_current_schema_test", "scripts/order-schema-tables.py"
    )

    schema = (ROOT / "internal/db/schema.sql").read_text(encoding="utf-8")
    reordered = mod.reorder_schema(schema)

    run_records_idx = reordered.index('CREATE TABLE IF NOT EXISTS "lidar_run_records"')
    run_tracks_idx = reordered.index('CREATE TABLE IF NOT EXISTS "lidar_run_tracks"')
    replay_cases_idx = reordered.index(
        'CREATE TABLE IF NOT EXISTS "lidar_replay_cases"'
    )
    replay_annotations_idx = reordered.index("CREATE TABLE lidar_replay_annotations")

    assert run_records_idx < run_tracks_idx
    assert replay_cases_idx < replay_annotations_idx
