// Derive gameplay metadata without modifying the measured city tiles.
// Run from the repository root: node tools/sf-drive/build-quest-route.mjs
import fs from "node:fs";
import { gunzipSync } from "node:zlib";
import { createHash } from "node:crypto";
import { format } from "prettier";
import { buildBakedRoadContact } from "../../public_html/src/js/sf-baked-road-contact.js";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
  intersectionsForTile,
  tileAt,
} from "../../public_html/src/js/sf-drive-core.js";

const dir = new URL("../../public_html/src/sf/", import.meta.url);
const raw = fs.readFileSync(new URL("tiles/manifest.json", dir));
const manifest = JSON.parse(raw);
const assignments = JSON.parse(
  fs.readFileSync(new URL("quest-assignments.json", import.meta.url), "utf8"),
);
const tiles = new Map(
  manifest.tiles.map(({ t }) => [
    t,
    JSON.parse(gunzipSync(fs.readFileSync(new URL(`tiles/${t}.json.gz`, dir)))),
  ]),
);
const indexes = new Map();
const targets = [];
const skipped = [];

for (const row of manifest.tiles) {
  const tile = tiles.get(row.t);
  const assignment = assignments[row.t];
  const surfaceId = assignment?.surfaceId;
  const signTile = assignment?.signTile ? tiles.get(assignment.signTile) : tile;
  if (!signTile) throw new Error(`Unknown sign source for ${row.t}`);
  const neighbours = [...tiles.values()].filter(
    (t) => Math.hypot(t.o[0] - row.x, t.o[1] - row.y) < 2200,
  );
  const collisionIndexes = neighbours.map((t) => {
    if (!indexes.has(t.t)) indexes.set(t.t, buildCollisionIndex(t));
    return indexes.get(t.t);
  });
  const candidates = intersectionsForTile(signTile, neighbours).sort(
    (a, b) =>
      Math.hypot(a.x - row.x, a.y - row.y) -
      Math.hypot(b.x - row.x, b.y - row.y),
  );
  const road =
    surfaceId && tile.playableRoads?.find(({ id }) => id === surfaceId);
  const surface = road && buildBakedRoadContact(road)?.surface;
  if (surfaceId && !surface)
    throw new Error(`Missing pickup contact ${surfaceId}`);
  const heightAt = surface
    ? (x, y) => surface.heightAt(x, y)
    : (x, y) => groundAtWorld(neighbours, x, y, true);
  const target = candidates.find((p) => {
    if (
      assignment?.intersection &&
      (p.names.length !== assignment.intersection.length ||
        !assignment.intersection.every((name) => p.names.includes(name)) ||
        tileAt(manifest.tiles, p.x, p.y).t !== row.t)
    )
      return false;
    const z = heightAt(p.x, p.y);
    return Number.isFinite(z) && !collisionAt(collisionIndexes, p, z);
  });
  if (target) {
    if (!assignment?.emoji)
      throw new Error(`Missing emoji assignment for ${row.t}`);
    targets.push({
      t: row.t,
      emoji: assignment.emoji,
      ...target,
      ...(surface && { surfaceId, z: heightAt(target.x, target.y) }),
    });
  } else {
    if (assignment)
      throw new Error(`No clear intersection for emoji assignment ${row.t}`);
    skipped.push({
      t: row.t,
      reason: tile.s.length
        ? "No clear, grounded intersection"
        : "No signposted intersection",
    });
  }
}
const glyphs = new Set(
  targets.map(({ emoji }) => emoji.replaceAll("\uFE0F", "")),
);
if (glyphs.size !== targets.length)
  throw new Error("Duplicate emoji assignments");
if (Object.keys(assignments).length !== targets.length)
  throw new Error("Emoji assignments include tiles outside the route");
const route = {
  version: 1,
  manifestSha256: createHash("sha256").update(raw).digest("hex"),
  // Coordinate changes start a fresh set of trial records and saved progress.
  placementSha256: createHash("sha256")
    .update(JSON.stringify(targets.map(({ t, x, y }) => [t, x, y])))
    .digest("hex"),
  targets,
  skipped,
};
fs.writeFileSync(
  new URL("quest-route.json", dir),
  await format(JSON.stringify(route), { parser: "json" }),
);
console.log(
  `${targets.length} intersection targets; ${skipped.length} tiles without a playable intersection`,
);
