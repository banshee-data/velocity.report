# Geometry-Prior service: architecture specification

- **Status:** Proposed (v2.0 scope)
- **Parent:** [vector-scene-map.md](./vector-scene-map.md)
- **Layers:** L4 Perception (extends Prior Loader interface)

Community-maintained supplemental geometry priors: ground surfaces, kerbs, vegetation; not well represented in OpenStreetMap. Served as static GeoJSON from a public CDN, keyed by coarsened GPS coordinates.

---

## Design goal

Enable a public file tree of supplemental geometry priors that any velocity.report deployment can optionally fetch, while preserving the local-first, privacy-by-default architecture. No cameras, no PII, no location data transmitted without explicit opt-in. The files contain geometry only: no speed, transit, or vehicle data.

---

## Architecture: local-first with optional static fetch

The prior service is purely additive. Without GPS or network access, the system runs LiDAR-only using its own learned background. With GPS and opt-in enabled, the Prior Loader fetches static GeoJSON files for the coarsened grid cell, applies them as soft-constraint weights, and never phones home with precise coordinates.

```
┌──────────────────────────────────────────────────────────┐
│  Prior Loader (L4)                                       │
│                                                          │
│  1. Read local prior files (always)                      │
│  2. If GPS available AND prior_service.enabled:          │
│     a. Coarsen GPS → 0.01° grid cell (e.g. 51.75_-1.26)  │
│     b. Fetch {base_url}/{lat_int}/{lon_int}/{cell}.geojson│
│     c. Validate schema, apply as weighted priors         │
│  3. Merge local + remote priors (local wins on conflict) │
└──────────────────────────────────────────────────────────┘
```

---

## Grid-Based folder structure

The canonical grid uses **2-decimal-place latitude/longitude (0.01°)** (~1.1 km N-S × ~0.7 km E-W at UK latitudes), matching the coarsening applied to GPS coordinates before any network request. This prevents precise deployment location disclosure while providing sufficient locality for scene priors.

| Path Component        | Resolution    | Example       | Max entries per parent                  |
| --------------------- | ------------- | ------------- | --------------------------------------- |
| `{lat_int}/`          | 1° (~111 km)  | `51/`         | 180 (−90 to +89)                        |
| `{lon_int}/`          | 1° (~70 km)   | `-1/`         | 360 (−180 to +179)                      |
| `{lat}_{lon}.geojson` | 0.01° (~1 km) | `51.75_-1.26` | up to 10,000 per `{lat_int}/{lon_int}/` |

**Negative longitudes** use the minus sign in the folder and filename (e.g. `-1/51.75_-1.26.geojson`).

**File count analysis:**

Each `{lat_int}/{lon_int}/` directory holds at most 100 × 100 = **10,000 files** (the 0.01° grid over one 1°×1° block). In practice, populated cells are heavily sparse: a typical UK town produces 50–200 files across 2–4 parent directories.

| Scope                  | Approximate file count                             |
| ---------------------- | -------------------------------------------------- |
| Single intersection    | 1–4 files                                          |
| Residential street     | 5–20 files                                         |
| Town / suburb          | 50–300 files                                       |
| County / large city    | 1,000–5,000 files                                  |
| Full UK coverage       | ~50,000–200,000 files                              |
| Global theoretical max | 648 million cells (unpopulated cells have no file) |

---

## Contribution model

Contributions are submitted as **pull requests** to a public repository (or file uploads to a community-managed bucket). No accounts or authentication required for read access; write access goes through standard PR review.

- Contributors export their sensor's learned scene map as GeoJSON.
- A CI validation job checks schema conformance, coordinate bounds, and file placement in the correct grid folder.
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

**File structure (each grid cell):** `{lat}_{lon}.geojson`

Each file is a GeoJSON FeatureCollection (RFC 7946) with a `metadata` object and a `features` array.

**Top-level metadata:**

| Field               | Type   | Required | Notes                           |
| ------------------- | ------ | -------- | ------------------------------- |
| `schema_version`    | string | Yes      | Currently `"1"`                 |
| `grid_cell`         | string | Yes      | `"{lat}_{lon}"` cell identifier |
| `created_at`        | string | Yes      | ISO 8601 timestamp              |
| `contributor_name`  | string | Yes      | Display name                    |
| `contributor_email` | string | No       | For GPG key lookup              |
| `gpg_fingerprint`   | string | No       | Fingerprint of signing key      |

**Feature properties (per feature):**

| Field        | Type   | Required | Notes                                    |
| ------------ | ------ | -------- | ---------------------------------------- |
| `class`      | string | Yes      | `"ground"`, `"structure"`, or `"volume"` |
| `confidence` | real   | Yes      | 0.0–1.0                                  |
| `updated_at` | string | Yes      | ISO 8601 timestamp                       |

Additional class-specific properties (`plane_normal`, `z_min`, etc.) vary by classification. Each feature's `geometry` is a GeoJSON Polygon.

When a contributor provides a GPG key, the export tool produces a detached signature file submitted alongside the GeoJSON in the same PR:

```
51.75_-1.26.geojson
51.75_-1.26.geojson.sig   # detached ASCII-armoured GPG signature
```

CI verifies the signature against the declared `gpg_fingerprint` at merge time. The result is recorded in the `_trust/` manifest: **the GeoJSON file itself is not touched**.

---

## CI trust manifest

Because prior files are immutable, CI maintains signature status in a separate directory:

```
priors/
  _trust/
    manifest.json   # CI-owned; updated on every merge
  51/
    -1/
      51.75_-1.26.geojson
      51.75_-1.26.geojson.sig
```

**`_trust/manifest.json` structure:**

```jsonc
{
  "generated_at": "2026-02-23T12:00:00Z",
  "files": {
    "51/-1/51.75_-1.26.geojson": {
      "signed": true,
      "gpg_fingerprint": "A1B2C3D4E5F6...",
      "contributor_name": "Alice Smith",
      "verified_at": "2026-02-23T12:00:00Z",
    },
    "51/-1/51.76_-1.26.geojson": {
      "signed": false,
      "contributor_name": "Bob Jones",
      "verified_at": null,
    },
  },
}
```

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

```json
"prior_service": {
  "enabled": true,
  "base_url": "https://priors.velocity.report",
  "require_signed": true
}
```

With `require_signed: true` the Prior Loader fetches `_trust/manifest.json` first, then only loads data files that appear with `signed: true`. With `require_signed: false` all files are loaded, but the Prior Loader logs a warning for each unsigned or manifest-absent file.

**Privacy safeguards:**

1. Location queries use coarsened coordinates (0.01° grid snapping, ~1 km²): no precise deployment location disclosure.
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
4. Weighted polygon union: merge overlapping features, weight by contributor confidence and signature status.
5. Emit synthetic FeatureCollection with `{ "source": "synthetic", "aggregated_at": "...", "contributor_count": N }`.
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

**Q1: Multi-contributor merging for the same grid cell.** Each 0.01° cell is a single file. If two contributors both submit priors for `51.75_-1.26.geojson`, whose data wins? Options range from last-write-wins to weighted polygon union to versioned per-contributor sub-files. Considerations: immutability constraint prevents in-place merge; per-contributor files (e.g. `51.75_-1.26.<fingerprint>.geojson`) preserve immutability but multiply file count; a server-side merge artefact (unsigned, clearly marked synthetic) could live alongside originals.

**Q2: Spam, abuse screening, and Git repo scalability.** Pull requests work at low volume but have two compounding problems at scale: unbounded Git pack history growth, and an open PR target inviting automated junk. CI schema checks cannot assess geometric plausibility. Sub-questions: what constitutes a valid prior? Is GPG signing sufficient as a spam disincentive? How to revoke or deprecate a bad cell file once distributed via CDN? See §Hosting Options for alternative submission mechanisms.

## Resolved design questions

| Decision                     | Resolution                                                            |
| ---------------------------- | --------------------------------------------------------------------- |
| PCAP research corpus hosting | Prefer Hugging Face Datasets; evaluate alternatives before committing |

---

## Implementation phases

| Phase  | Milestone | Scope                                                        |
| ------ | --------- | ------------------------------------------------------------ |
| **5a** | v1.0      | Define GeoJSON schema for local prior files                  |
| **5b** | v1.0      | Implement Prior Loader with file-system backend              |
| **5c** | v1.0      | Wire `w_prior` weights into ground-plane region scoring      |
| **5d** | v2.0      | Add HTTP backend to Prior Loader (static file fetch, opt-in) |
| **5e** | v2.0      | Define canonical grid folder structure and CI validation     |
| **5f** | v2.0      | Create public prior repository with contribution guidelines  |
| **5g** | v2.0      | Add GeoJSON scene-map export command for prior contribution  |
