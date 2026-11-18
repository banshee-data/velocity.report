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
- The subsequent [source-agreement follow-up](https://github.com/banshee-data/velocity.report/blob/e17854898638ad6288404a1395a7183ebd296d11/docs/lidar/operations/broderick-vv-walkthrough-2026-11.md#source-agreement-follow-up)
  completed the native Broderick walkthrough at implementation commit
  `dcd2bf739b99bc750f10f7d1054cd0f3860e73cc`.
  [#1065](https://github.com/banshee-data/velocity.report/pull/1065) merged on November 17,
  2026 as `e17854898638ad6288404a1395a7183ebd296d11`; this supersedes the earlier
  in-progress status recorded here. The Following action loaded the recording and evidence
  without file selection. A saved-review link selected capture `1788367178027002330` and
  the intended pair; the main 3D viewport displayed the cyan projected gap line. The
  relationship view showed 19.4 m, 1.69 s and “3D gap: recording and analysis match.”
- A bounded stream check matched the frame recording path/header digest to the verified
  session. Geometry matched source, calibration, canonical evidence and recording coordinates.
  Trail source IDs remained empty: recording association did not invent missing source IDs,
  and the header digest is not a checksum of every recording chunk.
- The new **Uncertain** software-test audit, `84522b80-59c8-4f1c-815b-c0912351dc13`,
  retained the exact frame recording path, header digest and capture time through the backend
  API. All four physical observations remained unknown and its conclusion was insufficient
  evidence. Both earlier audits were unchanged. The original recording was not regenerated
  or relabelled; raw checks and screenshots remained on attached storage in the isolated
  walkthrough workspace. This establishes software association and persistence, not real
  bumper or endpoint accuracy.
- The merged report records passing following-review, recorder and endpoint Go suites,
  focused API regressions and 26 targeted native review/evidence/audit tests. These checks
  support the software changes, not an independent physical reference assessment.

These reports do not establish DRAC estimator, threshold, calibration or numerical eligibility
rule tuning, nor do they establish physical accuracy. Absence of documented tuning is not a
proof that all numerical validation claims remain independent. This record is a bounded account
of the cited work, not a complete historical access audit or a replacement for runtime use ledgers.

## Further nominations in #1067

As checked on November 18, 2026, [#1067](https://github.com/banshee-data/velocity.report/pull/1067)
was open at `4ec3777bcab85f574a93821d24c4d76f8565bcd4`. Its
[committed nomination report](https://github.com/banshee-data/velocity.report/blob/4ec3777bcab85f574a93821d24c4d76f8565bcd4/docs/lidar/operations/broderick-vv-walkthrough-2026-11.md#additional-manual-review-nominations)
records selection of four further periods from the same frozen Broderick preparation:
E, closing following (92.699963–94.000411 s); F, non-closing control
(106.099941–109.200350 s); G, near-equal-speed sensitivity (105.100290–106.700260 s);
and H, record-gap negative (95.499806–97.599852 s). Ends are exclusive.

Selection and inspection of the analyser's numerical estimates have occurred; these are not
merely proposed future accesses. They extend the recorded software-development/QA exposure.
The report calls them development review cases, which describes their use, not a change to
the site's validation assignment. The PR also reports replay screening that exposed a
paused-seek delivery defect and motivated a software fix.

The separate review window, navigation and dialog are implemented in that PR, but its description
still lists the end-to-end new-window/four-example walkthrough as in progress. Do not infer
completed manual review from nominations, committed UI code or passing automated CI.
No completed physical labels, new perception runs or adopted threshold tuning are established
by this evidence. The frozen estimates are not physical error measurements. The numerical
independence assessment below remains outstanding.

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
