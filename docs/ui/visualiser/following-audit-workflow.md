# Following and experimental DRAC audit workflow

- **Status:** Implemented operator review records; physical qualification remains pending
- **Related:** [Evidence exporter](../../lidar/operations/following-evidence-export.md),
  [weekend plan](../../plans/lidar-weekend-following-drac-plan.md),
  [review readiness](following-review-2026-11.md)

## Review an explicit source

Open **Inspector** (or **Paint streets → Review tracks / following…**). This leaves
painting and closes the other task columns so viewport clicks select tracks; unsaved street
edits remain in the painter. Select a track in the list to expose classification and quality
flags. If the app joined an already running VRLOG, choose **Use current replay for labels**:
this locates its run without seeking or restarting playback. An unmatched or changing replay
remains unbound; use Runs to choose its recording explicitly.

Expand **Following / headway / DRAC evidence**. Open a
`following_evidence_v1` export produced by `velocity report headway`. Retain the exact
export on the attached LiDAR volume: an audit names its SHA-256, explicit site/source/binding,
analysis stage, estimator, observation model, method, parameter hash, analyser build and
eligibility policy. Moving unchanged bytes is safe; regenerating different bytes requires a
separate audit. Selecting a city site does not bind an export to the recording.

Use **Previous sample**, **Next sample**, or an exact Unix-nanosecond timestamp and follower
track ID. **Use frame time** copies the current frame time and selected track; it does not
interpolate or seek. The viewer retains the export's candidate decisions, projected speeds,
endpoint support, gaps, suppressions and experimental DRAC. It does not recompute them. The main replay database may hold runs and labels without the
source-bound state estimates needed for an export. A road mesh, accepted scan or saved lane set
does not supply those estimates. Missing exports require the separate replay/analysis workflow;
opening Inspector alone cannot produce following measurements.

Check recording coverage and capture/source, calibration and analysis-version provenance before
reviewing visible traffic. The audit captures current run, source path, recording path, capture
time, recording bounds, coordinate-frame ID and available frame source IDs as viewing context.
It deliberately does not label that context verified: a matching time or source ID alone cannot
establish all of those agreements. Missing recording or independent reference evidence remains
missing. Describe the evidence actually checked in the notes.

## Record a period

Choose **Audit this period…** on an exact follower sample. Supply a reviewer label, period end,
observations and notes. The default period is that row's exported interval. Periods are half-open
`[start, end)` in capture Unix nanoseconds; a terminal zero-duration sample is an instant.
The start must be an exact sample for this follower, and the end cannot exceed that follower's
exported coverage. The reviewer may extend the period to cover several samples, including
leader changes or record gaps. Those gaps stay gaps: the audit supplies no interpolation,
actor stitching or observed exposure.

Record each of these independently:

- Physical identity continuity, including fragmentation and possible lost returns.
- Leader/common-path evidence, including adjacent, crossing, turning or merging traffic.
- Visible front/rear endpoints and uncertain or unobserved geometry.
- Evidence for both projected speeds, including support changes and disagreement.

Each field starts **Unknown** and can remain unknown. **Consistent at reviewed scope** requires
all four observations to be consistent; otherwise retain insufficient evidence or disagreement.
This is an operator statement about reviewed evidence, not a new eligibility decision, physical
reference, speed validation or safety classification. Include negative cases as well as apparently
successful following. Do not choose a threshold from this small diagnostic review.

**Add review record** appends an observation without changing earlier records or the evidence.
The review-record list reopens the exact evidence sample; it does not seek the recording.
Add a new record describing a correction rather than silently overwriting an earlier observation.

## Save, reopen and report

**Save audit…** writes `following_audit_v1` JSON atomically to a separate file. Keep it and the
original export outside Git, on `/Volumes/lidar`; the NAS is an archival destination, not a
review dependency. Audit timestamps use UTC ISO 8601 and capture timestamps retain exact Int64
nanoseconds. **Open audit…** verifies the evidence digest and full binding, schema, unique record
IDs, period bounds and observation requirements. A mismatched file is refused. The Inspector
retains its session when hidden; save before quitting the app, which asks before discarding unsaved review work. Replacing evidence or an audit
asks before discarding unsaved review records.

For this cycle, review three or four permitted development periods, including a negative case,
from the existing baseline nominations after checking recording coverage. Summarise identities,
ambiguity, support, suspected lost returns and disagreement in an authored report in `docs/`.
Retain raw audit/export files locally with their fingerprints. Three recorded notes do not
constitute three independent physical encounters or complete the frozen-reference gates.

## Crash regression boundary

The supplied crash report trapped on foreground coordinate indexing. The publisher previously
held no reference while offering pooled point arrays to successive clients: an early consumer
or rejected queue could release them before the next client retained the frame. The broadcaster
now owns the arrays for the whole fan-out, and the last consumer releases them after sending.
A queued-client regression reproduces empty coordinate arrays without the fix.

Both native renderer paths also reject inconsistent required coordinate/intensity arrays before
allocating or indexing. Malformed point updates clear previous point counts; malformed background
updates clear their cache/transition. Later well-formed frames render normally. Optional foreground
classification retains its existing fallback. The decoder clamps out-of-range wire byte values
instead of trapping on conversion. These tests establish software robustness; the crash report
alone does not identify which server/client sequence produced that particular malformed frame.
