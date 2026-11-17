# Lombard–Broderick corpus exposure

This record separates cohort assignment from observed use and the claims that use can support.
It records existing evidence, not a new replay, physical assessment or authorisation to tune.

## Assignment and identity

The [cohort manifest](../../../config/cohorts/cohort-manifest.json) revision 4 retains
`lombard-broderick` as **validation**. Revision 4 adds exposure context to the existing rationale;
it changes no site/capture assignments, raw identities, access controls or eligibility rules.
The strict manifest schema is unchanged. The rationale links here rather than introducing
fields that the loader would reject. Its byte digest changes; preserve earlier manifest digests
in existing run records and record the new digest for subsequent runs.

The exposure concerns the prepared run
`following-review-9cfda4b3-358c-4a8d-a096-637e79342fc7`, whose frozen source, calibration,
producer and evidence digest are in the [walkthrough report](broderick-vv-walkthrough-2026-11.md).
The manifest's full capture is `lombard-broderick-full-20260902`, session
`731b905a-4377-41cf-aad2-d2f9dec5aa29`, with raw SHA-256
`1eb08d9e6ced4e20bf07db2140434fbff314b779beb909d602929d44b4c34643`.
Its first-segment capture remains separately listed under the same site. This is not
`lombard-laguna`, `laguna-eddy` or `marina-broderick`.

## Observed exposure

- The [earlier readiness pass](../../ui/visualiser/mesh-and-following-readiness-2026-11.md)
  inspected a numerical sample under annotation review. Its cancelled audit and lack of saved
  records apply only to that earlier pass, not the subsequent walkthrough.
- The November 17, 2026 [walkthrough, merged in #1064](https://github.com/banshee-data/velocity.report/pull/1064),
  inspected displayed gap, time gap and DRAC behaviour at 5.800229, 87.099931 and 87.200007
  seconds. These are software observations, not independently measured physical reference values.
- An **Uncertain** audit, `a7918b98-33f5-481f-85cd-fcfca9e9fb92`, was saved in an isolated
  review database with four unknown observations and an explicit software-persistence-test
  disclaimer. It is not an accepted physical label. The original server database was not used
  for test annotations.
- The walkthrough informed changes to selected-pair label prominence, optional context labels,
  the follower search prompt and retention of managed-review playback provenance. This is
  observed software-development/QA exposure, not merely untouched batch processing.
- As checked against main `866b9292bf919b765ebae0151e3a33b24d009886` on November 17, 2026,
  [#1065](https://github.com/banshee-data/velocity.report/pull/1065) remained open at
  `dcd2bf739b99bc750f10f7d1054cd0f3860e73cc`. Its description reports further use of prepared
  Broderick evidence to address withheld 3D overlays, recording-path/header agreement and
  codec provenance preservation. Native acceptance was still in progress, not a completed
  validation result in this record. The original recording was not regenerated or relabelled.

These reports do not establish DRAC estimator, threshold, calibration or numerical eligibility
rule tuning, nor do they establish physical accuracy. Absence of documented tuning is not a
proof that all numerical validation claims remain independent. This record is a bounded account
of the cited work, not a complete historical access audit or a replacement for runtime use ledgers.

## Claim limits and next decision

1. **UI/workflow independence:** do not call this an untouched end-to-end or UI holdout.
   It cannot independently validate the UI/review/provenance decisions it helped develop.
2. **Numerical validation:** before making an independent following-distance or DRAC claim,
   document the target estimator and code/configuration freeze, information seen, decisions
   influenced, and whether the fixes affect sample selection, matching or reported values.
   Require a reviewed isolation/impact assessment; until then, numerical holdout eligibility
   is unresolved, not affirmed by the `validation` label.
3. **Physical qualification:** remains unestablished. Unknown software-test observations do not
   validate vehicle identity, endpoints, projected speed, gap accuracy or DRAC accuracy.
4. **Reclassification:** no move to development is made here. If explicitly chosen later, move
   the entire site and all its sessions together, revise the assignment and eligibility metadata,
   and stop using it as independent validation of work it informed. A fresh clip or recording
   at the same site does not restore site independence. Do not undo or hide its exposure history.

Five sites remain assigned validation: Fulton–Divisadero, Lombard–Broderick,
Columbus–North Point, Embarcadero–Broadway and 1st–Mission. Moving Lombard–Broderick would
leave four assigned validation sites, not zero. This is a count of assignments, **not** a
certification that the other four are unexposed, sufficiently diverse or physically labelled.
Review their own use histories and claim coverage before naming a replacement holdout.

Embarcadero–Folsom remains separately reserved for qualification. It is not an immediately
available substitute: the manifest still has `qualification_storage_restricted=false` and no
custodians. Qualification access remains refused without the required isolation controls.
See the [cohort isolation plan](../../plans/lidar-qualification-cohort-isolation-plan.md).
