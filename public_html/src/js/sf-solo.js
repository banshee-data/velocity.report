// World: deliberately authored practice course, NOT a surveyed SF street or limit.
// Coordinates are local metres. The smooth crest has radius ~63.3 m at its top.
import { carStep } from "./sf-drive-core.js";
export const SOLO = Object.freeze({
  id: "crest-practice-v1",
  start: -150,
  end: 150,
  halfWidth: 6,
  limitMph: 20,
});
export const MPH = 2.2369362920544;
export function soloHeight(x) {
  return Math.abs(x) < 50 ? (8 * (1 + Math.cos((Math.PI * x) / 50))) / 2 : 0;
}
/** Body pitch in radians, measured along the car's forward axis. */
export function soloPitch(car) {
  if (!car.grounded) return Math.atan2(car.vz, Math.max(0.1, Math.abs(car.v)));
  const forwardX = Math.cos(car.heading) * 0.1;
  // The authored profile varies only with x; facing across it gives zero pitch.
  return Math.atan2(
    soloHeight(car.x + forwardX) - soloHeight(car.x - forwardX),
    0.2,
  );
}

export function soloSurface(x, y) {
  const known = x >= SOLO.start - 20 && x <= SOLO.end + 40 && Math.abs(y) <= 40;
  return {
    height: known ? soloHeight(x) : null,
    drivable: known ? Math.abs(y) <= SOLO.halfWidth : null,
    limit:
      known && Math.abs(y) <= SOLO.halfWidth && x < 100
        ? {
            mph: SOLO.limitMph,
            source: "Authored practice limit · crest-practice-v1",
          }
        : null,
  };
}
export function soloCar() {
  return {
    x: SOLO.start,
    y: 0,
    z: 0,
    heading: 0,
    v: 0,
    vz: 0,
    grounded: true,
    steering: 0,
  };
}

/** Physics: contact follows supplied geometry; missing geometry freezes rather than inventing a landing.
 * A support acceleration below -g releases contact. Airborne velocity has no tyre/input forces.
 * Fixed 120 Hz caller; point support is a game approximation, not a suspension model.
 */
export function contactStep(car, input, dt, surfaceAt) {
  if (!(dt > 0) || dt > 0.05)
    throw new RangeError("contact step must be in (0, .05]");
  const here = surfaceAt(car.x, car.y);
  if (here.height === null) return { ...car, blocked: true };
  const z = car.z ?? here.height;
  let next;
  if (car.grounded !== false) {
    const a = surfaceAt(
      car.x + Math.cos(car.heading) * 0.1,
      car.y + Math.sin(car.heading) * 0.1,
    );
    const b = surfaceAt(
      car.x - Math.cos(car.heading) * 0.1,
      car.y - Math.sin(car.heading) * 0.1,
    );
    if (a.height === null || b.height === null)
      return { ...car, blocked: true };
    next = carStep(car, input, dt, {
      slope: (a.height - b.height) / 0.2,
      offroad: here.drivable === false,
    });
  } else {
    next = {
      ...car,
      x: car.x + car.vx * dt,
      y: car.y + car.vy * dt,
      lateralAcceleration: 0,
    };
  }
  const there = surfaceAt(next.x, next.y);
  if (there.height === null) return { ...car, blocked: true };
  const ballisticZ = z + (car.vz || 0) * dt - 0.5 * 9.81 * dt * dt;
  if (car.grounded !== false && there.height >= ballisticZ - 1e-7) {
    return {
      ...next,
      z: there.height,
      // Use the endpoint tangent, not a backward secant (which would release
      // contact at half gravity on a curved surface).
      vz:
        (((surfaceAt(
          next.x + Math.cos(next.heading) * 0.1,
          next.y + Math.sin(next.heading) * 0.1,
        ).height ?? there.height) -
          (surfaceAt(
            next.x - Math.cos(next.heading) * 0.1,
            next.y - Math.sin(next.heading) * 0.1,
          ).height ?? there.height)) /
          0.2) *
        next.v,
      grounded: true,
      blocked: false,
    };
  }
  if (car.grounded !== false) {
    // Recompute the launch interval from existing momentum: no launch-frame steering/braking.
    next = {
      ...car,
      x: car.x + Math.cos(car.heading) * car.v * dt,
      y: car.y + Math.sin(car.heading) * car.v * dt,
      vx: Math.cos(car.heading) * car.v,
      vy: Math.sin(car.heading) * car.v,
      lateralAcceleration: 0,
    };
  }
  const landing = surfaceAt(next.x, next.y);
  if (landing.height === null) return { ...car, blocked: true };
  const vz = (car.vz || 0) - 9.81 * dt;
  if (ballisticZ <= landing.height) {
    const ahead = surfaceAt(
      next.x + Math.cos(next.heading) * 0.1,
      next.y + Math.sin(next.heading) * 0.1,
    );
    const tangent =
      ahead.height === null
        ? 0
        : ((ahead.height - landing.height) / 0.1) * next.v;
    return {
      ...next,
      z: landing.height,
      vz: tangent,
      grounded: true,
      blocked: false,
    };
  }
  return { ...next, z: ballisticZ, vz, grounded: false, blocked: false };
}

/** Data: simulation seconds, explicit unknown coverage, run-local traces. No score weights. */
export function createSoloRun() {
  return {
    fixture: SOLO.id,
    status: "ready",
    elapsed: 0,
    evaluated: 0,
    excluded: 0,
    known: 0,
    unknown: 0,
    compliant: 0,
    bands: [0, 0, 0],
    peak: 0,
    excursions: [],
    strikes: [],
    incidents: [],
    limits: [],
    activeExcursion: null,
    activeIncident: null,
    touching: false,
    limitKey: undefined,
  };
}
export function startSoloRun(run) {
  if (run.status === "ready") run.status = "running";
}
export function endSoloRun(run, status = "finished") {
  if (run.status !== "running" && run.status !== "ready") return;
  if (run.activeExcursion !== null) {
    run.excursions[run.activeExcursion].end = run.elapsed;
    run.excursions[run.activeExcursion].endReason = status;
  }
  run.activeExcursion = null;
  if (run.activeIncident !== null)
    run.incidents[run.activeIncident].end = run.elapsed;
  run.activeIncident = null;
  run.touching = false;
  run.status = status;
}
export function speedBand(excess) {
  return excess <= 0 ? -1 : excess < 5 ? 0 : excess < 10 ? 1 : 2;
}
export function accountSolo(run, dt, sample) {
  if (run.status !== "running" || !(dt > 0)) return;
  const at = run.elapsed;
  run.elapsed += dt;
  if (sample.excluded) {
    run.excluded += dt;
    return;
  }
  run.evaluated += dt;
  const known =
    Number.isFinite(sample.limit?.mph) &&
    sample.limit.mph > 0 &&
    !!sample.limit.source;
  const key = known ? `${sample.limit.mph}:${sample.limit.source}` : "unknown";
  if (key !== run.limitKey) {
    run.limits.push({
      at,
      mph: known ? sample.limit.mph : null,
      source: known ? sample.limit.source : null,
    });
    run.limitKey = key;
  }
  if (known) {
    run.known += dt;
    const excess = Math.max(0, sample.mph - sample.limit.mph);
    run.peak = Math.max(run.peak, excess);
    const band = speedBand(excess);
    if (band < 0) run.compliant += dt;
    else run.bands[band] += dt;
  } else run.unknown += dt;
  // A manoeuvre ends only on confirmed full re-entry, clear of the kerb.
  // All strikes within that manoeuvre share its incident, but retain separate trace rows.
  const incident = () => {
    if (run.activeIncident === null) {
      run.activeIncident = run.incidents.length;
      run.incidents.push({ start: at, end: null });
    }
    return run.activeIncident;
  };
  if (sample.curbContact && !run.touching) {
    run.strikes.push({
      at,
      incident: incident(),
      mph: sample.mph,
      normalMps: sample.normalMps || 0,
    });
  }
  run.touching = !!sample.curbContact;
  if (sample.drivable === false && run.activeExcursion === null) {
    run.activeExcursion = run.excursions.length;
    run.excursions.push({
      start: at,
      end: null,
      seconds: 0,
      peakMph: 0,
      incident: incident(),
    });
  }
  if (sample.drivable === false) {
    const event = run.excursions[run.activeExcursion];
    event.seconds += dt;
    event.peakMph = Math.max(event.peakMph, sample.mph);
  } else if (sample.drivable === true && run.activeExcursion !== null) {
    run.excursions[run.activeExcursion].end = at;
    run.excursions[run.activeExcursion].endReason = "re-entry";
    run.activeExcursion = null;
  }
  if (
    sample.drivable === true &&
    sample.clearOfCurb &&
    run.activeIncident !== null
  ) {
    run.incidents[run.activeIncident].end = at;
    run.activeIncident = null;
  }
}

// Physics/Data: swept tyre-envelope crossing of the authored 15 cm kerbs only.
// Air above kerb height cannot strike it; sustained overlap is one contact.
export function soloSample(before, after) {
  const surface = soloSurface(after.x, after.y);
  const extent = (c) =>
    Math.abs(Math.cos(c.heading)) * 0.96 + Math.abs(Math.sin(c.heading)) * 2.23;
  const a = Math.abs(before.y) + extent(before),
    b = Math.abs(after.y) + extent(after);
  const low =
    Math.min(before.z - soloHeight(before.x), after.z - soloHeight(after.x)) <=
    0.15;
  const crosses = Math.min(a, b) <= 6.1 && Math.max(a, b) >= 5.9;
  const overlaps = Math.abs(after.y) - extent(after) <= 6.1 && b >= 5.9;
  return {
    ...surface,
    mph: Math.abs(after.v) * MPH,
    curbContact: low && (crosses || overlaps),
    clearOfCurb: b < 5.8,
    normalMps: Math.abs(after.v * Math.sin(after.heading)),
  };
}
export function soloSummary(run) {
  const seconds = (n) => `${n.toFixed(1)} s`;
  return (
    `${run.status} · elapsed ${seconds(run.elapsed)} (no time reward). ` +
    `Known limit ${seconds(run.known)} / ${seconds(run.evaluated)} evaluated; unknown ${seconds(run.unknown)}; excluded ${seconds(run.excluded)}. ` +
    `Excess (0,5): ${seconds(run.bands[0])}; [5,10): ${seconds(run.bands[1])}; ≥10 mph: ${seconds(run.bands[2])}; peak ${run.peak.toFixed(1)} mph. ` +
    `${run.excursions.length} excursions, ${run.strikes.length} kerb strikes, ${run.incidents.length} distinct manoeuvres. No weighted score; calibration awaits paired playtests.`
  );
}
