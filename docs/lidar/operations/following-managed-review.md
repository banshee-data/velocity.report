# Recording-bound following review

The recording's Review action opens its paired following evidence and durable operator audit.
The server resolves the completed preparation from the registered run's recording path. A client
does not choose evidence files, submit filesystem paths or replace audit documents. Original
recordings, canonical evidence and minute slices remain on the attached evidence volume.

## Preparation and verification

A completed run needs an absolute `vrlog_path` whose resolved parent contains `complete.json`
from `following-review-prepare`. The receipt must describe that exact recording. Receipt artefact
paths must remain inside the resolved preparation directory, including after symlink resolution.
Unavailable files fail closed; a recording with no preparation does not acquire evidence from a
neighbouring run or an overlapping timestamp range.

The first request streams and hashes the complete canonical evidence, retaining only a bounded
projection of its fitted paths. It compares receipt, source identity, configured replay window,
producer version, header, calibration and slice provenance. The original slice bytes are returned
with their own digest and the canonical digest. Each requested minute is checked independently;
minute boundaries partition sample anchors, without clipping or inventing their intervals.

The cache keeps at most one preparation's compact geometry, bounded at 64 MiB. It holds neither
the canonical row population nor complete trajectories. Receipt/header changes or replacement
of the canonical file invalidate it. Initial verification can take longer than a cached minute
request, particularly for multi-gigabyte evidence. Clients should retain a loading state and
allow the initial verification to complete rather than repeatedly cancelling it.

## HTTP contract

All routes start at `/api/lidar/runs/{runID}/following-review`:

| Method and suffix                  | Response                                                                                                                                           |
| ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`                              | `following_review_session_v1`: exact run, source, calibration, canonical/header digests, recorded bounds, minute descriptors and the audit binding |
| `GET /evidence?at_ns=…`            | Original `following_review_slice_v1` bytes for the minute containing the exact Int64 capture timestamp                                             |
| `GET /geometry?at_ns=…&follower=…` | `following_review_geometry_v1`: source-bound fitted paths for that follower, or all followers in the verified minute if omitted                    |
| `GET /audit`                       | Existing `following_audit_v1` document for this run and canonical evidence version                                                                 |
| `POST /audit`                      | Append `{binding, record}`; return the persisted record and whether it was newly created                                                           |

`X-Evidence-SHA256` identifies exact slice bytes; `X-Canonical-Evidence-SHA256` identifies the
complete canonical export. Digests use `sha256:` followed by 64 lowercase hexadecimal digits.
The audit binding always uses the canonical digest, so moving between minutes does not create
or select another audit. Nanosecond values are exact Int64 values; do not route them through a
floating-point timestamp conversion.

Geometry is a separate response: it does not rewrite a slice under its existing digest. The
projection retains each canonical `LocalPath`, including its original vertices and knots, with
follower, method, window and refusal conditions. Quadratic lists of unrelated actors are omitted.
No path or endpoint is generated from a displayed bounding box. `recording_coordinates` is true
only for the explicitly verified identity sensor-to-sensor calibration. Otherwise clients must
refuse a 3D overlay whose coordinate agreement is unknown. A header label or accepted map pose
alone does not establish a surveyed frame or physical accuracy.

Unknown runs return 404; absent preparation files return 503. Mismatched evidence or stale audit
bindings return 409. Malformed timestamps and invalid observations return 400. Only GET and audit
POST are supported. Responses are not cached by the HTTP client.

## Append-only audit

The camelCase binding and record fields remain compatible with `FollowingAudit.swift`.
Observations are unknown, consistent, disputed or not visible; a consistent conclusion requires
all four observation fields to be consistent. Notes and a reviewer label are required and bounded.
An optional `proposedLeaderTrackID` is an operator observation, not a change to canonical pairing:
it must name a different actor present in the exact starting row's candidate set. Omission or
null means no assertion; an explicit empty candidate is refused. Missing playback source-ID
arrays are normalised to `[]` before persistence so the native audit remains reopenable.

An audit starts at an exact follower sample and stays within verified follower evidence and the
paired recording's bounds. Terminal zero-duration samples remain instants. Playback context does
not create a source match, merge tracks or establish physical truth.

SQLite retains the immutable binding and records per `(runID, canonical evidence digest)`.
Identical record-ID/content retries are idempotent; changed content under an existing identity
returns 409. Distinct records append atomically, including concurrent requests. Earlier evidence
versions retain their histories but are neither inherited by a new analysis nor accepted as the
current request binding. There is no mutable whole-document upload.

The store limits each version to 10,000 records and a conservative 16 MiB document budget,
including a 64 KiB wrapper reserve. A failed append leaves the prior document intact. A newer
observation can be appended; it does not silently erase or supersede an earlier review.

## Native review

In VV, choose **Following** beside a recording in Runs. Evidence, fitted paths and the existing
audit load together. Select a follower to see its chosen leader, competing candidates and
projected front-to-rear gap. Other visible following pairs retain their own gap measurements.
The timeline and recording clock stay linked; unavailable samples do not borrow nearby pairs.
Headway and experimental DRAC retain separate eligibility: a headway speed-floor exclusion does
not hide a supported DRAC calculation.

Correct, Wrong and Uncertain open a period review. An unfinished draft retains its original
recording, evidence version, notes and end-time text across playback or inspector changes.
Submitting captures that context before saving; a failed save can be retried with the same
record identity. Quitting with a draft or pending save asks before discarding it.

## Validation boundary

Fixture tests cover exact bytes, source/version/window mismatch, malformed receipts, symlink
escapes, geometry provenance, optional leader observations, durable retries and concurrent writes.
The opt-in `TestMountedReviewPreparation` reads an existing preparation without replay or writes;
set `VELOCITY_FOLLOWING_REVIEW_SMOKE_VRLOG` to its recording directory. Such a check establishes
artefact compatibility and repeatable retrieval, not vehicle identity, endpoint accuracy,
occlusion quality, accepted lanes or physically qualified following/DRAC measurements.
