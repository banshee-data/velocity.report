---
name: lidar-market-review
description: Fortnightly LiDAR market review. Re-checks tracked spinning LiDAR models and observed new and used offers, replaces the current assessment, and records changes in a bounded annual archive.
argument-hint: "[YYYY-MM-DD]"
---

# Skill: lidar-market-review

Keep the [LiDAR market watch](../../../docs/platform/hardware/lidar-market-watch.md) current:
separate model facts from offers, replace the latest assessment, and preserve dated evidence in
`docs/platform/hardware/lidar-market-reviews/YYYY.md`. Research and documentation only; never change
production code.

## Usage

```bash
/lidar-market-review            # review dated today (UTC)
/lidar-market-review 2026-10-19 # review with an explicit date
```

The page is the source of truth for selection thresholds, freshness, and entry format. This skill
holds the procedure. A caller, such as the scheduled routine, owns branch, commit, push, and
notification. Changing this procedure does not change the routine's fortnightly schedule.

## Procedure

### 1. Read the current assessment and relevant history

Read the page's **Selection rule**, **Latest**, and **Entry format**, then follow its latest
research link. Read earlier observations when a carried fact, offer, or correction depends on them.

- Record the selection-rule version and thresholds used: currently 100 m or more at 10% reflectivity, 360° multi-ring spinning, Ethernet UDP, and LiDAR plus IMU under US $500 (about US $450 for a used LiDAR).
- Built-in IMU is preferred, not required; weight and power must be vendor-stated. Parser compatibility needs checking separately from Ethernet UDP.
- Keep the latest assessment on the stable page. Choose the archive from the review's UTC year, even when a source or corrected entry belongs to another year.
- If that year's archive does not exist, create it with an opening paragraph linking to the current page and a `## Reviews` section; add one year link to the page's Log index. Leave earlier years closed to new reviews.
- Stop and report if the page lacks its model/listing sections; do not reconstruct the assessment from memory.

### 2. Research

Search the web. Attempt to re-check every tracked model and offer, then look for newcomers. Send
independent searches together. Record blocked or omitted checks explicitly.

**(a) Lifecycle of the unit in service.** Hesai Pandar40P: end-of-life notice, presence in the
Hesai SDK compatibility table (`HesaiTechnology/HesaiLidar_SDK_2.0`), and identifiable offers.
SDK inclusion or absence of a notice does not establish continued production.

**(b) Vendor facts and new offers.** Vendor product, datasheet, and announcement pages, plus
distributor stores, for:

- Hesai: OT128, Pandar128E3X, Pandar64, Pandar20, XT32, XT32M2X
- Ouster: OS2, OS1 Max, and OS1, with revisions kept distinct; Velodyne: VLP-16, VLP-32C, HDL-32E
- RoboSense: Helios-32, Helios-16P, Ruby Plus, RS-LiDAR-16, RS-LiDAR-32
- Any newcomer claiming 100 m or more at 10% on a 360° spinning design

On recorded vendor figures the XT32 and standard OS1 fall below range. Re-check rather than
assume, and resolve revision-specific sources before transferring facts between revisions.
Maximum range and range at 10% are different metrics, not contradictory figures by themselves.

**(c) Used offers.** eBay, including regional sites, and similar resale or surplus sites, one query
per model: Pandar40P, Pandar64, Pandar20, XT32, OS1, OS2, VLP-32C, VLP-16, HDL-32E, Helios,
RS-LiDAR-16, and RS-LiDAR-32. Flag under-budget leads for investigation, keeping evidence quality
visible. Confirm individual item identity, price, condition, and availability before shortlisting.

**(d) Announcements since the previous review.** Discontinuations, price changes, and new
100 m-class spinning units. Link to a previous year's latest entry at the year boundary.

### 3. Apply the evidence and freshness rules

- Record only what a page or snippet states, with source title and URL. Use "not stated" for absent information; do not invent prices, range, weight, availability, or revision identity.
- Mark an opened page as "page" and a search result as "snippet". If a snippet combines results, mark "pairing implied". A storefront price range is not an identifiable item price.
- Keep listed currencies, tax basis, condition, region, seller/item identity, and the observation date. Add a US dollar figure only if the source provides one; never compare currencies numerically as though they were the same.
- An opened primary hardware source must establish eligibility, and an opened individual offer must establish price, condition, and active availability before it enters the verified active shortlist. Include necessary accessories, delivery, tax, and IMU costs in the budget decision; unresolved costs remain a coverage gap.
- Record the check date independently of the last observation date. A blocked, omitted, or unsuccessful recheck does not refresh a carried fact or price. A storefront observation does not refresh an individual item.
- Record the current freshness window in the review: 28 elapsed UTC days. At age 28 days or more, remove an offer from the current table even if the recheck was blocked or reviews were missed. A missing observation date means freshness is unknown; keep it in the archive until observed again.
- Remove sold, ended, or unavailable offers immediately. Preserve their observations in the archive as historical price signals, never as active availability or completed sale prices.
- After removal, distinguish "none seen" after a successful search from "unknown" after a blocked check. Neither establishes that a model is over budget.

### 4. Replace the current assessment

Maintain the page's separate Models and Listings tables, model notes, verified shortlist, and
coverage gaps. Replace the Latest research summary with one link to this review. Do not accumulate
dated summaries or listing history on the current page.

- **Models:** update range, revision, lifecycle, and extra specifications only with attributable evidence. Keep range screening separate from full eligibility, price, availability, and confidence. Evidence links must identify each claim's observation date; do not assign today's date to an inherited claim.
- **Listings:** retain fresh active offers and fresh unverified leads with their availability/evidence labels. Update only the fields actually observed; record differing field dates in a short note. Keep sold, expired, and price-free research leads in the archive.
- **Price summary:** if useful, name the "lowest observed active asking price" separately for new and used offers in each listed currency. Only verified active offers qualify; no market-wide minimum or representative trend is implied.
- **Shortlist:** put newly verified under-budget offers first. If none are established, say so; cheap snippets remain investigation leads.
- **Coverage:** carry forward unresolved gaps and identify which checks failed this time. Preserve last observations when rechecks fail.
- Remove unused offer link definitions and resolved gap notes from the current page; their history belongs in the archive. Use short reference links to keep tables readable.

Add rows for newcomers. Move a model no longer tracked to the exclusion note with a reason and
dated sources. Keep exact model/revision identity, or explicitly say when the source leaves it
unknown. Do not add separate catalogue or per-review files.

### 5. Add the annual review entry

Insert one `### YYYY-MM-DD` section below `## Reviews` in the selected annual file, newest first.
Use the page's entry format: Assessment and changes, Listings observed, Announcements, and Checks
and coverage gaps. Record the selection-rule version and thresholds, freshness window, and evidence
limits. Aim for about 60 lines, preserving the details needed to interpret the review.

- Record new or changed observations with enough detail to survive a dead link: model/revision, source title/URL, seller/item identity, price/currency, condition, availability, region, last observed date, and evidence type. Prefer narrow tables plus short notes to one very wide table.
- Link to unchanged earlier observations instead of copying snapshots. Changed availability or successful re-observation may warrant a new record even when the asking price is unchanged.
- Keep source URLs or reference definitions in the annual file itself. Never make an archived observation depend on mutable offer link definitions in the current page; use a dated archive link for inherited evidence.
- "No change" still records successful, blocked, and omitted checks; it does not assert full verification.
- Published entries are immutable, including in the current year. Correct claims in a later entry and link back to the original; do not rewrite history or its observation dates. For another run on the same date, add a separate `### YYYY-MM-DD HH:MM:SS UTC` heading using the run time; do not alter a published entry or duplicate an anchor.
- Keep the two legacy dated anchors on the current page, but add no new dated headings there. Never replace either archive or current page with the other.
- Retain edited findings and provenance in Git. Keep scraped responses, raw search output, runtime logs, and other raw artefacts outside the checkout or in ignored local output; never stage them.

### 6. Check

Format the current page, skill, and new entry with the repository's pinned Prettier. Protect
published sections from formatter changes; the migrated legacy reviews already have range-ignore
markers. Inspect the diff to ensure earlier entries, source URLs, and observation dates are intact.

Run from the repository root (replace `YYYY` with the review year):

```bash
python3 scripts/check-doc-header-metadata.py docs/platform/hardware/lidar-market-watch.md docs/platform/hardware/lidar-market-reviews/YYYY.md
python3 scripts/check-british-spelling.py docs/platform/hardware/lidar-market-watch.md docs/platform/hardware/lidar-market-reviews/YYYY.md
python3 scripts/check-relative-links.py docs/platform/hardware/lidar-market-watch.md docs/platform/hardware/lidar-market-reviews/YYYY.md .claude/skills/lidar-market-review/SKILL.md
make check-no-results
```

Verify links to dated observations resolve to the right headings, including cross-year links.
Verify that the current page has only its two legacy dated headings. Fix findings in changed files;
report unrelated findings without changing them. The caller runs the repository's commit gates and
stages only the current page and affected annual file after inspecting the staged diff.

### 7. Report

Summarise in the session, in this order:

1. Newly verified active under-budget offers, with individual listing URLs.
2. Under-budget investigation leads, with their evidence and availability limits.
3. Pandar40P lifecycle: observations changed or unchanged, and what was actually verified.
4. Other model and offer changes, including expired or ended offers removed from current state.
5. Coverage gaps and links to the current assessment and dated annual entry.

## The routine

The existing Claude Code routine named "LiDAR market review" fires fortnightly in a fresh session.
Its prompt checks out `claude/lidar-market-watch`, invokes this skill, then commits, pushes, and
notifies. The schedule, environment, repository, and prompt are managed in the claude.ai routines
view. The procedure lives here; keep the prompt referring to the checked-in skill rather than
copying its tables or file scope into scheduler settings. Use the current skill from main when
preparing a run branch so a long-lived branch does not retain an obsolete procedure.

## Rules

- British English spelling; no em dashes.
- For a research run, edit only `docs/platform/hardware/lidar-market-watch.md` and the selected `docs/platform/hardware/lidar-market-reviews/YYYY.md` annual file. Procedure maintenance may edit this skill separately.
- Earlier published reviews stay unchanged. Corrections belong in later entries.
- The fortnightly cadence stays unchanged. The current page stays at one assessment; historical growth belongs in annual files.
