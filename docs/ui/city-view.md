# City view

- **Status:** Design, partly built. In the visualiser, switching between the sites of
  `city/sites.json` ships (toolbar site picker; Settings ▸ Servers ▸ City view). The city map
  itself does not yet.
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
- **Street names:** the City's own names, as labels for finding your way around.
- **Sites:** one marker per site with data, with its recording date. A site is placed in the city
  only by an accepted Gerardus pose. A site without one is a marker labelled "not yet placed".
- **Playing sites:** sites in view play in place, simplified with distance. Each plays only inside
  its own coverage, on its own clock and date, coloured by class and never by track. Each
  intersection is recorded on its own. Vehicles are not followed from one intersection to the next.

None of it is a plan, survey or specification of a street, and a feature that is not drawn is not
asserted absent. See [Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Files it reads

Everything is static, and there are no city-view endpoints:

- the Gerardus city tiles: a `tileset.json` and one `.glb` per tile;
- `city/sites.json`: every site with data and its placement;
- each scene's `assets/placement.json`;
- for the visualiser's live and replay data, the existing velocity.report calls.

The plan gives the frames, the level-of-detail rules and the open questions.
