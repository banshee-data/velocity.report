// Only exact, eligible estimates belong in a scene's relationship label.
export function followingLabel(track) {
  const value = track?.following;
  if (
    value?.bucket !== "following_eligible" ||
    !value.leader_id ||
    !Number.isFinite(value.gap_m) ||
    value.gap_m <= 0 ||
    !Number.isFinite(value.net_time_gap_s) ||
    value.net_time_gap_s < 0
  )
    return "";
  return `estimated gap ${value.gap_m.toFixed(1)} m · ${value.net_time_gap_s.toFixed(2)} s`;
}

export function sceneTrackLabel(track, mph) {
  const vehicle = ["car", "bus", "truck"].includes(track?.c);
  const speed =
    vehicle && Number.isFinite(mph) && mph >= 1 ? `${Math.round(mph)} mph` : "";
  return [speed, followingLabel(track)].filter(Boolean).join(" · ");
}
