# S2.3: A1 ablation run

Plan: `docs/plans/lidar-near-edge-tracked-state-plan.md`, section "What S2.3 built".

- Host: `hydra` (macOS 15.7.7, arm64)
- Source commit: `d87cc3b9` (branch `claude/upbeat-galileo-4xbaat-s2-a1`)
- Arm: `track_a1`, with the experiment set
  `solid_body,solid_body_face_hysteresis,solid_body_course_faces,solid_body_full_members,near_edge_track,near_edge_track_a1`
- Cases (tuning partition): `marina-webster-beach`, `columbus-broadway`
- Captures: read in place from the NAS, `-pcap-subdir s2`, `-sample-points 256`
- Evidence: per case on the internal disk, discarded after each case

A1 gates the tracked arm's body-centre tracks on the cluster medoid instead of the A2 face
residual. It is the do-nothing option that shows what A2 buys.

Outputs, once the run finishes:

- `track_a1/all/phase0-summary.json`: the phase-0 summary for both cases
- `track_a1/run.txt`: the last 200 lines of the run log
