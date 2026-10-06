import test from "node:test";
import assert from "node:assert/strict";
import {
  advanceSceneClock,
  autoplayScene,
  pointCloudOverlayOpacity,
  sceneLoopDuration,
  sceneGeneratorVersions,
  sceneSourcesAlign,
} from "../scene-playback.js";

test("scenes autoplay when the page loads", () => {
  assert.equal(autoplayScene(), true);
});

test("the point cloud fades out over its last five seconds", () => {
  assert.equal(pointCloudOverlayOpacity(0, 30), 1);
  assert.equal(pointCloudOverlayOpacity(24.9, 30), 1);
  assert.equal(pointCloudOverlayOpacity(27.5, 30), 0.5);
  assert.equal(pointCloudOverlayOpacity(30, 30), 0);
  assert.equal(pointCloudOverlayOpacity(300, 30), 0);
});

test("the point cloud fades back in for two seconds only after a loop", () => {
  const looping = { loopingIn: true };
  assert.equal(pointCloudOverlayOpacity(0, 30, looping), 0);
  assert.equal(pointCloudOverlayOpacity(1, 30, looping), 0.5);
  assert.equal(pointCloudOverlayOpacity(2, 30, looping), 1);
  assert.equal(pointCloudOverlayOpacity(0, 30), 1);
});

test("point and track exports must identify the same recording and frame", () => {
  const header = {
    start_ns: "123",
    source_vrlog_sha256: "abc",
    sensor_id: "lidar",
    coordinate_frame: { frame_id: "site/lidar", reference_frame: "ENU" },
  };
  assert.equal(sceneSourcesAlign(header, structuredClone(header)), true);
  assert.equal(
    sceneSourcesAlign(header, { ...header, start_ns: "124" }),
    false,
  );
  assert.equal(
    sceneSourcesAlign(header, { ...header, source_vrlog_sha256: "def" }),
    false,
  );
});

test("generator versions come from VRLOG provenance, without blanks or duplicates", () => {
  assert.deepEqual(
    sceneGeneratorVersions([
      { header: { build_version: "0.5.1-pre32" } },
      { header: { build_version: " 0.5.1-pre32 " } },
      { header: {} },
      { header: { build_version: "0.5.1-pre31" } },
    ]),
    ["0.5.1-pre32", "0.5.1-pre31"],
  );
});

test("the scene clock loops instead of stopping at the end", () => {
  const wrapped = advanceSceneClock(29.8, 0.4, 1, 30);
  assert.equal(wrapped.wrapped, true);
  assert.ok(Math.abs(wrapped.seconds - 0.2) < 1e-9);
  assert.deepEqual(advanceSceneClock(12, 0.25, 8, 30), {
    seconds: 14,
    wrapped: false,
  });
  assert.deepEqual(advanceSceneClock(0, 0.25, 1, 0), {
    seconds: 0,
    wrapped: false,
  });
});

test("the loop boundary can follow the opening or the full recording", () => {
  assert.equal(sceneLoopDuration(1994, 29.8, false), 1994);
  assert.equal(sceneLoopDuration(1994, 29.8, true), 29.8);
  assert.equal(sceneLoopDuration(1994, 0, true), 1994);
});
