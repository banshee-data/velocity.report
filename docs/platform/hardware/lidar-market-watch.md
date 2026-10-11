# LiDAR market watch

- **Status:** Active
- **Cadence:** Fortnightly, by a scheduled Claude routine running the
  [`lidar-market-review`](../../../.claude/skills/lidar-market-review/SKILL.md) skill; entries by
  hand are welcome too
- **Last reviewed:** 2026-10-11; the Latest table below is current as of that entry in the Log;
  open its Coverage gaps before treating a figure as settled
- **Related plan:** [lidar-route-capture-plan.md](../../plans/lidar-route-capture-plan.md)

A maintained table of listed prices for spinning LiDAR units that could replace or supplement the
Hesai Pandar40P for backpack and tripod survey capture, with a dated log of what changed at each
review. The Pandar40P is a legacy model with no published new-unit price, so supply is in practice
the used market, and the project needs to know when a current unit meets the selection rule at an
activist's price.
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

## Latest

The one table each review maintains in place: one row per tracked model, holding the lowest new
price and the lowest used asking price currently listed, each with its URL. A price leaves the
table when its listing is gone or older than two reviews; the Log keeps the history. Figures marked
"snippet" come from a search-engine result, not an opened page.

Status values: **Qualifies** (range rule met and a used asking price at or under US $450; this is
the shortlist), **Over budget** (range rule met, no qualifying price), **Unverified** (no vendor
figure at 10% reflectivity, or conflicting figures), **Below range** (vendor figure under 100 m at
10%, kept for price reference).

| Model                 | Range at 10%                        | Lowest new price listed                                                                                                                   | Lowest used asking price                                                                             | Lifecycle                                     | Status        | Checked    |
| --------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------- | ------------- | ---------- |
| Hesai Pandar40P       | 200 m                               | not stated                                                                                                                                | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar128E3X    | 200 m                               | not stated                                                                                                                                | none seen                                                                                            | Current; in SDK 2.0 table                     | Over budget   | 2026-10-11 |
| Hesai OT128           | 200 m                               | 7,200 EUR, listing title: [quadruped.de](https://www.quadruped.de/Hesai-OT128_1)                                                          | none seen                                                                                            | Current                                       | Over budget   | 2026-10-11 |
| Hesai Pandar64        | 200 m, snippet                      | not stated                                                                                                                                | none seen                                                                                            | In SDK 2.0 table                              | Over budget   | 2026-10-11 |
| Hesai Pandar20        | not stated                          | not stated                                                                                                                                | none seen                                                                                            | not stated                                    | Unverified    | 2026-10-11 |
| Hesai XT32            | 80 m                                | US $1,400, snippet, backordered: [Robotics Center](https://www.roboticscenter.ai/ar/store/product/hesai-technology-hesai-xt32)            | 1,750 EUR, snippet, condition not stated: [eBay.de](https://www.ebay.de/itm/406739554237)            | Current                                       | Below range   | 2026-10-11 |
| Hesai XT32M2X         | 80 m                                | US $10,499, snippet: [Openelab](https://openelab.com/products/hesai-xt32m2x-32-channel-lidar)                                             | none seen                                                                                            | Current                                       | Below range   | 2026-10-11 |
| Ouster OS2 Rev 7      | 200 m                               | 5,850 EUR ex VAT, OS2-128: [Ghostysky](https://www.ghostysky.com/home/shop-ouster-lidars/)                                                | US $6,000, OS2-32-U, ended: [eBay](https://www.ebay.com/itm/156943834624)                            | Current                                       | Over budget   | 2026-10-11 |
| Ouster OS1 Max        | 200 m                               | not stated                                                                                                                                | none seen                                                                                            | Current (Rev 8, May 2026)                     | Over budget   | 2026-10-11 |
| Ouster OS1 Rev 7/8    | 90 m                                | not stated                                                                                                                                | US $2,199, OS1-32, sold: [eBay](https://www.ebay.com/itm/197711710449)                               | Current                                       | Below range   | 2026-10-11 |
| Velodyne VLP-32C      | not stated                          | discontinued                                                                                                                              | US $1,198, upgraded bundle with GNSS, snippet: [eBay store](https://www.ebay.com/str/cezday)         | Support and repairs ended December 2024       | Unverified    | 2026-10-11 |
| Velodyne VLP-16       | not stated                          | discontinued                                                                                                                              | about US $399, snippet: [eBay](https://www.ebay.com/itm/305167420502)                                | Technical support to end of 2026              | Unverified    | 2026-10-11 |
| Velodyne HDL-32E      | not stated                          | discontinued                                                                                                                              | none seen                                                                                            | not stated                                    | Unverified    | 2026-10-11 |
| RoboSense Helios-32   | 110 m; 80 m at RobotShop (conflict) | US $1,630, snippet, Alibaba: [Alibaba](https://www.alibaba.co.uk/product-detail/Helios-RS-Helios16-32-Robosense-LiDAR_1601757765596.html) | no price in snippet: [eBay](https://www.ebay.com/itm/157485526988)                                   | End of life at one distributor                | Unverified    | 2026-10-11 |
| RoboSense Helios-16P  | 110 m or 90 m (conflict)            | not stated                                                                                                                                | none seen                                                                                            | As Helios-32                                  | Unverified    | 2026-09-19 |
| RoboSense Ruby Plus   | 240 m                               | not stated                                                                                                                                | none seen                                                                                            | Current                                       | Over budget   | 2026-09-19 |
| RoboSense RS-LiDAR-16 | not stated; 150 m maximum           | about 4,128 EUR inc. tax, snippet: [Generation Robots](https://generationrobots.com/blog/?p=6135)                                         | 523.91 EUR, snippet, pairing implied: [eBay.de](https://www.ebay.de/itm/316241231630)                | not stated                                    | Unverified    | 2026-10-11 |
| RoboSense RS-LiDAR-32 | not stated                          | not stated                                                                                                                                | none seen                                                                                            | not stated                                    | Unverified    | 2026-10-11 |

Excluded and not tracked: the Livox Mid-360 and Mid-360S, RoboSense Airy, LSLiDAR C32W and CH32R
(all under 100 m at 10%), and Seyond (no 360° spinning unit); sources in the 2026-09-19 entry.

## Entry format

Each review edits the Latest table in place and adds one dated section at the top of the Log,
newest first, recording only what moved:

- **Changes:** one bullet per Latest cell that changed, old value to new value, with the URL. A
  model that newly reaches **Qualifies** leads the list.
- **Listings seen:** table of model, asking price, condition claim, region, listing URL, date seen;
  every URL now in the Latest table appears here or in an earlier entry.
- **Announcements:** discontinuations, price changes, new units since the last review, with URLs.
- **Coverage gaps:** what could not be verified this time.

If nothing moved, the section is one "No change" line plus the checks run. "Not stated" is a valid
value. A figure without a URL is not an entry. Entries up to 2026-10-05 predate the Latest table
and carry full snapshot tables instead.

The routine pushes its entry to the `claude/lidar-market-watch` branch and summarises it in its
session. Merge the branch whenever the log is worth keeping; nothing else reads it.

**One file, not one per snapshot.** A dated filename would mark each review clearly too, but every
link to it would then point at a fixed date instead of the current state. Git already keeps every
past version (`git log -p` on this file is the full history), so the Latest table carries the
current state and the Log carries the deltas, under one URL a reader or another doc can rely on.

## Log

### 2026-10-11

Provenance: vendor, distributor, and resale pages failed to resolve through the proxy, so every
price below is a search snippet of the cited URL. The Hesai SDK 2.0 README on GitHub was opened.
No model newly reached **Qualifies**.

**Changes:**

- Hesai Pandar64: range "not yet checked" to 200 m at 10%, snippet of Hesai's product comparison page (https://www.hesaitech.com/?p=1360); lifecycle to "In SDK 2.0 table" (https://github.com/HesaiTechnology/HesaiLidar_SDK_2.0); new price to "not stated"; status Unverified to Over budget (no used listing seen).
- Hesai Pandar40P: lowest used asking price source from item 226044651928 (not re-seen) to the seller storefront, still US $149.50, snippet, condition not stated: https://www.ebay.com/usr/protechsupply
- Hesai Pandar128E3X: lifecycle "Current" to "Current; in SDK 2.0 table".
- Hesai XT32: lowest new US $2,138.07 (eBay) to US $1,400, backordered (https://www.roboticscenter.ai/ar/store/product/hesai-technology-hesai-xt32); lowest used US $2,250 (seen 2026-09-19 only) to 1,750 EUR (https://www.ebay.de/itm/406739554237).
- Hesai XT32M2X: lowest new US $6,900 (KartaView, not re-seen) to US $10,499 (https://openelab.com/products/hesai-xt32m2x-32-channel-lidar).
- Hesai Pandar20: range, new price, and lifecycle "not yet checked" to "not stated"; no Hesai specification page found, only a ROS message package naming Pandar20A and Pandar20B (https://docs.ros.org/en/humble/p/pandar_msgs).
- Velodyne VLP-32C: about US $399.99 (https://www.ebay.de/itm/176310211757, now no longer available) to US $1,198 for an upgraded bundle with GNSS (https://www.ebay.com/str/cezday).
- Velodyne HDL-32E: range and lifecycle "not yet checked" to "not stated"; new price to "discontinued".
- RoboSense Helios-32: range "110 m; 150 m max (conflict)" to "110 m; 80 m at RobotShop (conflict)": a RobotShop snippet for the RS-Helios-1615 gives 150 m with 80 m at 10%, worded "NIST" (https://robotshop.com/products/robosense-lidar-rs-helios-1615-150-m-us); Alibaba URL moved to the .co.uk listing, same US $1,630.
- RoboSense RS-LiDAR-16: range to "not stated; 150 m maximum" (https://www.robosense.ai/en/news-show-1482); new to about 4,128 EUR inc. tax (https://generationrobots.com/blog/?p=6135); used to 523.91 EUR, pairing implied (https://www.ebay.de/itm/316241231630).
- RoboSense RS-LiDAR-32: range, new price, and lifecycle to "not stated".

**Listings seen:**

| Model                 | Asking price                            | Condition claim                       | Region        | Listing                                                                                 | Seen       |
| --------------------- | --------------------------------------- | ------------------------------------- | ------------- | --------------------------------------------------------------------------------------- | ---------- |
| Hesai Pandar40P       | US $149.50 (was US $299)                | with warranty, new or used not stated | not shown     | https://www.ebay.com/usr/protechsupply                                                  | 2026-10-11 |
| Hesai Pandar40P       | US $550 (about 467 EUR)                 | used, 90-day warranty, four available | Michigan, USA | https://www.ebay.de/itm/365679803957                                                    | 2026-10-11 |
| Hesai XT32            | 1,750 EUR, offers accepted              | with original accessories, no returns | Germany       | https://www.ebay.de/itm/406739554237                                                    | 2026-10-11 |
| Hesai PandarXT32      | US $2,138.07                            | new                                   | not shown     | https://www.ebay.com/str/nebulaautomation/Industrial-Automation-Motion-Controls/_i.html | 2026-10-11 |
| Velodyne VLP-32C      | about US $399.99, no longer available   | used, no accessories                  | Ohio, USA     | https://www.ebay.de/itm/176310211757                                                    | 2026-10-11 |
| Velodyne VLP-32C      | 400 GBP, sold 10 August, year not shown | used, working                         | UK            | https://www.ebay.de/itm/396899887588                                                    | 2026-10-11 |
| Velodyne VLP-32C      | US $1,198                               | upgraded bundle with GNSS             | not shown     | https://www.ebay.com/str/cezday                                                         | 2026-10-11 |
| RoboSense RS-LiDAR-16 | 523.91 EUR, pairing implied             | used, "99% new", ten available        | Shenzhen      | https://www.ebay.de/itm/316241231630                                                    | 2026-10-11 |
| RoboSense RS-LiDAR-16 | US $541.01, pairing implied             | used, three available                 | Shenzhen      | https://www.ebay.de/itm/157485527025                                                    | 2026-10-11 |
| RoboSense RS-LiDAR-16 | US $395, sold                           | used                                  | Hefei         | https://www.ebay.de/itm/355618346574                                                    | 2026-10-11 |

The RS-LiDAR-16 sold at US $395 is under budget but the model has no vendor 10% figure, so it is
not shortlisted. No used Pandar64, Pandar20, HDL-32E, RS-LiDAR-32, Helios, OS1, or OS2 listing was
found on eBay; MyBotShop still lists a used OS1-128 Rev 6 demo unit without a price in the snippet
(https://www.mybotshop.de/Ouster-OS1-128-Gen-2-Rev-6-Demo-Unit-Used_2).

**Announcements:**

- No Pandar40P end-of-life notice found; the Pandar40P is still in the Pandar column of the Hesai SDK 2.0 model table, opened this run: https://github.com/HesaiTechnology/HesaiLidar_SDK_2.0
- Hesai Q2 2026 results analysis: average unit price below RMB 1,300, down 35% year on year, driven by automotive ATX/FTX units; no spinning-unit price change or discontinuation: https://longportapp.cn/news/296284832
- No new 100 m-class 360° spinning unit found since 2026-10-05.

**Coverage gaps:**

- Every vendor, distributor, and eBay page failed to resolve; all prices are snippets.
- Not re-checked this run: Helios-16P and Ruby Plus. Searched without new figures: OS2, OS1 Max, and OS1 prices.
- Pandar64 range rests on a snippet of Hesai's comparison page; no datasheet was opened.
- The Pandar40P US $149.50 price is on a storefront page and may be new stock; condition not stated.
- Helios range now has three figures (110 m, 90 m for the 16P, 80 m at RobotShop); unreconciled.

### 2026-10-05

Provenance: the proxy returned search snippets only; no vendor, distributor, or resale page was opened,
so every figure below is a snippet of the cited URL. Range figures are at 10% reflectivity. The
spec rows of the 2026-09-19 entry are unchanged and were not re-verified.

**New units:** no new 100 m-class spinning unit found since 2026-09-19. Prices newly seen:

| Model              | Channels | Range at 10%                                                                                  | Vertical FOV                                                                                         | IMU                                           | Weight        | Power      | Price                                                   | Source                                                                                                                                                                    |
| ------------------ | -------- | --------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------- | ------------- | ---------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hesai Pandar40P    | 200 m    | not stated                                                                                    | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai XT32 (below) | 32       | 120 m maximum, 80 m at 10%                                                                    | not stated                                                                                           | not stated                                    | not stated    | not stated | US $2,138.07 new on eBay                                | https://www.ebay.de/itm/388641059791                                                                                                                                      |
| Hesai XT32M2X      | 80 m     | US $10,499, snippet: [Openelab](https://openelab.com/products/hesai-xt32m2x-32-channel-lidar) | none seen                                                                                            | Current                                       | Below range   | 2026-10-11 |
| RoboSense Helios   | 16 or 32 | 150 m maximum, 10% figure not stated                                                          | not stated                                                                                           | not stated                                    | not stated    | not stated | US $1,630 to US $2,590 (Alibaba), US $4,786 (RobotShop) | https://www.alibaba.fr/product-detail/Helios-RS-Helios16-32-Robosense-LiDAR_1601757765596.html and https://robotshop.com/products/robosense-lidar-rs-helios-1615-150-m-us |

**Used listings** (snippet prices; qualifying means range rule met and at or under US $450):

| Model | Asking price | Condition claim | Region | Listing | Seen |
| -------------------- | -------------- | --------------- | ---------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ------------- | ---------- |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Velodyne VLP-32C | not stated | discontinued | US $1,198, upgraded bundle with GNSS, snippet: [eBay store](https://www.ebay.com/str/cezday) | Support and repairs ended December 2024 | Unverified | 2026-10-11 |
| Velodyne VLP-16 | not stated | discontinued | about US $399, snippet: [eBay](https://www.ebay.com/itm/305167420502) | Technical support to end of 2026 | Unverified | 2026-10-11 |
| Ouster OS1-128 Rev 6 | not in snippet | demo unit, used | Germany | https://www.mybotshop.de/Ouster-OS1-128-Gen-2-Rev-6-Demo-Unit-Used_1 | 2026-10-05 |

No used Pandar64, Pandar20, HDL-32E, RS-LiDAR-16, RS-LiDAR-32, or Ouster OS2 listing was found in the snippets.

**Announcements:**

- No Pandar40P end-of-life notice or SDK removal found; Hesai news page: https://www.hesaitech.com/news/
- No discontinuation, price change, or new 100 m-class spinning unit announced in the past week found: https://www.cmcmarkets.com/en-gb/opto/lidar-sector-roundup-ousters-physical-ai-bet

**Coverage gaps:**

- All pages were blocked or not opened; every figure is a search snippet.
- Used prices could not be tied to a single item; several snippets give no price and no region.
- Helios 150 m maximum versus the earlier 110 m figure is unreconciled; no 10% figure seen.
- Pandar40P lifecycle rests on the absence of a notice in search results.

### 2026-09-19

Provenance: the research session's proxy blocked every vendor, distributor, press, and resale page,
so each figure below is the search-engine snippet of the cited URL, not a page that was opened.
Confirm a figure before it enters a decision. Range figures are at 10% reflectivity.

**New units** (meet the rule unless noted):

| Model                           | Channels                            | Range at 10%                                                                                                                              | Vertical FOV                                                       | IMU                                                          | Weight      | Power        | Price                                                                                          | Source                                                                                                                                                                                                         |
| ------------------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------ | ----------- | ------------ | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hesai Pandar40P (in service)    | 40                                  | 200 m                                                                                                                                     | 40°                                                                | not stated                                                   | 1.52 kg     | 18 W typical | not stated; still listed in the Hesai SDK 2.0 compatibility table; no end-of-life notice found | https://www.manualslib.com/manual/1551481/Hesai-Pandar40p.html?page=7 and https://github.com/HesaiTechnology/HesaiLidar_SDK_2.0                                                                                |
| Hesai Pandar128E3X              | 200 m                               | not stated                                                                                                                                | none seen                                                          | Current; in SDK 2.0 table                                    | Over budget | 2026-10-11   |
| Hesai OT128                     | 200 m                               | 7,200 EUR, listing title: [quadruped.de](https://www.quadruped.de/Hesai-OT128_1)                                                          | none seen                                                          | Current                                                      | Over budget | 2026-10-11   |
| Ouster OS2 Rev 7                | 32, 64, or 128                      | 200 m                                                                                                                                     | 22.5°                                                              | yes; ICM-20948 in an earlier revision, Rev 7 part not stated | 930 g       | 2026-10-11   | 5,850 EUR ex VAT for OS2-128                                                                   | https://data.ouster.io/downloads/datasheets/datasheet-rev7-v2p5-os2.pdf and https://www.ghostysky.com/home/shop-ouster-lidars/                                                                                 |
| Ouster OS1 Max Rev 8 (May 2026) | 128 or 256                          | 200 m                                                                                                                                     | 45° in the press release, 43.9° in the datasheet                   | not stated                                                   | not stated  | not stated   | not stated                                                                                     | https://investors.ouster.com/news-releases/news-release-details/ouster-releases-rev8-os-family-worlds-first-native-color-lidar and https://data.ouster.io/downloads/datasheets/datasheet-rev8-v4p0-os1-max.pdf |
| RoboSense Helios-32             | 110 m; 80 m at RobotShop (conflict) | US $1,630, snippet, Alibaba: [Alibaba](https://www.alibaba.co.uk/product-detail/Helios-RS-Helios16-32-Robosense-LiDAR_1601757765596.html) | no price in snippet: [eBay](https://www.ebay.com/itm/157485526988) | End of life at one distributor                               | Unverified  | 2026-10-11   |
| RoboSense Helios-16P            | 16                                  | 110 m in the store, 90 m in the manual (open conflict)                                                                                    | 30°                                                                | not stated                                                   | about 1 kg  | 11 W typical | not stated                                                                                     | https://store.robosense.ai/products/helios-series and https://cdn.robosense.cn/20220629181832_42540.pdf                                                                                                        |
| RoboSense Ruby Plus             | 128                                 | 240 m                                                                                                                                     | 40°                                                                | not stated                                                   | not stated  | 30 W         | not stated                                                                                     | https://www.robosense.ai/en/news-show-1637                                                                                                                                                                     |

Below the rule on the vendors' own numbers, so excluded:

- Hesai XT32 (80 m at 10%), XT32M2X (80 m at 10%, 0.49 kg, 10 W, US $6,900 in a store snippet), and XT16 (80 m): https://www.hesaitech.com/product/xt16-32-32m/ and https://store.kartaview.org/products/hesai-xt32m2x
- Ouster OS1 Rev 7 and Rev 8 (90 m at 10%, IMU IAM-20680HT, 455 g, 14 to 20 W): https://data.ouster.io/downloads/datasheets/datasheet-rev7-v3p1-os1.pdf
- RoboSense Airy (30 m), Livox Mid-360S (38 m), LSLiDAR C32W (50 m) and CH32R (30 m): https://openelab.io/products/robosense-airy-96-channel-hemispherical-digital-lidar and https://www.livoxtech.com/mid-360s and https://www.lslidar.com/lslidars-ch32r-c32w-lidars/
- Velodyne VLP-16 and VLP-32C state no 10% figure and are discontinued; LSLiDAR C32 states 150 m maximum and no 10% figure: https://ouster.com/products/hardware/vlp-16 and https://data.ouster.io/downloads/datasheets/velodyne/63-9378_Rev-F_Ultra-Puck_Datasheet_Web.pdf and https://www.lslidar.com/product/c32-16-mechanical-lidar/
- Seyond sells no 360° spinning unit: https://seyond.com/seyond-to-showcase-complete-end-to-end-lidar-portfolio-and-mass-production-ready-solid-state-lidar-at-ces-2026/

**Used listings** (asking prices as shown in the snippet; "pairing implied" means the snippet
summarised several results, so the price may belong to a neighbouring item):

| Model | Asking price | Condition claim | Region | Listing | Seen |
| ---------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- | --------------------------------------------- | ------------- | ---------- |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai Pandar40P | 200 m | not stated | US $149.50, snippet, condition not stated: [eBay storefront](https://www.ebay.com/usr/protechsupply) | Legacy; in SDK 2.0 table; no end-of-life seen | **Qualifies** | 2026-10-11 |
| Hesai XT32 | 80 m | US $1,400, snippet, backordered: [Robotics Center](https://www.roboticscenter.ai/ar/store/product/hesai-technology-hesai-xt32) | 1,750 EUR, snippet, condition not stated: [eBay.de](https://www.ebay.de/itm/406739554237) | Current | Below range | 2026-10-11 |
| Ouster OS1-32 | US $2,199 best offer, sold | used once | Tempe, AZ | https://www.ebay.com/itm/197711710449 | 2026-09-19 |
| Ouster OS1-32-U | US $4,900 | used | Shenzhen | https://www.ebay.com/itm/397193528270 | 2026-09-19 |
| Ouster OS2-32-U | US $6,000, listing ended | new, open box | not shown | https://www.ebay.com/itm/156943834624 | 2026-09-19 |
| Velodyne VLP-32C | not stated | discontinued | US $1,198, upgraded bundle with GNSS, snippet: [eBay store](https://www.ebay.com/str/cezday) | Support and repairs ended December 2024 | Unverified | 2026-10-11 |
| Velodyne VLP-16 | not stated | discontinued | about US $399, snippet: [eBay](https://www.ebay.com/itm/305167420502) | Technical support to end of 2026 | Unverified | 2026-10-11 |
| RoboSense Helios | not in snippet | used, cosmetic wear | Shenzhen | https://www.ebay.com/itm/157485526988 | 2026-09-19 |

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
