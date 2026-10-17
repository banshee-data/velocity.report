---
name: run-experiments
description: Unattended experiment run for a set time (12 hours by default). Picks the next most impactful LiDAR experiments the host can run, writes each one's rule before its replays, runs it on whatever captures the host has (any local PCAP, the NAS on arrow-worker, kirk0 or the Hugging Face dataset in the cloud), adopts passing candidates as defaults, and ships one PR per experiment with a report.
argument-hint: "[duration: 12h | 5h | 90m] [area or slug ...]"
---

# Skill: run-experiments

Run experiments unattended until the time is up: choose the questions that move the product most,
answer each under a rule written before its replays, make what passes the default, and leave a
report, a PR and a summary for each. The method is the one the box rotation and box alignment runs
used (geom0005 to geom0010, l5id0001 to l5id0005).

## Usage

```
/run-experiments                 # 12 hours
/run-experiments 5h              # a set duration: h and m, e.g. 90m or 2h30m
/run-experiments 6h l5id         # only this area's questions, or named slugs (l5id0006 geom0011)
```

## 1. The clock

Record the start (`date -u`) and the deadline. Stop starting new arms once the time left is less
than the next arm's estimate plus the time to write it up: keep the last hour, or a sixth of a run
shorter than six hours, for reports, PRs and the summary. Check the clock between steps, not only
at the end. Never wait with a foreground `sleep`: run long jobs in the background and wait with an
until-loop or the Monitor tool, and keep checking disk and the clock while they run.

## 2. Where the run is

Find the host before choosing anything: it decides the captures, the scratch space and where raw
output may go.

```bash
hostname; uname -sm; echo "${CLAUDE_CODE_REMOTE:-}"; df -h . /tmp
ls /Volumes/lidar/lidar/pcaps /Volumes/banshee-captures/lidar /mnt/captures/lidar 2>/dev/null | head
```

**Local** (the Mac, or any machine with captures attached).

- Captures: any PCAP present. `LIDAR_PCAP_DIR` from `local.mk` (on the Mac
  `/Volumes/lidar/lidar/pcaps`: S2 segments under `s2/`, whole placements under
  `sf-street-speeds/raw/lidar/`), the NAS at `/Volumes/banshee-captures/lidar`, and kirk0.
- Scratch: `~/vr-scratch/<run>/` on the internal disk.
- Raw output: archived to `/Volumes/banshee-results/velocity-campaign/<run>/`, the results share
  on `arrow.local`. LiDAR data goes on no other volume.

**arrow-worker** (the VM `bansheeworker` on the TrueNAS host `arrow`).

- Captures: the NAS over NFS, read-only, at `/mnt/captures/lidar`
  (`-pcap-root /mnt/captures/lidar -pcap-subdir s2`).
- Scratch and raw output: `/srv/banshee/evidence/<run>/`.
- Limits: 8 cores, 5.8 GB memory, about 17 GB free, and `/tmp` is a 2.9 GB tmpfs. Run one case
  at a time with `-evidence-per-case -discard-evidence`, and at most two lanes.

**Cloud** (`CLAUDE_CODE_REMOTE=true`, no volumes mounted).

- Captures: kirk0 in the repository
  (`git lfs pull --include internal/lidar/perf/pcap/kirk0.pcapng`, 200 MB), and the
  `banshee-data/sf-street-speeds` dataset on Hugging Face.
- Scratch: the session's working directory. Its disk is a fixed allowance and the container is
  reclaimed when the session ends, so raw output does not survive: every number a decision rests
  on goes into the report before the PR.

**Hugging Face in the cloud.** Check the network policy reaches it first
(`curl -sI https://huggingface.co | head -1`); if it does not, run kirk0 only and say so. A whole
placement is about 3 GB (the dataset's 24 come to 73 GB), so fetch only the captures the question
needs, one or two at a time, and delete each once its summaries are written. Install the CLI
with `pip install -U huggingface_hub` if `hf` is missing.

```bash
hf download banshee-data/sf-street-speeds --repo-type dataset \
  --include "raw/lidar/<stem>.pcapng" --local-dir "$SCRATCH/hf"
hf download banshee-data/sf-street-speeds --repo-type dataset \
  --include "labels/<stem>/**" --local-dir "$SCRATCH/hf"
# kirk0 is 20251205T1417_80858-75c_kirkham-9th
```

Cut five-minute segments from a placement with `cmd/tools/pcap-split` when the question is scored
on segments. A cloud host has no Pi, no NAS and no labels beyond kirk0's: choose
questions that kirk0 and a few placements can answer, and say in the report which corpus stood in.

**Locally, keep reads and writes on different drives.** A replay that reads its capture from a
volume and writes its evidence to the same one contends with itself. Read captures from the LiDAR
volume or the NAS and write to the internal disk. NAS captures double a replay's wall time against
the internal disk, and the USB LiDAR volume is slower still, so for a long run stage the run's
segments on the internal disk when there is room (`df -h /` first; keep 10 GB free). SQLite
evidence on the USB volume is seek-bound: never write evidence there. Ports 8080, 8081 and UDP
2369 may belong to a long-running server: scratch servers use 18080, 18081 and
`--lidar-live-udp-port 12369`.

## 3. Preflight

1. **A clean base.** `git fetch origin main`, then work in a worktree on a branch from
   `origin/main`: other sessions and campaigns write in the shared checkout. Read
   `git worktree list` and `ps` before touching anything that is not yours.
2. **Build through `make`**, never a bare `go build` (`make build-radar-local`, and the tools with
   the `pcap` tag). Build the tools once into `$SCRATCH/bin` at the run's commit and rebuild after
   each merge that changes them; record the commit in each report's provenance.
3. **Disk.** `df -h` on every volume the run touches. Start a watchdog that pauses the run's
   replays (`kill -STOP`) when the internal disk falls under 10 GB or a capture volume under
   20 GB, resumes them over 14 GB, and logs each change. A watchdog that kills them loses the work
   in flight.
4. **What is already known.** Read the memory notes of earlier runs, `data/experiments/DEPENDENCIES.md`
   (`go run ./cmd/tools/experiments next`), the Recommendations of the latest reports under
   `docs/lidar/operations/`, and the backlog items those name. Note which held-out segments each
   earlier run has already read and for which measure: a segment read for a measure is no longer
   held out for it.
5. **Open PRs.** `gh pr list --state open`: finish or rebase this run's own before starting new
   work, and leave other people's alone.

## 4. Choosing the next experiment

Rank the candidates and record the ranking in the run's notes before running the first:

1. **The size of the defect, as measured**, in the product a user sees: published boxes, tracks,
   counts and speeds. A defect the latest report measured large beats one only suspected.
2. **What it unblocks.** A question whose answer would ship a default, or end a line of work that
   is not paying, ranks above one that only refines a measurement.
3. **Runnable here.** Every `requires.capabilities` of the spec is on this host, and no step needs
   a person (`manual:label`, `manual:annotate`, `manual:review`, `manual:capture`, a Pi): those go
   in the final summary as decisions, not into the run.
4. **Cost.** Among equals, the cheapest first, so an early answer can redirect the rest.

Follow a result where it leads: when a candidate fails because the cause is upstream, the next
question is that cause. Do not run an answered question again without a new lever, and do not
loosen a rule after seeing a result.

## 5. Each experiment

1. **The spec, before any replay.** `go run ./cmd/tools/experiments new <area>` for the slug, then
   `data/experiments/<area>/<slug>.md` with its question, candidates, the tuning set and the
   held-out set, and **the rule written before the replays**: the target and how much it must move;
   the non-inferiority bounds on every other measure; the held-out set read for the first time and
   how much of the improvement must survive it; and which candidate is adopted if more than one
   passes. Commit the spec on its own first, so the history shows the rule predates the result.
   `go run ./cmd/tools/experiments validate` must pass.
2. **Parameter sets.** Each arm runs from a file in `config/experiments/` with its row in that
   directory's README, as `AGENTS.md` requires. A new option starts off by default. Where an
   option only a replay or Go test needs would move the defaults' fingerprint, prefer a Go-only
   field until it is adopted: each fingerprint move means recapturing the perf baselines.
3. **Code with its tests.** A new option ships with unit tests in the same commit, and the scorer
   gains any measure the rule needs before the replays, not after.
4. **Replays.** Run the tuning set, score it, and apply the rule's first clause. Read the held-out
   set only for candidates that pass. Run arms in parallel lanes up to the host's cores and
   memory, one case per lane, and track units with `go run ./cmd/tools/experiments units` and
   `mark`, so a restarted lane skips what is done.
5. **Raw output stays out of Git.** Evidence, rows, logs and JSON summaries live in scratch, and
   are archived (locally to banshee-results) as each run directory finishes; delete the local
   per-frame rows only once the archived copy compares equal. Never stage a `results` directory;
   run `make check-no-results` before every commit.
6. **The report.** In `docs/lidar/operations/`, as `AGENTS.md` describes: question, method, arms,
   the numbers the rule was applied to, per-case summaries where they decide it, what it does not
   establish, provenance (commit, parameter set paths and fingerprints, corpus and segments) and
   recommendations. Answer the spec (`status: answered`, `report`, `actual`), regenerate
   `make experiments-graph`, and add a devlog bullet.
7. **Adoption.** A candidate that passes its whole rule, held out, becomes the default in the same
   PR: `config/tuning.defaults.json` and every parameter set that carries the block, the CONFIG.md
   row, then the gates that the moved fingerprint trips. Recapture the Mac perf baselines
   (`make perf-baseline-all`) from a shared clone at the PR head (Go's VCS stamp ignores a
   worktree), and update kirk0's source hash in `tools/s2-archive/continuity-coverage.json`. A
   pass that a standing rule says needs more (labelled held-out ground truth, Pi timing, a
   decision) stays off with the reason in the report, and the missing gate goes in the summary.
   Later experiments in the run start from the new defaults.
8. **The PR.** One per experiment, on its own branch, with the PR template filled from the diff.
   Wait for CI, fix what fails, address review comments, and squash-merge once CI is green and the
   threads are resolved. Leave open, with the reason in the summary, any PR that adds a migration,
   changes data on a deployed device, or publishes outside the repository. If pushing is refused,
   leave the commits on the branch and say so.

## 6. Guardrails

- Never open, migrate or write the repository root's `sensor_data.db`: copy it (`cp -c`) first.
- Never `--no-verify`, `rm -rf`, `git reset --hard` or a bare `git stash`. Commit with explicit
  paths.
- Nothing outward-facing without the user: no scene-asset publish, no Hugging Face upload, no
  change to the deployed Pi (`velocity.local`).
- Physical claims need physical evidence: a replay measures what the pipeline drew, not where the
  vehicle was. Reports say which.

## 7. The end of the run

Archive what remains, stop the watchdog and any lanes, and remove this run's worktrees. Write a
memory note for the run: what merged, what is open and why, where scratch and the archive are,
which held-out segments were read for what, and the traps met. Then give the user a summary: what
changed in the product, a table of experiments with each rule's result, what was found, the
decisions only they can make, the PRs left open, and the disk at the end.
