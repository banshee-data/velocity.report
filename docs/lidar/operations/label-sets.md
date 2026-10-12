# Label sets

A label set holds reference objects and their per-frame membership for one capture. It cites
returns by `(packet key, block, channel)`, where the packet key is a digest of the UDP payload the
sensor sent. So one label set applies to every run of the capture, at any settings, and to every
file that holds its packets: the source roll, a clipped excerpt, or the published PCAPNG. History is
a journal of deltas, one line per save, not a full copy per save.

- **Format:** [`internal/lidar/annotation/labelset/README.md`](../../../internal/lidar/annotation/labelset/README.md)
- **Design:** [capture-bound annotation format plan](../../plans/lidar-capture-bound-annotation-format-plan.md)

## Where things live

| Path                                          | What it is                                                                      |
| --------------------------------------------- | ------------------------------------------------------------------------------- |
| `<annotation dir>/labels/<name>/`             | A label set: `labels.json`, the head files, `journal.ndjson` and `.lock`        |
| `<annotation dir>/<pack>/`                    | A pack: one run's foreground of a capture, with `packets.bin` and `returns.bin` |
| `<annotation dir>/<pack>/label-set-link.json` | Names the label set holding the pack's membership, by id and relative path      |

The annotation dir is the server's `--lidar-annotation-dir` (`LIDAR_ANNOTATION_DIR` in `local.mk`).

Annotation is downstream of foreground separation. Every mask names, as its domain, the foreground
extraction it was made on. A return outside that foreground was never shown to the labeller and is
unlabelled for scoring, never a negative. Label on a generous foreground: what it leaves out, a
label set cannot measure.

## Commands

All are `velocity lidar labels <command>`; built through `make`, so records name the build.

| Command   | Use                                                                                                         |
| --------- | ----------------------------------------------------------------------------------------------------------- |
| `create`  | Start a label set from a pack and link the pack                                                             |
| `attach`  | Add another pack of the same capture as a second domain and link it                                         |
| `bind`    | Check a capture file holds the label set's packets; record its SHA-256 in `applies_to`. Changes no labels   |
| `verify`  | Replay the journal and check every revision's state digest                                                  |
| `info`    | Counts, head revision and digest, bindings, and where a migrated set came from                              |
| `project` | Print a linked pack's membership in the pack's own samples and point indices, at the head or `--revision N` |
| `publish` | Copy the current state, without the journal, to an empty directory                                          |
| `census`  | Count packet keys in capture files and report duplicates                                                    |

Everything that reads a pack's membership reads it through the label set: splits, physical
references, features, the per-frame and near-face evaluators, and the server's pack listing. It is
written through the label set too, by the macOS window's save or, in Go, `labelset.SaveMembership`.
The first save to a pack with no link creates a label set beside it, in
`<annotation dir>/labels/<capture stem>-<id>`, and links the pack. Freezing a split over a linked
pack binds its case to the label set's verified captures by SHA-256, so a replay of any verified
file holds its role and a different file does not.

## Where it shows

| Place                                   | What it shows                                                                                                                                                                                                    |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS annotation window, label set line | The label set's name and revision; each verified capture file with a short SHA-256, packets found and frames covered (or the `bind` command when there are none); and, for a migrated set, the pack it came from |
| macOS annotation window, replay check   | With "Follow the main view" on: whether the main view's current frame holds this label set's packets (same capture, partly, a different capture, or cannot check when frames carry no packet keys)               |
| Web, Segments page, Annotation packs    | Every pack: its label set and revision, or that it has no labels yet; reviewed work; and each verified capture file                                                                                              |

The replay check compares the packet keys on the gRPC stream with the label set's packet table, so it
is a check of content, whichever verified file the main view replays. A live source, a VRLOG recorded
before return identity, or an older server sends no keys, and the window says it cannot check.

## Version 1 packs

Version 1 packs had no returns, and kept their membership in the pack as `annotations.json` with a
full copy per revision. All of them were migrated to label sets on 2026-10-12, by an exact mapping
from each point to its decoded return; the method and results are in the
[migration report](annotation-label-set-migration.md). This build reads version 2 only: the Go
reader, the server's listing and the macOS window refuse a version 1 pack and say why. The
originals are archived in `work/annotation-v1/` on the LiDAR volume, outside the repository, and the
git tag `annotation-v1-reader` keeps the last build that reads them and the `labels migrate`
command that carried them across.

## Binding and publishing

`bind` reads a whole capture file, so bind once per file and expect it to take as long as reading
the file. A file that holds none of the packets, or holds one twice, is refused.

`tools/s2-archive/stage-label-sets.py` publishes label sets into the Hugging Face dataset, joined to
a capture row only by `raw_sha256`. See the [dataset layout](../../../tools/s2-archive/huggingface-dataset-layout.md#label-sets).

## Limits

- Only the Hesai Pandar40P layout (10 blocks × 40 channels) is decoded.
- A pack can be cut only from a VRLOG recorded by a build that carries return identity; re-record
  older runs by replaying their PCAP.
- Physical references, feature proposals and proposal layers stay bound to a pack (one run's view).
  Their membership pins name label set revisions through the projection.
