// Camera poses in the game's east/north/up frame. The overhead view keeps the
// car's forward direction at the top, matching the relative emoji seeker.
export const DRIVE_VIEWS = ["zoomed", "overhead", "incar"];

// View-space metres. Complete the fade before the far plane so geometry
// disappears into the same sky colour before it is clipped.
export const DRIVE_VIEW_RANGE = {
  near: 0.4,
  far: 360,
  fogNear: 155,
  fogFar: 320,
};
export const MIRROR_VIEW_RANGE = {
  near: 0.4,
  far: 160,
  fogNear: 55,
  fogFar: 140,
};
export const CHASE_DISTANCE = 34;
const DECK_PROBES = [-CHASE_DISTANCE, -28, -22, -16, -10, -4, 0, 6, 12, 18];
const CAMERA_DECK_MARGIN = 0.5;

// Body roll leans outward from the turn. Damp the response so changing the
// steering input or crossing onto dirt does not snap the horizon.
export function incarRoll(previous, lateralAcceleration, dt) {
  const maxRoll = (6 * Math.PI) / 180;
  const target = Math.max(-1, Math.min(1, lateralAcceleration / 7.5)) * maxRoll;
  return previous + (target - previous) * (1 - Math.exp(-dt / 0.16));
}

function cameraRoadStation(profile, { x, y }) {
  let distanceSquared = Infinity;
  let lateralSquared = Infinity;
  let station = 0;
  for (let i = 1; i < profile.length; i++) {
    const a = profile[i - 1];
    const b = profile[i];
    const [east, north] = a.position;
    const dx = b.position[0] - east;
    const dy = b.position[1] - north;
    const projected =
      ((x - east) * dx + (y - north) * dy) / (dx * dx + dy * dy);
    const fraction = Math.min(1, Math.max(0, projected));
    const score =
      (x - east - dx * fraction) ** 2 + (y - north - dy * fraction) ** 2;
    if (score < distanceSquared) {
      distanceSquared = score;
      // The chase camera may still be under the deck after the car hands
      // off to city terrain. Extend the last tangent for that departure.
      const along =
        i === profile.length - 1 ? Math.max(fraction, projected) : fraction;
      station = a.s_m + (b.s_m - a.s_m) * along;
      lateralSquared =
        (x - east - dx * along) ** 2 + (y - north - dy * along) ** 2;
    }
  }
  return { station, lateralSquared };
}

export function inTunnelInterior(surface, car) {
  if (surface?.kind !== "tunnel") return false;
  const interior = surface.appearance?.interior;
  // Older authored drives treat the full tunnel section as enclosed.
  if (!interior) return true;
  const { station } = cameraRoadStation(surface.profile, car);
  return station >= interior[0] && station <= interior[1];
}

export function coveredRoadBlend(surface, car) {
  const range = surface?.coveredRange;
  if (!range) return 0;
  const [start, end] = range;
  const { station, lateralSquared } = cameraRoadStation(surface.profile, car);
  // Retire the covered view when a car leaves the corridor sideways.
  if (lateralSquared > 20 ** 2) return 0;
  const distance = Math.max(0, start - station, station - end - CHASE_DISTANCE);
  const fraction = Math.max(0, 1 - distance / 24);
  return fraction * fraction * (3 - 2 * fraction);
}

export function overheadDeckBlend(
  car,
  z,
  heightAt,
  ceiling,
  departureHeightAt,
) {
  const hx = Math.cos(car.heading);
  const hy = Math.sin(car.heading);
  let blend = 0;
  for (const distance of DECK_PROBES) {
    const x = car.x + hx * distance;
    const y = car.y + hy * distance;
    // A rear probe can still be over the road the car just drove down.
    const pavement =
      (distance < 0 ? departureHeightAt?.(x, y) : null) ?? heightAt(x, y);
    if (!Number.isFinite(pavement)) continue;
    // Each probe compares the deck with the selected road's pavement at
    // that point. A slope or a neighbouring ramp may sit above the car.
    const deck = ceiling(x, y, Math.max(pavement, z) + 2);
    if (deck > pavement + 26) continue;
    const fraction = 1 - Math.max(0, distance - 6, -distance - 22) / 18;
    blend = Math.max(blend, fraction * fraction * (3 - 2 * fraction));
  }
  return blend;
}

export function driveCameraPose(
  view,
  car,
  z,
  ground = () => z,
  roll = 0,
  skylineLift = 0,
  coveredBlend = 0,
  deckCeiling,
) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  if (view === "overhead") {
    return {
      position: [car.x, car.y, z + 85],
      look: [car.x, car.y, z],
      up: [hx, hy, 0],
      attached: true,
      hideCar: false,
      mirror: false,
    };
  }
  if (view === "incar") {
    const slope = Math.max(
      -0.4,
      Math.min(
        0.4,
        ((ground(car.x + hx * 3, car.y + hy * 3) ?? z) -
          (ground(car.x - hx * 3, car.y - hy * 3) ?? z)) /
          6,
      ),
    );
    return {
      position: [car.x + hx * 0.35, car.y + hy * 0.35, z + 1.45 + slope * 0.35],
      look: [car.x + hx * 30, car.y + hy * 30, z + 1.45 + slope * 30],
      up: [hy * Math.sin(roll), -hx * Math.sin(roll), Math.cos(roll)],
      attached: true,
      hideCar: true,
      mirror: true,
    };
  }
  const followDistance = CHASE_DISTANCE - 26.5 * coveredBlend;
  const lift = 26 - 22.6 * coveredBlend;
  const lookDistance = 14 - 5 * coveredBlend;
  const x = car.x - hx * followDistance;
  const y = car.y - hy * followDistance;
  const support = Math.max(ground(x, y) ?? z, z);
  const ceiling =
    coveredBlend > 0 && deckCeiling ? deckCeiling(x, y, support + 2) : Infinity;
  return {
    position: [x, y, Math.min(support + lift, ceiling - CAMERA_DECK_MARGIN)],
    look: [
      car.x + hx * lookDistance,
      car.y + hy * lookDistance,
      z + 1.4 - 0.35 * coveredBlend + skylineLift,
    ],
    up: [0, 0, 1],
    attached: false,
    hideCar: false,
    mirror: false,
  };
}
