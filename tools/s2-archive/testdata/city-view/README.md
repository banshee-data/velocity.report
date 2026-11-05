# City view test fixtures

- `tileset.json`: Gerardus's committed city-tiles fixture,
  `tests/fixtures/city-view/tileset.json` at banshee-data/gerardus `c4df07f` (PR 71, format
  `gerardus-city-tiles-1`), copied unchanged but for a final newline. Its root transform declares the city frame and
  each tile's `extras.origin_city_enu_m` gives that cell centre in it.
- `gerardus-sites-columbus-broadway.json`: one entry of Gerardus's `GET /api/v1/sites` on
  2026-11-05: the columbus-broadway site frame's origin in WGS 84 at the stated epoch.

`tools/s2-archive/test_city_frame.py` and `tools/s2-hilbert/test/city-sites.test.mjs` check the
placement arithmetic against both. Replace them from Gerardus only with a new format.
