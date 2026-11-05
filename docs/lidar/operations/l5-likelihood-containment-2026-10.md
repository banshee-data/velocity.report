# S3 likelihood cost and containment: cloud comparison

- **Status:** Development screen complete; no adoption, full acceptance open
- **Layers:** L5 association
- **Experiment:** [l5id0011](../../../data/experiments/l5id/l5id0011.md)

## Result

None of the three candidates reaches the predeclared duplicate-rate reduction
on the available kirk0 capture. Likelihood cost alone reduces duplicate pairs
from 10 to 8 and their rate by 18.14 %, short of the required one-third reduction.
Containment claim increases pairs to 21, or 22 with likelihood cost; the rates
rise 74.34 % and 72.86 %. Every arm preserves at least 95 % of vehicle-sized
scored frames on this capture. All four first/repeat baselines match exactly.

These are development measurements, not the pooled corpus or reviewed identity
acceptance result. No candidate is adopted. Both S3 options, the identity and W3
candidates, and the shipped covariance inflation retain their existing defaults.

## Question and reproducible arms

Can likelihood cost charge an uncertain coasting track for its covariance without
losing vehicles, with containment deciding which competing bodies can claim a
cluster? The four registered files under `config/experiments/` share the current
default tracker plus a solid-body shadow, containment and full processed cluster
membership. The solid-body options other than `enabled`, `full_members` and
`containment` are false or zero, matching the original aliases.

| Arm         | Parameter file                       | Likelihood cost | Containment claim |
| ----------- | ------------------------------------ | --------------- | ----------------- |
| `a0`        | association-containment-control.json | false           | false             |
| `lik`       | association-likelihood.json          | true            | false             |
| `claim`     | association-containment-claim.json   | false           | true              |
| `lik-claim` | association-likelihood-claim.json    | true            | true              |

The control is an experimental geometry control, not a declaration that the
shipped tracker runs this shadow. On this capture its tracking baseline is
byte-identical to the earlier default controls in the identity and W3 cloud
screens. This does not establish equivalence on other captures.

The new optional `l5.cv_kf_v1.association` block makes the existing algorithms
reproducible from files. Startup and offline replay share its mapping. Tests load
all four files and compare the entire tracker configuration with the original
replay aliases. Claim requires an enabled contained body with full membership.
The existing likelihood/near-edge incompatibility also applies when settings
come from a file or a mixture of file and aliases. An absent association block
preserves the shipped configuration and fingerprint.

## Predeclared rule and scope

The approved rule requires at least one-third fewer duplicate pairs per hundred
published vehicle tracks pooled over 23 first-segment sites and two kirk captures;
corpus vehicle-sized scored frames must remain at least 95 % of control. Kirk0
reviewed-label matches must remain at least 98 %, HOTA and IDF1 no more than 0.005
lower, switches no higher and fragmentation no higher than control × 1.1 + 1.
Only tuning-pass candidates are scored once on label0001's frozen held-out window,
where the labelled limits must hold again. These levels are unchanged.

The cloud execution registration, commit `e2555d8d`, preceded all new replays.
After #928–#930 merged, the four arms ran from `b04f2c49` on the available whole
kirk0 capture using the committed corpus/index, six seconds of warm-up,
`-require-settled=false` and `-sample-points 1024`. Each primary run repeated and
required identical baselines; a separate box-evaluation replay used the same
file, window and warm-up. The short warm-up is a development window, not a
qualification of background settling. No thresholds changed after results.

Each arm records 771 scored frames after 61 warm-up frames, 832 capture-time
updates in total. The evidence noise screen includes 773 frames with observations
(two are warm-up frames); its denominator is distinct from the scored box rows.
No duplicate or backward timestamps or clamped prediction gaps occur; maximum
capture gap is 0.10409 s. Observation-source and immutable manifest identifiers
were retained separately for every arm.

## Published boxes and coverage on kirk0

A vehicle track has a confirmed published box with a side at least 3 m on at least
five scored frames. A duplicate pair overlaps more than half the smaller box on
at least five frames. Pair-frames count every overlapping pair-frame, including
pairs below that five-frame threshold. Vehicle-sized scored rows include coasted
rows; observed vehicle rows require a measurement. Neither is reviewed recall.

| Measure                       | `a0`   | `lik`   | `claim` | `lik-claim` |
| ----------------------------- | ------ | ------- | ------- | ----------- |
| Published vehicle tracks      | 44     | 43      | 53      | 56          |
| Duplicate pairs               | 10     | 8       | 21      | 22          |
| Pairs per hundred tracks      | 22.727 | 18.605  | 39.623  | 39.286      |
| Rate change against control   | —      | −18.14% | +74.34% | +72.86%     |
| Overlapping pair-frames       | 452    | 246     | 655     | 492         |
| Vehicle-sized scored rows     | 5,996  | 5,717   | 6,236   | 6,047       |
| Scored-row change             | —      | −4.65%  | +4.00%  | +0.85%      |
| Observed vehicle rows         | 4,523  | 4,579   | 4,509   | 4,546       |
| Observed-row change           | —      | +1.24%  | −0.31%  | +0.51%      |
| Claim pairing refusals, total | 0      | 0       | 415     | 316         |
| Track births, whole replay    | 148    | 150     | 158     | 152         |

The local duplicate-rate target would be at most 15.152 pairs per hundred;
none reaches it. The local 95 % scored-row threshold is 5,696.2 rows and every
candidate exceeds it. Those calculations illustrate the rule on this capture;
they do not stand in for its specified pooled populations or labelled clauses.
Likelihood's observed rows increase while its total rows fall, so the total-row
loss cannot be read as an equivalent loss of observed vehicle coverage.

## Label-free noise screen

The unchanged `lidar_noise_screen.py` counts confirmed evidence tracks. Brief
means under one second; lasting means at least two seconds. A near-static track
lasts at least two seconds, moves less than 2 m first to last and has p95 observed
speed below 0.5 m/s. Its lasting-vehicle count uses a p90 side length of at least
3 m and differs from the five-frame published-box population above.

The spurious-cluster proxy is observations not consumed by a lasting track,
divided by frames with observations. Track-observation counts stand in for
consumed clusters; this is a heuristic, not a labelled false-positive rate.
Candidate split links are nearby ordered end/start pairs, not true fragmentation.

| Measure                      | `a0`   | `lik`  | `claim` | `lik-claim` |
| ---------------------------- | ------ | ------ | ------- | ----------- |
| Confirmed evidence tracks    | 79     | 80     | 90      | 97          |
| Brief tracks                 | 12     | 14     | 17      | 16          |
| Lasting tracks               | 58     | 57     | 63      | 70          |
| Near-static lasting tracks   | 6      | 6      | 6       | 7           |
| Lasting vehicle-sized tracks | 36     | 33     | 38      | 41          |
| Candidate split links        | 0      | 1      | 1       | 3           |
| Spurious-cluster proxy/frame | 0.6947 | 0.6831 | 0.7878  | 0.7439      |

Likelihood slightly lowers this proxy but creates more brief tracks. Both claim
arms worsen duplicate and noise proxies. The measures do not support promoting
any arm as a stable-trail or noise improvement.

## A local bidder does not guarantee a global assignment

The original claim documentation overstated its safety. The filter retains a
bidder in each row, but several clusters can need the same remaining track.
A new synthetic counterexample establishes this without changing the algorithm:

| Cluster | Coasting track cost | Fresh track cost |
| ------- | ------------------- | ---------------- |
| C0      | 0.1, box fails      | 1.0, box holds   |
| C1      | forbidden           | 0.2, box holds   |

Before filtering, Hungarian assignment matches both clusters. The claim removes
C0's coasting edge because the fresh track can hold C0. Both rows retain the
fresh bidder, but only one cluster can take it: the maximum matching falls from
two to one. An unmatched cluster can seed a birth. The sole-bidder test and the
all-bidders-fail fallback do not prevent this multi-cluster case.

The test documents a mechanism the implementation permits. It does not attribute
the measured extra births or duplicate pairs to this mechanism; that needs a
per-frame counterfactual analysis. Source/config documentation now states this
limitation. The measured algorithm and all predeclared levels remain unchanged.
A cardinality-preserving claim is a new candidate requiring its own registration
before another comparison, rather than a post-result change to this arm.

## Validation, limits and recommendation

The full Go suite, native Makefile build, Go vet/lint, documentation lint,
experiment registry and config-order checks passed. Focused tests cover actual
file/alias/live parity, required inputs, round-trip fingerprints and mixed-option
refusals. The native executable reported its correct version and commit stamp.
The final cardinality counterexample is tested separately. The measured
association algorithm is unchanged by the subsequent documentation, test and
invalid-input diagnostic fixes.

All four repeats are exact. First-plus-repeat wall times were 85.901, 83.818,
86.185 and 86.759 s; separate box runs were 28.620, 27.963, 28.475 and 29.831 s.
These include instrumentation on a shared Linux host and do not qualify G12 or Pi
runtime. Raw evidence databases were compressed outside Git; decompressed SHA-256
was verified before removing each uncompressed copy. Recordings, source manifests,
commands, noise screens and box summaries remain outside the checkout.

The running cloud environment still enforced revision 1 without `us.aws.cdn.hf.co`;
a Hugging Face payload retry returned a proxy 403. The remaining corpus captures
and full reviewed kirk0 payloads were unavailable. `label0001` remains proposed
and needs manual labelling, full review and a frozen held-out split. No HOTA,
IDF1, labelled switch/fragmentation or held-out result was produced, and no
held-out window was read. Physical geometry, cross-site recall and current W2
acceptance are not established by these counts.

Keep both S3 switches off and leave covariance inflation unchanged. If this line
continues, first register a claim that preserves global assignment cardinality,
then obtain the missing corpus and reviewed truth and apply the unchanged
acceptance rule. Do not infer retention or identity quality from fewer overlaps
alone. The full l5id0011 acceptance remains open.

## Frozen provenance

- Registration commit: `e2555d8de06aee479b17915e023d5decb54c2262`.
- Execution commit: `b04f2c49e3b7f61ed0419ff274707c8ace959e0b`, based on merged
  tracker baseline `d7f51d563722da7074a4c55e654a97195974790a`.
- Tools: Go 1.27.1, Linux amd64, Makefile-stamped version `0.5.1-pre50`, `pcap` tag.
- Corpus tool SHA-256: `efa616037f7fb5b95fe44963ce345e22ed81ac691da58ff996c88aa122a13900`.
- Box tool SHA-256: `2b82b7948b3ef66490fe42b96d402fd9079e7fa6f8e8bee27ac026c9082c7a8d`.
- kirk0: 200,657,872 bytes from Git LFS; SHA-256
  `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
- Corpus/index: committed `cmd/tools/lidar-state-estimation-baseline/testdata/kirk0-{corpus,index}.json`;
  SHA-256 `ae334e0ed561a7e82011d0bc3fcaea816e7cb5cfe35f912241f5c6fb33b116ad`
  and `b1bd6c7d3d83d79a86abf1736b69089cf7ffd9b88dd9e221ca3c736b3a5d696a`.
- Capture bounds: `1764973024741089000`–`1764973107924472000` ns; UDP port 2369;
  `medoid_v0`, identity tracking transform, sensor origin `(0,0)`.
- Public dataset manifest inspected at revision `37ef1befc1876ad860800beea1bbeaa6c9078da9`;
  it does not establish that missing captures or labels were downloaded or scored.

| Arm         | Raw tuning-file SHA-256                                            |
| ----------- | ------------------------------------------------------------------ |
| `a0`        | `667f40645b39554b1672761101dcdd0378016d3cd8fe19d5f9827ffefc666d31` |
| `lik`       | `d20bcb35312a9ffd283d150777dbd23adfee563ebbf611bb4497c0c2262ed482` |
| `claim`     | `7420c9f9883332bed6eb459e2de71a42d7ccbc454b7722d99657f6ee0141fb19` |
| `lik-claim` | `6a89d51af9f28d171ba72069bf21cd865d53d962761313dae191b24411e4c434` |

Replay parameter SHA-256 includes the run settings as well as tuning; it is not
these raw file hashes:

- `a0`: `6bd780e38d5696309ab18da64348a09360ee33c878c2f7ca46206e671b264ea3`.
- `lik`: `c86ddc083097d18a64af6ea67a5af3b9a0eb4e37a6f55d4240ff044163b5491e`.
- `claim`: `b3ef9d5b7f9111d632c5529ea29e7b60e2a62e63ee90698f055ce33ceed14dcb`.
- `lik-claim`: `32b3bed799066a65201a333f1b54dff9a9dd694ee53424c6ad75be0a2e5ec3db`.
