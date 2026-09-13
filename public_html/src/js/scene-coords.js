// Coordinate math shared by the scene player and the deterministic capture
// tooling (tools/scene-capture). Kept pure and framework-free so the capture
// tool's bullet-time path generator can import it directly under plain Node,
// with no three.js or DOM dependency, and so the two never drift apart.
//
// Sensor data is ENU: X east, Y north, Z up. Three.js is Y-up, so east stays
// X, up becomes Y, and north becomes -Z to keep the frame right-handed.
export const toSceneX = (x) => x;
export const toSceneY = (z) => z;
export const toSceneZ = (y) => -y;

const deg2rad = (d) => (d * Math.PI) / 180;
const rad2deg = (r) => (r * 180) / Math.PI;
const mod360 = (d) => ((d % 360) + 360) % 360;

/**
 * Converts an azimuth/elevation/radius around a target into an explicit ENU
 * camera position.
 *
 * azimuthDeg is the compass bearing, 0 = north, increasing clockwise.
 * elevationDeg is the angle above the target's horizontal plane (0 = level,
 * 90 = directly overhead) — not the angle from vertical used internally by
 * the orbit camera's "polar" convention.
 *
 * Re-derived from scene-camera.js's apply() composed with the ENU->scene
 * transform above, not an independent formula: at azimuth 0 the camera sits
 * south of the target looking north, matching what the orbit camera actually
 * draws for azimuth_deg 0 today, whatever a vantage's doc comment implies.
 */
export function enuFromSpherical({ azimuthDeg, elevationDeg, radius, target }) {
  const az = deg2rad(azimuthDeg);
  const el = deg2rad(elevationDeg);
  const horiz = radius * Math.cos(el);
  return {
    x: target.x + horiz * Math.sin(az),
    y: target.y - horiz * Math.cos(az),
    z: target.z + radius * Math.sin(el),
  };
}

/** Inverse of enuFromSpherical: recovers azimuth/elevation/radius from a camera and target. */
export function sphericalFromEnu({ camera, target }) {
  const dx = camera.x - target.x;
  const dy = camera.y - target.y;
  const dz = camera.z - target.z;
  const radius = Math.hypot(dx, dy, dz);
  if (radius === 0) return { azimuthDeg: 0, elevationDeg: 0, radius: 0 };
  return {
    azimuthDeg: mod360(rad2deg(Math.atan2(dx, -dy))),
    elevationDeg: rad2deg(Math.asin(dz / radius)),
    radius,
  };
}

/**
 * True when a camera looks straight up or down at a target, in ENU.
 *
 * There is no roll parameter in the capture recipe, so a viewing direction
 * parallel to world-up has no defined "which way is up on screen" — any
 * horizontal direction is equally valid. Checked as the dot product of the
 * normalised view direction with the up axis, rather than via elevation
 * degrees, so it also catches a camera placed by explicit X/Y/Z rather than
 * by azimuth/elevation.
 */
export function isDegenerateOrientation(camera, target, epsilon = 1e-6) {
  const dx = target.x - camera.x;
  const dy = target.y - camera.y;
  const dz = target.z - camera.z;
  const len = Math.hypot(dx, dy, dz);
  if (len === 0) return true;
  return Math.abs(dz / len) > 1 - epsilon;
}
