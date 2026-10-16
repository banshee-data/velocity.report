import test from "node:test";
import assert from "node:assert/strict";

import { recordingWhen, summariseSpeed } from "../survey-model.js";
import { surveyView } from "../survey-view.js";

const raw = {
  bucket_size: 5,
  track_count: 20,
  p50: 18,
  p85: 22.6,
  p98: 27,
  max: 33,
  histogram: [
    { start_mph: 15, count: 8 },
    { start_mph: 20, count: 10 },
    { start_mph: 25, count: 2 },
  ],
};
function rec(id, iso, extra = {}) {
  const startMs = Date.parse(iso);
  const when = recordingWhen(startMs, 1200);
  return {
    id,
    startMs,
    when,
    label: when.label,
    sensor: "lidar",
    build: "b1",
    families: ["speed"],
    speed: summariseSpeed(raw),
    hasPlayback: true,
    hasPointCloud: false,
    ...extra,
  };
}
const site = {
  id: "s",
  title: "Site",
  recordings: [
    rec("a", "2026-01-15T16:05:00Z", { sensor: "radar", hasPlayback: false }),
    rec("b", "2026-09-02T20:21:00Z", { hasPointCloud: true }),
  ],
};

test("the view applies the limit to shares, bands and the finding", () => {
  const none = surveyView(site, "");
  assert.equal(none.limit, null);
  assert.equal(none.latest.over, null);
  assert.equal(none.latest.overText, "—");
  assert.deepEqual(none.latest.bands, []);
  assert.doesNotMatch(none.latest.charts.wide, /LIMIT/);

  const v = surveyView(site, 25);
  assert.equal(v.limit, 25);
  assert.equal(v.latest.over, 0.1);
  assert.equal(v.latest.overText, "10.0%");
  assert.deepEqual(
    v.latest.bands.map((b) => b.from),
    [25],
  );
  assert.match(v.latest.finding.map((s) => s.text).join(""), /25 mph limit/);
  assert.match(v.latest.charts.wide, /LIMIT 25/);
  assert.match(v.latest.charts.spark, /<svg/);
});

test("the view says what the page can show", () => {
  const v = surveyView(site, null);
  assert.equal(v.canCompare, true);
  assert.equal(
    v.hasPlayback,
    true,
    "an older LiDAR recording keeps the player",
  );
  assert.equal(v.hasPointCloud, true);
  assert.equal(v.latest.isLatest, true);
  assert.notEqual(v.recordings[0].colour, v.recordings[1].colour);
  const one = surveyView(
    { ...site, recordings: site.recordings.slice(0, 1) },
    null,
  );
  assert.equal(one.canCompare, false);
  assert.equal(one.hasPlayback, false);
});

test("the client payload leaves the rendered parts behind", () => {
  const v = surveyView(site, 25);
  for (const r of v.client.recordings) {
    assert.ok(!("charts" in r) && !("finding" in r) && !("bands" in r));
    assert.ok("speed" in r && "over" in r && "hasPlayback" in r);
  }
  assert.deepEqual(Object.keys(v.client), [
    "id",
    "title",
    "limit",
    "scale",
    "ymax",
    "recordings",
  ]);
  assert.throws(() => surveyView(undefined, null), /not in scenes.json/);
});
