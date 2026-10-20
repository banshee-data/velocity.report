# Centre against extent: how much of the reference centre error is the body's length (W9)

<!-- ignore-style-length -->

D-28 adopted Schäfer et al.'s decomposition as a measurement item: for kirk0's reference
vehicles, how much of each arm's centre error is the error in the body's length, and how much
remains. The physical reference pilot read it by eye. This report measures it for the near-edge
solid body, the geometry candidate and the published box, which the following gap now leans on.
It answers [geom0011](../../../data/experiments/geom/geom0011.md).

- **Status:** Measurement complete, 2026-10-20. One unattended run (exp-20261020); no default
  changed
- **Layers:** L5 (the published box and the solid body), scored against L10 physical references
- **Related:** [physical reference pilot](physical-reference-pilot-kirk0-2026-10.md),
  [published box refinement](published-box-refinement-2026-10.md),
  [duplicate tracks](l5-duplicate-tracks-2026-10.md),
  [W3 retention and fragmentation](w3-retention-fragmentation-kirk0-2026-10.md),
  [following endpoints](following-box-endpoints-2026-10.md),
  [geometry convergence plan](../../plans/lidar-tracker-geometry-convergence-plan.md)
- **Parameter sets:** [solid-body-a2](../../../config/experiments/solid-body-a2.json) (near-edge),
  [geometry-convergence-candidate](../../../config/experiments/geometry-convergence-candidate.json),
  the [defaults](../../../config/tuning.defaults.json) (box)

## Answer

**On the trucks, the centre error is the length's, under every arm.** Of the along-axis centre
error on kirk0's two reference trucks, the length's half explains 99 % under the published box,
81 % under the geometry candidate and all of it under the near-edge solid body, where the length
error predicts more than the centre moved. Each is above the two thirds the spec fixed before the
replays, so by its reading the following gap's error on trucks is the length's too, and the next
lever is upstream (one track per vehicle, the merge spans, the W3 fragmentation fix), not the
centre's placement.

The box shows it most cleanly. Its remainder on the trucks is 3 cm at the mean: the box places
the seen end of each truck where the reference does and draws the body short behind it. Truck 1
is drawn 6.3 to 6.8 m long against its reference 10.6 m (after one early pose at 10.3 m), truck
2 at 3.8 to 4.1 m against 9.6 m, which is the box model's vehicle floor. A truck's centre is then
off by half the missing length: 1.9 m behind for truck 1, 2.9 m ahead for truck 2, whose seen end
is its front. That is why the box's following gaps were within 0.5 m on every reference gap
([following endpoints](following-box-endpoints-2026-10.md)): each scored gap spans a seen end of
each vehicle, and the box holds those.

The car (car 8) has almost no along-axis error under the box (4 cm mean), so its share is not
meaningful. Under the candidate the car's centre is 0.11 m behind with half of it the length's;
under the near-edge arm it is 1.15 m ahead, all of it the length's.

## Question

The spec's reading, written 2026-10-18 before the replays: if two thirds or more of the trucks'
along-axis centre error is the length's, the gap's error is the length's too and the next lever
is upstream; if less, the centre's placement on the seen face is the question and the
containment interval is where to look. Cars apart from trucks, truck 2 apart from truck 1.

## Method

**The decomposition.** A new section of the physical scorer (`centre_extent` in the physical
JSON; "Centre against extent" in the markdown), computed for every pose whose centre and length
are both scored against a reference with a heading. With u the reference heading's unit vector,
the along error is (predicted centre − reference centre)·u and the across error the same
projected on u turned a quarter. With ΔL the predicted length less the reference length and s
+1 when the seen end is the front, −1 when the rear, the length's part is −s·ΔL/2: a body short
by ΔL whose seen end is right has its centre ΔL/2 away from the seen end. The remainder is the
along error less the length's part.

- **Seen end:** the reference bumper nearer the sensor. It reads only the reference, so it is the
  same for every arm; in every decomposed pose here the reference marks that end observed.
- **Share:** Σ|length's part| / Σ|along| over a group's poses. It exceeds 1 when half the length
  error predicts more along error than there is, as when a short body is not anchored on its
  seen end.

**Arms.** Each replayed on the whole of kirk0 (6 s warm-up, 1,024 sample points) and scored on the
frozen tuning split's following episode (samples 472 to 530), each against the near-edge arm.

| Arm       | Parameter set                                            | Read as                                      |
| --------- | -------------------------------------------------------- | -------------------------------------------- |
| near-edge | `config/experiments/solid-body-a2.json`                  | solid bodies (`-b-solid-body`)               |
| candidate | `config/experiments/geometry-convergence-candidate.json` | solid bodies (`-a-solid-body`)               |
| box       | `config/tuning.defaults.json`                            | published boxes (`-a-boxes`, from l8fol0003) |

## Findings

Metres, mean / median over the poses; prediction minus reference.

| Arm       | Vehicle | Poses | Seen front / rear | Along         | Length's part | Remainder     | Mean abs across | Share |
| --------- | ------- | ----: | ----------------- | ------------- | ------------- | ------------- | --------------: | ----: |
| box       | truck 1 |     9 | 0 / 9             | −1.90 / −2.11 | −1.86 / −2.10 | −0.03 / −0.03 |            0.08 |  0.98 |
| box       | truck 2 |     5 | 5 / 0             | +2.89 / +2.89 | +2.86 / +2.85 | +0.03 / +0.04 |            0.08 |  0.99 |
| box       | car 8   |     7 | 4 / 3             | +0.04 / +0.00 | +0.05 / +0.02 | −0.00 / −0.01 |            0.07 |  1.74 |
| box       | trucks  |    14 | 5 / 9             | −0.19 / −1.96 | −0.18 / −1.94 | −0.01 / −0.01 |            0.08 |  0.99 |
| candidate | truck 1 |     7 | 0 / 7             | −0.95 / −0.51 | −0.77 / −0.50 | −0.18 / −0.01 |            0.06 |  0.81 |
| candidate | truck 2 |     0 | –                 | –             | –             | –             |               – |     – |
| candidate | car 8   |     7 | 4 / 3             | −0.11 / −0.19 | +0.09 / +0.07 | −0.20 / −0.15 |            0.06 |  0.52 |
| candidate | trucks  |     7 | 0 / 7             | −0.95 / −0.51 | −0.77 / −0.50 | −0.18 / −0.01 |            0.06 |  0.81 |
| near-edge | truck 1 |     5 | 0 / 5             | −0.24 / −0.72 | −3.21 / −2.50 | +2.98 / +0.03 |            0.50 |  1.60 |
| near-edge | truck 2 |     0 | –                 | –             | –             | –             |               – |     – |
| near-edge | car 8   |     7 | 4 / 3             | +1.15 / +1.46 | +1.08 / +2.03 | +0.08 / −0.37 |            0.51 |  1.01 |
| near-edge | trucks  |     5 | 0 / 5             | −0.24 / −0.72 | −3.21 / −2.50 | +2.98 / +0.03 |            0.50 |  1.60 |

Truck 1 is `obj_35c661db`, truck 2 `obj_8e829a06`, car 8 `obj_8c728991` in the reviewed pack.

- **The box.** Remainders are within 4 cm on all three vehicles; across errors are under 0.1 m.
  Everything the box gets wrong about a vehicle's centre on this episode is its length, and the
  length is wrong only on the trucks.
- **The candidate.** Truck 2 has no scored centre. On truck 1 four fifths of the along error is
  the length's; the candidate's truck 1 is longer than the box's, so its centre is closer.
- **The near-edge arm.** Truck 2 has no scored centre. Its truck 1 share above 1 is two poses
  (samples 500 and 504) where the body is 0.2 m long against 10.6 m and its centre 2.2 m off,
  leaving a 7.4 m remainder each: a degenerate body, not a placement error. The same two samples
  are the near-edge arm's +7.9 m and +6.6 m gap errors in the following endpoint report. Its
  median remainder on truck 1 is 0.03 m.

Beside the decomposition, the same scoring gives the components' mean absolute errors against
the near-edge arm (centre 0.63 m candidate, 1.54 m box, 1.67 m near-edge; length 2.06, 3.02,
4.72 m; rear 0.39, 1.52, 2.78 m). The box's centre error is the largest of the three on the
trucks and the smallest on the car, which is the decomposition's finding again.

## Limitations

- **Three vehicles, one episode, tuning data.** Two trucks and a car over 59 samples of kirk0's
  frozen tuning split. A share is a statement about these vehicles, not a rate; nothing is held
  out.
- **The seen end is the reference's nearer bumper.** A rule on geometry, not on which returns the
  arm actually saw; the reference marks that end observed at every decomposed pose, so the rule
  and the evidence agree here.
- **Truck 2 is scored only under the box.** The two solid-body arms score no centre for it in
  this episode, so their truck figures are truck 1's.
- **Shares near zero along error mean little.** The box's car share (1.74) is the ratio of two
  centimetre-scale numbers.

## Provenance

| Item        | Value                                                                                                                                                                                                                                   |
| ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code        | decomposition at `64be1b591`, stacked on the l8fol0003 branch's box arm (`52fcab2d7`); spec approved at `8f593c8df` before the replays; harness at `d76ab354d`; every binary built with the Makefile's LDFLAGS                          |
| Tuning      | near-edge `solid-body-a2.json` (sha256 `32f9fd29…`, parameter hash `sha256:313d5e15…`); candidate `geometry-convergence-candidate.json` (`b6d34f03…`, `sha256:ec146259…`); box `tuning.defaults.json` (`e6cf6c8a…`, `sha256:15fe4df8…`) |
| Capture     | kirk0 (`internal/lidar/perf/pcap/kirk0.pcapng`), whole, 6 s warm-up                                                                                                                                                                     |
| References  | reviewed pack `ds_67842cb1b22b70e7`, sidecar revision 2232, physical revision 76, split `kirk0-tuning` of `kirk0-reference-tuning-r1.json` (manifest `sha256:5df49517…`), episode `following-472-530`, gate 3 m                         |
| Raw outputs | evidence databases and physical JSON under the run's scratch directory and the LiDAR volume (`exp-20261020/kirk/geom0011`, `results/geom0011`); not in Git                                                                              |

## Recommendations

1. **Treat the trucks' gap error as a length problem.** By the spec's reading the next lever is
   upstream of the body: one track per vehicle ([l5id0004](../../../data/experiments/l5id/l5id0004.md),
   off pending label0001), the merge guard's spans and the W3 fragmentation fix. A truck drawn at
   0.6 or 0.4 of its length is a truck held by more than one track or cluster, which the
   [duplicate tracks report](l5-duplicate-tracks-2026-10.md) and W3 already name.
2. **Do not spend the containment interval on the centre yet.** The remainders are centimetres
   under the box and the candidate's median; placement on the seen face is not where the error is.
3. **Read the box's length floor as a flag, not as evidence.** Truck 2's 3.8 m is the box model's
   vehicle floor. A row that records when the floor applied would let the gap and the report say
   "length unknown, at least 3.8 m" instead of drawing a short truck.
4. **Look at the near-edge arm's degenerate bodies.** Its 0.2 m truck at samples 500 and 504
   produces both its largest centre and its largest gap errors.
