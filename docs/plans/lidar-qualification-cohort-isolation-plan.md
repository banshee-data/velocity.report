# Site/capture qualification cohort isolation

- **Status:** Proposed
- **Target:** v0.5.4
- **Priority:** P0
- **Workstream:** S-PLATFORM (Ops / Pipeline)
- **Estimate:** M, approximately 4–7 engineering days, subject to loader audit
- **Backlog:** [v0.5.4](../BACKLOG.md#v054---post-encroachment-time--behaviour-metrics-054)

## Purpose

Protect independent physical-measurement qualification from development leakage across San Francisco's four S2 L10 regions. One populated region supplies existing baseline evidence; the next three are provisionally assigned development, validation and blind qualification roles. Assign specific L10 IDs after site diversity review. S2 cells stratify geography; **site and capture session** are the minimum exclusion units.

Existing contracts are not replaced: `internal/lidar/annotation/split.go` freezes object-disjoint tuning/held-out partitions within annotation packs; `internal/lidar/jobs/manifest.go` records capture provenance; `internal/lidar/dataset/dataset.go` manages published survey manifests. This plan adds a site/capture *eligibility* layer above those contracts.

## Contract

Add a versioned machine-readable manifest with canonical site ID, S2 L10/L13, capture ID, raw SHA-256, cohort role (`development`, `validation`, `qualification`), assignment revision, rationale, protocol digest and freeze timestamp. Group all sessions from the same site together. Validate that every capture has exactly one site and role, raw hashes match, and assignments cannot silently drift. Keep a digest of the frozen manifest in each experiment report.

**Enforcement boundaries:** training, tuning, development evaluation and developer-facing diagnostic ingestion must use one eligibility guard before loading samples. Unknown captures and conflicts fail closed. Authorized isolated qualification scoring may read qualification captures; record the authorization, exact code revision, manifest digest and input hashes. Qualification annotations, expected answers and per-example residuals need storage-level access restrictions; a manifest alone is not an access-control mechanism.

**Not gated:** ordinary LiDAR capture, production processing and eventual public survey publication. Existing published speed estimates remain available and are not reference labels. Delay release of detailed qualification feedback until the frozen run has been scored and signed off.

## Delivery steps

1. Audit all tuning/evaluation loader and dashboard ingestion paths; identify bypasses.
2. Define manifest schema, versioning, parser and site/capture consistency validator; add synthetic fixtures.
3. Integrate shared guard at all identified development data-loading boundaries; preserve object-disjoint annotation splits.
4. Isolate qualification references and diagnostics using permissions, with auditable evaluation execution.
5. Add CI tests and provenance reports; pilot with existing captures and freeze new-region assignments before collection.

## Acceptance

- A qualification capture submitted to a tuning job is rejected **before samples load**.
- Unknown, conflicting, or hash-mismatched capture assignments are rejected.
- A capture from the same site cannot be silently split across development and qualification.
- Authorized isolated qualification scoring succeeds and records the policy and input digests.
- Ordinary production processing and public publishing remain unaffected.
- CI exercises positive and negative paths, including direct-loader bypass attempts.

## Open decisions

Choose the exact three L10 assignments and representative sites; determine minimum independent cases and reference instruments; name qualification custodians; preregister numerical accuracy thresholds after the pilot. Once qualification results guide model changes, that cohort is no longer blind for the next claim.
