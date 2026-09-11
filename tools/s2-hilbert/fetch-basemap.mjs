#!/usr/bin/env node
/**
 * Cache the basemap tiles the scene map needs, once, into the site's own assets.
 *
 * This follows what the report map already does: the operator fetches tiles
 * deliberately, the result is stored, and every reader afterwards is served
 * from our own origin. The report stitches its tiles into one stored image
 * because a report is a fixed frame; a map a visitor can pan needs the tiles
 * themselves, so they are cached as tiles.
 *
 * The point is the same either way — a reader of velocity.report should not
 * have their address handed to a tile server to look at a map of where a
 * sensor stood. Downloading is a build step, not something a visitor triggers.
 *
 * Tiles are cached on disk and skipped when present, so a rebuild costs
 * nothing and the tile server is asked once per tile, ever.
 */
import { mkdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(TOOL_DIRECTORY, "..", "..");
// The generated scene data, not scene-sites.json: the latter holds only the
// published scenes, and the map shows every recorded site. Caching from the
// smaller set leaves a visitor panning onto bare grid the moment a site is
// published outside the frame the published four happened to span.
const SOURCE = path.join(
  REPO_ROOT,
  "public_html",
  "src",
  "_data",
  "scenes.json",
);
const TILE_ROOT = path.join(REPO_ROOT, "public_html", "src", "images", "tiles");

/** Zoom levels cached. 12 frames the city; 16 is a junction. */
export const MIN_ZOOM = 12;
export const MAX_ZOOM = 16;

/** Margin around the sites, in degrees, before the tile padding below. */
const MARGIN = 0.01;

/**
 * Extra tiles cached beyond the sites' own box, per side, per zoom.
 *
 * The map frames the sites, but the frame is much wider than it is tall, so
 * the view spills well past the box horizontally — the sites fill the height
 * and the width shows whatever is beside them. Padding in degrees cannot
 * express that; padding in tiles can, because a tile is the unit the view is
 * actually assembled from.
 */
const PAD_TILES = 4;

// The OSM tile usage policy requires a real identifying User-Agent and no
// heavy bulk downloading. This is a few hundred tiles, once, single-threaded.
const USER_AGENT =
  "velocity.report basemap cache (https://velocity.report; one-off build step)";
const TILE_URL = (z, x, y) =>
  `https://tile.openstreetmap.org/${z}/${x}/${y}.png`;

export function lonToTile(lon, z) {
  return Math.floor(((lon + 180) / 360) * 2 ** z);
}

export function latToTile(lat, z) {
  const r = (lat * Math.PI) / 180;
  return Math.floor(
    ((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * 2 ** z,
  );
}

/** The tile ranges, per zoom, that cover every located site plus a margin. */
export function tilePlan(positions, minZoom = MIN_ZOOM, maxZoom = MAX_ZOOM) {
  const lats = positions.map((p) => p.lat);
  const lons = positions.map((p) => p.lon);
  const south = Math.min(...lats) - MARGIN;
  const north = Math.max(...lats) + MARGIN;
  const west = Math.min(...lons) - MARGIN;
  const east = Math.max(...lons) + MARGIN;

  const plan = [];
  for (let z = minZoom; z <= maxZoom; z++) {
    const span = 2 ** z;
    const clamp = (v) => Math.max(0, Math.min(span - 1, v));
    const xMin = clamp(lonToTile(west, z) - PAD_TILES);
    const xMax = clamp(lonToTile(east, z) + PAD_TILES);
    // Latitude runs the other way: north is the smaller tile index.
    const yMin = clamp(latToTile(north, z) - PAD_TILES);
    const yMax = clamp(latToTile(south, z) + PAD_TILES);
    for (let x = xMin; x <= xMax; x++) {
      for (let y = yMin; y <= yMax; y++) plan.push({ z, x, y });
    }
  }
  return plan;
}

async function exists(file) {
  try {
    const info = await stat(file);
    return info.size > 0;
  } catch {
    return false;
  }
}

export async function fetchBasemap({ log = () => {} } = {}) {
  const source = JSON.parse(await readFile(SOURCE, "utf8"));
  const positions = (source.sites ?? [])
    .filter((s) => s.position)
    .map((s) => s.position);
  if (positions.length === 0)
    throw new Error("no located sites, so no basemap to cache");

  const plan = tilePlan(positions);
  log(
    `${plan.length} tiles cover ${positions.length} sites at z${MIN_ZOOM}-${MAX_ZOOM}`,
  );

  let fetched = 0;
  let cached = 0;
  for (const { z, x, y } of plan) {
    const file = path.join(TILE_ROOT, String(z), String(x), `${y}.png`);
    if (await exists(file)) {
      cached++;
      continue;
    }
    const response = await fetch(TILE_URL(z, x, y), {
      headers: { "User-Agent": USER_AGENT },
    });
    if (!response.ok) {
      log(`  ${z}/${x}/${y}: HTTP ${response.status}, skipped`);
      continue;
    }
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, Buffer.from(await response.arrayBuffer()));
    fetched++;
    // Courtesy pacing. The policy asks for no heavy bulk load, and this runs
    // once; there is nothing to gain by going faster.
    await new Promise((resolve) => setTimeout(resolve, 120));
    if (fetched % 50 === 0)
      log(`  ${fetched} fetched, ${cached} already cached`);
  }
  log(
    `done: ${fetched} fetched, ${cached} already cached, ${plan.length} total`,
  );
  return { fetched, cached, total: plan.length };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  fetchBasemap({ log: (m) => process.stdout.write(`${m}\n`) }).catch(
    (error) => {
      process.stderr.write(`basemap: ${error.message}\n`);
      process.exitCode = 1;
    },
  );
}
