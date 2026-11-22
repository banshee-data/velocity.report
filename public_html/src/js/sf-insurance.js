// The session ledger and rates are in cents. Trial checkpoints never store this ledger.
export const MANUAL_RATE = 0.12;

export function formatInsuranceCents(cents) {
  return `${cents.toFixed(2)}¢`;
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
  if (run.discount > 0) return bonuses.discount.rate;
  return MANUAL_RATE;
}

export function advanceInsurance(run, seconds) {
  if (run.mode !== "manual" || !Number.isFinite(seconds) || seconds <= 0)
    return;
  run.manualSeconds += seconds;
  for (const key of ["free", "discount"]) {
    const used = Math.min(seconds, run[key]);
    run[key] -= used;
    run.spent += used * bonuses[key].rate;
    seconds -= used;
  }
  run.spent += seconds * MANUAL_RATE;
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
