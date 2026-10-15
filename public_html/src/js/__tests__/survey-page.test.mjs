import test from "node:test";
import assert from "node:assert/strict";

import { recordingWhen, summariseSpeed } from "../survey-model.js";
import {
  compareCSV,
  compareSearch,
  filterRows,
  readSurveyState,
  renderCompare,
  sortRows,
} from "../survey-page.js";

const raw = (p85) => ({
  bucket_size: 5,
  track_count: 10,
  p50: 18,
  p85,
  p98: 27,
  max: 33,
  histogram: [
    { start_mph: 15, count: 4 },
    { start_mph: 20, count: 5 },
    { start_mph: 25, count: 1 },
  ],
});
function rec(id, iso, p85, sensor = "lidar") {
  const startMs = Date.parse(iso);
  const when = recordingWhen(startMs, 1200);
  return {
    id,
    startMs,
    when,
    label: when.label,
    sensor,
    build: "b1",
    families: ["speed"],
    speed: summariseSpeed(raw(p85)),
    over: 0.1,
    assetsUrl: `/x/${id}`,
    hasPlayback: sensor === "lidar",
  };
}
const data = {
  id: "site",
  title: "Site",
  limit: 25,
  recordings: [
    rec("jan", "2026-01-15T16:05:00Z", 23.8, "radar"),
    rec("sep", "2026-09-02T20:21:00Z", 22.6),
  ],
};
const json = JSON.stringify(data);

test("state defaults to the latest recording and reads ?recording and ?compare", () => {
  assert.equal(readSurveyState(json, "").recording.id, "sep");
  assert.equal(readSurveyState(json, "").compare, null);
  assert.equal(readSurveyState(json, "?recording=jan").recording.id, "jan");
  assert.equal(readSurveyState(json, "?recording=nope").recording.id, "sep");
  const { compare } = readSurveyState(json, "?compare=sep,jan&base=sep");
  assert.deepEqual(
    compare.recordings.map((r) => r.id),
    ["jan", "sep"],
  );
  assert.equal(compare.base.id, "jan", "the newest cannot be the baseline");
  assert.equal(readSurveyState(json, "?compare=sep").compare, null);
  assert.equal(readSurveyState(json, "?compare=sep").compareRequested, true);
  assert.equal(readSurveyState(json, "").compareRequested, false);
});

test("compare drops recordings without a speed summary and keeps the newest four", () => {
  const bare = {
    ...rec("bare", "2026-03-01T16:05:00Z", 20),
    speed: null,
    families: [],
  };
  const withBare = JSON.stringify({
    ...data,
    recordings: [data.recordings[0], bare, data.recordings[1]],
  });
  assert.equal(
    readSurveyState(withBare, "?compare=bare,sep").compare,
    null,
    "one speed-bearing recording is not a comparison",
  );
  const two = readSurveyState(withBare, "?compare=bare,sep,jan").compare;
  assert.deepEqual(
    two.recordings.map((r) => r.id),
    ["jan", "sep"],
  );

  const many = [1, 2, 3, 4, 5].map((n) =>
    rec(`r${n}`, `2026-0${n}-01T16:05:00Z`, 20 + n),
  );
  const sel = readSurveyState(
    JSON.stringify({ ...data, recordings: many }),
    "?compare=r1,r2,r3,r4,r5&base=r1",
  ).compare;
  assert.deepEqual(
    sel.recordings.map((r) => r.id),
    ["r2", "r3", "r4", "r5"],
  );
  assert.equal(
    sel.base.id,
    "r2",
    "a baseline that was cut falls back to the earliest kept",
  );
  const html = renderCompare({ ...data, recordings: many }, sel);
  assert.doesNotMatch(html, /undefined|NaN|null/);
  assert.match(html, /COMPARING 4/);
});

test("compare URLs keep commas readable", () => {
  assert.equal(
    compareSearch(["jan", "sep"], "jan"),
    "?compare=jan,sep&base=jan",
  );
});

test("the compare view names the baseline, the change and the sensor mix", () => {
  const { compare } = readSurveyState(json, "?compare=jan,sep");
  const html = renderCompare(data, compare);
  assert.match(html, /COMPARING 2/);
  assert.match(html, /BASELINE/);
  assert.match(html, /P85 fell <b>1\.2 mph<\/b> to 22\.6 mph/);
  assert.match(html, /Radar \+ LiDAR/);
  assert.match(html, /was a radar survey/);
  assert.doesNotMatch(
    html,
    /Other measurements across recordings/,
    "no other families exist yet",
  );
  assert.equal(
    compareCSV(data, compare).split("\n")[0],
    "metric,jan,sep,delta_vs_jan",
  );
});

test("listing filters match name or cell and sort missing values last", () => {
  const rows = [
    {
      site: "a",
      name: "Columbus at Broadway",
      cell: "80858-0f4",
      recordings: "2",
      years: "2026 2026 ",
      p85: "22.6",
    },
    {
      site: "b",
      name: "Bush at Powell",
      cell: "80858-08c",
      recordings: "1",
      years: "2025 ",
      p85: "",
    },
    {
      site: "c",
      name: "Ashbury at Downey",
      cell: "80858-0ac",
      recordings: "1",
      years: "2026 ",
      p85: "25.8",
    },
  ];
  assert.deepEqual(
    filterRows(rows, "broad", []).map((r) => r.site),
    ["a"],
  );
  assert.deepEqual(
    filterRows(rows, "08c", []).map((r) => r.site),
    ["b"],
  );
  assert.deepEqual(
    filterRows(rows, "", ["multi"]).map((r) => r.site),
    ["a"],
  );
  assert.deepEqual(
    filterRows(rows, "", ["year:2025"]).map((r) => r.site),
    ["b"],
  );
  assert.deepEqual(
    sortRows(rows, "p85", 1).map((r) => r.site),
    ["a", "c", "b"],
  );
  assert.deepEqual(
    sortRows(rows, "p85", -1).map((r) => r.site),
    ["c", "a", "b"],
  );
  assert.deepEqual(
    sortRows(rows, "name", 1).map((r) => r.site),
    ["c", "b", "a"],
  );
});
