import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import * as THREE from "three";
import { TrackVisual } from "../scene-player.js";

// The oracle starts with data-space corners, rotates in XY, then maps points
// to (x, z, -y). It does not reuse the player's Euler rotation or yaw sign.
function assertBoxGeometry(visual, track, yaw) {
  const cx = track.bx ?? track.x;
  const cy = track.by ?? track.y;
  // The public viewer keeps small markers at least 0.9 m across.
  const length = Math.max(track.l, 0.9);
  const width = Math.max(track.w, 0.9);
  const height = Math.max(track.h, 0.9);
  for (const object of [visual.fill, visual.edges]) {
    object.updateMatrixWorld(true);
    const direction = new THREE.Vector3(1, 0, 0).transformDirection(
      object.matrixWorld,
    );
    const expectedDirection = new THREE.Vector3(
      Math.cos(yaw),
      0,
      -Math.sin(yaw),
    );
    assert.ok(
      direction.distanceTo(expectedDirection) < 1e-10,
      `length axis must follow data yaw ${yaw}, got ${direction.toArray()}`,
    );
    for (const along of [-0.5, 0.5]) {
      for (const across of [-0.5, 0.5]) {
        for (const up of [-0.5, 0.5]) {
          const x = along * length;
          const y = -across * width;
          const expected = new THREE.Vector3(
            cx + x * Math.cos(yaw) - y * Math.sin(yaw),
            track.z + (up + 0.5) * height,
            -(cy + x * Math.sin(yaw) + y * Math.cos(yaw)),
          );
          const actual = new THREE.Vector3(along, up, across).applyMatrix4(
            object.matrixWorld,
          );
          assert.ok(actual.distanceTo(expected) < 1e-9, "box corner in scene");
        }
      }
    }
  }
}

describe("TrackVisual box orientation", () => {
  const base = { x: 10, y: -3, z: -2, bx: 11, by: -4, l: 4.5, w: 1.8, h: 1.5 };
  for (const yaw of [
    0,
    Math.PI / 4,
    -Math.PI / 4,
    (3 * Math.PI) / 4,
    (-3 * Math.PI) / 4,
    Math.PI,
    -Math.PI,
    0.37,
    Math.PI + 0.2,
    -Math.PI - 0.2,
  ]) {
    test(`transforms length, width and corners at yaw ${yaw}`, () => {
      const scene = new THREE.Scene();
      const visual = new TrackVisual(scene, 0x00ff00);
      try {
        const track = { ...base, bh: yaw, hdg: yaw + 0.7 };
        visual.update(track);
        assertBoxGeometry(visual, track, yaw);
      } finally {
        visual.dispose(scene);
      }
    });
  }

  for (const [name, angles, yaw] of [
    ["zero box yaw takes precedence over course", { bh: 0, hdg: 1.2 }, 0],
    ["legacy course fallback", { hdg: -0.7 }, -0.7],
    ["null box yaw uses course", { bh: null, hdg: 0.8 }, 0.8],
    ["absent headings default to zero", {}, 0],
  ]) {
    test(name, () => {
      const scene = new THREE.Scene();
      const visual = new TrackVisual(scene, 0x00ff00);
      try {
        const { bx, by, ...legacy } = base;
        const track = { ...legacy, ...angles };
        visual.update(track);
        assertBoxGeometry(visual, track, yaw);
      } finally {
        visual.dispose(scene);
      }
    });
  }
});

// Existing published chunks are the frozen regression inputs, not copies of
// raw capture data. IDs and playback windows are documented in the report.
for (const [site, chunk, id, start, end] of [
  ["lombard-broderick", "0001", "gi", 104.6, 111.7],
  ["embarcadero-broadway", "0000", "1v", 28.6, 43.9],
]) {
  test(`${site}: published turn has the same orientation after sequential and non-sequential updates`, () => {
    const path = new URL(
      `../../scenes/${site}/assets/part-000/frames/chunk_${chunk}.ndjson.gz`,
      import.meta.url,
    );
    const tracks = gunzipSync(readFileSync(path))
      .toString("utf8")
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line))
      .filter((frame) => frame.t / 1e6 >= start && frame.t / 1e6 <= end)
      .flatMap((frame) => frame.tr.filter((track) => track.id === id));
    assert.ok(tracks.length > 30, "retain the complete turn window");
    const scene = new THREE.Scene();
    const sequential = new TrackVisual(scene, 0x00ff00);
    const seek = new TrackVisual(scene, 0x00ff00);
    try {
      for (const track of tracks) {
        sequential.update(track);
        assertBoxGeometry(sequential, track, track.bh);
      }
      // Non-sequential visits must not inherit the preceding frame's pose.
      for (const index of [
        0,
        tracks.length - 1,
        Math.floor(tracks.length / 2),
        0,
      ]) {
        const track = tracks[index];
        seek.update(track);
        assertBoxGeometry(seek, track, track.bh);
        sequential.update(track);
        assertBoxGeometry(sequential, track, track.bh);
        assert.deepEqual(
          seek.fill.matrixWorld.elements,
          sequential.fill.matrixWorld.elements,
        );
        assert.deepEqual(
          seek.edges.matrixWorld.elements,
          sequential.edges.matrixWorld.elements,
        );
      }
    } finally {
      sequential.dispose(scene);
      seek.dispose(scene);
    }
  });
}
