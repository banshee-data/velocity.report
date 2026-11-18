import test from "node:test";
import assert from "node:assert/strict";
import { contextMatches } from "../following-review.mjs";

test("static lanes require the same deployment window and checked pose", () => {
  const frame = { frame_id: "sensor", reference_frame: "sensor" };
  const header = {
    site: "site",
    sensor_id: "sensor",
    coordinate_frame: frame,
    start_ns: "1000000000",
    duration_sec: 1,
  };
  const published = { sensor_id: "sensor", coordinate_frame: frame };
  const entry = {
    id: "site",
    static_parts: [
      { start: "1970-01-01T00:00:00Z", end: "1970-01-01T00:00:03Z" },
    ],
  };
  const vectors = { site: "site", placement: { check: { passed: true } } };
  assert.equal(contextMatches(header, published, entry, vectors), true);
  assert.equal(
    contextMatches(
      { ...header, start_ns: "9000000000" },
      published,
      entry,
      vectors,
    ),
    false,
  );
  assert.equal(
    contextMatches(
      header,
      { ...published, sensor_id: "other" },
      entry,
      vectors,
    ),
    false,
  );
  assert.equal(
    contextMatches(header, published, entry, {
      ...vectors,
      placement: { check: { passed: false } },
    }),
    false,
  );
});
