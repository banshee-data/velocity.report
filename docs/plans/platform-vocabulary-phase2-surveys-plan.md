# P2-F: sites, deployments, surveys and publication

Join the platform's different place identities and make reports and public publication refer to
explicit surveys and deployments.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 14, 15 and 16)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. Sites/deployments precede surveys; reports/publication follow
surveys. Capture exports also require P2-A digests.

## Delivery outline

1. Merge site identities using the agreed provenance/precedence policy; set aside coordinate conflicts rather than averaging them.
2. Introduce deployment rows and explicit mounting/calibration relationships for both sensors.
3. Create surveys and report relationships with complete historical row-accounting and operator workflows.
4. Move public publication onto surveys with old-page redirects, explicit export tiers and tests excluding private contact/trajectory fields.

## PR structure

Separate site/deployment migration, survey/report migration, operator workflow and public
export/redirect PRs with device timing where required.

## Acceptance

- [ ] Every existing report maps to exactly one survey under a documented policy; radar and LiDAR resolve the same site identity.
- [ ] Mounting/calibration values retain their meaning and conflicts have an inspectable disposition.
- [ ] Existing public URLs redirect; exports exclude surveyor/contact/trajectory data and retain verifiable capture provenance.
- [ ] Deleting a site has a declared and tested effect on its deployments. Today `site_config_periods.site_id` is `ON DELETE CASCADE`, so deleting a site deletes its deployment history; the merged model keeps that only by decision.

## Boundaries and risks

No silent site merge or reassignment of ambiguous reports. A terminology change does not create a
deployment or survey relationship.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
