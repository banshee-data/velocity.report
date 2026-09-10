# Motion/static parameter tuning (v0.6.x)

- **Status:** Draft — investigation designed, not run
- **Layers:** LiDAR pipeline (L3 background model), `pcapsplit`, capture index
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md) for how a capture is classified
- **Companion:** [static-sensor-nudge-tolerance-plan](static-sensor-nudge-tolerance-plan.md) is one case this must not break; [continuous-classification-brief](continuous-classification-brief.md) is the open question about stream length

Implementation references below describe [PR #569](https://github.com/banshee-data/velocity.report/pull/569) and its local archive experiments. Capture indexing, session classification, and multi-file replay remain branch work until that PR merges. This investigation document does not announce those capabilities as shipped.

## Why

Every parameter that decides what counts as motion was chosen against
five-minute files and has never been swept. The archive has since shown three
separate ways they are wrong, and each was patched where it surfaced rather
than at the parameter that caused it:

| Symptom                                            | Patched as                                           |
| -------------------------------------------------- | ---------------------------------------------------- |
| Settling at a file boundary read as motion         | Discount motion starting at a capture's first packet |
| A site split by a 241 s gap, 99 s of it settling   | The same discount, capped to gaps near the bridge    |
| A nudged tripod read as three stops and two drives | Not patched; the junction is simply absent           |

Three patches on the same underlying fault. The bridge constant of 180 s is the
clearest sign: it recovers exactly the expected site count on two days, and 300
merges genuinely separate sites. A constant that has to be that precise is
compensating for something rather than measuring it.

## Parameters in scope

| Parameter                        | Current | Chosen when                         |
| -------------------------------- | ------- | ----------------------------------- |
| `--settling-sec`                 | 75      | Files were five minutes long        |
| `--motion-trigger-sec`           | 5       | Untested against nudges             |
| `--max-motion-gap-sec`           | 45      | Untested                            |
| `--min-segment-sec`              | 10      | Untested                            |
| `MIN_SITE` (index)               | 10 min  | Assumes a twenty-minute protocol    |
| `BRIDGE_SECONDS` (index)         | 180     | Tuned to recover a known site count |
| `DISCOUNT_CEILING` (index)       | 300     | Chosen to stop a discount cascade   |
| `SafetyMarginMetres`             | —       | Background tolerance; never swept   |
| `ClosenessSensitivityMultiplier` | —       | Background tolerance; never swept   |

The last two are the interesting ones. Everything above them is a threshold on
a decision the background model has already made; if the model tolerates a
nudge, most of the thresholds stop mattering.

## Method

**Score against the field map, not against segment counts.** The archive has 24
operator marks with times and named intersections, and 23 recorded sites. A
parameter set is better when it recovers more of them at the right time. Fewer
segments is not better; a single 8-hour "static" segment would score worst on
this and best on segment count.

1. Build the scoring harness first: given an index, report matched sites,
   unmatched marks, and sites with no mark, per day. Nothing can be judged
   without it.
2. Establish the baseline: today's parameters against today's marks.
3. Sweep the background tolerances alone, with every threshold fixed. If the
   model can be made to tolerate a nudge without tolerating a drive, do that
   and re-derive the thresholds afterwards rather than tuning them in parallel.
4. Sweep the thresholds around whatever the model then needs.
5. Retire the compensating constants. `BRIDGE_SECONDS` and `DISCOUNT_CEILING`
   exist to undo the per-file artefact; if classification runs continuously and
   the model tolerates nudges, both should be removable rather than retuned.

## Acceptance

- Every recorded day keeps or improves its matched-mark count
- van Ness at Sacramento classifies as one site
- No site over 30 minutes survives unexamined: `lombard-laguna` is 38.8 min and
  is probably two stays that the current bridge merges
- The 180 s bridge is gone, or has a stated physical meaning

## Risks

| Risk                                                     | Mitigation                                                     |
| -------------------------------------------------------- | -------------------------------------------------------------- |
| Tuning to 24 marks overfits one campaign                 | Hold out a day; tune on two, score on the third                |
| The marks are hand-read and a few minutes out            | Score on matching within tolerance, never on exact times       |
| A sweep across the whole archive is expensive            | Sweep on one day, confirm the winner on all three              |
| Better classification changes published scene boundaries | Scenes are derived; re-export is cheap and the VRLOGs are kept |
