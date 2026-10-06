// Virtual drone: a slow, constant-speed circuit of a scene's vantages.
//
// A static three-quarter view of a junction tells a reader less than a slow
// circle around it, which shows the same traffic from every approach without
// anyone having to touch a control. The flight runs on its own clock, so it
// keeps moving whether or not the recording is playing.
//
// The vantages are treated as points on a circle rather than as stations to
// stop at. Flying vantage-to-vantage means accelerating, arriving, holding and
// setting off again four times a lap, and every one of those is a jolt for
// whoever is watching. Instead one orbit is fitted to all of them and swept at
// a constant rate: no stops, no easing, no change of elevation, one continuous
// turn. It passes through every vantage's bearing exactly, and through the
// vantage itself as closely as a single circle can — which, for a junction
// surveyed from four equivalent approaches, is exactly.
//
// This module is pure: it takes a time in seconds and returns the viewpoint
// the camera should be at. Nothing here touches the DOM, a camera or three.js.

/** Seconds for one full revolution. 40 s is about 9 degrees a second. */
export const DEFAULT_CIRCUIT_SEC = 40;

const mod360 = (d) => ((d % 360) + 360) % 360;

/**
 * The vantages a flight takes into account.
 *
 * `"fly": false` opts a vantage out. An overhead plan view and a wide
 * establishing shot are useful to pick deliberately and wrong to average into
 * an orbit — one would flatten the circle onto the ground, the other would
 * push it out past the scene. Everything else joins by default, because a
 * vantage worth naming is usually worth flying past.
 *
 * Sorted by bearing, so a lap starts where the circle does and the order
 * `velocity scene vantages` prints is the order it flies.
 */
export function flightPath(vantages) {
  return (vantages ?? [])
    .filter((v) => v && v.fly !== false)
    .slice()
    .sort((a, b) => (a.azimuth_deg ?? 0) - (b.azimuth_deg ?? 0));
}

/**
 * Fits one orbit to a set of vantages: the elevation and distance the circle
 * is flown at.
 *
 * A plain mean. Something more elaborate — a curve leaning toward each vantage
 * as it passes — would put back the rising and falling this exists to remove.
 * Where the vantages agree, as four equivalent approaches to a junction do,
 * the mean is each of them exactly.
 *
 * The per-vantage offsets are deliberately not carried across. An offset shifts
 * what a single view looks at, to frame one approach from where the sensor
 * happens to stand — it belongs to that view and not to a circuit of all of
 * them. Averaging them moves the centre of the orbit off the scene by whatever
 * those hand-tuned nudges happen to sum to, which is nothing anyone chose. An
 * orbit goes round the scene.
 */
export function orbitFrom(path) {
  const n = path.length;
  if (n === 0) return null;
  const mean = (key) =>
    path.reduce((sum, v) => sum + (Number(v[key]) || 0), 0) / n;
  return {
    polar_deg: mean("polar_deg"),
    zoom: mean("zoom"),
  };
}

/**
 * A looping orbit over a scene's vantages.
 *
 * @param {object} opts
 * @param {Array} opts.vantages the scene's full list; those opted out are skipped
 * @param {number} [opts.circuitSeconds] seconds for one revolution
 */
export function createSceneFlight({
  vantages,
  circuitSeconds = DEFAULT_CIRCUIT_SEC,
} = {}) {
  const path = flightPath(vantages);
  const orbit = orbitFrom(path);
  // Start where the first vantage stands, so a lap begins on a named view
  // rather than at an arbitrary bearing.
  const start = path[0]?.azimuth_deg ?? 0;
  const cycle = circuitSeconds > 0 ? circuitSeconds : DEFAULT_CIRCUIT_SEC;
  /** Degrees per second, and constant: that is the whole point. */
  const rate = 360 / cycle;

  /** The viewpoint to be at `seconds` into the flight. */
  function sample(seconds) {
    if (!orbit) return null;
    return { ...orbit, azimuth_deg: mod360(start + seconds * rate) };
  }

  /**
   * When the orbit next points at a given bearing.
   *
   * Turning the drone back on picks up from the view already on screen rather
   * than cutting to wherever its clock had drifted, so the camera carries on
   * turning from where the reader left it.
   */
  function phaseFor(azimuthDeg) {
    return mod360((Number(azimuthDeg) || 0) - start) / rate;
  }

  return { sample, phaseFor, path, orbit, cycle, rate };
}
