// Milestone 2: expands a bullet_time spec into an explicit list of
// camera/target views orbiting a frozen target, and feeds them through the
// identical milestone 1 views[] pipeline — no separate render path, and no
// relation to the live player's own drone orbit (scene-flight.js), which
// fits a mean circle to named vantages for a different purpose entirely.

import {
  enuFromSpherical,
  sphericalFromEnu,
} from "../../public_html/src/js/scene-coords.js";

function clamp01(t) {
  return Math.max(0, Math.min(1, t));
}

/** Classic smoothstep: zero slope at both ends, no overshoot. */
function smoothstep(t) {
  const c = clamp01(t);
  return c * c * (3 - 2 * c);
}

/**
 * Piecewise-smoothstep interpolation across ascending {t, deg} keyframes
 * spanning [0, 1]. Each segment is eased independently between its own two
 * keyframes, so the result never overshoots past the values it is
 * interpolating between — unlike a single spline fitted through all of them.
 */
export function interpolateElevation(keyframes, t) {
  const target = clamp01(t);
  for (let i = 1; i < keyframes.length; i++) {
    const a = keyframes[i - 1];
    const b = keyframes[i];
    if (target <= b.t) {
      const span = b.t - a.t;
      const local = span > 0 ? (target - a.t) / span : 0;
      return a.deg + (b.deg - a.deg) * smoothstep(local);
    }
  }
  return keyframes[keyframes.length - 1].deg;
}

/**
 * Resolves a bullet_time.target against an already-resolved frame's tracks.
 * An explicit point passes straight through; an object_id must be present in
 * this exact frame — a stale id from a different frame is a hard error, not
 * a silent fallback.
 */
export function resolveBulletTimeTarget(target, tracks) {
  if (target.point) return target.point;
  const track = tracks.find((t) => t.id === target.objectId);
  if (!track) {
    throw new Error(
      `bullet_time target object_id "${target.objectId}" was not found in the resolved frame`,
    );
  }
  return { x: track.x, y: track.y, z: track.z };
}

/**
 * Expands one bullet_time spec into `imageCount` named views, azimuth
 * advancing linearly across [arcStartDeg, arcEndDeg] relative to the base
 * view's own azimuth around `target`, elevation following
 * interpolateElevation, at a fixed radius (explicit, or derived from the
 * base view's own distance to target).
 *
 * @param {object} bulletTime parsed recipe.bulletTime (see recipe.mjs)
 * @param {{camera: {x,y,z}, fovDeg: number|null}} baseView the recipe view
 *   bulletTime.baseView names
 * @param {{x:number,y:number,z:number}} target resolved target point
 * @returns {{name:string, camera:object, target:object, fovDeg:number|null}[]}
 */
export function expandBulletTime(bulletTime, baseView, target) {
  const baseSpherical = sphericalFromEnu({ camera: baseView.camera, target });
  const radius = bulletTime.radiusM ?? baseSpherical.radius;
  const { imageCount, arcStartDeg, arcEndDeg, elevationKeyframes } = bulletTime;

  const views = [];
  for (let i = 0; i < imageCount; i++) {
    const t = imageCount > 1 ? i / (imageCount - 1) : 0;
    const azimuthDeg =
      baseSpherical.azimuthDeg + arcStartDeg + (arcEndDeg - arcStartDeg) * t;
    const elevationDeg = interpolateElevation(elevationKeyframes, t);
    const camera = enuFromSpherical({
      azimuthDeg,
      elevationDeg,
      radius,
      target,
    });
    views.push({
      name: `bullet-time-${String(i).padStart(3, "0")}`,
      camera,
      target,
      fovDeg: baseView.fovDeg,
    });
  }
  return views;
}
