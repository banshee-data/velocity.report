# City view

- **Status:** Design, not built. This page describes the intended behaviour; nothing here ships yet.
- **Plan:** [lidar-city-view-plan.md](../plans/lidar-city-view-plan.md) (the work and its order)
- **Companion:** Gerardus
  [CITY-VIEW-TILES](https://github.com/banshee-data/gerardus/blob/main/docs/plans/CITY-VIEW-TILES.md)
  (the city tiles)

A whole-city view, in the macOS visualiser and on the public scene site. It shows the street grid,
the buildings and every site with data. A reader zooms into an intersection to play its recording,
and moves to the next site with data while the city stays drawn around it.

## What it draws

- **City tiles:** the City's building footprints with their 2010 heights, the City's street
  centrelines and 3DEP ground. They are measured spatial data at each source's acquisition time,
  drawn as context for the traffic, at three levels of detail.
- **Sites:** one marker per site with data. A marker shows whether the site's placement is an
  accepted Gerardus pose or an approximate eye-read position.
- **One playing site:** the recording of the site the reader has zoomed into, placed in the city by
  its placement. Other sites do not play at the same time.

None of it is a plan, survey or specification of a street, and a feature that is not drawn is not
asserted absent. See [Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Files it reads

Everything is static, and there are no city-view endpoints:

- the Gerardus city tiles: a `tileset.json` and one `.glb` per tile;
- `city/sites.json`: every site with data and its placement;
- each scene's `assets/placement.json`;
- for the visualiser's live and replay data, the existing velocity.report calls.

The plan gives the frames, the level-of-detail rules and the open questions.
