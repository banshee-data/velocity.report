// Tests for the street-geometry layer of the scene player. The geometry is
// plain arrays and three.js objects with no renderer, so it runs under Node.

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";

import {
  VECTOR_STYLE,
  createVectorGroup,
  loadVectors,
  ringNormal,
  triangulatePolygon,
  vectorLayers,
} from "../scene-vectors.js";

const feature = (layer, geometry, props = {}) => ({
  type: "Feature",
  geometry,
  properties: { layer, kind: "", evidence_status: "observed", ...props },
});

const square = (z = -2.5) => [
  [0, 0, z],
  [4, 0, z],
  [4, 2, z],
  [0, 2, z],
  [0, 0, z],
];

describe("vectorLayers", () => {
  test("maps the sensor frame to three.js axes: east x, up y, north -z", () => {
    const [l] = vectorLayers({
      features: [
        feature("city_streets", {
          type: "LineString",
          coordinates: [
            [1, 2, 3],
            [4, 5, 6],
          ],
        }),
      ],
    });
    assert.deepEqual(Array.from(l.lines), [1, 3, -2, 4, 6, -5]);
  });

  test("outlines a polygon once, without its closing repeat, and fills a filled layer", () => {
    const [l] = vectorLayers({
      features: [
        feature("lane_markings", { type: "Polygon", coordinates: [square()] }),
      ],
    });
    // Four edges, two endpoints each, three numbers per endpoint.
    assert.equal(l.lines.length, 4 * 2 * 3);
    assert.equal(l.fill.positions.length, 4 * 3);
    assert.equal(l.fill.indices.length, 2 * 3);
  });

  test("outlines a crosswalk's area without filling it", () => {
    const [l] = vectorLayers({
      features: [
        feature(
          "lane_markings",
          { type: "Polygon", coordinates: [square()] },
          { marking: "crosswalk" },
        ),
      ],
    });
    assert.equal(l.fill, null);
    assert.equal(l.lines.length, 4 * 2 * 3);
  });

  test("lifts a lane marking off the road by its layer's lift", () => {
    const [l] = vectorLayers({
      features: [
        feature("lane_markings", {
          type: "Polygon",
          coordinates: [square(-2.5)],
        }),
      ],
    });
    const up = l.fill.positions[1];
    assert.ok(Math.abs(up - (-2.5 + VECTOR_STYLE.lane_markings.lift)) < 1e-6);
  });

  test("triangulates an upright wall in its own plane", () => {
    const wall = [
      [0, 5, -2.5],
      [6, 5, -2.5],
      [6, 5, 7],
      [3, 5, 9],
      [0, 5, 7],
    ];
    const n = ringNormal(wall);
    assert.ok(Math.abs(n[2]) < 1e-9, "a wall's normal is level");
    assert.equal(
      triangulatePolygon(wall).length,
      3 * 3,
      "a gable end is three triangles",
    );
    const [l] = vectorLayers({
      features: [
        feature("wall_surfaces", {
          type: "Polygon",
          coordinates: [[...wall, wall[0]]],
        }),
      ],
    });
    assert.equal(l.fill.indices.length, 9);
  });

  test("a polygon with no area is outlined but not filled", () => {
    const flat = [
      [0, 0, 0],
      [1, 0, 0],
      [2, 0, 0],
      [0, 0, 0],
    ];
    const [l] = vectorLayers({
      features: [
        feature("wall_planes", { type: "Polygon", coordinates: [flat] }),
      ],
    });
    assert.equal(l.fill, null);
    assert.ok(l.lines.length > 0);
  });

  test("stands a tree up from its base, whichever end the segment starts at", () => {
    const props = { crown_radius_m: 3, crown_base_m: 2, height_m: 10 };
    const tree = (coordinates) =>
      vectorLayers({
        features: [
          feature("trees", { type: "LineString", coordinates }, props),
        ],
      })[0];
    const a = tree([
      [5, 5, -2.5],
      [5, 5, 0],
    ]);
    const b = tree([
      [5, 5, 0],
      [5, 5, -2.5],
    ]);
    assert.deepEqual(Array.from(a.lines), Array.from(b.lines));
    // The trunk runs from the base to the crown's bottom, 2 m up.
    assert.deepEqual(
      Array.from(a.lines.slice(0, 6)),
      [5, -2.5, -5, 5, -0.5, -5],
    );
    const ys = [];
    for (let i = 1; i < a.lines.length; i += 3) ys.push(a.lines[i]);
    assert.ok(
      Math.abs(Math.max(...ys) - 7.5) < 1e-5,
      "the crown reaches height_m",
    );
  });

  test("leaves out layers it has no style for", () => {
    const layers = vectorLayers({
      features: [
        feature("osm", {
          type: "LineString",
          coordinates: [
            [0, 0, 0],
            [1, 1, 0],
          ],
        }),
        feature(
          "observed",
          {
            type: "LineString",
            coordinates: [
              [0, 0, 0],
              [1, 1, 0],
            ],
          },
          { kind: "kerb" },
        ),
      ],
    });
    assert.deepEqual(
      layers.map((l) => l.layer),
      ["observed"],
    );
  });
});

describe("createVectorGroup", () => {
  test("draws fills and lines under the boxes, writing no depth", () => {
    const group = createVectorGroup({
      features: [
        feature("lane_markings", { type: "Polygon", coordinates: [square()] }),
      ],
    });
    const names = group.children.map((c) => c.name);
    assert.deepEqual(names, ["lane_markings:fill", "lane_markings:lines"]);
    for (const child of group.children) {
      assert.ok(child.renderOrder < 0);
      assert.equal(child.material.depthWrite, false);
      assert.equal(child.material.transparent, true);
    }
    assert.ok(group.children[0] instanceof THREE.Mesh);
    assert.ok(group.children[1] instanceof THREE.LineSegments);
  });
});

describe("loadVectors", () => {
  test("is null for a scene without the file", async () => {
    const saved = globalThis.fetch;
    globalThis.fetch = async () => new Response("", { status: 404 });
    try {
      assert.equal(await loadVectors("/missing"), null);
    } finally {
      globalThis.fetch = saved;
    }
  });

  test("reads a gzipped file", async () => {
    const collection = {
      type: "FeatureCollection",
      features: [
        feature("city_streets", {
          type: "LineString",
          coordinates: [
            [0, 0, 0],
            [1, 0, 0],
          ],
        }),
      ],
    };
    const gz = await new Response(
      new Blob([JSON.stringify(collection)])
        .stream()
        .pipeThrough(new CompressionStream("gzip")),
    ).arrayBuffer();
    const saved = globalThis.fetch;
    globalThis.fetch = async () => new Response(gz, { status: 200 });
    try {
      const got = await loadVectors("/vectors.geojson.gz");
      assert.equal(got.features.length, 1);
    } finally {
      globalThis.fetch = saved;
    }
  });
});
