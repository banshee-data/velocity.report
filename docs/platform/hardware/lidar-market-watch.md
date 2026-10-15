# LiDAR market watch

- **Status:** Active
- **Cadence:** Fortnightly, by a scheduled Claude routine running the
  [`lidar-market-review`](../../../.claude/skills/lidar-market-review/SKILL.md) skill; entries by
  hand are welcome too
- **Related plan:** [lidar-route-capture-plan.md](../../plans/lidar-route-capture-plan.md)

This page compares spinning LiDAR models and observed offers for backpack and tripod survey
capture. It holds the current assessment; dated research belongs in the [annual archives](#log).
The Pandar40P in service is a legacy model whose supply is chiefly the used market. Its sensor
facts live in the [hardware knowledge module](../../../.github/knowledge/hardware.md).

The watch supports purchasing investigations. Its asking prices and uneven search coverage do
not establish completed sale prices, market-wide minima, or representative price trends.

## Selection rule

Selection rule **market-watch-v1** uses the thresholds below. A review records this version and
the price-freshness rule it used; change the version when a selection threshold changes.

| Requirement               | Value                                     | Why                                                                    |
| ------------------------- | ----------------------------------------- | ---------------------------------------------------------------------- |
| Range at 10% reflectivity | 100 m or more                             | Junction approaches and speeds at range                                |
| Horizontal coverage       | 360°, multi-ring spinning                 | The L3 background model is a ring-by-azimuth range image               |
| Interface                 | Ethernet UDP point packets                | PCAP is the capture artefact; parser support needs checking separately |
| Built-in IMU              | Preferred, not required                   | The plan's first hardware tier without a bracket                       |
| Weight and power          | Stated by the vendor                      | Backpack carried, battery powered                                      |
| Price                     | Stated asking price, currency, and source | An observation must be attributable to an offer                        |
| Budget                    | LiDAR plus IMU under US $500 in total     | About US $450 for a used LiDAR and under US $50 for an IMU             |

Meeting the range screen does not establish full eligibility. Confirm the other required hardware
facts on primary sources and account for necessary accessories, delivery, and taxes before a
purchase decision. Ethernet UDP alone does not establish compatibility with the existing parser.

## Latest

Latest research: [October 5, 2026][review-october]. This layout was migrated
from that assessment without new market research. Specifications carried from September 19 retain
that observation date. All inherited evidence is from search snippets; no verified active offer
has been established. The Pandar40P's cheap offers are leads to investigate.

### Models

Keep one row per model or materially different revision. **Meets range** and **Below range** apply
only to the recorded 10% figure; **Unverified** means a missing, conflicting, or uncertain figure.
The Evidence column qualifies those assessments. It links to the dated observation that contains
the source URLs and limitations, not to a new verification of them.

| Model/revision        | Range at 10%                           | Range screen | Lifecycle claim                                  | Evidence                                                                   |
| --------------------- | -------------------------------------- | ------------ | ------------------------------------------------ | -------------------------------------------------------------------------- |
| Hesai Pandar40P       | 200 m                                  | Meets range  | Legacy; no end-of-life notice seen               | [Snippet, September 19][review-september]                                  |
| Hesai Pandar128E3X    | 200 m                                  | Meets range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Hesai OT128           | 200 m                                  | Meets range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Hesai Pandar64        | Not yet checked                        | Unverified   | Not yet checked                                  | [Gap, October 5][review-october]                                           |
| Hesai Pandar20        | Not yet checked                        | Unverified   | Not yet checked                                  | [Gap, October 5][review-october]                                           |
| Hesai XT32            | 80 m                                   | Below range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Hesai XT32M2X         | 80 m                                   | Below range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Ouster OS2 Rev 7      | 200 m                                  | Meets range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Ouster OS1 Max Rev 8  | 200 m                                  | Meets range  | Announced May 2026                               | [Snippet, September 19][review-september]                                  |
| Ouster OS1 Rev 7      | 90 m                                   | Below range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| Ouster OS1 Rev 8      | 90 m reported                          | Below range  | Reported current; revision source needs checking | [Snippet, September 19][review-september]                                  |
| Velodyne VLP-32C      | Not stated                             | Unverified   | Support and repairs ended December 2024          | [Snippet, September 19][review-september]                                  |
| Velodyne VLP-16       | Not stated                             | Unverified   | Technical support to end of 2026                 | [Snippet, September 19][review-september]                                  |
| Velodyne HDL-32E      | Not yet checked                        | Unverified   | Not yet checked                                  | [Gap, October 5][review-october]                                           |
| RoboSense Helios-32   | 110 m reported; attribution unresolved | Unverified   | End-of-life claim at one distributor             | [Snippets, September 19][review-september] and [October 5][review-october] |
| RoboSense Helios-16P  | 110 m or 90 m                          | Unverified   | As Helios-32                                     | [Conflicting snippets, September 19][review-september]                     |
| RoboSense Ruby Plus   | 240 m                                  | Meets range  | Reported current                                 | [Snippet, September 19][review-september]                                  |
| RoboSense RS-LiDAR-16 | Not yet checked                        | Unverified   | Not yet checked                                  | [Gap, October 5][review-october]                                           |
| RoboSense RS-LiDAR-32 | Not yet checked                        | Unverified   | Not yet checked                                  | [Gap, October 5][review-october]                                           |

Model notes retain the extra specification claims without widening the comparison table. Each
observation link supplies its sources; these notes are replaced when better evidence arrives.

- **Pandar40P:** 40 channels, 40° vertical FOV, 1.52 kg, 18 W typical; IMU not stated. SDK compatibility and absence of a notice do not prove continued production. [Observed September 19][review-september].
- **Pandar128E3X:** 128 channels, 40° vertical FOV, 1.63 kg, 27 W maximum; IMU not stated. [Observed September 19][review-september].
- **OT128:** 128 channels, 40° vertical FOV, 2.2 kg; 29 W in brochure/manual snippets conflicts with 18 W at a reseller. IMU not stated. [Observed September 19][review-september].
- **XT32M2X:** 0.49 kg and 10 W reported; IMU not stated. [Observed September 19][review-september].
- **OS2 Rev 7:** 32, 64, or 128 channels, 22.5° vertical FOV, 930 g; 20 W supply and 22–28 W peak cold. An IMU is reported, but its part number came from an earlier revision. [Observed September 19][review-september].
- **OS1 Max Rev 8:** 128 or 256 channels; 45° vertical FOV in the announcement versus 43.9° in the datasheet snippet. Weight, power, and IMU not stated. [Observed September 19][review-september].
- **OS1:** 455 g, 14–20 W, and IAM-20680HT IMU were reported under a combined Rev 7/8 label. Keep those claims unassigned until revision-specific evidence is checked; the cited datasheet is Rev 7. The recorded Rev 7 FOV conflict is 45° versus 42.4°. [Observed September 19][review-september].
- **Helios-32:** 32 channels, 26° vertical FOV, about 1.0 kg, 12 W; **Helios-16P:** 16 channels, 30°, about 1 kg, 11 W typical. Both IMUs are unstated. A 150 m maximum is not comparable to a 10% range, so it is not itself a contradiction of 110 m; model attribution and the 16P's 110/90 m conflict still need checking. [Observed September 19][review-september] and [October 5][review-october].
- **Ruby Plus:** 128 channels, 40° vertical FOV, 30 W; weight and IMU not stated. [Observed September 19][review-september].

Excluded from routine tracking: Livox Mid-360 and Mid-360S, RoboSense Airy, LSLiDAR C32W and CH32R
(reported below range), and Seyond (no spinning unit seen). Keep the reason and
[dated sources][review-september] when moving a model out of tracking.

### Listings

**Verified active shortlist:** none established by the inherited research. For a shortlisted
offer, primary hardware sources must establish eligibility, and an opened listing page must tie
the price, condition, and active availability to one identifiable item. The offer must meet the
budget rule. A snippet, storefront range, or implied price pairing remains an unverified lead.

Freshness is **28 elapsed UTC days from the last observation of that offer**, independent of how
many reviews ran. At 28 days or more it is stale and leaves this table; sold, ended, and unavailable
offers leave immediately. Both remain in the archive. A blocked recheck preserves the last
observation date and cannot restore an expired offer. Readers apply the same age rule between
runs.
When a listing is removed, distinguish **none seen** after a successful search from **unknown**
after a blocked check. Neither means the model is over budget.

These inherited leads have unknown availability. The table preserves the stated currencies and
condition claims; it does not select a lowest active price until availability is verified.

| Model/revision                       | Asking price       | Condition claim                          | Availability/evidence               | Last observed | Source                                     |
| ------------------------------------ | ------------------ | ---------------------------------------- | ----------------------------------- | ------------- | ------------------------------------------ |
| Pandar40P, revision not stated       | US $149.50–299     | Warranty; individual item not identified | Unknown; storefront snippet         | 2026-10-05    | [eBay: protechsupply][offer-protechsupply] |
| Pandar40P, revision not stated       | US $425 or US $575 | 90-day warranty                          | Unknown; variant price not resolved | 2026-09-19    | [PremiumPLC][offer-premiumplc]             |
| Pandar40P, revision not stated       | US $520            | Used; box, PSU, case; 60-day warranty    | Unknown; snippet                    | 2026-09-19    | [Lablink: 6257123][offer-lablink]          |
| XT32, revision not stated            | US $2,138.07       | New                                      | Unknown; snippet                    | 2026-10-05    | [eBay: 388641059791][offer-xt32-new]       |
| XT32, revision not stated            | US $2,250          | New, original packaging                  | Unknown; pairing implied            | 2026-09-19    | [eBay: 316843061791][offer-xt32-pairing]   |
| XT32M2X                              | US $6,900          | New store offer                          | Unknown; snippet                    | 2026-09-19    | [KartaView][offer-kartaview]               |
| XT32M2X                              | US $10,499         | New reseller offer                       | Unknown; snippet                    | 2026-10-05    | [Openelab][offer-openelab]                 |
| OS2-128 Rev 7 attribution to confirm | 5,850 EUR ex VAT   | New store offer                          | Unknown; snippet                    | 2026-09-19    | [Ghostysky][offer-ghostysky]               |
| OT128                                | 7,200 EUR          | New store offer                          | Unknown; listing-title snippet      | 2026-09-19    | [quadruped.de][offer-ot128]                |
| VLP-32C                              | About US $399.99   | Used, no accessories                     | Unknown; snippet                    | 2026-10-05    | [eBay: 176310211757][offer-vlp32c]         |
| VLP-16                               | About US $399      | Used, 30-day returns                     | Unknown; snippet                    | 2026-10-05    | [eBay: 305167420502][offer-vlp16]          |
| Helios, 16/32 variant unresolved     | US $1,630–2,590    | New store offer                          | Unknown; snippet                    | 2026-10-05    | [Alibaba: 1601757765596][offer-alibaba]    |
| Helios, 1615 attribution to confirm  | US $4,786          | New store offer                          | Unknown; snippet                    | 2026-10-05    | [RobotShop][offer-robotshop]               |

The US $149.50 Pandar40P item was observed only on September 19, with an implied price pairing.
The October 5 observation was of the seller storefront; it does not refresh that item. Its
[original observation][review-september] stays in the archive, as do sold and
ended Ouster offers. Other listings without a stated price remain there as research leads.

### Coverage gaps

- All inherited specifications and prices came from snippets. Verify primary hardware pages and individual offers before shortlisting; a seller's condition claim is not a physical inspection.
- Range and lifecycle remain unchecked for Pandar64, Pandar20, HDL-32E, RS-LiDAR-16, and RS-LiDAR-32. The Helios model/range attribution and several Ouster revision claims remain unresolved.
- Missing weights, power figures, and IMU facts are recorded in the model notes. Currency, shipping, tax, accessory, and IMU costs must be resolved for a complete budget.
- Lifecycle evidence does not establish ongoing production: SDK support and absence of an end-of-life notice are limited observations.

## Entry format

Each review replaces the Latest assessment and adds a dated section, newest first, to
`lidar-market-reviews/YYYY.md`. The year is the review's UTC year, not the year of a cited source.
Create that year's file on its first review and add one link below. Approximately 26 short
fortnightly entries fit in a year; close the file to new reviews when the year ends.

An entry records the selection-rule version and thresholds, the freshness window, and:

- **Assessment and changes:** old value to new value, with evidence links; verified under-budget offers lead. Link to earlier observations rather than copying unchanged tables.
- **Listings observed:** new or changed offers with exact model/revision, asking price and currency, condition, availability, seller/item identifier, region, last observed date, source title/URL, and page/snippet/pairing evidence. Write "not stated" where the source does not say.
- **Announcements:** relevant model, lifecycle, and price changes, with sources.
- **Checks and coverage gaps:** what was checked, what opened, what was blocked, and which facts remain inherited. Record check dates independently of observation dates. "No change" still requires this account.

Keep entries near 60 lines without dropping evidence needed to interpret the findings. Published
entries are historical records: corrections go in later entries with links to the original claim,
including when the original is in an earlier year's file. Do not copy scraped pages, raw search
responses, or runtime logs into the archive. Retain edited observations and source identifiers.

The routine pushes both the current page and the annual entry to `claude/lidar-market-watch` and
summarises the review in its session. The
[skill](../../../.claude/skills/lidar-market-review/SKILL.md)
defines the procedure; the scheduler retains its existing fortnightly cadence.

## Log

Annual archives preserve the research record under bounded files. This page keeps one latest
assessment, not an accumulating review log. Git records edits; archives explain the observations.

- [2026 reviews](lidar-market-reviews/2026.md)

### 2026-10-05

The [October 5 review][review-october] moved to the annual archive. This
heading preserves the original link target.

### 2026-09-19

The [September 19 review][review-september] moved to the annual archive. These
two legacy anchors stay in place; future dated headings belong only in the annual files.

[review-september]: lidar-market-reviews/2026.md#2026-09-19
[review-october]: lidar-market-reviews/2026.md#2026-10-05
[offer-protechsupply]: https://www.ebay.com/usr/protechsupply
[offer-premiumplc]: https://premiumplc.com/products/hesai-pandar-40p-pandar40p-40-channel-360-spinning-long-range-lidar-uspi
[offer-lablink]: https://www.lablink.com/listings/6257123-used-hesai-pandar40p-40-channel-360-spinning-long-range-lidar-with-warranty
[offer-xt32-new]: https://www.ebay.de/itm/388641059791
[offer-xt32-pairing]: https://www.ebay.com/itm/316843061791
[offer-kartaview]: https://store.kartaview.org/products/hesai-xt32m2x
[offer-openelab]: https://openelab.com/products/hesai-xt32m2x-32-channel-lidar
[offer-ghostysky]: https://www.ghostysky.com/home/shop-ouster-lidars/
[offer-ot128]: https://www.quadruped.de/Hesai-OT128_1
[offer-vlp32c]: https://www.ebay.de/itm/176310211757
[offer-vlp16]: https://www.ebay.com/itm/305167420502
[offer-alibaba]: https://www.alibaba.fr/product-detail/Helios-RS-Helios16-32-Robosense-LiDAR_1601757765596.html
[offer-robotshop]: https://robotshop.com/products/robosense-lidar-rs-helios-1615-150-m-us
