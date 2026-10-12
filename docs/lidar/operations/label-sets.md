# Label sets

A label set holds reference objects and their per-frame membership for one capture. It cites
returns by `(packet key, block, channel)`, where the packet key is a digest of the UDP payload the
sensor sent. So one label set applies to every run of the capture, at any settings, and to every
file that holds its packets: the source roll, a clipped excerpt, or the published PCAPNG. History is
a journal of deltas, one line per save, not a full copy per save.

- **Format:** [`internal/lidar/annotation/labelset/README.md`](../../../internal/lidar/annotation/labelset/README.md)
- **Design:** [capture-bound annotation format plan](../../plans/lidar-capture-bound-annotation-format-plan.md)

## Where things live

| Path                                                | What it is                                                                                |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `<annotation dir>/labels/<name>/`                   | A label set: `labels.json`, the head files, `journal.ndjson` and `.lock`                  |
| `<annotation dir>/<pack>/`                          | A version 2 pack: one run's foreground of a capture, with `packets.bin` and `returns.bin` |
| `<annotation dir>/<pack>/label-set-link.json`       | Names the label set holding the pack's membership, by id and relative path                |
| `<annotation dir>/<pack>/annotations.json` (legacy) | A version 1 pack's membership. Read-only; migrate it                                      |

The annotation dir is the server's `--lidar-annotation-dir` (`LIDAR_ANNOTATION_DIR` in `local.mk`).

Annotation is downstream of foreground separation. Every mask names, as its domain, the foreground
extraction it was made on. A return outside that foreground was never shown to the labeller and is
unlabelled for scoring, never a negative. Label on a generous foreground: what it leaves out, a
label set cannot measure.

## Commands

All are `velocity lidar labels <command>`; built through `make`, so records name the build.

| Command   | Use                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------ |
| `migrate` | Carry a version 1 pack's labels into a label set and write a version 2 copy of the pack linked to it (below) |
| `create`  | Start a label set from a version 2 pack and link the pack                                                    |
| `attach`  | Add another version 2 pack of the same capture as a second domain and link it                                |
| `bind`    | Check a capture file holds the label set's packets; record its SHA-256 in `applies_to`. Changes no labels    |
| `verify`  | Replay the journal and check every revision's state digest                                                   |
| `info`    | Counts, head revision and digest, bindings, and where a migrated set came from                               |
| `project` | Print a linked pack's membership as a version 1 sidecar, at the head or `--revision N`                       |
| `publish` | Copy the current state, without the journal, to an empty directory                                           |
| `census`  | Count packet keys in capture files and report duplicates                                                     |

A linked pack reads its membership through the label set wherever a sidecar used to be read:
splits, physical references, features, the per-frame and near-face evaluators, and the server's
pack listing. A sidecar save to a linked pack is refused; membership is written through the label
set. Freezing a split over a linked pack binds its case to the label set's verified captures by
SHA-256, so a replay of any verified file holds its role and a different file does not.

## Migrating a version 1 pack

```bash
make build-velocity
./velocity lidar labels migrate \
  --pack   "$LIDAR_ANNOTATION_DIR/<v1 pack>" \
  --pcap   "$LIDAR_PCAP_DIR/<capture the pack was cut from>" \
  --labels "$LIDAR_ANNOTATION_DIR/labels/<name>" \
  --view   "$LIDAR_ANNOTATION_DIR/<v1 pack>-v2" \
  --name   <name> --report <report.json>
```

The migration decodes the capture through the L1 parser and an offline L2 frame builder, finds each
pack sample's frame by its start time, and requires every pack point to be exactly one decoded
return of that frame: bit-identical float32 coordinates, in frame order. A point with no such return,
or one that coincides with another return, refuses the whole migration and leaves the version 1
files as they were. Labels move only by an exact mapping, never by nearby coordinates.

It converts every retained version 1 revision into a journal line with its original author, session,
operation and time, so revision _n_ stays revision _n_ and pins on version 1 revisions still name
the same membership. Returns two objects claim in one window, which later validation forbids, are
resolved: a reviewed claim keeps the return and every proposed claim moves it to that mask's
uncertain set. Two reviewed claims on one return refuse the migration.

The version 2 copy has the same points, samples and backgrounds, plus their returns. It is linked to
the label set, and physical references, feature proposals and proposal layers are carried across with
their pins moved: pack digest, dataset id and each pinned membership revision's digest, which now
names the label set projected onto the new pack. Their own revision histories stay with the version
1 pack. The version 1 pack is not modified; archiving or pruning it is a separate decision.

## Binding and publishing

`bind` reads a whole capture file, so bind once per file and expect it to take as long as reading
the file. A file that holds none of the packets, or holds one twice, is refused.

`tools/s2-archive/stage-label-sets.py` publishes label sets into the Hugging Face dataset, joined to
a capture row only by `raw_sha256`. See the [dataset layout](../../../tools/s2-archive/huggingface-dataset-layout.md#label-sets).

## Limits

- Only the Hesai Pandar40P layout (10 blocks × 40 channels) is decoded.
- A version 2 pack can be cut only from a VRLOG recorded by a build that carries return identity;
  re-record older runs by replaying their PCAP.
- Physical references, feature proposals and proposal layers stay bound to a pack (one run's view).
  Their membership pins name label set revisions through the projection.
