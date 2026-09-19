# LiDAR deterministic track identity plan

- **Status:** Specification; narrower interim fix already shipped (see 0)
- **Layers:** L5 Tracks, storage, L9 Endpoints, live ingest
- **Target:** v0.5.4; sequenced after the narrower evidence-oracle fix, before any tooling depends on track_id for cross-run comparison
- **Canonical:** [lidar-state-estimation-plan](lidar-state-estimation-plan.md) (owns L5 track lifecycle; this plan owns only the identity assigned at track birth)
- **Depends on:** Nothing structurally; can proceed independently. Benefits from the evidence-oracle fix in 0 already having exercised the failure mode.

## 0. What's already shipped, and why it isn't enough on its own

Comparing two independent replays of the same phase01 corpus (one from Sept 13, one from Sept 15) showed 99.2% of frames' `lidar_track_estimates`/`lidar_track_residuals` rows as "different." The actual x/y/vx/vy/covariance/residual values were byte-identical in every one of 42,947 rows once compared correctly; the apparent drift was entirely `track_id` — a random UUID assigned at [`tracking.go:491`](../../internal/lidar/l5tracks/tracking.go), deliberately, so it stays collision-free across tracker resets and restarts — propagating into `estimate_id` and getting hashed as content by the evidence-oracle.

The interim fix (shipped): persist the tracker's existing deterministic per-run `CreationSequence` alongside each estimate, and have the evidence-oracle key/group by that instead of the random `track_id`/`estimate_id`. This makes offline corpus comparisons trustworthy. It does not, and was never meant to, give the live system a deterministic identity — `CreationSequence` resets to 1 on every tracker construction, which is fine for a single replay invocation and unsafe as a persisted key in a system that restarts in the field.

This plan is the wider fix: can track identity be made deterministic (reproducible across an identical replay) **and** safe to use as the actual persisted `track_id` in the live system, closing the gap left by 0.

## 1. Why this is worth doing, not just nice to have

Two separate things currently masquerade as one field:

| Need                                                                                          | Currently served by                 | Problem                                                                                |
| --------------------------------------------------------------------------------------------- | ----------------------------------- | -------------------------------------------------------------------------------------- |
| Global, collision-free identity for the live system across resets/restarts/years of uptime    | `track_id` (random UUID)            | Correct for this need, but not reproducible, so it can't also serve determinism        |
| Reproducible identity across two replays of identical input, for corpus/baseline verification | `CreationSequence` (interim fix, 0) | Correct for this need, but resets per process — cannot be the live system's actual key |

Every future tool that wants to answer "did this code change alter the tracker's output" has to know about this split and route around it, as the evidence-oracle now does. A single identity scheme that is both reproducible and restart-safe would remove that whole class of future workaround.

## 2. Options considered

### 2.1 Monotonic counter as the actual `track_id` — rejected

Rejected in review before this plan was written. `NextTrackID` resets to 1 on every `NewTracker()` call ([`tracking.go:265`](../../internal/lidar/l5tracks/tracking.go)). Used as a persisted `PRIMARY KEY` in the live schema, every process restart would immediately collide with the prior session's track IDs — not a rare edge case, a guaranteed failure on the very next restart. Making it restart-safe would require bootstrapping the counter from `MAX(...)` in the database at startup, which reintroduces exactly the kind of coordination a UUID exists to avoid, for a benefit (reproducibility) the live system never asked for.

### 2.2 Deterministic UUID from timestamp alone — rejected

A UUID derived purely from a creation timestamp (regardless of resolution — micro, nano, whatever) collides whenever two tracks are born in the same frame, because `frame_unix_nanos` is identical for every cluster processed within that frame by construction. This is common, not rare — the phase01 embarcadero-folsom capture used in the investigation regularly carries 27 simultaneous tracks in a single frame, and not all of them are survivors from earlier frames. Timestamp resolution is not the constraint; the input is genuinely tied.

### 2.3 Deterministic UUID from `(sensor_id, frame_unix_nanos, cluster_id)` — close, but not sufficient alone

A v5-style UUID (`uuid.NewSHA1(namespace, name)`) hashed from the frame's cluster identity disambiguates same-frame births (`cluster_id` is unique within a frame) and is computed from fields that already flow identically through live and replay ([`l4perception.WorldCluster`](../../internal/lidar/l4perception/), [`l2frames.LiDARFrame`](../../internal/lidar/l2frames/)). It is also restart-safe: there is no counter to reset, so a track born after a restart naturally gets a new ID as long as its frame timestamp is new, which it always will be in the live system (real time keeps advancing).

The gap: `sensor_id` in this codebase is the sensor _model_ string (`"hesai-pandar40p"`), not a per-deployment identity. Two different physical sites running the same sensor model would produce identical `(sensor_id, frame_unix_nanos, cluster_id)` tuples whenever their clocks happen to align, colliding the moment their data is combined into one database — which the corpus/evidence-oracle work already does deliberately (marina-webster-beach, columbus-broadway, and embarcadero-folsom all share `sensor_id="hesai-pandar40p"` and are only actually distinguished today by `source_id`, a replay-only per-capture content hash with no live-side equivalent).

### 2.4 Deterministic UUID from `(site_id, sensor_id, frame_unix_nanos, cluster_id)` — proposed

Fold in `site_id` (already an existing, live concept: `lidar_sites.site_id TEXT PRIMARY KEY`, [`internal/lidar/storage/sqlite/site_store.go`](../../internal/lidar/storage/sqlite/site_store.go)) as the missing per-deployment namespace. This seed is then:

- **Symmetric between live and replay**, once threaded through (see 4 for the gap in replay's current identity model).
- **Collision-safe across sites** sharing a sensor model, which 2.3 was not.
- **Restart-safe**, for the same reason as 2.3.

Residual risk, stated plainly rather than glossed over: a hash collision if `(site_id, sensor_id, frame_unix_nanos, cluster_id)` genuinely repeats for two different physical birth events. That requires the sensor's clock to replay an exact prior timestamp (an NTP step-adjustment or reboot-without-RTC scenario, not impossible over years of field deployment) _and_ DBSCAN to assign the same cluster ordinal to a different object in that repeated frame _and_ that cluster to start a new track both times. Rare and coincidental, not structural — a materially different risk profile from 2.1's guaranteed-on-every-restart collision.

## 3. Recommendation

Proceed with 2.4. It is the only option that satisfies both the live system's collision-safety requirement and the reproducibility goal with one mechanism, rather than trading one for the other.

## 4. What has to be true first

`site_id` is not currently available at the point a track is born (`Tracker.initTrack`, [`tracking.go`](../../internal/lidar/l5tracks/tracking.go)) in either the live or replay path — it would need threading down from wherever the pipeline is configured. Concretely:

- **Live path**: confirm what identifies "this deployment" at startup (site configuration, `site_config_periods`) and thread it into `TrackerConfig` or the `Tracker` constructor.
- **Replay path**: `replayeval.Config`/`TrackingPipelineConfig` would need an equivalent field. `source_id` (already deterministic, already unique per capture) is the natural replay-side stand-in for `site_id` — the two are not the same concept, but both answer "which physical deployment/capture produced this," so the plan should treat them as the two arms of one abstraction rather than force replay to fabricate a site identity it doesn't have.
- **Format compatibility**: `track_id` is `TEXT` with no format-constraining `CHECK`, and every one of the 20+ consuming files treats it as an opaque string (confirmed by inspection during this plan's review; not exhaustively unit-tested). A v5 UUID is a valid UUID string, so this should be a non-breaking format change, but each consumer is worth a pass before cutover, particularly `l9endpoints` (visualiser protocol) and `server/track_api*.go` (public HTTP surface).
- **Historical data**: 3.5M+ existing rows keep the old random-UUID format permanently. This is a forward-only identity change, not a backfill; tooling should not assume a uniform ID format across all history.

## 5. Rollout shape (sketch, not yet sequenced into phases)

1. Thread a per-deployment identity into both the live and replay pipeline configs (the prerequisite in 4).
2. Add the deterministic derivation as an opt-in alternate path in `Tracker.initTrack`, gated by config, so it can be validated against the existing random-UUID path on real corpora before becoming default.
3. Validate: replay the same corpus twice, confirm `track_id` itself is now byte-identical between runs (the evidence-oracle's `CreationSequence`-based workaround from 0 becomes unnecessary, though it stays correct either way).
4. Validate collision-safety empirically against the full available corpus history (multi-site, multi-day) before flipping the live default.
5. Flip default; keep the random-UUID path available behind the same flag as a rollback.

## 6. Open questions

- Does the live ingest path currently have a stable `site_id` available at the point tracks are constructed, or does this require plumbing changes beyond `Tracker` itself (e.g., in `internal/lidar/pipeline` config wiring)?
- Should `source_id` and `site_id` be formally unified into one "deployment identity" concept shared by both paths, or kept as two names for the same role? Unifying is cleaner; keeping them separate is less invasive to the replay code that already depends on `source_id`'s exact current derivation.
- Is there any consumer of `track_id` that depends on it being _unguessable_ (not just unique)? Not found during this plan's review, but not exhaustively checked either.
