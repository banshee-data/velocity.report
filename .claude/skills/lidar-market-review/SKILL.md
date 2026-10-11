---
name: lidar-market-review
description: Fortnightly LiDAR market review. Re-checks listed new and used prices for every tracked spinning LiDAR against the selection and budget rules, updates the Latest table in docs/platform/hardware/lidar-market-watch.md in place, and adds a dated change log entry.
argument-hint: "[YYYY-MM-DD]"
---

# Skill: lidar-market-review

Keep the [LiDAR market watch](../../../docs/platform/hardware/lidar-market-watch.md) current: one
maintained **Latest** table of listed prices, plus a dated Log entry recording what moved. Research
and documentation only; never change code.

## Usage

```
/lidar-market-review            # review dated today (UTC)
/lidar-market-review 2026-10-19 # review with an explicit date
```

The page is the source of truth for the rules and the table layout. This skill holds the
procedure. A caller (such as the scheduled routine) owns branch, commit, push, and notification.

## Procedure

### 1. Read the page

Read `docs/platform/hardware/lidar-market-watch.md`: **Selection rule**, **Latest**, **Entry
format**, and the newest Log entry. Follow them exactly. In short:

- Range rule: 100 m or more at 10% reflectivity, 360° multi-ring spinning, Ethernet UDP output.
- Budget rule: LiDAR plus IMU under US $500, so a used LiDAR at or under about US $450.
- Built-in IMU preferred, not required; weight and power must be vendor-stated.
- The Livox Mid-360 class is excluded by range.

If the page lacks a Latest table, stop and report; do not rebuild the page from memory.

### 2. Research

Search the web. Re-check every row of the Latest table, then look for newcomers. Send independent
searches together.

**(a) Lifecycle of the unit in service.** Hesai Pandar40P: end-of-life notice, presence in the
Hesai SDK compatibility table (`HesaiTechnology/HesaiLidar_SDK_2.0`), any price.

**(b) Listed new prices and vendor figures.** Vendor product, datasheet, and press pages, and
distributor stores, for:

- Hesai: OT128, Pandar128E3X, Pandar64, Pandar20, XT32, XT32M2X
- Ouster: OS2, OS1 Max, OS1; Velodyne legacy VLP-16, VLP-32C, HDL-32E
- RoboSense: Helios-32, Helios-16P, Ruby Plus, RS-LiDAR-16, RS-LiDAR-32
- Any newcomer claiming 100 m or more at 10% on a 360° spinning design

On vendor figures the Hesai XT32 and standard Ouster OS1 fall under 100 m at 10%; re-check rather
than assume. Fill any "not yet checked" cell you can.

**(c) Used asking prices.** eBay (including regional sites such as ebay.de and ebay.co.uk) and
similar resale or surplus sites, one query per model:
"Hesai Pandar40P", "Hesai Pandar64", "Hesai Pandar20", "Hesai XT32", "Ouster OS1", "Ouster OS2",
"Velodyne VLP-32C", "Velodyne VLP-16", "Velodyne HDL-32E", "RoboSense Helios",
"RoboSense RS-LiDAR-16", "RoboSense RS-LiDAR-32".
Flag every listing at or under US $450 for a model that meets the range rule.

**(d) Announcements since the previous Log entry.** Discontinuations, price changes, new 100
m-class spinning units.

### 3. Evidence rules

- Record only prices a page or snippet actually states. Never invent a price, range, or weight:
  write "not stated" with the URL checked.
- When a page is blocked by the proxy, use the search snippet, mark the figure "snippet", and name
  the block in Coverage gaps.
- When a snippet summarises several results, mark the price "pairing implied".
- Convert nothing silently: keep the listed currency; add a US dollar figure only when the page
  gives one.
- An eBay sold or ended listing is a price signal, not availability: say "sold" or "ended".

### 4. Update the Latest table in place

For each row:

- **Lowest new price listed** and **Lowest used asking price**: the lowest current price with its
  URL. Replace a price whose listing is gone or older than two reviews with the next lowest, or
  "none seen".
- **Range at 10%** and **Lifecycle**: update only on a vendor or distributor source.
- **Status**: recompute from the definitions on the page. **Qualifies** requires the range rule met
  on a vendor figure and a used asking price at or under US $450 with a URL.
- **Checked**: today's date for every row re-checked this run.

Add a row for a newcomer that meets the range rule. Do not delete rows; a model that stops being
relevant moves to the "Excluded and not tracked" line with a reason.

### 5. Add the Log entry

Insert `### YYYY-MM-DD` at the top of `## Log`, in the format the page's **Entry format** section
defines (Changes, Listings seen, Announcements, Coverage gaps). Keep it under about 60 lines. If
nothing moved, write one "No change" line plus the checks run. Update the header's
**Last reviewed** date.

### 6. Check

From the repository root:

```bash
prettier --write docs/platform/hardware/lidar-market-watch.md
python3 scripts/check-doc-header-metadata.py
python3 scripts/check-british-spelling.py
python3 scripts/check-relative-links.py
```

Fix anything reported in the market watch page. Report, but do not fix, findings in other files.

### 7. Report

Summarise in the session, in this order:

1. Models that newly reached **Qualifies**, with listing URLs.
2. Every listing at or under US $450 for a range-qualifying model, with URLs.
3. Pandar40P lifecycle: changed or unchanged.
4. Other Latest cells that changed.
5. Coverage gaps.

## The routine

A Claude Code routine named "LiDAR market review" fires fortnightly in a fresh session, with a
prompt that checks out the `claude/lidar-market-watch` branch, invokes this skill, then commits,
pushes, and notifies. The schedule, the environment, the attached repository, and the prompt are
edited in the claude.ai routines view; the method is edited here, so the two never disagree.

## Rules

- British English spelling; no em dashes.
- Edit only `docs/platform/hardware/lidar-market-watch.md`.
- Earlier Log entries are history: never rewrite them.
