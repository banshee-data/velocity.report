export function readProgress(key, storage) {
  try {
    return JSON.parse((storage ?? globalThis.localStorage).getItem(key));
  } catch {
    return null;
  }
}

export function writeProgress(key, progress, storage) {
  try {
    (storage ?? globalThis.localStorage).setItem(key, JSON.stringify(progress));
  } catch {
    // The game keeps the running session in memory if storage is unavailable.
  }
}

export function restoreTrial(trial, saved) {
  if (!saved || !Array.isArray(saved.collected)) return null;
  const valid = new Set(trial.targets.map((target) => target.t));
  trial.collected = new Set(
    saved.collected.filter((token) => valid.has(token)),
  );
  trial.elapsed = Number.isFinite(saved.elapsed)
    ? Math.max(0, saved.elapsed)
    : 0;
  trial.crashes = Number.isSafeInteger(saved.crashes)
    ? Math.max(0, saved.crashes)
    : 0;
  if (valid.size && trial.collected.size === valid.size)
    trial.status = "finished";
  else if (trial.elapsed > 0 || trial.collected.size) trial.status = "running";
  const { checkpoint } = saved;
  return checkpoint &&
    [checkpoint.x, checkpoint.y, checkpoint.heading].every(Number.isFinite)
    ? { x: checkpoint.x, y: checkpoint.y, heading: checkpoint.heading }
    : null;
}

export function bestAirtimes(entries, landing) {
  const records = Array.isArray(entries) ? [...entries] : [];
  if (landing) records.push(landing);
  return records
    .filter((record) => Number.isFinite(record?.seconds) && record.seconds > 0)
    .map(({ seconds, peak, ending }) => ({
      seconds,
      peak: Number.isFinite(peak) ? peak : 0,
      ...(ending === "impact" ? { ending } : {}),
    }))
    .sort((a, b) => b.seconds - a.seconds || b.peak - a.peak)
    .slice(0, 3);
}
