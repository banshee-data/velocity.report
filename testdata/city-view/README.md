# City view test fixtures

Shared by the macOS visualiser's tests and the web city view (lidar-city-view-plan Items 3 and
5–8). Copied fixtures, not experiment output.

| Path                                             | What it is                                                 |
| ------------------------------------------------ | ---------------------------------------------------------- |
| `tiles/`                                         | Gerardus's city-tiles fixture: 19 tiles over two L13 cells |
| `sites.json`                                     | Three entries of `city/sites.json`: two placed, one not    |
| `scenes/columbus-broadway/assets/placement.json` | One scene's placement sidecar                              |

`tiles/` is copied from Gerardus `tests/fixtures/city-view/` at commit `c4df07f`
(banshee-data/gerardus PR 71, format `gerardus-city-tiles-1`). Its `manifest.json` carries the
sources, licences and data purpose (the only change is a final newline on the two JSON files, which the
repository's hooks add): the tiles are measured spatial data (the City's footprints and
centrelines, 3DEP ground) given as context for traffic data, not a survey or plan. Refresh it by
copying that directory again when the format changes, and record the new commit here.

`sites.json` and `placement.json` are copied from the published files that
`tools/s2-hilbert/generate-scene-map.mjs` and `tools/s2-archive/publish-scenes.py` write
(`public_html/src/city/sites.json`, `public_html/src/scenes/<id>/assets/placement.json`; plan
Item 1). They hold placement and provenance only. The format is in
[city-view.md](../../docs/ui/city-view.md).
