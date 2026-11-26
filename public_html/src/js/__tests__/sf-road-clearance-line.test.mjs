import assert from "node:assert/strict";
import { test } from "node:test";
import { roadClearanceOffsets } from "../../../../tools/sf-drive/road-clearance-line.mjs";

const stations = Array.from({ length: 15 }, (_, i) => ({ s: 2 * i }));

test("a clear road keeps the centreline", () => {
  assert.deepEqual(
    roadClearanceOffsets(stations, 3.5, () => false),
    stations.map(() => 0),
  );
});

test("the guide steers around a footprint within the allowed lateral change", () => {
  const blocked = ({ s }, offset) => s >= 12 && s <= 16 && offset > -1;
  const offsets = roadClearanceOffsets(stations, 2, blocked);
  assert.equal(offsets[0], 0);
  assert.equal(offsets.at(-1), 0);
  for (const [i, offset] of offsets.entries()) {
    assert.ok(Math.abs(offset) <= 2);
    assert.equal(blocked(stations[i], offset), false);
    if (i) assert.ok(Math.abs(offset - offsets[i - 1]) <= 0.5);
  }
});

test("the guide rejects a cross-section without a clear lane", () => {
  assert.throws(
    () => roadClearanceOffsets(stations, 2, ({ s }) => s === 12),
    /No clear lane at 12 m/,
  );
});
