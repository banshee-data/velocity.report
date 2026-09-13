import { test, describe } from "node:test";
import assert from "node:assert/strict";

import { parseRecipe, RecipeError } from "../recipe.mjs";

function baseRecipe(overrides = {}) {
  return {
    version: 1,
    source: "./assets",
    frame: { frame_index: 10 },
    views: [
      {
        name: "north",
        camera: { x: 10, y: 0, z: 5 },
        target: { x: 0, y: 0, z: 0 },
      },
    ],
    layers: { lidar: false, boxes: true, trails: true },
    output: { dir: "./out" },
    ...overrides,
  };
}

describe("parseRecipe", () => {
  test("accepts a minimal, well-formed recipe with defaults filled in", () => {
    const recipe = parseRecipe(baseRecipe(), "/recipes");
    assert.equal(recipe.source, "/recipes/assets");
    assert.equal(recipe.viewport.width, 1280);
    assert.equal(recipe.viewport.height, 720);
    assert.equal(recipe.layers.trailHistorySec, 5);
    assert.equal(recipe.output.contactSheet, false);
    assert.equal(recipe.views[0].fovDeg, null);
  });

  test("rejects a version other than 1", () => {
    assert.throws(
      () => parseRecipe(baseRecipe({ version: 2 }), "/r"),
      RecipeError,
    );
  });

  test("rejects a missing source", () => {
    const raw = baseRecipe();
    delete raw.source;
    assert.throws(() => parseRecipe(raw, "/r"), RecipeError);
  });

  describe("frame selector", () => {
    test("rejects both timestamp_us and frame_index given together", () => {
      assert.throws(
        () =>
          parseRecipe(
            baseRecipe({ frame: { timestamp_us: 1, frame_index: 1 } }),
            "/r",
          ),
        /exactly one/,
      );
    });

    test("rejects neither given", () => {
      assert.throws(
        () => parseRecipe(baseRecipe({ frame: {} }), "/r"),
        /exactly one/,
      );
    });

    test("accepts timestamp_us alone", () => {
      const recipe = parseRecipe(
        baseRecipe({ frame: { timestamp_us: 5_000_000 } }),
        "/r",
      );
      assert.deepEqual(recipe.frame, { timestampUs: 5_000_000 });
    });
  });

  describe("views", () => {
    test("rejects zero views", () => {
      assert.throws(
        () => parseRecipe(baseRecipe({ views: [] }), "/r"),
        /1 to 50/,
      );
    });

    test("rejects 51 views", () => {
      const views = Array.from({ length: 51 }, (_, i) => ({
        name: `v${i}`,
        camera: { x: 1, y: 0, z: 0 },
        target: { x: 0, y: 0, z: 0 },
      }));
      assert.throws(() => parseRecipe(baseRecipe({ views }), "/r"), /1 to 50/);
    });

    test("accepts exactly 50 views", () => {
      const views = Array.from({ length: 50 }, (_, i) => ({
        name: `v${i}`,
        camera: { x: 1, y: 0, z: 0 },
        target: { x: 0, y: 0, z: 0 },
      }));
      const recipe = parseRecipe(baseRecipe({ views }), "/r");
      assert.equal(recipe.views.length, 50);
    });

    test("rejects a duplicate view name", () => {
      const raw = baseRecipe({
        views: [
          {
            name: "a",
            camera: { x: 1, y: 0, z: 0 },
            target: { x: 0, y: 0, z: 0 },
          },
          {
            name: "a",
            camera: { x: -1, y: 0, z: 0 },
            target: { x: 0, y: 0, z: 0 },
          },
        ],
      });
      assert.throws(() => parseRecipe(raw, "/r"), /duplicate/);
    });

    test("rejects an invalid view name", () => {
      const raw = baseRecipe({
        views: [
          {
            name: "Not Valid!",
            camera: { x: 1, y: 0, z: 0 },
            target: { x: 0, y: 0, z: 0 },
          },
        ],
      });
      assert.throws(() => parseRecipe(raw, "/r"), RecipeError);
    });

    test("rejects a non-finite camera coordinate", () => {
      const raw = baseRecipe({
        views: [
          {
            name: "a",
            camera: { x: "nope", y: 0, z: 0 },
            target: { x: 0, y: 0, z: 0 },
          },
        ],
      });
      assert.throws(() => parseRecipe(raw, "/r"), RecipeError);
    });

    test("carries an explicit fov_deg through", () => {
      const raw = baseRecipe({
        views: [
          {
            name: "a",
            camera: { x: 1, y: 0, z: 0 },
            target: { x: 0, y: 0, z: 0 },
            fov_deg: 40,
          },
        ],
      });
      assert.equal(parseRecipe(raw, "/r").views[0].fovDeg, 40);
    });
  });

  describe("layers", () => {
    test("requires all three visibility flags explicitly", () => {
      assert.throws(
        () =>
          parseRecipe(
            baseRecipe({ layers: { lidar: false, boxes: true } }),
            "/r",
          ),
        /trails/,
      );
    });

    test("rejects a non-boolean layer flag", () => {
      assert.throws(
        () =>
          parseRecipe(
            baseRecipe({ layers: { lidar: "no", boxes: true, trails: true } }),
            "/r",
          ),
        RecipeError,
      );
    });

    test("rejects a negative trail_history_sec", () => {
      assert.throws(
        () =>
          parseRecipe(
            baseRecipe({
              layers: {
                lidar: false,
                boxes: true,
                trails: true,
                trail_history_sec: -1,
              },
            }),
            "/r",
          ),
        /negative/,
      );
    });
  });

  describe("viewport", () => {
    test("rejects a non-integer width", () => {
      assert.throws(
        () =>
          parseRecipe(
            baseRecipe({ viewport: { width: 12.5, height: 720 } }),
            "/r",
          ),
        RecipeError,
      );
    });

    test("accepts an explicit viewport", () => {
      const recipe = parseRecipe(
        baseRecipe({ viewport: { width: 640, height: 480 } }),
        "/r",
      );
      assert.deepEqual(recipe.viewport, { width: 640, height: 480 });
    });
  });

  describe("output", () => {
    test("rejects a missing output.dir", () => {
      assert.throws(
        () => parseRecipe(baseRecipe({ output: {} }), "/r"),
        RecipeError,
      );
    });

    test("carries contact_sheet through", () => {
      const recipe = parseRecipe(
        baseRecipe({ output: { dir: "./out", contact_sheet: true } }),
        "/r",
      );
      assert.equal(recipe.output.contactSheet, true);
    });
  });

  describe("bullet_time", () => {
    function withBulletTime(overrides = {}) {
      return baseRecipe({
        bullet_time: {
          target: { x: 0, y: 0, z: 0 },
          base_view: "north",
          ...overrides,
        },
      });
    }

    test("is null when omitted", () => {
      assert.equal(parseRecipe(baseRecipe(), "/r").bulletTime, null);
    });

    test("accepts a minimal spec, filling in defaults", () => {
      const recipe = parseRecipe(withBulletTime(), "/r");
      assert.deepEqual(recipe.bulletTime.target, {
        point: { x: 0, y: 0, z: 0 },
      });
      assert.equal(recipe.bulletTime.baseView, "north");
      assert.equal(recipe.bulletTime.imageCount, 25);
      assert.equal(recipe.bulletTime.arcStartDeg, -90);
      assert.equal(recipe.bulletTime.arcEndDeg, 90);
      assert.equal(recipe.bulletTime.radiusM, null);
      assert.deepEqual(recipe.bulletTime.elevationKeyframes, [
        { t: 0, deg: 20 },
        { t: 1 / 3, deg: 30 },
        { t: 2 / 3, deg: 20 },
        { t: 1, deg: 10 },
      ]);
    });

    test("accepts an object_id target", () => {
      const recipe = parseRecipe(
        withBulletTime({ target: { object_id: "3" } }),
        "/r",
      );
      assert.deepEqual(recipe.bulletTime.target, { objectId: "3" });
    });

    test("rejects a base_view that does not name a recipe view", () => {
      assert.throws(
        () => parseRecipe(withBulletTime({ base_view: "nope" }), "/r"),
        RecipeError,
      );
    });

    test("rejects an image_count outside [2, 50]", () => {
      assert.throws(
        () => parseRecipe(withBulletTime({ image_count: 1 }), "/r"),
        RecipeError,
      );
      assert.throws(
        () => parseRecipe(withBulletTime({ image_count: 51 }), "/r"),
        RecipeError,
      );
    });

    test("accepts image_count at the boundaries", () => {
      assert.equal(
        parseRecipe(withBulletTime({ image_count: 2 }), "/r").bulletTime
          .imageCount,
        2,
      );
      assert.equal(
        parseRecipe(withBulletTime({ image_count: 50 }), "/r").bulletTime
          .imageCount,
        50,
      );
    });

    test("rejects a non-positive radius_m", () => {
      assert.throws(
        () => parseRecipe(withBulletTime({ radius_m: 0 }), "/r"),
        RecipeError,
      );
    });

    test("carries an explicit radius_m through", () => {
      const recipe = parseRecipe(withBulletTime({ radius_m: 12.5 }), "/r");
      assert.equal(recipe.bulletTime.radiusM, 12.5);
    });

    describe("elevation_keyframes", () => {
      test("rejects fewer than 2 entries", () => {
        assert.throws(
          () =>
            parseRecipe(
              withBulletTime({ elevation_keyframes: [{ t: 0, deg: 10 }] }),
              "/r",
            ),
          RecipeError,
        );
      });

      test("rejects a list not starting at t=0", () => {
        assert.throws(
          () =>
            parseRecipe(
              withBulletTime({
                elevation_keyframes: [
                  { t: 0.1, deg: 10 },
                  { t: 1, deg: 20 },
                ],
              }),
              "/r",
            ),
          /start at t=0/,
        );
      });

      test("rejects a list not ending at t=1", () => {
        assert.throws(
          () =>
            parseRecipe(
              withBulletTime({
                elevation_keyframes: [
                  { t: 0, deg: 10 },
                  { t: 0.9, deg: 20 },
                ],
              }),
              "/r",
            ),
          /end at t=1/,
        );
      });

      test("rejects non-ascending t values", () => {
        assert.throws(
          () =>
            parseRecipe(
              withBulletTime({
                elevation_keyframes: [
                  { t: 0, deg: 10 },
                  { t: 0.5, deg: 20 },
                  { t: 0.5, deg: 25 },
                  { t: 1, deg: 30 },
                ],
              }),
              "/r",
            ),
          /ascending/,
        );
      });

      test("accepts a custom, valid keyframe list", () => {
        const recipe = parseRecipe(
          withBulletTime({
            elevation_keyframes: [
              { t: 0, deg: 15 },
              { t: 0.5, deg: 45 },
              { t: 1, deg: 15 },
            ],
          }),
          "/r",
        );
        assert.equal(recipe.bulletTime.elevationKeyframes.length, 3);
      });
    });
  });
});
