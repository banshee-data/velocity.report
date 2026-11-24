import { carStep } from "./sf-drive-core.js";
// Authored local longitudinal metres; no surveyed street controls or traffic inference.
export const APPROACH = Object.freeze({
  id: "downhill-v1",
  dt: 1 / 120,
  slope: -0.06,
  line: 110,
  crossing: 118,
  end: 145,
  length: 4.6,
});
const car = (x, v) => ({ x, y: 0, heading: 0, v, steering: 0 });
export const signalAt = (t) => (t < 3 ? "green" : t < 6 ? "amber" : "red");
export function stopOutcome(front) {
  return front >= APPROACH.crossing
    ? "Entered crosswalk"
    : front > APPROACH.line
      ? "Past limit line · short of crosswalk"
      : "Before limit line";
}
export function following(player, lead) {
  if (
    !lead ||
    ![player.x, player.y, player.v, lead.x, lead.y, lead.v].every(
      Number.isFinite,
    ) ||
    Math.abs(player.y - lead.y) > 1 ||
    lead.x <= player.x
  )
    return { status: "unavailable" };
  const gap = lead.x - player.x - APPROACH.length;
  const relative = player.v - lead.v;
  const closing = Math.abs(relative) < 1e-6 ? 0 : relative;
  return {
    gap,
    closing,
    headway: player.v > 0.05 && gap > 0 ? gap / player.v : null,
    stoppedGap: player.v === 0 ? gap : null,
    drac: gap > 0 && closing > 0 ? closing ** 2 / (2 * gap) : null,
    status: gap <= 0 ? "contact" : closing > 0 ? "closing" : "non-closing",
  };
}
export function createApproach() {
  return {
    tick: 0,
    player: car(0, 10),
    lead: car(80, 10),
    frames: [],
    event: null,
    replay: false,
    crossed: null,
    contact: false,
    finished: false,
    previousAcceleration: 0,
  };
}
export function stepApproach(run, input) {
  if (run.finished) return;
  const dt = APPROACH.dt,
    t = run.tick * dt;
  const before = run.player;
  // The scripted actor observes only its clock and speed.
  // Same carStep and grade as the player; brake at 6s and remain stopped.
  const leadInput =
    t >= 6
      ? { brake: 0.35 }
      : { throttle: Math.max(0, (10 - run.lead.v) * 0.3) };
  run.lead = carStep(run.lead, leadInput, dt, { slope: APPROACH.slope });
  if (input.brake && before.v > 0.05 && !run.event)
    run.event = {
      start: t,
      x: before.x,
      speed: before.v,
      peakDeceleration: 0,
      peakJerk: 0,
      frames: [],
      end: null,
    };
  run.player = carStep(
    before,
    { throttle: input.brake ? 0 : input.throttle, brake: input.brake },
    dt,
    { slope: APPROACH.slope },
  );
  const acceleration = (run.player.v - before.v) / dt;
  const front = run.player.x + APPROACH.length / 2;
  if (run.crossed === null && front > APPROACH.line) run.crossed = signalAt(t);
  const gap = following(run.player, run.lead);
  if (gap.status === "contact") {
    run.contact = true;
    run.finished = true; // End on contact; never turn an impact into a successful braking stop.
  }
  run.tick++;
  const frame = {
    t: run.tick * dt,
    player: { ...run.player },
    lead: { ...run.lead },
    signal: signalAt(t),
    gap,
    brake: input.brake || 0,
  };
  // Store 20Hz recorded states; physics and extrema remain 120Hz.
  if (run.tick % 6 === 0) run.frames.push(frame);
  const event = run.event;
  if (event && event.end === null) {
    event.peakDeceleration = Math.max(event.peakDeceleration, -acceleration);
    event.peakJerk = Math.max(
      event.peakJerk,
      Math.abs(acceleration - run.previousAcceleration) / dt,
    );
    if (run.tick % 6 === 0) event.frames.push(frame);
    if (run.player.v === 0 && !run.contact) {
      event.end = frame.t;
      event.distance = run.player.x - event.x;
      event.outcome = stopOutcome(front);
      event.signal = run.crossed || "Did not cross line";
      event.frames.push(frame);
    }
  }
  run.previousAcceleration = acceleration;
  if (run.player.v !== 0 || run.contact) run.replay = false;
  if (front >= APPROACH.end || run.tick >= 120 * 90) run.finished = true;
}
export function openReplay(run) {
  run.replay =
    run.player.v === 0 &&
    !run.contact &&
    run.event?.end !== null &&
    !!run.event?.frames.length;
  return run.replay;
}
