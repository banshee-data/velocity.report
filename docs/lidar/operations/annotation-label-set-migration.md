# Annotation label set migration, October 2026

The five version 1 packs that held labels were carried into capture-bound label sets. The question was
whether labels made on one run's export could be moved onto the capture's returns exactly, keeping
their history, and then apply to another file of the same capture. They could: every labelled
point mapped to exactly one decoded return, every revision was carried, and labels migrated from a
source roll bind to the published PCAPNG.

- **Plan:** [capture-bound annotation format](../../plans/lidar-capture-bound-annotation-format-plan.md)
- **Guide:** [label sets](label-sets.md)

## Method

`velocity lidar labels migrate` (build `0.5.1-pre45`, `5d5af1fb3` for kirk0; `f16362dd1` for
columbus-broadway, whose migration predates the merge and pins nothing) decodes the capture through
the L1 parser and an offline L2 frame builder, finds each pack sample's frame by its start time,
and requires each pack point to be exactly one decoded return of that frame: bit-identical float32
coordinates, in frame order. A point with no return, or one coinciding with another return, refuses
the migration. Each version 1 revision becomes one journal line with its original author, session,
operation and time. Returns two objects claimed in one window are resolved: a reviewed claim keeps
the return, every proposed claim moves it to that mask's uncertain set.

The migration also wrote a version 2 copy of each pack, linked to its label set, carrying physical
references, feature proposals and proposal layers with their pins moved to the new pack. The
version 1 packs were not modified.

## Results

Three packs carry the work that matters (below); two earlier columbus-broadway attempts, on the same
points as the third, follow in [their own table](#the-earlier-columbus-broadway-attempts).

| Measure                                  | kirk0 reference (`ad8b9438`)    | kirk0 sweep ground truth (`8e422582`)                                    | columbus-broadway (`aadfe835`)                |
| ---------------------------------------- | ------------------------------- | ------------------------------------------------------------------------ | --------------------------------------------- |
| Capture                                  | `kirk0.pcapng`                  | `kirk0.pcapng`                                                           | `s2_sf_3_20260902132037_00002.pcap`           |
| Samples (frames decoded)                 | 832 (832)                       | 832 (832)                                                                | 200 (3,004)                                   |
| Pack points, exactly one return each     | 1,833,561 of 1,833,561          | 1,833,561 of 1,833,561                                                   | 1,076,843 of 1,076,843                        |
| Coincident or unmatched                  | 0                               | 0                                                                        | 0                                             |
| Packet table                             | 149,741                         | 149,741                                                                  | 36,001                                        |
| Revisions converted                      | 2,232                           | 402                                                                      | 30                                            |
| Head objects, masks (reviewed)           | 30, 4,102 (4,102)               | 67, 4,661 (442)                                                          | 13, 1,054 (0)                                 |
| Head members                             | 1,756,903                       | 1,732,644                                                                | 377,791                                       |
| Returns two objects claimed, at the head | 0                               | 17,622 (4,548 against a reviewed claim; 13,074 proposed only), 129 masks | 18,550 proposed only, 98 masks                |
| Revisions with such claims               | 0                               | 340                                                                      | 16                                            |
| Version 1 history                        | 44.7 GB                         | 9.9 GB                                                                   | 72 MB                                         |
| Journal                                  | 23.5 MB                         | 18.2 MB                                                                  | 9.0 MB                                        |
| Head files                               | 14.4 MB                         | 15.1 MB                                                                  | 8.3 MB                                        |
| Head projected onto the version 2 pack   | Identical to the version 1 head | Differs only by the claims moved to uncertain                            | Differs only by the claims moved to uncertain |
| Pins moved to the version 2 pack         | 97                              | none carried                                                             | none carried                                  |
| Migration time                           | 88 min                          | 19 min                                                                   | 77 s                                          |

The kirk0 history shrank from 44.7 GB to a 23.5 MB journal: a typical version 1 save changed one
mask by a few returns and stored the whole 26 MB document. The kirk0 migration was CPU-bound
(4,735 s of CPU in 5,297 s): it decodes, converts and validates each of 2,232 whole revisions, then
replays the journal to check every digest.

Every journal replays to the digest each revision recorded (`labels verify`). The kirk0 reference
pack's physical references validate on its version 2 copy with the same content as on version 1
(3 objects, 44 keyframes, 2 following references, revision 76).

### The earlier columbus-broadway attempts

Two earlier label files on the same 1,076,843 points as `aadfe835-…175735` were migrated as their
own label sets. The `055307` pack's manifest says `full` coverage, but all its points are classed
foreground, so its domain is a foreground extraction like the others.

| Measure                                  | `columbus-broadway-0921-0553` (`…055307`) | `columbus-broadway-0921-1917` (`…191749`)     |
| ---------------------------------------- | ----------------------------------------- | --------------------------------------------- |
| Pack points, exactly one return each     | 1,076,843 of 1,076,843                    | 1,076,843 of 1,076,843                        |
| Revisions converted                      | 87                                        | 5                                             |
| Head objects, masks (reviewed)           | 2, 61 (0)                                 | 4, 800 (0)                                    |
| Returns two objects claimed, at the head | 0                                         | 5,876 proposed only, 474 masks                |
| Version 1 history, journal               | 22.4 MB, 6.8 MB                           | 2.1 MB, 7.2 MB                                |
| Head projected onto the version 2 pack   | Identical to the version 1 head           | Differs only by the claims moved to uncertain |

A short history can grow: the journal's first line records the whole domain (every foreground return
of every window), so a set with five revisions has a larger journal than its version 1 snapshots.

Across all five, 6,897,651 pack points each matched exactly one return and 2,756 revisions were
carried.

### The tuning split

`kirk0-ad8b9438-tuning-r1` was re-frozen on the version 2 pack as revision 2, superseding it:
`sha256:74ddd659…5617`. It pins annotation revision 2232 by the label set's state digest, the
physical references at revision 76 with the same scorable counts, and binds its case to
`kirk0.pcapng` by SHA-256. `annotation-split verify` reports every pin holding.

### The sweep's ground truth

`gt_extract.py` was run on the version 1 pack at revision 204, the revision the 11,861-config sweep's
`ground-truth.json` came from, and on the version 2 pack with the label set projected at revision 204.

| Records                | Identical      | Changed | Removed |
| ---------------------- | -------------- | ------- | ------- |
| Reviewed object-frames | 292 of 292     | 0       | 0       |
| All object-frames      | 4,562 of 4,605 | 21      | 22      |

The 43 that differ are proposed masks whose overlapping returns moved to uncertain; 22 became empty.
Scored against three of the sweep's reference cluster dumps (`06e73464b0d2`, `1d4da66a3242`,
`2487de34446d`), the reviewed-mask scores are identical to every reported digit. The all-mask
scores move slightly: road object-frames fall from 2,820 to 2,798 and recall changes by at most
0.0035, with merge rate lower because merged proposals no longer share returns.

The version 1 extraction itself differs from the stored `ground-truth.json` only in the last digit
of some derived floats (for example a range of 34.74197934077826 against 34.74197934077825), which
is the extractor's arithmetic on this machine, not the data.

### One label set, two files

The columbus-broadway label set was migrated from the source roll `s2_sf_3_20260902132037_00002.pcap`.
Bound to the published `20260902T1320_80858-0f4_columbus-broadway.pcapng` (4.9 GB), it found all
36,001 of its packets, all 200 windows complete and no packet twice, and the file's SHA-256
(`sha256:aa69875b…3b66`) equals the dataset manifest's `raw_sha256`. The same labels now apply to
both files. The two earlier columbus-broadway sets bind to the published file the same way.

## Packet key census

A label set binds a file only when each of its packets appears there once, so the packet key must be
unique within a capture. `velocity lidar labels census` read every capture under `LIDAR_PCAP_DIR`
(238 files, 284.2 GB, skipping the analysis, segment, VRLOG and dot directories the capture scanner
skips) and keyed every UDP packet on port 2369.

| Group               | Files | Size     | Packets     | Keys seen twice in one file |
| ------------------- | ----- | -------- | ----------- | --------------------------- |
| `s2`                | 190   | 133.7 GB | 99,808,979  | 0                           |
| `sf-street-speeds`  | 24    | 73.2 GB  | 54,646,936  | 0                           |
| `soma`              | 4     | 23.4 GB  | 17,450,302  | 0                           |
| `kirk`              | 4     | 14.1 GB  | 10,543,439  | 0                           |
| `morg`              | 4     | 11.7 GB  | 8,695,097   | 0                           |
| `broadway-columbus` | 2     | 11.5 GB  | 8,570,299   | 0                           |
| `clar`              | 2     | 8.0 GB   | 5,968,496   | 0                           |
| `2025-10-08-chior`  | 7     | 4.4 GB   | 3,322,169   | 0                           |
| `hesai-2025-09-26`  | 1     | 4.1 GB   | 3,077,197   | 0                           |
| **All**             | 238   | 284.2 GB | 212,082,914 | 0                           |

Every file read, every file held packets on the port, and no key appeared twice within any file:
212,082,914 packets, 212,082,914 distinct keys per file summed. The census does not test keys
across files, where repeats are expected by design: a published PCAPNG holds the same packets as
the rolls it was clipped from, which is what lets one label set bind to both.

## Interpretation and limits

The exact-mapping condition held for all three packs: the decoder's geometry has not changed since
the builds that made them, and a pack's points are a run's L2 points in frame order. That is evidence for these
packs and this decoder, not a guarantee for packs a different decoder made; the migration refuses
rather than guesses when the condition fails.

The labels remain foreground-domain labels: each mask names the foreground of the run it was made
on, and returns that run classed background were never labelled. The kirk0 masks are all `partial`.

Resolving overlapping claims changes proposals, never a reviewed mask. Scores on reviewed masks are
unchanged; scores that count proposals as reference change slightly, as the table shows.

Physical references, feature proposals and proposal layers were carried as their current state;
their own revision histories remain with the version 1 packs.

## Provenance

| Label set                     | Id                                     | Revision | State digest           | From pack (revision)          | Mapping digest         |
| ----------------------------- | -------------------------------------- | -------- | ---------------------- | ----------------------------- | ---------------------- |
| `kirk0-reference`             | `dd6579b7-4705-4a95-a2a3-2a4c12bef749` | 2232     | `sha256:75a268d1…590c` | `sha256:95be6fb2…49d6` (2232) | `sha256:e9d93b33…2c9e` |
| `kirk0-sweep-gt`              | `a7ff24ac-5e7c-4bee-bf30-4be25db72b3d` | 402      | `sha256:3b76c3e4…581b` | `sha256:a2027d43…736d` (402)  | `sha256:8bd75d25…57d9` |
| `columbus-broadway-proposals` | `5bdcf414-d69d-416f-b265-24ee4fb06c56` | 30       | `sha256:22643376…8576` | `sha256:524b1594…4147` (30)   | `sha256:06305626…793e` |
| `columbus-broadway-0921-0553` | `af2d0692-a778-4987-b4e1-46bf5ec40cff` | 87       | `sha256:2b4a5afb…9a29` | `sha256:524b1594…4147` (87)   | `sha256:06305626…793e` |
| `columbus-broadway-0921-1917` | `1f862387-3835-432d-be5e-c7b708ff4b3d` | 5        | `sha256:19d4b118…9c32` | `sha256:524b1594…4147` (5)    | `sha256:06305626…793e` |

| Capture                                            | SHA-256                |
| -------------------------------------------------- | ---------------------- |
| `kirk0.pcapng`                                     | `sha256:2864ebde…8b9a` |
| `s2_sf_3_20260902132037_00002.pcap`                | `sha256:135bf02a…e6f0` |
| `20260902T1320_80858-0f4_columbus-broadway.pcapng` | `sha256:aa69875b…3b66` |

The label sets and version 2 packs are under the annotation directory (`labels/<name>/` and
`<version 1 pack>-v2/`); the migration reports and the census table are local output and are not
committed. The three columbus-broadway sets share a mapping digest: the mapping covers the pack
digest, the capture and the returns, and all three were cut from the same pack points.

## Recommendations

- Archive or prune the version 1 packs' `annotation-revisions/` (55 GB across the two kirk0 packs)
  once the label sets are accepted; the journals hold the same history.
- Label new work through version 2 packs; version 1 packs open read-only.
- Publish label sets with `tools/s2-archive/stage-label-sets.py` once the capture they bind is in the
  dataset; kirk0 is not, and the columbus-broadway set holds proposals only.
