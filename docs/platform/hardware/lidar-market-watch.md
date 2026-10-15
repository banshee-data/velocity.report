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

Latest research: [October 15, 2026][review-latest]. Every price and most specifications are
search snippets; the proxy failed to resolve vendor, distributor, and resale pages. Only the Hesai
SDK 2.0 README opened. No verified active offer has been established. The Pandar40P's cheap offers,
including one newly seen eBay item, remain leads to investigate.

### Models

Keep one row per model or materially different revision. **Meets range** and **Below range** apply
only to the recorded 10% figure; **Unverified** means a missing, conflicting, or uncertain figure.
The Evidence column qualifies those assessments. It links to the dated observation that contains
the source URLs and limitations, not to a new verification of them.

| Model/revision        | Range at 10%                       | Range screen | Lifecycle claim                                      | Evidence                                                                                                 |
| --------------------- | ---------------------------------- | ------------ | ---------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Hesai Pandar40P       | 200 m                              | Meets range  | Legacy; in SDK 2.0 table; no end-of-life notice seen | [Snippet, September 19][review-september]; [SDK page opened, October 15][review-latest]                  |
| Hesai Pandar128E3X    | 200 m                              | Meets range  | Reported current                                     | [Snippet, September 19][review-september]                                                                |
| Hesai OT128           | 200 m                              | Meets range  | Launched September 2024; reported in mass production | [Snippet, September 19][review-september]; [snippet, October 15][review-latest]                          |
| Hesai Pandar64        | 200 m reported                     | Meets range  | In SDK 2.0 table                                     | [Snippet, October 11][review-october-11]                                                                 |
| Hesai Pandar20        | Not stated                         | Unverified   | Not stated; no Hesai specification page found        | [Searches, October 11][review-october-11] and [October 15][review-latest]                                |
| Hesai XT32            | 80 m                               | Below range  | Reported current                                     | [Snippet, September 19][review-september]                                                                |
| Hesai XT32M2X         | 80 m                               | Below range  | Reported current                                     | [Snippet, September 19][review-september]                                                                |
| Ouster OS2 Rev 7      | 200 m                              | Meets range  | Reported current                                     | [Snippet, September 19][review-september]                                                                |
| Ouster OS1 Max Rev 8  | 200 m                              | Meets range  | Announced May 2026                                   | [Snippet, September 19][review-september]                                                                |
| Ouster OS1 Rev 7      | 90 m                               | Below range  | Reported current                                     | [Snippet, September 19][review-september]                                                                |
| Ouster OS1 Rev 8      | 90 m reported                      | Below range  | Reported current; revision source needs checking     | [Snippet, September 19][review-september]                                                                |
| Velodyne VLP-32C      | Not stated                         | Unverified   | Support and repairs ended December 2024              | [Snippet, September 19][review-september]                                                                |
| Velodyne VLP-16       | Not stated                         | Unverified   | Technical support to end of 2026                     | [Snippet, September 19][review-september]                                                                |
| Velodyne HDL-32E      | Not stated; 100 m maximum reported | Unverified   | Discontinued; no notice date seen                    | [Snippets, October 11][review-october-11] and [October 15][review-latest]                                |
| RoboSense Helios-32   | 110 m, 80 m, and 70 m figures seen | Unverified   | End-of-life claim at one distributor                 | [Snippets, September 19][review-september], [October 11][review-october-11], [October 15][review-latest] |
| RoboSense Helios-16P  | 110 m or 90 m                      | Unverified   | As Helios-32                                         | [Conflicting snippets, September 19][review-september]                                                   |
| RoboSense Ruby Plus   | 240 m                              | Meets range  | Reported current                                     | [Snippet, September 19][review-september]; [snippet, October 15][review-latest]                          |
| RoboSense RS-LiDAR-16 | Not stated; 150 m maximum          | Unverified   | Not stated; vendor names Helios-16P its upgrade      | [Snippets, October 11][review-october-11] and [October 15][review-latest]                                |
| RoboSense RS-LiDAR-32 | Not stated                         | Unverified   | Not stated                                           | [Searches, October 11][review-october-11] and [October 15][review-latest]                                |

Model notes retain the extra specification claims without widening the comparison table. Each
observation link supplies its sources; these notes are replaced when better evidence arrives.

- **Pandar40P:** 40 channels, 40° vertical FOV, 1.52 kg, 18 W typical; IMU not stated. SDK compatibility and absence of a notice do not prove continued production. [Observed September 19][review-september]; SDK table [opened October 15][review-latest].
- **Pandar128E3X:** 128 channels, 40° vertical FOV, 1.63 kg, 27 W maximum; IMU not stated. [Observed September 19][review-september].
- **OT128:** 128 channels, 40° vertical FOV, 2.2 kg; 29 W in brochure/manual snippets conflicts with 18 W at a reseller. IMU not stated. [Observed September 19][review-september].
- **Pandar64:** the 200 m figure is a snippet of Hesai's comparison page; no datasheet opened. Weight, power, and IMU not stated. [Observed October 11][review-october-11].
- **XT32M2X:** 0.49 kg and 10 W reported; IMU not stated. [Observed September 19][review-september].
- **OS2 Rev 7:** 32, 64, or 128 channels, 22.5° vertical FOV, 930 g; 20 W supply and 22–28 W peak cold. An IMU is reported, but its part number came from an earlier revision. [Observed September 19][review-september].
- **OS1 Max Rev 8:** 128 or 256 channels; 45° vertical FOV in the announcement versus 43.9° in the datasheet snippet. Weight, power, and IMU not stated; resellers quote price on enquiry. [Observed September 19][review-september] and [October 15][review-latest].
- **OS1:** 455 g, 14–20 W, and IAM-20680HT IMU were reported under a combined Rev 7/8 label. Keep those claims unassigned until revision-specific evidence is checked; the cited datasheet is Rev 7. The recorded Rev 7 FOV conflict is 45° versus 42.4°. [Observed September 19][review-september].
- **Helios-32:** 32 channels, 26° vertical FOV, about 1.0 kg, 12 W; **Helios-16P:** 16 channels, 30°, about 1 kg, 11 W typical. Both IMUs are unstated. The Helios 32 H32F70 variant (70° vertical FOV) is reported at 110 m at 10% for channels 7–13 only; a distributor snippet gives 70 m, and RobotShop gives 80 m for the RS-Helios-1615. A 150 m maximum is not a 10% figure. [Observed September 19][review-september], [October 11][review-october-11], and [October 15][review-latest].
- **Ruby Plus:** 128 channels, 40° vertical FOV, 30 W; weight and IMU not stated. [Observed September 19][review-september].

Excluded from routine tracking: Livox Mid-360 and Mid-360S, RoboSense Airy, LSLiDAR C32W and CH32R
(reported below range), and Seyond (no spinning unit seen). Keep the reason and
[dated sources][review-september] when moving a model out of tracking.

### Listings

**Verified active shortlist:** none established. For a shortlisted offer, primary hardware sources
must establish eligibility, and an opened listing page must tie the price, condition, and active
availability to one identifiable item. The offer must meet the budget rule. A snippet, storefront
range, or implied price pairing remains an unverified lead.

**Under-budget leads:** Pandar40P from ProTech Supply (storefront US $149.50–299; eBay item
227057977340 newly seen with US $179.40 used and US $399 new, pairing implied) and PremiumPLC
(US $425 variant, unresolved). None has an opened page; delivery and tax are not resolved.

Freshness is **28 elapsed UTC days from the last observation of that offer**, independent of how
many reviews ran. At 28 days or more it is stale and leaves this table; sold, ended, and unavailable
offers leave immediately. Both remain in the archive. A blocked recheck preserves the last
observation date and cannot restore an expired offer. Readers apply the same age rule between
runs.
When a listing is removed, distinguish **none seen** after a successful search from **unknown**
after a blocked check. Neither means the model is over budget.

Availability is unknown for every row: no listing page opened. The table preserves the stated
currencies and condition claims; it does not select a lowest active price. Rows last observed on
2026-09-19 reach 28 days on 2026-10-17.

| Model/revision                       | Asking price        | Condition claim                            | Availability/evidence             | Last observed | Source                                       |
| ------------------------------------ | ------------------- | ------------------------------------------ | --------------------------------- | ------------- | -------------------------------------------- |
| Pandar40P, revision not stated       | US $149.50–299      | Warranty; individual item not identified   | Unknown; storefront snippet       | 2026-10-15    | [eBay: protechsupply][offer-protechsupply]   |
| Pandar40P, revision not stated       | US $179.40 or $399  | Used, 30-day or new, 60-day guarantee      | Unknown; pairing implied          | 2026-10-15    | [eBay: 227057977340][offer-40p-protech-item] |
| Pandar40P, revision not stated       | US $425 or US $575  | 90-day warranty                            | Unknown; variant price unresolved | 2026-09-19    | [PremiumPLC][offer-premiumplc]               |
| Pandar40P, revision not stated       | US $520             | Used; box, PSU, case; 60-day warranty      | Unknown; snippet                  | 2026-09-19    | [Lablink: 6257123][offer-lablink]            |
| Pandar40P, revision not stated       | US $550             | Used, 90-day warranty; four, Michigan      | Unknown; snippet                  | 2026-10-11    | [eBay: 365679803957][offer-40p-michigan]     |
| XT32, revision not stated            | 1,750 EUR or offer  | Original accessories; private, Germany     | Unknown; snippet                  | 2026-10-15    | [eBay: 406739554237][offer-xt32-used]        |
| XT32, revision not stated            | US $2,138.07        | New                                        | Unknown; store snippet            | 2026-10-15    | [eBay: nebulaautomation][offer-xt32-new]     |
| XT32, XT16 attribution to confirm    | US $1,400           | New; backordered                           | Unknown; snippet                  | 2026-10-15    | [Robotics Center][offer-xt32-rc]             |
| XT32, revision not stated            | US $2,250           | New, original packaging                    | Unknown; pairing implied          | 2026-09-19    | [eBay: 316843061791][offer-xt32-pairing]     |
| XT32M2X                              | US $6,900           | New store offer                            | Unknown; snippet                  | 2026-09-19    | [KartaView][offer-kartaview]                 |
| XT32M2X                              | US $10,499          | New reseller offer                         | Unknown; snippet                  | 2026-10-15    | [Openelab][offer-openelab]                   |
| OS2-128 Rev 7 attribution to confirm | 5,850 EUR ex VAT    | New store offer                            | Unknown; snippet                  | 2026-10-11    | [Ghostysky][offer-ghostysky]                 |
| OT128                                | 7,200 EUR           | New store offer                            | Unknown; listing-title snippet    | 2026-10-11    | [quadruped.de][offer-ot128]                  |
| OT128                                | US $12,476          | New supplier listing                       | Unknown; snippet                  | 2026-10-15    | [Accio][offer-ot128-accio]                   |
| OS1-32, revision not stated          | US $4,599 or $5,230 | New; Hefei or Hong Kong                    | Unknown; pairing implied          | 2026-10-15    | [eBay: 316022902445][offer-os1-32]           |
| VLP-32C                              | US $1,198           | Upgraded bundle with GNSS                  | Unknown; store snippet            | 2026-10-15    | [eBay: cezday][offer-vlp32c-cezday]          |
| VLP-16                               | US $399 or offer    | Used; Israel; US $65 shipping              | Unknown; snippet                  | 2026-10-15    | [eBay: 267463768804][offer-vlp16-israel]     |
| VLP-16                               | About US $399       | Used, 30-day returns                       | Unknown; snippet                  | 2026-10-05    | [eBay: 305167420502][offer-vlp16]            |
| Helios, 16/32 variant unresolved     | US $1,630–2,590     | New supplier offer                         | Unknown; snippet                  | 2026-10-15    | [Alibaba: 1601757765596][offer-alibaba]      |
| Helios series, variant unresolved    | US $1,800           | New, device only                           | Unknown; snippet                  | 2026-10-15    | [RoboSense store][offer-helios-store]        |
| Helios 32 H32F70                     | US $4,669; 3,999 €  | New store offer                            | Unknown; snippet                  | 2026-10-15    | [OpenELab][offer-helios-openelab]            |
| Helios, 1615 attribution to confirm  | US $4,786           | New store offer, on demand                 | Unknown; snippet                  | 2026-10-15    | [RobotShop][offer-robotshop]                 |
| RS-LiDAR-16                          | 523.91 EUR          | Used, "99% new"; ten; Shenzhen; 35 EUR P&P | Unknown; snippet                  | 2026-10-15    | [eBay: 316241231630][offer-rs16-a]           |
| RS-LiDAR-16                          | US $541.01          | Used; three; Shenzhen; US $30 shipping     | Unknown; snippet                  | 2026-10-15    | [eBay: 157485527025][offer-rs16-b]           |

The VLP-32C at about US $399.99 (eBay 176310211757) is reported out of stock and left this table;
sold VLP-32C, RS-LiDAR-16, and OS1-32 offers and the ended Ouster offers stay in the
[archive][review-latest]. Listings without a stated price remain there as research leads.

### Coverage gaps

- Vendor, distributor, and resale pages failed to resolve again; all prices are snippets. Verify primary hardware pages and individual offers before shortlisting; a seller's condition claim is not a physical inspection.
- No 10% range figure for Pandar20, HDL-32E, RS-LiDAR-16, or RS-LiDAR-32. The Helios figures conflict (110, 90, 80, and 70 m), and Pandar64's figure rests on one snippet.
- Several Ouster revision claims remain unresolved; no OS2 price was found this run.
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
[review-october-11]: lidar-market-reviews/2026.md#2026-10-11
[review-latest]: lidar-market-reviews/2026.md#2026-10-15
[offer-protechsupply]: https://www.ebay.com/usr/protechsupply
[offer-40p-protech-item]: https://www.ebay.de/itm/227057977340
[offer-premiumplc]: https://premiumplc.com/products/hesai-pandar-40p-pandar40p-40-channel-360-spinning-long-range-lidar-uspi
[offer-lablink]: https://www.lablink.com/listings/6257123-used-hesai-pandar40p-40-channel-360-spinning-long-range-lidar-with-warranty
[offer-40p-michigan]: https://www.ebay.de/itm/365679803957
[offer-xt32-used]: https://www.ebay.de/itm/406739554237
[offer-xt32-new]: https://www.ebay.com/str/nebulaautomation/Industrial-Automation-Motion-Controls/_i.html
[offer-xt32-rc]: https://www.roboticscenter.ai/ar/store/product/hesai-technology-hesai-xt32
[offer-xt32-pairing]: https://www.ebay.com/itm/316843061791
[offer-kartaview]: https://store.kartaview.org/products/hesai-xt32m2x
[offer-openelab]: https://openelab.com/products/hesai-xt32m2x-32-channel-lidar
[offer-ghostysky]: https://www.ghostysky.com/home/shop-ouster-lidars/
[offer-ot128]: https://www.quadruped.de/Hesai-OT128_1
[offer-ot128-accio]: https://es.accio.com/plp/ot128
[offer-os1-32]: https://www.ebay.de/itm/316022902445
[offer-vlp32c-cezday]: https://www.ebay.com/str/cezday
[offer-vlp16-israel]: https://www.ebay.de/itm/267463768804
[offer-vlp16]: https://www.ebay.com/itm/305167420502
[offer-alibaba]: https://www.alibaba.fr/product-detail/Helios-RS-Helios16-32-Robosense-LiDAR_1601757765596.html
[offer-helios-store]: https://store.robosense.ai/products/helios-series
[offer-helios-openelab]: https://openelab.com/products/robosense-helios-h32f70-lidar-sensor
[offer-robotshop]: https://robotshop.com/products/robosense-lidar-rs-helios-1615-150-m-us
[offer-rs16-a]: https://www.ebay.de/itm/316241231630
[offer-rs16-b]: https://www.ebay.de/itm/157485527025
