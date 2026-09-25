# Columbus Broadway at 0.25x: PR #559 against main

- **Status:** Recorded, 2026-09-24. One site, one capture, no ground truth.
- **Scope:** The whole 2,032.9 s Columbus Broadway capture replayed through the live server
  pipeline at 0.25x, once with PR #559 and once with main, and the two recordings compared
- **Revisions:** PR #559 at `97c9cf766` merged with main's #593 (`399f10b4d`, 0.5.1-pre36);
  main at `7bc810c9b` (0.5.1-pre35)
- **Capture:** `sf-street-speeds/raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng`
  (`sha256:aa69875be4dd45094d79c371ca25e434b3a4d8bec0966c116931720cc1653b66`)
- **Frozen result:** [state-estimation-columbus-0p25x-vs-main-20260924.json](state-estimation-columbus-0p25x-vs-main-20260924.json)
- **Related:** [State estimation](../../plans/lidar-state-estimation-plan.md) (21.1 D5),
  [heading coherence sprint](../../plans/lidar-heading-coherence-sprint-plan.md),
  [playback speed and track quality](playback-speed-vs-track-quality.md)

## Verdict

PR #559 changes what the heading does and leaves what the tracker finds and how fast it says
things move where main has them. Every measure of heading quality improves by a wide margin; track
counts, lifetimes and speed percentiles agree to within noise. That is the intended shape: the
heading-lock release and fragment guard ship on by default, while the position the filter tracks
is the medoid in both (D2's OBB centre is opt-in since D5).

Bumper-to-bumper gap is measured along the box, so a box that points where the vehicle is going
is a precondition for the headway metric. The one figure that moved the wrong way is the count of
frames with two overlapping boxes, which is the duplicate-identity signal headway pairing is
sensitive to; see [What this does not show](#what-this-does-not-show).

## Results

Both runs processed all 3,659,580 LiDAR packets into 20,330 frames: nothing was dropped at 0.25x,
and the two recordings cover the same 2,032.8 s (temporal IoU 1.0). A = main, B = PR #559.

**Heading**, from `vrlog-analyse compare`:

| Measure                                    |  Main | PR #559 |
| ------------------------------------------ | ----: | ------: |
| Heading updates accepted                   | 0.605 |   0.789 |
| Median course alignment (box against path) | 51.9° |   24.0° |
| Tracks ending with an unrecovered lock     | 0.449 |   0.099 |
| Mean observations per compared track       |  73.5 |    86.0 |

**Population**, from each run's `lidar_run_tracks`:

| Measure                       |   Main | PR #559 |
| ----------------------------- | -----: | ------: |
| Tracks                        |  3,915 |   3,924 |
| Median lifetime               | 2.80 s |  2.84 s |
| 90th percentile lifetime      | 14.7 s |  14.6 s |
| Share under 1 s               |   0.26 |    0.26 |
| Never confirmed (VRLOG share) |  0.455 |   0.461 |

The class mix is the same within a few tracks: dynamic 2,421 and 2,383, bird 669 and 704, car 278
and 290, pedestrian 229 and 233, cyclist 33 and 34, bus 7 and 9. The comparison pairs 3,891 tracks
by temporal overlap (24 only in main, 33 only in PR #559); 89.4% of pairs agree on class, and their
speeds differ by a median of 0.22 m/s.

**Speed**, percentiles of each track's maximum speed:

| Class      | n (main, #559) | p50 (mph)  | p85 (mph)  | p98 (mph)  |
| ---------- | -------------: | ---------- | ---------- | ---------- |
| Car + bus  |       285, 299 | 18.5, 18.5 | 22.1, 22.2 | 26.3, 26.2 |
| Pedestrian |       229, 233 | 3.0, 3.0   | 6.0, 5.9   | 12.3, 12.1 |
| Cyclist    |         33, 34 | 14.5, 14.6 | 19.7, 23.3 | 27.6, 28.3 |

Vehicle speeds agree within 0.15 mph at every percentile. The cyclist p85 moves by 3.6 mph on 34
tracks, which is two or three tracks at that rank.

## What this does not show

- **Accuracy.** There is no ground truth for this capture. A higher heading acceptance or a lower
  course error is agreement with the direction of travel, not a measured body yaw.
- **Duplicates.** Frames holding two live tracks whose boxes intersect rose from 11,843 to 12,849
  of 20,599 (57.5% to 62.4%). That is a proximity signal, not duplicate-object truth, and longer-
  lived tracks alone raise it. It is the figure to check against the kirk0 annotation reference
  before headway pairing relies on it.
- **Matching.** The comparison pairs tracks by temporal overlap, not identity.
- **Generality.** One site at one speed. The published scenes are built at 0.5x.

## Found while running it

- PR #559 alone panics at start-up when the server runs outside the repository tree, as it does
  on a Pi: `DefaultDBSCANParams` in `NewFrameCallback` calls `MustLoadDefaultConfig`, which looks
  for `config/tuning.defaults.json` on disk. Main's #593 gives that loader an embedded fallback;
  the branch picks it up on merge, which is why the PR #559 arm here includes it.
- Replay filters on the server's own `--lidar-udp-port`; the request has no port override. With a
  dev server holding UDP 2369, each arm replayed a copy of the capture with only the LiDAR
  destination port rewritten (`tcprewrite --portmap=2369:2479 --fixcsum`, and 2579 for main).
  Both copies were compared packet by packet with the original: 3,660,702 packets, identical
  payloads and timestamps to the microsecond, no mismatches.

## Reproduction

Artefacts, logs, both scratch databases and the full comparison
(`compare/main-vs-branch.json`, `sha256:f1316aa7a0fa189f1246392057bf8d772056a5dfb3337fc5f84e1c7589a8e2e3`)
are on the LiDAR volume under `velocity-campaign/columbus-0p25x-20260924/`, with a README.

Each server ran with its own database and ports, for example:

```bash
velocity-report-local --disable-radar --enable-transit-worker=false --enable-lidar \
  --lidar-forward --lidar-forward-mode=grpc --lidar-udp-port 2479 \
  --listen 127.0.0.1:18180 --lidar-listen 127.0.0.1:18181 --lidar-grpc-listen 127.0.0.1:50151 \
  --lidar-pcap-dir=/Volumes/lidar/lidar --lidar-vrlog-dir=<run>/vrlog --db-path=<run>/sensor_data.db
```

and was given the published scenes' replay request with the speed changed:

```json
{
  "pcap_files": ["<port-rewritten copy>"],
  "analysis_mode": true,
  "speed_mode": "scaled",
  "speed_ratio": 0.25,
  "settle_before_recording": false,
  "duration_seconds": 2032.925
}
```

The recordings were compared with `vrlog-analyse compare <main.vrlog> <branch.vrlog>`, and the
population and speed tables read from each run's `lidar_run_tracks`.
