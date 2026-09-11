/** Scene playback starts without waiting for a button press. */
export function autoplayScene() {
  return true;
}

/** Advance a scene clock and wrap it cleanly at the end. */
export function advanceSceneClock(seconds, wallSeconds, rate, duration) {
  if (!(duration > 0)) return { seconds: 0, wrapped: false };
  const elapsed = Math.max(0, wallSeconds) * Math.max(0, rate);
  const next = Math.max(0, seconds) + elapsed;
  return {
    seconds: next % duration,
    wrapped: next >= duration,
  };
}
