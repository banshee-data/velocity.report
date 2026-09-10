import test from "node:test";
import assert from "node:assert/strict";
import { advanceSceneClock, autoplayScene } from "../scene-playback.js";

test("scenes autoplay unless the reader has requested less motion", () => {
  assert.equal(autoplayScene(), true);
  assert.equal(autoplayScene(() => ({ matches: false })), true);
  assert.equal(autoplayScene(() => ({ matches: true })), false);
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
