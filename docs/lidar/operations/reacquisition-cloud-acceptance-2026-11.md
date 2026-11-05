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

Replays and measured results are recorded below when available. No default is
changed based on label-free or tuning-only evidence.

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
