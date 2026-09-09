# Archive ingest in Go (v0.6.x)

- **Status:** Draft
- **Layers:** LiDAR pipeline (L1, capture index), CLI, database
- **Target:** v0.6.x; index the capture archive through the Go capture index, and delete the Python that currently stands between an operator and it
- **Companion plans:** [lidar-scene-catalogue-publishing-plan](lidar-scene-catalogue-publishing-plan.md) owns archive-scale publishing; this plan owns getting the archive into the index correctly in the first place
- **Canonical:** [geographic-indexing.md](../lidar/architecture/geographic-indexing.md) for S2 conventions

## Motivation

Indexing the 189 captures on `/Volumes/lidar/lidar/s2` currently runs through
Python in `tools/s2-archive/`: a script to find recording blocks from filenames,
a script to stitch motion/static segments back into sites, and a hand-maintained
JSON of positions. None of that belongs in an operator's path. It exists because
the segmentation on the drive was produced **per capture file**, and the
segmenter's own help says not to do that:

> Several `--pcap` flags analyse the captures as one continuous stream, which
> keeps the background model settled across the file boundaries. Analysing each
> file separately restarts that model and reports the settling as motion.

The Python is a workaround for that mistake, not a missing feature. The Go
pipeline already classifies a whole session as one joined stream, through the
same engine the CLI uses. The work is to run the archive through what exists,
close three gaps it has, and retire both the scripts and the per-file JSON on the
drive.

## Current state

### Already in Go, and correct

| Capability                           | Where                                                |
| ------------------------------------ | ---------------------------------------------------- |
| Scan a volume, detect drift          | `internal/lidar/capindex` (`Indexer.Refresh`)        |
| Probe a capture's packet extent      | `capindex`, `CaptureStore.RecordProbe`               |
| Derive sessions from probed extents  | `CaptureStore.DeriveSessions`                        |
| Motion/static classification         | `internal/lidar/pcapsplit` (`Analyse`)               |
| Classify a **session** as one stream | `server.sessionMotionPass` → `lidar_capture_periods` |
| Join captures for replay             | `internal/lidar/capseq`, `readPCAPSequence`          |
| Publish a scene from a recording     | `velocity scene export`                              |

`sessionMotionPass` already sets `cfg.PCAPFiles` to every capture in the session,
so the background model stays settled across file boundaries. It is the same
`pcapsplit.Analyse` the CLI calls. The correct behaviour is present and wired to
`POST /api/lidar/capture/motion-pass`.

### In Python, and should not be

| Script                                 | What it does                                              | Why it exists                                        |
| -------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------- |
| `tools/s2-archive/deployments.py`      | Groups captures into recording blocks from filenames      | Nothing exposed block structure                      |
| `tools/s2-archive/build-site-index.py` | Stitches per-file segments into sites, attaches positions | The drive's segmentation is per file                 |
| `tools/s2-archive/map-marks.json`      | Hand-read positions                                       | Legitimate input, but belongs with the site it names |

### On the drive, and should be retired

`s2/analysis/` holds 142 per-file `segments.json` — the artefact that forces the
stitching. `s2/analysis-continuous/` holds the correct per-block output produced
while writing this plan. Neither should be an input to anything once the index
holds periods.

## Findings

| Area                    | Current state                                                                      | Severity | Release view                                                      |
| ----------------------- | ---------------------------------------------------------------------------------- | -------- | ----------------------------------------------------------------- |
| Per-file classification | Each file restarts the background model; its settling is reported as motion        | Blocker  | Classify per recording block, which `sessionMotionPass` does      |
| Site truncation         | 3 of 23 sites end early where a motion gap exceeds the stitcher's 180 s bridge      | High     | Two are the artefact; one is a real event the bridge cannot judge  |
| Bridge constant         | 180 s recovers the expected site count; 300 s merges genuinely separate sites      | High     | A tuned constant with no principled value; delete it, not tune it |
| Settling on long streams | A 27-capture stream misses a stop the per-file run finds, and lags the rest by ~4 min | High     | Re-derive the settling parameters for stream-length input          |
| Capture attribution     | `MotionPeriod` records times and frames, no captures; the JSON records none either | High     | Periods must name the captures they span                          |
| Recording blocks        | A day is not one stream: 9/1 restarts at 16:29 after a 176 s gap, sequence resets  | Medium   | Session derivation must split there, and be verified to           |
| Validity                | Nothing asserts a site is plausible; a 12-minute "20-minute site" passes silently  | Medium   | A site spanning fewer than four captures is a defect, not a site  |
| Position provenance     | Positions live in a JSON beside a script, not against the thing they locate        | Medium   | A period's site carries its own position, per the site model      |

### Evidence for the classification defect

Measured on this archive, per-file classification splits static stretches into
fragments at file boundaries: 9/2 and 9/3 yield 17 and 19 static fragments where
9 and 8 sites were recorded. Stitching across motion shorter than 180 s recovers
exactly 9 and 8 — and 300 s merges two separate sites — which is the signature of
a threshold compensating for an artefact rather than measuring anything.

9/1, classified here as continuous streams, needed no stitching at all: six
static stretches of 18 to 22 minutes, one per site.

The three short sites separate along the same line. s13 (9/2 13:20) and s21 (9/3
14:40) are truncated by motion gaps of 241 s and 221 s that each straddle a file
boundary, and the part on the far side — 99 s and 50 s, beginning at the new
file's first packet — is settling, not motion. Take it out and the real gaps are
141 s and 171 s, inside the bridge. s16 (9/3 10:35) is different: its 202 s gap
sits wholly inside one capture, well after that file's model had settled, so it
is a real motion event mid-site. Continuous classification fixes the first two by
construction. The third is a judgement no bridge constant can make, and is the
case for `--min-captures` reporting rather than silently repairing.

That expectation is only half borne out. Re-running 9/3's morning block as one
27-capture stream does **not** reproduce the per-file result: it reports two
static stretches where three were recorded, each beginning about four minutes
after the per-file analysis puts the stop, and it swallows the 10:35 site
(s16) into a single 68-minute opening motion. The operator's field map records a
10:35 site, so the per-file analysis is closer to the truth there and the
continuous run has a failure mode of its own — a long drive before the first
stop leaves the background model matched to motion, and it is slow to re-converge
when the platform finally stops.

Continuous classification is still the right default, because it removes an
artefact that is definitely wrong. But it is not a free win: the settling
parameters were chosen for a per-file run and have not been re-derived for a
two-hour stream. Workstream 2 must compare both classifications against the
field map on all three days before the per-file output is retired, not assume
the continuous one supersedes it.

## Design / approach

### 1. The archive goes through the capture index, not through scripts

Scan and probe the volume, derive sessions, run a motion pass per session. Every
step already exists and is reachable from `POST /api/lidar/capture/scan` and
`POST /api/lidar/capture/motion-pass`. What is missing is a way to ask for the
whole volume at once rather than session by session, and that is a CLI, not a new
pipeline:

```text
velocity lidar index --root /Volumes/lidar/lidar/s2   # scan, probe, sessions
velocity lidar classify --root /Volumes/lidar/lidar/s2 [--session ses-…]
velocity lidar sites --root /Volumes/lidar/lidar/s2 [--min-captures 4]
```

`classify` is a loop over sessions calling the existing motion pass. `sites`
reads `lidar_capture_periods` and prints or emits the static ones. No new
classification code.

### 2. A period names the captures it spans

`MotionPeriod` carries `start_ns`/`end_ns` but no captures, so every consumer
re-derives coverage from timestamps — the JSON exporter does not even try, and
the Python got it wrong for a whole day. Store it once, at derivation, where the
capture list is already in hand.

The session's files and their probed extents are both known to
`sessionMotionPass`, so coverage is an intersection, not a guess.

### 3. Sessions split where recording stopped

A recording block ends when the recorder is restarted: the sequence number resets
and a real gap appears. On 9/1 that is a 176-second gap at 16:29 with the
sequence going 45 → 1. Classification across that boundary is meaningless, and
`velocity lidar pcap-split` already refuses it — correctly — as "captures do not
form one continuous stream".

Session derivation must produce the same split so the motion pass never assembles
a stream the classifier will reject. This is a verification and, if needed, a
correction to `DeriveSessions`, not new machinery.

### 4. A site is a static period with a position

The site model landed for replay cases (`lidar_sites`, canonical pose distinct
from a case's sensor pose). A static period is the other thing that belongs at a
site. Linking them gives the archive a position without a parallel store:

- a static period may reference a site
- a site keeps its canonical pose, label, and derived S2 tokens
- `map-marks.json` becomes an import into that table, run once, not a file the
  index reads every time

### 5. Validity is checked, not assumed

The recording protocol is roughly twenty minutes per site, which is four or more
five-minute captures. A static period spanning fewer is either a truncation or
something that was not a site. `sites --min-captures` reports them rather than
letting a short period pass as a location worth publishing.

## Scope

### Workstream 1: Periods carry their captures

**Steps:**

1. Add capture coverage to `MotionPeriod` and its table, written at derivation.
2. Populate it in `sessionMotionPass` from the session's capture list and probed
   extents.
3. Surface it on `GET /api/lidar/capture/periods` and in the Captures page.
4. Backfill existing periods by intersection; there are few and they are cheap.

**Milestone:** v0.6.0

### Workstream 2: Whole-volume CLI

**Steps:**

1. `velocity lidar index --root` — scan, probe, derive sessions, report drift.
2. `velocity lidar classify --root` — motion pass per session, resumable, skipping
   sessions whose periods are current.
3. `velocity lidar sites --root` — static periods with duration, captures and site,
   with `--min-captures` and `--json`.
4. Verify session derivation splits on recorder restarts, with a test built from
   the 9/1 shape: sequence reset plus a gap of a few minutes.
5. Compare continuous against per-file classification on all three days, scored
   against the field map, before the per-file output stops being an input.

**Milestone:** v0.6.0

### Workstream 3: Sites and positions

**Steps:**

1. Let a static period reference a site; derive S2 tokens from the site's pose.
2. One-shot import of `map-marks.json` into sites, recording each position's
   source and confidence rather than flattening it to a number.
3. Scene publication reads the site's pose instead of `public_html/scene-sites.json`
   carrying its own copy.

**Milestone:** v0.6.1

### Workstream 4: Retire the workarounds

**Steps:**

1. Delete `tools/s2-archive/*.py` once `sites --json` produces the same index.
2. Stop reading `s2/analysis/`; the per-file JSON stays on the drive as history,
   not as an input.
3. Re-classify 9/2 and 9/3 as continuous streams so all three days are comparable.

**Milestone:** v0.6.1

## Phasing

**Phase 0 — correctness.** Workstreams 1 and 2. The archive can be indexed end to
end in Go, periods name their captures, and the short-site check runs. Done when
`velocity lidar sites --min-captures 4` reports nothing on a freshly classified
archive.

**Phase 1 — position.** Workstream 3, once coordinates are worth storing.

**Phase 2 — retirement.** Workstream 4, once Phase 0 output matches the committed
index.

## Risks

| Risk                                                             | Likelihood | Impact | Mitigation                                                                          |
| ---------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------- |
| Re-classifying changes site boundaries the published scenes used | Medium     | Medium | Scenes are derived; re-export is cheap and the VRLOGs are kept                      |
| Session derivation splits differently from the classifier        | Medium     | High   | One test fixture drives both, built from the 9/1 restart                            |
| Classifying the whole archive is slow                            | High       | Low    | Roughly 11 s per five-minute capture measured here; resumable and per session       |
| Backfilled capture coverage disagrees with a future derivation   | Low        | Medium | Backfill by the same intersection the writer uses, not a separate code path         |
| `map-marks.json` positions are treated as surveyed               | Medium     | High   | Import carries source and confidence; a read position is `operator`, not `surveyed` |

## Checklist

### Outstanding

- [ ] W1: capture coverage on `MotionPeriod`, written at derivation (`M`)
- [ ] W1: surface coverage on the periods endpoint and Captures page (`S`)
- [ ] W1: backfill coverage for existing periods (`S`)
- [ ] W2: `velocity lidar index --root` (`M`)
- [ ] W2: `velocity lidar classify --root`, resumable (`M`)
- [ ] W2: `velocity lidar sites --root --min-captures --json` (`M`)
- [ ] W2: session derivation splits on recorder restart, with a 9/1-shaped test (`M`)
- [ ] W2: score continuous against per-file on all three days before retiring per-file (`M`)
- [ ] W3: static period references a site; tokens derive from the site pose (`M`)
- [ ] W3: one-shot import of read positions into sites (`S`)
- [ ] W3: scene publication reads the site pose (`S`)
- [ ] W4: delete `tools/s2-archive/*.py` (`S`)
- [ ] W4: re-classify 9/2 and 9/3 as continuous streams (`S`)

### Accepted residuals (no action planned)

- [ ] The per-file `segments.json` on the drive stays as history; nothing reads it
- [ ] Positions read from a photograph are accurate to a few hundred metres, and
      are a starting point for a survey rather than one
- [ ] A real motion event mid-site still splits it; `--min-captures` reports the
      short period rather than the classifier guessing the two halves are one
