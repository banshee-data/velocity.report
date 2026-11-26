import assert from "node:assert/strict";
import { test } from "node:test";
import {
  auditBakedRoadLinks,
  auditBakedRoadSpans,
} from "../../../../tools/sf-drive/baked-road-link-audit.mjs";
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
    scrapes: [],
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

test("the span audit drives the directions allowed by the road terminals", () => {
  const roads = [
    makeRoad("two-way", 0, ["start", "end"]),
    {
      ...makeRoad("exit-only", 60, ["start", "end"]),
      groundExitOnly: ["end"],
    },
  ].map(buildBakedRoadContact);
  const audit = auditBakedRoadSpans(roads, [], new Map());
  assert.equal(audit.testedSurfaces, 2);
  assert.equal(audit.testedDirections, 3);
  assert.equal(audit.passed, 3);
  assert.deepEqual(audit.failures, []);
});

test("the span audit expands bidirectional and reverse-only joins", () => {
  const roads = [
    makeRoad("west", 0, ["start"], { end: seam }),
    makeRoad("middle", 30, [], { start: seam, end: seam }),
    makeRoad("east", 60, ["end"], { start: seam }),
  ].map(buildBakedRoadContact);
  const joins = [
    {
      ...join,
      id: "western",
      from: "west:end",
      to: "middle:start",
      direction: "both",
    },
    {
      ...join,
      id: "eastern",
      from: "east:start",
      to: "middle:end",
      direction: "both",
    },
  ];
  const bidirectional = auditBakedRoadSpans(roads, joins, new Map());
  assert.equal(bidirectional.testedDirections, 6);
  assert.equal(bidirectional.passed, 6);
  const reverse = auditBakedRoadSpans(
    roads,
    [joins[0], { ...joins[1], direction: "reverse" }],
    new Map(),
  );
  assert.equal(reverse.testedDirections, 4);
  assert.equal(reverse.passed, 4);
});

test("the span audit drives against the baked guardrails", () => {
  const ramp = {
    ...makeRoad("hairpin", 0, ["start", "end"]),
    width: 3,
    points: Array.from({ length: 25 }, (_, i) => {
      const angle = (Math.PI * i) / 24;
      return [Math.cos(angle) * 12, Math.sin(angle) * 12, 3];
    }),
  };
  const run = (road) =>
    auditBakedRoadSpans([buildBakedRoadContact(road)], [], new Map());
  assert.equal(run(ramp).passed, 2);
  const withRails = run({ ...ramp, railSides: ["left", "right"] });
  assert.equal(withRails.passed, 0);
  assert.equal(withRails.failures.length, 2);
  assert.ok(
    withRails.failures.every(({ attempts }) =>
      attempts.some(({ outcome }) => outcome === "rail contact"),
    ),
  );
});

test("the span audit reaches an obstacle between otherwise sound terminals", () => {
  const road = buildBakedRoadContact(makeRoad("blocked", 0, ["start", "end"]));
  const tree = {
    kind: "tree",
    tile: "808586ec",
    index: 7,
    x: 15,
    y: 0,
    base: 3,
    top: 5,
    radius: 5,
  };
  const collisions = new Map([
    ["0,-1", [tree]],
    ["0,0", [tree]],
  ]);
  const audit = auditBakedRoadSpans([road], [], collisions);
  assert.equal(audit.passed, 0);
  assert.equal(audit.failures.length, 2);
  for (const { attempts } of audit.failures) {
    assert.ok(attempts.every(({ metres }) => metres > 2));
    assert.ok(attempts.every(({ lastScrape }) => lastScrape?.row === 7));
  }
});

test("the link audit checks scenery on the ground approach to an otherwise supported ramp", () => {
  const road = buildBakedRoadContact(makeRoad("ramp", 0, ["start", "end"]));
  const tree = {
    kind: "tree",
    tile: "808586ec",
    index: 7,
    x: -6,
    y: 0,
    base: 3,
    top: 5,
    radius: 5,
  };
  const collisions = new Map([
    ["-1,-1", [tree]],
    ["-1,0", [tree]],
  ]);
  const clear = auditBakedRoadLinks([road], [], [pavement], new Map());
  assert.equal(clear.groundHandoffs.passed, 4);
  const blocked = auditBakedRoadLinks([road], [], [pavement], collisions);
  const entry = blocked.groundHandoffs.failures.find(
    ({ direction }) => direction === "entry",
  );
  assert.ok(entry);
  assert.ok(entry.attempts.some(({ lastScrape }) => lastScrape?.row === 7));
});
