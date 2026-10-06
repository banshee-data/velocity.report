// Polar angles are measured from vertical. Isometric is acos(1/sqrt(3)),
// or 54.7356 degrees from vertical (35.2644 degrees above the ground).
export const ISOMETRIC_POLAR_DEG =
  (Math.acos(1 / Math.sqrt(3)) * 180) / Math.PI;
const wrap = (angle) => ((angle % 360) + 360) % 360;

export function compassPose(view, northAzimuthDeg) {
  const tilt = Math.max(0, Math.min(ISOMETRIC_POLAR_DEG, view.polar_deg));
  return {
    rotation: wrap(view.azimuth_deg + (northAzimuthDeg ?? 0) + 180),
    scaleY: Math.cos((tilt * Math.PI) / 180),
  };
}

export function northOverhead(view, northAzimuthDeg) {
  // The camera sits opposite north so north projects towards the screen top.
  // Preserve the area and zoom the reader was inspecting.
  return {
    ...view,
    azimuth_deg: wrap(180 - (northAzimuthDeg ?? 0)),
    polar_deg: 0,
  };
}

export function updateCompass(button, view, northAzimuthDeg) {
  if (!button) return;
  const { rotation, scaleY } = compassPose(view, northAzimuthDeg);
  button
    .querySelector("[data-compass-tilt]")
    .setAttribute("transform", `translate(40 40) scale(1 ${scaleY})`);
  button
    .querySelector("[data-compass-bearing]")
    .setAttribute("transform", `rotate(${rotation})`);
  const known = Number.isFinite(northAzimuthDeg);
  button.querySelector("[data-compass-label]").textContent = known ? "N" : "N?";
  const label = known
    ? "True north — overhead view"
    : "Overhead view (north unmeasured; using sensor zero)";
  button.setAttribute("aria-label", label);
  button.title = label;
}
