import assert from "node:assert/strict";
import test from "node:test";

import { parseArguments } from "../cli.mjs";
import {
  buildS2HilbertModel,
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
