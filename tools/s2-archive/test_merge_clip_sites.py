import importlib.util
import json
from pathlib import Path

import pytest

SCRIPT = Path(__file__).with_name("merge-clip-sites.py")
SPEC = importlib.util.spec_from_file_location("merge_clip_sites", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def archive(entry_id):
    return {"id": entry_id, "site": "s01", "start": "2026-09-01T12:25:33-07:00"}


def clip(entry_id, **extra):
    value = {
        "id": entry_id,
        "site": entry_id,
        "origin": "clip",
        "clip_id": f"c-{entry_id}",
    }
    value.update(extra)
    return value


def test_clip_entries_follow_the_archive_sorted_by_id():
    index = [archive("ashbury-downey"), archive("haight-divisadero")]
    merged, changes = MODULE.merge(index, [clip("soma-s2"), clip("kirkham-9th-k3")])
    assert [e["id"] for e in merged] == [
        "ashbury-downey",
        "haight-divisadero",
        "kirkham-9th-k3",
        "soma-s2",
    ]
    assert changes == [("added", "soma-s2"), ("added", "kirkham-9th-k3")]


def test_a_served_entry_replaces_its_stored_one():
    index = [archive("a"), clip("k3", where="old")]
    merged, changes = MODULE.merge(index, [clip("k3", where="new")])
    assert merged[1]["where"] == "new"
    assert changes == [("updated", "k3")]
    _, again = MODULE.merge(merged, [clip("k3", where="new")])
    assert again == [("unchanged", "k3")]


def test_entries_the_server_no_longer_serves_are_kept_unless_pruned():
    index = [archive("a"), clip("gone")]
    merged, changes = MODULE.merge(index, [])
    assert [e["id"] for e in merged] == ["a", "gone"]
    assert changes == [("kept", "gone")]
    pruned, changes = MODULE.merge(index, [], prune=True)
    assert [e["id"] for e in pruned] == ["a"]
    assert changes == [("removed", "gone")]


def test_a_clip_may_not_take_an_archive_id():
    with pytest.raises(ValueError, match="archive site"):
        MODULE.merge([archive("ashbury-downey")], [clip("ashbury-downey")])


def test_only_clip_entries_are_accepted_from_the_server():
    with pytest.raises(ValueError, match="non-clip"):
        MODULE.merge([], [{"id": "x"}])


def test_main_writes_the_index(tmp_path, monkeypatch, capsys):
    index = tmp_path / "site-index.json"
    index.write_text(json.dumps([archive("a")]))
    monkeypatch.setattr(
        MODULE,
        "fetch",
        lambda api: ([clip("k3")], {"soma-s3": "the clip is not located"}),
    )
    assert MODULE.main(["--index", str(index), "--dry-run"]) == 0
    assert json.loads(index.read_text()) == [archive("a")]
    assert MODULE.main(["--index", str(index)]) == 0
    written = json.loads(index.read_text())
    assert [e["id"] for e in written] == ["a", "k3"]
    assert index.read_text().endswith("]\n")
    out = capsys.readouterr().out
    assert "not ready soma-s3: the clip is not located" in out
