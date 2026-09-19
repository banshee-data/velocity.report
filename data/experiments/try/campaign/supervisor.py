#!/usr/bin/env python3
"""Campaign supervisor for docs/plans/lidar-parameter-experiment-campaign-2026-09.md.

Reads manifest.json (a small ordered graph of stages), runs whichever
pending stages have all dependencies resolved, and writes results back to
the manifest plus a compact status.json rollup. Rereads manifest.json fresh
at the start of every pass, so a later Claude session can edit any
still-pending stage's params/depends_on between check-ins without
disturbing stages already done/blocked/failed -- those are the permanent
record of completed work and this script never rewrites them once terminal.

Does not run any stage's *work* concurrently with another -- one stage
executes at a time -- but within a single pass it tries every currently
eligible stage once before sleeping, so cheap independent work (e.g. the Go
builds) is never stuck behind a slow one (e.g. waiting for the L3 sweep).

Critical safety property: this script only ever *waits* for the L3 broad
sweep (data/experiments/try/l3-settling-sweep/run_sweep.py) that the
operator already started by hand -- it never launches, restarts, or
otherwise touches that process. See handle_wait_process().

Usage:
    python3 supervisor.py [--manifest manifest.json] [--once]

--once runs a single pass and exits (used for testing); the real 12-hour
run omits it and lets the loop run until the manifest is fully resolved or
the wall-clock budget in manifest.json is spent.
"""

import argparse
import json
import re
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[3]
L3_DIR = REPO_ROOT / "data/experiments/try/l3-settling-sweep"
L5_DIR = REPO_ROOT / "data/experiments/try/l5-gt-sweep"
BIN_DIR = HERE / "bin"

TERMINAL = {
    "done",
    "done_no_op",
    "blocked",
    "failed",
    "skipped",
    "not_started_budget_exceeded",
}


def log(msg):
    line = f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {msg}"
    print(line, flush=True)
    with (HERE / "supervisor.log").open("a") as f:
        f.write(line + "\n")


def load_manifest(path):
    manifest = json.loads(path.read_text())
    for stage in manifest["stages"]:
        if stage["status"] == "running":
            # A prior invocation died mid-stage. Every handler here is
            # idempotent/resumable (the underlying sweep scripts dedupe
            # already-completed rows), so it's always safe to just retry.
            stage["status"] = "pending"
    return manifest


def save_manifest(path, manifest):
    path.write_text(json.dumps(manifest, indent=2) + "\n")


def find_pids(match_substr, exclude_substrs=()):
    """pgrep -f match_substr, minus any hit whose full command line also
    contains one of exclude_substrs. Needed because run_sweep.py is reused
    for the broad Batch-1 sweep *and* the narrowed/replicate follow-ups
    (--capture-ordinal/--sweep-json flags): a bare "run_sweep.py" match
    would also catch those and make wait_process wait on the wrong
    invocation (hit for real on 2026-09-18: a replicate pass triggered
    during supervisor testing matched "run_sweep.py" and would have been
    mistaken for the still-running Batch 1 process)."""
    result = subprocess.run(
        ["pgrep", "-fla", match_substr], capture_output=True, text=True
    )
    pids = []
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        pid, _, cmdline = line.partition(" ")
        if any(ex in cmdline for ex in exclude_substrs):
            continue
        pids.append(pid)
    return pids


def deps_resolved(stage, by_id):
    return all(by_id[d]["status"] in TERMINAL for d in stage["depends_on"])


def another_sweep_already_running():
    """True if any L3 driver (run_sweep.py -- broad, narrowed, replicate --
    or run_repeat_check.py / run_interaction_grid.py) is currently active. Every stage that launches run_sweep.py checks
    this first and backs off (returns "pending", not "failed") rather than
    starting a second one: run_sweep.py's own dedup is computed once at
    process start, so two concurrent invocations racing on the same
    results.csv can each decide the same not-yet-done row is theirs to run,
    producing duplicate rows -- wasteful, and against the "never rerun/
    duplicate" rule even though it wouldn't corrupt the file. Also avoids
    doubling up disk reads against the external volume for no benefit (see
    docs/plans/lidar-heading-coherence-sprint-plan.md Sec 5.2's
    evidence-writes-vs-pcap-reads finding -- same principle, applied to two
    readers instead of a reader and a writer).

    The match requires a Python interpreter followed by the script as a whole
    path component. A bare `pgrep -f run_sweep.py` also matched any process
    merely *mentioning* the name -- caught on 2026-09-18 when the launching
    shell's own command line did, stalling a stage -- and would equally match
    an operator's `less run_sweep.py` or an editor left open, making the
    supervisor wait forever on something that isn't a sweep."""
    return any(
        find_pids(rf"[Pp]ython.*[/ ]{re.escape(name)}( |$)")
        for name in ("run_sweep.py", "run_repeat_check.py", "run_interaction_grid.py")
    )


def run_py(script, args, timeout=None):
    cmd = [sys.executable, str(script)] + [str(a) for a in args]
    proc = subprocess.run(
        cmd, cwd=REPO_ROOT, capture_output=True, text=True, timeout=timeout
    )
    return proc.returncode, proc.stdout, proc.stderr


# ---- stage handlers -------------------------------------------------------
# Each returns (new_status, info_dict). Returning status "pending" means
# "not ready yet, don't change anything, try again next pass" -- used only
# by handle_wait_process.


def handle_wait_process(stage, manifest):
    pids = find_pids(
        stage["params"]["match"], exclude_substrs=stage["params"].get("exclude", [])
    )
    if pids:
        return "pending", {"still_running_pids": pids}
    csv_path = L3_DIR / "results.csv"
    n_rows = 0
    if csv_path.exists():
        import csv as csv_mod

        with csv_path.open() as f:
            n_rows = sum(
                1
                for row in csv_mod.DictReader(f)
                if row.get("capture_ordinal", "0") == "0"
            )
    expected = stage["params"].get("expected_rows")
    note = f"process not running; results.csv has {n_rows} ordinal-0 (broad sweep) rows"
    if expected and n_rows < expected:
        note += f" (expected {expected} -- sweep may have been interrupted; treating as complete anyway, since re-running it is explicitly not this supervisor's job)"
    return "done", {
        "detected_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "note": note,
        "row_count": n_rows,
    }


def handle_build_go_tool(stage, manifest):
    BIN_DIR.mkdir(parents=True, exist_ok=True)
    out_bin = BIN_DIR / stage["params"]["bin_name"]
    build = subprocess.run(
        ["go", "build", "-tags=pcap", "-o", str(out_bin), stage["params"]["pkg"]],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        timeout=300,
    )
    if build.returncode != 0:
        return "failed", {"step": "build", "stderr": build.stderr[-4000:]}
    if stage["params"].get("run_tests"):
        test = subprocess.run(
            ["go", "test", stage["params"]["pkg"] + "..."],
            cwd=REPO_ROOT,
            capture_output=True,
            text=True,
            timeout=300,
        )
        if test.returncode != 0:
            return "failed", {
                "step": "test",
                "stdout": test.stdout[-4000:],
                "stderr": test.stderr[-4000:],
            }
    return "done", {"binary": str(out_bin)}


def handle_analyze_l3_sensitivity(stage, manifest):
    results = L3_DIR / "results.csv"
    if not results.exists():
        return "failed", {"error": f"{results} does not exist"}
    out = L3_DIR / stage["params"].get("out_name", "sensitivity-analysis.json")
    code, stdout, stderr = run_py(
        L3_DIR / "analyze_sensitivity.py", ["--results", results, "--out", out]
    )
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {"summary": stdout.strip().splitlines(), "out": str(out)}


def handle_l3_narrowed_sweep(stage, manifest):
    if another_sweep_already_running():
        return "pending", {
            "note": "another run_sweep.py invocation is active; waiting rather than starting a second one"
        }
    sensitivity = L3_DIR / "sensitivity-analysis.json"
    if not sensitivity.exists():
        return "blocked", {
            "reason": "sensitivity-analysis.json missing (analyze_l3_batch1 did not complete)"
        }
    narrowed = L3_DIR / "narrowed-sweep.json"
    code, stdout, stderr = run_py(
        L3_DIR / "plan_narrowed_sweep.py",
        ["--sensitivity", sensitivity, "--out", narrowed],
    )
    if code != 0:
        return "failed", {"step": "plan", "stdout": stdout, "stderr": stderr}
    spec = json.loads(narrowed.read_text())
    if not spec:
        return "done_no_op", {
            "reason": "no sensitive keys, or narrowing produced no new untested points",
            "plan_output": stdout.strip(),
        }

    settling_eval = BIN_DIR / "settling-eval"
    code, stdout, stderr = run_py(
        L3_DIR / "run_sweep.py",
        [
            "--settling-eval",
            settling_eval,
            "--sweep-json",
            narrowed,
            "--duration-seconds",
            stage["params"].get("duration_seconds", 120),
            "--skip-baseline",
        ],
        timeout=6 * 3600,
    )
    if code != 0:
        return "failed", {
            "step": "sweep",
            "stdout": stdout[-4000:],
            "stderr": stderr[-4000:],
        }
    return "done", {"plan": spec, "stdout_tail": stdout.strip().splitlines()[-5:]}


def handle_l3_replicate_sweep(stage, manifest):
    if another_sweep_already_running():
        return "pending", {
            "note": "another run_sweep.py invocation is active; waiting rather than starting a second one"
        }
    settling_eval = BIN_DIR / "settling-eval"
    code, stdout, stderr = run_py(
        L3_DIR / "run_sweep.py",
        [
            "--settling-eval",
            settling_eval,
            "--capture-ordinal",
            1,
            "--duration-seconds",
            stage["params"].get("duration_seconds", 120),
        ],
        timeout=6 * 3600,
    )
    if code != 0:
        return "failed", {"stdout": stdout[-4000:], "stderr": stderr[-4000:]}
    return "done", {"stdout_tail": stdout.strip().splitlines()[-5:]}


def handle_analyze_replicate_consistency(stage, manifest):
    results = L3_DIR / "results.csv"
    out = L3_DIR / stage["params"].get("out_name", "replicate-consistency.json")
    code, stdout, stderr = run_py(
        L3_DIR / "analyze_replicate_consistency.py",
        ["--results", results, "--out", out],
    )
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {"summary": stdout.strip().splitlines(), "out": str(out)}


def handle_l5_gt_sweep(stage, manifest):
    p = stage["params"]
    gt_eval_bin = BIN_DIR / "lidar-ground-truth-eval"
    velocity_bin = BIN_DIR / "velocity-isolated"
    out_dir = (REPO_ROOT / p["out_dir"]) if p.get("out_dir") else L5_DIR
    args = [
        "--velocity-bin",
        velocity_bin,
        "--gt-eval-bin",
        gt_eval_bin,
        "--reference-db",
        REPO_ROOT / p["reference_db"],
        "--reference-run-id",
        p["reference_run_id"],
        "--pcap-file",
        p["pcap_file"],
        "--pcap-root",
        p["pcap_root"],
        "--udp-port",
        p["udp_port"],
        "--duration-seconds",
        p["duration_seconds"],
    ]
    if p.get("out_dir"):
        args += ["--out-dir", out_dir]
    code, stdout, stderr = run_py(
        L5_DIR / "run_l5_gt_sweep.py",
        args,
        timeout=6 * 3600,
    )
    if code == 3:
        blocked_file = out_dir / "blocked.json"
        reason = (
            json.loads(blocked_file.read_text())["reason"]
            if blocked_file.exists()
            else "unknown (exit 3, no blocked.json)"
        )
        return "blocked", {"reason": reason}
    if code != 0:
        return "failed", {"stdout": stdout[-4000:], "stderr": stderr[-4000:]}
    return "done", {
        "out_dir": str(out_dir),
        "stdout_tail": stdout.strip().splitlines()[-10:],
    }


def combo_ids_all_errored(csv_path, expected_combos):
    """True combo ids among expected_combos where every row ever written for
    that combo has a non-empty error field -- i.e. the combo has zero usable
    data despite the process exiting 0. Added after the 2026-09-18 incident
    where a type bug (neighbour_confirmation_count serialized as 1.0 instead
    of 1) made settling-eval reject the generated config for every site at
    that level, and the stage was still recorded "done" because the grid
    script itself never crashes on a per-row error -- it just records it and
    moves on (see run_interaction_grid.py's per-row try/except). A single
    site failing is tolerated (that's what per-row error fields are for);
    every site failing for the same combo means the combo itself is broken."""
    import csv as csv_mod

    if not csv_path.exists():
        return []
    seen_ok = set()
    seen_any = set()
    with csv_path.open() as f:
        for row in csv_mod.DictReader(f):
            cid = row["combo"]
            seen_any.add(cid)
            if not row.get("error"):
                seen_ok.add(cid)
    return [c for c in expected_combos if c in seen_any and c not in seen_ok]


def handle_l3_interaction_grid(stage, manifest):
    p = stage["params"]
    if another_sweep_already_running():
        return "pending", {"note": "another L3 driver is active; waiting"}
    sens_name = p.get("sensitivity_file", "sensitivity-analysis.json")
    sensitivity = L3_DIR / sens_name
    if not sensitivity.exists():
        return "blocked", {"reason": f"{sens_name} missing"}
    levels_path = L3_DIR / p.get("levels_name", "interaction-levels.json")
    plan_args = ["--sensitivity", sensitivity, "--out", levels_path]
    if p.get("max_keys"):
        plan_args += ["--max-keys", p["max_keys"]]
    if p.get("only_keys"):
        plan_args += ["--only-keys", ",".join(p["only_keys"])]
    if p.get("policy"):
        plan_args += ["--policy", p["policy"]]
    code, stdout, stderr = run_py(L3_DIR / "plan_interaction_levels.py", plan_args)
    if code != 0:
        return "failed", {"step": "plan", "stdout": stdout, "stderr": stderr}
    levels = json.loads(levels_path.read_text())
    if len(levels) < 2:
        return "done_no_op", {
            "reason": "fewer than 2 sensitive keys -- no interaction question to ask",
            "plan_output": stdout.strip(),
        }

    settling_eval = BIN_DIR / "settling-eval"
    code, stdout, stderr = run_py(
        L3_DIR / "run_interaction_grid.py",
        [
            "--settling-eval",
            settling_eval,
            "--levels-json",
            levels_path,
            "--duration-seconds",
            stage["params"].get("duration_seconds", 120),
        ],
        timeout=6 * 3600,
    )
    if code != 0:
        return "failed", {
            "step": "grid",
            "stdout": stdout[-4000:],
            "stderr": stderr[-4000:],
        }

    import itertools as _itertools

    sys.path.insert(0, str(L3_DIR))
    from run_interaction_grid import combo_id as _combo_id  # noqa: E402

    keys = sorted(levels.keys())
    expected_combos = [
        _combo_id(dict(zip(keys, combo)))
        for combo in _itertools.product(*(levels[k] for k in keys))
    ]
    broken = combo_ids_all_errored(L3_DIR / "interaction-results.csv", expected_combos)
    if broken:
        return "failed", {
            "step": "validate",
            "reason": f"combo(s) with zero usable rows across all sites (every attempt errored): {broken}",
            "stdout_tail": stdout.strip().splitlines()[-5:],
        }
    return "done", {"levels": levels, "stdout_tail": stdout.strip().splitlines()[-5:]}


def handle_analyze_interaction_grid(stage, manifest):
    p = stage["params"]
    results = L3_DIR / "interaction-results.csv"
    levels_path = L3_DIR / p.get("levels_name", "interaction-levels.json")
    if not results.exists() or not levels_path.exists():
        return "failed", {
            "error": f"interaction-results.csv or {levels_path.name} missing"
        }
    out = L3_DIR / p.get("analysis_name", "interaction-grid-analysis.json")
    code, stdout, stderr = run_py(
        L3_DIR / "analyze_interaction_grid.py",
        ["--results", results, "--levels", levels_path, "--out", out],
    )
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {"summary": stdout.strip(), "out": str(out)}


def handle_analyze_l5_results(stage, manifest):
    p = stage["params"]
    by_id = {s["id"]: s for s in manifest["stages"]}
    results_paths = []
    for dep_id in p["source_stages"]:
        dep = by_id[dep_id]
        out_dir = (dep.get("result") or {}).get("out_dir", str(L5_DIR))
        results_paths.append(Path(out_dir) / "results.csv")
    out = L5_DIR / p.get("out_name", "ranked-results.json")
    args = ["--out", out]
    for rp in results_paths:
        args += ["--results", rp]
    code, stdout, stderr = run_py(L5_DIR / "analyze_l5_results.py", args)
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {"summary": stdout.strip().splitlines(), "out": str(out)}


def values_all_errored(csv_path, sweep, ordinal="0"):
    """(key, value) pairs in `sweep` for which results.csv has rows but none
    without an error -- i.e. settling-eval rejected that setting at every site
    (e.g. an out-of-range value, or a wrongly typed one). Also returns how many
    pairs had no rows at all (planned but never run)."""
    import csv as csv_mod

    seen_ok, seen_any = set(), set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv_mod.DictReader(f):
                if row.get("capture_ordinal", "0") != str(ordinal):
                    continue
                pair = (row["param_key"], row["param_value"])
                seen_any.add(pair)
                if not row.get("error"):
                    seen_ok.add(pair)
    wanted = [(k, str(v)) for k, vals in sweep.items() for v in vals]
    broken = [
        f"{k}={v}" for k, v in wanted if (k, v) in seen_any and (k, v) not in seen_ok
    ]
    never_ran = sum(1 for pair in wanted if pair not in seen_any)
    return broken, never_ran, len(wanted)


def values_short_of_sites(csv_path, sweep, ordinal, n_sites):
    """(key=value, rows_found) for every planned pair with fewer rows than there
    are sites with a capture at this ordinal. Rows with an error count as rows
    (values_all_errored judges those); this catches rows that were never
    recorded at all. Added after 2026-09-18, when a pre-commit hook replaced
    results.csv mid-run, the driver kept appending to the unlinked file, and
    l3_extended_sweep_b was recorded "done" with 6 of 24 sites."""
    import csv as csv_mod

    if not n_sites or not csv_path.exists():
        return []
    have = {}
    with csv_path.open() as f:
        for row in csv_mod.DictReader(f):
            if row.get("capture_ordinal", "0") == str(ordinal):
                pair = (row["param_key"], row["param_value"])
                have[pair] = have.get(pair, 0) + 1
    return [
        (f"{k}={v}", have.get((k, str(v)), 0))
        for k, vals in sweep.items()
        for v in vals
        if have.get((k, str(v)), 0) < n_sites
    ]


def run_l3_sweep(sweep, spec_path, ordinal, duration, skip_baseline=True):
    """Shared by every stage that drives run_sweep.py from an inline sweep
    spec. Returns (status, info)."""
    if another_sweep_already_running():
        return "pending", {
            "note": "another L3 driver is active; waiting rather than starting a second one"
        }
    spec_path.parent.mkdir(parents=True, exist_ok=True)
    spec_path.write_text(json.dumps(sweep, indent=2) + "\n")
    args = [
        "--settling-eval",
        BIN_DIR / "settling-eval",
        "--sweep-json",
        spec_path,
        "--capture-ordinal",
        ordinal,
        "--duration-seconds",
        duration,
    ]
    if skip_baseline:
        args.append("--skip-baseline")
    code, stdout, stderr = run_py(L3_DIR / "run_sweep.py", args, timeout=6 * 3600)
    if code != 0:
        return "failed", {"stdout": stdout[-4000:], "stderr": stderr[-4000:]}
    broken, never_ran, total = values_all_errored(
        L3_DIR / "results.csv", sweep, ordinal
    )
    info = {
        "n_values": total,
        "stdout_tail": stdout.strip().splitlines()[-3:],
    }
    if broken:
        info["warning"] = (
            f"{len(broken)} of {total} values errored at every site (rejected by "
            f"settling-eval, so they carry no evidence): {broken}"
        )
        info["all_errored_values"] = broken
    if never_ran:
        info["values_never_run"] = never_ran
    sys.path.insert(0, str(L3_DIR))
    from run_sweep import load_sites

    n_sites = len(load_sites(ordinal))
    short = values_short_of_sites(L3_DIR / "results.csv", sweep, ordinal, n_sites)
    if short:
        info["warning"] = (
            f"{len(short)} of {total} values have fewer than {n_sites} rows in "
            "results.csv (runs completed but rows are missing, or the sweep "
            "stopped early); analysis over them would silently use a subset of "
            f"sites: {short[:6]}"
        ) + (f" | {info['warning']}" if "warning" in info else "")
        info["short_values"] = short
        return "failed", info
    if total and len(broken) == total:
        return "failed", info
    return "done", info


def handle_l3_sweep_json(stage, manifest):
    p = stage["params"]
    return run_l3_sweep(
        p["sweep"],
        L3_DIR / "sweeps" / f"{stage['id']}.json",
        p.get("capture_ordinal", 0),
        p.get("duration_seconds", 120),
        p.get("skip_baseline", True),
    )


def handle_l3_replicate_planned(stage, manifest):
    p = stage["params"]
    if another_sweep_already_running():
        return "pending", {"note": "another L3 driver is active; waiting"}
    sens = L3_DIR / p["sensitivity_file"]
    if not sens.exists():
        return "blocked", {"reason": f"{sens.name} missing"}
    spec_path = L3_DIR / "sweeps" / f"{stage['id']}.json"
    spec_path.parent.mkdir(parents=True, exist_ok=True)
    code, stdout, stderr = run_py(
        L3_DIR / "plan_replicate_sweep.py",
        ["--sensitivity", sens, "--keys", ",".join(p["keys"]), "--out", spec_path],
    )
    if code != 0:
        return "failed", {"step": "plan", "stdout": stdout, "stderr": stderr}
    sweep = json.loads(spec_path.read_text())
    if not sweep:
        return "done_no_op", {"reason": "every key was inert", "plan": stdout.strip()}
    status, info = run_l3_sweep(
        sweep, spec_path, 1, p.get("duration_seconds", 120), skip_baseline=True
    )
    info["plan"] = stdout.strip()
    return status, info


def handle_l3_repeat_check(stage, manifest):
    p = stage["params"]
    if another_sweep_already_running():
        return "pending", {"note": "another L3 driver is active; waiting"}
    code, stdout, stderr = run_py(
        L3_DIR / "run_repeat_check.py",
        [
            "--settling-eval",
            BIN_DIR / "settling-eval",
            "--configs-json",
            json.dumps(p["configs"]),
            "--duration-seconds",
            p.get("duration_seconds", 120),
        ],
        timeout=6 * 3600,
    )
    if code != 0:
        return "failed", {"stdout": stdout[-4000:], "stderr": stderr[-4000:]}
    return "done", {"stdout_tail": stdout.strip().splitlines()[-3:]}


def handle_analyze_l3_repeat_check(stage, manifest):
    results = L3_DIR / "repeat-results.csv"
    if not results.exists():
        return "failed", {"error": "repeat-results.csv missing"}
    out = L3_DIR / stage["params"].get("out_name", "repeat-check-analysis.json")
    code, stdout, stderr = run_py(
        L3_DIR / "analyze_repeat_check.py",
        [
            "--results",
            results,
            "--sweep-results",
            L3_DIR / "results.csv",
            "--out",
            out,
        ],
    )
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {"summary": stdout.strip().splitlines(), "out": str(out)}


def handle_gt_oat_sweep(stage, manifest):
    p = stage["params"]
    out_dir = REPO_ROOT / p["out_dir"]
    code, stdout, stderr = run_py(
        L5_DIR / "run_gt_oat_sweep.py",
        [
            "--velocity-bin",
            BIN_DIR / "velocity-isolated",
            "--gt-eval-bin",
            BIN_DIR / "lidar-ground-truth-eval",
            "--reference-db",
            REPO_ROOT / p["reference_db"],
            "--reference-run-id",
            p["reference_run_id"],
            "--pcap-file",
            p["pcap_file"],
            "--pcap-root",
            p["pcap_root"],
            "--udp-port",
            p["udp_port"],
            "--duration-seconds",
            p["duration_seconds"],
            "--sweep-json",
            json.dumps(p["sweep"]),
            "--out-dir",
            out_dir,
        ],
        timeout=6 * 3600,
    )
    if code == 3:
        blocked = out_dir / "blocked.json"
        reason = (
            json.loads(blocked.read_text())["reason"]
            if blocked.exists()
            else "unknown (exit 3, no blocked.json)"
        )
        return "blocked", {"reason": reason}
    if code != 0:
        return "failed", {"stdout": stdout[-4000:], "stderr": stderr[-4000:]}
    import csv as csv_mod

    rows = []
    csv_path = out_dir / "results.csv"
    if csv_path.exists():
        with csv_path.open() as f:
            rows = list(csv_mod.DictReader(f))
    errored = [r for r in rows if r.get("error")]
    info = {
        "out_dir": str(out_dir),
        "n_rows": len(rows),
        "n_errors": len(errored),
        "stdout_tail": stdout.strip().splitlines()[-3:],
    }
    if errored:
        info["warning"] = (
            f"{len(errored)} of {len(rows)} runs errored; first: {errored[0]['error'][:200]}"
        )
    if rows and len(errored) == len(rows):
        return "failed", info
    return "done", info


def handle_analyze_gt_oat(stage, manifest):
    p = stage["params"]
    by_id = {s["id"]: s for s in manifest["stages"]}
    args = ["--out", L5_DIR / p.get("out_name", "gt-oat-analysis.json")]
    for dep_id in p["source_stages"]:
        out_dir = (by_id[dep_id].get("result") or {}).get("out_dir")
        if not out_dir:
            continue
        args += ["--results", Path(out_dir) / "results.csv"]
    if "--results" not in args:
        return "blocked", {"reason": "no source stage produced results"}
    code, stdout, stderr = run_py(L5_DIR / "analyze_gt_oat.py", args)
    if code != 0:
        return "failed", {"stdout": stdout, "stderr": stderr}
    return "done", {
        "summary": stdout.strip().splitlines(),
        "out": str(args[1]),
    }


def handle_recover_raw_rows(stage, manifest):
    """Rebuild results.csv rows whose raw reports survived (see
    recover_rows_from_raw.py). Idempotent: a second run recovers nothing."""
    p = stage["params"]
    if another_sweep_already_running():
        return "pending", {"note": "another L3 driver is active; waiting"}
    record = L3_DIR / p.get("record_name", "results-recovered-rows.json")
    code, stdout, stderr = run_py(
        L3_DIR / "recover_rows_from_raw.py",
        [
            "--results",
            L3_DIR / "results.csv",
            "--raw-dir",
            L3_DIR / "raw",
            "--keys",
            ",".join(p["keys"]),
            "--ordinal",
            p.get("capture_ordinal", 0),
            "--duration-seconds",
            p.get("duration_seconds", 120),
            "--after",
            p["after"],
            "--before",
            p["before"],
            "--record",
            record,
        ],
    )
    lines = stdout.strip().splitlines()
    if code == 2:  # some reports failed verification; record lists which
        return "done", {
            "warning": "some raw reports failed verification and were not recovered: "
            + " | ".join(lines[:6]),
            "record": str(record),
        }
    if code != 0:
        return "failed", {"stdout": stdout[-2000:], "stderr": stderr[-2000:]}
    return "done", {"summary": lines, "record": str(record)}


def handle_finalize(stage, manifest):
    return "done", {"finalized_at": time.strftime("%Y-%m-%dT%H:%M:%S%z")}


HANDLERS = {
    "wait_process": handle_wait_process,
    "build_go_tool": handle_build_go_tool,
    "analyze_l3_sensitivity": handle_analyze_l3_sensitivity,
    "l3_narrowed_sweep": handle_l3_narrowed_sweep,
    "l3_replicate_sweep": handle_l3_replicate_sweep,
    "analyze_replicate_consistency": handle_analyze_replicate_consistency,
    "l5_gt_sweep": handle_l5_gt_sweep,
    "l3_interaction_grid": handle_l3_interaction_grid,
    "analyze_interaction_grid": handle_analyze_interaction_grid,
    "analyze_l5_results": handle_analyze_l5_results,
    "l3_sweep_json": handle_l3_sweep_json,
    "l3_replicate_planned": handle_l3_replicate_planned,
    "l3_repeat_check": handle_l3_repeat_check,
    "analyze_l3_repeat_check": handle_analyze_l3_repeat_check,
    "gt_oat_sweep": handle_gt_oat_sweep,
    "recover_raw_rows": handle_recover_raw_rows,
    "analyze_gt_oat": handle_analyze_gt_oat,
    "finalize": handle_finalize,
}


# ---- status rollup ---------------------------------------------------------


def write_status(manifest, status_path, started_at, budget_hours):
    by_status = {}
    for s in manifest["stages"]:
        by_status.setdefault(s["status"], []).append(s["id"])
    elapsed_h = (time.time() - started_at) / 3600
    status = {
        "updated_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "elapsed_hours": round(elapsed_h, 2),
        "budget_hours": budget_hours,
        "stage_status_counts": {k: len(v) for k, v in by_status.items()},
        "stages": [
            {
                "id": s["id"],
                "status": s["status"],
                "result_summary": _summarize(s.get("result")),
            }
            for s in manifest["stages"]
        ],
    }
    status_path.write_text(json.dumps(status, indent=2) + "\n")


def _summarize(result):
    if result is None:
        return None
    if isinstance(result, dict):
        keys = (
            "warning",
            "reason",
            "note",
            "summary",
            "stdout_tail",
            "error",
            "binary",
        )
        for k in keys:
            if k in result:
                return {k: result[k]}
        return {k: result[k] for k in list(result)[:1]}
    return result


# ---- main loop --------------------------------------------------------------


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest", default=str(HERE / "manifest.json"))
    ap.add_argument(
        "--once", action="store_true", help="run a single pass and exit (testing)"
    )
    args = ap.parse_args()

    manifest_path = Path(args.manifest)
    status_path = HERE / "status.json"
    started_at = time.time()

    log(f"supervisor starting, manifest={manifest_path}")
    while True:
        manifest = load_manifest(manifest_path)
        budget_hours = manifest.get("budget_hours", 12)
        poll_interval = manifest.get("poll_interval_seconds", 60)
        elapsed_hours = (time.time() - started_at) / 3600

        if elapsed_hours > budget_hours:
            changed = False
            for s in manifest["stages"]:
                if s["status"] == "pending":
                    s["status"] = "not_started_budget_exceeded"
                    changed = True
            if changed:
                save_manifest(manifest_path, manifest)
                write_status(manifest, status_path, started_at, budget_hours)
            log(f"wall-clock budget ({budget_hours}h) exceeded; stopping")
            break

        by_id = {s["id"]: s for s in manifest["stages"]}
        eligible = [
            s
            for s in manifest["stages"]
            if s["status"] == "pending" and deps_resolved(s, by_id)
        ]

        if not eligible:
            if all(s["status"] in TERMINAL for s in manifest["stages"]):
                log("all stages terminal; campaign complete")
                break
            log(f"nothing eligible this pass; sleeping {poll_interval}s")
            time.sleep(poll_interval)
            continue

        progressed = False
        for stage in eligible:
            manifest = load_manifest(
                manifest_path
            )  # pick up any external edits between stages
            stage = next(s for s in manifest["stages"] if s["id"] == stage["id"])
            if stage["status"] != "pending":
                continue  # edited or already advanced concurrently; skip

            handler = HANDLERS[stage["type"]]
            log(f"running stage {stage['id']} ({stage['type']})")
            try:
                new_status, info = handler(stage, manifest)
            except Exception as e:
                new_status, info = "failed", {"exception": repr(e)}

            if new_status == "pending":
                log(f"  {stage['id']}: not ready yet ({info})")
                continue

            # Reload before saving. A handler can run for over an hour, and the
            # manifest we hold was read before it started: saving that copy would
            # silently discard any edit to a still-pending stage made meanwhile,
            # which is exactly how a check-in revises future work.
            manifest = load_manifest(manifest_path)
            fresh = next(
                (s for s in manifest["stages"] if s["id"] == stage["id"]), None
            )
            if fresh is None:
                log(
                    f"  {stage['id']}: removed from the manifest while running; result not recorded ({new_status})"
                )
                continue
            stage = fresh
            stage["status"] = new_status
            stage["result"] = info
            stage["finished_at"] = time.strftime("%Y-%m-%dT%H:%M:%S%z")
            progressed = True
            save_manifest(manifest_path, manifest)
            write_status(manifest, status_path, started_at, budget_hours)
            log(f"  {stage['id']}: {new_status} -- {_summarize(info)}")

            if (time.time() - started_at) / 3600 > budget_hours:
                break

        if args.once:
            break
        if not progressed:
            time.sleep(poll_interval)

    log("supervisor exiting")


if __name__ == "__main__":
    main()
