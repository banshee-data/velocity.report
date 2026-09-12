# S2 archive site index

Every site the sensor was parked at on `/Volumes/lidar/lidar/s2`, across the
three recording days, with the captures each one spans and an approximate
position.

A site is a **static** stretch: the car drove between junctions with the sensor
running, so a day is one continuous recording and the sites are the stretches
within it where the platform was stationary. `site-index.json` is the output;
everything else here produces or feeds it.

| File                  | What it is                                                               |
| --------------------- | ------------------------------------------------------------------------ |
| `site-index.json`     | The index. Generated — edit `map-marks.json` and rebuild instead.        |
| `map-marks.json`      | Operator-named positions and optional orientation measurements.          |
| `site-joins.json`     | Operator assertions that separated static fragments belong to one visit. |
| `build-site-index.py` | Stitches the segment analysis and attaches positions.                    |
| `deployments.py`      | Reconstructs recording blocks from capture filenames alone.              |

The scene catalogue being developed in [PR #569](https://github.com/banshee-data/velocity.report/pull/569) reads this index. Its map generator and `make render-scene-map` target are part of a later extraction, not this archive-tooling change. Each scene export is joined to a site
by the wall clock in its header and inherits that site's position, so a mark
corrected here moves the marker, the cell and the token on `/scenes/` once
`make render-scene-map` has run. A scene the archive does not cover, or whose
map reading is known to be wrong, is positioned in
`public_html/scene-overrides.json` instead, and the override wins.

## The two analyses, and why they differ

`velocity lidar pcap-split` classifies a capture into motion and static
segments. The multi-file CLI in [PR #569](https://github.com/banshee-data/velocity.report/pull/569) documents how to run a continuous analysis:

> Several `--pcap` flags analyse the captures as one continuous stream, which
> keeps the background model settled across the file boundaries. Analysing each
> file separately restarts that model and reports the settling as motion.

The archive's original analysis under `s2/analysis/` was run per file, so every
five-minute boundary interrupts a site: 9/2 and 9/3 arrive as 17 and 19
fragments rather than 9 and 8 sites. `build-site-index.py` stitches those back
together, bridging motion shorter than three minutes, which recovers exactly the
expected counts — and no more, since bridging five minutes starts merging
genuinely separate sites.

9/1 had no analysis at all. It was re-analysed as continuous streams into
`s2/analysis-continuous/`, one per recording block, and comes out clean: six
static stretches of 18 to 22 minutes with no stitching needed. Continuous analysis is not yet a replacement for the per-file inputs: a 9/3 run misses a recorded stop and starts two others late. Keep both inputs while the [classification investigation](../../docs/plans/continuous-classification-brief.md) is open.

## Which captures a site spans

A per-file analysis names its one capture and nothing else. A continuous
analysis reports the whole block as one stream: it lists every capture in
`config.pcap_files` but attributes no segment to any of them, and its top-level
`input_file` is only the first. Reading that field per segment credits a whole
day to one capture, which is why 9/1 first came out as six single-capture
sites. Consecutive captures abut, so each one covers the clock from its own
start to the next one's, and a segment spans whichever of those it overlaps.

A site is roughly twenty minutes, which is four or more five-minute captures.
Fewer than four means the period was truncated, not that the site was short.

## Positions are approximate

`map-marks.json` holds positions read by eye from a photograph of a hand-marked
paper map, matched to sites by day and clock time. They are neighbourhood-level:
where a published scene gives an independent check, the map reading was about
450 m out. Treat them as a starting point for a survey, not as one — which is
why each carries a confidence and why a mark that could not be read leaves the
position null rather than guessed.

## Rebuild

```bash
python3 tools/s2-archive/build-site-index.py   # after editing map-marks.json
python3 tools/s2-archive/deployments.py        # recording blocks from filenames
```

## Static PCAPNG release set

`export-static-pcaps.py` makes one PCAPNG and one provenance sidecar per indexed
site. It clips the original files to the exact indexed start/end and joins the
pieces in order. The export uses the complete operator-approved site interval,
including an asserted tripod nudge such as `van-ness-sacramento`; classifier
fragments remain in `static_parts` for audit but do not silently remove packets.

```bash
python3 tools/s2-archive/export-static-pcaps.py \
  --archive /Volumes/lidar/lidar/s2 \
  --output /Volumes/lidar/lidar/s2/static-huggingface
```

Use `--dry-run` to review paths, or repeat `--site van-ness-sacramento` for one
export. The command refuses to overwrite an existing PCAPNG or sidecar, so a
published artifact cannot be revised by accident.

## Rebuild inputs and publication state

The scripts read the local archive at `/Volumes/lidar/lidar/s2`; the PCAPs and analysis JSON are not included in this PR. The committed index is an archive snapshot, not evidence that every linked recording is published on `main`. Its `published_as` fields record scene identifiers observed on the development branch. A rebuild on a checkout without those exports sets the corresponding fields to null.

`build-site-index.py` reads both analysis trees, the field marks, the operator joins, and any locally available scene headers. It writes only `site-index.json`. `deployments.py` prints a filename-based grouping; its eleven-minute grouping threshold is not the production replay continuity policy. The publishing launcher remains with PR #569 because it requires the new multi-file replay API.

## Scene alignment tools

Open a scene with `?dev=true` to reveal **Copy this view** and developer
alignment controls. This preference persists in local storage across scene
visits; `?dev=false` disables it. If storage is blocked, the query still applies
to the current page.

Use **Align from above** to stop the flight and compare the ground grid with
the kerbs. Adjust the grid azimuth and the sensor's clockwise bearing from true
north independently; the lower-right compass previews true north. Angles wrap into
0–360 degrees, zero is a measurement, and blank means unmeasured. These controls
adjust rotation, not a translation of the grid. Compass vantage labels preview
the north measurement; authored vantage positions stay in sensor coordinates.

The compass is available outside developer mode too. It follows the view bearing
and tilt, capped at 54.7 degrees from vertical (true isometric), and lies flat
in overhead views. Click it to stop flight and face north-up directly overhead,
keeping the current centre and zoom. This is another vantage: selecting it
clears the selection on every labelled vantage button. `N?` means north has not been measured; the fallback uses sensor zero.

**Save to map marks…** opens a file picker in Chrome or Edge on HTTPS or
localhost. Select the checkout's `tools/s2-archive/map-marks.json`; the viewer
updates only the two angle fields on the uniquely matching site ID, preserving
other marks and metadata. Cancelling leaves only a preview. Measurements are
not saved in local storage or in scene overrides. The page displays the site ID
and angles for manual transfer when direct file access is unavailable.

After saving, publish the canonical measurements through the existing pipeline:

```bash
python3 tools/s2-archive/build-site-index.py
pnpm s2-hilbert:scene-map
pnpm --dir public_html build
```

The index rebuild requires the local archive described above. **Copy this view**
continues to produce the sensor-relative camera JSON for that site's
`assets/vantages.json`; it does not change the orientation measurements.
