from __future__ import annotations

import importlib.machinery
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
UPDATE_RELEASE_JSON = ROOT / "scripts" / "update-release-json.py"


def load_update_release_json_module():
    loader = importlib.machinery.SourceFileLoader(
        "update_release_json_test", str(UPDATE_RELEASE_JSON)
    )
    spec = importlib.util.spec_from_loader("update_release_json_test", loader)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def test_release_asset_patterns_accept_single_binary_and_legacy_names():
    mod = load_update_release_json_module()

    linux = mod.PLATFORM_ASSET_RE["linux_arm64"]
    mac = mod.PLATFORM_ASSET_RE["mac_arm64"]

    assert linux.match("velocity-0.5.1-pre22-linux-arm64")
    assert mac.match("velocity-0.5.1-pre22-darwin-arm64")
    assert linux.match("velocity-report-0.5.0-linux-arm64")
    assert mac.match("velocity-report-0.5.0-darwin-arm64")
    assert linux.match("velocity-report-linux-arm64")
    assert linux.match("velocity-report-linux-arm64_0.5.0")
    assert mac.match("velocity-report-mac-arm64")
    assert mac.match("velocity-report-mac-arm64_0.5.0")


def test_pick_release_accepts_v050_stable_asset_name_variant():
    mod = load_update_release_json_module()

    releases = [
        {
            "tag_name": "v0.5.0",
            "draft": False,
            "prerelease": False,
            "assets": [{"name": "velocity-report-linux-arm64_0.5.0"}],
        },
        {
            "tag_name": "v0.4.0",
            "draft": False,
            "prerelease": False,
            "assets": [{"name": "velocity-report-linux-arm64"}],
        },
    ]

    release, asset = mod.pick_release(
        releases, "stable", mod.PLATFORM_ASSET_RE["linux_arm64"]
    )

    assert release["tag_name"] == "v0.5.0"
    assert asset["name"] == "velocity-report-linux-arm64_0.5.0"


def test_image_binary_build_paths_use_single_velocity_artifact():
    dockerfile = (ROOT / "image" / "Dockerfile.build").read_text()
    build_script = (ROOT / "image" / "scripts" / "build-image.sh").read_text()

    for text in (dockerfile, build_script):
        assert "./cmd/velocity" in text
        assert "./cmd/radar" not in text
        assert "./cmd/velocity-ctl" not in text

    assert "/out/velocity" in dockerfile
    assert "/out/velocity-report" not in dockerfile
    assert "/out/velocity-ctl" not in dockerfile
    assert '"$BINARIES_DIR/velocity"' in build_script
    assert '"$BINARIES_DIR/velocity-report"' not in build_script
    assert '"$BINARIES_DIR/velocity-ctl"' not in build_script


def test_docker_go_cache_cleanup_makes_readonly_module_cache_writable():
    build_script = (ROOT / "image" / "scripts" / "build-image.sh").read_text()

    assert "remove_docker_temp_cache_dir()" in build_script
    assert 'chmod -R u+w "$path"' in build_script
    assert 'rm -rf "$path"' in build_script
    assert 'remove_docker_temp_cache_dir "$DOCKER_GO_MOD_CACHE_DIR"' in build_script
    assert 'remove_docker_temp_cache_dir "$DOCKER_GO_BUILD_CACHE_DIR"' in build_script
    assert 'remove_docker_temp_cache_dir "$DOCKER_GO_TMP_DIR"' in build_script


def test_image_runtime_defaults_use_scoped_sudo_and_embedded_tuning_defaults():
    stage_script = (ROOT / "image" / "stage-velocity" / "03-velocity-config" / "00-run.sh").read_text()
    cleanup_script = (ROOT / "image" / "stage-velocity" / "06-cleanup" / "00-run.sh").read_text()
    service_unit = (
        ROOT
        / "image"
        / "stage-velocity"
        / "03-velocity-config"
        / "files"
        / "velocity-report.service"
    ).read_text()

    assert "rm -f /etc/sudoers.d/010_pi-nopasswd" in stage_script
    assert "/etc/sudoers.d/020_velocity-nopasswd" in stage_script
    assert "rm -f /etc/sudoers.d/010_pi-nopasswd" in cleanup_script
    assert "cancel-rename pi 2>/dev/null || true" in cleanup_script
    assert "rm -f /etc/systemd/system/getty@tty1.service.d/userconf.conf" in cleanup_script
    assert "--config /opt/velocity-report/config/tuning.defaults.json" not in service_unit
    assert (
        "ExecStart=/usr/local/bin/velocity-report --listen :80 --db-path /var/lib/velocity-report/sensor_data.db"
        in service_unit
    )
