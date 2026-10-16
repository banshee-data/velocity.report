# Label sets: reference labels bound to a capture's returns

A label set holds reference objects and their per-frame membership, citing returns in a capture
rather than points in one run's export. It applies to every run of the capture and to every file
that holds its packets. The design and its decisions are in the
[capture-bound annotation format plan](../../../../docs/plans/lidar-capture-bound-annotation-format-plan.md).
This file is the format specification; the macOS client implements the same rules and is held to
them by the fixtures in `testdata/parity`.

## Identity

| Term          | Definition                                                                                                                                               |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Packet key    | First 8 bytes of SHA-256 over the UDP payload, read big-endian (`sha256-64:udp-payload`). Written as 16 lower-case hex digits in JSON                    |
| Packet table  | The label set's packets in a fixed order. Append-only: a position, once given, never changes                                                             |
| Layout        | `blocks` and `channels` per packet. Pandar40P: 10 × 40                                                                                                   |
| Return number | `packet_index × blocks × channels + block × channels + (channel − 1)`; block from 0, channel from 1                                                      |
| Window        | `[start, end)` in return numbers: every return of one frame as the labeller saw it                                                                       |
| Offset        | A return number minus its window's `start`. Masks and domain returns hold offsets                                                                        |
| Domain        | The foreground extraction a mask was made on (`kind: "foreground"`, run ID, config, params and build). Annotation is downstream of foreground separation |

A return outside a mask's domain was never shown to the labeller. Scoring treats it as unlabelled,
never as a negative.

## Files

| Path                                   | Content                                                                                              |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `labels.json`                          | Identity, layout, packet key scheme, verified files (`applies_to`), the head, `storage`, provenance  |
| `packets.bin`                          | Packet keys as little-endian uint64, in table order                                                  |
| `objects.json`                         | `correspondences`, `dismissed_proposals` and `objects`, one record per line                          |
| `domains.json`                         | Domains sorted by `domain_id`, one per line                                                          |
| `windows.json`                         | Windows sorted by `window_id`, one per line                                                          |
| `masks/<first>-<last>.ndjson`          | The masks of windows _first_ to _last_, one per line, sorted by `window_id` then `object_id`         |
| `domain-returns/<first>-<last>.ndjson` | Domain returns of the same windows, one line per (domain, window): `returns` as ranges               |
| `journal.ndjson`                       | One delta per save. Local only; never published                                                      |
| `journal-base/`                        | The head a journal started from, when it did not start at revision 1, in the same layout. Local only |
| `.lock`                                | Writer lock. Local only                                                                              |
| `.pending`                             | The revision a save is committing, while it commits it. Local only                                   |
| `views/<view>/`                        | Packs linked to the set (`label-set-link.json` → `../..`). Not part of the state                     |

A set of returns is written as `[[start, length], …]`: sorted, every range non-empty, and each range
starting after the previous one's end plus one, so a set has one encoding. An empty set is `[]`.
`uncertain` is omitted from a mask when empty; `members` never is.

## Storage layout

A state has one identity, its canonical encoding (below), and the files on disk are laid out for
people reading their diffs and for plain Git. `labels.json`'s `storage.window_chunk` (100 by
default) is how many windows each `masks/` and `domain-returns/` file spans: a file is named by its
first and last window as six-digit numbers, starts at a multiple of the span, and exists only when
it holds a line. A mask edit rewrites one small file, every text file can stay under Hugging Face's
10 MiB limit for plain Git, and Git merges two branches that edited different masks line by line.

- `objects.json`, `domains.json` and `windows.json` hold one record per line: the canonical
  encoding, except that each element of the top-level array, or of an array that is a top-level
  field, sits on its own line, indented two spaces per level, with `": "` after a key. An empty
  array is `[]`, and the file ends in `\n`.
- A chunk file holds canonical lines in canonical order.
- Reading decodes the files, digests their canonical encoding, and refuses files that are not
  exactly the layout of the state they decode to, a chunk of another span or holding another
  window's line, and the single files of the layout before (`masks.ndjson` beside the chunks).
  Hidden files (Finder's `.DS_Store`) and `*.tmp` in a chunk directory are passed over.
- A save writes only the files whose bytes change and removes the chunk files it empties.

`testdata/parity/storage/` holds the fixture state in this layout, one window per chunk.
`velocity lidar labels relayout` converts a set from the single-file layout, or changes its span;
the state, its digest and the journal are unchanged, and an interrupted run is finished by running
it again.

## Canonical JSON

The canonical encoding of a state is `packets.bin` and five files of canonical JSON, each followed
by `\n`: `objects.json`, `domains.json` and `windows.json` on one line, and `domain-returns.ndjson`
and `masks.ndjson` one line per record. It defines the state digest; on disk the same content is
kept in the storage layout above. Every journal line is canonical JSON too:

- Object keys sorted by code point, no whitespace between tokens.
- Numbers as ECMAScript `Number.prototype.toString` writes them, which is what Go's
  `encoding/json` produces: `1`, `0.1`, `0.000015`, `1e-7`, `1e+21`, `1.8000000000000003`.
- Strings escape only `"`, `\`, and control characters below U+0020 (`\b \f \n \r \t`, otherwise
  `\u00xx` in lower-case hex). `/`, `<`, `>`, `&` and all non-ASCII characters are written as
  UTF-8.
- Optional fields (`omitempty` in Go) are omitted when empty, zero or false; required fields are
  always present.

`testdata/parity/canonical-values.json` holds the awkward cases.

`labels.json` is the same encoding indented: keys sorted, two spaces per level, `": "` between a
key and its value, empty objects and arrays as `{}` and `[]`, then `\n`. It is outside the state
digest, but Go and the macOS client both rewrite it on every save, so they write it identically.
Otherwise a save from the other client would reorder the whole file. `testdata/parity/labels.json`
and `labels-minimal.json` pin it.

## State digest

`sha256:` plus lower-case hex of SHA-256 over the canonical encoding:

```text
"velocity.report label set state v2\n"
then, for each canonical file in the order packets.bin, objects.json, domains.json, windows.json,
domain-returns.ndjson, masks.ndjson:
  "<name>\n<decimal byte length>\n" followed by the file's bytes
```

The digest does not depend on the storage layout: a set laid out with any chunk span, or relaid,
keeps it.

`labels.json` records the head's revision and state digest. A revision is pinned as
`(label_set_id, revision, state digest)`.

## Validation

A state is refused when:

- the packet table holds a key twice;
- a window is empty, runs past the table, or overlaps another;
- a domain is not `foreground`, or domain returns cite an unknown domain or window or run past
  the window;
- an object id is empty or repeated, a status is not `proposed`, `reviewed` or `rejected`, or a
  correspondence cites an unknown object;
- a mask is repeated for its (object, window), cites an unknown object, window or domain, has an
  invalid status, completeness or visibility, or has members or uncertain returns outside its
  domain's returns for that window;
- a return is both a member and uncertain in one mask, or a member of two objects in one window.

`ReviewedMasks` returns only reviewed masks on reviewed objects, as a pack's projection does.

## Journal and saves

A journal line is a delta from its parent: `revision`, `parent_revision`, `parent_state`, `state`,
`change` (`author`, `session`, `operation`, `created_utc`), and the changes. Packets are appended.
Objects, domains, windows and domain returns are put or deleted whole. A mask carries its fields
(`domain_id`, `completeness`, `visibility`, `status`, `pose`, `provenance`) whole when any of
them changes or the mask is new, and its membership as `members_add`, `members_remove`,
`uncertain_add` and `uncertain_remove`. Applying a delta refuses to remove what is absent, add
what is present, or create a mask without its fields.

A save, holding an exclusive `flock` on `.lock`:

1. checks that `labels.json`'s head is the revision and digest the editor loaded, and that the
   head files still hold it; otherwise the editor must reload (`ErrConflict`);
2. computes the delta; writes nothing if it is empty;
3. writes `.pending` naming the revision it makes and its parent's state;
4. appends the line to `journal.ndjson` and syncs it;
5. replaces each changed head file by writing a temporary file beside it, syncing and renaming,
   and removes the chunk files it emptied;
6. replaces `labels.json`, which commits the revision, then removes `.pending`.

Opening a label set whose journal is one line ahead of `labels.json`, with `.pending` naming that
line and the head as its parent, rebuilds the head from the journal: that is a save interrupted
between steps 4 and 6. A final line without a newline is an interrupted append, never committed,
and is dropped. Any other disagreement is never repaired from the journal:

- Head files that do not hold `labels.json`'s state at the journal's revision were changed outside
  the store, by a merge or by hand, and opening refuses them.
- A journal that neither ends at the head nor is an interrupted save records edits to another head
  (`ErrForeignJournal`). In a Git working tree that is most often another branch's, since the
  journal is local and outlives a checkout. Switch back to that branch, or move `journal.ndjson`
  and `journal-base/` aside.

A save to a set with no journal (a published copy) first copies its head into `journal-base/` and
writes a base line, so editing a published set keeps local history from that point.

Restoring revision _n_ appends a delta that recreates it, with `restored_from: n`.

## Merging

`velocity lidar labels rehead` makes the head files on disk the head, after a Git merge or a
hand-resolved conflict. Git merges the head files of two branches that edited different masks;
`labels.json` conflicts, since each branch moved its head, and the copy keeps its own. `Rehead`
decodes the files in the declared layout and refuses a state that does not validate, such as two
branches giving one return to two objects. A state other than the head's becomes the next
revision. A local journal gets one line from its tip to that state, so its history replays through
the merge, and it must end at `labels.json`'s head; a copy without a journal just has a new head.
Files holding the head's state in another layout are rewritten without a revision.

Git can finish a merge by itself (`labels git-setup`). The `labels.json` driver,
`MergeManifests`, keeps this branch's head, adds the other branch's verified files, and records
the merge as `merge: {base, theirs}` when both branches moved their heads. A set holding a merge
record opens for nothing but `FinishMerge`, which clears it and reheads; the hooks run it, through
`FinishMerges`, before Git commits the merge. A rebase does not run the hooks, and leaves sets that
refuse to open until `finish-merge` runs. `physical-references.json` merges record by record
(`annotation.MergePhysicalReferences`).

## Binding a capture file

`Bind` reads a PCAP or PCAPNG, keys every UDP packet to or from the port, and records in
`applies_to` the file's SHA-256 and size, how many of the table's packets it holds, and how many
windows it covers completely, partly or not at all. A file that holds one of the table's packets
twice is refused. Binding changes no state and makes no revision.

## Views

An annotation pack is one run's view of a capture: its points and samples, plus `packets.bin` and
`returns.bin`, one little-endian uint64 return number per point in point order, increasing within
each sample, and each sample's frame range (`return_start`, `return_end`). The pack's return
numbers use its own packet table; `ViewMap` joins them to a label set by packet key. Every pack is
a view: `annotation.Export` builds one from a VRLOG whose point frames carry their return identity
(`packet_keys`, `return_refs` and the frame range on `PointCloudFrame`), and refuses a recording
made without it. Editing a mask through a view replaces only the members that view shows, so
returns another run kept and this one did not survive the edit.

`NewFromPack` starts a label set from a pack, and `AddPackDomain` adds another run's foreground
of the same capture as a second domain. `SaveMembership` folds a pack's edited projection back
into its label set; for a pack not linked yet, it creates a label set in the pack's capture
folder (`annotation.CaptureDirOf`), `<capture>/<capture stem>-<id>`, holding the membership as
revision 1, and links it. The tree a set and its packs sit in is in
[`annotation/layout.go`](../layout.go); `Nest` moves an older tree into it.
`Project` renders a revision as the pack-shaped membership the evaluators and the macOS client
read.

## Reader levels

`labels.json` may carry `min_reader`, the lowest reader level that reads the set correctly, as a
pack's `manifest.json` and `physical-references.json` may (`annotation.ReaderLevel`). A reader
refuses a file above its level; a file without the field is level 1; a save keeps the level it
read. It is outside the state digest. `schema_version` still changes when a format breaks.

| Level | Adds                                   |
| ----- | -------------------------------------- |
| 1     | The storage layout, as first published |
