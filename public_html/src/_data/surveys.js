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
 *     "vehicle_speed": { same shape as timeline.json's vehicle_speed } }
 *
 * Site-level settings that are not measured, such as the posted speed limit,
 * live in each page's front matter (speedLimitMph) and are applied in the
 * templates.
 */
const fs = require("fs");
const path = require("path");
const sceneIndex = require("./scenes.json");

const SCENES = path.join(__dirname, "..", "scenes");

function readJSON(file) {
  return fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : null;
}

function recordingDirs(siteId) {
  const dirs = [];
  const assets = path.join(SCENES, siteId, "assets");
  if (fs.existsSync(assets))
    dirs.push({ dir: assets, url: `/surveys/${siteId}/assets` });
  const extra = path.join(SCENES, siteId, "recordings");
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

function loadRecording(model, { dir, url }) {
  const summary = readJSON(path.join(dir, "summary.json"));
  const header = readJSON(path.join(dir, "part-000", "header.json"));
  const timeline = readJSON(path.join(dir, "part-000", "timeline.json"));
  const source = summary ?? header;
  if (!source) return null;

  const startMs = summary
    ? Date.parse(summary.start)
    : Number(BigInt(header.start_ns) / 1000000n);
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
    hasPlayback: Boolean(
      header && fs.existsSync(path.join(dir, "manifest.json")),
    ),
    hasPointCloud: fs.existsSync(path.join(dir, "clip", "manifest.json")),
    speed,
    families: speed ? ["speed"] : [],
  };
}

module.exports = async function () {
  const model = await import("../js/survey-model.js");

  const sites = sceneIndex.sites
    .map((site) => {
      const recordings = recordingDirs(site.id)
        .map((d) => loadRecording(model, d))
        .filter(Boolean)
        .sort((a, b) => a.startMs - b.startMs);
      const latest = recordings.at(-1) ?? null;
      return {
        id: site.id,
        title: site.title,
        page: site.page,
        summary: site.summary,
        published: site.published,
        located: site.located,
        cell: site.cells?.neighbourhood?.display ?? null,
        area: site.cells?.area?.display ?? null,
        recordings,
        latest,
        sensors: [...new Set(recordings.map((r) => r.sensor))],
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
    sensorKinds: [...new Set(sites.flatMap((s) => s.sensors))],
    pipeline: { build: commonBuild, count: commonCount, builds },
    years: [...new Set(recordings.map((r) => r.when.year))].sort(),
  };
};
