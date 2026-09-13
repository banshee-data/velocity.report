// Tests for the trail-reconstruction pieces of the scene player. TrackVisual
// builds real three.js scene-graph objects (BufferGeometry, materials) but
// none of it needs a WebGLRenderer or a DOM, so it runs fine under plain Node.

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";

import {
  TrackVisual,
  buildTrailHistories,
  sceneTrailPoint,
} from "../scene-player.js";

describe("sceneTrailPoint", () => {
  test("applies the ENU->scene transform and the base-of-box offset", () => {
    const p = sceneTrailPoint({ x: 3, y: 5, z: 1, h: 2 });
    assert.equal(p[0], 3, "east stays x");
    assert.equal(p[1], 1 - 2 / 2 + 0.05, "up becomes y, offset to the base");
    assert.equal(p[2], -5, "north becomes -z");
  });

  test("floors the height used for the offset, matching the rendered box", () => {
    const p = sceneTrailPoint({ x: 0, y: 0, z: 1, h: 0.01 });
    assert.equal(p[1], 1 - 0.9 / 2 + 0.05, "tiny height is floored to 0.9");
  });
});

describe("buildTrailHistories", () => {
  test("distributes samples per track, one pass over the frames", () => {
    const frames = [
      { t: 0, tr: [{ id: "a", x: 0, y: 0, z: 0, h: 1 }] },
      {
        t: 1,
        tr: [
          { id: "a", x: 1, y: 0, z: 0, h: 1 },
          { id: "b", x: 5, y: 0, z: 0, h: 1 },
        ],
      },
    ];
    const byTrack = buildTrailHistories(frames, ["a", "b"]);
    assert.equal(byTrack.get("a").length, 2);
    assert.equal(byTrack.get("b").length, 1);
  });

  test("ignores tracks not in the requested id set", () => {
    const frames = [{ t: 0, tr: [{ id: "gone", x: 0, y: 0, z: 0 }] }];
    const byTrack = buildTrailHistories(frames, ["a"]);
    assert.equal(byTrack.get("a").length, 0);
    assert.equal(byTrack.has("gone"), false);
  });

  test("returns an empty array, not undefined, for a track with no samples", () => {
    const byTrack = buildTrailHistories([], ["a"]);
    assert.deepEqual(byTrack.get("a"), []);
  });
});

describe("TrackVisual trail buffer", () => {
  function fresh() {
    const scene = new THREE.Scene();
    return { scene, visual: new TrackVisual(scene, 0x00ff00) };
  }

  // Regression test: a brand-new track (or trails toggled off) calls
  // setTrail([]) before any non-empty call has ever run. _ensureTrailCapacity
  // used to skip creating the position attribute entirely when the requested
  // count was <= the starting capacity of 0, so setTrail's very next line —
  // reading trailGeo.attributes.position.array — threw
  // "Cannot read properties of undefined (reading 'array')". Caught by
  // exercising the toggle in a real browser; this pins it so it can't return.
  test("setTrail([]) on a freshly-constructed track does not throw", () => {
    const { visual } = fresh();
    assert.doesNotThrow(() => visual.setTrail([]));
    assert.equal(visual.trailPoints.length, 0);
  });

  test("setTrail grows the buffer, then shrinks back to empty without throwing", () => {
    const { visual } = fresh();
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
      [2, 0, 0],
    ]);
    assert.equal(visual.trailPoints.length, 3);
    assert.doesNotThrow(() => visual.setTrail([]));
    assert.equal(visual.trailPoints.length, 0);
  });

  test("a trail of 1 point is not visible; 2 or more is", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);

    visual.setTrail([[0, 0, 0]]);
    assert.equal(visual.trail.visible, false, "a single point draws no line");

    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);
    assert.equal(visual.trail.visible, true);
  });

  test("boxes off hides the trail without discarding the trails preference", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);
    assert.equal(visual.trail.visible, true);

    visual.setVisible(false);
    assert.equal(visual.trail.visible, false, "boxes off hides the trail too");

    visual.setVisible(true);
    assert.equal(
      visual.trail.visible,
      true,
      "re-enabling boxes restores the trails preference, not a forced-off state",
    );
  });

  test("the trails toggle alone hides the trail while boxes stay visible", () => {
    const { visual } = fresh();
    visual.setVisible(true);
    visual.setTrailVisible(true);
    visual.setTrail([
      [0, 0, 0],
      [1, 0, 0],
    ]);

    visual.setTrailVisible(false);
    assert.equal(visual.trail.visible, false);
    assert.equal(visual.edges.visible, true, "boxes are a separate concern");
  });
});
