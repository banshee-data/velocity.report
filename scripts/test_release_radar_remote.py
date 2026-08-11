from __future__ import annotations

import importlib.machinery
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = ROOT / "scripts" / "release-radar-remote.py"


def load_script_module():
    loader = importlib.machinery.SourceFileLoader(
        "release_radar_remote_test", str(SCRIPT_PATH)
    )
    spec = importlib.util.spec_from_loader("release_radar_remote_test", loader)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def test_parse_args_defaults():
    mod = load_script_module()

    args = mod.parse_args([])

    assert args.host == "velocity.local"
    assert args.user == "pi"
    assert args.local_binary == str(ROOT / "image" / "velocity-binaries" / "velocity")
    assert args.remote_temp_dir == "/tmp/up"
    assert args.remote_binary == "/usr/local/bin/velocity-report"
    assert args.remote_db_path == "/var/lib/velocity-report/sensor_data.db"
    assert args.service == "velocity-report.service"


def test_staged_remote_path_normalises_slashes():
    mod = load_script_module()

    assert mod.staged_remote_path("/tmp/up") == "/tmp/up/velocity-report"
    assert mod.staged_remote_path("/tmp/up/") == "/tmp/up/velocity-report"
    assert mod.staged_remote_path("/") == "/velocity-report"


def test_build_remote_deploy_script_contains_release_steps():
    mod = load_script_module()

    script = mod.build_remote_deploy_script()

    assert 'BACKUP_PATH="${REMOTE_BIN}.${STAMP}.bak"' in script
    assert 'sudo systemctl stop "$SERVICE"' in script
    assert '"$REMOTE_BIN" version' in script
    assert 'sudo "$REMOTE_BIN" migrate --db-path "$DB_PATH" up' in script
    assert 'sudo systemctl start "$SERVICE"' in script
    assert 'sudo systemctl is-active --quiet "$SERVICE"' in script


def test_main_runs_expected_ssh_and_scp_commands(monkeypatch, tmp_path: Path):
    mod = load_script_module()
    binary = tmp_path / "velocity-report"
    binary.write_bytes(b"binary")

    calls: list[tuple[list[str], str | None]] = []

    def fake_run(command, check, text, input=None):
        calls.append((list(command), input))
        return None

    monkeypatch.setattr(mod.subprocess, "run", fake_run)

    assert (
        mod.main(
            [
                "--host",
                "device.local",
                "--user",
                "operator",
                "--local-binary",
                str(binary),
            ]
        )
        == 0
    )

    assert calls[0] == (
        ["ssh", "operator@device.local", "mkdir", "-p", "/tmp/up"],
        None,
    )
    assert calls[1] == (
        ["scp", str(binary.resolve()), "operator@device.local:/tmp/up/velocity-report"],
        None,
    )
    assert calls[2][0] == [
        "ssh",
        "operator@device.local",
        "bash",
        "-s",
        "--",
        "/tmp/up",
        "/usr/local/bin/velocity-report",
        "velocity-report.service",
        "/var/lib/velocity-report/sensor_data.db",
        "/tmp/up/velocity-report",
    ]
    assert 'sudo "$REMOTE_BIN" migrate --db-path "$DB_PATH" up' in calls[2][1]
