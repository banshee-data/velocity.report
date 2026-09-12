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

/** Select the boundary used by the playback clock. */
export function sceneLoopDuration(fullDuration, openingDuration, openingOnly) {
  const full = Math.max(0, fullDuration || 0);
  if (!openingOnly) return full;
  const opening = Math.max(0, openingDuration || 0);
  return opening > 0 ? Math.min(opening, full || opening) : full;
}

/** Opacity for a point-cloud overlay that occupies the start of a longer scene. */
export function pointCloudOverlayOpacity(
  seconds,
  duration,
  { fadeOutSeconds = 5, fadeInSeconds = 2, loopingIn = false } = {},
) {
  if (!(duration > 0)) return 0;
  const t = Math.max(0, seconds);
  if (t >= duration) return 0;

  const fadeOut = Math.min(Math.max(0, fadeOutSeconds), duration);
  const outOpacity = fadeOut
    ? Math.min(1, (duration - t) / fadeOut)
    : 1;
  const inOpacity = loopingIn && fadeInSeconds > 0
    ? Math.min(1, t / fadeInSeconds)
    : 1;
  return Math.max(0, Math.min(outOpacity, inOpacity));
}

/** Fail closed unless two exports begin in the same coordinate and recording. */
export function sceneSourcesAlign(primary, overlay) {
  if (!primary || !overlay) return false;
  return Boolean(
    primary.start_ns &&
      primary.start_ns === overlay.start_ns &&
      primary.source_vrlog_sha256 &&
      primary.source_vrlog_sha256 === overlay.source_vrlog_sha256 &&
      primary.sensor_id === overlay.sensor_id &&
      primary.coordinate_frame?.frame_id === overlay.coordinate_frame?.frame_id &&
      primary.coordinate_frame?.reference_frame ===
        overlay.coordinate_frame?.reference_frame,
  );
}

/** Unique recorder versions carried by the VRLOG-derived scene parts. */
export function sceneGeneratorVersions(parts) {
  return [
    ...new Set(
      (parts ?? [])
        .map((part) => part?.header?.build_version?.trim())
        .filter(Boolean),
    ),
  ];
}
