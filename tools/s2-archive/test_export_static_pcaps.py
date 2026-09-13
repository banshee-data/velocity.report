import importlib.util
import json
from pathlib import Path

import pytest

SCRIPT = Path(__file__).with_name("export-static-pcaps.py")
SPEC = importlib.util.spec_from_file_location("export_static_pcaps", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def site(**overrides):
    value = {
        "id": "small-site",
        "lat": 37.7638,
        "lon": -122.4466,
        "position_confidence": "high",
        "pcap_split_build_version": "0.5.1-pre32",
    }
    value.update(overrides)
    return value


def test_publication_metadata():
    assert MODULE.publication_metadata(site()) == {
        "latitude": 37.7638,
        "longitude": -122.4466,
        "pcap_split_build_version": "0.5.1-pre32",
    }


@pytest.mark.parametrize(
    "overrides",
    [
        {"lat": None},
        {"lon": None},
        {"lat": 91},
        {"lon": -181},
        {"pcap_split_build_version": ""},
    ],
)
def test_publication_metadata_rejects_incomplete_provenance(overrides):
    with pytest.raises(RuntimeError):
        MODULE.publication_metadata(site(**overrides))


def test_backfill_sidecar_preserves_existing_provenance(tmp_path):
    capture = tmp_path / "small-site.pcapng"
    capture.write_bytes(b"small capture")
    sidecar = tmp_path / "small-site.json"
    original = {
        "schema_version": 1,
        "site_id": "small-site",
        "site": "s01",
        "source_index": "tools/s2-archive/site-index.json",
        "position_confidence": "high",
        "source_files": [{"name": "source.pcap", "sha256": "sha256:source"}],
        "output_file": capture.name,
        "output_sha256": "sha256:output",
        "output_bytes": capture.stat().st_size,
    }
    sidecar.write_text(json.dumps(original) + "\n")

    MODULE.backfill_sidecar(site(), tmp_path)

    updated = json.loads(sidecar.read_text())
    for key, value in original.items():
        if key not in {"site", "source_index", "position_confidence"}:
            assert updated[key] == value
    assert not {"site", "source_index", "position_confidence"} & updated.keys()
    assert updated["latitude"] == 37.7638
    assert updated["longitude"] == -122.4466
    assert updated["pcap_split_build_version"] == "0.5.1-pre32"


def test_backfill_refuses_changed_capture(tmp_path):
    capture = tmp_path / "small-site.pcapng"
    capture.write_bytes(b"changed")
    (tmp_path / "small-site.json").write_text(
        json.dumps(
            {
                "site_id": "small-site",
                "output_file": capture.name,
                "output_bytes": capture.stat().st_size + 1,
            }
        )
    )

    with pytest.raises(RuntimeError, match="size no longer matches"):
        MODULE.backfill_sidecar(site(), tmp_path)
