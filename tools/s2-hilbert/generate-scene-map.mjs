#!/usr/bin/env node
/**
 * Regenerate the published scene map and the data the scenes page reads.
 *
 * Three steps, all derived. Each scene export is joined to an archive site by
 * the wall clock in its header, which gives scene-sites.json its positions;
 * the map and the page data then derive from that. So a position corrected
 * once — in tools/s2-archive/map-marks.json, or as an override for a scene the
 * archive does not cover — moves the marker, the cell and the token together.
 */
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  buildSceneMapModel,
  renderSceneMapSvg,
  LEVEL_AREA,
  LEVEL_NEIGHBOURHOOD,
  LEVEL_SITE,
} from "./scene-map.mjs";
import {
  buildSceneSites,
  sceneStartMs,
  unpublishedSites,
} from "./scene-sites.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(TOOL_DIRECTORY, "..", "..");
const SOURCE = path.join(REPO_ROOT, "public_html", "scene-sites.json");
const OVERRIDES = path.join(REPO_ROOT, "public_html", "scene-overrides.json");
const SCENES_DIR = path.join(REPO_ROOT, "public_html", "src", "scenes");
const SITE_INDEX = path.join(
  REPO_ROOT,
  "tools",
  "s2-archive",
  "site-index.json",
);
const DATA_OUTPUT = path.join(
  REPO_ROOT,
  "public_html",
  "src",
  "_data",
  "scenes.json",
);
const SVG_OUTPUT = path.join(
  REPO_ROOT,
  "public_html",
  "src",
  "_includes",
  "scene-map.svg",
);

/**
 * Read every published scene export: its identity from the manifest and its
 * recording clock from the part header, which is what joins it to a site.
 */
async function discoverScenes() {
  const entries = await readdir(SCENES_DIR, { withFileTypes: true });
  const scenes = [];
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const assets = path.join(SCENES_DIR, entry.name, "assets");
    let manifest;
    let header;
    try {
      manifest = JSON.parse(
        await readFile(path.join(assets, "manifest.json"), "utf8"),
      );
      header = JSON.parse(
        await readFile(path.join(assets, "part-000", "header.json"), "utf8"),
      );
    } catch {
      // A directory without an export is a page in progress, not a scene.
      continue;
    }
    scenes.push({
      id: manifest.site?.id ?? entry.name,
      title: manifest.site?.title ?? entry.name,
      startMs: sceneStartMs(header),
      durationSec: header.duration_sec ?? 0,
    });
  }
  return scenes;
}

/** Regenerate scene-sites.json from the exports and the archive index. */
export async function generateSceneSites() {
  const scenes = await discoverScenes();
  if (scenes.length === 0) {
    throw new Error(
      `${path.relative(REPO_ROOT, SCENES_DIR)} holds no published scene exports.`,
    );
  }
  const index = JSON.parse(await readFile(SITE_INDEX, "utf8"));
  const overrides = JSON.parse(await readFile(OVERRIDES, "utf8")).scenes ?? {};
  const doc = buildSceneSites({ scenes, index, overrides });
  await writeFile(SOURCE, `${JSON.stringify(doc, null, 2)}\n`, "utf8");
  return doc;
}

export async function generateSceneMap() {
  const source = await generateSceneSites();
  const scenes = (source.sites ?? []).map((site) => ({
    ...site,
    published: true,
  }));
  if (scenes.length === 0) {
    throw new Error(`${path.relative(REPO_ROOT, SOURCE)} lists no sites.`);
  }

  // Every recorded site goes on the map, not only the published ones, so the
  // map answers "where has this been?" rather than "what can I watch?".
  const index = JSON.parse(await readFile(SITE_INDEX, "utf8"));
  const sites = [...scenes, ...unpublishedSites({ index, scenes })];

  const model = buildSceneMapModel({ sites });
  const svg = renderSceneMapSvg(model);

  // The page reads only this file, so it carries the derived tokens rather than
  // asking a template to do S2 arithmetic in Nunjucks.
  const data = {
    generated_by: "tools/s2-hilbert/generate-scene-map.mjs",
    levels: {
      area: LEVEL_AREA,
      neighbourhood: LEVEL_NEIGHBOURHOOD,
      site: LEVEL_SITE,
    },
    located_count: model.located.length,
    unlocated_count: model.sites.length - model.located.length,
    published_count: model.sites.filter((s) => s.published).length,
    recorded_count: model.sites.length,
    sites: model.sites.map((site) => ({
      id: site.id,
      title: site.title,
      page: site.page,
      published: site.published === true,
      archive_site: site.archive_site ?? null,
      summary: site.summary ?? "",
      located: site.located,
      position: site.position ?? null,
      position_note: site.position_note ?? "",
      cells: site.cells ?? null,
    })),
  };

  await mkdir(path.dirname(DATA_OUTPUT), { recursive: true });
  await mkdir(path.dirname(SVG_OUTPUT), { recursive: true });
  await writeFile(DATA_OUTPUT, `${JSON.stringify(data, null, 2)}\n`, "utf8");
  await writeFile(SVG_OUTPUT, `${svg}\n`, "utf8");

  return { written: [DATA_OUTPUT, SVG_OUTPUT], model };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  generateSceneMap()
    .then(({ written, model }) => {
      process.stdout.write(`${path.relative(process.cwd(), SOURCE)}\n`);
      for (const file of written) {
        process.stdout.write(`${path.relative(process.cwd(), file)}\n`);
      }
      process.stdout.write(
        `${model.located.length} located, ${model.sites.length - model.located.length} awaiting a position\n`,
      );
    })
    .catch((error) => {
      process.stderr.write(`scene map: ${error.message}\n`);
      process.exitCode = 1;
    });
}
