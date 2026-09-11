import test from "node:test";
import assert from "node:assert/strict";

import { speedHistogramModel } from "../scene-speed-stats.js";

test("scene speed histogram uses percentages and preserves empty 5 mph buckets", () => {
  const model = speedHistogramModel({
    bucket_size: 5,
    histogram: [
      { start_mph: 0, count: 1 },
      { start_mph: 5, count: 0 },
      { start_mph: 10, count: 3 },
    ],
  });
  assert.equal(model.total, 4);
  assert.deepEqual(model.values.map((bucket) => [bucket.start, bucket.end, bucket.percentage]), [
    [0, 5, 25],
    [5, 10, 0],
    [10, 15, 75],
  ]);
  assert.equal(model.ceiling, 75);
});

test("an empty speed summary produces a stable chart scale", () => {
  assert.deepEqual(speedHistogramModel({ histogram: [] }), {
    total: 0,
    values: [],
    ceiling: 5,
  });
});
