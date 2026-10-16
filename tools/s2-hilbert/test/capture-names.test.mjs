import assert from "node:assert/strict";
import test from "node:test";

import {
  DATASET_URL,
  captureStem,
  rawLidarPath,
  timestampParts,
  tokenDisplay,
} from "../../s2-archive/capture-names.mjs";

// Columbus at Broadway as site-index.json records it.
const COLUMBUS = {
  id: "columbus-broadway",
  start: "2026-09-02T13:20:47.000000-07:00",
  lat: 37.7978,
  lon: -122.4066,
};

test("the filename timestamp is the site-local start to the minute", () => {
  assert.equal(timestampParts("2026-09-02T13:20:47-07:00"), "20260902T1320");
  // The same instant written in UTC names the same file.
  assert.equal(timestampParts("2026-09-02T20:20:47Z"), "20260902T1320");
  assert.equal(tokenDisplay("808580f4"), "80858-0f4");
});

test("a site's capture is named by its start, its L13 cell and its id", () => {
  assert.equal(
    captureStem(COLUMBUS),
    "20260902T1320_80858-0f4_columbus-broadway",
  );
  assert.equal(
    rawLidarPath(COLUMBUS),
    "raw/lidar/20260902T1320_80858-0f4_columbus-broadway.pcapng",
  );
  assert.match(
    DATASET_URL,
    /^https:\/\/huggingface\.co\/datasets\/banshee-data\/sf-street-speeds$/,
  );
});

test("a site with no position cannot be named yet", () => {
  assert.equal(captureStem({ ...COLUMBUS, lat: null, lon: null }), null);
  assert.equal(rawLidarPath({ id: "x", start: COLUMBUS.start }), null);
});
