const OFFSET_STEP = 0.5;
const CLEARANCE_STEPS = 4;

// Choose a low-curvature line through sampled clear poses. A half-metre
// lateral move between two-metre stations bounds the steering demand.
export function roadClearanceOffsets(stations, maxOffset, blocked) {
  const half = Math.floor(maxOffset / OFFSET_STEP);
  const candidates = Array.from(
    { length: 2 * half + 3 },
    (_, i) => i - half - 1,
  );
  let previous = [{ offset: 0, change: 0, cost: 0, parent: null }];
  for (const station of stations) {
    const blockedOffsets = candidates.filter((offset) =>
      blocked(station, offset * OFFSET_STEP),
    );
    const next = new Map();
    for (const parent of previous) {
      for (const change of [-1, 0, 1]) {
        const offset = parent.offset + change;
        if (Math.abs(offset) > half || blockedOffsets.includes(offset))
          continue;
        const distance = Math.min(
          CLEARANCE_STEPS,
          ...blockedOffsets.map((obstacle) => Math.abs(obstacle - offset)),
        );
        const cost =
          parent.cost +
          (offset * OFFSET_STEP) ** 2 * 0.025 +
          ((CLEARANCE_STEPS - distance) / 3) ** 2 * 1.5 +
          change ** 2 * 0.23 +
          (change - parent.change) ** 2 * 1.7;
        const key = `${offset},${change}`;
        if (cost < (next.get(key)?.cost ?? Infinity))
          next.set(key, { offset, change, cost, parent });
      }
    }
    if (!next.size) throw new Error(`No clear lane at ${station.s} m`);
    previous = [...next.values()];
  }
  const finishCost = ({ cost, offset }) => cost + 5 * offset ** 2;
  const offsets = [];
  let pose = previous.reduce((best, next) =>
    finishCost(next) < finishCost(best) ? next : best,
  );
  while (pose.parent) {
    offsets.push(pose.offset * OFFSET_STEP);
    pose = pose.parent;
  }
  return offsets.reverse();
}
