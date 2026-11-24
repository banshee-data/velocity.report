import test from "node:test";
import assert from "node:assert/strict";
import {
  APPROACH,
  createApproach,
  stepApproach,
  following,
  signalAt,
  stopOutcome,
  openReplay,
} from "../sf-following.js";
test("authored boundaries distinguish limit overrun from crossing", () => {
  assert.equal(stopOutcome(110), "Before limit line");
  assert.equal(stopOutcome(110.01), "Past limit line · short of crosswalk");
  assert.equal(stopOutcome(117.99), "Past limit line · short of crosswalk");
  assert.equal(stopOutcome(118), "Entered crosswalk");
  assert.deepEqual(
    [signalAt(0), signalAt(3), signalAt(6)],
    ["green", "amber", "red"],
  );
});
test("supported moving, closing and stopped gaps; absent and off-path lead", () => {
  const p = { x: 0, y: 0, v: 10 },
    l = { x: 24.6, y: 0, v: 10 };
  assert.equal(following(p, l).headway, 2);
  assert.equal(following(p, l).drac, null);
  assert.equal(following(p, { ...l, v: 0 }).drac, 2.5);
  assert.equal(following({ ...p, v: 0 }, { ...l, v: 0 }).stoppedGap, 20);
  assert.equal(following(p, null).status, "unavailable");
  assert.equal(following(p, { ...l, y: 5 }).status, "unavailable");
  assert.equal(following(p, { ...l, x: 4 }).status, "contact");
});
function attempt(brakeAt) {
  const r = createApproach();
  for (let i = 0; i < 2400; i++)
    stepApproach(r, { brake: i >= brakeAt ? 1 : 0, throttle: 0 });
  return r;
}
test("same fixed inputs yield identical trace and deterministic lead", () => {
  assert.deepEqual(attempt(300), attempt(300));
  const a = attempt(0),
    b = attempt(300);
  assert.deepEqual(a.lead, b.lead);
  assert.equal(a.lead.v, 0);
  assert.ok(a.lead.x - 2.3 > APPROACH.crossing);
});
test("brake onset, stop distance/time, extrema, recorded replay and renewed movement", () => {
  const r = attempt(300),
    e = r.event;
  assert.equal(e.start, 2.5);
  assert.ok(
    e.distance > 0 &&
      e.end > e.start &&
      e.peakDeceleration > 0 &&
      e.peakJerk > 0,
  );
  assert.equal(e.outcome, "Before limit line");
  assert.equal(e.frames.at(-1).player.v, 0);
  assert.equal(openReplay(r), true);
  stepApproach(r, { throttle: 1, brake: 0 });
  assert.equal(r.replay, false);
  assert.equal(openReplay(r), false);
  assert.equal(openReplay(createApproach()), false);
});
test("late response ends in contact without manufacturing a completed stop", () => {
  const r = createApproach();
  for (let i = 0; i < 2400; i++) stepApproach(r, { throttle: 1 });
  assert.equal(r.contact, true);
  assert.equal(r.finished, true);
  assert.equal(openReplay(r), false);
});
test("early release leaves first event open until actual rest", () => {
  const r = createApproach();
  for (let i = 0; i < 60; i++) stepApproach(r, { brake: 1 });
  for (let i = 0; i < 60; i++) stepApproach(r, { throttle: 1 });
  assert.equal(r.event.end, null);
  assert.equal(openReplay(r), false);
});
test("real approach runs distinguish early, line-only and crosswalk stops", () => {
  const early = attempt(1020),
    line = attempt(1080),
    crossing = attempt(1104);
  assert.equal(early.event.outcome, "Before limit line");
  assert.equal(line.event.outcome, "Past limit line · short of crosswalk");
  assert.equal(crossing.event.outcome, "Entered crosswalk");
  assert.equal(line.event.signal, "red");
  assert.equal(crossing.contact, false);
});

test("non-finite observations are unavailable and numerical rest is not closing", () => {
  const p = { x: 0, y: 0, v: 0 };
  assert.equal(following(p, { x: 20, y: 0, v: NaN }).status, "unavailable");
  assert.equal(following(p, { x: 20, y: 0, v: -1e-15 }).status, "non-closing");
  assert.equal(following(p, { x: 20, y: 0, v: -1e-15 }).drac, null);
});
