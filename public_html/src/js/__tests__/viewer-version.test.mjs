import { test } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
const require = createRequire(import.meta.url);
const build = require("../../_data/build.js");
test("viewer version comes from the canonical Makefile, independent of old recording versions", () => {
  const expected = readFileSync(
    new URL("../../../../Makefile", import.meta.url),
    "utf8",
  ).match(/^VERSION\s*:=\s*(\S+)/m)[1];
  assert.equal(build().version, expected);
  const template = readFileSync(
    new URL("../../_layouts/scene.njk", import.meta.url),
    "utf8",
  );
  const provenance = readFileSync(
    new URL("../../_includes/survey/provenance.njk", import.meta.url),
    "utf8",
  );
  assert.match(
    provenance,
    /data-scene-viewer-version[^>]*>\{\{ build.version \}\}/,
  );
  const badge = template.match(/<p class="scene-stage__build"[\s\S]*?<\/p>/)[0];
  assert.match(badge, /pipeline <span data-scene-generator-version>/);
  assert.doesNotMatch(badge, /viewer|build.version/);
  assert.doesNotMatch(provenance, /truncate/);
  const speedCard = readFileSync(
    new URL("../../_includes/survey/family-speed.njk", import.meta.url),
    "utf8",
  );
  assert.doesNotMatch(speedCard, /3D playback/);
});
