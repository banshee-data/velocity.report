from __future__ import annotations

import importlib.machinery
import importlib.util
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
