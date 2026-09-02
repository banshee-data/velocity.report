import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { COMPOSITE_RENDER_OPTIONS } from "../generate-composite.mjs";
import {
  buildS2CompositeModel,
  cellSlantDegrees,
  compassBearing,
  compositeAdjacency,
  orientationName,
  renderCompositeSvg,
  snapToCellAxis,
} from "../index.mjs";

const SELECTION_URL = new URL(
  "../selection/80858-1-l13-cells.json",
  import.meta.url,
);
const GENERATED = new URL("../generated/", import.meta.url);
const QUAD = ["808581", "808587", "808f7d", "808f7f"];

const loadSelection = async () =>
  JSON.parse(await readFile(SELECTION_URL, "utf8"));
const heroModel = async () =>
  quadModel([
    {
      parent: "808581",
      role: "hero",
      cellSelection: await loadSelection(),
      showOffMapContinuations: true,
    },
    { parent: "808587", showOffMapContinuations: true },
    { parent: "808f7d" },
    { parent: "808f7f" },
  ]);
const quadModel = (panels) =>
  buildS2CompositeModel({ width: 1400, height: 1400, padding: 56, panels });

test("orientation names come from the S2 cell, not from a lookup by token", () => {
  assert.equal(orientationName("808587"), "canonical");
  assert.equal(orientationName("808f7f"), "swapped");
  assert.equal(orientationName("808581"), "inverted");
  assert.equal(orientationName("808f7d"), "inverted");
  assert.equal(orientationName("808585"), "swapped + inverted");
});

test("the four L10 cells tile: four shared edges and two shared corners", () => {
  const adjacency = compositeAdjacency(
    quadModel(QUAD.map((parent) => ({ parent }))),
  );
  assert.equal(adjacency.filter((pair) => pair.relation === "edge").length, 4);
  assert.equal(
    adjacency.filter((pair) => pair.relation === "corner").length,
    2,
  );
});

test("one shared projection places the panels, so neighbours actually join", () => {
  const model = quadModel(QUAD.map((parent) => ({ parent })));
  // A shared fit means every panel reports the same scale and viewBox.
  const scales = new Set(model.panels.map(() => model.projection.scale));
  assert.equal(scales.size, 1);

  // 808581 and 808587 share an edge, so two of their drawn vertices coincide.
  const round = (vertex) => `${vertex.x.toFixed(6)},${vertex.y.toFixed(6)}`;
  const first = new Set(
    model.panels
      .find((p) => p.parent.token === "808581")
      .parent.vertices.map(round),
  );
  const shared = model.panels
    .find((p) => p.parent.token === "808587")
    .parent.vertices.filter((vertex) => first.has(round(vertex)));
  assert.equal(shared.length, 2, "shared edge survives projection");
});

test("each panel carries both a 16-step and a 64-step traversal", () => {
  for (const panel of quadModel(QUAD.map((parent) => ({ parent }))).panels) {
    assert.equal(panel.coarse.path.length, 16, panel.parent.token);
    assert.equal(panel.detail.path.length, 64, panel.parent.token);
  }
});

test("the hero panel resolves its selection to real tiles inside its own parent", async () => {
  const cellSelection = await loadSelection();
  const model = quadModel([
    { parent: "808581", role: "hero", cellSelection },
    { parent: "808587" },
    { parent: "808f7d" },
    { parent: "808f7f" },
  ]);
  const hero = model.panels[0];
  const enabled = cellSelection.filter((cell) => cell.enabled).length;

  assert.equal(hero.role, "hero");
  assert.equal(hero.selectedCount, enabled);
  assert.equal(hero.selectedCells.length, enabled);
  for (const cell of hero.selectedCells) {
    assert.equal(cell.level, 13);
    assert.ok(cell.token.startsWith("80858"), cell.token);
  }
  // Context panels carry no selection at all.
  for (const panel of model.panels.slice(1))
    assert.equal(panel.selectedCount, null);
});

test("a selection naming cells outside the panel's parent is refused", () => {
  assert.throws(
    () =>
      quadModel([
        {
          parent: "808581",
          role: "hero",
          cellSelection: [{ token: "808f7f01", enabled: true }],
        },
      ]),
    /not L13 descendants/,
  );
});

test("orientation captions are off by default but the metadata survives", async () => {
  const svg = renderCompositeSvg(await heroModel());
  assert.match(svg, /data-role="hero"/);
  assert.equal((svg.match(/data-role="context"/g) ?? []).length, 3);
  // No printed captions.
  assert.equal((svg.match(/<text class="panel-label/g) ?? []).length, 0);
  assert.doesNotMatch(svg, />canonical</);
  assert.doesNotMatch(svg, />swapped</);
  assert.doesNotMatch(svg, />inverted</);
  // The orientation is still recorded on each panel for anyone reading the file.
  assert.match(svg, /data-orientation-label="canonical"/);
  assert.match(svg, /data-orientation-label="swapped"/);
  assert.equal(
    (svg.match(/data-orientation-label="inverted"/g) ?? []).length,
    2,
  );
  // 15 per panel, plus one for the single link and one for each of the 6 stubs.
  assert.equal(
    (svg.match(/<polyline class="composite-chevron/g) ?? []).length,
    15 * 4 + 7,
  );
});

test("captions can be switched back on, centred and set on the cell's slant", async () => {
  const model = quadModel([
    {
      parent: "808581",
      role: "hero",
      showLabel: true,
      cellSelection: await loadSelection(),
    },
    { parent: "808587", showLabel: true },
    { parent: "808f7d", showLabel: true },
    { parent: "808f7f", showLabel: true },
  ]);
  const svg = renderCompositeSvg(model);
  const rotations = [
    ...svg.matchAll(/class="panel-label[^"]*"[^>]*rotate\((-?[\d.]+)\)"/g),
  ];
  assert.equal(rotations.length, 4);
  for (const [, angle] of rotations) {
    assert.notEqual(Number(angle), 0, "labels follow the shear");
    assert.ok(Math.abs(Number(angle)) < 45, `unexpected label angle ${angle}`);
  }
});

test("the label angle tracks the cell's own top edge", () => {
  // A square cell has a level top edge; a sheared one does not.
  assert.equal(
    cellSlantDegrees([
      { x: 0, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
    ]),
    0,
  );
  const sheared = cellSlantDegrees([
    { x: 0, y: 0 },
    { x: 10, y: 5 },
    { x: 10, y: 15 },
    { x: 0, y: 10 },
  ]);
  assert.ok(
    sheared > 0 && sheared < 45,
    `expected a positive slant, got ${sheared}`,
  );
});

test("every panel shows its full detail run, weighted only where selected", async () => {
  const model = await heroModel();
  const svg = renderCompositeSvg(model);
  const [hero, ...context] = model.panels;

  // The hero splits its 63 detail segments into heavy and light runs.
  assert.equal(hero.detail.classifications.length, 63);
  const heavy = hero.detail.classifications.filter((c) => c === "selected").length;
  assert.ok(heavy > 0 && heavy < 63);
  assert.match(svg, /<path class="hilbert-detail hilbert-detail-heavy"/);
  // Context panels draw the same 64-cell run, light throughout.
  for (const panel of context) assert.equal(panel.detail.classifications, null);
  assert.equal(
    (svg.match(/<path class="hilbert-detail hilbert-detail-light"/g) ?? [])
      .length,
    4,
  );
  // One brown at one weight across all four cells: only the blue run varies.
  assert.equal((svg.match(/<path class="hilbert-coarse"/g) ?? []).length, 4);
  assert.doesNotMatch(svg, /hilbert-coarse-light/);
});

test("the detail run is drawn through cell centres, not around their edges", async () => {
  const model = await heroModel();
  for (const panel of model.panels) {
    for (const [index, point] of panel.detail.path.entries()) {
      const cell = panel.detail.cells[index];
      assert.equal(point.x, cell.centre.x);
      assert.equal(point.y, cell.centre.y);
      // The centre must sit well inside the cell, nowhere near a vertex. It is
      // the spherical centre, so it does not land exactly on the planar
      // centroid of the projected corners, and should not be asserted to.
      const nearestVertex = Math.min(
        ...cell.vertices.map((v) => Math.hypot(point.x - v.x, point.y - v.y)),
      );
      const cellSpan = Math.hypot(
        cell.vertices[0].x - cell.vertices[2].x,
        cell.vertices[0].y - cell.vertices[2].y,
      );
      assert.ok(nearestVertex > cellSpan * 0.3, "centre, not a vertex");
    }
  }
});

test("entries and exits come from the S2 curve, not from what looks close", async () => {
  const model = await heroModel();
  const { links, stubs } = model.connections;

  // 808f7d's successor really is 808f7f, so that is the one link on the page.
  assert.equal(links.length, 1);
  assert.deepEqual(
    [links[0].fromToken, links[0].toToken],
    ["808f7d", "808f7f"],
  );

  // Everything else leaves the mapped area. 808f7f's entry is not repeated as a
  // stub, because 808f7d's exit link already draws that joint.
  assert.equal(stubs.length, 6);
  const describe = (token, kind) => {
    const stub = stubs.find((s) => s.panelToken === token && s.kind === kind);
    return stub && `${stub.neighbourToken} ${stub.bearing}`;
  };
  assert.equal(
    describe("808581", "entry"),
    "80857f NE",
    "80858-1 receives from its right",
  );
  assert.equal(
    describe("808587", "exit"),
    "808589 W",
    "80858-7 sends to the left",
  );
  assert.equal(
    describe("808f7d", "entry"),
    "808f7b SW",
    "808f7-d receives from below",
  );
  assert.equal(
    describe("808f7f", "exit"),
    "808f81 E",
    "808f7-f leaves to the right",
  );
  assert.equal(
    describe("808f7f", "entry"),
    undefined,
    "covered by the link, not doubled",
  );
  assert.equal(
    describe("808f7d", "exit"),
    undefined,
    "covered by the link, not doubled",
  );

  const svg = renderCompositeSvg(model);
  // The one link carries both runs across the join: brown and blue.
  assert.equal(
    (svg.match(/data-run="coarse" data-inside="true"/g) ?? []).length,
    1,
  );
  assert.equal(
    (svg.match(/data-run="detail" data-inside="true"/g) ?? []).length,
    1,
  );
  assert.equal((svg.match(/data-inside="false"/g) ?? []).length, 6);
  assert.deepEqual(
    [links[0].detail.start.token, links[0].detail.end.token],
    [
      model.panels.find((p) => p.parent.token === "808f7d").detail.path.at(-1)
        .token,
      model.panels.find((p) => p.parent.token === "808f7f").detail.path[0]
        .token,
    ],
  );
});

test("stubs continue the brown run: one step long, on a cell edge axis", async () => {
  const model = await heroModel();
  const { stubs, typicalStep } = model.connections;
  const panelOf = (token) => model.panels.find((p) => p.parent.token === token);

  for (const stub of stubs) {
    const panel = panelOf(stub.panelToken);
    const dx = stub.end.x - stub.start.x;
    const dy = stub.end.y - stub.start.y;
    // Same length as a step of the run it continues.
    assert.ok(
      Math.abs(Math.hypot(dx, dy) - typicalStep) < 1e-6,
      stub.panelToken,
    );
    // Parallel to one of the cell's own edges, not aimed off at a diagonal.
    const axis = snapToCellAxis(panel.parent.vertices, { x: dx, y: dy });
    const length = Math.hypot(dx, dy);
    assert.ok(
      Math.abs(axis.x - dx / length) < 1e-9 &&
        Math.abs(axis.y - dy / length) < 1e-9,
      `${stub.panelToken} ${stub.kind} is off-axis`,
    );
    // An entry ends on the run's first point; an exit leaves from its last.
    const anchor = stub.kind === "entry" ? stub.end : stub.start;
    const expected =
      stub.kind === "entry" ? panel.coarse.path[0] : panel.coarse.path.at(-1);
    assert.equal(anchor.x, expected.x);
    assert.equal(anchor.y, expected.y);
  }
});

test("connections use the same chevron as the run, not a separate arrowhead", async () => {
  const svg = renderCompositeSvg(await heroModel());
  assert.doesNotMatch(svg, /marker-end/, "no triangle markers anywhere");
  assert.doesNotMatch(svg, /<marker\b/);
  // Stubs and the link are the same brown at the same weight as the run.
  assert.equal((svg.match(/<path class="chain-stub"/g) ?? []).length, 6);
  assert.equal((svg.match(/<path class="chain-connector"/g) ?? []).length, 1);
  assert.doesNotMatch(svg, /stroke-dasharray/);
});

test("a compass bearing reads screen space, where y grows downwards", () => {
  assert.equal(compassBearing(0, -10), "N");
  assert.equal(compassBearing(0, 10), "S");
  assert.equal(compassBearing(10, 0), "E");
  assert.equal(compassBearing(-10, 0), "W");
  assert.equal(compassBearing(10, -10), "NE");
});

test("the generated composite matches a fresh build", async () => {
  const svg = await readFile(
    new URL("s2-l10-quad-composite.svg", GENERATED),
    "utf8",
  );
  const model = await heroModel();
  for (const token of QUAD)
    assert.match(svg, new RegExp(`id="panel-${token}"`));
  assert.equal(
    (svg.match(/class="tile-selected"/g) ?? []).length,
    model.panels[0].selectedCount,
  );
  assert.equal(svg, renderCompositeSvg(model, COMPOSITE_RENDER_OPTIONS));
});

test("the orientation legend no longer prints cell id tags", async () => {
  const svg = await readFile(
    new URL("s2-hilbert-orientations.svg", GENERATED),
    "utf8",
  );
  assert.doesNotMatch(svg, /legend-token/);
  for (const displayToken of ["80858-7", "808f7-f", "80858-1", "80858-5"]) {
    assert.doesNotMatch(svg, new RegExp(`>${displayToken}<`), displayToken);
  }
  // The orientation names themselves stay.
  assert.match(svg, />canonical</);
  assert.match(svg, />swapped \+ inverted</);
});
