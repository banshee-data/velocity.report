// Deterministic capture preparation: the one function that turns a recipe's
// view spec into an exact, frozen camera and layer set. Used by both a
// URL-booted capture harness page and the capture tool driving that same
// page directly for every view after the first in one recipe — there is
// never a second, URL-only render path.
//
// Kept mostly pure (no THREE, no DOM) so it can be unit-tested directly; only
// applyCapturePose touches a real three.js camera.

import { SceneError } from "./scene-reader.js";
import {
  isDegenerateOrientation,
  toSceneX,
  toSceneY,
  toSceneZ,
} from "./scene-coords.js";

/**
 * Parses a capture view out of a URLSearchParams. Returns null when no
 * capture view is present (cam_x/cam_y/cam_z absent), so a plain scene page
 * load is unaffected.
 *
 * @param {URLSearchParams} searchParams
 */
export function parseCaptureParams(searchParams) {
  if (
    !searchParams.has("cam_x") &&
    !searchParams.has("cam_y") &&
    !searchParams.has("cam_z")
  ) {
    return null;
  }

  const num = (key) => {
    const raw = searchParams.get(key);
    return raw == null ? null : Number(raw);
  };
  const bool = (key, fallback) => {
    const raw = searchParams.get(key);
    return raw == null ? fallback : raw === "true" || raw === "1";
  };

  return {
    camera: { x: num("cam_x"), y: num("cam_y"), z: num("cam_z") },
    target: { x: num("tgt_x"), y: num("tgt_y"), z: num("tgt_z") },
    fovDeg: num("fov_deg"),
    layers: {
      lidar: bool("lidar", false),
      boxes: bool("boxes", true),
      trails: bool("trails", true),
      trailHistorySec: num("trail_history_sec") ?? 5,
    },
    frame: {
      timestampUs: num("timestamp_us"),
      frameIndex: num("frame_index"),
    },
  };
}

/** Inverse of parseCaptureParams — the URL a human could load to reproduce this exact view. */
export function serializeCaptureParams(viewSpec) {
  const params = new URLSearchParams();
  params.set("cam_x", String(viewSpec.camera.x));
  params.set("cam_y", String(viewSpec.camera.y));
  params.set("cam_z", String(viewSpec.camera.z));
  params.set("tgt_x", String(viewSpec.target.x));
  params.set("tgt_y", String(viewSpec.target.y));
  params.set("tgt_z", String(viewSpec.target.z));
  if (viewSpec.fovDeg != null) params.set("fov_deg", String(viewSpec.fovDeg));
  params.set("lidar", String(Boolean(viewSpec.layers?.lidar)));
  params.set("boxes", String(Boolean(viewSpec.layers?.boxes)));
  params.set("trails", String(Boolean(viewSpec.layers?.trails)));
  if (viewSpec.layers?.trailHistorySec != null) {
    params.set("trail_history_sec", String(viewSpec.layers.trailHistorySec));
  }
  if (viewSpec.frame?.timestampUs != null) {
    params.set("timestamp_us", String(viewSpec.frame.timestampUs));
  }
  if (viewSpec.frame?.frameIndex != null) {
    params.set("frame_index", String(viewSpec.frame.frameIndex));
  }
  return params;
}

/**
 * Resolves a capture frame selector against a SceneSession, strictly: an
 * out-of-range request throws rather than clamping, unlike the live player's
 * own scrubbing (session.locate), which must keep clamping for that UX.
 *
 * @returns {Promise<{partIndex: number, frame: object, us: number}>} us is
 *   the resolved frame's own timestamp within its part, in microseconds —
 *   the same unit trailHistoryForPart expects.
 */
export async function resolveFrameSelector(session, selector) {
  const hasTimestamp = selector?.timestampUs != null;
  const hasIndex = selector?.frameIndex != null;
  if (hasTimestamp === hasIndex) {
    throw new SceneError(
      "A capture frame selector needs exactly one of timestamp_us or frame_index",
    );
  }

  if (hasTimestamp) {
    const seconds = selector.timestampUs / 1e6;
    if (
      !Number.isFinite(seconds) ||
      seconds < 0 ||
      seconds > session.duration
    ) {
      throw new SceneError(
        `timestamp_us ${selector.timestampUs} is out of range for this scene ` +
          `(0-${Math.round(session.duration * 1e6)})`,
      );
    }
    const { partIndex, part, us } = session.locate(seconds);
    const frame = await part.frameAtOffset(us);
    if (!frame) throw new SceneError("No frame available at this timestamp");
    return { partIndex, frame, us };
  }

  const { partIndex, frame } = await session.frameAtIndex(selector.frameIndex);
  return { partIndex, frame, us: frame.t };
}

/**
 * Throws with a message a recipe author can act on when a view cannot be
 * rendered: non-finite coordinates, a camera sitting on its target, or a
 * viewing direction with no defined screen orientation.
 */
export function validateView(viewSpec) {
  const { camera, target, fovDeg } = viewSpec;
  for (const [label, point] of [
    ["camera", camera],
    ["target", target],
  ]) {
    for (const axis of ["x", "y", "z"]) {
      if (!Number.isFinite(point?.[axis])) {
        throw new SceneError(`${label}.${axis} must be a finite number`);
      }
    }
  }

  const dx = camera.x - target.x;
  const dy = camera.y - target.y;
  const dz = camera.z - target.z;
  if (Math.hypot(dx, dy, dz) < 1e-4) {
    throw new SceneError("camera and target must not be coincident");
  }
  if (isDegenerateOrientation(camera, target)) {
    throw new SceneError(
      "camera looks straight up or down at the target, which has no " +
        "defined screen orientation without a roll parameter",
    );
  }

  if (
    fovDeg != null &&
    (!Number.isFinite(fovDeg) || fovDeg <= 0 || fovDeg >= 180)
  ) {
    throw new SceneError(`fov_deg must be between 0 and 180, got ${fovDeg}`);
  }
}

/**
 * Points a real three.js camera at an exact ENU pose. The only function that
 * moves the camera for a capture view — no other code path sets its
 * position, so a URL-booted view and a script-driven one always agree.
 *
 * @param {import('three').PerspectiveCamera} camera
 * @param {{x:number,y:number,z:number}} cameraEnu
 * @param {{x:number,y:number,z:number}} targetEnu
 * @param {number|null} [fovDeg] resolved renderer default when omitted
 */
export function applyCapturePose(camera, cameraEnu, targetEnu, fovDeg) {
  camera.position.set(
    toSceneX(cameraEnu.x),
    toSceneY(cameraEnu.z),
    toSceneZ(cameraEnu.y),
  );
  // No roll parameter in the capture recipe: the camera always stays level.
  camera.up.set(0, 1, 0);
  camera.lookAt(
    toSceneX(targetEnu.x),
    toSceneY(targetEnu.z),
    toSceneZ(targetEnu.y),
  );
  if (fovDeg != null) camera.fov = fovDeg;
  camera.updateProjectionMatrix();
}
