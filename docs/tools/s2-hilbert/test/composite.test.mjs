import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  buildS2CompositeModel,
  cellSlantDegrees,
  compositeAdjacency,
  orientationName,
  renderCompositeSvg,
} from "../index.mjs";
import { COMPOSITE_RENDER_OPTIONS } from "../generate-composite.mjs";

const SELECTION_URL = new URL("../selection/80858-1-l13-cells.json", import.meta.url);
const GENERATED = new URL("../generated/", import.meta.url);
const QUAD = ["808581", "808587", "808f7d", "808f7f"];

const loadSelection = async () => JSON.parse(await readFile(SELECTION_URL, "utf8"));
const heroModel = async () =>
  quadModel([
    { parent: "808581", role: "hero", cellSelection: await loadSelection() },
    { parent: "808587" },
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
  const adjacency = compositeAdjacency(quadModel(QUAD.map((parent) => ({ parent }))));
  assert.equal(adjacency.filter((pair) => pair.relation === "edge").length, 4);
  assert.equal(adjacency.filter((pair) => pair.relation === "corner").length, 2);
});

test("one shared projection places the panels, so neighbours actually join", () => {
  const model = quadModel(QUAD.map((parent) => ({ parent })));
  // A shared fit means every panel reports the same scale and viewBox.
  const scales = new Set(model.panels.map(() => model.projection.scale));
  assert.equal(scales.size, 1);

  // 808581 and 808587 share an edge, so two of their drawn vertices coincide.
  const round = (vertex) => `${vertex.x.toFixed(6)},${vertex.y.toFixed(6)}`;
  const first = new Set(
    model.panels.find((p) => p.parent.token === "808581").parent.vertices.map(round),
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
  for (const panel of model.panels.slice(1)) assert.equal(panel.selectedCount, null);
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

test("every panel is labelled, including the hero, and each label is rotated", async () => {
  const svg = renderCompositeSvg(await heroModel());
  assert.match(svg, /data-role="hero"/);
  assert.equal((svg.match(/data-role="context"/g) ?? []).length, 3);
  // Four labels: three context plus the hero, which reads "inverted" too.
  assert.equal((svg.match(/<text class="panel-label/g) ?? []).length, 4);
  assert.equal((svg.match(/>inverted</g) ?? []).length, 2);
  assert.match(svg, />canonical</);
  assert.match(svg, />swapped</);
  // Centred and set on the cell's own slant, not axis-aligned.
  const rotations = [...svg.matchAll(/class="panel-label[^"]*" transform="translate\([^)]+\) rotate\((-?[\d.]+)\)"/g)];
  assert.equal(rotations.length, 4);
  for (const [, angle] of rotations) {
    assert.notEqual(Number(angle), 0, "labels follow the shear");
    assert.ok(Math.abs(Number(angle)) < 45, `unexpected label angle ${angle}`);
  }
  assert.equal((svg.match(/<polyline class="composite-chevron/g) ?? []).length, 15 * 4);
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
  assert.ok(sheared > 0 && sheared < 45, `expected a positive slant, got ${sheared}`);
});

test("every panel shows its full detail run, weighted only where selected", async () => {
  const model = await heroModel();
  const svg = renderCompositeSvg(model);
  const [hero, ...context] = model.panels;

  // The hero splits its 63 detail segments into heavy and light runs.
  assert.equal(hero.detail.classifications.length, 63);
  const heavy = hero.detail.classifications.filter((c) => c === "land").length;
  assert.ok(heavy > 0 && heavy < 63);
  assert.match(svg, /<path class="hilbert-detail hilbert-detail-heavy"/);
  // Context panels draw the same 64-cell run, light throughout.
  for (const panel of context) assert.equal(panel.detail.classifications, null);
  assert.equal(
    (svg.match(/<path class="hilbert-detail hilbert-detail-light"/g) ?? []).length,
    4,
  );
  // The hero's 16-step run is the light one; the others stay solid.
  assert.equal(
    (svg.match(/<path class="hilbert-coarse hilbert-coarse-light"/g) ?? []).length,
    1,
  );
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

test("panels chain end-to-start, and only the chain's own ends keep markers", async () => {
  const model = await heroModel();
  const order = model.chain.order.map((index) => model.panels[index].parent.token);
  assert.deepEqual(order, ["808587", "808f7d", "808f7f", "808581"]);
  assert.equal(model.chain.links.length, 3);

  // Two junctions are a single coarse step; the third is a jump across the figure.
  const continuous = model.chain.links.filter((link) => link.continuous);
  assert.equal(continuous.length, 2);
  const jump = model.chain.links.find((link) => !link.continuous);
  assert.equal(jump.fromToken, "808587");
  assert.ok(jump.length > jump.typicalStep * 2);

  // Only 808f7d -> 808f7f is a true S2 succession; the rest merely abut.
  const consecutive = model.chain.links.filter((link) => link.s2Consecutive);
  assert.equal(consecutive.length, 1);
  assert.deepEqual(
    [consecutive[0].fromToken, consecutive[0].toToken],
    ["808f7d", "808f7f"],
  );

  // Interior junctions are de-duped: one start marker and one end marker only.
  const svg = renderCompositeSvg(model);
  assert.equal((svg.match(/<circle class="composite-start"/g) ?? []).length, 1);
  assert.equal((svg.match(/<circle class="composite-end"/g) ?? []).length, 1);
  assert.equal((svg.match(/<path class="chain-connector/g) ?? []).length, 3);
  assert.equal(
    (svg.match(/<path class="chain-connector chain-connector-jump"/g) ?? []).length,
    1,
  );
  assert.equal((svg.match(/data-s2-consecutive="true"/g) ?? []).length, 1);
});

test("the generated composite matches a fresh build", async () => {
  const svg = await readFile(new URL("s2-l10-quad-composite.svg", GENERATED), "utf8");
  const model = await heroModel();
  for (const token of QUAD) assert.match(svg, new RegExp(`id="panel-${token}"`));
  assert.equal(
    (svg.match(/class="tile-selected"/g) ?? []).length,
    model.panels[0].selectedCount,
  );
  assert.equal(svg, renderCompositeSvg(model, COMPOSITE_RENDER_OPTIONS));
});

test("the orientation legend no longer prints cell id tags", async () => {
  const svg = await readFile(new URL("s2-hilbert-orientations.svg", GENERATED), "utf8");
  assert.doesNotMatch(svg, /legend-token/);
  for (const displayToken of ["80858-7", "808f7-f", "80858-1", "80858-5"]) {
    assert.doesNotMatch(svg, new RegExp(`>${displayToken}<`), displayToken);
  }
  // The orientation names themselves stay.
  assert.match(svg, />canonical</);
  assert.match(svg, />swapped \+ inverted</);
});
