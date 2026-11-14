# Following evidence baseline reproducibility campaign

The Kirk pilot produced a reproducible offline evidence export from one completed database:
export A and B were byte-identical and all readable follower-time reconciled. This is an
engineering result on development evidence, not physical qualification of following or DRAC.
The baseline tool also reported equal tracking summaries across its automatic raw replay repeat;
that narrower result does not establish byte-identical independent recordings or databases.

- **Status:** Kirk pilot complete; operator/physical review and later campaigns remain separate
- **Related:** [Experiment l8fol0006](../../../data/experiments/l8fol/l8fol0006.md),
  [offline evidence exporter](following-evidence-export.md),
  [weekend plan PR](https://github.com/banshee-data/velocity.report/pull/1022)

## Question and frozen method

Can one explicit development source/site/version regenerate exact analytical evidence with
complete interval and exclusion accounting? The single arm used the checked-in
[weekend-following-baseline.json](../../../config/experiments/weekend-following-baseline.json),
a byte-for-byte copy of current defaults with zero changed tuning keys. No candidate tuning,
classifier relaxation or additional motion estimator was introduced.

The authored l8fol0006 rule preceded execution. The pilot selected the existing kirk0 corpus
and index, 20 seconds of warm-up and 30 seconds of scoring, with 256 retained sample points per
observation and one replay lane. The tool wrote the first replay's evidence database and ran
its own repeat. The offline exporter then read the same completed database twice using the
published boxes and online estimate stage, with explicit source, site and policy context.
No PDF renderer or physical-reference score was involved. The retained following-analysis
parameters are the existing uncalibrated scenario defaults, not newly qualified physical limits:
0.5 m/s following speed floor, 1.75 m corridor half-width, 0.35 rad relative-heading limit,
60 m leader range, 2-sigma separability, 12 m unresolved separation, 0.15 s record-gap bound,
and 1 s minimum opportunity. The full path/exposure settings remain in the export, bound by
method hash `2789569b98eff6a4`. Experimental DRAC has no inherited speed floor.

The cohort guard admitted Kirkham–9th as development: replay was recorded as
`development_evaluation` and the two exports as `developer_diagnostic`. The existing annotation
protection root was used. No validation or qualification capture was consumed for this pilot.
The use ledger records whole-capture eligibility, not a newly frozen held-out interval.

## Scope of the three windows

These populations must remain distinct:

| Population                 | Extent and meaning                                                                                             |
| -------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Pipeline processing        | 501 frames, approximately 50 seconds, including warm-up                                                        |
| Baseline scoring/recording | 300 frames per first/repeat run, declared 30-second scoring window after 20-second warm-up                     |
| Exported database evidence | 3,906 readable sample rows across 53 tracks; earliest and latest sample timestamps differ by 43.803110 seconds |

The retained database includes warm-up estimates once they were filed. The exported totals
below therefore do **not** describe only the 30-second scoring window. Its sampled bounds are
1764973030923544330 through 1764973074726654330 Unix nanoseconds. The scoring recording header
starts at 1764973044826850330 and ends at 1764973074726654330; these are sample timestamps,
not an extra inferred duration or an instruction to crop the exporter post hoc.

The export's 452.926531 seconds are **follower-time**: each readable follower contributes its
own intervals, including simultaneous followers. They are neither unique scene-time nor the
number/duration of independent physical encounters. The 20 stored interaction records group
track pairs under existing L8 semantics; they are not 20 independently reviewed encounters.
The two export repetitions are a reproducibility check, never two samples of traffic.

## Observed findings

Both exports contained 35,293,054 bytes and had SHA-256
`12837697918a2fe288c9207cf1a9465fd5ff0093b0193b7a18b3e505bc1ea001`.
Their filename identity was
`73422a9801997181ac7c496559072d59c53935475cf04755e5def6622dc0514f`.
The evidence contained 53 trajectories, 3,906 rows and 20 interaction records. Read quality
reported zero boxless and zero unfiled estimates; no solid-body rows were requested.

### Exact interval reconciliation

Each readable sample owns the half-open interval to its next sample; terminal samples own
zero duration. The buckets are mutually exclusive and sum to 452.926531 follower-seconds.
Record gaps are retained accounting, not observed metric exposure.

| Evidence bucket    |  Rows | Follower-seconds |
| ------------------ | ----: | ---------------: |
| Following eligible |   155 |        15.400613 |
| Following excluded |   451 |        44.798092 |
| No leader          | 1,499 |       148.013372 |
| Pairing excluded   | 1,516 |       148.612796 |
| Record gap         |   285 |        96.101658 |
| Total              | 3,906 |       452.926531 |

DRAC has separate applicability; the following speed floor is not a DRAC floor. These buckets
independently reconcile to the same complete follower-time population:

| Experimental DRAC outcome |  Rows | Follower-seconds |
| ------------------------- | ----: | ---------------: |
| No candidate              | 3,015 |       296.626168 |
| Ineligible evidence       |   385 |        38.199092 |
| Missing input             |    48 |         4.799192 |
| Non-closing               |   128 |        12.700454 |
| Record gap                |   285 |        96.101658 |
| Supported arithmetic      |    27 |         2.700159 |
| Unresolved positive gap   |    18 |         1.799808 |
| Total                     | 3,906 |       452.926531 |

The DRAC wrapper records 383 class-not-supported rows (37.999218 follower-seconds) and two
extent-not-converged rows (0.199874 seconds) among ineligible evidence. No-candidate decisions
include 1,409 no-common-path rows (138.012293 seconds) and 107 ambiguous-leader rows
(10.600503 seconds); the remaining no-candidate rows carry no such exclusion reason. These
are software decisions under the existing selector, not reviewed statements of actual class,
lane occupancy or physical leader identity.

The 27 supported arithmetic values range from approximately 0.0000192 to 0.0361592 m/s².
They use the constant-speed leader/immediate uniform follower braking model. They have no
joint uncertainty bounds, calibrated threshold or independent physical reference. Their small
magnitudes do not establish safe traffic, accurate bumper gaps or absence of conflict. Retained
marginal sigmas were not converted into independent relative-speed uncertainty.

### Repeatability and database integrity

The two complete JSON files were compared directly, not merely by row count. Their analytical
content and full bytes agree. The database hash recorded after export A and checked again after
export B was unchanged:
`907ecd6fa1a34633f9361c0c3a542d5ded5bdec5c80cd4dd402431be7072a0c6`.
This confirms the database was unchanged **between** the two exports; the file named
`db-before.sha256` was actually obtained after A, so it is not evidence of a pre-A byte check.
The writer's read-only snapshot behaviour has separate software tests.

The baseline tool's separate repeat reported `baseline_equal: true`, 300 scoring frames in each
run and matching tracking-baseline JSON hashes
`2c52a2e7ae96616c54cc88df43632def763b96f0eaa946ee84fb29d161bd4359`.
First/repeat frame chunks, headers, manifests and compressed filter records have differing byte
hashes. No independent second evidence database was exported here and no post hoc track-ID
correspondence was invented. The tool's declared tracking-summary comparison passed; stronger
raw-artifact equality is not established by it.

## Source-bound review nominations

These are the earliest exported examples of each named outcome, nominated for W3, not operator
reviewed truth. Inspect approximately one second either side of each absolute sample timestamp
within the source/version above. Capture-relative offsets have not been independently verified.
All IDs below are tracker IDs, not assertions of physical vehicle identity.

| Outcome                 | Sample Unix nanoseconds | Follower → leader                                                                       | Why inspect                                                                                                           |
| ----------------------- | ----------------------: | --------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Supported arithmetic    |     1764973046326707330 | `trk_be90aa98-34e1-4e9d-89f9-40edac7f54b4` → `trk_0191ee8b-fb5d-47b3-80d7-78bff5407262` | DRAC 0.016688 m/s², closing speed 0.684898 m/s; the point retains `estimate_not_final`                                |
| Non-closing             |     1764973046926741330 | Same pair as above                                                                      | Closing speed −0.107476 m/s; no DRAC value. This overlaps the first review period and is not an independent encounter |
| Class-excluded evidence |     1764973034726762330 | `trk_f74fcfc2-c250-4de1-bec9-bdd415a356d1` → `trk_ca003c00-ae2c-4a19-93c4-72d5a7af23e4` | Class, common path, orientation, extent and non-final reasons coexist; no calculation                                 |
| Record gap              |     1764973031223324330 | `trk_a0230d31-33e7-4f18-888f-a130949a88de` → no leader                                  | Interval 0.199795 s exceeds the 0.15 s record-gap bound; no calculation                                               |

The first two rows nominate one period viewed at two instants, alongside two negative-case
periods. The shortlist does not claim a completed multi-vehicle review or three qualified
encounters. Confirm source, seek landing, leader identity, visible endpoint support and uncertainty
in the existing review tools before adding a physical interpretation.

## Frozen provenance

| Item                              | Recorded identity                                                                                                                               |
| --------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Fresh main code base              | `4591a8e49` (merged offline exporter #1034); the later campaign commits add specification/configuration and guard tests, not runtime algorithms |
| Producer and analyser build       | `0.5.1-pre52`, Git `79d36b80d43c92319d1028febe71276c49f4cb29`                                                                                   |
| Build timestamps                  | Baseline tool 2026-11-14T08:28:52Z; analyser 2026-11-14T08:28:54Z                                                                               |
| Baseline executable SHA-256       | `e067e13b85445c5132586236fbc8bdab8427646d488e88ff70a037adeb771ef8`                                                                              |
| Config file SHA-256               | `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`                                                                              |
| Semantic/execution config SHA-256 | `ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`; short fingerprint `ab0c4035780d`                                            |
| Baseline case-parameter identity  | `a420a69447ffd29db3f38fade181daa3ce081b6cd46a33fcd0abf5cf7bd71b02`, distinct from tuning identity                                               |
| Capture                           | kirk0.pcapng, 200,657,872 bytes, SHA-256 `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`                                     |
| Source                            | `source/v1/10ed005c71bc2d0c73b7810b812576866bf9a64f5ef5d141c7f4510b1e351b6a`                                                                    |
| Source manifest SHA-256           | `f9cd7dec43dd6a74d36145f3e173a448cb2a408a8598c4aa3f31119419361c54`                                                                              |
| Corpus / index SHA-256            | `ae334e0ed561a7e82011d0bc3fcaea816e7cb5cfe35f912241f5c6fb33b116ad` / `b1bd6c7d3d83d79a86abf1736b69089cf7ffd9b88dd9e221ca3c736b3a5d696a`         |
| Calibration                       | `calibration/v1/33b2530bc8f586eba3ceedb3b2c66351e471178d85e417c727f6a9a6ba982edd`                                                               |
| Site binding                      | namespace `cohort-site`, site `kirkham-9th`, binding `l8fol0006/cohort-r2/kirk0`                                                                |
| Eligibility policy SHA-256        | `3be8da2d17749c47f64165c93f8841d296519b73dd2ca28dfe211bbae53d5f63`                                                                              |
| Analysis                          | stage `online`, estimator `cv_kf_v1`, observation model `published_box/medoid_v0`, method `following_encounter_v1/2789569b98eff6a4`             |

The source manifest retains ordered capture identities and extractor configuration. The source
identity does not encode a replay window; the source ID alone cannot distinguish two windows
of the same capture. Preserve the scoring options, actual sample bounds, completed DB digest
and export identity above when comparing or regenerating this result. The evidence's absolute
capture paths describe this execution environment; moving them is a separate portability check,
not a result established by byte-repeat at the same paths.

The existing protection root was `/Users/david/code/sensor_data/lidar/annotation-packs` and
eligibility decisions were retained in the campaign's local ledger. Its records have empty
build fields and the replay entry records the configuration path rather than its digest; use
the stamped binaries and frozen manifest above for those identities. The ledger records no
held-out score and supplies no physical label-set revision. This pilot performs no reviewed
geometry, identity or relative-speed score and claims no such reference.

## Cost and evidence retention

The owned replay wrapper ran 2026-11-14T08:30:46.287899Z–08:31:10.342482Z, about 24.05 seconds
including setup and the automatic repeat. Export wrappers ran 08:33:03.726271Z–08:33:06.736017Z
and 08:33:32.092647Z–08:33:35.105592Z, about 3.01 seconds each. These are wrapper wall times,
including polling overhead, not isolated algorithm performance benchmarks. Recorded internal
free space after the runs was at least 29.32 decimal GB (about 27.31 GiB), above the 10 GiB campaign floor.

The local raw pilot was archived under NAS campaign `l8fol0006-20261114-0823/kirk0-pilot` on the
`banshee-results` share. The completed archive-verification record lists 24 files totalling
203,243,860 bytes, with matching source/archive byte hashes, including the database, both
exports, first/repeat replay artefacts and source/binding manifests. Its SHA-256 is
`7b4a026bdd25190300d8a75de90f1439fcad3f3b0dd605e2e5950dc4423ae206`.
A separate verified `pilot-process-evidence` archive retains 11 process/build/guard records
and a consistent eligibility-ledger snapshot (1,082,065 bytes). Its verification-record SHA-256
is `011659e428417b42f3e01dac7da2c2352d2fc6760db54054e59d58785ab672ce`.
The source ledger continues to record the separately scoped S2 campaign. The watchdog uses binary
thresholds: 10 GiB internal and 20 GiB capture floor, resuming above 14 and 24 GiB respectively. No local evidence removal is claimed.
Raw outputs are not imported into this report or committed to Git.

## Interpretation and recommendations

The pilot meets the bounded engineering question for same-DB deterministic regeneration and
exact accounting: complete readable follower-time remains explainable even when most intervals
do not support DRAC arithmetic. The tool's automatic summary repeat also passed its declared
comparison, but raw-artifact equality and physical validity remain separate unresolved claims.

The largest named diagnostic exclusions are path/leader selection and class evidence. Those
counts motivate operator review of source-bound periods; they do not justify relaxing the
classifier, retuning the path or treating inferred geometry as truth. Review several supported,
ambiguous and excluded periods before choosing a tracker intervention. Independent references
are still needed for actual endpoint, speed and identity accuracy.

Any additional site screen belongs to a separately frozen rule with exact development captures,
window options and permitted-use identities before execution. Do not pool this pilot or its two
exports with later windows as independent encounter evidence. Geometry-attribution, a new facet
estimator, 20 Hz and SRAC remain separate decisions; nothing here promotes them.
