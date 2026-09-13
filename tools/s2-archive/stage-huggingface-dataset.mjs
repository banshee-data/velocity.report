#!/usr/bin/env node
/** Stage the static S2 release into the shallow Hugging Face dataset layout. */
import {
  mkdir,
  readFile,
  readdir,
  rename,
  stat,
  writeFile,
} from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { s2 } from "s2js";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_INDEX = path.join(HERE, "site-index.json");
const DEFAULT_SOURCE = "/Volumes/lidar/lidar/s2/static-huggingface";
const TIMEZONE = "America/Los_Angeles";

function parseArgs(args) {
  const options = {
    index: DEFAULT_INDEX,
    source: DEFAULT_SOURCE,
    output: null,
    apply: false,
  };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (["--index", "--source", "--output"].includes(arg)) {
      const value = args[index + 1];
      if (!value) throw new Error(`${arg} needs a path`);
      options[arg.slice(2)] = value;
      index += 1;
    } else if (arg === "--apply") {
      options.apply = true;
    } else if (arg === "--help") {
      console.log(
        "Usage: stage-huggingface-dataset.mjs --output DIR [--source DIR] [--index FILE] --apply",
      );
      process.exit(0);
    } else {
      throw new Error(`unknown option: ${arg}`);
    }
  }
  if (!options.output) throw new Error("--output is required");
  return options;
}

function tokenDisplay(token) {
  return `${token.slice(0, 5)}-${token.slice(5)}`;
}

function timestampParts(timestamp) {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-CA", {
      timeZone: TIMEZONE,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    })
      .formatToParts(new Date(timestamp))
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  );
  return `${parts.year}${parts.month}${parts.day}T${parts.hour}${parts.minute}`;
}

function itemFor(site, source, output) {
  const leaf = s2.cellid.fromLatLng(s2.LatLng.fromDegrees(site.lat, site.lon));
  const l13 = s2.cellid.toToken(s2.cellid.parent(leaf, 13));
  const l10 = s2.cellid.toToken(s2.cellid.parent(leaf, 10));
  const display = tokenDisplay(l13);
  const timestamp = timestampParts(site.start);
  const stem = `${timestamp}_${display}_${site.id}`;
  const legacyStem = `${site.day.replaceAll("-", "")}_${site.id}`;
  return {
    site,
    stem,
    l10,
    l13,
    sourcePCAP: path.join(source, display, `${legacyStem}.pcapng`),
    sourceJSON: path.join(source, display, `${legacyStem}.json`),
    sourceScene: path.join(source, display, `${legacyStem}_vrlog`),
    targetPCAP: path.join(output, "raw", "lidar", `${stem}.pcapng`),
    targetScene: path.join(output, "derived", "scenes", `${stem}_vrlog`),
  };
}

async function assertAbsent(target) {
  try {
    await stat(target);
    throw new Error(`destination already exists: ${target}`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
}

async function loadItem(item) {
  const [pcap, scene, sidecarText] = await Promise.all([
    stat(item.sourcePCAP),
    stat(item.sourceScene),
    readFile(item.sourceJSON, "utf8"),
  ]);
  if (!scene.isDirectory())
    throw new Error(`${item.site.id}: scene export is not a directory`);
  const sidecar = JSON.parse(sidecarText);
  if (sidecar.site_id !== item.site.id)
    throw new Error(`${item.site.id}: sidecar site_id does not match`);
  if (sidecar.output_bytes !== pcap.size)
    throw new Error(`${item.site.id}: PCAPNG size does not match sidecar`);
  return { ...item, pcapBytes: pcap.size, sidecar };
}

function manifestEntry(item) {
  const duration = (new Date(item.site.end) - new Date(item.site.start)) / 1000;
  return {
    capture_id: `${item.stem.replace(item.l13.slice(0, 5) + "-" + item.l13.slice(5), item.l13)}`,
    timestamp: item.site.start,
    filename_timestamp: item.stem.slice(0, 13),
    timezone: TIMEZONE,
    latitude: item.site.lat,
    longitude: item.site.lon,
    s2_l10: item.l10,
    s2_l13: item.l13,
    site_slug: item.site.id,
    sensor_type: "lidar",
    sensor_model: null,
    capture_type: "static",
    duration_seconds: duration,
    raw_path: path.posix.join("raw", "lidar", path.basename(item.targetPCAP)),
    scene_path: path.posix.join(
      "derived",
      "scenes",
      path.basename(item.targetScene),
    ),
    tracks_path: null,
    map_path: null,
    metrics_path: null,
    report_path: null,
    pipeline_version: item.sidecar.pcap_split_build_version,
    pipeline_git_sha: null,
    raw_sha256: item.sidecar.output_sha256,
    raw_bytes: item.pcapBytes,
    export_provenance: {
      static_export_policy: item.sidecar.static_export_policy,
      operator_overrides: item.sidecar.operator_overrides,
      source_files: item.sidecar.source_files,
    },
  };
}

function datasetReadme() {
  return `---\nconfigs:\n- config_name: manifest\n  data_files: manifest.json\n---\n\n# San Francisco street-speed dataset\n\nThis release keeps raw evidence under \`raw/\` and derived outputs under \`derived/\`. The authoritative corpus index is \`manifest.json\`. Filenames use \`<timestamp>_<s2-l13-display>_<site-slug>\`; timestamps are America/Los_Angeles. Canonical S2 tokens and all richer provenance live in the manifest.\n\n## Layout\n\n\`\`\`text\nraw/lidar/\nderived/scenes/\nderived/tracks/\nderived/maps/\nderived/metrics/\nreports/\nmanifest.json\n\`\`\`\n\nThe scene outputs are chunked web VRLOG export directories, so they retain their internal part/frame structure below \`derived/scenes/\`. No train/test split is encoded in paths; define subsets or splits in this card's YAML when needed.\n`;
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const rootEntries = await readdir(options.output).catch((error) => {
    if (error.code === "ENOENT") return [];
    throw error;
  });
  if (rootEntries.length > 0)
    throw new Error(`${options.output}: target must be empty`);
  const sites = JSON.parse(await readFile(options.index, "utf8"));
  const items = await Promise.all(
    sites.map((site) =>
      loadItem(itemFor(site, options.source, options.output)),
    ),
  );
  const ids = new Set(items.map((item) => item.stem));
  if (ids.size !== items.length)
    throw new Error("duplicate target filename stem");
  for (const item of items)
    console.log(`${item.sourcePCAP} -> ${item.targetPCAP}`);
  if (!options.apply) {
    console.log(
      `dry run: ${items.length} captures, ${items.reduce((sum, item) => sum + item.pcapBytes, 0)} PCAPNG bytes; pass --apply to move`,
    );
    return;
  }
  for (const directory of [
    "raw/lidar",
    "raw/radar",
    "derived/scenes",
    "derived/tracks",
    "derived/maps",
    "derived/metrics",
    "reports",
  ]) {
    await mkdir(path.join(options.output, directory), { recursive: true });
  }
  for (const item of items) {
    await Promise.all([
      assertAbsent(item.targetPCAP),
      assertAbsent(item.targetScene),
    ]);
    await rename(item.sourcePCAP, item.targetPCAP);
    await rename(item.sourceScene, item.targetScene);
  }
  await writeFile(
    path.join(options.output, "manifest.json"),
    `${JSON.stringify(items.map(manifestEntry), null, 2)}\n`,
  );
  await writeFile(path.join(options.output, "README.md"), datasetReadme());
  console.log(
    `moved ${items.length} captures and scene exports; wrote manifest.json; legacy sidecars retained at source`,
  );
}

main().catch((error) => {
  console.error(`stage Hugging Face dataset: ${error.message}`);
  process.exit(1);
});
