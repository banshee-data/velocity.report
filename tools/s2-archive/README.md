# S2 archive site index

Every site the sensor was parked at on `/Volumes/lidar/lidar/s2`, across the
three recording days, with the captures each one spans and an approximate
position.

A site is a **static** stretch: the car drove between junctions with the sensor
running, so a day is one continuous recording and the sites are the stretches
within it where the platform was stationary. `site-index.json` is the output;
everything else here produces or feeds it.

| File                  | What it is                                                         |
| --------------------- | ------------------------------------------------------------------ |
| `site-index.json`     | The index. Generated — edit `map-marks.json` and rebuild instead.  |
| `map-marks.json`      | Positions read off the field map, by hand. The only input to edit. |
| `build-site-index.py` | Stitches the segment analysis and attaches positions.              |
| `deployments.py`      | Reconstructs recording blocks from capture filenames alone.        |

## The two analyses, and why they differ

`velocity lidar pcap-split` classifies a capture into motion and static
segments. Its own help is explicit about how to run it:

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
static stretches of 18 to 22 minutes with no stitching needed. Rerunning 9/2 and
9/3 the same way would remove the need to bridge them.

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
