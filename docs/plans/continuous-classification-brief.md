# Brief: why continuous classification reports more motion

- **Status:** Open investigation, ready to hand over
- **Layers:** LiDAR pipeline (L3 background model), `pcapsplit`
- **Parent plan:** [archive-ingest-in-go-plan](archive-ingest-in-go-plan.md), Workstream 2
- **Blocks:** retiring the per-file `segments.json` on the archive volume
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md) for how a capture is classified

## The question

Analysing a recording block as one continuous stream is supposed to be strictly
better than analysing its five-minute captures one at a time: the background
model stays settled across file boundaries instead of restarting and reporting
its own settling as motion. On 9/1 it is better. On 9/3 it is worse, and the
task is to find out why before the per-file output stops being an input.

## What was measured

Same binary, same parameters — `--settling-sec 75 --motion-trigger-sec 5
--max-motion-gap-sec 45 --min-segment-sec 10` — on two recording blocks:

| Block                 | Captures | Static  | Opening motion | Sites found | Sites recorded |
| --------------------- | -------- | ------- | -------------- | ----------- | -------------- |
| 9/1 block0 (222 min)  | 45       | 55.0 %  | 10.6 min       | 6           | 6              |
| 9/3 block0 (132 min)  | 27       | 25.5 %  | **68.4 min**   | 2           | 3              |

For comparison, the archive's own per-file analysis of whole days: 9/2 is 56.2 %
static, 9/3 is 44.1 %. The protocol was the same on all three days — drive to a
junction, park about twenty minutes, drive on — so a block should come out near
half static. 9/3 continuous comes out at a quarter.

The three sites the per-file analysis finds in that window are 10:35 (11.7 min),
11:05 (20.0 min) and 11:35 (20.0 min). Continuous finds two:

```
motion 10:00:51 → 11:09:15   68.4 min
static 11:09:15 → 11:26:11   16.9 min
motion 11:26:11 → 11:39:36   13.4 min
static 11:39:36 → 11:56:24   16.8 min
```

Two things are wrong, and they may be one thing:

1. **The 10:35 site is gone.** It is absorbed whole into the opening motion. The
   operator's field map records a site there ("Van Ness near Market, Civic
   Centre"), so its absence is a miss, not a correction.
2. **Both surviving statics start about four minutes late** — 11:09 against
   11:05, 11:39 against 11:35 — while their ends line up. The recording is not
   losing time at the end of a stop, only at the beginning.

That asymmetry is the strongest clue in the data. Whatever is wrong takes about
four minutes to recover from after the platform stops, and on the 10:35 site —
the shortest of the three — it apparently never recovers at all.

## Hypotheses, most likely first

**H1 — the background model is converging from a moving platform, and the
recovery time scales with how long it drove first.** The model is built during
settling and adapts thereafter. On 9/1 the block opens with 10.6 minutes of
driving before the first stop; on 9/3 it drives about 34 minutes. A model that
has absorbed 34 minutes of a changing scene has a wide variance estimate, so a
stationary street's returns still read as foreground, and the frame keeps
scoring as motion until the estimate tightens. This predicts the four-minute
lag, predicts that a longer preceding drive makes the lag worse, and predicts
that the shortest stop is the one that disappears. It is testable directly.

**H2 — `--settling-sec 75` is the whole story.** The settling window is 75
seconds from the first packet. On both blocks the platform is moving throughout
it, so the model starts wrong in both cases; the difference would then be only
how long it takes to recover. Distinguishing H2 from H1 matters: if it is H2 the
fix is to settle on a detected stop rather than on a fixed prefix.

**H3 — the parameters were tuned against five-minute files.** Every default here
was chosen when a stream was 300 seconds long. `--max-motion-gap-sec 45` and
`--motion-trigger-sec 5` may simply not mean the same thing over 132 minutes.

**H4 — there was no 10:35 site and the per-file run invented it.** Against: the
field map records one, and the per-file analysis gives it 11.7 minutes. Cheap to
settle by looking at the frames, and worth settling first so the rest of the
work is not chasing a phantom.

**Excluded.** The tuning configuration is not the cause. The only parameter that
differs between the good and bad runs is `pipeline.frame_budget_ms`, which is
read solely by `lidarbench` for reporting and never reaches the pipeline
(`internal/config/profile.go:155`, `internal/lidar/lidarbench/lidarbench.go:380`).
It changes the config hash and nothing else.

## Tasks

1. **Settle H4 first.** Confirm from the frames that the platform is stationary
   between 10:35 and 10:47 on 9/3. If it is not, stop: the per-file result is
   the artefact and the rest of this brief is moot.
2. **Instrument the decision.** `pcapsplit.Analyse` currently emits a verdict per
   segment. Emit the per-frame statistic it decides on — foreground fraction, or
   whatever the motion trigger reads — so a 68-minute motion segment can be read
   as a curve. Without this the rest is guesswork.
3. **Sweep the settling window** on 9/3 block0: 75 s (baseline), 300 s, 600 s,
   and a run seeked to start at the first stop. If starting at a stop recovers
   all three sites, H1/H2 are confirmed and the fix is a settling policy, not a
   constant.
4. **Measure the convergence lag against preceding motion duration** across every
   block on all three days. If the lag is a function of the preceding drive, it
   can be corrected for; if it is not, H1 is wrong.
5. **Run 9/2 continuously.** It has never been run this way, and it is the third
   data point that decides whether 9/1 or 9/3 is the outlier.
6. **Score both classifications against the field map.** This is the acceptance
   criterion for the parent plan: 23 recorded sites with known clock times in
   `tools/s2-archive/site-index.json` and `map-marks.json`. A classification is
   better when it recovers more of them at the right time, not when it produces
   fewer segments.

## Where things are

| Thing                          | Path                                                     |
| ------------------------------ | -------------------------------------------------------- |
| Classifier                     | `internal/lidar/pcapsplit`                               |
| Background model               | `internal/lidar/l3grid`                                  |
| Session-level caller           | `server.sessionMotionPass` (`capture_motion_pcap.go`)    |
| Per-file analysis (the archive) | `/Volumes/lidar/lidar/s2/analysis/*/segments.json`       |
| Continuous runs so far          | `/Volumes/lidar/lidar/s2/analysis-continuous/`           |
| Site index and field marks      | `tools/s2-archive/`                                      |

A 27-capture block takes about 20 minutes of wall clock to analyse, and a
45-capture block about 32, so a parameter sweep is an afternoon rather than a
coffee break. Plan the sweep in one batch.

## What a good answer looks like

A statement of which classification is correct for the 9/3 morning and why,
supported by the per-frame statistic rather than by segment counts; a settling
policy that recovers all three sites without merging separate ones; and the
scoring run from task 6 showing the chosen policy recovers at least as many of
the 23 recorded sites as the per-file analysis does.
