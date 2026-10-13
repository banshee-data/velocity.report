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

| File                    | Content                                                                                    | In the state digest |
| ----------------------- | ------------------------------------------------------------------------------------------ | ------------------- |
| `labels.json`           | Identity, layout, packet key scheme, verified files (`applies_to`), the head, provenance   | No                  |
| `packets.bin`           | Packet keys as little-endian uint64, in table order                                        | Yes                 |
| `objects.json`          | `{"correspondences":[…],"dismissed_proposals":[…],"objects":[…]}`                          | Yes                 |
| `domains.json`          | Array of domains, sorted by `domain_id`                                                    | Yes                 |
| `windows.json`          | Array of windows, sorted by `window_id`                                                    | Yes                 |
| `domain-returns.ndjson` | One line per (domain, window), sorted by `domain_id` then `window_id`: `returns` as ranges | Yes                 |
| `masks.ndjson`          | One line per mask, sorted by `window_id` then `object_id`                                  | Yes                 |
| `journal.ndjson`        | One delta per save. Local only; never published                                            | No                  |
| `journal-base/`         | The head a journal started from, when it did not start at revision 1. Local only           | No                  |
| `.lock`                 | Writer lock. Local only                                                                    | No                  |

A set of returns is written as `[[start, length], …]`: sorted, every range non-empty, and each range
starting after the previous one's end plus one, so a set has one encoding. An empty set is `[]`.
`uncertain` is omitted from a mask when empty; `members` never is.

## Canonical JSON

Every head file except `packets.bin` and `labels.json`, and every journal line, is canonical JSON
followed by `\n`:

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

`sha256:` plus lower-case hex of SHA-256 over:

```text
"velocity.report label set state v2\n"
then, for each file in the order packets.bin, objects.json, domains.json, windows.json,
domain-returns.ndjson, masks.ndjson:
  "<name>\n<decimal byte length>\n" followed by the file's bytes
```

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
   head files still digest to it; otherwise the editor must reload (`ErrConflict`);
2. computes the delta; writes nothing if it is empty;
3. appends the line to `journal.ndjson` and syncs it;
4. replaces each head file by writing a temporary file beside it, syncing and renaming;
5. replaces `labels.json` last, which commits the revision.

Opening a label set whose head disagrees with the journal's last line rebuilds the head from the
journal. A final line without a newline is an interrupted append, never committed, and is dropped.
A save to a set with no journal (a published copy) first copies its head into `journal-base/` and
writes a base line, so editing a published set keeps local history from that point.

Restoring revision _n_ appends a delta that recreates it, with `restored_from: n`.

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
into its label set; for a pack not linked yet, it creates a label set beside the pack, in
`<pack parent>/labels/<capture stem>-<id>`, holding the membership as revision 1, and links it.
`Project` renders a revision as the pack-shaped membership the evaluators and the macOS client
read.
