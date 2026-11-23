// The bundled tiles carry height and crown radius. Use the same inferred
// trunk radius for both the displayed tree and its collision cylinder.
export const treeRadius = (height) => Math.max(0.12, 0.04 * height);

export function treeDamageForImpact(tree, mass, normalSpeed, nx, ny, previous) {
  if (!tree || tree.broken || !(normalSpeed > 0) || !(mass > 0)) return null;
  const diameter = 2 * tree.radius;
  // Game calibration. Bending capacity for a solid circular section scales
  // with diameter cubed. Energy-to-failure is a tuning proxy for that capacity.
  const breakWork = 12000 * (diameter / 0.3) ** 3;
  const energy = 0.5 * mass * normalSpeed ** 2;
  const work = Math.min(breakWork, (previous?.work ?? 0) + 0.28 * energy);
  if (work < 0.42 * breakWork) return null;
  const stage = work >= breakWork ? "broken" : "cracked";
  return {
    tile: tree.tile,
    index: tree.index,
    stage,
    diameter,
    anchor: [tree.x, tree.y],
    work,
    ...(previous && { priorLean: [previous.east, previous.north] }),
    east: -nx,
    north: -ny,
    absorbed: Math.min(1, (work - (previous?.work ?? 0)) / energy),
  };
}

export function treeFallProgress(seconds) {
  return Math.max(0, Math.min(1, seconds / 0.9)) ** 2;
}

export function treeFallAngle(event, seconds) {
  const angle = event.stage === "broken" ? Math.PI * 0.48 : Math.PI / 19;
  return angle * treeFallProgress(seconds);
}

// Saved clips can outlive a tile bake that reorders the tree rows.
export function replayTreeIndex(tile, event) {
  if (!Array.isArray(event.anchor) || !event.anchor.every(Number.isFinite))
    return -1;
  const matches = ([x, y, height]) =>
    tile.o[0] + x / 10 === event.anchor[0] &&
    tile.o[1] + y / 10 === event.anchor[1] &&
    2 * treeRadius(height) === event.diameter;
  const original = Number.isInteger(event.index) && tile.tr[event.index];
  return original && matches(original)
    ? event.index
    : tile.tr.findIndex(matches);
}
