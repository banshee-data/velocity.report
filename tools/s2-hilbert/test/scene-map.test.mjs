import assert from "node:assert/strict";
import test from "node:test";

import { buildSceneMapModel, deriveSiteCells, renderSceneMapSvg } from "../scene-map.mjs";
import { s2 } from "../hierarchy.mjs";

const BROADWAY = {
  id: "broadway-columbus",
  title: "Broadway & Columbus",
  page: "/scenes/broadway-columbus/",
  position: { lat: 37.7987, lon: -122.4073, source: "operator" },
};

const UNLOCATED = {
  id: "soma1",
  title: "SoMa 1",
  page: "/scenes/soma1/",
  position: null,
};

test("derives the same S2 family the Go index derives", () => {
  // These are the tokens internal/lidar/geoindex produced for this position;
  // two independent S2 implementations agreeing is the point of pinning them.
  const { tokens } = deriveSiteCells(BROADWAY);
  assert.equal(tokens.area.token, "808581");
  assert.equal(tokens.neighbourhood.token, "808580f4");
  assert.equal(tokens.site.token, "808580f3f");
  assert.equal(tokens.neighbourhood.display, "80858-0f4");
});

test("takes the hierarchy from CellID parents, not from token characters", () => {
  const { tokens } = deriveSiteCells(BROADWAY);
  const site = s2.cellid.fromToken(tokens.site.token);
  assert.equal(s2.cellid.toToken(s2.cellid.parent(site, 13)), tokens.neighbourhood.token);
  assert.equal(s2.cellid.toToken(s2.cellid.parent(site, 10)), tokens.area.token);
  // Truncating the L16 token to the L13 token's length gives a different
  // string, which is exactly why truncation is not allowed to stand in for it.
  assert.notEqual(tokens.site.token.slice(0, tokens.neighbourhood.token.length), tokens.neighbourhood.token);
});

test("refuses a position that is not on Earth", () => {
  assert.throws(
    () => deriveSiteCells({ id: "bad", position: { lat: 91, lon: 0 } }),
    /not on Earth/,
  );
  assert.throws(
    () => deriveSiteCells({ id: "bad", position: { lat: "37.8", lon: 0 } }),
    /finite numbers/,
  );
});

test("carries an unlocated site through instead of dropping or placing it", () => {
  const model = buildSceneMapModel({ sites: [UNLOCATED, BROADWAY] });
  assert.equal(model.sites.length, 2);
  assert.equal(model.located.length, 1);
  const soma = model.sites.find((site) => site.id === "soma1");
  assert.equal(soma.located, false);
  assert.equal(soma.cells, null);
});

test("draws the neighbourhood cell inside its area cell", () => {
  const model = buildSceneMapModel({ sites: [BROADWAY] });
  const bounds = (points) => {
    const pairs = points.split(" ").map((pair) => pair.split(",").map(Number));
    return {
      minX: Math.min(...pairs.map(([x]) => x)),
      maxX: Math.max(...pairs.map(([x]) => x)),
      minY: Math.min(...pairs.map(([, y]) => y)),
      maxY: Math.max(...pairs.map(([, y]) => y)),
    };
  };
  const area = bounds(model.map.areaCells[0].points);
  const cell = bounds(model.map.neighbourhoodCells[0].points);
  assert.ok(cell.minX >= area.minX && cell.maxX <= area.maxX, "cell must sit within its area in x");
  assert.ok(cell.minY >= area.minY && cell.maxY <= area.maxY, "cell must sit within its area in y");

  // Three levels of subdivision is eight cells per side, so the drawn cell
  // should be about an eighth of the area's width. A projection or fit error
  // shows up here long before anyone squints at the SVG.
  const ratio = (cell.maxX - cell.minX) / (area.maxX - area.minX);
  assert.ok(ratio > 0.1 && ratio < 0.15, `expected about an eighth, got ${ratio}`);
});

test("places the marker inside the cell that indexes it", () => {
  const model = buildSceneMapModel({ sites: [BROADWAY] });
  const [marker] = model.map.markers;
  const pairs = model.map.neighbourhoodCells[0].points
    .split(" ")
    .map((pair) => pair.split(",").map(Number));
  const minX = Math.min(...pairs.map(([x]) => x));
  const maxX = Math.max(...pairs.map(([x]) => x));
  const minY = Math.min(...pairs.map(([, y]) => y));
  const maxY = Math.max(...pairs.map(([, y]) => y));
  assert.ok(marker.x >= minX && marker.x <= maxX, "marker x outside its own cell");
  assert.ok(marker.y >= minY && marker.y <= maxY, "marker y outside its own cell");
});

test("scales the bar at the map's latitude, not the equator", () => {
  const model = buildSceneMapModel({ sites: [BROADWAY] });
  const { scaleBar, areaCells } = model.map;
  const pairs = areaCells[0].points.split(" ").map((pair) => pair.split(",").map(Number));
  const widthSvg = Math.max(...pairs.map(([x]) => x)) - Math.min(...pairs.map(([x]) => x));
  const widthMetres = (widthSvg / scaleBar.length) * scaleBar.metres;
  // An L10 cell is roughly 8 km across at San Francisco's latitude. Measuring
  // at the equator instead would inflate this by about a quarter.
  assert.ok(widthMetres > 6500 && widthMetres < 9500, `area cell measured ${widthMetres} m`);
});

test("escapes markup in a site name", () => {
  const svg = renderSceneMapSvg(buildSceneMapModel({ sites: [BROADWAY] }));
  assert.ok(svg.includes("Broadway &amp; Columbus"));
  assert.ok(!/Broadway & Columbus/.test(svg), "a bare ampersand would be invalid XML");
});

test("keeps scene markers hero green in light and dark mode", () => {
  const svg = renderSceneMapSvg(buildSceneMapModel({ sites: [BROADWAY] }));
  const markerRules = svg.match(/scene-map__marker \{ fill: #10b981/g) ?? [];
  assert.equal(markerRules.length, 2);
});

test("renders an honest empty state when nothing has a position", () => {
  const model = buildSceneMapModel({ sites: [UNLOCATED] });
  assert.equal(model.map, null);
  const svg = renderSceneMapSvg(model);
  assert.ok(svg.startsWith("<svg"));
  assert.ok(svg.includes("nothing to place"), "the empty state should say why it is empty");
  assert.ok(!svg.includes("<polygon"), "nothing may be drawn for a site with no position");
});
