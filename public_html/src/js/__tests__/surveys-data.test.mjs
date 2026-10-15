import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

// The build-time data file reads the published scene assets, so this test
// holds the conventions it relies on against the real tree.
const require = createRequire(import.meta.url);
const loadSurveys = require("../../_data/surveys.js");

test("every published survey site resolves to a recording with a speed summary", async () => {
  const surveys = await loadSurveys();
  assert.ok(surveys.sites.length >= 25);
  assert.equal(surveys.totals.sites, surveys.sites.length);
  for (const site of surveys.sites.filter((s) => s.published)) {
    assert.ok(site.recordings.length >= 1, `${site.id} has no recording`);
    assert.ok(site.latest.speed, `${site.id} has no speed summary`);
    assert.ok(site.latest.hasPlayback, `${site.id} has no 3D playback`);
    assert.equal(site.latest.sensor, "lidar");
    assert.match(site.latest.id, /^\d{4}-\d{2}-\d{2}T\d{4}Z$/);
    assert.ok(site.latest.when.minutes > 0);
    assert.deepEqual(site.families, ["speed"]);
    assert.ok(site.years.every((y) => y >= 2025));
  }
  const columbus = surveys.byId["columbus-broadway"];
  assert.equal(columbus.latest.speed.count, 337);
  assert.equal(columbus.latest.speed.p85, 22.6);
  assert.equal(columbus.latest.build, "0.5.1-pre43");
  assert.equal(columbus.latest.assetsUrl, "/surveys/columbus-broadway/assets");
  assert.ok(surveys.pipeline.build);
  assert.deepEqual(surveys.sensorKinds, ["lidar"]);
});
