# LiDAR market watch

- **Status:** Active
- **Cadence:** Weekly, by a scheduled Claude routine; entries by hand are welcome too
- **Related plan:** [lidar-backpack-capture-plan.md](../../plans/lidar-backpack-capture-plan.md)

A running log of spinning LiDAR units that could replace or supplement the Hesai Pandar40P for
backpack and tripod survey capture. The Pandar40P is discontinued, so supply is the used market,
and the project needs to know when a current unit meets the selection rule at an activist's price.
Sensor facts for the unit in service live in the
[hardware knowledge module](../../../.github/knowledge/hardware.md).

## Selection rule

| Requirement               | Value                                           | Why                                                                       |
| ------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------- |
| Range at 10% reflectivity | 100 m or more                                   | Junction approaches and speeds at range; excludes the Livox Mid-360 class |
| Horizontal coverage       | 360°, multi-ring spinning                       | The L3 background model is a ring-by-azimuth range image                  |
| Interface                 | Ethernet UDP point packets                      | L1 parses UDP; PCAP is the capture artefact                               |
| Built-in IMU              | Preferred, not required                         | Tier 1 of the plan's hardware ladder without a bracket                    |
| Weight and power          | Stated by the vendor                            | Backpack carried, battery powered                                         |
| Price                     | New list price or used asking price, with a URL | Only prices a page actually states                                        |

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

First snapshot pending.
