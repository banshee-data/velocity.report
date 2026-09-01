import assert from "node:assert/strict";
import test from "node:test";

import { parseArguments } from "../cli.mjs";
import {
  buildS2HilbertModel,
  idNamespace,
  renderOrientationLegendSvg,
  projectWebMercator,
  renderHilbertSvg,
  unwrapLongitude,
} from "../index.mjs";

test("Web Mercator keeps north up and unwraps local antimeridian geometry", () => {
  assert.ok(projectWebMercator({ lat: 40, lng: 0 }).y < projectWebMercator({ lat: 0, lng: 0 }).y);
  assert.equal(unwrapLongitude(-179, 179), 181);
  assert.equal(unwrapLongitude(179, -179), -181);
});

test("a custom viewBox controls SVG coordinates independently of physical size", () => {
  const model = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 12,
    width: 800,
    height: 600,
    padding: 10,
    viewBox: [100, 200, 400, 300],
  });
  assert.deepEqual(model.projection.viewBox, [100, 200, 400, 300]);
  for (const point of model.path) {
    assert.ok(point.x >= 110 && point.x <= 490);
    assert.ok(point.y >= 210 && point.y <= 490);
  }
});

test("SVG output retains semantic groups, classes, data, and directional markers", () => {
  const model = buildS2HilbertModel({ parent: "808581", targetLevel: 12 });
  const svg = renderHilbertSvg(model);
  assert.match(svg, /<g id="cells">/);
  assert.match(svg, /<g id="hilbert-path">/);
  assert.match(svg, /<g id="markers">/);
  assert.match(svg, /class="s2-cell"/);
  assert.match(svg, /class="hilbert-start"/);
  assert.match(svg, /class="hilbert-end"/);
  assert.match(svg, /marker-end="url\(#hilbert-arrow\)"/);
  assert.match(svg, /data-s2-token="808581"/);
  assert.equal((svg.match(/class="s2-cell"/g) ?? []).length, 16);
});

test("the CLI explicitly converts display tokens and parses geometry options", () => {
  assert.deepEqual(
    parseArguments([
      "--parent",
      "80858-1",
      "--target-level",
      "13",
      "--view-box",
      "0 0 1200 800",
      "--hide-cells",
      "--show-labels",
    ]),
    {
      width: 1000,
      height: 1000,
      padding: 40,
      showCells: false,
      showLabels: true,
      printIds: false,
      parent: "808581",
      targetLevel: 13,
      viewBox: [0, 0, 1200, 800],
    },
  );
});

const idsIn = (svg) => new Set([...svg.matchAll(/\sid="([^"]+)"/g)].map(([, id]) => id));

test("ids are bare by default, so a standalone file is unchanged", () => {
  const svg = renderHilbertSvg(buildS2HilbertModel({ parent: "808581", targetLevel: 12 }));
  assert.match(svg, /<title id="svg-title">/);
  assert.match(svg, /aria-labelledby="svg-title svg-description"/);
  assert.match(svg, /<marker id="hilbert-arrow"/);
});

test("idPrefix namespaces every generated id, references included", () => {
  const model = buildS2HilbertModel({ parent: "808581", targetLevel: 12 });
  const svg = renderHilbertSvg(model, { idPrefix: "fig3-" });

  for (const id of idsIn(svg)) {
    assert.ok(id.startsWith("fig3-"), `unprefixed id leaked: ${id}`);
  }
  // The two reference forms that actually break on a collision.
  assert.match(svg, /aria-labelledby="fig3-svg-title fig3-svg-description"/);
  assert.match(svg, /<marker id="fig3-hilbert-arrow"/);
  assert.match(svg, /marker-end="url\(#fig3-hilbert-arrow\)"/);
});

test("two prefixed renders share no ids, so both can be inlined at once", () => {
  const model = buildS2HilbertModel({ parent: "808581", targetLevel: 12 });
  const first = idsIn(renderHilbertSvg(model, { idPrefix: "a-" }));
  const second = idsIn(renderHilbertSvg(model, { idPrefix: "b-" }));
  assert.ok(first.size > 0);
  const shared = [...first].filter((id) => second.has(id));
  assert.deepEqual(shared, [], "ids must not collide between embedded copies");
});

test("the legend and composite take an idPrefix too", () => {
  const entries = ["808587", "808f7f", "808581", "808585"].map((parent, index) => ({
    orientation: index,
    label: "x",
    model: buildS2HilbertModel({
      parent,
      targetLevel: 12,
      width: 190,
      height: 190,
      padding: 13,
      viewBox: [0, 0, 190, 190],
    }),
  }));
  const legend = renderOrientationLegendSvg(entries, { idPrefix: "leg-" });
  for (const id of idsIn(legend)) assert.ok(id.startsWith("leg-"), id);
  assert.match(legend, /marker-end="url\(#leg-legend-arrow\)"/);
  // An explicit markerId still wins, for callers already passing one.
  assert.match(renderOrientationLegendSvg(entries, { markerId: "zz" }), /<marker id="zz"/);
});

test("idNamespace refuses a non-string prefix rather than stringifying it", () => {
  assert.equal(idNamespace()("x"), "x");
  assert.equal(idNamespace("p-")("x"), "p-x");
  assert.throws(() => idNamespace(7), /must be a string/);
});
