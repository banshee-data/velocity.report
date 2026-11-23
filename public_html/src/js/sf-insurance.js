// The session ledger and rates are in cents. Trial checkpoints never store this ledger.
export const MANUAL_RATE = 0.001; // cents per second
const MPH = 2.2369362920544;
const CRASH_CENTS_PER_JERK = 0.00001;

export function formatInsuranceCents(cents) {
  return `${cents.toFixed(cents > 0 && cents < 1 ? 4 : 2)}¢`;
}
export function formatInsuranceDollars(cents) {
  const dollars = cents / 100;
  return `$${dollars.toFixed(dollars > 0 && dollars < 0.01 ? 6 : 4)}`;
}

export function gameInsuranceLimit(car, surface) {
  if (!car?.grounded || !Number.isFinite(car.z)) return null;
  return surface?.kind === "bridge_deck"
    ? 50
    : surface?.kind === "ramp"
      ? 30
      : 25;
}

export function speedMultiplier(mph, limit) {
  if (!Number.isFinite(mph) || !Number.isFinite(limit) || limit <= 0) return 1;
  const steps = Math.max(0, Math.ceil((Math.abs(mph) - limit) / 5 - 1e-9));
  return 2 ** steps;
}

export function recordCrash(run, peakJerk) {
  run.infractions++;
  if (run.mode !== "manual" || !Number.isFinite(peakJerk) || peakJerk <= 0)
    return 0;
  const charge = peakJerk * CRASH_CENTS_PER_JERK;
  run.spent += charge;
  return charge;
}

export const BONUS_RADIUS = 40;

const bonuses = {
  free: { label: "Free insurance", rate: 0, seconds: 10 },
  discount: {
    label: "Half-rate insurance",
    rate: MANUAL_RATE / 2,
    seconds: 20,
  },
};

export function createInsurance(automatic, stops = []) {
  return {
    mode: automatic ? "automatic" : "ready",
    manualSeconds: 0,
    spent: 0,
    infractions: 0,
    limit: null,
    multiplier: 1,
    stops,
    delivered: 0,
    collected: new Set(),
    free: 0,
    discount: 0,
  };
}

export function insuranceRate(run) {
  if (run.mode !== "manual") return 0;
  if (run.free > 0) return bonuses.free.rate;
  if (run.discount > 0) return bonuses.discount.rate * run.multiplier;
  return MANUAL_RATE * run.multiplier;
}

export function advanceInsurance(run, seconds, speedMps = 0, limit = null) {
  if (run.mode !== "manual" || !Number.isFinite(seconds) || seconds <= 0)
    return;
  run.limit = limit;
  run.multiplier = speedMultiplier(speedMps * MPH, limit);
  run.manualSeconds += seconds;
  for (const key of ["free", "discount"]) {
    const used = Math.min(seconds, run[key]);
    run[key] -= used;
    run.spent += used * bonuses[key].rate * run.multiplier;
    seconds -= used;
  }
  run.spent += seconds * MANUAL_RATE * run.multiplier;
}

export function visitSurvey(run, site, position, streetHeight, index) {
  if (
    !position.grounded ||
    !Number.isFinite(streetHeight) ||
    !Number.isFinite(position.z) ||
    Math.abs(position.z - streetHeight) > 2 ||
    Math.hypot(position.x - site.x, position.y - site.y) > BONUS_RADIUS
  )
    return null;
  const delivery = run.stops[run.delivered] === site.id;
  if (delivery) run.delivered++;
  if (run.collected.has(site.id)) return delivery ? "Delivery complete" : null;
  run.collected.add(site.id);
  const key = index % 2 ? "discount" : "free";
  const { label, seconds } = bonuses[key];
  run[key] += seconds;
  return `${delivery ? "Delivery complete · " : ""}${label} for ${seconds} s`;
}
