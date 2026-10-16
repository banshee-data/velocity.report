import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);
const { loadSurveys } = require("../../_data/surveys.js");

// The build-time data file reads the published scene assets, so this test
// holds the conventions it relies on against the real tree.
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
    assert.match(site.latest.summaryUrl, /part-000\/timeline\.json$/);
  }
  const columbus = surveys.byId["columbus-broadway"];
  assert.equal(columbus.latest.speed.count, 337);
  assert.equal(columbus.latest.speed.p85, 22.6);
  assert.equal(columbus.latest.build, "0.5.1-pre43");
  assert.equal(columbus.latest.assetsUrl, "/surveys/columbus-broadway/assets");
  assert.equal(columbus.latest.hasPointCloud, true);
  assert.ok(surveys.pipeline.build);
  assert.deepEqual(surveys.sensorKinds, ["lidar"]);
});

// A scratch scenes tree exercising the paths no published site takes yet.
const SPEED = {
  units: "mph",
  bucket_size: 5,
  minimum_speed: 5,
  track_count: 4,
  p50: 18,
  p85: 22,
  p98: 26,
  max: 30,
  histogram: [
    { start_mph: 15, count: 2 },
    { start_mph: 20, count: 2 },
  ],
};
const INDEX = {
  sites: [
    {
      id: "x",
      title: "X",
      page: "/surveys/x/",
      summary: "",
      published: true,
      located: false,
      cells: null,
    },
  ],
};

function scratch(files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "surveys-"));
  for (const [rel, body] of Object.entries(files)) {
    const file = path.join(root, rel);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(
      file,
      typeof body === "string" ? body : JSON.stringify(body),
    );
  }
  return root;
}
const header = (ns, extra = {}) => ({
  sensor_id: "hesai-pandar40p",
  start_ns: String(ns),
  duration_sec: 600,
  build_version: "b1",
  ...extra,
});
const NS = 1788380476586544330n; // 2026-09-02T20:21Z

test("a radar summary, a same-month visit and a same-minute pair are told apart", async () => {
  const root = scratch({
    "x/assets/manifest.json": { version: 1 },
    "x/assets/part-000/header.json": header(NS),
    "x/assets/part-000/timeline.json": { vehicle_speed: SPEED },
    "x/recordings/radar-same-minute/summary.json": {
      sensor_id: "ops243-a",
      start: "2026-09-02T20:21:30Z",
      duration_sec: 900,
      build_version: "b1",
      vehicle_speed: SPEED,
    },
    "x/recordings/later-in-month/summary.json": {
      sensor_id: "ops243-a",
      start: "2026-09-20T18:00:00Z",
      duration_sec: 900,
      vehicle_speed: SPEED,
    },
    "x/recordings/january/summary.json": {
      sensor_id: "ops243-a",
      start: "2026-01-15T16:05:00Z",
      duration_sec: 900,
      build_version: "b0",
      vehicle_speed: SPEED,
    },
  });
  const warnings = [];
  const { sites, sensorKinds, totals } = await loadSurveys({
    scenesDir: root,
    index: INDEX,
    warn: (m) => warnings.push(m),
  });
  const [site] = sites;
  assert.deepEqual(
    site.recordings.map((r) => r.label),
    ["Jan 2026", "2 Sep 2026", "2 Sep 2026", "20 Sep 2026"],
  );
  assert.deepEqual(
    site.recordings.map((r) => r.id),
    [
      "2026-01-15T1605Z",
      "2026-09-02T2021Z",
      "2026-09-02T2021Z-radar",
      "2026-09-20T1800Z",
    ],
  );
  assert.deepEqual(
    site.recordings.map((r) => r.sensor),
    ["radar", "lidar", "radar", "radar"],
  );
  assert.deepEqual(
    site.recordings.map((r) => r.hasPlayback),
    [false, true, false, false],
  );
  assert.equal(
    site.recordings[1].summaryUrl,
    "/surveys/x/assets/part-000/timeline.json",
  );
  assert.equal(
    site.recordings[2].summaryUrl,
    "/surveys/x/recordings/radar-same-minute/summary.json",
  );
  assert.equal(site.recordings[3].build, null);
  assert.deepEqual(site.sensors, ["radar", "lidar"]);
  assert.deepEqual(sensorKinds, ["radar", "lidar"]);
  assert.equal(totals.multi, 1);
  assert.equal(warnings.length, 1);
  assert.match(
    warnings[0],
    /two recordings start at 2026-09-02T2021Z; using 2026-09-02T2021Z-radar/,
  );
});

test("a recording with no measurement is left out with a warning, and a site without assets is empty", async () => {
  const root = scratch({
    "x/assets/manifest.json": { version: 1 },
    "x/assets/part-000/header.json": header(NS),
  });
  const warnings = [];
  const { sites } = await loadSurveys({
    scenesDir: root,
    index: INDEX,
    warn: (m) => warnings.push(m),
  });
  assert.deepEqual(sites[0].recordings, []);
  assert.equal(sites[0].latest, null);
  assert.match(
    warnings[0],
    /x: \/surveys\/x\/assets reports no measurement; left out/,
  );

  const empty = await loadSurveys({
    scenesDir: scratch({}),
    index: INDEX,
    warn: () => {},
  });
  assert.equal(empty.sites[0].latest, null);
  assert.equal(empty.totals.withRecordings, 0);
});

test("a broken file fails the build naming the file", async () => {
  const bad = scratch({ "x/assets/part-000/header.json": "{ not json" });
  await assert.rejects(
    loadSurveys({ scenesDir: bad, index: INDEX }),
    /x\/assets\/part-000\/header\.json: /,
  );
  const noStart = scratch({
    "x/assets/part-000/header.json": { sensor_id: "hesai-pandar40p" },
  });
  await assert.rejects(
    loadSurveys({ scenesDir: noStart, index: INDEX }),
    /x\/assets: no readable start time/,
  );
  const badStart = scratch({
    "x/recordings/r/summary.json": { start: "yesterday", vehicle_speed: SPEED },
  });
  await assert.rejects(
    loadSurveys({ scenesDir: badStart, index: INDEX }),
    /recordings\/r: no readable start time/,
  );
});
