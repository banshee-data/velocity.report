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

| Command        | Use                                                                                                                                                       |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `create`       | Start a label set from a pack and link the pack                                                                                                           |
| `attach`       | Add another pack of the same capture as a second domain and link it                                                                                       |
| `bind`         | Check a capture file holds the label set's packets; record its SHA-256 in `applies_to`. Changes no labels                                                 |
| `verify`       | Replay the journal and check every revision's state digest                                                                                                |
| `info`         | Counts, head revision and digest, bindings, and where a migrated set came from                                                                            |
| `project`      | Print a linked pack's membership in the pack's own samples and point indices, at the head or `--revision N`                                               |
| `publish`      | Copy the current state, without the journal, to an empty directory; `--replace` refreshes a published copy that is behind and has not been edited         |
| `publish-view` | Copy a linked pack beside a published label set as its view, with its data and physical references, after checking the view as a fresh copy would hold it |
| `check-view`   | Check a view and its label set as a fresh copy of their tracked files would hold them, with no history behind them                                        |
| `census`       | Count packet keys in capture files and report duplicates                                                                                                  |

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
| macOS annotation window, no label set   | "No label set yet": the first save makes one in `<annotation dir>/labels`; buttons attach the pack to an existing label set of the same capture, with how many packets they share                                |
| macOS annotation window, version 1 pack | Refused when opened, with where its labels went and the `annotation-v1-reader` tag that keeps the last build that reads it                                                                                       |
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
A set already published at the same head is left as it is, and `--replace` refreshes one that is
behind its source. It never writes over a copy someone has edited.

### Publishing a view

A label set holds no points, so nothing can be drawn or edited from it alone. A **view** is a pack
published beside its label set: `--view PACK` stages the pack's label set (found through its link)
and runs `labels publish-view`. The view gets the pack's data and physical references, and its link
is rewritten to the set beside it:

```text
labels/<capture stem>/
  <label set name>/      the label set: labels.json and the head files
  <pack directory>/      its view: manifest, samples, points, packets, returns, background,
                         physical-references.json, label-set-link.json -> ../<label set name>
```

Publish the run the labels were drawn on, so the view shows every labelled return. `publish-view`
refuses a pack whose foreground is not one of the set's domains, since the first save from it
would add that foreground whole: `attach` the pack and republish the set first. It also refuses
any physical reference pinned to a revision a copy without history cannot rebuild. Facets,
proposal layers, locks and revision archives stay behind.

### Editing from the dataset

Anyone with the dataset and a build of this repository can edit a published label set. Its files
on disk are then the new state, and the change is a small commit.

1. Clone the dataset with git and Git Xet installed, and let `git diff` show LFS files as text:
   `git config diff.lfs.textconv cat`.
2. `git checkout -b <your branch>`.
3. Start the server with `--lidar-annotation-dir=<clone>/labels/<capture stem>`. The window saves
   membership itself, but physical-reference and facet saves go through the server, which finds a
   pack only directly inside that directory.
4. In the macOS window, **Open Annotation Pack…** the view, and grant `labels/<capture stem>` when
   asked: the label set beside the view is written there.
5. Edit and save. The first save of a published set copies its head to `journal-base/` and starts a
   local journal; both are gitignored, with the locks and revision archives the editors write.
6. Before committing, run
   `velocity lidar annotation-reference repin --pack <view> --author <you>`, then
   `velocity lidar labels check-view --pack <view>`. A review pins the membership it was made
   against, and a later copy has no history to rebuild it. `repin` moves every review whose frames
   the edit did not touch to the new head. `check-view` checks the view as a fresh copy of the
   tracked files would hold, and names any review that needs doing again.
7. `git status` lists the label set's `labels.json` and head files, and the view's
   `physical-references.json` if it changed. Commit them. To propose the change, open a pull
   request on the dataset's Hub page, then `git push origin <your branch>:refs/pr/<number>`.

A one-mask edit of kirk0's view changed three lines of `labels.json` (the head), one line of
`masks.ndjson`, and 56 lines of `physical-references.json` (the re-pin of 25 reviews).

- **Branches do not merge.** Every save rewrites the head in `labels.json`, and `*.ndjson` is in
  LFS. Two branches from one head conflict; make the second edit again on the new head.
- **One object edit is one long line.** `objects.json` and `windows.json` are single-line canonical
  files, so a change to any object shows as the whole line.
- **Every save stores the whole `masks.ndjson` again in local LFS.** That is about 7 MB for kirk0.
  Xet uploads only the chunks that changed.
- **Splits land in the view's folder.** A split frozen through the server is written to
  `labels/<capture stem>/splits/`. Commit it only if it is meant to be published.
- **Facets break for later copies.** A facet pins the label set revision it was authored on, so it
  stops loading once membership moves past that revision. `check-view` reports it; there is no
  facet re-pin yet.

## Limits

- Only the Hesai Pandar40P layout (10 blocks × 40 channels) is decoded.
- A pack can be cut only from a VRLOG recorded by a build that carries return identity; re-record
  older runs by replaying their PCAP.
- Physical references, feature proposals and proposal layers stay bound to a pack (one run's view).
  Their membership pins name label set revisions through the projection, so after a membership
  edit the reviews need `annotation-reference repin` before the edit is published.
