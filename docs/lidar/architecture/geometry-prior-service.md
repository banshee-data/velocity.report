# Geometry-Prior service: architecture specification

- **Status:** Proposed (v2.0 scope)
- **Parent:** [vector-scene-map.md](./vector-scene-map.md)
- **Layers:** L4 Perception (extends Prior Loader interface)

Community-maintained supplemental geometry priors: ground surfaces, kerbs, vegetation; not well represented in OpenStreetMap. Served as static GeoJSON from a public CDN, partitioned by canonical S2 cells. Remote prior fetches are explicit opt-in and optional; local LiDAR-only operation remains the default. The S2 representation follows the repository-wide [geographic-indexing decision](geographic-indexing.md).

---

## Design goal

Enable a public file tree of supplemental geometry priors that any velocity.report deployment can optionally fetch, while preserving the local-first, privacy-by-default architecture. No cameras, no PII, no location data transmitted without explicit opt-in. The files contain geometry only: no speed, transit, or vehicle data.

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
omit cells for which no prior exists.

The L10 parent must be calculated from the L13 CellID with `Parent(10)`. It must
never be obtained by truncating the L13 token or extracting a lexical prefix.
The optional family displays (`80858-1` and `80858-004`) are for human-facing
text only; they do not appear in paths. UI may add the non-text scan cue inside
the family prefix, but selection and copying must still yield `80858-1` or
`80858-004` with no space character.

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

**File structure (each fine partition):** `{s2-l13-token}.geojson`, beneath its
canonical `{s2-l10-token}/` parent directory.

Each file is a GeoJSON FeatureCollection (RFC 7946) with a `metadata` object and a `features` array.

**Top-level metadata:**

| Field               | Type   | Required | Notes                                             |
| ------------------- | ------ | -------- | ------------------------------------------------- |
| `schema_version`    | string | Yes      | Currently `"1"`                                   |
| `s2_level`          | int    | Yes      | Canonical fine level; `13`                        |
| `s2_l13_token`      | string | Yes      | Canonical S2 L13 token                            |
| `s2_l10_token`      | string | Yes      | Canonical parent token, derived with `Parent(10)` |
| `created_at`        | string | Yes      | ISO 8601 timestamp                                |
| `contributor_name`  | string | Yes      | Display name                                      |
| `contributor_email` | string | No       | For GPG key lookup                                |
| `gpg_fingerprint`   | string | No       | Fingerprint of signing key                        |

**Feature properties (per feature):**

| Field        | Type   | Required | Notes                                    |
| ------------ | ------ | -------- | ---------------------------------------- |
| `class`      | string | Yes      | `"ground"`, `"structure"`, or `"volume"` |
| `confidence` | real   | Yes      | 0.0–1.0                                  |
| `updated_at` | string | Yes      | ISO 8601 timestamp                       |

Additional class-specific properties (`plane_normal`, `z_min`, etc.) vary by classification. Each feature's `geometry` is a GeoJSON Polygon.

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

**Q1: Multi-contributor merging for the same geographic partition.** Each S2 L13 cell has a single aggregate file. If two contributors both submit priors for `80858004.geojson`, whose data wins? Options range from last-write-wins to weighted polygon union to versioned per-contributor sub-files. Considerations: immutability prevents in-place merge; per-contributor files (for example, `80858004.<fingerprint>.geojson`) preserve immutability but multiply file count; a server-side merge artefact (unsigned, clearly marked synthetic) could live alongside originals.

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
| **5d** | v2.0      | Implement shared WGS84 → L13 and `Parent(10)` S2 utilities   |
| **5e** | v2.0      | Define canonical L10/L13 folder structure and CI validation  |
| **5f** | v2.0      | Create public prior repository with contribution guidelines  |
| **5g** | v2.0      | Add HTTP backend to Prior Loader (static file fetch, opt-in) |
| **5h** | v2.0      | Add GeoJSON scene-map export command for prior contribution  |
| **5i** | v2.0      | Add known-vector, hierarchy, and non-lexical-parent tests    |
