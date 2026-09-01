---
title: S2 addressing profile
description: "How velocity.report writes S2 cell identifiers: canonical tokens, the L10 and L13 display forms, parent and child semantics, and test vectors for checking an implementation."
date: 2026-09-01T12:00:00Z
version: "1.0"
og_image: /img/s2-10-sf.jpg
---

The exact conventions velocity.report uses when it writes down an [S2 cell](https://s2geometry.io/) identifier. This page is a contract: the reasoning behind it is in the field note [Addresses you can calculate](/notes/s2-spatial-indexing/), and changes here are versioned. <!-- link-ignore -->

## Terminology

| Term                | Meaning                                                                                                      |
| ------------------- | ------------------------------------------------------------------------------------------------------------ |
| **CellID**          | The 64-bit integer S2 assigns to a cell. The authoritative identity. Everything else is a rendering of it.   |
| **Canonical token** | The hexadecimal string form defined by S2, with trailing zeros removed. `808581`. Lower case, no separators. |
| **Display token**   | This project's readable form, with a hyphen after the fifth character. `80858-1`. Presentation only.         |
| **Level**           | Depth in the hierarchy, 0 (a cube face) to 30 (a few centimetres). Each level quarters the one above it.     |
| **Parent**          | The cell one or more levels above that contains a given cell.                                                |
| **Descendant**      | Any cell contained by a given cell at a deeper level.                                                        |

## Canonical tokens

The canonical token is the form produced by S2's own `ToToken` and accepted by `FromToken`, in every port of the library. It is lower-case hexadecimal with trailing zeros stripped, and it is the only form that may be passed to an S2 library, stored in a database column, or used as a dictionary key.

Because the token length is fixed by the level, the two levels this project uses have fixed widths:

| Level | Canonical width | Example    | Cells per L10 parent |
| ----- | --------------- | ---------- | -------------------- |
| L10   | 6 characters    | `808581`   | n/a                  |
| L13   | 8 characters    | `80858064` | 64                   |

Those widths hold everywhere on Earth, not just in the examples below: at a given level the lowest set bit of a CellID is at a fixed position, so the number of significant hexadecimal digits cannot vary.

## Display tokens

Long hexadecimal strings are hard to compare by eye, and cells in the same area share a leading run of characters. The display form makes that run visible by inserting a hyphen after the fifth character:

| Level | Display shape | Example     |
| ----- | ------------- | ----------- |
| L10   | 5+1           | `80858-1`   |
| L13   | 5+3           | `80858-064` |

The hyphen has no meaning in S2. It is typography. Strip it before the value reaches a library, a query, a filename, or a database column, and add it only when rendering for a human. The generator in [tools/s2-hilbert](https://github.com/banshee-data/velocity.report/tree/main/tools/s2-hilbert) enforces this: its core API rejects a hyphenated token outright, and the command-line layer converts at the boundary before calling in.

## Roles of the two levels

| Level | Role                   | Typical size                   | Used for                                                                     |
| ----- | ---------------------- | ------------------------------ | ---------------------------------------------------------------------------- |
| L10   | Coarse partition       | 50 to 100 km², 7 to 10 km wide | Top-level grouping of a body of data: a town, or a district of a larger city |
| L13   | Fine and interoperable | About 1 km², roughly 1 km wide | The unit quoted externally, and the grouping key for a single deployment     |

Levels other than these two may appear in working material and in the diagrams: L12, for example, is drawn in the infographic because sixteen steps read more clearly than sixty-four. Only L10 and L13 are part of this profile.

## Parent and child semantics

Containment is decided by CellID arithmetic, never by comparing strings. Every S2 port provides both operations:

- **Parent:** ask the library for the ancestor of a CellID at a given level.
- **Contains:** ask the library whether one CellID contains another.

A cell at L13 has exactly one L10 ancestor. An L10 cell has exactly 64 L13 descendants, which occupy a contiguous range of CellIDs but not a contiguous range of alphabetically sorted tokens.

### Do not infer hierarchy by truncating a token

This is the single most likely way to get a wrong answer, and it fails quietly.

`80858-1` is an L10 cell. Of its 64 L13 descendants, exactly 32 have canonical tokens beginning `808581`. The other 32 do not. `80858004` is one of them: it is genuinely inside `808581`, and a prefix test says it is not.

The reverse holds too. `48760-5` is the L10 cell over Trafalgar Square. The L13 cell for the same point is `487604cc`, whose sixth character is `4` rather than `5`. Truncating to six characters gives `487604`, which is a different cell.

Neither case is unusual or a boundary condition. Half of every L10 cell's descendants behave this way, because the level bits that mark where a token ends move as the level changes. Use `contains` or `parent`. There is no string operation that substitutes for them.

## Test vectors

Computed with [s2js](https://www.npmjs.com/package/s2js) 1.44.0, the Apache-2.0 port pinned by this repository. Coordinates are degrees, WGS84, latitude then longitude.

| Location                        | Latitude | Longitude | L10 canonical | L10 display | L13 canonical | L13 display |
| ------------------------------- | -------- | --------- | ------------- | ----------- | ------------- | ----------- |
| Ferry Building, San Francisco   | 37.7955  | -122.3937 | `808581`      | `80858-1`   | `80858064`    | `80858-064` |
| Clarendon Avenue, San Francisco | 37.7554  | -122.4525 | `808f7d`      | `808f7-d`   | `808f7dfc`    | `808f7-dfc` |
| Trafalgar Square, London        | 51.5080  | -0.1281   | `487605`      | `48760-5`   | `487604cc`    | `48760-4cc` |
| Sydney Opera House              | -33.8568 | 151.2153  | `6b12af`      | `6b12a-f`   | `6b12ae64`    | `6b12a-e64` |

The same cells as integers. S2 defines the CellID as unsigned; tools with only signed 64-bit integers, BigQuery among them, show the two's-complement value instead. Both columns name the same cell.

| Canonical token | Unsigned CellID     | Signed 64-bit form   |
| --------------- | ------------------- | -------------------- |
| `808581`        | 9260950045757276160 | -9185794027952275456 |
| `80858064`      | 9260949375742377984 | -9185794697967173632 |
| `808f7d`        | 9263760397477871616 | -9182983676231680000 |
| `808f7dfc`      | 9263761479809630208 | -9182982593899921408 |
| `487605`        | 5221366315540807680 | 5221366315540807680  |
| `487604cc`      | 5221366092202508288 | 5221366092202508288  |
| `6b12af`        | 7715421526173941760 | 7715421526173941760  |
| `6b12ae64`      | 7715420856159043584 | 7715420856159043584  |

## Verifying an implementation

An implementation agrees with this profile if, for every row above, it produces the listed canonical token from the coordinate and level, and reports the L10 cell as the parent of the L13 cell.

### With BigQuery

BigQuery's [S2 functions](https://cloud.google.com/bigquery/docs/reference/standard-sql/geography_functions) take a point and a level and return the CellID as a signed integer. Note the argument order: `ST_GEOGPOINT` takes longitude first.

```sql
SELECT
  S2_CELLIDFROMPOINT(ST_GEOGPOINT(-122.3937, 37.7955), level => 10) AS l10,
  S2_CELLIDFROMPOINT(ST_GEOGPOINT(-122.3937, 37.7955), level => 13) AS l13;
```

The result should match the signed column above: `-9185794027952275456` and `-9185794697967173632`.

### With this repository

The generator resolves descendants through the library rather than through string handling, so its output is a direct check of the hierarchy rules on this page.

```bash
pnpm install
npm run s2-hilbert -- --parent 808581 --target-level 13 --json /tmp/80858-1-l13.json
```

The JSON lists all 64 descendants in Hilbert order, with canonical and display tokens side by side.

## Compatibility

Profile version 1.0.

The canonical token rules are S2's, not ours, and will not change. The display convention, the choice of L10 and L13, and the roles assigned to them are this project's, and a change to any of them takes a new version number on this page.
