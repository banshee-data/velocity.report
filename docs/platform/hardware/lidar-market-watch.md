# LiDAR market watch

- **Status:** Active
- **Cadence:** Weekly, by a scheduled Claude routine; entries by hand are welcome too
- **Related plan:** [lidar-route-capture-plan.md](../../plans/lidar-route-capture-plan.md)

A running log of spinning LiDAR units that could replace or supplement the Hesai Pandar40P for
backpack and tripod survey capture. The Pandar40P is a legacy model with no published new-unit
price, so supply is in practice the used market, and the project needs to know when a current unit
meets the selection rule at an activist's price.
Sensor facts for the unit in service live in the
[hardware knowledge module](../../../.github/knowledge/hardware.md).

## Selection rule

| Requirement               | Value                                           | Why                                                                                  |
| ------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------ |
| Range at 10% reflectivity | 100 m or more                                   | Junction approaches and speeds at range; excludes the Livox Mid-360 class            |
| Horizontal coverage       | 360°, multi-ring spinning                       | The L3 background model is a ring-by-azimuth range image                             |
| Interface                 | Ethernet UDP point packets                      | L1 parses UDP; PCAP is the capture artefact                                          |
| Built-in IMU              | Preferred, not required                         | Tier 1 of the plan's hardware ladder without a bracket                               |
| Weight and power          | Stated by the vendor                            | Backpack carried, battery powered                                                    |
| Price                     | New list price or used asking price, with a URL | Only prices a page actually states                                                   |
| Budget                    | LiDAR plus IMU under US $500 in total           | A used LiDAR at or under about US $450 and an IMU under US $50; an activist's budget |

## Shortlist

Units that met both the range rule and the budget rule in the latest snapshot, with the used
asking prices seen. A model appears here only with a listing URL in the log.

| Model           | Range at 10% | Used asking prices seen          | Snapshot   |
| --------------- | ------------ | -------------------------------- | ---------- |
| Hesai Pandar40P | 200 m        | US $299 to US $575, snippet-only | 2026-09-19 |

Meet the range rule but not the budget, watched for price drops: Hesai OT128 and Pandar128E3X,
Ouster OS2 and OS1 Max, RoboSense Helios-32 and Ruby Plus, Velodyne VLP-32C. Not yet seen in a
snapshot and added to the search list: Velodyne HDL-32E, RoboSense RS-LiDAR-16 and RS-LiDAR-32,
Hesai Pandar64 and Pandar20.

## Entry format

Each weekly check adds one dated section at the top of the log, newest first, with:

- **New units:** table of model, channels, range at 10%, vertical FOV, IMU, weight, power, price, source URL.
- **Used listings:** table of model, asking price, condition claim, region, listing URL, date seen.
- **Announcements:** discontinuations, price changes, new units, with URLs.
- **Coverage gaps:** what could not be verified this week.

"Not stated" is a valid value. A figure without a URL is not an entry.

The weekly routine pushes its entry to the `claude/lidar-market-watch` branch and summarises it in
its session. Merge the branch whenever the log is worth keeping; nothing else reads it.

## Log

### 2026-09-19

Provenance: the research session's proxy blocked every vendor, distributor, press, and resale page,
so each figure below is the search-engine snippet of the cited URL, not a page that was opened.
Confirm a figure before it enters a decision. Range figures are at 10% reflectivity.

**New units** (meet the rule unless noted):

| Model                           | Channels       | Range at 10%                                           | Vertical FOV                                     | IMU                                                          | Weight       | Power                                                     | Price                                                                                          | Source                                                                                                                                                                                                         |
| ------------------------------- | -------------- | ------------------------------------------------------ | ------------------------------------------------ | ------------------------------------------------------------ | ------------ | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hesai Pandar40P (in service)    | 40             | 200 m                                                  | 40°                                              | not stated                                                   | 1.52 kg      | 18 W typical                                              | not stated; still listed in the Hesai SDK 2.0 compatibility table; no end-of-life notice found | https://www.manualslib.com/manual/1551481/Hesai-Pandar40p.html?page=7 and https://github.com/HesaiTechnology/HesaiLidar_SDK_2.0                                                                                |
| Hesai Pandar128E3X              | 128            | 200 m                                                  | 40°                                              | not stated                                                   | 1.63 kg      | 27 W max                                                  | not stated                                                                                     | https://store.kartaview.org/products/hesai-pandar128e3x                                                                                                                                                        |
| Hesai OT128                     | 128            | 200 m                                                  | 40°                                              | not stated                                                   | 2.2 kg       | 29 W in brochure and manual; 18 W in one reseller snippet | 7,200 EUR in a listing title                                                                   | https://www.mapix.com/wp-content/uploads/2025/10/Hesai_OT128_brochure.pdf and https://www.quadruped.de/Hesai-OT128_1                                                                                           |
| Ouster OS2 Rev 7                | 32, 64, or 128 | 200 m                                                  | 22.5°                                            | yes; ICM-20948 in an earlier revision, Rev 7 part not stated | 930 g        | 20 W supply, 22 to 28 W peak cold                         | 5,850 EUR ex VAT for OS2-128                                                                   | https://data.ouster.io/downloads/datasheets/datasheet-rev7-v2p5-os2.pdf and https://www.ghostysky.com/home/shop-ouster-lidars/                                                                                 |
| Ouster OS1 Max Rev 8 (May 2026) | 128 or 256     | 200 m                                                  | 45° in the press release, 43.9° in the datasheet | not stated                                                   | not stated   | not stated                                                | not stated                                                                                     | https://investors.ouster.com/news-releases/news-release-details/ouster-releases-rev8-os-family-worlds-first-native-color-lidar and https://data.ouster.io/downloads/datasheets/datasheet-rev8-v4p0-os1-max.pdf |
| RoboSense Helios-32             | 32             | 110 m                                                  | 26°                                              | not stated                                                   | about 1.0 kg | 12 W                                                      | 3,200 EUR in a listing title; end-of-life notice at one distributor                            | https://store.robosense.ai/products/helios-series and https://www.quadruped.de/RoboSense-RS-Helios_1 and https://store.indrorobotics.com/products/rs-helios-32-lidar                                           |
| RoboSense Helios-16P            | 16             | 110 m in the store, 90 m in the manual (open conflict) | 30°                                              | not stated                                                   | about 1 kg   | 11 W typical                                              | not stated                                                                                     | https://store.robosense.ai/products/helios-series and https://cdn.robosense.cn/20220629181832_42540.pdf                                                                                                        |
| RoboSense Ruby Plus             | 128            | 240 m                                                  | 40°                                              | not stated                                                   | not stated   | 30 W                                                      | not stated                                                                                     | https://www.robosense.ai/en/news-show-1637                                                                                                                                                                     |

Below the rule on the vendors' own numbers, so excluded:

- Hesai XT32 (80 m at 10%), XT32M2X (80 m at 10%, 0.49 kg, 10 W, US $6,900 in a store snippet), and XT16 (80 m): https://www.hesaitech.com/product/xt16-32-32m/ and https://store.kartaview.org/products/hesai-xt32m2x
- Ouster OS1 Rev 7 and Rev 8 (90 m at 10%, IMU IAM-20680HT, 455 g, 14 to 20 W): https://data.ouster.io/downloads/datasheets/datasheet-rev7-v3p1-os1.pdf
- RoboSense Airy (30 m), Livox Mid-360S (38 m), LSLiDAR C32W (50 m) and CH32R (30 m): https://openelab.io/products/robosense-airy-96-channel-hemispherical-digital-lidar and https://www.livoxtech.com/mid-360s and https://www.lslidar.com/lslidars-ch32r-c32w-lidars/
- Velodyne VLP-16 and VLP-32C state no 10% figure and are discontinued; LSLiDAR C32 states 150 m maximum and no 10% figure: https://ouster.com/products/hardware/vlp-16 and https://data.ouster.io/downloads/datasheets/velodyne/63-9378_Rev-F_Ultra-Puck_Datasheet_Web.pdf and https://www.lslidar.com/product/c32-16-mechanical-lidar/
- Seyond sells no 360° spinning unit: https://seyond.com/seyond-to-showcase-complete-end-to-end-lidar-portfolio-and-mass-production-ready-solid-state-lidar-at-ces-2026/

**Used listings** (asking prices as shown in the snippet; "pairing implied" means the snippet
summarised several results, so the price may belong to a neighbouring item):

| Model            | Asking price                        | Condition claim                                  | Region          | Listing                                                                                                              | Seen       |
| ---------------- | ----------------------------------- | ------------------------------------------------ | --------------- | -------------------------------------------------------------------------------------------------------------------- | ---------- |
| Hesai Pandar40P  | US $149.50 each, pairing implied    | used, fully operational, 30-day guarantee        | not shown       | https://www.ebay.com/itm/226044651928                                                                                | 2026-09-19 |
| Hesai Pandar40P  | US $299 and US $399                 | pre-owned, with warranty                         | not shown       | https://www.ebay.com/usr/protechsupply                                                                               | 2026-09-19 |
| Hesai Pandar40P  | US $425 and US $575                 | 90-day warranty                                  | not shown       | https://premiumplc.com/products/hesai-pandar-40p-pandar40p-40-channel-360-spinning-long-range-lidar-uspi             | 2026-09-19 |
| Hesai Pandar40P  | US $520                             | used, 60-day warranty, connection box, PSU, case | not shown       | https://www.lablink.com/listings/6257123-used-hesai-pandar40p-40-channel-360-spinning-long-range-lidar-with-warranty | 2026-09-19 |
| Hesai Pandar40P  | not in snippet                      | open box, with case                              | Santa Clara, CA | https://www.ebay.com/itm/196517152527                                                                                | 2026-09-19 |
| Hesai XT32       | US $2,250, pairing implied          | new, original packaging                          | not shown       | https://www.ebay.com/itm/316843061791                                                                                | 2026-09-19 |
| Ouster OS1-32    | US $2,199 best offer, sold          | used once                                        | Tempe, AZ       | https://www.ebay.com/itm/197711710449                                                                                | 2026-09-19 |
| Ouster OS1-32-U  | US $4,900                           | used                                             | Shenzhen        | https://www.ebay.com/itm/397193528270                                                                                | 2026-09-19 |
| Ouster OS2-32-U  | US $6,000, listing ended            | new, open box                                    | not shown       | https://www.ebay.com/itm/156943834624                                                                                | 2026-09-19 |
| Velodyne VLP-32C | US $799.95, completed listing       | used                                             | not shown       | https://www.ebay.com/itm/389775140420                                                                                | 2026-09-19 |
| Velodyne VLP-16  | US $384 to US $395, pairing implied | pre-owned                                        | not shown       | https://www.ebay.com/itm/234740701349                                                                                | 2026-09-19 |
| RoboSense Helios | not in snippet                      | used, cosmetic wear                              | Shenzhen        | https://www.ebay.com/itm/157485526988                                                                                | 2026-09-19 |

**Announcements:**

- 2026-01-05, CES: Hesai plans to double annual capacity to over 4 million units in 2026; no XT successor, and no Pandar40P end-of-life or price change found: https://www.prnewswire.com/news-releases/hesai-announces-plan-to-double-annual-lidar-production-capacity-at-ces-2026-302652276.html
- 2026-05-04: Ouster Rev 8 family, including the new OS1 Max at 200 m at 10%; the standard Rev 8 OS1 stays at 90 m; no Rev 8 OS2: https://investors.ouster.com/news-releases/news-release-details/ouster-releases-rev8-os-family-worlds-first-native-color-lidar
- Undated: a distributor page says the RoboSense Helios is reaching end of life with reduced production: https://store.indrorobotics.com/products/rs-helios-32-lidar
- Velodyne: VLP-16 repairs supported to 2026-06-30 and technical support to the end of 2026; VLP-32C support and repairs ended December 2024: https://digiflec.com/important-update-for-ouster-velodyne-vlp-16-users/ and https://digiflec.com/product-discontinuation-notice-final-chance-to-buy-velodyne-ultra-puck-vlp-32c/
- 2026-07-09: RoboSense first-half results; no new mechanical 360° unit: https://www.prnewswire.com/news-releases/robosense-announces-h1-2026-lidar-sales-of-719-200-units-as-robotics-segment-grows-by-510-4-302821684.html

**Coverage gaps:**

- Every page was blocked by the research session's proxy; all figures are search snippets.
- Not stated anywhere seen: OS1 Max weight, power, and price; Ruby Plus weight and price; Pandar128E3X price; IMU presence for every Hesai and RoboSense spinning unit.
- Open conflicts: Helios-16P range at 10% (110 m store, 90 m manual); OT128 power (29 W versus 18 W); OS1 Rev 7 vertical FOV (45° versus 42.4°).
- Used prices could not always be tied to one item number; most seller regions are not shown; eBay sold dates carry no year.
- Pandar40P lifecycle status rests on the SDK compatibility table and the absence of an end-of-life notice in search results; Hesai's FY2025 annual report could not be read.
