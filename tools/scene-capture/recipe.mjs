// Parses and validates a capture recipe: a JSON file describing a source
// scene export, one frozen frame, 1-50 named camera views, explicit layer
// visibility, a viewport, and an output destination.
//
// Validation is strict and happens up front, before any browser is launched,
// so a malformed recipe fails with one clear message rather than partway
// through a capture run.

import { readFile } from "node:fs/promises";
import path from "node:path";

export class RecipeError extends Error {}

const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/;

function requireNumber(value, label) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new RecipeError(`${label} must be a finite number`);
  }
  return value;
}

function requirePoint(value, label) {
  if (value == null || typeof value !== "object") {
    throw new RecipeError(`${label} must be an {x, y, z} object`);
  }
  return {
    x: requireNumber(value.x, `${label}.x`),
    y: requireNumber(value.y, `${label}.y`),
    z: requireNumber(value.z, `${label}.z`),
  };
}

function parseFrameSelector(frame) {
  const hasTimestamp = frame?.timestamp_us != null;
  const hasIndex = frame?.frame_index != null;
  if (hasTimestamp === hasIndex) {
    throw new RecipeError(
      "frame must specify exactly one of timestamp_us or frame_index",
    );
  }
  return hasTimestamp
    ? { timestampUs: requireNumber(frame.timestamp_us, "frame.timestamp_us") }
    : { frameIndex: requireNumber(frame.frame_index, "frame.frame_index") };
}

function parseView(raw, index) {
  if (raw == null || typeof raw !== "object") {
    throw new RecipeError(`views[${index}] must be an object`);
  }
  if (typeof raw.name !== "string" || !NAME_PATTERN.test(raw.name)) {
    throw new RecipeError(
      `views[${index}].name must match ${NAME_PATTERN} (got ${JSON.stringify(raw.name)})`,
    );
  }
  return {
    name: raw.name,
    camera: requirePoint(raw.camera, `views[${index}].camera`),
    target: requirePoint(raw.target, `views[${index}].target`),
    fovDeg:
      raw.fov_deg == null
        ? null
        : requireNumber(raw.fov_deg, `views[${index}].fov_deg`),
  };
}

function parseViews(raw) {
  if (!Array.isArray(raw) || raw.length < 1 || raw.length > 50) {
    throw new RecipeError("views must be an array of 1 to 50 entries");
  }
  const views = raw.map(parseView);
  const seen = new Set();
  for (const v of views) {
    if (seen.has(v.name)) {
      throw new RecipeError(`duplicate view name: ${v.name}`);
    }
    seen.add(v.name);
  }
  return views;
}

function parseLayers(raw) {
  if (raw == null || typeof raw !== "object") {
    throw new RecipeError(
      "layers must be an object with explicit lidar/boxes/trails booleans",
    );
  }
  for (const key of ["lidar", "boxes", "trails"]) {
    if (typeof raw[key] !== "boolean") {
      throw new RecipeError(`layers.${key} must be an explicit boolean`);
    }
  }
  const trailHistorySec =
    raw.trail_history_sec == null
      ? 5
      : requireNumber(raw.trail_history_sec, "layers.trail_history_sec");
  if (trailHistorySec < 0) {
    throw new RecipeError("layers.trail_history_sec must not be negative");
  }
  return {
    lidar: raw.lidar,
    boxes: raw.boxes,
    trails: raw.trails,
    trailHistorySec,
  };
}

function parseViewport(raw) {
  const width = raw?.width ?? 1280;
  const height = raw?.height ?? 720;
  if (!Number.isInteger(width) || width <= 0) {
    throw new RecipeError("viewport.width must be a positive integer");
  }
  if (!Number.isInteger(height) || height <= 0) {
    throw new RecipeError("viewport.height must be a positive integer");
  }
  return { width, height };
}

function parseOutput(raw) {
  if (raw == null || typeof raw.dir !== "string" || !raw.dir) {
    throw new RecipeError("output.dir must be a non-empty string path");
  }
  return { dir: raw.dir, contactSheet: Boolean(raw.contact_sheet) };
}

const DEFAULT_ELEVATION_KEYFRAMES = [
  { t: 0, deg: 20 },
  { t: 1 / 3, deg: 30 },
  { t: 2 / 3, deg: 20 },
  { t: 1, deg: 10 },
];

function parseElevationKeyframes(raw) {
  if (!Array.isArray(raw) || raw.length < 2) {
    throw new RecipeError(
      "bullet_time.elevation_keyframes must have at least 2 entries",
    );
  }
  const keyframes = raw.map((kf, i) => ({
    t: requireNumber(kf?.t, `bullet_time.elevation_keyframes[${i}].t`),
    deg: requireNumber(kf?.deg, `bullet_time.elevation_keyframes[${i}].deg`),
  }));
  if (keyframes[0].t !== 0) {
    throw new RecipeError("bullet_time.elevation_keyframes must start at t=0");
  }
  if (keyframes[keyframes.length - 1].t !== 1) {
    throw new RecipeError("bullet_time.elevation_keyframes must end at t=1");
  }
  for (let i = 1; i < keyframes.length; i++) {
    if (keyframes[i].t <= keyframes[i - 1].t) {
      throw new RecipeError(
        "bullet_time.elevation_keyframes must have strictly ascending t",
      );
    }
  }
  return keyframes;
}

function parseBulletTime(raw, views) {
  if (raw == null || typeof raw !== "object") {
    throw new RecipeError("bullet_time must be an object");
  }

  const target =
    raw.target != null &&
    typeof raw.target === "object" &&
    "object_id" in raw.target
      ? {
          objectId: requireNonEmptyString(
            raw.target.object_id,
            "bullet_time.target.object_id",
          ),
        }
      : { point: requirePoint(raw.target, "bullet_time.target") };

  if (
    typeof raw.base_view !== "string" ||
    !views.some((v) => v.name === raw.base_view)
  ) {
    throw new RecipeError(
      `bullet_time.base_view must name one of the recipe's views (got ${JSON.stringify(raw.base_view)})`,
    );
  }

  const imageCount = raw.image_count ?? 25;
  if (!Number.isInteger(imageCount) || imageCount < 2 || imageCount > 50) {
    throw new RecipeError(
      "bullet_time.image_count must be an integer between 2 and 50",
    );
  }

  const radiusM =
    raw.radius_m == null
      ? null
      : requireNumber(raw.radius_m, "bullet_time.radius_m");
  if (radiusM != null && radiusM <= 0) {
    throw new RecipeError("bullet_time.radius_m must be positive");
  }

  return {
    target,
    baseView: raw.base_view,
    radiusM,
    imageCount,
    arcStartDeg: requireNumber(
      raw.arc_start_deg ?? -90,
      "bullet_time.arc_start_deg",
    ),
    arcEndDeg: requireNumber(raw.arc_end_deg ?? 90, "bullet_time.arc_end_deg"),
    elevationKeyframes:
      raw.elevation_keyframes == null
        ? DEFAULT_ELEVATION_KEYFRAMES
        : parseElevationKeyframes(raw.elevation_keyframes),
  };
}

function requireNonEmptyString(value, label) {
  if (typeof value !== "string" || !value) {
    throw new RecipeError(`${label} must be a non-empty string`);
  }
  return value;
}

/**
 * Parses and validates a recipe object. `recipeDir` is the directory the
 * recipe file itself lives in — `source` resolves relative to it, matching
 * how a recipe author would naturally write a path.
 */
export function parseRecipe(raw, recipeDir) {
  if (raw == null || typeof raw !== "object") {
    throw new RecipeError("recipe must be a JSON object");
  }
  if (raw.version !== 1) {
    throw new RecipeError(
      `recipe.version must be 1, got ${JSON.stringify(raw.version)}`,
    );
  }
  if (typeof raw.source !== "string" || !raw.source) {
    throw new RecipeError("recipe.source must be a non-empty string path");
  }

  const views = parseViews(raw.views);

  return {
    version: 1,
    source: path.resolve(recipeDir, raw.source),
    frame: parseFrameSelector(raw.frame),
    views,
    layers: parseLayers(raw.layers),
    viewport: parseViewport(raw.viewport),
    output: parseOutput(raw.output),
    bulletTime:
      raw.bullet_time == null ? null : parseBulletTime(raw.bullet_time, views),
  };
}

/** Reads and parses a recipe file from disk. */
export async function loadRecipe(recipePath) {
  const text = await readFile(recipePath, "utf8");
  let raw;
  try {
    raw = JSON.parse(text);
  } catch (err) {
    throw new RecipeError(`${recipePath} is not valid JSON: ${err.message}`);
  }
  return parseRecipe(raw, path.dirname(path.resolve(recipePath)));
}
