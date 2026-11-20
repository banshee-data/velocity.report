# Manual bumper and heading review across three Kirk0 passages

Three previously unmeasured passages within the Kirk0 development capture supplied manually
placed, bounded endpoints and headings for a fixed comparison of ordinary and lifecycle body fits.

## Evidence and selection

The [visible-end probe](following-visible-end-probe-2026-11.md) restored six development
comparisons but shared fit evidence with the existing physical references. This pass asked
whether the ordinary and lifecycle proposals agree with a new set of hand-placed endpoints
and body headings drawn from previously unmeasured vehicle passages.

Selection used reviewed object membership and the returns in the same capture. A coarse
screen used point counts, centroid motion over one second and proximity to nominate moving
pairs. The shortlist also included a queue with dense returns. Vehicle roles, orientation and
end visibility were inspected in VelocityVisualiser. The vehicles in the earlier three-vehicle
review were excluded from selection. The context spans in the table aided review. Each saved
following decision covered one keyframe instant, with leader identity and bumper coordinates
specific to that instant.

| Pair            | Context samples | Reference sample | Follower / leader returns | Visible evidence                                                      |
| --------------- | --------------- | ---------------: | ------------------------: | --------------------------------------------------------------------- |
| Car 5 → car 7   | 470–473         |              471 |                  829 / 71 | Follower side strip and leader rear band. Follower front unavailable. |
| Car 10 → car 9  | 654–665         |              660 |                 888 / 348 | Queue. Partly sampled follower front and visible leader rear.         |
| Car 12 → car 10 | 730–738         |              734 |                 114 / 182 | Moving pair. Narrow follower nose band and visible leader rear.       |

The manual review used a separate physical-reference view, with the main-window link off and
the prior fits absent. The reviewer placed the face centre in the Top view and checked the
end against the elevation. Headings came from the direction of a visible side or the normal
of an end band, with the forward sign checked from the vehicle's movement. The position
radius includes the uncertainty of selecting the centre of a partially visible, curved face.
Body size, centre offset and hidden ends remained unknown. Sample 471 retained the follower's
heading without placing a bumper from the side strip alone. The reviewer saved and separately
reviewed the records in VV before either fit was run.

| Sample | Role            | Visible face centre (x, y) and radius in metres | Heading and bound |
| -----: | --------------- | ----------------------------------------------- | ----------------: |
|    471 | Car 5 follower  | Unknown                                         |        −125° ± 8° |
|    471 | Car 7 leader    | Rear (−17.946, −9.984), ±0.70 m                 |       −126° ± 12° |
|    660 | Car 10 follower | Front (−4.571, 2.436), ±0.80 m                  |         57° ± 10° |
|    660 | Car 9 leader    | Rear (−0.862, 10.086), ±0.45 m                  |          54° ± 8° |
|    734 | Car 12 follower | Front (−8.392, −4.835), ±0.85 m                 |         53° ± 12° |
|    734 | Car 10 leader   | Rear (−0.297, 10.456), ±0.45 m                  |          59° ± 8° |

Coordinates are in the pack's ENU sensor frame. Decimal places preserve the recorded
placement. The declared radii express its precision. Headings are measured from +X towards
+Y and describe body direction, including the stationary queue car.

Both fit arms used the same pack, frozen masks and default fit options. The baseline is
`annotation-reference fit --sample`, the ordinary offline fit evaluated at the requested
keyframes. The lifecycle arm adds `--lifecycle`, excludes detached or unresolved support
from its body fit and abstains at unsupported poses. Both offline arms have access to the
reviewed passage for body estimation and use the same prior replay. The reference and
candidate gaps use their respective follower axes and the same worst-case propagation for
endpoint and heading bounds. An additional calculation projects the fitted endpoints onto
the manual follower axis to show how much of the gap difference comes from the heading.

## Lifecycle import failure

The initial lifecycle fit for car 12 failed import validation. Its persistent shell
claimed five observed rears at samples 773–777, where the returns along the fitted axis
stopped 0.52–0.57 m short even at the shell's lower length bound. These later frames are
outside the following comparison, but the importer correctly refused the whole object.
The lifecycle proposal now checks each observed endpoint against the same return support
as the importer. A failed claim becomes inferred if a full length is available, or unknown
otherwise. The importer still checks all observed claims.

The corrected build successfully imported all five bodies and 254 poses. Checking every
pose against the first fit found five observed-to-inferred changes, all on those rears.
The check also verified equality of the body dimensions, pose positions, anchors and headings.

## Gap and heading comparison

| Sample and pair      |                       Manual gap, m |                           Ordinary fit, m |                 Lifecycle fit, m |
| -------------------- | ----------------------------------: | ----------------------------------------: | -------------------------------: |
| 471, car 5 → car 7   | Unavailable, follower front unknown |                       10.36 [8.06, 12.66] |              10.36 [8.12, 12.59] |
| 660, car 10 → car 9  |                  8.44 [5.49, 11.39] | Unavailable, follower direction ambiguous | Unavailable, lifecycle abstained |
| 734, car 12 → car 10 |                17.08 [11.89, 22.27] |                      16.95 [14.97, 18.94] |             16.96 [14.91, 19.01] |

Brackets show propagated worst-case bounds. Each fit produced a comparison on one of the
two instants with a manual gap. On the moving pair at sample 734, the ordinary fit was
0.129 m short of the reference and the lifecycle fit was 0.123 m short. The 0.006 m
difference between the fits is small relative to the reference bounds. The lifecycle
interval was 0.13 m wider than the ordinary interval. A common-axis calculation projected
the fitted bumpers onto the manual follower heading. It gave endpoint contributions of
−0.204 m and −0.198 m for ordinary and lifecycle fits, respectively. Each fitted follower
heading added about 0.075 m to its projected gap.

| Sample and role      | Ordinary endpoint difference, m | Lifecycle endpoint difference, m | Heading difference in both fits |
| -------------------- | ------------------------------: | -------------------------------: | ------------------------------: |
| 471, car 5 follower  |                     Unavailable |                      Unavailable |                           −5.5° |
| 471, car 7 leader    |                            0.34 |                             0.34 |                           −5.0° |
| 660, car 10 follower |                     Unavailable |                      Unavailable |                      Unresolved |
| 660, car 9 leader    |                     Unavailable |                      Unavailable |                      Unresolved |
| 734, car 12 follower |                            0.61 |                             0.61 |                           +2.3° |
| 734, car 10 leader   |                            0.18 |                             0.17 |                           +8.2° |

Endpoint differences are planar distances from the manual placement. Heading differences
are the signed fitted-minus-manual angles on the shortest arc. A small longitudinal gap
difference can coexist with a larger individual bumper displacement.

At sample 471, both fits assigned the follower a front using the side strip's extent. The
review could only place its heading, so the fitted gap has no manual gap to compare against.
At sample 660 both fit headings were front/rear ambiguous. The lifecycle diagnostics
recorded an abstention for both vehicles. Neither missing queue pose met the visible-end
probe's resolved-heading rule, so that rule restored zero gaps.

## Interpretation

The results measure agreement against six hand-placed keyframes from one reviewer working
with the same LiDAR returns as the fits. Car 10 contributes at two instants. The queue
compares geometry only. The strongest next test is to review how a confirmed heading
carries through a stop and measure coverage on held-out traversals with independently
measured bumper centres and tighter heading references. A change to DRAC geometry needs
that evidence and separate speed validation.

## Frozen provenance

- Pack `sha256:67842cb1b22b70e7dff46aa53af48405e44ddd69503d3c43a0c7670450a1507e`, membership revision 2233 with digest `sha256:12b47778fdf9119e83d18e9e8e0ebeadaea1b52b5ba32b35441c83d8e8edfad8`.
- VV label revision 12, SHA-256 `89550f7ecdfb1bb65687e1fe7e9356dbf35acec60b3f8cc08fd49dd0f2f0531b`. The server recorded six reviewed, independent keyframes across five vehicles.
- Reference revision 13 with the three following decisions, SHA-256 `d2115acb7e05941031c1d8209353213f6135c877110b03490a7fd2830b1deba0`.
- Selection and visibility decisions, SHA-256 `36eb531ff0d2fa0f603c87812a53c00ecdeb63f3f58f099773f3b64a25465158`.
- Make-built CLI `v0.5.1-pre52`, source `3c98dc8d807afea4f2f6e79ff9090fe927587bd4`, produced the original diagnostics and ordinary comparison. The corrected stamped CLI at `df25db84130b81b280b86b90015af0c27f6a5114` ran the lifecycle arm. An earlier diagnostic import in a separate view revised the same five rejected claims and produced identical comparison rows.
- Byte-identical repeats produced SHA-256 `cb03b1464c090bdef0bc048db9325fd104f1705b13a809e2e12eb3a89e9e43cf` for the ordinary fit and `ccf2950059b328ac50f135c10e37f190fc3a1fff5187defe88660cf5af35a1a3` for the corrected lifecycle fit. Both runs retained all three sample decisions and the same frozen reference digest.
- The attached LiDAR-backed evidence volume retains the VV screenshots, mask screening, frozen labels, selected cases, fit diagnostics, comparisons and repeat receipts under `lifecycle-following/fresh-bumper-review-20261120`.
