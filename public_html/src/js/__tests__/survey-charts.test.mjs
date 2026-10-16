import test from "node:test";
import assert from "node:assert/strict";

import { summariseSpeed } from "../survey-model.js";
import {
  histogramSvg,
  percentileRowsSvg,
  recordingColour,
  shareCeiling,
  sparklineSvg,
  speedScale,
} from "../survey-charts.js";

const speed = summariseSpeed({
  bucket_size: 5,
  p50: 18.6,
  p85: 22.6,
  p98: 26.8,
  max: 34.2,
  histogram: [
    { start_mph: 15, count: 3 },
    { start_mph: 20, count: 6 },
    { start_mph: 25, count: 1 },
  ],
});

test("the histogram draws a limit line and red bars only when a limit is set", () => {
  const plain = histogramSvg(speed, { width: 400, height: 200 });
  assert.doesNotMatch(plain, /LIMIT/);
  assert.doesNotMatch(plain, /rx="1.5" fill="#f87171"/, "no red bars");
  const limited = histogramSvg(speed, { width: 400, height: 200, limit: 25 });
  assert.match(limited, /LIMIT 25/);
  assert.equal(
    (limited.match(/fill="#f87171" opacity="0.85"/g) ?? []).length,
    1,
    "one bar at or above 25",
  );
  assert.match(limited, /aria-label="[^"]*posted limit 25 mph"/);
});

test("the shared scale and ceiling cover every recording", () => {
  assert.deepEqual(speedScale([speed], null), { lo: 5, hi: 40 });
  assert.deepEqual(speedScale([speed], 40), { lo: 5, hi: 45 });
  assert.equal(shareCeiling([speed]), 60);
  assert.equal(shareCeiling([]), 5);
});

test("percentile rows and sparklines escape labels and tolerate no limit", () => {
  const svg = percentileRowsSvg(
    [{ label: "Jan <2026>", colour: "#fff", speed }],
    { width: 300 },
  );
  assert.match(svg, /Jan &lt;2026&gt;/);
  assert.doesNotMatch(svg, /LIMIT/);
  assert.match(sparklineSvg(speed), /^<svg/);
  assert.equal(sparklineSvg(null), "");
});

test("the latest recording always takes the latest colour", () => {
  assert.equal(recordingColour(2, 3), "#6ee7b7");
  assert.equal(recordingColour(0, 1), "#6ee7b7");
  assert.notEqual(recordingColour(0, 3), "#6ee7b7");
});
