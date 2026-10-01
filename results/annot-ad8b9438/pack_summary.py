#!/usr/bin/env python3
"""pack_summary.py <copy-dir> <pack-dir> <site-index.json> <out-dir>

Read-only summary of an annotation pack. <copy-dir> holds copies of the pack's
small files (manifest.json, samples.json, segment.json, annotations.json,
physical-references.json); <pack-dir> is listed and its annotation-revisions/
read, never written. Writes summary.json and summary.txt into <out-dir>.
"""
import collections
import json
import os
import sys
import time

ROAD_USERS = {"car", "van", "truck", "bus", "motorcycle", "pedestrian", "cyclist"}


def load(path):
    if not os.path.isfile(path):
        return None
    with open(path) as f:
        return json.load(f)


def ignore_reason(obj, m):
    """The per-frame evaluator's default policy (annotation.BuildReference)."""
    ostat = (obj or {}).get("status")
    if ostat == "rejected" or m.get("status") == "rejected":
        return "dropped_rejected"
    reason = ""
    if not m.get("point_indices"):
        if not m.get("uncertain_indices"):
            return "dropped_empty"
        reason = "uncertain_membership"
    if not (ostat == "reviewed" and m.get("status") == "reviewed"):
        return "unreviewed"
    if (obj.get("class") or "").strip().lower() not in ROAD_USERS:
        return "not_road_user"
    if m.get("visibility") not in ("present", "partly_occluded"):
        return "visibility"
    if reason:
        return reason
    if m.get("completeness") == "unreviewed":
        return "incomplete_mask"
    return "scored"


def summarise_sidecar(sc, samples):
    objects = {o["object_id"]: o for o in sc.get("objects", [])}
    masks = sc.get("masks", [])
    sample_ts = {s["sample_id"]: s["timestamp_ns"] for s in (samples or [])}
    per_obj = collections.OrderedDict()
    for oid, o in sorted(objects.items()):
        per_obj[oid] = {
            "class": o.get("class"), "status": o.get("status"),
            "algorithm": (o.get("provenance") or {}).get("algorithm"),
            "masks": 0, "mask_status": collections.Counter(), "algorithms": collections.Counter(),
            "completeness": collections.Counter(), "visibility": collections.Counter(),
            "eval": collections.Counter(), "points": [], "uncertain": 0,
            "first_sample": None, "last_sample": None, "with_pose": 0,
        }
    orphan = 0
    totals = {k: collections.Counter() for k in ("status", "algorithm", "completeness", "visibility", "eval")}
    authors = collections.Counter()
    for m in masks:
        oid = m.get("object_id")
        r = per_obj.get(oid)
        if r is None:
            orphan += 1
            continue
        prov = m.get("provenance") or {}
        alg = prov.get("algorithm") or "by_hand"
        ev = ignore_reason(objects[oid], m)
        r["masks"] += 1
        r["mask_status"][m.get("status")] += 1
        r["algorithms"][alg] += 1
        r["completeness"][m.get("completeness")] += 1
        r["visibility"][m.get("visibility")] += 1
        r["eval"][ev] += 1
        r["points"].append(len(m.get("point_indices") or []))
        r["uncertain"] += len(m.get("uncertain_indices") or [])
        if m.get("pose"):
            r["with_pose"] += 1
        sid = m.get("sample_id")
        r["first_sample"] = sid if r["first_sample"] is None else min(r["first_sample"], sid)
        r["last_sample"] = sid if r["last_sample"] is None else max(r["last_sample"], sid)
        totals["status"][m.get("status")] += 1
        totals["algorithm"][alg] += 1
        totals["completeness"][m.get("completeness")] += 1
        totals["visibility"][m.get("visibility")] += 1
        totals["eval"][ev] += 1
        authors[prov.get("author") or ""] += 1

    freeze_blockers = []
    rows = []
    for oid, r in per_obj.items():
        pts = sorted(r["points"])
        med = pts[len(pts) // 2] if pts else 0
        rows.append({
            "object_id": oid, "class": r["class"], "status": r["status"], "masks": r["masks"],
            "mask_status": dict(r["mask_status"]), "algorithms": dict(r["algorithms"]),
            "completeness": dict(r["completeness"]), "visibility": dict(r["visibility"]),
            "eval": dict(r["eval"]), "median_points": med, "min_points": pts[0] if pts else 0,
            "uncertain_points": r["uncertain"], "with_pose": r["with_pose"],
            "first_sample": r["first_sample"], "last_sample": r["last_sample"],
            "span_s": ((sample_ts.get(r["last_sample"], 0) - sample_ts.get(r["first_sample"], 0)) / 1e9)
            if r["first_sample"] is not None and sample_ts else None,
        })
    # Freeze readiness: annotation-split freeze refuses an object not reviewed, with a
    # proposed mask, or with a reviewed mask of unstated completeness.
    unstated = collections.Counter()
    for m in masks:
        if m.get("status") == "reviewed" and m.get("completeness") == "unreviewed":
            unstated[m.get("object_id")] += 1
    for row in rows:
        if row["status"] == "rejected":
            continue
        why = []
        if row["status"] != "reviewed":
            why.append("object %s" % row["status"])
        if row["mask_status"].get("proposed"):
            why.append("%d proposed masks" % row["mask_status"]["proposed"])
        if unstated.get(row["object_id"]):
            why.append("%d reviewed masks with completeness unstated" % unstated[row["object_id"]])
        if why:
            freeze_blockers.append({"object_id": row["object_id"], "class": row["class"], "why": why})

    return {
        "schema_version": sc.get("schema_version"), "dataset_id": sc.get("dataset_id"),
        "pack_digest": sc.get("pack_digest"), "revision": sc.get("revision"),
        "updated_utc": sc.get("updated_utc"), "change": sc.get("change"),
        "objects": len(objects),
        "objects_by_status_class": {"%s/%s" % k: v for k, v in sorted(collections.Counter(
            (o.get("status"), o.get("class")) for o in objects.values()).items())},
        "masks": len(masks), "orphan_masks": orphan,
        "mask_totals": {k: dict(v) for k, v in totals.items()},
        "mask_authors": dict(authors),
        "correspondences": len(sc.get("correspondences") or []),
        "dismissed_proposals": len(sc.get("dismissed_proposals") or []),
        "per_object": rows, "freeze_blockers": freeze_blockers,
    }


def summarise_revisions(pack_dir, budget_s=600):
    d = os.path.join(pack_dir, "annotation-revisions")
    if not os.path.isdir(d):
        return {"count": 0}
    names = sorted(n for n in os.listdir(d) if n.endswith(".json"))
    pick = names
    if len(names) > 300:
        step = max(1, len(names) // 280)
        pick = sorted(set(names[::step] + names[-20:]))
    out, t0 = [], time.time()
    for n in pick:
        if time.time() - t0 > budget_s:
            out.append({"file": n, "skipped": "time budget"})
            continue
        p = os.path.join(d, n)
        try:
            with open(p) as f:
                sc = json.load(f)
        except Exception as e:  # noqa: BLE001
            out.append({"file": n, "error": str(e)[:200]})
            continue
        ms = sc.get("masks", [])
        ch = sc.get("change") or {}
        out.append({
            "file": n, "bytes": os.path.getsize(p), "revision": sc.get("revision"),
            "updated_utc": sc.get("updated_utc"), "operation": ch.get("operation"),
            "author": ch.get("author"), "session": ch.get("session"),
            "objects": len(sc.get("objects", [])), "masks": len(ms),
            "reviewed_masks": sum(1 for m in ms if m.get("status") == "reviewed"),
            "reviewed_objects": sum(1 for o in sc.get("objects", []) if o.get("status") == "reviewed"),
            "restored_from": sc.get("restored_from"),
        })
    return {"count": len(names), "parsed": len(pick),
            "total_bytes": sum(os.path.getsize(os.path.join(d, n)) for n in names), "revisions": out}


def summarise_physical(pr):
    if pr is None:
        return None
    objs = pr.get("objects") or []
    kfs = [k for o in objs for k in (o.get("keyframes") or [])]

    def reviewed_indep(x):
        rv = (x or {}).get("review") or {}
        return rv.get("status") == "reviewed" and rv.get("origin") == "independent"
    return {"revision": pr.get("revision"), "objects": len(objs), "keyframes": len(kfs),
            "keyframes_reviewed_independent": sum(1 for k in kfs if reviewed_indep(k)),
            "bodies_reviewed_independent": sum(1 for o in objs if reviewed_indep(o.get("body"))),
            "following": len(pr.get("following") or [])}


def match_case(manifest, segment, site_index):
    base = ((manifest or {}).get("source") or {}).get("pcap_basename") or ""
    hits = []
    for s in site_index or []:
        names = set(s.get("captures") or [])
        for part in s.get("static_parts") or []:
            names.update(part.get("captures") or [])
        if base and (base in names or s.get("id", "~") in base or (s.get("published_as") or "~") in base):
            hits.append(s.get("id"))
    seg_case = (segment or {}).get("case_id")
    return {"pcap_basename": base, "site_index_matches": hits, "segment_case_id": seg_case}


def main():
    copy_dir, pack_dir, site_index_path, out_dir = sys.argv[1:5]
    manifest = load(os.path.join(copy_dir, "manifest.json"))
    samples_doc = load(os.path.join(copy_dir, "samples.json"))
    samples = samples_doc.get("samples") if isinstance(samples_doc, dict) else samples_doc
    segment = load(os.path.join(copy_dir, "segment.json"))
    sidecar = load(os.path.join(copy_dir, "annotations.json")) or {}
    physical = load(os.path.join(copy_dir, "physical-references.json"))
    site_index = load(site_index_path)

    listing = []
    for n in sorted(os.listdir(pack_dir)):
        p = os.path.join(pack_dir, n)
        st = os.stat(p)
        listing.append({"name": n, "dir": os.path.isdir(p), "bytes": st.st_size,
                        "mtime_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(st.st_mtime)),
                        "entries": len(os.listdir(p)) if os.path.isdir(p) else None})
    ts = [s["timestamp_ns"] for s in (samples or [])]
    man = dict(manifest or {})
    out = {
        "pack_dir": pack_dir, "listing": listing,
        "manifest": man, "segment": segment,
        "samples": {"count": len(ts), "first_ns": min(ts) if ts else None, "last_ns": max(ts) if ts else None,
                    "duration_s": (max(ts) - min(ts)) / 1e9 if ts else None,
                    "points_total": sum(s.get("point_count", 0) for s in (samples or []))},
        "case": match_case(manifest, segment, site_index),
        "sidecar": summarise_sidecar(sidecar, samples) if sidecar else None,
        "physical": summarise_physical(physical),
        "revisions": summarise_revisions(pack_dir),
    }
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, "summary.json"), "w") as f:
        json.dump(out, f, indent=1, sort_keys=True, default=str)
        f.write("\n")

    L = []
    sc = out["sidecar"] or {}
    L.append("pack: %s" % pack_dir)
    L.append("case: %s" % json.dumps(out["case"]))
    src = man.get("source") or {}
    L.append("source: pcap=%s sensor=%s params_hash=%s build=%s %s" % (
        src.get("pcap_basename"), src.get("sensor_id"), src.get("params_hash"),
        src.get("build_version"), src.get("build_git_sha")))
    L.append("coverage=%s samples=%s points=%s duration_s=%.1f" % (
        man.get("coverage"), out["samples"]["count"], out["samples"]["points_total"],
        out["samples"]["duration_s"] or 0))
    L.append("segment: %s" % json.dumps(segment)[:600] if segment else "segment: none")
    L.append("sidecar: revision=%s updated=%s objects=%s masks=%s" % (
        sc.get("revision"), sc.get("updated_utc"), sc.get("objects"), sc.get("masks")))
    L.append("objects by status/class: %s" % json.dumps(sc.get("objects_by_status_class")))
    for k, v in (sc.get("mask_totals") or {}).items():
        L.append("masks by %s: %s" % (k, json.dumps(v)))
    L.append("freeze blockers: %d objects" % len(sc.get("freeze_blockers") or []))
    for b in (sc.get("freeze_blockers") or [])[:60]:
        L.append("  %s %s: %s" % (b["object_id"], b["class"], "; ".join(b["why"])))
    L.append("per object: id class status masks first-last span_s median_pts eval")
    for r in sc.get("per_object") or []:
        L.append("  %s %s %s %d %s-%s %s %d %s" % (
            r["object_id"], r["class"], r["status"], r["masks"], r["first_sample"], r["last_sample"],
            "%.1f" % r["span_s"] if r["span_s"] is not None else "-", r["median_points"],
            json.dumps(r["eval"])))
    rv = out["revisions"]
    L.append("revisions: %s files, %s bytes, %s parsed" % (rv.get("count"), rv.get("total_bytes"), rv.get("parsed")))
    for r in (rv.get("revisions") or [])[-40:]:
        L.append("  %s" % json.dumps(r))
    L.append("physical references: %s" % json.dumps(out["physical"]))
    with open(os.path.join(out_dir, "summary.txt"), "w") as f:
        f.write("\n".join(L) + "\n")
    print("\n".join(L[:120]))


if __name__ == "__main__":
    main()
