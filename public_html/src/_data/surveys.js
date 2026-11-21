/**
 * Survey sites and their recordings, read from the published scene assets at
 * build time so the listing and location pages carry real numbers without
 * waiting for the 3D player.
 *
 * A site's recordings are found by convention:
 *   src/scenes/<site>/assets/                 the original recording
 *   src/scenes/<site>/recordings/<name>/      any further recordings
 * Each directory holds either a scene export (part-000/header.json and
 * part-000/timeline.json) or, for a recording with no 3D playback such as a
 * radar survey, a summary.json:
 *   { "sensor_id", "start": ISO-8601, "duration_sec", "build_version",
 *     "raw_path": dataset-relative path of its raw file, if published,
 *     "vehicle_speed": { same shape as timeline.json's vehicle_speed } }
 * A recording that reports no measurement is left out with a warning: the
 * page would have nothing to show for it.
 *
 * Site-level facts that are not measured come from scenes.json: the posted
 * speed limit (speed_limit_mph, set on the site's mark in
 * tools/s2-archive/map-marks.json) and the capture's raw file in the published
 * dataset (capture.raw_path, named by tools/s2-archive/capture-names.mjs).
 */
const fs = require("fs");
const path = require("path");

const DEFAULT_SCENES = path.join(__dirname, "..", "scenes");
const DRAC_REPORT = path.resolve(
  __dirname,
  "../../../docs/lidar/operations/survey-experimental-drac.md",
);

// Read the reviewed headline table. The public build has no input from the
// local receipt, evidence slices or per-vehicle measurements.
function readSurveyDRAC(markdown) {
  const section = markdown.split(/^## Survey figures\s*$/m)[1];
  const cells = (line) =>
    line
      .trim()
      .slice(1, -1)
      .split("|")
      .map((v) => v.trim());
  const [header, rule, ...rows] = (section ?? "")
    .split(/^## /m)[0]
    .split(/\r?\n/)
    .filter((line) => line.startsWith("|"))
    .map(cells);
  const columns = [
    "Site",
    "Capture",
    "Stationary seconds",
    "Accounted follower-seconds",
    "Supported follower-seconds",
    "Below 1",
    "1 to below 2",
    "2 to below 4",
    "4 and above",
  ];
  if (
    header?.join("|") !== columns.join("|") ||
    rule?.length !== columns.length ||
    rows.length === 0
  ) {
    throw new Error("survey DRAC: missing or changed headline table");
  }
  const bySite = new Map();
  for (const row of rows) {
    const [site, capture, ...values] = row;
    if (
      row.length !== columns.length ||
      bySite.has(site) ||
      !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(site) ||
      !/^\d{8}T\d{4}_[a-z0-9_-]+\.pcapng$/.test(capture) ||
      !capture.endsWith(`_${site}.pcapng`) ||
      !values.every((v) => /^\d+(?:\.\d+)?$/.test(v))
    ) {
      throw new Error(
        `survey DRAC: invalid or duplicate headline row for ${site}`,
      );
    }
    const [stationary, accounted, supported, ...counts] = values.map(Number);
    const episodes = counts.reduce((sum, count) => sum + count, 0);
    if (
      ![stationary, accounted, supported].every(Number.isFinite) ||
      stationary <= 0 ||
      supported > accounted ||
      !Number.isSafeInteger(episodes) ||
      !counts.every(Number.isSafeInteger) ||
      (episodes === 0) !== (supported === 0)
    ) {
      throw new Error(
        `survey DRAC: inconsistent exposure or episode counts for ${site}`,
      );
    }
    const date = `${capture.slice(0, 4)}-${capture.slice(4, 6)}-${capture.slice(6, 8)}`;
    const peak = Math.max(1, ...counts);
    const bands = counts.map((count, i) => ({
      label: ["<1", "1–<2", "2–<4", "4+"][i],
      count,
      bar: (count * 100) / peak,
    }));
    bySite.set(site, {
      capture,
      date,
      stationary,
      accounted,
      supported,
      episodes,
      bands,
    });
  }
  return bySite;
}

function readJSON(file) {
  if (!fs.existsSync(file)) return null;
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch (err) {
    throw new Error(`${file}: ${err.message}`);
  }
}

function recordingDirs(scenesDir, siteId) {
  const dirs = [];
  const assets = path.join(scenesDir, siteId, "assets");
  if (fs.existsSync(assets))
    dirs.push({ dir: assets, url: `/surveys/${siteId}/assets` });
  const extra = path.join(scenesDir, siteId, "recordings");
  if (fs.existsSync(extra)) {
    for (const entry of fs.readdirSync(extra, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      dirs.push({
        dir: path.join(extra, entry.name),
        url: `/surveys/${siteId}/recordings/${entry.name}`,
      });
    }
  }
  return dirs;
}

/**
 * Every file a recording publishes. The player fetches frames as playback
 * reaches them, so saving a page records only the first few; the page links
 * each file so an archiver's outlink capture can reach the rest, and a link
 * costs the page load nothing.
 */
function publishedFiles(dir, url) {
  return fs
    .readdirSync(dir, { recursive: true })
    .filter(
      (file) =>
        !path.basename(file).startsWith(".") &&
        fs.statSync(path.join(dir, file)).isFile(),
    )
    .sort()
    .map((file) => `${url}/${file.split(path.sep).join("/")}`);
}

function startTime(dir, summary, header) {
  const ms = summary
    ? Date.parse(summary.start)
    : header.start_ns == null
      ? NaN
      : Number(BigInt(header.start_ns) / 1000000n);
  if (!Number.isFinite(ms))
    throw new Error(
      `${dir}: no readable start time (summary.json "start" or header.json "start_ns")`,
    );
  return ms;
}

/**
 * One recording. `raw` is the dataset path of its raw file: the site's capture
 * for the original export, or `raw_path` in a later recording's summary.json.
 */
function loadRecording(
  model,
  { dir, url },
  { rawPath = null, dataset = null },
) {
  const summary = readJSON(path.join(dir, "summary.json"));
  const header = readJSON(path.join(dir, "part-000", "header.json"));
  const timeline = readJSON(path.join(dir, "part-000", "timeline.json"));
  const source = summary ?? header;
  if (!source) return null;

  const startMs = startTime(dir, summary, header);
  const durationSec = source.duration_sec ?? timeline?.duration_sec ?? 0;
  const speed = model.summariseSpeed(
    summary ? summary.vehicle_speed : timeline?.vehicle_speed,
  );
  const sensor = model.sensorKind(source.sensor_id);
  const when = model.recordingWhen(startMs, durationSec);
  return {
    id: model.recordingId(startMs),
    label: when.label,
    when,
    startMs,
    startISO: new Date(startMs).toISOString(),
    durationSec,
    sensor,
    sensorId: source.sensor_id ?? null,
    build: source.build_version ?? null,
    tuningHash: header?.tuning_hash ?? null,
    sourceSha256: header?.source_vrlog_sha256 ?? null,
    generatedAt: header?.generated_at ?? null,
    assetsUrl: url,
    files: publishedFiles(dir, url),
    // The raw recording in the published dataset, when it has been named.
    raw: model.datasetLinks(dataset?.url, summary?.raw_path ?? rawPath),
    // The processed file the page offers for download, whichever exists.
    summaryUrl: summary
      ? `${url}/summary.json`
      : timeline
        ? `${url}/part-000/timeline.json`
        : null,
    summaryKind: summary
      ? "speed summary"
      : "speed summary and traffic timeline",
    hasPlayback: Boolean(
      header && fs.existsSync(path.join(dir, "manifest.json")),
    ),
    hasPointCloud: fs.existsSync(path.join(dir, "clip", "manifest.json")),
    // Gerardus's street geometry, written only where the site's pose is
    // accepted (tools/s2-archive/scene-vectors.py).
    hasVectors: fs.existsSync(path.join(dir, "vectors", "vectors.geojson.gz")),
    speed,
    families: speed ? ["speed"] : [],
  };
}

/**
 * Labels and ids must tell a site's recordings apart. Two visits in one
 * month take the day in their label; two starting in the same minute (a
 * radar and a LiDAR survey side by side) take the sensor in their id.
 */
function disambiguate(siteId, recordings, warn) {
  const labels = new Map();
  for (const r of recordings)
    labels.set(r.label, (labels.get(r.label) ?? 0) + 1);
  for (const r of recordings)
    if (labels.get(r.label) > 1) r.label = r.when.labelDay;

  const seen = new Set();
  for (const r of recordings) {
    if (seen.has(r.id)) {
      const base = `${r.id}-${r.sensor}`;
      let id = base;
      for (let n = 2; seen.has(id); n += 1) id = `${base}-${n}`;
      warn(`${siteId}: two recordings start at ${r.id}; using ${id}`);
      r.id = id;
    }
    seen.add(r.id);
  }
}

async function loadSurveys({
  scenesDir = DEFAULT_SCENES,
  index = require("./scenes.json"),
  drac = readSurveyDRAC(fs.readFileSync(DRAC_REPORT, "utf8")),
  warn = (message) => console.warn(`[surveys] ${message}`),
} = {}) {
  const model = await import("../js/survey-model.js");
  const dataset = index.dataset ?? null;

  const sites = index.sites
    .map((site) => {
      const recordings = recordingDirs(scenesDir, site.id)
        .map((d) =>
          loadRecording(model, d, {
            // The site's capture is the original export under assets/.
            rawPath: d.url.endsWith("/assets")
              ? (site.capture?.raw_path ?? null)
              : null,
            dataset,
          }),
        )
        .filter(Boolean)
        .filter((r) => {
          if (r.families.length) return true;
          warn(`${site.id}: ${r.assetsUrl} reports no measurement; left out`);
          return false;
        })
        .sort((a, b) => a.startMs - b.startMs);
      disambiguate(site.id, recordings, warn);
      const latest = recordings.at(-1) ?? null;
      const experimentalDRAC = drac.get(site.id);
      const currentCapture =
        experimentalDRAC?.capture ===
        path.basename(site.capture?.raw_path ?? "");
      if (experimentalDRAC && !currentCapture)
        warn(`${site.id}: experimental DRAC belongs to a different capture`);
      return {
        id: site.id,
        title: site.title,
        page: site.page,
        summary: site.summary,
        published: site.published,
        located: site.located,
        cell: site.cells?.neighbourhood?.display ?? null,
        area: site.cells?.area?.display ?? null,
        // The posted limit, from the site's field mark via scenes.json; null
        // until it is known, and the page then draws no limit line.
        speedLimitMph: model.validLimit(site.speed_limit_mph),
        // Operator-measured angles from the field mark, either of which may be
        // null; the 3D viewer turns its grid and compass by them.
        gridAzimuthDeg: site.grid_azimuth_deg ?? null,
        northAzimuthDeg: site.north_azimuth_deg ?? null,
        capture: site.capture ?? null,
        experimentalDRAC: currentCapture ? experimentalDRAC : null,
        recordings,
        latest,
        sensors: [...new Set(recordings.map((r) => r.sensor))],
        years: [...new Set(recordings.map((r) => r.when.year))].sort(),
        families: model.FAMILIES.filter((f) =>
          recordings.some((r) => r.families.includes(f.id)),
        ).map((f) => f.id),
      };
    })
    .sort((a, b) => a.title.localeCompare(b.title));

  const recordings = sites.flatMap((s) => s.recordings);
  const builds = {};
  for (const s of sites)
    if (s.latest?.build)
      builds[s.latest.build] = (builds[s.latest.build] ?? 0) + 1;
  const [commonBuild, commonCount] = Object.entries(builds).sort(
    (a, b) => b[1] - a[1],
  )[0] ?? [null, 0];

  return {
    families: model.FAMILIES,
    familyById: Object.fromEntries(model.FAMILIES.map((f) => [f.id, f])),
    sensors: model.SENSORS,
    sites,
    byId: Object.fromEntries(sites.map((s) => [s.id, s])),
    totals: {
      sites: sites.length,
      recordings: recordings.length,
      withRecordings: sites.filter((s) => s.recordings.length).length,
      multi: sites.filter((s) => s.recordings.length > 1).length,
    },
    dataset,
    sensorKinds: [...new Set(sites.flatMap((s) => s.sensors))],
    pipeline: { build: commonBuild, count: commonCount, builds },
    years: [...new Set(recordings.map((r) => r.when.year))].sort(),
  };
}

// Eleventy calls the default export with no arguments.
module.exports = () => loadSurveys();
module.exports.loadSurveys = loadSurveys;
module.exports.readSurveyDRAC = readSurveyDRAC;
