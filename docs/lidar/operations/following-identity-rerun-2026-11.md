# Following baseline: calibration and replay-window provenance

- **Status:** completed development-evidence rerun
- **Scope:** l8fol0008; unchanged baseline on Kirkham at 9th, Columbus at Broadway and Embarcadero at Bryant

## Question and result

Did the calibration-manifest and replay-window fixes propagate consistently through
fresh three-source baseline runs, without rewriting the earlier campaign?

All three cases passed the preregistered provenance, same-DB export-repeat and
accounting checks. Actual per-case calibrations replaced the former manifest placeholder,
and the new window-bound source identities agreed end to end. Rejection probes refused
both stale declarations and a changed-window export binding.

This reruns the exact sources and windows of the [Kirk pilot](following-baseline-reproducibility-2026-11.md)
and [two-site screen](following-development-prefixes-2026-11.md). It is a provenance and
reproducibility check, with no algorithm intervention, parameter adoption, held-out read
or claim of physical calibration, identity, bumper-gap, speed or DRAC accuracy.
Following JSON itself does not embed a calibration ID. Calibration agreement is
established through the companion manifest, sidecar and complete input-DB audit;
the export is source/window-bound but is not a self-contained calibration record.

The [preregistered rule](../../../data/experiments/l8fol/l8fol0008.md) preceded all replays.

## Method and frozen inputs

Current main at launch was `0359442e6d33d4de50df01fcfa0a8277736aecad`, containing
calibration fix #1039 and window fix #1040. Runtime build commit
`a46b25547aa5cba81efe0e79b239fa8c9f204180` adds only the validated preregistration.
The initial spec metadata used an unsupported mixed-corpus name; it was corrected to
explicit bounded direct steps, validated, committed and rebuilt before any replay.
No runtime algorithm or parameter changed during the rerun.

One arm ran from `config/experiments/weekend-following-baseline.json`, byte SHA-256
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675` and semantic
fingerprint `ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
Its effective execution-config bytes independently hash to that fingerprint in
both the first and repeat recordings. The file is unchanged defaults: no changed keys.
The Make-built tools report `v0.5.1-pre52` and the runtime commit above; server and
tool build times were 17:55:27Z and 17:55:46Z on November 14, 2026.

| Case               | Capture basename                    | Bytes       | SHA-256                                                            |
| ------------------ | ----------------------------------- | ----------- | ------------------------------------------------------------------ |
| kirk0              | `kirk0.pcapng`                      | 200,657,872 | `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a` |
| columbus-broadway  | `s2_sf_3_20260902132037_00002.pcap` | 724,578,312 | `135bf02a909df5eeee1f2e526914d975e52cdb562781729174082e7fd6b5e6f0` |
| embarcadero-bryant | `s2_sf_8_20260903144037_00011.pcap` | 724,995,832 | `3c55a2292c59ee964927f38689e29ac46cbfd2790880f317e295db5f47c29c33` |

Kirk retained its original declared path
`/private/tmp/velocity-following-campaign-20261114/internal/lidar/perf/pcap/kirk0.pcapng`;
S2 paths were `/Volumes/lidar/lidar/pcaps/s2/<basename>`.

| Used executable                 | Binary SHA-256                                                     |
| ------------------------------- | ------------------------------------------------------------------ |
| velocity                        | `cd08bf912a747663d0fbf57867c89ab123b707aadfb61b34ad2605a11b4fa331` |
| lidar-state-estimation-baseline | `855a34f22b728fd5348e4500e8c8fd73448aebbc67fd20b383825fb2e12a1553` |

Kirk used the checked-in corpus/index under
`cmd/tools/lidar-state-estimation-baseline/testdata/`; S2 used
`data/experiments/corpora/following-baseline-development-prefixes.json` and
`first-segment-site-index.json`. Each source remained at its original declared capture
path, avoiding relocation as a confound in the identity comparison. New manifests,
recordings, databases, bindings and exports were written under
`/private/tmp/velocity-following-identity-rerun-20261114`. Previous campaign files
were preserved.

The cohort manifest SHA-256 was
`9048156868f42fc2f0d82a57f644f1eb683b04bcb6b21c30709440b47b64079d`.
The real annotation registry resolved to
`/Volumes/lidar/offload/sensor_data/lidar/annotation-packs`; the existing Kirk protection
split SHA-256 was `5df495173ec5a3c3ad4e4637d816c69deed784d67d3f4862880dc331f0b673b3`.
A freshly migrated, separate schema-71 eligibility ledger recorded developer-diagnostic
admission. Validation and qualification captures were not substituted or read.

## Execution and time scopes

One replay lane processed Kirk first, then Columbus and Bryant sequentially. Settled
background was required and 256 L4 sample points were retained. Each first run retained
its evidence DB. The tool's automatic repeat separately checked its tracking summary;
it did not write a second evidence DB. Each completed DB then exported twice, sequentially.

| Process                     | Start UTC                   | Finish UTC                  | Wall seconds |
| --------------------------- | --------------------------- | --------------------------- | ------------ |
| kirk-replay                 | 2026-11-14T18:00:12.421106Z | 2026-11-14T18:00:42.466661Z | 30.045555    |
| kirk0-export-a              | 2026-11-14T18:02:22.165885Z | 2026-11-14T18:02:25.168714Z | 3.002829     |
| kirk0-export-b              | 2026-11-14T18:02:25.211009Z | 2026-11-14T18:02:28.216802Z | 3.005793     |
| s2-replay                   | 2026-11-14T18:02:53.196878Z | 2026-11-14T18:15:18.525301Z | 745.328423   |
| columbus-broadway-export-a  | 2026-11-14T18:50:07.092838Z | 2026-11-14T18:50:52.187568Z | 45.094730    |
| columbus-broadway-export-b  | 2026-11-14T18:52:32.471962Z | 2026-11-14T18:53:17.573021Z | 45.101059    |
| embarcadero-bryant-export-a | 2026-11-14T18:56:42.319599Z | 2026-11-14T18:57:21.405846Z | 39.086247    |
| embarcadero-bryant-export-b | 2026-11-14T18:58:46.525732Z | 2026-11-14T18:59:25.618314Z | 39.092582    |

The complete verification interval was `2026-11-14T18:00:12.421106Z` to `2026-11-14T19:01:17.319348Z`
(3664.898242 seconds), including transfer/staging gaps. This is the spec’s
`actual.wall` scope, excluding build/preflight, final archival, reporting and CI.

These process durations include wrapper, preparation and source-read costs where applicable;
they are operational observations, not a controlled throughput benchmark. The watchdog
monitored internal scratch, including the resident Kirk input, at a 10 GiB floor and
external `/Volumes/lidar` at 20 GiB. Resume thresholds were 14/24 GiB. Disk pauses: 0.
Completed S2 frame chunks and DBs were copied to the fresh NAS archive and
hash-verified before releasing their local copies. Ordinary read-only SQLite
access to the snapshot on this Mac SMB mount failed; immutable snapshot access worked, but random
reads were slow. Analysis therefore staged one byte-identical local DB at a time.
Each export was published locally, copied and hash-verified on NAS, then released
before the next export. Local links preserve the completed recording, DB and export
paths. The Mac SMB mount rejected the exporter's hard-link publication operation,
so no evidence export was published directly there. This is not a capability claim
about the separate NFS export used by arrow-worker. A small APFS-compression probe worked,
but the actual DB copy remained uncompressed; compression was abandoned. Its bytes
matched the NAS snapshot and the copy was used as the local stage. Only a duplicate
generated build binary was replaced by a link to the identical retained binary;
no earlier campaign file was removed.
An inherited prelaunch check initially applied both floors to the internal disk and
refused before starting a process; monitoring was corrected to the two campaign volumes.
Initial read-only SQL grouping attempts on Kirk and Columbus returned open-file errors.
Kirk succeeded on a repeat connection; subsequent completed-snapshot verifier connections used immutable mode and
memory-backed temporary storage, without WAL/shared-memory writes. These were
reader failures before exports, not a reason to replay packets or migrate a
source DB. Transfer/staging and capture rehash costs make these operational
timings unsuitable for comparison as an algorithm performance benchmark.

Retained evidence includes warm-up. Scoring-window tracker summaries and scoring VRLOG
headers have their separately declared bounds; they are not the population denominator
of full-DB following exports. Follower-time sums simultaneous tracked followers, rather
than unique scene time or independent physical vehicles.

Population counts below refer to the first retained DB. The scored first/repeat
frame counts were 300/300 for Kirk, 2,299/2,299 for Columbus and 2,300/2,300 for Bryant.

Actual retained and recorded bounds are Unix nanoseconds. Recorded last timestamps
are last-frame stamps, not an instruction to extend exposure beyond the final sample.

| Case               | Retained first ns   | Retained last ns    | Recorded first ns   | Recorded last ns    |
| ------------------ | ------------------- | ------------------- | ------------------- | ------------------- |
| kirk0              | 1764973030923544330 | 1764973074726654330 | 1764973044826850330 | 1764973074726654330 |
| columbus-broadway  | 1788380443186460330 | 1788380736886903330 | 1788380507086402330 | 1788380736886903330 |
| embarcadero-bryant | 1788471644027858330 | 1788471937815733330 | 1788471707925341330 | 1788471937815733330 |

| Case               | Packet first ns     | Packet last ns      | Requested score start ns | Requested score end ns | Requested beyond EOF ns |
| ------------------ | ------------------- | ------------------- | ------------------------ | ---------------------- | ----------------------- |
| kirk0              | 1764973024741089000 | 1764973107924472000 | 1764973044741089000      | 1764973074741089000    | 0                       |
| columbus-broadway  | 1788380436986265000 | 1788380737384473000 | 1788380506986265000      | 1788380736986265000    | 0                       |
| embarcadero-bryant | 1788471637854124000 | 1788471938414528000 | 1788471707854124000      | 1788471937854124000    | 0                       |

The EOF column is the positive difference between the requested end and available
packet end; it is requested unavailable duration, not missing observed exposure.
Actual recording bounds remain separate. No final-frame interval was invented.

## Identity and calibration verification

Every source manifest has schema 3 and an explicit `replay_window`; none has the former
global calibration placeholder. The manifest records these capture-relative nanosecond
windows in the order scoring start, warm-up, duration:

- Kirk: `(20000000000, 20000000000, 30000000000)`.
- Columbus and Bryant: `(70000000000, 70000000000, 230000000000)`.

Python independently recomputed the NUL-delimited source identity hash from ordered original
capture paths, verified capture hashes, clip, extractor and these window values. Recomputing
without a window reproduced each previous `source/v1/` identity; adding the window produced
these new `source/v2/` identities. Changing each valid boundary separately changed the hash.

| Case               | New source ID                                                                | Original v1 source ID                                                        | Calibration ID                                                                    |
| ------------------ | ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| kirk0              | `source/v2/b435d781e725967fc8423ac8de2c7d420c7db208f501b8871185616fd126afe7` | `source/v1/10ed005c71bc2d0c73b7810b812576866bf9a64f5ef5d141c7f4510b1e351b6a` | `calibration/v1/979644821aa0bf6473d4309ae659e6f3fec41ac8e463e867196f08d5c1f12c2f` |
| columbus-broadway  | `source/v2/e1e00f306ab90f361cd360b2d3cb87829675497964f44f071046a26d7232a158` | `source/v1/170038f79f5d33aa0180958ea7b978404fc6e43f3d3ecc9e8b6542277ea90f7d` | `calibration/v1/9aef58534232a6c8d29a1030b3b153bf0745329e30c22804e0b62b5bc4555f08` |
| embarcadero-bryant | `source/v2/3113317687b919157242256b3c826d2f39464e18a78b897c3fc07e88724fdaf7` | `source/v1/1c6dcdca9426d6d907c93a181d77ed009f31198155ddf0047b4c2833ad610863` | `calibration/v1/2531210ed7059b23ca650ec4330fb9baf0377ef91334efc205b56522a798d8c4` |

For each DB, unfiltered grouping of **all** observations and **all** estimates yielded exactly
one source/calibration pair: the pair declared for that case. There was no calibration filter
that could select away a discrepant row. Every estimate joined its filed observation with
the same source, calibration and frame timestamp. Independently recomputed observation IDs
also matched, and serialised observation JSON agreed with its SQL identity/provenance columns.

| Case               | All L4 rows | All L5 rows | Track IDs | Scored frames | Retained estimate span s | Recomputed observation IDs | Broken joins | Serialised mismatches |
| ------------------ | ----------- | ----------- | --------- | ------------- | ------------------------ | -------------------------- | ------------ | --------------------- |
| kirk0              | 4178        | 3906        | 53        | 300           | 43.803110000             | 4178                       | 0            | 0                     |
| columbus-broadway  | 70388       | 66198       | 668       | 2299          | 293.700443000            | 70388                      | 0            | 0                     |
| embarcadero-bryant | 63001       | 55390       | 907       | 2300          | 293.787875000            | 63001                      | 0            | 0                     |

First-replay sidecars matched source/calibration IDs, build, parameters, warm-up and scoring
bounds. Both replay sidecars declared the same window. Their recording headers matched the
declared build and parameters, and their effective configs independently hashed to the
parameter fingerprint. Header frame counts equalled the recorded-frame counts; recorded
bounds lay inside each sidecar's scoring window. These are legacy scoring track VRLOGs with replay
sidecars: their headers do **not** embed source/calibration IDs or the full window. The
sidecar supplies that binding and must be retained. This run did not emit foreground
observation containers or replace the separate wire-format regression tests in #1040.

The database carries the opaque source ID; the manifest and export binding retain the
explicit window from which it is derived. Losing those records loses a recoverable window
contract. The calibration identity preserves the existing site-index bearing convention;
it does not prove the convention matches surveyed alignment or apply a new point transform.

## Repeatability and accounting

| Case               | Each export bytes | A/B export SHA-256                                                 | DB SHA-256 before/after                                            | Manifest SHA-256                                                   |
| ------------------ | ----------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| kirk0              | 35,292,604        | `51153670d12af61d43dc6039b46b0f635513d3aa60d2be39976228992680af57` | `a660843c24c8520fcff3fc715c018685c2e1d9176ccd648cd0616bd0f044da68` | `9023de7f33307e257efcfe4ed3121d8a823c492b2b60303c253d77b7ebaafed7` |
| columbus-broadway  | 562,303,233       | `4e59a15e041e8f7f69b4e9db200996713c4dcf13b6bc2136dc4f834d7794929f` | `e2c249e2838bf31162c85630e5e46d89051a339ff2d35edcfd5146b473ec3cfa` | `ba96edaaad537e3e8b4612bf3298d2015e6126180cfc2d912dff49fa8b55bc4d` |
| embarcadero-bryant | 462,502,499       | `2d863d442e039ae404528e33bdaeb8626205a5ebbead444793bb92d6362203c5` | `aa28beae4adec5a5208faf6d539aad0e2f3b370ff0c4d60c04e88e4571d89326` | `ba96edaaad537e3e8b4612bf3298d2015e6126180cfc2d912dff49fa8b55bc4d` |

All A/B comparisons use one completed source DB. S2 local stages first matched the
full verified NAS snapshot hash. Each staged DB was then hashed before exports and
again after exports and the all-row audit. No SQL conversion, migration or journal-mode
change was applied to the copies. The DB SHA-256 stayed unchanged before and after
both exports and the all-row read-only audits. This does not establish
byte equality between independently replayed databases. Raw-repeat tracking summaries were
byte-identical within each case. Comparisons to the previous campaign are descriptive:
The five named population/accounting fields (rows, trajectories, pair records and both
interval tables) match for each case, as do the comparable tracking-summary bytes. Export
documents differ across campaigns: source/window/build context changed, and track IDs are
run-local. No per-row identity correspondence or independent-database byte comparison is inferred.

| Outcome            | kirk0         | columbus-broadway | embarcadero-bryant |
| ------------------ | ------------- | ----------------- | ------------------ |
| following_eligible | 15.400613000  | 88.395155000      | 202.381849000      |
| following_excluded | 44.798092000  | 199.809114000     | 100.575113000      |
| no_leader          | 148.013372000 | 1136.217257000    | 1141.725896970     |
| pairing_excluded   | 148.612796000 | 4604.191085000    | 3430.955142060     |
| record_gap         | 96.101658000  | 2221.795764000    | 2500.536122670     |
| **Total**          | 452.926531000 | 8250.408375000    | 7376.174123700     |

| Outcome                 | kirk0         | columbus-broadway | embarcadero-bryant |
| ----------------------- | ------------- | ----------------- | ------------------ |
| ineligible_evidence     | 38.199092000  | 177.403812000     | 84.181220000       |
| invalid_input           | 0.000000000   | 3.999406000       | 0.000000000        |
| missing_input           | 4.799192000   | 16.304202000      | 16.193972000       |
| no_candidate            | 296.626168000 | 5740.408342000    | 4572.681039030     |
| non_closing             | 12.700454000  | 48.894910000      | 138.885825000      |
| record_gap              | 96.101658000  | 2221.795764000    | 2500.536122670     |
| supported               | 2.700159000   | 41.601939000      | 63.496024000       |
| unresolved_positive_gap | 1.799808000   | 0.000000000       | 0.199921000        |
| **Total**               | 452.926531000 | 8250.408375000    | 7376.174123700     |

The two accounting tables report full-retained follower-time in seconds, displayed
to nine decimal places. Both exclusive bucket tables reconcile exactly to each case's integer-nanosecond
row-interval sum. Missing, invalid, unsupported and gap evidence retains its named outcome;
it is not promoted to an observed physical value. Supported experimental DRAC is a nominal
software calculation, not calibrated crash-risk evidence.

The following-export schema contains no calibration field (confirmed in the
Kirk export and in the shared `EvidenceDocument`/`CaptureSource` types). The
source/v2 hash binds capture, extractor and window, **not** calibration: retain
the companion manifest and per-case DB verification. A future portable export
should carry and validate the selected calibration identity explicitly and refuse ambiguous mixed-calibration inputs. This
rerun changed no implementation to add that field.

## Rejection probes

Kirk copies with the old calibration placeholder or a scoring duration changed from
30 to 31 seconds were rejected by existing-manifest verification, with no replay headers,
sidecars or evidence DB created. All three changed-window export bindings were rejected
before output. Original-file digests stayed unchanged. Each case also produced a different
independent source hash when scoring start, a valid warm-up value, or duration changed alone.

Original manifests and completed evidence remained unchanged. Rejection output, commands and
copies are retained with the local and archived verification records. The producer probes
used Kirk; the changed-window following-export probe ran for every case. Independent hash
mutations covered all three window fields for every case. No claim is made that the corpus
CLI independently varies scoring start and warm-up: its `-warmup` sets both.

## Archive and recommendations

Verified **149 files / 6,331,478,983 bytes** by source and NAS
readback SHA-256 under `/Volumes/banshee-results/velocity-campaign/l8fol0008-20261114-1752/evidence`.
The verification-record SHA-256 is
`69994b5b22b09ced56ecb04f5877a240693a42baede4661a287b5176057786c0`.
Headers and verification summaries remain local; completed S2 DBs, frame chunks and
exports remain readable through links to their verified archive copies. Kirk data stays
local. No previous campaign file was removed or overwritten.

The archive includes fresh source manifests, completed DBs, recordings and sidecars, both
exports, source/build preflight, process logs and commands, the final consistent eligibility
snapshot, independent verifier scripts and their results. The report remains readable
without those raw files; row-level evidence stays outside Git.

Accept the provenance repair for this fixed three-source baseline. Future runs should use
fresh schema-3 manifests/output paths and current readers; frozen v1 evidence remains v1.
A distinct window remains a distinct extraction, **not** independent traffic exposure:
overlapping windows must not be summed as independent samples. Preserve the legacy track
VRLOG sidecars or use an observation container where the full extraction contract must
travel in the recording itself. Physical reference review, lane-constrained leader choice,
vehicle-end geometry and uncertainty qualification remain separate work.
