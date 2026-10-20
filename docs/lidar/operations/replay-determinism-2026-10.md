# Replays that start from nothing: the state a server carried between replays, October 2026

perf0002 found that a replay of one capture records different tracks depending on what the server
had replayed before it, by up to a fifth of the tracks, and named two L3 states from its lanes'
logs as the likely cause. This report isolates them (l3bg0020), finds a third, fixes all three and
tests the fix on a server that is never restarted (l3bg0021), and chooses the semantics of the
first settling check, which the fix had to make explicit.

- **Status:** Measured, October 18–19, 2026 (experiments
  [l3bg0020](../../../data/experiments/l3bg/l3bg0020.md) and
  [l3bg0021](../../../data/experiments/l3bg/l3bg0021.md)). The fix is in this report's PR; the
  first check keeps its current semantics
- **Layers:** L2 frame assembly in replay, L3 settling and the region store
- **Related:** [replay speed and analysis mode](replay-speed-and-analysis-mode-2026-10.md) (perf0002),
  [one L3 for every replay](replay-l3-consolidation-2026-10.md) (l3bg0023),
  [PCAP analysis mode](pcap-analysis-mode.md)

## Summary

- **Two L3 states made a replay depend on the server's history, as perf0002 read them.** On the
  unfixed code, each of the four server histories l3bg0020 set up landed on the outcome predicted
  from perf0002's sweep, by its settling log and by its tracks, at both sites:

  | History                                                       | Settling log                      | embarcadero-bay tracks | ashbury-downey tracks |
  | ------------------------------------------------------------- | --------------------------------- | ---------------------: | --------------------: |
  | New process, empty database                                   | converged at frame 59 (5.9 s)     |                    858 |                   382 |
  | New process, database holding this capture's snapshot         | restored by source path, frame 10 |                    876 |                   385 |
  | Process that replayed the other site, no snapshot of this one | settled at the 30 s ceiling       |                    774 |                   313 |
  | Both                                                          | restored by source path, frame 10 |                    876 |                   385 |

  The first is **the convergence state `ResetGrid` left in memory**: the settling check compares
  each cell's spread with its spread at the previous check, and a new process has no previous
  check, so its first one passes on a delta of zero; a reused process compared the new capture's
  spreads with the old capture's and then never met the threshold. The second is **the region
  store**: a settled replay persisted its region map and grid by source path, and any later replay
  of the capture against that database restored both ten frames in, whatever its own tuning.

- **A third mechanism appears only when the host stalls.** One of l3bg0020's new-process replays,
  the first on its server while the machine was swapping, differs from every other new-process
  replay of that capture in 319 of 9,552 frames. Its server logged three "Discarding incomplete
  frame" lines during it. The frame builder finalises a frame in progress once its last point is
  older than the buffer timeout by the wall clock, which live input needs when the sensor stops
  sending; a replay reader stalled longer than that had its rotation cut in two, and the remainder
  discarded.
- **Fixed, a replay now starts from nothing.** `ResetGrid` clears the convergence state; a PCAP
  replay neither restores a region snapshot nor persists one, except for a settling pass's own;
  and a replay's frames end only on their azimuth wrap. On one server, never restarted, on a
  database already holding both captures' old snapshots, three analysis replays of
  embarcadero-bay, two of ashbury-downey interleaved with them, and a 0.5x replay of
  embarcadero-bay were each identical row for row to a replay on a new process. That held under
  both first-check semantics.
- **The first check keeps its current semantics.** Making it explicit, the honest version (an
  unmeasured spread delta is not convergence) settles every replay at the 30 s ceiling, because
  the delta flattens near 0.005 against a threshold of 0.001; it scores HOTA 0.363 on kirk0
  against 0.376, more than the rule's 0.01 below, so the current version (the first check passes on
  coverage and confidence alone) stays, now deliberate rather than an accident of an empty slice.
  Whether the 0.001 threshold is ever met is l3bg0022.

## Method

**Replays.** Analysis-mode replays of the published corpus's embarcadero-bay (955 s, 9,552
rotations) and ashbury-downey (1,227 s, 12,273 rotations) captures, staged on the internal disk,
each a whole capture with no settling pass and the defaults' tuning, through
`replay-speed-sweep.py` on isolated servers, each with its own database, ports and VRLOG
directory. Every replay keeps the slice of its server's debug log that it wrote (settling,
restore and persist lines), and every replay is exported at stride 1 from its first frame and
compared with others row for row on capture time.

**l3bg0020, on the unfixed code** (`a8c99eba0`). Four server histories per case: a new process on
an empty database; the case replayed on a new process, then a new process on the same database
replays it again; a new process on an empty database replays the other site, then the case; and
the case, the other site and the case again on one process. Predictions for each were written
from perf0002's outcomes before the replays.

**l3bg0021, on the fixed code** (`497c1e2fd`, L3; `a7003dbb8`, the frame builder). Per arm, a new
process on an empty database replays each case once (the reference). Then one server, never
restarted, on a copy of a database that already held a region snapshot of each capture from the
unfixed code (embarcadero-bay settled at frame 59, ashbury-downey at frame 300), replays
embarcadero-bay, ashbury-downey, embarcadero-bay, ashbury-downey, embarcadero-bay in analysis mode,
then embarcadero-bay at 0.5x. The two arms are the first check's semantics: `first-check`, the
current behaviour, and `honest`, run from a build with the honest check forced on. Both are scored
on kirk0 and kirk1 offline, with the replay experiment `settling_honest` for the second, against
the reviewed pack's per-frame ground truth.

## Findings

### The fix, on a server that is never restarted

| Replay of the sequence          | first-check, against its reference | honest, against its reference | Settling log, every replay                                  |
| ------------------------------- | ---------------------------------- | ----------------------------- | ----------------------------------------------------------- |
| embarcadero-bay, 1st (analysis) | identical, 9,552 frames            | identical                     | first-check: converged at frame 59; honest: ceiling at 30 s |
| ashbury-downey, 1st             | identical, 12,273 frames           | identical                     |                                                             |
| embarcadero-bay, 2nd            | identical                          | identical                     |                                                             |
| ashbury-downey, 2nd             | identical                          | identical                     |                                                             |
| embarcadero-bay, 3rd            | identical                          | identical                     |                                                             |
| embarcadero-bay at 0.5x         | identical                          | identical                     |                                                             |

No replay restored or persisted a region snapshot, though the database held both captures'. The
first-check references are identical row for row to perf0002's new-process replays of the same
captures, made by an older binary: the fix changes nothing a new process does. Under the honest
semantics every replay lands on what perf0002 called the reused-process outcome (embarcadero-bay
53,008 track rows, ashbury-downey 36,037), which is what a settle at the ceiling is.

### The third mechanism

| Server, period                                              | Discarded incomplete frames |
| ----------------------------------------------------------- | --------------------------: |
| l3bg0020, unfixed, while the host was swapping              |                          14 |
| l3bg0020's rerun of one history, unfixed, host not swapping |                           1 |
| l3bg0021's first attempt, unfixed frame builder, swapping   |                           7 |
| l3bg0021's determinism runs                                 |                           0 |
| perf0002's four lanes                                       |               1, 0, 0 and 0 |

At its worst during these runs the host had 64 MB free and 10 of its 11 GB of swap in use; replays that take
2 to 3 minutes alone took 5 to 25. Only the replay with discards differs from its peers; the other
new-process replays of the same capture on the same binary, some of which ran as slowly, match
perf0002's row for row. Pausing a whole server process with `SIGSTOP` for 2 s every 15 s did not
reproduce the discards on either binary, since a paused process stops its reader and its timer
together; a page-faulting reader stalls alone. The unit test reproduces the cut directly: a
rotation whose reader stalls past the timeout comes out as two frames of 50 points each without
the fix and one of 100 with it.

### The first check's semantics, on kirk

| Arm           | kirk0 HOTA |  IDF1 | ID switches | Object-frame recall (car / bus / pedestrian) | kirk1 references covered | Settles at |
| ------------- | ---------: | ----: | ----------: | -------------------------------------------- | -----------------------: | ---------- |
| `first-check` |      0.376 | 0.350 |         124 | 0.587 (0.627 / 0.529 / 0.283)                |                 38 of 49 | frame 59   |
| `honest`      |      0.363 | 0.369 |          76 | 0.479 (0.55 / 0 / 0)                         |                 31 of 49 | 30 s       |

`first-check` at the fixed commit reproduces the defaults at the earlier commit exactly; the fix
does not change an offline replay, which always starts from a new manager. `honest` waits 30 s for
foreground, so kirk0's bus and pedestrians, which pass early, are missed entirely; fewer tracks
also means fewer identity switches and a higher IDF1.

## The rules, applied

**l3bg0020** (written 2026-10-18 before its replays). Predictions 1 to 4 hold on both cases by the
debug log and by the tracks. Embarcadero-bay's "both" history was rerun, its middle replay having
timed out under memory pressure the first time; the rerun matches store-only's tracks and rows
(876, 27,037) and, row for row, differs from it only in nine frames at 658.7 to 659.9 s around
one extra frame, where its server logged a discarded 374-point remainder: the third mechanism
again, on the unfixed frame builder, not the store. The prediction that names a third mechanism if it fails
did, for one replay, and named the frame builder's idle timer.

**l3bg0021** (written 2026-10-18 before its replays).

1. **Determinism**, required of both arms: holds for both. Every replay of a capture in the
   interleaved sequence, and the 0.5x replay, is identical row for row to a new process, and no
   replay restored or persisted a snapshot.
2. **Which semantics:** `honest` is 0.013 HOTA below `first-check` on kirk0, more than 0.01, so
   `first-check` is adopted. kirk1 has no per-frame ground truth; its reference coverage agrees
   (31 against 38).
3. **The reset and the store change are adopted**, since clause 1 holds for `first-check`. The
   spec's `restore_regions` start option is left out: nothing needs a replay to opt back into the
   store. The frame builder's change was not in the spec; it is the third mechanism l3bg0020's rule
   anticipated.

The spec's live step is a decision about a deployed sensor's start-up, which settles by the same
first check: with `first-check` kept, nothing changes for it.

## A fourth mechanism: the overlap end's order (October 20)

The default-off overlap end (`overlap_end_1` with `overlap_end_body` and `overlap_end_older`,
l5id0004's one track per vehicle) did not repeat. On the first segments of the 23 corpus sites
on October 20, `lidar-state-estimation-baseline` refused 10 of them under that arm because the
repeat run's baseline differed, while the same captures under the defaults repeated exactly.

The tracker judged its unmatched tracks in Go map order (`for trackID, track := range
t.Tracks` in Step 4 of `Tracker.Update`), which differs from run to run. The overlap end runs
inside that loop, and keeping the older track it can end the live track; once the live track is
gone, later coasting tracks no longer lie on it. With two coasting tracks on one live body, one
older and one younger than it, the older judged first ends the live track and the younger lives;
the younger judged first is ended, then the older ends the live track. Which track survived
therefore depended on map order. A smaller case of the same kind: in box mode the live list was
built from the `matched` map, so a coasting track lying on two live boxes took whichever came
first.

Step 4 now visits tracks in creation order (`sortedActiveTrackIDs`, creation sequence then ID,
the order association already uses), so the oldest coasting track is judged first, which is what
"keeping the older track" means; the box-mode live list is built in the same order.
`TestOverlapEndKeepOlderRepeats` builds the two-coasting-tracks case and requires 64 runs to agree
(it failed on the second run before the fix). On fulton-divisadero with the arm the harness
repeat failed before and passes after, as it does on laguna-eddy; under the defaults the
repeat-checked `tracking_baseline.json` is byte-identical before and after, as it must be: with the
overlap end off, each unmatched track's step changes only that track and counters, so order
cannot matter.

**What it means for earlier results.** l5id0004 and l5id0005 measured the arm before this fix,
so wherever two coasting tracks shared a live body their figures are one draw of the survivor
choice; this note does not establish how those runs treated a case whose repeat differed.
Rerunning the identity line's figures at this commit would say how much the draw mattered. For
l8fol0001's following screen it did not: replayed with the fix, all 23 first segments repeat, 18
screen identically to the first runs, and the pooled following figures move by under 0.3 %
(following opportunity (#805)).

## What this does not establish

- **That nothing else carries over.** The sequence covers two captures, the defaults' tuning and
  analysis and 0.5x pacing; a tuning file that sets Go-only options, or the classifier's state,
  are not exercised by it.
- **The third mechanism end to end.** It is shown by its occurrence and the unit test, not by a
  replay made to stall on demand.
- **That the first check's semantics are right.** They are the better of two on one site's
  labels; l3bg0022 asks whether the threshold should be met at all.

## Recommendations

1. **Run perf0003** on one server without restarts, after l3bg0023 decides the defaults' L3: with
   the server no longer carrying state, perf0002's pacing question can be asked as it meant to be.
2. **Run l3bg0022:** the honest criterion is never met on these captures, so convergence decides
   nothing and the duration ceiling decides every honest settle.
3. **Republish scenes only after both**, in one batch.

## Provenance

- **Code:** branch `claude/l3-consolidation-20261018`. l3bg0020 ran the server at `a8c99eba0`;
  l3bg0021's determinism ran `497c1e2fd` (`first-check`) and `497c1e2fd` with the honest check
  forced on in `settling_eval.go` (`honest`, a scratch build from a one-line patch, never
  committed); the frame builder fix is `a7003dbb8`. kirk replays and scores ran
  `lidar-state-estimation-baseline` and `lidar-ground-truth-eval` at `497c1e2fd`.
- **Captures:** `sf-street-speeds/raw/lidar/20260902T1235_80858-0fc_embarcadero-bay.pcapng` and
  `20260901T1225_80858-0ac_ashbury-downey.pcapng`, staged on the internal disk.
- **Host:** the development Mac (M1 Pro, 8 cores, 16 GB), October 18–19, 2026, shared with other
  work and for some hours short of memory, as above.
- **Raw output** (VRLOGs, per-replay results, settling logs, exported scenes, comparisons, the
  server databases) is outside the repository, under
  `/Volumes/banshee-results/velocity-campaign/consol-20261018/`.
