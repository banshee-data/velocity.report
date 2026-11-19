# Following analysis across paired recordings

- **Status:** Provisional evidence analysis; physical qualification remains open
- **Related:** [Managed review](following-managed-review.md), [weekend plan](../../plans/lidar-weekend-following-drac-plan.md), [review preparation](../../../cmd/tools/following-review-prepare/README.md)

The [four-site run report](following-corpus-four-sites-2026-11.md) records the completed batch,
per-site counts, verified exports, UI checks and limitations. Its repeated reduction reproduces
the frozen estimates; it does not rerun or qualify their physical measurements.

The Runs page offers **Analyse following** for a selected recording. The backend resolves its
registered paired preparation and verifies source, calibration, header, canonical identity and
every evidence minute before computing the distribution. The browser supplies no paths.
Unprepared recordings report their missing prerequisite. One analysis runs at a time per service;
changing recordings cancels the browser request and discards its result. Playback and labels
are unchanged.

## Headless corpus

Build through `make build-radar-local`, then select explicit recording IDs:

```sh
velocity lidar following --db /absolute/local/review.db \
  --runs run-one,run-two --output /absolute/local/new-analysis --scenes
```

The output directory must not exist. `complete.json` appears only after every requested
recording and optional scene succeeds. A failure leaves diagnostic output without a completion
marker. The registered database is read-only. No VV, browser, listener or interactive input is
required. The corpus is explicit: no glob silently adds validation material. Repeated windows
are not additional independent exposure.

This command reduces already computed following evidence; it does not rerun tracking or change
parameters. For fresh following decisions from packets, first run the stamped
[review preparer](../../../cmd/tools/following-review-prepare/README.md) with an explicit capture,
calibration, site binding, cohort admission and checked-in tuning file. Its headless replay
produces these paired inputs. A scene export cannot recover missing estimator rows.

## Minimum time-gap population

Each recording contributes one minimum **estimated net time gap** per follower track with at
least one eligible, positive-duration sample. This is projected front-to-rear gap divided by
follower speed, not front-to-front passage headway. Non-closing pairs remain eligible for this
distribution even though closing DRAC is unavailable. The bins are `[0, 0.5)`, `[0.5, 1)`,
`[1, 1.5)`, `[1.5, 2)` and `[2, infinity)` seconds.

Tracks without usable estimates are counted separately. Record gaps, excluded pairings,
speed-floor exclusions and terminal samples supply no minimum. Each minimum retains its exact
decimal capture timestamp, leader and gap. Bin counts sum to tracks with a minimum; row buckets
independently sum to all evidence rows.

Software track fragmentation may count one physical vehicle more than once. A minimum over an
incomplete trajectory is not its true minimum. Online estimates remain provisional; aggregation
does not qualify endpoints, speed, common path or physical identity.

## Scene distance export

The shared player also draws the projected path from the follower's leading extreme to the
leader's trailing extreme. It is an arc-length projection, not a centre-to-centre ruler or a
lane boundary. Exact eligible samples retain both analysis speeds, DRAC availability and the
whole-track minimum net time gap. Gap-time colours use the five histogram bins above; the
track minimum can depend on later frames and is labelled accordingly. Missing values stay
unavailable. The recording's site histogram includes tracks without a usable minimum.

Following exports use the existing timeline strip to show maximum supported experimental DRAC
and minimum instantaneous time gap per bucket. Seeking never interpolates a missing sample.
Package three explicit development sites through the existing scene tool:

```sh
node tools/scene-capture/cli.mjs following-review /absolute/corpus /absolute/new-web-export \
  ashbury-downey,columbus-broadway,embarcadero-bryant /absolute/local-context
```

The optional context root holds `SITE/assets/` from the existing vector exporter, including its
checked placement. The packager verifies the archive deployment window and sensor frame before
copying context. Saved inferred lanes are cyan; unapproved lane proposals are violet. Missing
lanes are reported rather than substituted with a vehicle path. Serve the resulting directory
over local HTTP; its player, dependencies and scene data are self-contained.

After a full headless replay, `velocity lidar lane-field --recording VRLOG --run LOCAL_ID --out
NEW_FIELDS --upload --l16 TOKEN --gerardus http://arrow-worker:50061` reuses the lane job's existing
confirmed-track reader. It needs no invented database run and no second tracking replay. Only
the existing privacy-thresholded aggregate fields are uploaded. Retrieve `lane_proposals` through
the existing Gerardus/vector workflow to retain proposals without approving them.

`--scenes` writes a full-resolution tracks scene under each `recording-NNN/scene` with a
standalone `recording-NNN/manifest.json` for the shared scene reader. Exact samples
carry `following`: leader, bucket and eligible projected `gap_m`, `net_time_gap_s` and supported
`experimental_drac_mps2`. Missing samples are never interpolated. Ineligible samples retain their
bucket without values. An absent recorded leader removes measurements and produces
`leader_not_in_frame`.

Both actors use the scene's local keys; raw track IDs are not published. Header provenance
retains canonical digest, calibration, analysis producer and method, with provisional status.
Scene labels display usable estimated distance and time gap together, including when the
recording's per-frame class differs from the analysis inputs. Recorded classes are unchanged;
inspect disagreements rather than treating eligibility as physical classification. These remain local review
artefacts until the [publication checklist](../../geometry-priors-data-purpose.md#publication-checklist)
passes.

The four prepared development sites are Kirkham–9th, Ashbury–Downey, Columbus–Broadway and
Embarcadero–Bryant. Broderick remains annotation-review material in the validation cohort and is
excluded from this development batch.
