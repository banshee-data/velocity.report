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
  const extra = [];
  if (
    Number.isFinite(value.experimental_drac_mps2) &&
    value.experimental_drac_mps2 >= 0
  )
    extra.push(`DRAC ${value.experimental_drac_mps2.toFixed(2)} m/s²`);
  if (
    Number.isFinite(value.track_minimum_time_gap_s) &&
    value.track_minimum_time_gap_s >= 0
  )
    extra.push(`track min ${value.track_minimum_time_gap_s.toFixed(2)} s`);
  return [
    `estimated gap ${value.gap_m.toFixed(1)} m · ${value.net_time_gap_s.toFixed(2)} s`,
    ...extra,
  ].join(" · ");
}

export const GAP_BANDS = [
  { label: "<0.5 s", upper: 0.5, colour: "#fa6673" },
  { label: "0.5–1 s", upper: 1, colour: "#ffae62" },
  { label: "1–1.5 s", upper: 1.5, colour: "#f4dc7c" },
  { label: "1.5–2 s", upper: 2, colour: "#75d9ce" },
  { label: "2+ s", upper: Infinity, colour: "#83b6ff" },
];

export function gapBand(value) {
  return Number.isFinite(value) && value >= 0
    ? GAP_BANDS.find((b) => value < b.upper)
    : null;
}

export function followingRelationships(frame) {
  const tracks = new Map((frame?.tr ?? []).map((t) => [t.id, t]));
  return [...tracks.values()].flatMap((t) => {
    const f = t.following,
      leader = tracks.get(f?.leader_id);
    if (
      !leader ||
      !(followingLabel(t) || dracOnlyLabel(t)) ||
      !Array.isArray(f.projected_gap_xy) ||
      f.projected_gap_xy.length < 2 ||
      f.projected_gap_xy.some(
        (p) => !Array.isArray(p) || p.length !== 2 || !p.every(Number.isFinite),
      )
    )
      return [];
    return [
      {
        follower: t,
        leader,
        path: f.projected_gap_xy,
        colour: gapBand(f.track_minimum_time_gap_s)?.colour ?? "#d5e1e8",
      },
    ];
  });
}

export function renderFollowingStats(host, following) {
  if (!host) return;
  host.replaceChildren();
  host.hidden = !following?.summary;
  if (host.hidden) return;
  const doc = host.ownerDocument,
    s = following.summary;
  const title = doc.createElement("h3");
  title.textContent = "Minimum estimated net time gap · recording population";
  host.append(title);
  for (const [i, bin] of (s.histogram ?? []).entries()) {
    const row = doc.createElement("div");
    row.style.cssText =
      "display:grid;grid-template-columns:6em 1fr 4em;gap:.5em;margin:.4em 0;align-items:center";
    const label = doc.createElement("span");
    label.textContent = GAP_BANDS[i]?.label ?? bin.label;
    const bar = doc.createElement("div");
    bar.style.height = "12px";
    bar.style.width = `${(100 * bin.count) / Math.max(1, ...s.histogram.map((b) => b.count))}%`;
    bar.style.background = GAP_BANDS[i]?.colour ?? "#aaa";
    const n = doc.createElement("span");
    n.textContent = String(bin.count);
    row.append(label, bar, n);
    host.append(row);
  }
  const note = doc.createElement("p");
  note.textContent = `${s.tracks - s.without_time_gap} with estimates · ${s.without_time_gap} unavailable · ${s.tracks} software tracks. One minimum per track over the whole recording, including later evidence. Identity breaks can count a vehicle twice. Provisional, unvalidated estimates.`;
  host.append(note);
  const key = doc.createElement("p");
  key.textContent =
    "Timeline: orange = peak experimental DRAC (m/s²); band = instantaneous net time gap. Boxes = whole-track minimum. Grey = unavailable. Cyan lines = saved inferred lanes; violet = proposals. Gap lines measure the projected following path.";
  host.append(key);
}

export function sceneTrackLabel(track, mph, review = false) {
  const vehicle = ["car", "bus", "truck"].includes(track?.c);
  const speed =
    vehicle && Number.isFinite(mph) && mph >= 1 ? `${Math.round(mph)} mph` : "";
  if (review) {
    const f = track?.following;
    const minimum = gapBand(f?.track_minimum_time_gap_s)
      ? `min ${f.track_minimum_time_gap_s.toFixed(2)} s`
      : "";
    if (followingLabel(track) || dracOnlyLabel(track)) {
      const gap = followingLabel(track)
        ? `${f.net_time_gap_s.toFixed(2)} s`
        : "time gap unavailable";
      const drac =
        Number.isFinite(f.experimental_drac_mps2) &&
        f.experimental_drac_mps2 >= 0
          ? `DRAC ${f.experimental_drac_mps2.toFixed(2)} m/s²`
          : "DRAC unavailable";
      return [
        `${track.id} → ${f.leader_id}`,
        `${f.gap_m.toFixed(1)} m · ${gap}`,
        [drac, minimum].filter(Boolean).join(" · "),
      ].join("\n");
    }
    return [speed ? `${track.id} · ${speed}` : minimum ? track.id : "", minimum]
      .filter(Boolean)
      .join("\n");
  }
  return [speed, followingLabel(track) || dracOnlyLabel(track)]
    .filter(Boolean)
    .join(" · ");
}

export function dracOnlyLabel(track) {
  const v = track?.following;
  if (
    !v?.leader_id ||
    v.drac_status !== "supported" ||
    !Number.isFinite(v.experimental_drac_mps2) ||
    v.experimental_drac_mps2 < 0 ||
    !Number.isFinite(v.gap_m) ||
    v.gap_m <= 0
  )
    return "";
  return `estimated gap ${v.gap_m.toFixed(1)} m · DRAC ${v.experimental_drac_mps2.toFixed(2)} m/s² · net time gap unavailable`;
}
