import assert from "node:assert/strict";
import { test } from "node:test";
import { auditBakedRoadLinks } from "../../../../tools/sf-drive/baked-road-link-audit.mjs";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";

const seam = { width: 6, bearing: 0 };
const makeRoad = (id, start, groundEnds, seams) => ({
  id,
  kind: "ramp",
  width: 6,
  points: Array.from({ length: 16 }, (_, i) => [start + i * 2, 0, 3]),
  groundEnds,
  profiles: [],
  owners: ["808586ec", "808586ec"],
  seams,
});
const join = {
  id: "through",
  from: "incoming:end",
  to: "outgoing:start",
  direction: "forward",
  status: "joined",
  support: "interpolated",
};
const pavement = {
  o: [30, 0],
  g: { n: 3, s: 100, x0: -100, y0: -100, z: Array(9).fill(30) },
};

test("the link audit drives a directed join and records a ground exit that stays on the road", () => {
  const contacts = [
    makeRoad("incoming", 0, ["end"], { end: seam }),
    {
      ...makeRoad("outgoing", 30, ["end"], { start: seam }),
      groundExitOnly: ["end"],
    },
  ].map(buildBakedRoadContact);
  const audit = auditBakedRoadLinks(contacts, [join], [pavement]);
  assert.deepEqual(audit.directedJoins, {
    total: 1,
    passed: 1,
    adjusted: 0,
    failures: [],
  });
  assert.equal(audit.groundHandoffs.total, 3);
  assert.equal(audit.groundHandoffs.passed, 2);
  assert.deepEqual(audit.groundHandoffs.continued, [
    {
      road: "incoming",
      terminal: "incoming:end",
      direction: "exit",
      onto: "outgoing",
    },
  ]);
  assert.deepEqual(audit.groundHandoffs.failures, []);
});

test("a declared join does not pass a driving audit when the tyres cannot cross", () => {
  const contacts = [
    makeRoad("incoming", 0, [], { end: seam }),
    makeRoad("outgoing", 35, [], { start: seam }),
  ].map(buildBakedRoadContact);
  const { directedJoins } = auditBakedRoadLinks(contacts, [join], []);
  assert.equal(directedJoins.passed, 0);
  assert.equal(directedJoins.failures.length, 1);
  const [failure] = directedJoins.failures;
  assert.equal(failure.from, join.from);
  assert.equal(failure.to, join.to);
  assert.ok(failure.attempts.every(({ outcome }) => outcome === "stopped"));
});
