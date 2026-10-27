# Geometry-Prior service: architecture specification

- **Status:** Proposed (v2.0 scope)
- **Parent:** [vector-scene-map.md](./vector-scene-map.md)
- **Layers:** L4 Perception (extends Prior Loader interface)

Community-maintained supplemental geometry priors: ground surfaces, kerbs, vegetation; not well represented in OpenStreetMap. Served as static GeoJSON from a public CDN, partitioned by canonical S2 cells. Remote prior fetches are explicit opt-in and optional; local LiDAR-only operation remains the default. The S2 representation follows the repository-wide [geographic-indexing decision](geographic-indexing.md).

---

## Purpose and interpretation

Geometry priors are measured spatial data: observed or inferred positions, extents and surfaces of
stationary features (ground, structures, volumes) at a stated acquisition time, with a stated
source, coordinate system, units and uncertainty. They describe what was observable and nothing
more. They are not plans, drawings or specifications, they carry no material, finish, interior or
construction-method information, and a feature that was not observed is not asserted absent.
Intended uses are localisation, spatial indexing, alignment and change detection; they are not a
boundary survey, a structural assessment or an authoritative statement of current site conditions.
The canonical statement, and the publication checklist that §File format specification implements,
is [Geometry priors: data purpose and interpretation](../../geometry-priors-data-purpose.md).

---

## Design goal

Enable a public file tree of supplemental geometry priors that any velocity.report deployment can
optionally fetch, while preserving the local-first, privacy-by-default architecture. No cameras, no
PII, no location data transmitted without explicit opt-in. The files contain measured geometry and
its provenance only: no speed, transit, or vehicle data, and no attributes beyond geometry (see
§Property allow-list).

---

## Architecture: local-first with optional static fetch

The prior service is purely additive. Without GPS or network access, the system runs LiDAR-only using its own learned background. With GPS and opt-in enabled, the Prior Loader calculates the canonical S2 L13 cell, derives its L10 parent with S2 CellID semantics, fetches that partition's static GeoJSON, applies it as soft-constraint weights, and never sends the precise WGS84 measurement.

```
┌──────────────────────────────────────────────────────────┐
│  Prior Loader (L4)                                       │
│                                                          │
│  1. Read local prior files (always)                      │
│  2. If GPS available AND prior_service.enabled:          │
│     a. Convert GPS → canonical S2 L13 CellID/token       │
│     b. Derive L10 with CellID.Parent(10)                 │
│     c. Fetch {base_url}/{l10}/{l13}.geojson              │
│     d. Validate schema, apply as weighted priors         │
│  3. Merge local + remote priors (local wins on conflict) │
└──────────────────────────────────────────────────────────┘
```

---

## S2 folder structure

The data uses S2 L13 as its canonical fine geographic partition and S2 L10 as
the coarse filesystem partition. Both path components are canonical S2 tokens:

```text
priors/
  808581/             # canonical L10 token
    80858004.geojson  # canonical L13 token
```

An L10 cell contains exactly 64 L13 descendants (`4³` for the three S2 levels
between L10 and L13), so a directory has at most 64 aggregate cell files before
any deliberately separate contribution/version objects. Sparse areas simply
omit cells for which no prior exists. The four-child subdivision and
face-local Hilbert ordering are explained in the canonical
[geographic-indexing decision](geographic-indexing.md#how-s2-positions-and-numbers-cells)
and the official [S2 Cells developer
guide](https://s2geometry.io/devguide/s2cell_hierarchy).

The L10 parent must be calculated from the L13 CellID with `Parent(10)`. It must
never be obtained by truncating the L13 token or extracting a lexical prefix.
The optional family displays (`80858-1` and `80858-004`) are for human-facing
text only; they do not appear in paths. UI may add the non-text scan cue inside
the family prefix, but selection and copying must still yield `80858-1` or
`80858-004` with no space character.

---

## Contribution model

Contributions are submitted as **pull requests** to a public repository (or file uploads to a community-managed bucket). No accounts or authentication required for read access; write access goes through standard PR review.

- Contributors export the static geometry their sensor has measured as GeoJSON.
- A CI validation job checks schema conformance (including the property allow-list and the required provenance members), coordinate bounds, and file placement in the correct grid folder.
- Merged files become immediately available on the CDN.
- Contributor identity is a **chosen name** plus an optional email address and GPG key fingerprint (see §File Format Specification). Once merged, **the GeoJSON file is never modified**: CI records signature status separately in the `_trust/` manifest (see §Trust Tiers and Host Routing) so that end users can always verify the original signature against the original file bytes.

---

## Future-Compatibility strategy

Design choices in v1.0 that ensure the online service is additive, not a rewrite:

| Decision (v1.0)                                        | Future Benefit (v2.0+)                                                                        |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------------- |
| GeoJSON file format for local priors                   | Same schema served by HTTP endpoint; no format conversion needed                              |
| Prior weights are advisory (0–1), not hard constraints | Service can return confidence-weighted priors; client applies them identically to local files |
| Prior Loader abstraction separates file I/O from maths | Swap file reader for HTTP client behind the same interface                                    |
| Sensor-local coordinate system (no GPS required)       | GPS is additive: if present, enables location-based prior lookup; if absent, local files work |
| Privacy by default                                     | Online fetch is opt-in; no location data transmitted without explicit user consent            |
| `_trust/` manifest separate from data files            | CI updates trust status without touching contributor files; signatures remain verifiable      |

---

## File format specification (v2.0 scope)

All prior files are **GeoJSON FeatureCollections** (RFC 7946). Prior files are **immutable once merged**: CI never modifies the contributor-uploaded content, ensuring that detached GPG signatures remain independently verifiable.

**File structure (each fine partition):** `{s2-l13-token}.geojson`, beneath its
canonical `{s2-l10-token}/` parent directory.

Each file is a GeoJSON FeatureCollection (RFC 7946) with a `metadata` object and a `features` array.

**Top-level metadata:**

| Field                | Type   | Required                               | Notes                                                                                                                                                                                                                                                                                                                       |
| -------------------- | ------ | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `schema_version`     | string | Yes                                    | Currently `"1"`                                                                                                                                                                                                                                                                                                             |
| `s2_level`           | int    | Yes                                    | Canonical fine level; `13`                                                                                                                                                                                                                                                                                                  |
| `s2_l13_token`       | string | Yes                                    | Canonical S2 L13 token                                                                                                                                                                                                                                                                                                      |
| `s2_l10_token`       | string | Yes                                    | Canonical parent token, derived with `Parent(10)`                                                                                                                                                                                                                                                                           |
| `created_at`         | string | Yes                                    | ISO 8601 timestamp of file creation, not of observation                                                                                                                                                                                                                                                                     |
| `contributor_name`   | string | Yes                                    | Display name                                                                                                                                                                                                                                                                                                                |
| `contributor_email`  | string | No                                     | For GPG key lookup                                                                                                                                                                                                                                                                                                          |
| `gpg_fingerprint`    | string | No                                     | Fingerprint of signing key                                                                                                                                                                                                                                                                                                  |
| `crs`                | string | Yes                                    | Coordinate reference system of every coordinate: `"EPSG:4326"` (WGS84, longitude then latitude, RFC 7946 axis order), or the named sensor-local frame of a local-only file                                                                                                                                                  |
| `units`              | string | Yes                                    | Unit of every length-valued property: `"metres"`                                                                                                                                                                                                                                                                            |
| `vertical_reference` | string | Yes                                    | What heights and `z_*` values are measured from: `"sensor_origin"`, `"local_ground"` or a named vertical datum                                                                                                                                                                                                              |
| `dataset_version`    | string | Yes                                    | Version of the dataset this file belongs to; changes when a feature is added, removed or re-measured                                                                                                                                                                                                                        |
| `processing_version` | string | Yes                                    | Version of the export tool and settings that produced the geometry                                                                                                                                                                                                                                                          |
| `sources`            | array  | Yes                                    | One entry per input dataset: `id`, `kind` (`sensor` or `source_data`), `name`, `terms` (licence identifier or statement of the redistribution terms) and `attribution` where the terms require it                                                                                                                           |
| `acquired_interval`  | object | No                                     | `start` and `end` ISO 8601 timestamps bounding every observation in the file, where known; omitted rather than estimated                                                                                                                                                                                                    |
| `coverage_notes`     | array  | No                                     | Known gaps (occlusion, range limit, unobserved sides) as short strings; a gap is not a statement that nothing is there                                                                                                                                                                                                      |
| `uncertainty`        | object | When any feature states an uncertainty | The estimator and the assumptions behind every half-width in the file: model, range, azimuth and elevation accuracy with their sources, cell spread factor, residual quantile, registration (§5 of the [positional-uncertainty estimator](../../../data/maths/proposals/20261027-geometry-prior-positional-uncertainty.md)) |

**Feature properties (per feature):**

| Field                    | Type   | Required                      | Notes                                                                                                                                                                                                                                                                    |
| ------------------------ | ------ | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `class`                  | string | Yes                           | `"ground"`, `"structure"`, or `"volume"`                                                                                                                                                                                                                                 |
| `confidence`             | real   | Yes                           | 0.0–1.0                                                                                                                                                                                                                                                                  |
| `updated_at`             | string | Yes                           | ISO 8601 timestamp of the last change to the feature record; observation time is `acquired_at` or `acquired_interval`                                                                                                                                                    |
| `evidence_status`        | string | Yes                           | `"observed"`, `"inferred"`, `"source_data"` or `"prior_only"` (see §Evidence status)                                                                                                                                                                                     |
| `source_id`              | string | Yes                           | `id` of the `sources` entry the feature comes from                                                                                                                                                                                                                       |
| `acquired_at`            | string | Either                        | ISO 8601 instant at which the geometry was observed or, for `source_data`, the date of the source snapshot                                                                                                                                                               |
| `acquired_interval`      | object | Either                        | `start` and `end` ISO 8601 timestamps when the observation spanned a period; give this or `acquired_at`                                                                                                                                                                  |
| `position_uncertainty_m` | real   | For `observed` and `inferred` | Conservative horizontal half-width in metres from the [positional-uncertainty estimator](../../../data/maths/proposals/20261027-geometry-prior-positional-uncertainty.md), under the assumptions in `uncertainty`; omitted for `source_data` when the source states none |
| `vertical_uncertainty_m` | real   | No                            | The same for heights and `z_*` values; omitted when a height is censored (`z_min_censored`, `z_max_censored`)                                                                                                                                                            |
| `coverage`               | real   | No                            | 0.0–1.0 fraction of the feature's extent directly supported by returns                                                                                                                                                                                                   |
| `point_count`            | int    | No                            | Source point observations                                                                                                                                                                                                                                                |

Class-specific geometric properties (`plane_normal`, `z_min`, etc.) are limited to the allow-list
below. Each feature's `geometry` is a GeoJSON Polygon.

An omitted optional member means "not recorded", never "zero" or "none". The format has no place
for a value an exporter could only guess. The
[positional-uncertainty estimator](../../../data/maths/proposals/20261027-geometry-prior-positional-uncertainty.md)
says how the half-widths are bounded; an
exporter that cannot supply one of its inputs omits the half-width, and the feature then fails the
requirement rather than carrying a placeholder.

### Evidence status

`evidence_status` says how the geometry is known, in the vocabulary of
[internal/lidar/annotation/physical.go](../../../internal/lidar/annotation/physical.go). That file
defines `observed`, `inferred`, `prior_only` and `unknown`; this format adds `source_data` for
third-party inputs and never exports `unknown` (a component with no known value is omitted).

| Status        | Meaning                                                                                               |
| ------------- | ----------------------------------------------------------------------------------------------------- |
| `observed`    | The sensor saw this surface or extent directly, at the stated time, in a named frame                  |
| `inferred`    | Follows from observed geometry by a stated step (a fitted plane or bounding rectangle across gaps)    |
| `source_data` | Taken from a third-party dataset with that source's own accuracy and date; not checked by the sensor  |
| `prior_only`  | A class-level default with no measurement for this feature; evidence about the class, not the feature |

Statuses rank `observed`, `inferred`, `source_data`, `prior_only`, strongest first, and a feature
carries the weakest status of its parts: an observed footprint with a height taken from
`source_data` is `source_data`. A partial view stays a lower bound; a plausible outline drawn over
it does not make the unseen sides observed.

### Property allow-list

A prior file carries geometry and the context needed to interpret it, nothing else. The allow-list
is exactly the metadata members tabled above plus the feature properties tabled above and below.
Material, finish, colour, interior layout, floor count and construction-method attributes are not
part of the format, whatever the source dataset records. CI rejects a file with any metadata member
or feature property outside the list, and a `subclass` outside the closed vocabulary of
[vector-scene-map.md](vector-scene-map.md) §2. It rejects rather than strips, because a merged file
is never modified. The volume property names are proposed here and fixed when the volume export is
implemented.

| Group                       | Properties                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------- |
| Identity and classification | `id`, `class`, `subclass`, `lod`, `parent_id`                                               |
| Evidence and provenance     | `evidence_status`, `source_id`, `acquired_at`, `acquired_interval`, `updated_at`            |
| Uncertainty and quality     | `confidence`, `position_uncertainty_m`, `vertical_uncertainty_m`, `coverage`, `point_count` |
| Ground geometry             | `plane_normal`, `plane_offset`, `planarity`, `curvature_deg`, `area_m2`                     |
| Structure geometry          | `z_min`, `z_max`, `z_min_censored`, `z_max_censored`, `wall_count`                          |
| Volume geometry             | `bounding_type`, `dimensions_m`, `orientation_deg`, `point_density`                         |

`plane_normal` and `plane_offset` describe the plane $\mathbf{n}\cdot\mathbf{p} + d = 0$, with
$d$ = `plane_offset`, in a frame whose origin is the feature's centroid at the
`vertical_reference` height: local east-north-up in a georeferenced file, the named sensor frame in
a local one, in `units`. The surface's height at the centroid is then $-d/n_z$. A `z_max` (or
`z_min`) is censored when the sensor's highest (or lowest) ring still hit the surface, so the true
edge lies beyond the field of view: the value is the highest (or lowest) observed return, a lower
(or upper) bound, and `z_max_censored` (or `z_min_censored`) is `true`.

**Ground-plane exports as priors.** The ground-plane tile exports
([ground-plane-extraction.md](ground-plane-extraction.md) §7 and the
[PCAP export tool](../../plans/pcap-ground-plane-export-tool-plan.md)) name some members for their
own use. A tile contributed as a prior is written with the allow-list's names:

| Ground-plane export                                         | Prior member                                                  |
| ----------------------------------------------------------- | ------------------------------------------------------------- |
| `plane_normal`, or `plane_a`, `plane_b`, `plane_c`          | `plane_normal`, re-expressed in the centroid frame            |
| `plane_offset` or `plane_d` (in the tool's own frame)       | `plane_offset`, recomputed for the centroid frame             |
| `z_offset` or `mean_height` (height at the tile centre)     | `plane_offset` $= -n_z \cdot$ height, with the centroid frame |
| `planarity`, `curvature_deg`, `point_count`, `confidence`   | The same names                                                |
| `height_std_dev`                                            | An input to $\rho_{95}$ in the estimator, not a member        |
| `settled`                                                   | Not written: only settled tiles are contributed               |
| `tile_x`, `tile_y`, `curvature_class`, `settlement_time_ms` | Not written: local grid indices and processing state          |
| `coordinate_system` (`"Sensor-XY"` or `"WGS84"`)            | `crs` (the named sensor frame, or `"EPSG:4326"`)              |
| `units`, `vertical_reference`, `capture_start_time`         | `units`, `vertical_reference`, `acquired_interval.start`      |

When a contributor provides a GPG key, the export tool produces a detached signature file submitted alongside the GeoJSON in the same PR:

```
80858004.geojson
80858004.geojson.sig   # detached ASCII-armoured GPG signature
```

CI verifies the signature against the declared `gpg_fingerprint` at merge time. The result is recorded in the `_trust/` manifest: **the GeoJSON file itself is not touched**.

---

## CI trust manifest

Because prior files are immutable, CI maintains signature status in a separate directory:

```
priors/
  _trust/
    manifest.json   # CI-owned; updated on every merge
  808581/
    80858004.geojson
    80858004.geojson.sig
```

**`_trust/manifest.json` structure:**

The manifest is a JSON object with a `generated_at` ISO 8601 timestamp and a `files` map. Each key is a relative file path; each value contains:

| Field              | Type    | Description                                           |
| ------------------ | ------- | ----------------------------------------------------- |
| `signed`           | bool    | Whether a valid GPG signature exists                  |
| `gpg_fingerprint`  | string  | Signing key fingerprint (if signed)                   |
| `contributor_name` | string  | Display name of the contributor                       |
| `verified_at`      | string? | ISO 8601 timestamp of verification (null if unsigned) |

The manifest is the **only** place `signed` status is recorded. Clients fetch `_trust/manifest.json` once per session (or cache it) and consult it when deciding whether to trust a prior file. The data files carry no trust annotation: their content is exactly what the contributor submitted.

---

## Trust tiers and host routing

Host operators can mirror or gate the public repository to expose only the files they trust. Because the manifest is separate from the data files, a host serves a filtered view simply by controlling which files it copies:

| Trust tier    | Manifest `signed` | How to host                                          | Example base URL                            |
| ------------- | ----------------- | ---------------------------------------------------- | ------------------------------------------- |
| **Verified**  | `true` only       | Copy only files listed as `signed: true` in manifest | `https://priors.velocity.report/`           |
| **Community** | `false` included  | Copy all files regardless of manifest status         | `https://priors-community.velocity.report/` |
| **Local**     | either            | Full local copy from the repo                        | `file:///var/lib/velocity-report/priors/`   |

Configuration:

| Setting                        | Type   | Default                            | Description                                       |
| ------------------------------ | ------ | ---------------------------------- | ------------------------------------------------- |
| `prior_service.enabled`        | bool   | `true`                             | Enable remote prior fetching                      |
| `prior_service.base_url`       | string | `"https://priors.velocity.report"` | CDN base URL for prior files                      |
| `prior_service.require_signed` | bool   | `true`                             | Only load files marked `signed: true` in manifest |

With `require_signed: true` the Prior Loader fetches `_trust/manifest.json` first, then only loads data files that appear with `signed: true`. With `require_signed: false` all files are loaded, but the Prior Loader logs a warning for each unsigned or manifest-absent file.

**Privacy safeguards:**

1. Location queries use a canonical S2 L13 partition rather than transmitting the precise WGS84 measurement. The requested partition still reveals an approximate area and must remain explicit opt-in; S2 is an index, not anonymisation.
2. No authentication required for read access (public static files).
3. Contributor identity is a **freely chosen name**: no accounts, no verification of real-world identity. Email and GPG key are entirely optional. Signatures authenticate the _key_, not the person; status is recorded only in `_trust/manifest.json`.
4. All prior data is geometry only: no speed, transit, or vehicle data.

---

## Server-Generated union artefact

Individual contribution files are immutable and per-contributor. The practical served file for most clients is a **server-generated union**: a daily aggregate produced by a scheduled job.

**Pipeline (at most once per 24 h per changed cell):**

1. Collect all contributions for each cell.
2. Spatial deduplication: remove duplicate polygons within tolerance.
3. Spam/sanity rejection: coordinate bounds check, minimum polygon area, schema validation, implausibility heuristics.
4. Weighted polygon union: merge overlapping features, weight by contributor confidence and signature status. A merged feature keeps the weakest `evidence_status` among its parts and the widest `position_uncertainty_m`; its acquisition interval spans the merged observations.
5. Emit synthetic FeatureCollection with `{ "source": "synthetic", "aggregated_at": "...", "contributor_count": N }`. The aggregate's `sources` lists every contributing input with its recorded terms, and a feature whose source terms do not cover redistribution is left out; `dataset_version` names the aggregation run.
6. Sign aggregate with project GPG key → publish to served CDN path.

The aggregate file is clearly labelled `source: synthetic` and signed with the **project key** (not a contributor key). Clients that set `require_signed: true` load it because it carries a known-good signature. Individual contributor files remain in the contribution store for transparency and re-aggregation.

---

## Hosting options

| Platform                             | Cost                     | Max size                | Notes                                                                                                          |
| ------------------------------------ | ------------------------ | ----------------------- | -------------------------------------------------------------------------------------------------------------- |
| **Cloudflare R2 + Worker**           | Free to ~10 GB / 1 M req | Object store (no Git)   | ~50-line Worker validates schema, rate-limits by IP, stores to R2, triggers daily aggregation. No egress fees. |
| **Hugging Face Datasets**            | Free (public)            | Git + LFS               | Designed for research data; supports GeoJSON natively; Spaces for submission form.                             |
| **Internet Archive**                 | Free, unlimited          | Immutable items, S3 API | Good for archival snapshots; not ideal for live incremental updates.                                           |
| **GitHub Releases (aggregate only)** | Free                     | Binary assets per tag   | Contribution store elsewhere; daily aggregate published as release asset. Clients pin to a release URL.        |
| **GitHub Pages**                     | Free                     | Static site             | CI verifies signatures and updates `_trust/manifest.json` on each merge. Zero ops cost.                        |
| **Any static CDN**                   | Varies                   | Unlimited               | Cloudflare Pages, S3 + CloudFront, self-hosted by municipalities: any HTTP server.                             |

---

## PCAP research corpus (future)

LiDAR PCAP files are large (100 MB–10 GB per capture) and not suitable for Git. When a public research corpus is warranted:

| Platform                  | Cost          | Notes                                                                                     |
| ------------------------- | ------------- | ----------------------------------------------------------------------------------------- |
| **Zenodo**                | Free          | CERN/OpenAIRE backed; DOI per version; CC licensing. Preferred for a citable PCAP corpus. |
| **Academic Torrents**     | Free          | BitTorrent-based; good for static versioned releases.                                     |
| **Internet Archive**      | Free          | Permanent, high-bandwidth; S3-compatible upload API.                                      |
| **Hugging Face Datasets** | Free (public) | LFS quotas apply; good discoverability in ML community.                                   |

No dedicated LiDAR PCAP repository exists for low-speed urban traffic data. Hugging Face Datasets is the current preferred option for discoverability in the ML community. Evaluate alternatives (Zenodo for citable DOIs, Academic Torrents for static releases, Internet Archive for permanence) before committing to a platform.

---

## Open questions

These questions should be addressed before the v2.0 contribution pipeline is built.

**Q1: Multi-contributor merging for the same geographic partition.** Each S2 L13 cell has a single aggregate file. If two contributors both submit priors for `80858004.geojson`, whose data wins? Options range from last-write-wins to weighted polygon union to versioned per-contributor sub-files. Considerations: immutability prevents in-place merge; per-contributor files (for example, `80858004.<fingerprint>.geojson`) preserve immutability but multiply file count; a server-side merge artefact (unsigned, clearly marked synthetic) could live alongside originals.

**Q2: Spam, abuse screening, and Git repo scalability.** Pull requests work at low volume but have two compounding problems at scale: unbounded Git pack history growth, and an open PR target inviting automated junk. CI schema checks cannot assess geometric plausibility. Sub-questions: what constitutes a valid prior? Is GPG signing sufficient as a spam disincentive? How to revoke or deprecate a bad cell file once distributed via CDN? See §Hosting Options for alternative submission mechanisms.

## Resolved design questions

| Decision                     | Resolution                                                                                                                                                                         |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| PCAP research corpus hosting | Prefer Hugging Face Datasets; evaluate alternatives before committing                                                                                                              |
| Positional uncertainty (Q3)  | Linear worst-case half-widths with their assumptions recorded: [positional-uncertainty estimator](../../../data/maths/proposals/20261027-geometry-prior-positional-uncertainty.md) |

---

## Implementation phases

| Phase  | Milestone | Scope                                                        |
| ------ | --------- | ------------------------------------------------------------ |
| **5a** | v1.0      | Define GeoJSON schema (provenance, evidence, allow-list)     |
| **5b** | v1.0      | Implement Prior Loader with file-system backend              |
| **5c** | v1.0      | Wire `w_prior` weights into ground-plane region scoring      |
| **5d** | v2.0      | Implement shared WGS84 → L13 and `Parent(10)` S2 utilities   |
| **5e** | v2.0      | Define canonical L10/L13 layout; CI schema and allow-list    |
| **5f** | v2.0      | Create public prior repository with contribution guidelines  |
| **5g** | v2.0      | Add HTTP backend to Prior Loader (static file fetch, opt-in) |
| **5h** | v2.0      | Add GeoJSON scene-map export command for prior contribution  |
| **5i** | v2.0      | Add known-vector, hierarchy, and non-lexical-parent tests    |
