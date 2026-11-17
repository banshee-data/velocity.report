# Broderick recording review and attached-drive build

## Question and scope

Can fresh-main VelocityVisualiser open the prepared Broderick recording through one review
action, show its following relationships, move between periods, and retain an uncertain audit?
This was a software walkthrough on November 17, 2026. It did not label physical vehicle identity,
approve a held-out cohort for tuning, establish true endpoints, or qualify DRAC measurements.

## Frozen inputs

The server and native app were built through Make from
`ac5ade912425ee15415ab1dbf7ec367f7836017b`, reporting `0.5.1-pre52`. The app's About window and
the server's version response agreed. This includes the bounded failed-window retry change.
The recording producer remains the earlier `0d9e55fbe7eec91c3468b361046e60248022b396`:
rebuilding the reviewer does not regenerate its evidence.

| Identity              | Value                                                                             |
| --------------------- | --------------------------------------------------------------------------------- |
| Run                   | `following-review-9cfda4b3-358c-4a8d-a096-637e79342fc7`                           |
| Site                  | `lombard-broderick`                                                               |
| Source                | `source/v2/f8e605d8900ebade4097df84aeb454974e541894546ead58c18d16835ad888a5`      |
| Calibration           | `calibration/v1/30226f66f62ba71b24694c07b3793ab761657aa67d43b929c4cb33ad6d099e28` |
| Canonical evidence    | `sha256:867c595c0df057c331f7ec57b6f5c4dbb46ecc071d2eb3a3585647b17e58a70f`         |
| First capture         | `1788367172226773330` ns                                                          |
| Last recorded capture | `1788368303926512330` ns                                                          |

The preparation remains under the attached volume's `offload/sensor_data/lidar/vrlog` tree.
It contains 11,318 frames, 159,265 evidence rows and 19 review slices. A small isolated review
database retained the required run and foreign-key dependencies; the existing server database
was not used for test annotations. HTTP port 18080 and stream port 50061 were loopback only.

## Observed review behaviour

Runs displayed the prepared recording and its Following action. That action loaded the VRLOG,
evidence, geometry and audit without any file chooser. Initial canonical verification took
26.1 seconds on this disk; the initial geometry and slice requests completed in 0.3 and 0.6
seconds respectively. These are individual observations, not latency guarantees.

| Elapsed capture | Follower suffix | Leader suffix | Displayed gap | Displayed time gap | DRAC behaviour           |
| --------------- | --------------- | ------------- | ------------- | ------------------ | ------------------------ |
| 5.800229 s      | `26e2d885`      | `c349b05a`    | 19.4 m        | 1.69 s             | Supported, 0.000503 m/s² |
| 87.099931 s     | `1d2fe0d7`      | `84ee1cb8`    | 22.6 m        | 2.32 s             | Non-closing, unavailable |
| 87.200007 s     | `1d2fe0d7`      | `84ee1cb8`    | 22 m          | 2.22 s             | Supported, 8.97e-05 m/s² |

The first case's exported gap was 19.355976341435948 m and its experimental DRAC was
0.0005033193052732292 m/s². The UI showed rounded values and the provisional estimate status.
Its endpoints were temporally inferred, not independently observed reference endpoints.

The initial exact sample was positioned through the supported replay API after pausing the
isolated stream. Subsequent checks used VV: the follower timeline sought an excluded sample,
the saved-review link returned to the original sample, and the recording slider crossed into
the next minute. Next-sample stepping moved from 87.099931 to 87.200007 seconds and changed
DRAC eligibility without suppressing the available time gap. Missing follower samples showed
an explicit refusal instead of borrowing a neighbouring pair.

An Uncertain review was saved as `a7918b98-33f5-481f-85cd-fcfca9e9fb92`, with reviewer
`software-walkthrough`, four unknown observations, and a note stating that this was a UI
persistence test, not physical validation. The server returned 201 and the UI displayed one
saved record with the same period and notes. Its exact interval was
`1788367178027002330` to `1788367178127036330` ns, exclusive at the end. The audit retained the
canonical evidence digest, source identity, producer and original recording path.

Code review then found that managed drafts omitted frame-source IDs and coordinate-frame
context that the manual-import flow retained. The first audit's empty source list therefore
does not establish that the recording itself lacks those IDs. Both flows now capture context
through one method, accepting only the displayed frame and trail points at the exact capture
time. Historical, empty and stale-frame identities cannot supply a match. This fixes evidence
retention; it does not turn the context into automatic physical or source agreement.

The relationship map exposed a readability defect: labels for unrelated candidates overlapped
the selected pair, and a context pair's distance was more prominent than the selected gap.
The follow-up keeps context markers and hypotheses visible, hides their text labels by default,
and draws the selected follower, leader and gap labels last. Candidate labels remain available
through an explicit control. The follower search also uses a follower-specific prompt.

## Source-agreement follow-up

The missing 3D source agreement was traced through the producer and storage paths. The L5
history adapter did not attach a capture-manifest source ID, and the protobuf recording codec
copied only trail X/Y/acquisition time in both directions. It therefore discarded richer
trail metadata even when supplied. The existing overlay guard required an exact-frame trail
source, so the displayed recording could not establish agreement through those points.

The follow-up preserves all existing trail fields through the codec without filling missing
IDs. Replay instead adds the resolved path and exact header digest of the recording actually
opened by the reader, replacing any playback identity embedded in recorded frames. The review
session supplies the same resolved path after its receipt, source manifest, calibration,
canonical evidence and header checks. VV requires both path and digest to match, refuses
conflicting trail identities, and retains the original actor, exact timestamp and coordinate
guards. A different path is refused even when the header bytes match.

The [managed review contract](following-managed-review.md) defines this additional association
check and the bounded audit context. It operates on the original prepared recording, without
regenerating it, relabelling track histories, adopting tuning changes or promoting physical
accuracy. Matching the reader's recording to its reviewed analysis is distinct from proving
that inferred endpoints represent real bumpers.

## Attached-drive diagnosis

The physical evidence volume was Journaled HFS+ on a USB rotating disk. Both direct HFS+
writes and a growing APFS disk image exhibited intermittent long synchronisation waits.
A direct 64 KiB probe took 8.37 seconds to write and 3.68 seconds to synchronise. An APFS
probe sometimes completed below one millisecond and sometimes took several seconds.
These samples establish intermittent storage latency; they do not identify a unique cause.

A 256 MiB fixed-image control did not establish a preallocation benefit. With quiet serial
probes, the sparse-image median was 0.132 seconds and the fixed-image median 0.215 seconds
(12 samples each). The attempted larger preallocation was cancelled. No physical volume was
reformatted, repaired or forcibly unmounted, and no original evidence was deleted.

Spotlight indexing on the physical evidence volume was disabled and the build used a
`work.noindex` directory. Brief background-process scheduling and suspension tests were
reversed. They did not establish that a particular service caused the stalls. A read-only live
filesystem verification was interrupted without a completed result; filesystem health remains
unverified. The storage driver reported no read/write errors in the inspected counters, which
does not prove the disk is healthy.

Both native builds completed with an APFS image physically stored on `/Volumes/lidar`,
mounted at `/Volumes/velocity-build`, two build jobs and no competing bulk copy. The cold
native build took approximately 50 minutes; the subsequent fresh-main incremental build took
approximately five minutes. The fresh Go server build also completed there. Existing package
source checkouts were read from the checkout; build products, Go cache, temporary linker
outputs, review database and raw walkthrough evidence stayed on attached storage.

The working arrangement is a mitigation, not a disk repair. Keep builds and replay serial on
this disk, retain the original recording tree, and verify the image is actually mounted before
starting another command. Do not create an ordinary directory at a missing image mountpoint.
Machine-specific Make overrides belong in untracked configuration. Follow the
[local storage policy](../../platform/operations/local-evidence-storage.md) before any cleanup.

## Evidence and remaining gates

Raw logs, screenshots, probes and the isolated review database remain in the attached
`velocity-broderick-walkthrough-20261117` workspace and its APFS image. They are not committed.
The reviewed numbers above are summaries of those artefacts, not a substitute for physical
references. The [managed review contract](following-managed-review.md) defines the source,
geometry and durable audit checks. Physical identity, occlusion, lane membership, endpoint
accuracy and projected-speed accuracy remain operator and reference-data work.
