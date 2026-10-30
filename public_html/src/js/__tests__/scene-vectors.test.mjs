// Tests for the street-geometry layer of the scene player. The geometry is
// plain arrays and three.js objects with no renderer, so it runs under Node.

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";

import {
  createGroundGroup,
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
        feature("wall_surfaces", { type: "Polygon", coordinates: [square()] }),
      ],
    });
    // Four edges, two endpoints each, three numbers per endpoint.
    assert.equal(l.lines.length, 4 * 2 * 3);
    assert.equal(l.fill.positions.length, 4 * 3);
    assert.equal(l.fill.indices.length, 2 * 3);
  });

  test("omits experimental layers even in old exports, retaining street lines", () => {
    const geometry = {
      type: "LineString",
      coordinates: [
        [0, 0, 0],
        [1, 1, 0],
      ],
    };
    assert.deepEqual(
      vectorLayers({
        features: ["lane_markings", "street_furniture", "city_streets"].map(
          (l) => feature(l, geometry),
        ),
      }).map((l) => l.layer),
      ["city_streets"],
    );
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
        feature("wall_surfaces", { type: "Polygon", coordinates: [flat] }),
      ],
    });
    assert.equal(l.fill, null);
    assert.ok(l.lines.length > 0);
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
        feature("wall_surfaces", { type: "Polygon", coordinates: [square()] }),
      ],
    });
    const names = group.children.map((c) => c.name);
    assert.deepEqual(names, ["wall_surfaces:fill", "wall_surfaces:lines"]);
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

describe("registered ground", () => {
  test("keeps slope and sensor placement, without a synthetic fallback", () => {
    assert.equal(createGroundGroup(null).children.length, 0);
    const g = createGroundGroup({
      ground: {
        positions: [
          [10, 20, -3],
          [11, 20, -2],
          [10, 21, -3],
        ],
        indices: [0, 1, 2],
      },
    });
    assert.deepEqual(
      Array.from(g.children[0].geometry.attributes.position.array),
      [10, -3, -20, 11, -2, -20, 10, -3, -21],
    );
    assert.deepEqual(g.position.toArray(), [0, 0, 0]);
    assert.equal(g.children.length, 2);
  });
  test("footprints are mask data when full walls are present", () => {
    const geometry = { type: "Polygon", coordinates: [square()] };
    assert.deepEqual(
      vectorLayers({
        features: [
          feature("footprints", geometry),
          feature("wall_surfaces", geometry),
        ],
      }).map((l) => l.layer),
      ["wall_surfaces"],
    );
  });
});

describe("public trees and buildings", () => {
  test("omits registered facade patches while retaining building context", () => {
    const geometry = { type: "Polygon", coordinates: [square()] };
    const layers = vectorLayers({
      features: [
        feature("wall_planes", geometry),
        feature("observed", geometry, { kind: "facade" }),
        feature("wall_surfaces", geometry),
      ],
    });
    assert.deepEqual(
      layers.map((l) => l.layer),
      ["wall_surfaces"],
    );
  });
  test("draws dark green foliage with the measured crown dimensions", () => {
    const make = (points) =>
      vectorLayers({
        features: [
          feature(
            "trees",
            {
              type: "LineString",
              coordinates: points,
            },
            { crown_radius_m: 3, crown_base_m: 2, height_m: 10 },
          ),
        ],
      })[0];
    const a = make([
        [5, 5, -2.5],
        [5, 5, 0],
      ]),
      b = make([
        [5, 5, 0],
        [5, 5, -2.5],
      ]);
    assert.deepEqual(a.lines, b.lines);
    assert.deepEqual(a.fill, b.fill);
    assert.equal(a.style.colour, 0x245238);
    const ys = [];
    for (let i = 1; i < a.fill.positions.length; i += 3)
      ys.push(a.fill.positions[i]);
    assert.equal(Math.min(...ys), -0.5);
    assert.equal(Math.max(...ys), 7.5);
    assert.ok(a.fill.indices.length > 0);
  });
});
