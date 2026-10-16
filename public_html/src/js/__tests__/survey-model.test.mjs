import test from "node:test";
import assert from "node:assert/strict";

import {
  FAMILIES,
  comparabilityNotes,
  datasetLinks,
  compareFinding,
  compareRows,
  compareSelection,
  excessBands,
  familyAvailability,
  recordingId,
  recordingWhen,
  sensorKind,
  shareAtOrAbove,
  speedFinding,
  summariseSpeed,
  validLimit,
} from "../survey-model.js";

// Columbus at Broadway, build 0.5.1-pre43.
const RAW = {
  units: "mph",
  bucket_size: 5,
  minimum_speed: 5,
  track_count: 337,
  p50: 18.63,
  p85: 22.59,
  p98: 26.82,
  max: 34.18,
  histogram: [
    { start_mph: 5, count: 33 },
    { start_mph: 10, count: 45 },
    { start_mph: 15, count: 138 },
    { start_mph: 20, count: 105 },
    { start_mph: 25, count: 14 },
    { start_mph: 30, count: 2 },
  ],
};

function recording(id, startISO, raw, extra = {}) {
  const startMs = Date.parse(startISO);
  const when = recordingWhen(startMs, 1200);
  return {
    id,
    startMs,
    when,
    label: when.label,
    build: "b1",
    sensor: "lidar",
    families: ["speed"],
    speed: summariseSpeed(raw),
    ...extra,
  };
}

test("speed summary rounds percentiles and turns counts into shares", () => {
  const s = summariseSpeed(RAW);
  assert.equal(s.count, 337);
  assert.deepEqual([s.p50, s.p85, s.p98, s.max], [18.6, 22.6, 26.8, 34.2]);
  assert.equal(s.bins.length, 6);
  assert.ok(Math.abs(s.bins.reduce((a, b) => a + b.share, 0) - 1) < 1e-9);
  assert.equal(summariseSpeed({ histogram: [] }), null);
  assert.equal(summariseSpeed(null), null);
});

test("a blank or invalid limit means no limit", () => {
  for (const v of [null, undefined, "", 0, -5, "abc"])
    assert.equal(validLimit(v), null);
  assert.equal(validLimit(25), 25);
  assert.equal(validLimit("30"), 30);
});

test("share at or above the limit is exact on a bucket edge and absent between edges", () => {
  const s = summariseSpeed(RAW);
  assert.ok(Math.abs(shareAtOrAbove(s, 25) - 16 / 337) < 1e-9);
  assert.equal(shareAtOrAbove(s, 27), null, "27 falls inside a bucket");
  assert.equal(shareAtOrAbove(s, 40), 0);
  assert.equal(shareAtOrAbove(s, null), null);
  assert.deepEqual(
    excessBands(s, 25).map((b) => [b.from, b.count]),
    [
      [25, 14],
      [30, 2],
    ],
  );
  assert.deepEqual(excessBands(s, null), []);
});

test("the finding mentions the limit only when one is set", () => {
  const s = summariseSpeed(RAW);
  const without = speedFinding(s, null)
    .map((x) => x.text)
    .join("");
  assert.equal(
    without,
    "Half of vehicles were at or below 18.6 mph and 85 in 100 at or below 22.6 mph. The fastest reached 34.2 mph.",
  );
  const withLimit = speedFinding(s, 25)
    .map((x) => x.text)
    .join("");
  assert.equal(
    withLimit,
    "Most drivers stayed under the 25 mph limit. 4.7% reached 25 mph or more; the fastest reached 34.2 mph.",
  );
});

test("sensor kind comes from the sensor id", () => {
  assert.equal(sensorKind("hesai-pandar40p"), "lidar");
  assert.equal(sensorKind("ops243-a"), "radar");
  assert.equal(sensorKind(undefined), "unknown");
});

test("recording ids and times are stable and site-local", () => {
  const ms = Number(1788380476586544330n / 1000000n);
  assert.equal(recordingId(ms), "2026-09-02T2021Z");
  const when = recordingWhen(ms, 1993);
  assert.equal(when.label, "Sep 2026");
  assert.equal(when.time, "Wed 13:21");
  assert.equal(when.context, "Weekday · midday");
  assert.equal(when.minutes, 33);
});

test("compare needs two recordings and defaults the baseline to the earliest", () => {
  const a = recording("a", "2026-01-15T16:05:00Z", RAW);
  const b = recording("b", "2026-09-02T20:21:00Z", RAW);
  assert.equal(compareSelection([a], ["a"]), null);
  assert.equal(compareSelection([a, b], ["a", "nope"]), null);
  const sel = compareSelection([b, a], ["b", "a"]);
  assert.deepEqual(
    sel.recordings.map((r) => r.id),
    ["a", "b"],
  );
  assert.equal(sel.base.id, "a");
  assert.equal(sel.latest.id, "b");
  // The newest recording is what changes are measured to, never the baseline.
  assert.equal(compareSelection([a, b], ["a", "b"], "b").base.id, "a");
  const c = recording("c", "2026-10-02T20:21:00Z", RAW);
  assert.equal(compareSelection([a, b, c], ["a", "b", "c"], "b").base.id, "b");
  assert.equal(compareSelection([a, b, c], ["a", "b", "c"], "c").base.id, "a");
});

test("recording labels carry the day when a site needs it", () => {
  const when = recordingWhen(Date.parse("2026-09-02T20:21:00Z"), 600);
  assert.equal(when.label, "Sep 2026");
  assert.equal(when.labelDay, "2 Sep 2026");
});

test("a recording without a pipeline build is a comparability warning", () => {
  const a = recording("a", "2026-01-15T16:05:00Z", RAW, { build: null });
  const b = recording("b", "2026-09-02T20:21:00Z", RAW);
  const notes = comparabilityNotes(compareSelection([a, b], ["a", "b"]));
  assert.equal(notes.length, 1);
  assert.equal(notes[0].tone, "warn");
  assert.match(notes[0].text, /^Jan 2026 does not record the pipeline build/);
  assert.doesNotMatch(notes[0].text, /null/);
});

test("compare rows give latest minus baseline, and add the limit row only with a limit", () => {
  const faster = {
    ...RAW,
    p85: 24.9,
    histogram: [
      { start_mph: 20, count: 60 },
      { start_mph: 25, count: 40 },
    ],
  };
  const a = recording("a", "2026-01-15T16:05:00Z", faster);
  const b = recording("b", "2026-09-02T20:21:00Z", RAW);
  const sel = compareSelection([a, b], ["a", "b"]);
  const rows = compareRows(sel, null);
  assert.deepEqual(
    rows.map((r) => r.key),
    ["p50", "p85", "p98", "max"],
  );
  assert.equal(rows[1].delta, -2.3);
  const limited = compareRows(sel, 25);
  const over = limited.at(-1);
  assert.equal(over.key, "over");
  assert.equal(over.delta, Math.round((16 / 337 - 0.4) * 1000) / 10);
  const text = compareFinding(sel, 25)
    .map((s) => s.text)
    .join("");
  assert.match(text, /^P85 fell 2\.3 mph to 22\.6 mph/);
  assert.match(text, /from 40\.0% to 4\.7%\.$/);
});

test("comparability notes flag weekends, mixed builds and mixed sensors", () => {
  const a = recording("a", "2026-06-13T18:20:00Z", RAW, {
    sensor: "radar",
    families: ["speed"],
  });
  const b = recording("b", "2026-09-02T20:21:00Z", RAW, { build: "b2" });
  const notes = comparabilityNotes(compareSelection([a, b], ["a", "b"]));
  assert.equal(notes.length, 3);
  assert.match(notes[0].text, /weekend recording/);
  assert.match(notes[1].text, /different pipeline builds/);
  assert.match(notes[2].text, /radar survey/);
  const c = recording("c", "2026-09-01T20:00:00Z", RAW, { build: "b2" });
  const same = comparabilityNotes(compareSelection([c, b], ["c", "b"]));
  assert.deepEqual(
    same.map((n) => n.tone),
    ["info"],
  );
  assert.match(same[0].text, /^Both were processed/);
});

test("family availability distinguishes not recorded by the sensor from not reported", () => {
  const extra = { id: "stop", name: "Stop compliance", sensors: ["lidar"] };
  FAMILIES.push(extra);
  try {
    const a = recording("a", "2026-01-15T16:05:00Z", RAW, { sensor: "radar" });
    const b = recording("b", "2026-06-02T20:21:00Z", RAW);
    const c = recording("c", "2026-09-02T20:21:00Z", RAW, {
      families: ["speed", "stop"],
    });
    const avail = familyAvailability(
      compareSelection([a, b, c], ["a", "b", "c"]),
    );
    assert.deepEqual(avail, [
      { family: extra, cells: ["not-recorded", "missing", "measured"] },
    ]);
  } finally {
    FAMILIES.pop();
  }
  assert.equal(FAMILIES.length, 1);
});

test("dataset links name the file, its page and its format from the path", () => {
  const links = datasetLinks(
    "https://huggingface.co/datasets/x/y/",
    "raw/lidar/a.pcapng",
  );
  assert.equal(
    links.download,
    "https://huggingface.co/datasets/x/y/resolve/main/raw/lidar/a.pcapng?download=true",
  );
  assert.equal(
    links.page,
    "https://huggingface.co/datasets/x/y/blob/main/raw/lidar/a.pcapng",
  );
  assert.equal(links.format, "PCAPNG");
  assert.equal(datasetLinks("https://h/d", "raw/radar/a.log").format, "LOG");
  assert.equal(datasetLinks("https://h/d", "raw/radar/noext").format, null);
  assert.equal(datasetLinks(null, "raw/lidar/a.pcapng"), null);
  assert.equal(datasetLinks("https://h/d", null), null);
});
