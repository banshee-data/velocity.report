# Refreshing the published scenes after the web yaw correction

This refresh checks the published scene exports against the corrected web box transform.
It records what the replay, exported data, and rendered playback establish, with physical
body-pose validation kept separate.

- **Status:** Complete; all 24 indexed exports passed numerical checks and sampled rendered review
- **Layers:** Scene publishing, L9 recording/export, public web viewer
- **Related:** [Yaw investigation](published-scene-yaw-investigation.md), [web scene export plan](../../plans/lidar-web-scene-export-plan.md)

The files describe observed or inferred geometry at the capture time. They are measured spatial
data, subject to the [geometry data purpose and publication checklist](../../geometry-priors-data-purpose.md).

## Question and acceptance

Does a fresh export retain complete, internally consistent playback under the corrected box
transform, including the two turn windows that exposed the viewer defect?

Acceptance requires readable complete chunks, coherent frame/index/time coverage, finite
geometry, the intended build and tuning provenance, valid retained placement, passing viewer
regression tests, and rendered inspection. Counts or agreement with velocity do not establish
physical vehicle yaw, identity, or detection accuracy.

## Method

The refresh uses the 24 indexed S2 sites. The legacy `soma1` page is outside that index and is
retained. The settings are the checked-in `config/tuning.defaults.json`, settled replay before
recording, analysis speed, stride 2, and `cv_kf_v1/online` trails. No tuning arm is introduced.
Lane-proposal writes are disabled: this operation publishes scene assets rather than changing
Gerardus's proposals.

The Go binary was built through `make build-radar-local`: version `0.5.1-pre52`, Git SHA
`42c8b39d909e4b077aa355e3cc7a8fae1df77f18`, the reviewed yaw-fix commit. Independent workers
use that same binary and checked-in configuration, with separate HTTP ports, databases, and
asset trees. Completed scene assets are collected serially into the publication branch, and
its catalogue is regenerated after collection.

The API's resolved-configuration fingerprint is `ab0c4035780d`. The exported transition field `tuning_hash` is
`sha256:c927ea3afe40a1be0fb1812083763d05a37269c7bfe6fe1098f91866fdec570d`.
These are different fingerprint contracts; the latter is the recording's effective parameter
hash, not the file's byte digest.
The checked-in file's SHA-256 byte digest is
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`.

The published corpus is the normal source. Franklin–McAllister's corpus manifest describes
699.227 s, while the checked-in site interval is 1,213.026 s. Its original archive is therefore
used for that site, with the same checked-in interval. The corpus and its manifest are not
silently rewritten to make the preflight pass.

The current definitive Gerardus service supplies street geometry. Where several accepted scans
exist for a site, the export explicitly selects the registration already named in that recording's
published placement. List order cannot identify a sensor position. The selected pose must still
pass the placement check against the new settled background.

An interrupted Embarcadero attempt on shared port 8080 is excluded: unrelated test traffic issued
a replay-stop request during its settling pass. Subsequent work uses private API ports. Completed
recordings whose export fails are exported again, without repeating their expensive replay.
Ashbury's first export needed that recovery because its site now has two accepted scans.

Captures are read from the external volume. Temporary database and recording writes use internal
scratch storage after external read/write contention slowed concurrent runs. Later packet inputs
are staged in each server's private safe directory, retaining the original relative paths and
modification times. Source and staged byte digests must match before replay; staging changes
storage location, not packet bytes or tuning. Packet copies are serialised to avoid disk thrashing.
Completed published
recordings are archived externally with every copied file's SHA-256 checked before the scratch
copy is replaced by a stable link. Raw recordings, runtime logs, captures, and row-level comparison
output stay outside Git.

## Validation

The chunk check reads every exported track frame, verifies gzip/JSON readability, chunk counts
and time bounds, monotonic timestamps, unique track IDs within a frame, finite fields,
non-negative extents, header frame count and duration, build stamp, trail estimator, and placement.
The complete product check passed 697 files, including 154,370 track frames and the
301-frame Columbus–Broadway point clip. The full repository `make test` passed on the frozen
producer checkout during the refresh. Viewer tests were rerun against the collected assets.
The viewer tests compute corners in data coordinates before mapping them into Three.js axes.
Their two diagnostic playback windows cover all boxes rather than relying on export-local IDs.

Rendered checks use the repository's deterministic scene-capture harness, with fixed timestamps
and identical overhead and oblique cameras. Capture manifests retain the resolved frame and
source-file hashes. These images are review evidence, not independent physical references.

### Diagnostic comparison

Lombard–Broderick retains 6,009 frames, 149,305 track rows, 6,433 export-local tracks, and 54,803
moving vehicle rows. Its median box-axis discrepancy from velocity course remains 1.12 degrees.
All 6,009 frames have identical track records to the previous published export after removing
export-local IDs and source frame numbering. Its playback timestamps are unchanged. Comparing
records by the same short ID across exports would incorrectly report widespread changes.

The diagnostic Lombard sample remains at +109.807450 s, box centre `(9.56, -47.15)` m, box yaw
`-2.132` rad, and speed 5.66 m/s. A source frame number may change when a replay starts with a
fresh server; elapsed playback time identifies the review sample.

Ashbury–Downey's completed export has 6,137 frames, 28,667 track rows, and 1,560 export-local
tracks. Its selected published registration passes the placement check: score 0.042 at the pose,
against at most 0.021 among the tested displaced poses.

## Complete batch findings

Every indexed scene passed the complete chunk check. All previously placed overlays remain
placed and pass their displaced-pose checks. Point-clip exports, where present, share their
parent recording fingerprint and start time. The legacy `soma1` assets are unchanged.

The table counts exported frames, track rows, and export-local tracks. Parentheses give changes
from the previous publication. The axis/course column gives the median axial discrepancy between a
box and its velocity course for vehicle rows at least 3 m/s; it is a consistency proxy, not
physical heading error. No minimum discrepancy was used to select or accept a scene.

| Site                  | Frames (change) | Track rows (change) | Tracks (change) | Axis/course median, old → new | At pose / best moved |
| --------------------- | --------------: | ------------------: | --------------: | ----------------------------: | -------------------: |
| 1st-mission           |      6,008 (+0) |        324,239 (+0) |      6,644 (+0) |                 1.23° → 1.23° |        0.004 / 0.004 |
| 3rd-folsom            |      6,008 (+0) |        164,379 (+0) |      5,376 (+0) |                 0.99° → 0.99° |        0.013 / 0.009 |
| ashbury-downey        |      6,137 (+0) |       28,667 (+126) |     1,560 (+29) |                 1.06° → 1.05° |        0.042 / 0.021 |
| broadway-franklin     |      5,588 (+0) |        58,518 (+16) |      1,751 (+0) |                 1.12° → 1.12° |        0.037 / 0.016 |
| bush-powell           |      6,009 (+0) |        129,710 (+0) |      4,210 (+0) |                 1.10° → 1.10° |        0.022 / 0.014 |
| california-leon-baker |      6,261 (+0) |        33,774 (+10) |        951 (+0) |                 0.84° → 0.84° |        0.037 / 0.021 |
| columbus-broadway     |     10,165 (+0) |        358,754 (+0) |      8,994 (+0) |                 1.23° → 1.23° |        0.052 / 0.029 |
| columbus-north-point  |      5,519 (+0) |         72,511 (+0) |      2,201 (+0) |                 1.24° → 1.24° |        0.023 / 0.016 |
| embarcadero-bay       |      4,776 (+0) |        148,040 (+0) |      3,109 (+0) |                 1.16° → 1.16° |        0.135 / 0.109 |
| embarcadero-broadway  |      6,048 (+0) |        187,209 (+0) |      5,299 (+0) |                 1.30° → 1.30° |        0.036 / 0.024 |
| embarcadero-bryant    |      6,813 (+0) |        220,403 (+0) |      9,439 (+0) |                 1.28° → 1.28° |        0.083 / 0.037 |
| embarcadero-folsom    |      6,064 (+0) |        150,883 (+0) |      4,669 (+0) |                 1.21° → 1.21° |        0.074 / 0.045 |
| franklin-mcallister   |      6,065 (+0) |        176,212 (+0) |      4,579 (+0) |                 1.21° → 1.21° |        0.040 / 0.030 |
| fulton-divisadero     |      6,049 (+0) |         93,896 (+0) |      2,889 (+0) |                 1.03° → 1.03° |        0.047 / 0.024 |
| haight-divisadero     |      5,775 (+0) |         32,949 (+6) |      1,042 (+0) |                 1.29° → 1.29° |        0.025 / 0.020 |
| howard-6th            |      6,008 (+0) |         96,359 (+8) |      2,971 (+0) |                 1.41° → 1.41° |        0.045 / 0.028 |
| hyde-ofarrell         |      6,005 (+0) |         90,098 (+0) |      2,032 (+0) |                 1.18° → 1.18° |        0.014 / 0.011 |
| laguna-eddy           |      6,554 (+0) |         55,862 (+6) |      1,664 (+0) |                 1.53° → 1.53° |        0.039 / 0.026 |
| lombard-broderick     |      6,009 (+0) |        149,305 (+0) |      6,433 (+0) |                 1.12° → 1.12° |        0.054 / 0.039 |
| lombard-laguna        |     11,638 (+0) |        242,609 (+0) |     11,378 (+0) |                 0.82° → 0.82° |        0.057 / 0.035 |
| marina-broderick      |      6,011 (+0) |         68,039 (+3) |      1,908 (-1) |                 0.86° → 0.86° |        0.018 / 0.016 |
| marina-webster-beach  |      6,010 (+0) |         48,711 (+0) |     1,811 (+13) |                 0.87° → 0.87° |        0.022 / 0.019 |
| pierce-haight         |      6,507 (+0) |         64,897 (+1) |      1,529 (-1) |                 1.71° → 1.71° |        0.012 / 0.010 |
| van-ness-sacramento   |      6,343 (+0) |        180,945 (+0) |      6,156 (+0) |                 0.87° → 0.87° |        0.050 / 0.027 |

The placement columns give the share of tested wall and kerb samples supported by the background
at the retained pose and at the best tested displacement, using the published rounded figures.
First–Mission remains a weak case: its previous score was 0.004 versus 0.003 moved; the refresh
reports 0.004 versus 0.004 after rounding, with the check's passed flag retained. The registration
record is unchanged, and the sampled boxes follow their trails, but these small scores do not
establish physical map alignment. Further registration QA is recommended for this site.

The refreshed batch contains 154,370 track frames and 3,176,969 track rows. Counts and small replay differences are reported rather than treated as accuracy improvements.
Export-local identities may be assigned differently between replays.

## Rendered review and acceptance

Each of the 24 scenes was rendered in overhead and oblique views. The reproducible sample
selects the exported frame nearest the recording midpoint, then the moving vehicle nearest
the sensor (falling back to any track if no such vehicle is present). The camera target is that
box centre; the overhead offset is `(0, -1, 30)` m and the oblique offset is `(20, -20, 18)` m.
Boxes, five seconds of trails, and the placed geometry are visible; point overlays are disabled.
Selection uses time and distance, never box/course agreement. Each capture reached the harness's
stable-render condition; the resulting views were inspected for absent geometry, corrupt boxes,
and the systematic sideways orientation identified in the investigation.

The two original turn windows were also reviewed at +109.807450 s (Lombard–Broderick) and
+36.819015 s (Embarcadero–Broadway). In overhead and oblique views the diagnostic boxes follow
the displayed trails under the corrected transform. Playback in the local production-template
preview was checked as well. Fragmented tracks and unstable individual trails remain visible;
the public research-preview caveat continues to apply. The review accepts these exports for
publication, without claiming physical body-pose accuracy.

## Frozen provenance and retained evidence

The archived batch is identified as `velocity-scene-refresh-20261114`. Raw recordings, logs,
packet staging hashes, numerical-check output, and capture manifests remain local in that archive
or its review scratch directory. Capture manifests name their exact source files, hashes,
resolved timestamp, and cameras. Completed recording copies are checked file by file before
scratch data is replaced by archive links. Failed and interrupted attempts remain excluded.

The following SHA-256 values identify each published recording's `header.json` plus `index.bin`,
the export's source fingerprint contract; they are not claimed as hashes of all recording payloads.
The published asset header also retains acquisition time, units, coordinate frame, evidence status,
build stamp, and the effective-parameter hash. Geometry's source attribution and limitations remain
in the vector and placement metadata and on each page's Data & methods section. Third-party
source terms name USGS public-domain data and DataSF PDDL-1.0 data. The existing local-scan
source still labels its terms as `not yet set`; this refresh retains that unresolved metadata
and makes no new licence assertion.

| Site                  | Published source recording fingerprint                             |
| --------------------- | ------------------------------------------------------------------ |
| 1st-mission           | `5297abd67f3b5849a816856f8d5abeaff9c240c3726c2f23fddf836b9e9f3c16` |
| 3rd-folsom            | `f7c78bab49397ac8b4bedc4e2d38d599fc3240337c19d1a0aea3bea91a81d264` |
| ashbury-downey        | `a2c85917526d2fe041dd4a220504fa306c785c55b5725d69f469e3b73a0e0d09` |
| broadway-franklin     | `8d7a1c481423e5c5e958d3a5e41dcb5493ebe18935c2f0ddb1de7252a9cc8862` |
| bush-powell           | `06ef45263d72b9c279879b39b551821c8ef3cf6b63f4f5f8b1e2330ea8f0a495` |
| california-leon-baker | `dc1b58eb83dd0d4121df5e229caffa547ad57371e9f0d5307d728b62f84635e0` |
| columbus-broadway     | `07875172a384ccc7c28ef46857fc22da45f0013bd6551e3dcfec03d0b9dee07d` |
| columbus-north-point  | `d635f9e1b8b8918631bec7c63db8fe519de038b7a70460af27707f3561aab15f` |
| embarcadero-bay       | `9f2100bcf8de79bb1b3e980505b643bd70e326af5bd7f233f8691ea886685d00` |
| embarcadero-broadway  | `3f5e9cc994b850f6773ae7757c846eb809fac7de7bc79ee8e41997f23e93aab8` |
| embarcadero-bryant    | `532130b038e1c79f6829550ad2aea7d8ca1a89e6eb37b14e56cb272f95b23ba8` |
| embarcadero-folsom    | `bc6411b28a51bfca31af43108a8e178ddd5b52c7a7c1fc82b1685efc8c533ce0` |
| franklin-mcallister   | `3297e2241e46b5b1cda7cb4ff0398db320db147ef06750a789fe07f459cf6f54` |
| fulton-divisadero     | `4665f5fa1d8b4751347d073d5630d0dd99ca5054c991bcf44652ce886d0f5b0d` |
| haight-divisadero     | `1a77bef90caceefa708e36cd50a20a12690f8901544e90c743314914452b87f1` |
| howard-6th            | `80a66ecdad3b3c16837163312c480cd068f9e4879d5a9bbceb9c791747b5dfd8` |
| hyde-ofarrell         | `50dd7faebce7386ebbf0cee8d4f5a0d83caa245e2c1d048b192d8184bc9b8dd8` |
| laguna-eddy           | `47fc51b805ea6c681ec79a6b513653a9986f7a64a0365e2877db536858c7349a` |
| lombard-broderick     | `9ac7344ffb043917cecb5dd4a4550ea6c219b63819ae9c49e98f24942e901c10` |
| lombard-laguna        | `a9e3cb82316e03bae6fe9126ef2b8d8e2aec9536a9356256df12e9ce8af25a33` |
| marina-broderick      | `fc0863e951844b792aa012ce3a18be37c9c50f8bd16d4fcd25b7294761bb369f` |
| marina-webster-beach  | `4e95010dc50a25f88b5affc1364859d7b1f2f51ac2ce6f4298145e79fd0226b4` |
| pierce-haight         | `ed388cb17b0e3b3778c4d64d1cab8fb2090ab3b65699a6dcaa4c613e90779cad` |
| van-ness-sacramento   | `f8397891c020b111f3e3e4884c95aa1fd172b0f177f87a048efe135d9a8d54d5` |

## Limits

A readable export, a successful placement check, and a box aligned with its inferred course do
not validate a physical vehicle's heading or dimensions. The physical-reference work identified
in the yaw investigation remains separate. The yaw correction changes the displayed orientation;
this refresh does not adopt a new tracker or tune a motion model to improve its appearance.

## Recommendations

Publish the refreshed assets with the corrected viewer and retain the physical-reference work
before making body-heading or dimension-accuracy claims. Keep future regression samples tied
to playback windows rather than export-local IDs.

For the next refresh, retain the stamped producer and checked-in tuning file, select the accepted
registration explicitly where a site has several scans, and reuse a completed recording when
only its export fails. Reconcile Franklin–McAllister's corpus manifest and checked site interval
before using that corpus entry in place of its original archive.
