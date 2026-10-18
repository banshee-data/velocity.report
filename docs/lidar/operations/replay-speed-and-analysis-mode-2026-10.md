# Replay speed and analysis mode for published scenes, October 2026

Published scenes are replayed at half speed, which costs a 24-site batch about 18 hours. Analysis
mode replays as fast as the pipeline takes packets and is meant to record every frame and lose
nothing. This experiment asked whether it does, whether pacing changes what a replay records, and
whether 0.5x is the right ratio if pacing stays.

- **Status:** Measured, October 18, 2026 (experiment
  [perf0002](../../../data/experiments/perf/perf0002.md)). Nothing shipped changes: the rule
  written before the replays keeps 0.5x, and the reason it does is not speed
- **Layers:** L1 replay pacing, L3 replay override, the server's state between replays
- **Related:** [PCAP analysis mode](pcap-analysis-mode.md),
  [scene publishing](../../../tools/s2-archive/README.md#replay-speed)

## Summary

- **Speed makes no difference to what a replay records while the replay keeps its pace.** On a
  freshly started server, analysis mode, 0.25x and 2x replays of the same capture produce the same
  scene, row for row, on all three sites, and two fresh analysis replays agree with each other. A
  replay that falls behind its pace differs in a small share of frames: embarcadero-bay at 4x,
  which took 5.8 minutes for a nominal 4, in 179 of 9,552.
- **The server's previous replay does.** A replay that follows another on the same server
  records something else. The divergence starts within the first second of capture time, and it
  moves track rows by up to 22 % (columbus-broadway, 84,175 against 102,328) and confirmed
  tracks by up to a fifth (ashbury-downey, 313 against 382). The pre-registered arms ran several
  replays per server, so their spread across speeds was this carry-over, not pacing. A scene batch
  replays every site on one server, so **each published scene depends on the site replayed
  before it**.
- **Analysis mode is about 17 times faster** than 0.5x: 1.8, 2.4 and 4.1 minutes against 32, 41
  and 68. Every replay at every speed processed exactly the capture's rotations, with no frame
  dropped and none throttled.
- **The rule could not answer its question, and keeps 0.5x by default.** Its clauses were applied
  to the pre-registered replays, which ran several captures in turn on each server, so every
  comparison mixed speed with the previous replay. Analysis mode's failures were all that mix: its
  two embarcadero-bay repeats followed different captures, and the x0.25 replays it was measured
  against were affected by their own predecessors. Nothing here shows analysis mode losing frames
  or being nondeterministic: on fresh servers it processed every rotation and repeated exactly.
  The rule is not loosened after the fact. 0.5x stays until the question is asked again on fresh
  servers, where on this evidence analysis mode looks likely to pass.
- **Settling first does not remove the carry-over.** Settled analysis replays of one capture, on a
  fresh server and after another site, still differ (ashbury-downey 385 against 319 confirmed
  tracks), and settling changes the result against an unsettled replay. Whatever survives also
  survives the settling pass's grid reload and tracking reset.
- **The L3 override paced replays substitute more than doubles foreground**: 2.4 to 5.8 clusters a
  frame at ashbury-downey, 3.1 to 7.8 at embarcadero-bay, 5.6 to 11.6 at columbus-broadway, and 1.4
  to 1.6 times the confirmed tracks. Every scene published at 0.5x ran it, and its header records
  the tuning file's hash, not the L3 it ran.

## Method

**Cases.** The three published-corpus captures the spec names, the files `publish-scenes.py`
replays: ashbury-downey (1,227 s, 12,273 rotations), embarcadero-bay (955 s, 9,552) and
columbus-broadway (2,033 s, 20,330). The rotation counts are from `pcap-split -dry-run`, read
offline. The captures were staged on the internal disk so every replay read the same bytes from
the same disk. VRLOGs were written to the LiDAR volume, never to the disk a capture was read from.

**Arms.** Each replay is one capture whole, as `publish-scenes.py` sends it: `analysis_mode: true`
(grid preserved), no settling pass, and the arm's speed. Every replay names its tuning file, so a
server never carries one arm's parameters into the next.

| Arm                  | Speed mode, ratio | L3                                                                |
| -------------------- | ----------------- | ----------------------------------------------------------------- |
| `analysis`           | analysis          | the defaults' L3                                                  |
| `analysis-replay-l3` | analysis          | `config/experiments/replay-l3-overrides.json`: the substituted L3 |
| `x0.25`              | scaled, 0.25      | the defaults' L3 (`keep_tuning_l3`)                               |
| `x0.5`               | scaled, 0.5       | the defaults' L3 (`keep_tuning_l3`)                               |
| `x0.5-replay-l3`     | scaled, 0.5       | the substituted L3: today's published configuration               |
| `x1`                 | realtime          | the defaults' L3 (`keep_tuning_l3`)                               |
| `x2`, `x4`           | scaled, 2 and 4   | the defaults' L3 (`keep_tuning_l3`)                               |

`keep_tuning_l3` is the replay option this experiment added: without it a scaled or realtime
replay substitutes five L3 values for the tuning file's (closeness 2.0 against 3, safety margin
0.30 m against 0.15, neighbour confirmation 5 against 3; seed from first observation and noise
relative 0.02 unchanged).

**Running.** Paced arms ran on four servers at once, each with its own ports, database and VRLOG
directory, pulling units from one queue; a paced replay's wall time is set by its ratio, and the
frame accounting shows whether one fell behind. The analysis arms then ran one at a time on one
server, with the other three stopped, because their wall time is a measurement. `analysis` and
`x0.5` ran twice. Each server ran several units in turn; which unit followed which is in the run's
lane logs, and it turned out to matter (below).

**Measures.** `tools/s2-archive/replay-speed-sweep.py` drove every unit. Frames come from
`vrlog-check` (VRLOG frames, background snapshots, empty frames) and from the server's run record
(frames processed, total clusters). The VRLOG records only the clusters no track took, so
clusters per frame here are the run record's total over frames processed. Throttled frames come
from the throttle's own count in the server's debug log, and dropped frames from its
`[PCAP quality]` line. Confirmed tracks, track rows and track life are from `vrlog-analyse report`.
Row-for-row agreement compares every frame of two replays exported at stride 1, keyed by capture
time, with track ids matched one to one: ids are drawn per run, and the published export's
`--start-frame 400` counts VRLOG frames, which include background snapshots taken on the wall
clock, so two published exports of one capture can start a frame apart and, at stride 2, hold
alternate frames.

**The diagnostic, after the rule was applied.** The rule's arms could not tell speed from what a
server had replayed before, so eight further replays were run, outside the pre-registered arms:
analysis replays on a server started fresh for each, two analysis replays each following another
site's on the same server, and 2x replays on fresh servers.

## Findings

### Frames

Every replay at every speed processed exactly the capture's rotations, recorded none empty through
the throttle, and logged no dropped frames, with one exception: embarcadero-bay's first analysis
repeat processed 9,553 frames against 9,552 rotations (one frame split at a boundary; it holds two
frames the second repeat does not and lacks one it holds). The throttle the spec expected to thin
x4 never acts on these replays: it is skipped whenever the grid is preserved, which
`publish-scenes.py` always asks for, as the note added to the spec before the replays said.
Three paced replays fell behind their pace and lost no frame doing so: embarcadero-bay at 4x (5.8
minutes against 4.0 nominal, alone on the host) and columbus-broadway at 2x and 4x (24.8 and 21.4
against 17.0 and 8.5, with four servers sharing the host).

### Content and cost, per arm

Clusters per frame, confirmed tracks and track rows; wall time in minutes. Rows marked † started on
a freshly started server (see the diagnostic).

**ashbury-downey** (12,273 rotations)

| Arm                | Wall | Clusters / frame | Confirmed tracks | Track rows | Life p50 / p90 (s) |
| ------------------ | ---: | ---------------: | ---------------: | ---------: | -----------------: |
| analysis r1        |  2.4 |            2.435 |              353 |     27,065 |         4.6 / 18.7 |
| analysis r2        |  2.4 |            2.435 |              353 |     27,065 |         4.6 / 18.7 |
| analysis-replay-l3 |  2.4 |            5.778 |              544 |     66,100 |         6.7 / 32.5 |
| x0.25 †            | 82.1 |            2.538 |              382 |     28,794 |         4.0 / 22.4 |
| x0.5 r1            | 41.1 |            2.569 |              385 |     29,147 |         4.1 / 22.4 |
| x0.5 r2            | 41.1 |            2.569 |              385 |     29,147 |         4.1 / 22.4 |
| x0.5-replay-l3     | 41.1 |            5.512 |              535 |     63,144 |         6.6 / 27.9 |
| x1                 | 20.8 |            2.049 |              313 |     23,026 |         4.6 / 14.1 |
| x2                 | 10.4 |            2.569 |              385 |     29,147 |         4.1 / 22.4 |
| x4                 |  6.1 |            2.569 |              385 |     29,147 |         4.1 / 22.4 |

**embarcadero-bay** (9,552 rotations)

| Arm                | Wall | Clusters / frame | Confirmed tracks | Track rows | Life p50 / p90 (s) |
| ------------------ | ---: | ---------------: | ---------------: | ---------: | -----------------: |
| analysis r1        |  1.8 |            3.086 |              876 |     27,037 |          3.7 / 9.6 |
| analysis r2        |  1.8 |            3.086 |              876 |     27,037 |          3.7 / 9.6 |
| analysis-replay-l3 |  1.8 |            7.779 |            1,235 |     69,908 |         5.1 / 15.2 |
| x0.25              | 63.8 |            3.086 |              876 |     27,037 |          3.7 / 9.6 |
| x0.5 r1            | 32.1 |            3.425 |              774 |     30,293 |         4.3 / 10.7 |
| x0.5 r2            | 32.1 |            3.425 |              774 |     30,293 |         4.3 / 10.7 |
| x0.5-replay-l3     | 32.1 |            8.514 |            1,158 |     76,189 |         5.3 / 17.4 |
| x1                 | 16.1 |            3.537 |              789 |     31,278 |         4.4 / 12.3 |
| x2                 |  8.1 |            4.002 |              791 |     35,404 |         4.6 / 16.3 |
| x4 †               |  5.8 |            2.982 |              858 |     26,045 |          3.6 / 9.4 |

**columbus-broadway** (20,330 rotations)

| Arm                |  Wall | Clusters / frame | Confirmed tracks | Track rows | Life p50 / p90 (s) |
| ------------------ | ----: | ---------------: | ---------------: | ---------: | -----------------: |
| analysis r1        |   4.1 |            5.625 |            2,535 |    102,328 |         3.9 / 13.0 |
| analysis r2        |   4.1 |            5.625 |            2,535 |    102,328 |         3.9 / 13.0 |
| analysis-replay-l3 |   4.1 |           11.624 |            4,160 |    216,609 |         4.5 / 17.2 |
| x0.25 †            | 135.8 |            4.852 |            2,576 |     84,175 |         3.6 / 10.6 |
| x0.5 r1 †          |  68.1 |            4.852 |            2,576 |     84,175 |         3.6 / 10.6 |
| x0.5 r2            |  68.1 |            5.145 |            2,421 |     93,944 |         3.8 / 12.0 |
| x0.5-replay-l3     |  68.1 |           11.475 |            4,141 |    210,656 |         4.4 / 17.2 |
| x1                 |  34.1 |            5.018 |            2,580 |     86,193 |         3.6 / 11.2 |
| x2                 |  24.8 |            5.625 |            2,535 |    102,328 |         3.9 / 13.0 |
| x4                 |  21.4 |            5.625 |            2,535 |    102,328 |         3.9 / 13.0 |

Content does not follow speed. The arms fall into a few exact outcomes per site that cut across
speeds: at ashbury-downey x0.5, x2 and x4 agree row for row; at columbus-broadway x0.25 and the
first x0.5 repeat agree row for row, as do the two analysis repeats with x2 (10 frames apart) and
x4 (45); and columbus-broadway's two x0.5 repeats disagree in 19,969 of 20,330 frames. Where two
replays disagree, the first differing frame is 0.9 s into the capture in every pair checked but
four, which part at 5.9 s, 284 s, 296 s and 487 s.

### Determinism

| Pair                             | ashbury-downey | embarcadero-bay   | columbus-broadway    |
| -------------------------------- | -------------- | ----------------- | -------------------- |
| analysis r1 against r2           | identical      | 194 frames differ | identical            |
| x0.5 r1 against r2               | identical      | identical         | 19,969 frames differ |
| analysis on fresh servers, twice | (one run)      | identical         | (one run)            |

### The diagnostic: fresh servers and predecessors

| Replay                                          | Agrees row for row with                                                                                    |
| ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| embarcadero-bay, analysis, fresh server (twice) | each other, and 2x on a fresh server; not the sweep's x4 †, which fell behind its pace (179 frames differ) |
| embarcadero-bay, analysis, after ashbury-downey | not the fresh replays (3,270 frames differ); the sweep's x0.25 and analysis r2                             |
| ashbury-downey, analysis, fresh server          | 2x on a fresh server, and the sweep's x0.25 †                                                              |
| ashbury-downey, analysis, after embarcadero-bay | not the fresh replay (9,152 frames differ); the sweep's x1, which also followed embarcadero-bay            |
| columbus-broadway, analysis, fresh server       | the sweep's x0.25 † and x0.5 r1 †                                                                          |

On a fresh server, then, analysis, x0.25 and 2x give one result per site, and that result is
reproducible. After another replay the result changes, to one of a few recurring outcomes, at any
speed. A paced replay that cannot keep its pace departs from the result by a few frames in a
hundred or fewer: 179 of 9,552 at embarcadero-bay 4x, and 10 and 45 of 20,330 at
columbus-broadway 2x and 4x, against the analysis replays they otherwise match.

### The settling pass, after the rule

The scenes are published without the settling pass (`settle_before_recording`), because it trains
the background on the traffic the recording exists to show. Whether settling first would remove
the dependence on the previous replay was asked after the rule was applied, with four more
replays: analysis mode with the settling pass, on a fresh server and again after the other site.

| Replay (settled first)                | Confirmed tracks | Track rows | Clusters / frame, both passes |
| ------------------------------------- | ---------------: | ---------: | ----------------------------: |
| embarcadero-bay, fresh server         |              878 |     27,203 |                         3.044 |
| embarcadero-bay, after ashbury-downey |              791 |     31,318 |                         3.482 |
| ashbury-downey, fresh server          |              385 |     29,190 |                         2.556 |
| ashbury-downey, after embarcadero-bay |              319 |     23,965 |                         2.092 |

Tracks and rows are the recorded pass's; the run record counts clusters over both passes. The
settled pairs disagree as much as the unsettled ones (8,729 and 9,361 frames differ),
and ashbury-downey after embarcadero-bay lands near the unsettled replay after the same
predecessor (319 tracks against 313). Settling also moves the fresh result itself (embarcadero-bay
858 to 878 tracks, ashbury-downey 382 to 385) and doubles the replay's time (3.4 and 4.4 minutes).
So settling does not clear the carried state, which survives the pass's reload of the settled grid
and its reset of tracking: that points away from the L3 grid and the tracker as where it lives.

### Wall time

Analysis mode took 1.8, 2.4 and 4.1 minutes; x0.5 took 32.1, 41.1 and 68.1: 5.6 %, 5.8 % and
6.0 % of x0.5's time. The rule's bound is a quarter. Analysis replays share their host with any
other work; these ran alone on the experiment's side, with other sessions' work on the machine.

## The rule, applied

The rule was written before the replays (2026-10-18). Analysis mode is adopted for publishing if
all four of its clauses hold on every case.

| Clause                                                  | Result                                                                                                                          |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| VRLOG frames equal rotations, none throttled or dropped | **Fails by one frame**: embarcadero-bay r1 processed 9,553 of 9,552                                                             |
| Its two repeats agree row for row                       | **Fails**: embarcadero-bay's repeats differ in 194 frames                                                                       |
| Within 2 % of x0.25's clusters, tracks and rows         | **Fails** on ashbury-downey (rows −6.0 %, tracks −7.6 %) and columbus-broadway (rows +21.6 %); holds on embarcadero-bay (equal) |
| At most a quarter of x0.5's wall time                   | Holds (about 6 %)                                                                                                               |

Under the rule analysis mode is not adopted. The fallback names the fastest paced ratio within 2 % of x0.25 on
every case: none qualifies (x0.5 is 12 % from x0.25 at embarcadero-bay, x1 20 % at
ashbury-downey, x2 and x4 22 % at columbus-broadway), so 0.5x keeps its place under the rule's
last clause.

What the rule measured was not what it was written to measure. Every clause that failed, failed
on the difference between replays that followed different predecessors on their server, and the
diagnostic shows that difference vanishes, at every speed that keeps its pace, when each replay
starts on a fresh server. The extra frame came from a replay that followed another; every
fresh-server replay processed exactly the capture's rotations. So these are failures of the test,
not findings against analysis mode. The rule is not loosened here: the verdict stands because it
was the rule, and the next run asks the question again on fresh servers.

## Interpretation

Pacing was the hypothesis and is not the variable. The pipeline's background warm-up and freeze
already run on capture time, and the throttle never acts on a grid-preserving replay, so a replay
that keeps its pace records what an analysis replay records, and on a fresh server it does. The
one speed effect left is a paced replay that falls behind. A likely cause, not tested here, is
that the frame builder still times its buffer and cleanup on arrival, which a replay running late
sees differently. That is a reason to
prefer analysis mode, which has no pace to fall behind.

Something survives from one replay to the next. `resetAllState` clears the frame builder, the L3
grid and the tracker before every replay; it does not clear what the tracking pipeline holds
outside them (the classifier, the pipeline's own per-frame state, and anything else built once at
startup), nor anything a tuning file leaves set when the next file does not name it. The DBSCAN
subsample is seeded from each frame's points, so it is not the cause. Which state it is, this run
does not establish. That the first divergence is usually 0.9 s into the capture says it acts from
the replay's first frames.

For published scenes this means the scene a site gets depends on which site the batch replayed
before it, and a single site republished alone gets a third result. The October 17 rerun that
matched the September recording would have matched only if its predecessor matched too.

The L3 override is the larger effect on content, and it is a tuning question, not a speed one:
closeness 2 against 3 roughly doubles the foreground and clusters, adds 40 to 65 % confirmed tracks
and lengthens their lives. Published scenes have all run it, and they record the tuning file's
hash rather than the L3 they ran.

## What this does not establish

- **Which replay is right.** None of these sites is labelled. Agreement between replays says the
  result is reproducible, not that its tracks are correct.
- **Which state carries over.** The diagnostic shows that a predecessor changes the result and a
  fresh server removes the change; it does not name the state.
- **That the fresh-server result is what a long-running live server sees.** A deployed sensor runs
  live input continuously; this concerns replays only.
- **Three sites.** A quiet residential corner, the shortest stretch and a busy junction. Busier or
  longer captures could reveal a speed effect these did not, though none of them showed one.
- **The analysis replays' wall time on a quiet machine.** Other sessions were running on the host;
  the times are an upper bound for this machine.

## Recommendations

1. **Find and clear the state that survives between replays**, so every replay starts from what a
   fresh server starts from. Until then, a batch that wants reproducible scenes should start each
   scene on a fresh server. Settling first is not a substitute: it does not clear the carried
   state, and it trains the background on the traffic the scene shows.
2. **Then ask perf0002's question again on fresh servers** (analysis twice and x0.25 per case,
   about two and a half hours for these three). On fresh servers the diagnostic's analysis
   replays matched x0.25 at two sites and 2x at all three, so analysis mode looks likely to pass,
   which would cut a 24-site batch from about 18 hours to about one.
3. **Record the L3 a replay ran in its VRLOG header**, not only the tuning file's hash, so a scene
   that ran the override says so. Whether published scenes should keep the override is a tuning
   decision for an `l3bg` spec, as the spec said.
4. **Export published scenes by capture time, not VRLOG position.** `--start-frame 400` counts
   background snapshots taken on the wall clock, so two exports of one capture can start a frame
   apart and, at stride 2, hold alternate frames.

## Provenance

- **Code:** branch `claude/perf0002-replay-speed`; the replays ran the binary built at `53f80c92`
  (`velocity-report v0.5.1-pre45`), with `keep_tuning_l3` added in `a7118201`. The driver is
  `tools/s2-archive/replay-speed-sweep.py` at the branch head.
- **Tuning:** `config/tuning.defaults.json` (fingerprint `c2a4fe43ac43`) and
  `config/experiments/replay-l3-overrides.json`, each named in every replay's start request.
- **Captures:** `sf-street-speeds/raw/lidar/20260901T1225_80858-0ac_ashbury-downey.pcapng`,
  `20260902T1235_80858-0fc_embarcadero-bay.pcapng` and
  `20260902T1320_80858-0f4_columbus-broadway.pcapng`, copied unchanged to the internal disk.
- **Host:** the development Mac (M1 Pro, 8 cores, 16 GB), October 18, 2026, 00:44 to 06:10 PDT.
- **Raw output** (VRLOGs, per-unit results, analyses, scenes, server and lane logs, the tables and
  comparisons) is archived outside the repository at
  `/Volumes/banshee-results/velocity-campaign/perf0002-20261018/`. Lane 1's server database was
  replaced during the diagnostic; its run-record totals for the sweep's units are kept in the
  archived `table.json`.
