# Reacquisition acceptance on current defaults

- **Status:** Implementation and cloud evaluation; no default promotion
- **Spec:** [l5id0013](../../../data/experiments/l5id/l5id0013.md)
- **Scope:** Default-off identity candidates and the labelled per-frame gate component

## Implementation

The optional `l5.cv_kf_v1.identity` block carries tentative priority and overlap
end options in a checked-in tuning file. Absent, it changes neither defaults nor
their fingerprint. The offline replay uses the same tracker mapping as startup;
replay aliases remain accepted, and conflicting overlap frame counts refuse.
The two registered parameter sets express body-older and body-older-tentative.
No new survivor-belief inheritance is claimed: that part of l5id0006 remains open.

`lidar-ground-truth-eval perframe -identity-gate` applies l5id0004's labelled
limits to the scorer's paired population. It writes each clause into JSON and
Markdown before returning 3 for failure or insufficient evidence. A tuning,
unreviewed, unfrozen or empty reference cannot pass. A pass covers only this
component; corpus duplicates/coverage, W5 geometry, cost and independent
physical-reference gates still apply.

## Evidence and remaining acceptance

The reference pcap was retrieved and digest-checked through Git LFS. The public
Hugging Face manifest and small annotation metadata were readable, but binary
label/point payloads redirected to a CDN outside this running instance's network
policy. The owner saved an environment update; retries still returned proxy 403
and the runtime continued reporting spec revision 1 without the CDN host.
Missing labels or frozen splits are insufficient evidence, not a zero-error score.
The independently reviewed label0001 held-out window is a separate prerequisite;
public kirk0 tuning masks must not be relabelled as independent held-out truth.

No default is changed based on label-free or tuning-only evidence.

## Current-default kirk0 screen

All three arms replayed whole kirk0 from six seconds of warm-up, with
`-require-settled=false` to reproduce the established development window. The
corpus tool verified identical baseline repeats for every arm. Each recording
held 771 scored frames; the diagnostic frame clock counted 773 updates, maximum
interval 0.10409 s and no backward, duplicate or clamped intervals. These
numbers come from base main plus this PR, before the separate empty-frame PR.

| Measure                                           |     a0 | body-older | body-older-tentative |
| ------------------------------------------------- | -----: | ---------: | -------------------: |
| Published vehicle tracks, at least five frames    |     44 |         39 |                   37 |
| Duplicate pairs, at least five overlapping frames |     10 |          3 |                    4 |
| Duplicate pairs per 100 published vehicle tracks  | 22.727 |      7.692 |               10.811 |
| Overlapping pair-frames                           |    452 |        107 |                   98 |
| Observed vehicle frames                           |  4,523 |      4,491 |                4,563 |
| Brief confirmed tracks, under 1 s                 |     12 |         29 |                   18 |
| Near-static lasting tracks                        |      6 |          7 |                    6 |
| Approximate spurious clusters per observed frame  | 0.6947 |     0.9250 |               0.8176 |

The duplicate figures use the existing `lidar-box-eval -variants none` definition,
including coasting boxes; observed vehicle frames count only matched rows.
The noise screen uses `scripts/lidar_noise_screen.py` on observed evidence,
with its distinct lasting-track definition. Do not compare its vehicle counts
with the published five-frame denominator. Raw overlap is a proxy, not proof
that a pair is the same physical road user.

Against a0, body-older lowered duplicate pairs per hundred by 66.2 % while
observed vehicle frames fell 0.71 %. Adding tentative priority lowered that
duplicate measure by 52.4 % against a0 and raised observed vehicle frames 0.88 %.
It produced fewer brief tracks than body-older (18 versus 29), but more than a0
(12). Its duplicate rate was 3.119 per hundred above body-older on kirk0: the
old pooled-corpus non-inferiority finding cannot simply be carried forward to
this build from one capture. This screen does not decide l5id0006's pooled rule.

Labelled HOTA, IDF1, switches and fragmentation were **not scored**: the complete
reviewed pack and frozen split were unavailable. The labelled acceptance
component therefore has **insufficient evidence**. Neither physical identity
accuracy nor safe promotion follows from fewer overlapping boxes.

## Commands and frozen execution

The Makefile stamped both replay and duplicate-count tools from
`56fb96248ea19578cdebc1998207582f78574951`, Go 1.27.1 on Linux amd64.
The primary run used `lidar-state-estimation-baseline` with the committed
`kirk0-corpus.json` and `kirk0-index.json` testdata, one arm's tuning path,
`-warmup 6 -require-settled=false -sample-points 1024`, an immutable source
manifest and a separate evidence database. Its first/repeat baseline bytes
agreed for every arm. Executable SHA-256:
`b4c80755c231ccd7d61f14798cc4fce7d5c47ebe4aa1557795aec82c7e165253`.

A single `lidar-box-eval -pcap kirk0.pcapng -warmup 6 -variants none` replay per
arm supplied the duplicate counts. Whole primary first/repeat runs took 90.1,
84.4 and 82.9 s; duplicate-count runs took 26.8, 42.9 and 51.9 s. They ran
alongside other cloud work and are provenance, not quiet-host performance
comparisons or Pi timing gates.

## Provenance and limits

Base: `1ec1ccea5947ec8f72d155b337ed86d066f342c4`.
Pre-run rule committed before replays. Dataset revision:
`37ef1befc1876ad860800beea1bbeaa6c9078da9`.
Kirk0 SHA-256: `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
Raw outputs stay outside the checkout. Linux replay cannot establish Pi cost,
live capture behaviour or native visualiser correctness.

## Arm file hashes before replay

- `config/tuning.defaults.json`: `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`.
- `config/experiments/identity-body-older.json`: `d4c0e4fc4b83bb99260512125ba0c507d28881e514b4a644f9c0a29348d9339f`.
- `config/experiments/identity-body-older-tentative.json`: `3571e0187d867f5b2e9f7735a060ec4b568b396d3dbdb40587ab70a53fbc15a6`.
