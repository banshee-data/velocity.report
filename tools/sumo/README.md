# SUMO comparison tools

Tools for comparing a published scene's measured traffic with the
[SUMO](https://eclipse.dev/sumo/) traffic simulator. The first comparison, at Columbus Avenue
and Broadway, and what it found are in the
[report](../../docs/lidar/operations/sumo-columbus-broadway-headways-2026-10.md). What it would
take to publish a SUMO-ready card with each survey is in the
[demand card plan](../../docs/plans/lidar-sumo-demand-card-plan.md).

| File                                       | What it is                                                                                    |
| ------------------------------------------ | --------------------------------------------------------------------------------------------- |
| `lidar-counting-line.py`                   | Counting-line headways from a published scene's tracks (`modes`, `profile`, `headways`)       |
| `columbus-broadway/build-net.sh`           | Builds `colbway.net.xml` from an OpenStreetMap extract                                        |
| `columbus-broadway/sf25.typ.xml`           | 25 mph for San Francisco roads OSM gives no limit                                             |
| `columbus-broadway/routes.rou.xml`         | Demand and one vehicle type per car-following model, with shared parameters                   |
| `columbus-broadway/counting-lines.add.xml` | Per-vehicle counting lines on Columbus north-west-bound: stop line and 20 m past the junction |
| `columbus-broadway/colbway.sumocfg`        | Network, one hour plus a 300 s tail at 0.1 s steps, seed 42                                   |
| `columbus-broadway/colbway.view.xml`       | sumo-gui view centred on the junction                                                         |
| `columbus-broadway/compare.sh`             | Runs each model on the same demand and prints per-lane headways                               |
| `columbus-broadway/headways.py`            | Per-lane headway summary of a SUMO counting-line output                                       |

`colbway.net.xml` and `out/` are generated and ignored.

## Requirements

SUMO 1.20 or later with `SUMO_HOME` set (the comparison used a source build of 1.27.1+679). A
Homebrew install works with `SUMO_HOME=/opt/homebrew/share/sumo`. Python 3.11 or later, standard
library only.

## Measured side

From the repository root, on the published assets in `public_html/src/scenes/<site>/assets`:

```bash
python3 tools/sumo/lidar-counting-line.py modes --centres 103 152 282 331
python3 tools/sumo/lidar-counting-line.py profile --bearing 103.3
python3 tools/sumo/lidar-counting-line.py headways --bearing 103.3 --lines -2 10 25
```

`modes` finds the travel directions in the sensor frame; `profile` shows, for one direction,
where its vehicles stop and how they split across lanes; `headways` reads off the counting
lines. The bearing, lane split and line positions are site-specific and come from the first two.

## Simulated side

Any OpenStreetMap extract covering the junction will do. The comparison used a 2024 Overpass
extract of downtown San Francisco; SUMO's `osmGet.py` fetches a fresh one, after which the edge
IDs in `routes.rou.xml` and `counting-lines.add.xml` may need updating.

```bash
python3 "$SUMO_HOME/tools/osmGet.py" --bbox -122.4110,37.7955,-122.4020,37.8010 --prefix colbway
tools/sumo/columbus-broadway/build-net.sh colbway_bbox.osm.xml
tools/sumo/columbus-broadway/compare.sh krauss idm eidm
```

To watch a model, run `compare.sh` first, then from `columbus-broadway/`:

```bash
sumo-gui -c colbway.sumocfg -g colbway.view.xml -r out/routes.idm.rou.xml -a out/counting-lines.idm.add.xml --output-prefix gui-
```

`--output-prefix gui-` keeps the window's detector output apart from `compare.sh`'s; without it a
window left open overwrites the comparison's files.
