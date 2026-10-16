/**
 * Survey data model shared by the Eleventy build (_data/surveys.js, filters in
 * .eleventy.js) and the browser (survey-page.js). Pure functions only: no DOM,
 * no file access, so the same numbers reach the static page and the compare
 * view.
 *
 * A site has one or more recordings. A recording carries a summary per metric
 * family it measured. Today only vehicle speed exists; a new family is added
 * by appending to FAMILIES, giving recordings a summary under its id, and
 * adding a card include for it. Nothing here assumes speed is the only one.
 */

/** Metric families, in page order. `sensors` lists the sensor kinds that can measure it. */
export const FAMILIES = [
  {
    id: "speed",
    name: "Vehicle speed",
    short: "Speed",
    sensors: ["radar", "lidar"],
    definition: "vehicles > 5 mph · peak speed per track",
  },
];

export const SENSORS = {
  lidar: { label: "LiDAR", tag: "LIDAR", description: "Fixed roadside LiDAR" },
  radar: { label: "Radar", tag: "RADAR", description: "Doppler radar" },
  unknown: { label: "Sensor", tag: "SENSOR", description: "Unrecorded sensor" },
};

/** Survey sites are in San Francisco; times are shown in local time. */
export const SITE_TIME_ZONE = "America/Los_Angeles";

export function sensorKind(sensorId) {
  const id = String(sensorId || "").toLowerCase();
  if (/hesai|pandar|ouster|velodyne|lidar/.test(id)) return "lidar";
  if (/ops2|omnipresense|radar/.test(id)) return "radar";
  return "unknown";
}

/** Families a sensor kind can produce, in page order. */
export function familiesForSensor(kind) {
  return FAMILIES.filter((family) => family.sensors.includes(kind));
}

/** Stable, URL-safe recording id from its start time: 2026-09-02T2021Z. */
export function recordingId(startMs) {
  const iso = new Date(startMs).toISOString();
  return `${iso.slice(0, 10)}T${iso.slice(11, 13)}${iso.slice(14, 16)}Z`;
}

function zonedParts(startMs, options) {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: SITE_TIME_ZONE,
    ...options,
  }).formatToParts(new Date(startMs));
  return Object.fromEntries(parts.map((p) => [p.type, p.value]));
}

/** Display strings for a recording's start, in site-local time. */
export function recordingWhen(startMs, durationSec) {
  const p = zonedParts(startMs, {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
  const minutes = Math.max(1, Math.round(durationSec / 60));
  const hour = Number(p.hour);
  const weekend = p.weekday === "Sat" || p.weekday === "Sun";
  const period =
    hour < 6
      ? "night"
      : hour < 10
        ? "AM peak"
        : hour < 15
          ? "midday"
          : hour < 19
            ? "PM peak"
            : "evening";
  return {
    label: `${p.month} ${p.year}`,
    labelDay: `${p.day} ${p.month} ${p.year}`,
    date: `${p.weekday} ${p.day} ${p.month} ${p.year}`,
    time: `${p.weekday} ${p.hour}:${p.minute}`,
    clock: `${p.hour}:${p.minute} PT`,
    context: `${weekend ? "Weekend" : "Weekday"} · ${period}`,
    year: Number(p.year),
    minutes,
  };
}

const round1 = (v) => (Number.isFinite(v) ? Math.round(v * 10) / 10 : null);

/**
 * Normalise a pipeline vehicle_speed summary (timeline.json) into the shape
 * the pages use. Shares are fractions of counted tracks, 0..1.
 */
export function summariseSpeed(raw) {
  if (!raw || !Array.isArray(raw.histogram)) return null;
  const size = raw.bucket_size ?? 5;
  const total = raw.histogram.reduce((sum, b) => sum + (b.count ?? 0), 0);
  if (total === 0) return null;
  return {
    units: raw.units ?? "mph",
    minimum: raw.minimum_speed ?? 5,
    count: raw.track_count ?? total,
    bucketSize: size,
    p50: round1(raw.p50),
    p85: round1(raw.p85),
    p98: round1(raw.p98),
    max: round1(raw.max),
    bins: raw.histogram.map((b) => ({
      from: b.start_mph,
      to: b.start_mph + size,
      count: b.count ?? 0,
      share: (b.count ?? 0) / total,
    })),
  };
}

/** A speed limit the pages can use, or null when unset or invalid. */
export function validLimit(limit) {
  const n = Number(limit);
  return limit !== null && limit !== "" && Number.isFinite(n) && n > 0
    ? n
    : null;
}

/**
 * Share of tracks at or above the limit, from the histogram. Only exact when
 * the limit falls on a bucket edge; otherwise null rather than an estimate.
 */
export function shareAtOrAbove(speed, limit) {
  const lim = validLimit(limit);
  if (!speed || lim === null) return null;
  if (lim >= speed.bins.at(-1).to) return 0;
  if (lim <= speed.bins[0].from) return 1;
  if ((lim - speed.bins[0].from) % speed.bucketSize !== 0) return null;
  return speed.bins
    .filter((b) => b.from >= lim)
    .reduce((s, b) => s + b.share, 0);
}

/** Per-bucket shares at or above the limit, for the "excess over limit" bands. */
export function excessBands(speed, limit) {
  const lim = validLimit(limit);
  if (!speed || lim === null || shareAtOrAbove(speed, lim) === null) return [];
  return speed.bins
    .filter((b) => b.from >= lim && b.count > 0)
    .map((b) => ({ from: b.from, to: b.to, share: b.share, count: b.count }));
}

export function pct(share, digits = 1) {
  if (share === null || share === undefined) return "—";
  return `${(share * 100).toFixed(digits)}%`;
}

export const mph = (v) => (Number.isFinite(v) ? v.toFixed(1) : "—");

/**
 * The one-sentence key observation, as segments so both Nunjucks and the
 * browser render it identically. tone: "plain" | "strong" | "over".
 */
export function speedFinding(speed, limit) {
  if (!speed) return [];
  const lim = validLimit(limit);
  const over = shareAtOrAbove(speed, lim);
  if (lim !== null && over !== null) {
    const under = over < 0.5;
    return [
      {
        text: under
          ? "Most drivers stayed under the "
          : "Most drivers reached the ",
      },
      { text: `${lim} mph`, tone: "strong" },
      { text: " limit. " },
      {
        text: `${pct(over)} reached ${lim} mph or more`,
        tone: over > 0 ? "over" : "plain",
      },
      { text: "; the fastest reached " },
      {
        text: `${mph(speed.max)} mph`,
        tone: speed.max >= lim ? "over" : "plain",
      },
      { text: "." },
    ];
  }
  return [
    { text: "Half of vehicles were at or below " },
    { text: `${mph(speed.p50)} mph`, tone: "strong" },
    { text: " and 85 in 100 at or below " },
    { text: `${mph(speed.p85)} mph`, tone: "strong" },
    { text: ". The fastest reached " },
    { text: `${mph(speed.max)} mph`, tone: "strong" },
    { text: "." },
  ];
}

export const PERCENTILES = [
  { key: "p50", label: "P50", name: "Median" },
  { key: "p85", label: "P85", name: "85th percentile" },
  { key: "p98", label: "P98", name: "98th percentile" },
  { key: "max", label: "MAX", name: "Fastest" },
];

/** Signed one-decimal difference, or null when either side is missing. */
export function delta(current, base) {
  if (!Number.isFinite(current) || !Number.isFinite(base)) return null;
  return Math.round((current - base) * 10) / 10;
}

/**
 * Pick the recordings to compare and the baseline from URL-style input.
 * Unknown ids are dropped; fewer than two leaves compare off. Changes are
 * measured from the baseline to the newest selected recording, so the newest
 * cannot be the baseline; the baseline defaults to the earliest. Recordings
 * are oldest first.
 */
export function compareSelection(recordings, ids, baseId, maxCount = 4) {
  const byId = new Map(recordings.map((r) => [r.id, r]));
  const wanted = (ids && ids.length ? ids : recordings.map((r) => r.id))
    .filter((id, i, all) => byId.has(id) && all.indexOf(id) === i)
    .map((id) => byId.get(id))
    .sort((a, b) => a.startMs - b.startMs)
    .slice(-maxCount);
  if (wanted.length < 2) return null;
  const latest = wanted.at(-1);
  const base = wanted.find((r) => r.id === baseId && r !== latest) ?? wanted[0];
  return { recordings: wanted, base, latest };
}

/** Rows for the compare table: values per recording and latest minus baseline. */
export function compareRows(selection, limit) {
  const { recordings, base, latest } = selection;
  const rows = PERCENTILES.map((p) => ({
    ...p,
    values: recordings.map((r) => r.speed?.[p.key] ?? null),
    delta: delta(latest.speed?.[p.key], base.speed?.[p.key]),
    unit: "mph",
  }));
  const lim = validLimit(limit);
  if (lim !== null) {
    const shares = recordings.map((r) => shareAtOrAbove(r.speed, lim));
    const b = shareAtOrAbove(base.speed, lim);
    const l = shareAtOrAbove(latest.speed, lim);
    rows.push({
      key: "over",
      label: `≥${lim}`,
      name: "Share at or above limit",
      values: shares,
      delta: b === null || l === null ? null : Math.round((l - b) * 1000) / 10,
      unit: "pts",
      share: true,
    });
  }
  return rows;
}

/** Change sentence for compare mode, as finding segments. */
export function compareFinding(selection, limit) {
  const { base, latest } = selection;
  if (base === latest || !base.speed || !latest.speed) return [];
  const d = delta(latest.speed.p85, base.speed.p85);
  const verb = d < 0 ? "fell" : d > 0 ? "rose" : "held at";
  const segs = [
    { text: `P85 ${verb} ` },
    ...(d === 0
      ? []
      : [
          { text: `${Math.abs(d).toFixed(1)} mph`, tone: "strong" },
          { text: " to " },
        ]),
    { text: `${mph(latest.speed.p85)} mph` },
    { text: ` (${base.label} → ${latest.label}).` },
  ];
  const lim = validLimit(limit);
  const b = shareAtOrAbove(base.speed, lim);
  const l = shareAtOrAbove(latest.speed, lim);
  if (b !== null && l !== null) {
    segs.push(
      { text: ` The share at or above the ${lim} mph limit went from ` },
      { text: pct(b), tone: "over" },
      { text: " to " },
      { text: pct(l), tone: "over" },
      { text: "." },
    );
  }
  return segs;
}

/**
 * Caveats that stop a comparison being like-for-like. Each note is
 * { tone: "warn" | "info", text }.
 */
export function comparabilityNotes(selection) {
  const notes = [];
  const recs = selection.recordings;
  const contexts = new Set(recs.map((r) => r.when.context.split(" · ")[0]));
  if (contexts.size > 1) {
    const odd = recs.filter((r) => r.when.context.startsWith("Weekend"));
    notes.push({
      tone: "warn",
      text: `${odd.map((r) => r.label).join(", ")} ${odd.length === 1 ? "was a weekend recording" : "were weekend recordings"}. Differences may reflect time of week as well as change on the street.`,
    });
  }
  const builds = new Set(recs.map((r) => r.build));
  const unknown = recs.filter((r) => !r.build).map((r) => r.label);
  notes.push(
    unknown.length
      ? {
          tone: "warn",
          text: `${unknown.join(", ")} ${unknown.length === 1 ? "does" : "do"} not record the pipeline build that processed ${unknown.length === 1 ? "it" : "them"}, so method cannot be ruled out as a factor.`,
        }
      : builds.size === 1
        ? {
            tone: "info",
            text: `${recs.length === 2 ? "Both" : `All ${recs.length}`} were processed with the same pipeline build (${[...builds][0]}), so method is not a factor.`,
          }
        : {
            tone: "warn",
            text: "These recordings were processed with different pipeline builds. Some of the difference may come from the method rather than the street.",
          },
  );
  const sensors = new Set(recs.map((r) => r.sensor));
  if (sensors.size > 1) {
    const radar = recs.filter((r) => r.sensor === "radar").map((r) => r.label);
    notes.push({
      tone: "info",
      text: `${radar.join(", ")} ${radar.length === 1 ? "was a radar survey" : "were radar surveys"}. Vehicle speed compares across radar and LiDAR; other measurements are only available from LiDAR recordings.`,
    });
  }
  return notes;
}

/**
 * Availability of each family across the selected recordings, for families
 * other than the one being compared. A family a sensor cannot measure reads
 * "not recorded"; one it can measure but did not report reads "missing".
 */
export function familyAvailability(selection, excludeId = "speed") {
  return FAMILIES.filter((f) => f.id !== excludeId).map((family) => ({
    family,
    cells: selection.recordings.map((r) =>
      r.families.includes(family.id)
        ? "measured"
        : family.sensors.includes(r.sensor)
          ? "missing"
          : "not-recorded",
    ),
  }));
}

/**
 * Links for a file in the published dataset, from the dataset's page URL and
 * a dataset-relative path: the file itself, and its page on the Hub.
 */
export function datasetLinks(datasetUrl, relativePath) {
  if (!datasetUrl || !relativePath) return null;
  const base = datasetUrl.replace(/\/+$/, "");
  return {
    path: relativePath,
    download: `${base}/resolve/main/${relativePath}?download=true`,
    page: `${base}/blob/main/${relativePath}`,
  };
}
