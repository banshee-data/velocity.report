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

test("full recordings require exact catalogue-bound review windows across gaps", () => {
  const frame = { frame_id: "sensor", reference_frame: "sensor" };
  const sha = "sha256:" + "a".repeat(64);
  const header = {
    site: "site",
    sensor_id: "sensor",
    coordinate_frame: frame,
    start_ns: "1000000000",
    duration_sec: 8,
    following: {
      review_scope: {
        schema: "following_static_review_v1",
        catalog_sha256: sha,
        site: "site",
        periods: [
          { start_ns: "1000000000", end_ns: "3000000000" },
          { start_ns: "6000000000", end_ns: "9000000001" },
        ],
      },
    },
  };
  const published = { sensor_id: "sensor", coordinate_frame: frame },
    entry = {
      id: "site",
      static_parts: [
        { start: "1970-01-01T00:00:00Z", end: "1970-01-01T00:00:03Z" },
        { start: "1970-01-01T00:00:06Z", end: "1970-01-01T00:00:10Z" },
      ],
    },
    vectors = { site: "site", placement: { check: { passed: true } } };
  assert.equal(contextMatches(header, published, entry, vectors, sha), true);
  assert.equal(contextMatches(header, published, entry, vectors), false);
  for (const change of [
    (s) => (s.schema = "future"),
    (s) => (s.site = "other"),
    (s) => (s.catalog_sha256 = "changed"),
    (s) => (s.periods[0].end_ns = "6000000000"),
    (s) => s.periods.pop(),
    (s) => (s.periods[0].start_ns = "bad"),
  ]) {
    const bad = structuredClone(header);
    change(bad.following.review_scope);
    assert.equal(contextMatches(bad, published, entry, vectors, sha), false);
  }
  assert.equal(
    contextMatches(
      header,
      published,
      { ...entry, static_parts: [] },
      vectors,
      sha,
    ),
    false,
  );
  assert.equal(
    contextMatches(
      header,
      published,
      { ...entry, static_parts: [{ start: "bad", end: "bad" }] },
      vectors,
      sha,
    ),
    false,
  );
  assert.equal(
    contextMatches(
      header,
      published,
      entry,
      { ...vectors, placement: { check: { passed: false } } },
      sha,
    ),
    false,
  );
});
