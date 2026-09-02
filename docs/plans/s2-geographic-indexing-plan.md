# S2 geographic indexing plan

- **Status:** Planned; implementation targeted at v2.0
- **Layers:** Storage, filesystem artefacts, replay cases, run records
- **Style guide:** [geographic-indexing](../lidar/architecture/geographic-indexing.md) is normative for levels, tokens, and displays. This plan does not restate those rules; it applies them
- **Related:** [pcap-ground-plane-export-tool-plan](pcap-ground-plane-export-tool-plan.md), [pcap-analysis-mode](../lidar/operations/pcap-analysis-mode.md), [VRLOG_FORMAT](../../data/structures/VRLOG_FORMAT.md)

## Scope

Everything here is future implementation work. It was separated from the style
guide so that the guide can be adopted immediately, as a convention, without
implying that any existing mechanism changes at the same time.

The two documents divide as follows:

| Question                                                         | Answered in                                                 |
| ---------------------------------------------------------------- | ----------------------------------------------------------- |
| Which levels, how are tokens written, where may each form appear | [Style guide](../lidar/architecture/geographic-indexing.md) |
| What gets built, in what order, and what must agree with what    | This plan                                                   |

## Open implementation questions

Carried over from the style guide, which deliberately does not answer them:

- How to represent the unsigned S2 CellID in SQLite's signed 64-bit `INTEGER`,
  and whether canonical token strings are the safer database boundary.
- How JSON serialises the value.
- Which indexes and migrations are needed, and for which tables.

## Static PCAP and VRLOG artefact conformance

The shipped `pcap-split` workflow produces motion and static PCAP segments,
`segments.json`, and a human-readable summary. The upcoming S2 implementation
must extend that workflow so a located static segment carries one consistent
geographic tag through every derived artefact:

```text
representative WGS84 position
        → canonical S2 L13
        → Parent(10)
        → canonical S2 L10
             ├── static PCAP filename and L10 directory
             ├── segments.json and summary.txt
             ├── derived VRLOG directory and header.json
             └── replay-case and run-record database columns
```

The filename tag makes a detached static capture coarse-addressable without
opening a sidecar. The one machine naming grammar is:

```text
<prefix>-static-<index>-s2-l10-<canonical-l10-token>.pcap
```

For example:

```text
capture-static-0-s2-l10-808581.pcap
capture-static-0-s2-l10-808581.vrlog/
```

`808581` is the canonical L10 token. The family display `80858-1` and the UI
scan cue never appear in a machine filename or directory. A coarse archive may
also place both artefacts beneath `808581/`; repeating the canonical tag in the
basename is intentional because files are often copied out of their parent
directory.

### Tag derivation and eligibility

- Prefer a configured surveyed WGS84 site origin. Otherwise use a quality-
  filtered representative fix from the static segment.
- Calculate L13 once from that WGS84 position and derive L10 only with
  `Parent(10)`.
- Attach a single L10 tag only to a static segment whose accepted position
  evidence is consistent with that L10 cell. A requested geographic export
  fails closed when accepted fixes span L10 cells; it must not choose a token
  from the filename or silently use the first fix.
- Motion segments may cross geographic cells and do not receive a single-cell
  filename tag. A future moving-capture covering is a set of S2 cells, not a
  fabricated single token.
- GPS remains additive. Without an accepted WGS84 source, `pcap-split` still
  emits its existing sensor-local artefacts, leaves S2 fields absent/NULL, and
  records the reason as `geographic_status: "unavailable"`.

### Metadata surfaces

Each `segments.json` record carries a `geographic_status`. For a located static
segment, the value is `"located"` and the record also carries
`s2_l13_token`, `s2_l10_token`, and `geographic_source`. A static segment
without accepted WGS84 provenance uses `"unavailable"` and omits both tokens;
a moving segment uses `"not_applicable"` because it cannot truthfully claim
one cell.

The text summary repeats the canonical tokens beneath each located static
segment, for example:

```text
[1] static  ...  capture-static-0-s2-l10-808581.pcap
    S2 L13 token: 80858004
    S2 L10 token: 808581
```

It may add the plain family display as a human aid, but never the UI-only scan
cue. `segments.json` is the import authority; the summary is a human-readable
projection of the same record. The PCAP bytes themselves remain standard
packet-capture data and are not rewritten to embed velocity.report metadata.

A VRLOG recorded from that static PCAP copies the exact canonical tokens into
`header.json`; it does not recompute them from the source basename. Database
registration copies the same pair into nullable `s2_l13_token` and
`s2_l10_token` columns on the replay case and run record. These are geographic
attributes, not replacements for replay-case, run, session, or VRLOG identity.

Conformance validation derives `Parent(10)` from the stored L13 CellID and
requires the filename tag, `segments.json`, text summary, VRLOG header, and
database values to agree wherever each surface is present. A disagreement is a
hard provenance error. Legacy and sensor-local artefacts may omit all S2 fields,
but partial or contradictory tagging is invalid.

## Planned implementation sequence

All of this remains future implementation work:

1. Introduce a shared S2 geographic utility layer.
2. Add deterministic WGS84 position → L13 conversion.
3. Derive L10 with `CellID.Parent(10)`.
4. Choose and document the canonical SQLite representation for L13, including
   signed/unsigned and token-string trade-offs.
5. Define the canonical L10 filesystem contract.
6. Add an explicitly L10/L13-aware family-display formatter and a separate UI
   scan-cue renderer. Keep the cue outside text selection, copying,
   accessibility names, and serialisation; keep diagnostics for other levels
   level-aware.
7. Extend `pcap-split` with optional WGS84 ingestion, static-segment L13/L10
   derivation, canonical L10 filename tags, and summary/JSON provenance.
8. Propagate static-source S2 tags into VRLOG directory names and `header.json`.
9. Add nullable L13/L10 columns and indexes to replay cases and run records,
   plus cross-surface conformance validation.
10. Add database indexes and migrations for other S2-partitioned datasets.
11. Migrate planned or existing geographic partition references without
    replacing site, deployment, or session identities.
12. Update APIs, JSON, analytics, and tooling to recognise canonical S2 IDs.
13. Add known-vector, parent/child, level-marker, and boundary tests, including
    parents that do not resemble lexical truncation.
14. Validate filesystem determinism, canonical-token round trips, and PCAP /
    VRLOG / database tag consistency.
15. Validate interoperability against known Waymo and Valgo L13 cells.
