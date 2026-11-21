# Experimental DRAC for three development surveys

This report records the first survey publication of experimental deceleration rate to avoid
a crash (DRAC) for Ashbury–Downey, Columbus–Broadway and Embarcadero–Bryant. It asks how much
of each capture supports a calculation and how the predicted braking demand is distributed
across supported software episodes.

## Method

The batch used the existing full paired preparations and the checked-in
[`weekend-following-baseline.json`](../../../config/experiments/weekend-following-baseline.json).
There was one fixed method, with an independent repeat of the reduction. The 70-second warm-up
prefix was processed during preparation and omitted from its recording. The reduction verified
the recording, source and calibration binding, canonical evidence digest and every review slice.
It applied the checked-in catalogue's stationary windows to the existing evidence, requiring both
complete path supports and the full positive sample interval to lie within one such window for
each supported result. The source captures were left intact.

DRAC divides squared closing speed by twice the positive projected bumper gap. Both speeds are
projected along the established shared forward path. It assumes the leader maintains speed and
the follower brakes immediately and uniformly. The source fits supply point estimates. Independent
checks of physical endpoints, relative speed, identity and pose stability remain open.

Each positive-duration follower interval has one DRAC or exclusion status. The survey denominator
is accounted follower-time within the scoped evidence, excluding record gaps and stationary or
path-support exclusions. It includes non-closing and other unavailable cases within that scope.
Follower-time can accumulate from several tracks at once. Stationary seconds describe the union
of admitted capture windows on the clock. Zero-duration terminal samples supply neither exposure
nor a peak.

A supported episode is a contiguous sequence for the same software follower and leader. A missing
or unsupported interval, a leader change or a stationary-window boundary ends the sequence. The
table counts one peak per episode in four descriptive bands, in m/s². The lower edge of each band
is inclusive. Occlusion and broken track identities can split a physical encounter into several
episodes. Each card names its capture and describes this separate replay analysis.

## Survey figures

| Site               | Capture                                           | Stationary seconds | Accounted follower-seconds | Supported follower-seconds | Below 1 | 1 to below 2 | 2 to below 4 | 4 and above |
| ------------------ | ------------------------------------------------- | -----------------: | -------------------------: | -------------------------: | ------: | -----------: | -----------: | ----------: |
| ashbury-downey     | 20260901T1225_80858-0ac_ashbury-downey.pcapng     |           1157.260 |                   2796.426 |                     58.598 |      37 |            0 |            0 |           0 |
| columbus-broadway  | 20260902T1320_80858-0f4_columbus-broadway.pcapng  |           1648.385 |                  35522.546 |                    132.303 |     121 |            0 |            0 |           0 |
| embarcadero-bryant | 20260903T1440_80858-074_embarcadero-bryant.pcapng |           1050.262 |                  15552.321 |                    107.994 |      83 |            0 |            0 |           0 |

## Evidence accounting

| Site               | Verified rows | Terminal samples | Supported samples | No candidate (follower-s) | Non-closing (follower-s) | Record gap (follower-s) | Outside stationary windows (follower-s) | Path support excluded (follower-s) |
| ------------------ | ------------: | ---------------: | ----------------: | ------------------------: | -----------------------: | ----------------------: | --------------------------------------: | ---------------------------------: |
| Ashbury–Downey     |        31,000 |              350 |               586 |                 2,645.133 |                   64.796 |               1,125.439 |                                       0 |                              9.799 |
| Columbus–Broadway  |       459,139 |            4,130 |             1,323 |                34,470.446 |                  247.895 |              12,685.797 |                               6,811.670 |                            159.690 |
| Embarcadero–Bryant |       213,147 |            3,826 |             1,080 |                14,744.664 |                  310.591 |               9,656.032 |                               3,044.345 |                            166.090 |

All 703,286 rows reconcile to terminal samples or the exclusive status counters. The in-scope
cases also include 901.897 follower-seconds of ineligible evidence, 173.178 with missing inputs,
10.799 with invalid inputs and 3.000 with unresolved positive gap. An independent calculation
recomputed all 4,962 supported DRAC values before stationary filtering from the exported gap and
projected speeds. The largest arithmetic difference was under `3e-16` m/s². The 2,989 admitted
samples match the earlier [scoped scene check](lifecycle-following-three-sites-2026-11.md#scoped-export-and-browser-review).
An independent reduction agreed exactly on each status count and interval sum, on the
terminal counts, and on all episode-peak bands and stationary durations. The built survey
pages showed the matching capture date, exposure and bands on all three cards.

## Interpretation and corpus coverage

The 241 episode peaks fell below 1 m/s². The batch supported 298.894 of 53,871.293 accounted
follower-seconds, about 0.55%. Most accounted time carried no established candidate. Read the
bands as a description of the supported subset. Physical checks of fixed leader choices, reviewed
endpoints, closing speed and pose are needed before comparing safety across sites. The
[manual bumper review](following-fresh-bumper-review-2026-11.md) illustrates why an unknown end
or unresolved queue heading deserves its own review even when an estimator can produce a number
nearby in time.

The archive preflight found 24 catalogued sites, including 18 development sites. The three above
had whole-capture paired inputs for this policy. The other 15 development sites have local source
captures totalling 318.5 minutes and need new paired preparations. Kirkham–9th has a paired clip
outside the stationary catalogue used here. Five validation sites and one qualification site
remain outside the development publication batch. The pilot's retained preparations range from
1.4 to 23 GiB with about 40 GiB free on the attached volume at preflight. Expansion needs capacity
for additional recordings, databases and receipts while preserving existing evidence.

## Frozen provenance

- The original estimator produced these paired recordings at
  `130020360a560b4e965d4451497e7efaba64add3`, using parameter fingerprint
  `sha256:ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08` and method
  `following_encounter_v1/2789569b98eff6a4`. The configuration file's byte SHA-256 is
  `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`.
- Calibration `calibration/v1/30226f66f62ba71b24694c07b3793ab761657aa67d43b929c4cb33ad6d099e28`
  is the sensor-frame coordinate contract. Independent site pose and calibration accuracy are
  separate review work.
- The [catalogue](../../../tools/s2-archive/site-index.json) digest is
  `sha256:219c5649224da771cd9c204654334fac8a89e2392f8d31f2e494239a9a96dceb`. The replay input
  identities and paths are independently described in the
  [full-capture preparation report](lifecycle-following-three-sites-2026-11.md#frozen-sources-and-placement).
- The Ashbury data uses the original archive capture. Its SHA-256 matches the source listed in
  the published dataset manifest for Ashbury's exported file. For Columbus and Embarcadero,
  the prepared source SHA-256 matches the published capture directly.
- The reduction used the Make-built `v0.5.1-pre52` CLI stamped
  `ad68161dd362ca8094071624cea14c4e768a3975`. The completion receipt SHA-256 is
  `9ed7b18de02b31d91a533f56f4fd889e0c17666be09d8302cd0132839c83dce6`.
- The attached LiDAR-backed workspace retains the independent arithmetic check, build stamp,
  completion receipts and preflight under `lifecycle-following/survey-drac-20261120`.

| Site               | Canonical evidence SHA-256                                         | Paired recording header SHA-256                                    |
| ------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Ashbury–Downey     | `0b6fc3e5cc6f3df0371497746474f4236be35938d2ef4d3188110139fa54ace8` | `81c47ae25b5c1f87814873833e0382333b976f13c3ff572f606169c072a8b609` |
| Columbus–Broadway  | `ee56aa96ef7075ac4f18fe3c78e15c83230cf9a5c3f991561e1b7805fb1cdf8c` | `8d31d2cadc816a6e84980036c0d934b6628972a154195847d420cb4f971b7841` |
| Embarcadero–Bryant | `41fed50aa9d147e353f3f0115d632266939af7b97c0ae2ee5210a0909506245e` | `16d3032b96ff9d6f183eebca07f0533b2e1d7d72b2fd8ba046dce219641b0b3e` |
