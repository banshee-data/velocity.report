import importlib.util
import shutil
import subprocess
from collections import defaultdict
from pathlib import Path
import tempfile

import pytest

REPO_ROOT = Path(__file__).resolve().parents[1]
GROUP_DOT_PATH = REPO_ROOT / "scripts/sqlite-erd/group-dot.py"
GRAPH_SH_PATH = REPO_ROOT / "scripts/sqlite-erd/graph.sh"
SCHEMA_SQL_PATH = REPO_ROOT / "internal/db/schema.sql"


def load_group_dot_module():
    spec = importlib.util.spec_from_file_location("group_dot", GROUP_DOT_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def test_topo_levels_ignore_self_referential_foreign_keys():
    group_dot = load_group_dot_module()

    component_names = [
        "lidar_run_records",
        "lidar_replay_cases",
        "lidar_run_tracks",
        "lidar_replay_annotations",
    ]
    parents = defaultdict(set)
    children = defaultdict(set)

    parents["lidar_run_records"].add("lidar_run_records")
    children["lidar_run_records"].add("lidar_run_records")

    parents["lidar_replay_cases"].add("lidar_run_records")
    children["lidar_run_records"].add("lidar_replay_cases")

    parents["lidar_run_tracks"].add("lidar_run_records")
    children["lidar_run_records"].add("lidar_run_tracks")

    parents["lidar_replay_annotations"].update(
        {"lidar_replay_cases", "lidar_run_tracks"}
    )
    children["lidar_replay_cases"].add("lidar_replay_annotations")
    children["lidar_run_tracks"].add("lidar_replay_annotations")

    levels = group_dot.topo_levels(component_names, parents, children)

    assert levels == [
        ["lidar_run_records"],
        ["lidar_replay_cases", "lidar_run_tracks"],
        ["lidar_replay_annotations"],
    ]


@pytest.mark.skipif(shutil.which("dot") is None, reason="Graphviz dot not installed")
def test_graph_script_generate_dot_and_compile():
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_path = Path(tmpdir)
        generated_svg = tmp_path / "schema.svg"
        generated_dot = tmp_path / "schema.dot"
        compiled_svg = tmp_path / "compiled.svg"

        subprocess.run(
            [
                "bash",
                str(GRAPH_SH_PATH),
                "--generate",
                "--svg-output",
                str(generated_svg),
                str(SCHEMA_SQL_PATH),
            ],
            cwd=REPO_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        assert generated_svg.exists()
        assert not generated_dot.exists()

        subprocess.run(
            [
                "bash",
                str(GRAPH_SH_PATH),
                "--generate-dot",
                "--dot-output",
                str(generated_dot),
                str(SCHEMA_SQL_PATH),
            ],
            cwd=REPO_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        assert generated_dot.exists()
        assert "digraph structs" in generated_dot.read_text()

        subprocess.run(
            [
                "bash",
                str(GRAPH_SH_PATH),
                "--compile",
                "--svg-output",
                str(compiled_svg),
                str(generated_dot),
            ],
            cwd=REPO_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
        assert compiled_svg.exists()


if __name__ == "__main__":
    test_topo_levels_ignore_self_referential_foreign_keys()
    test_graph_script_generate_dot_and_compile()
