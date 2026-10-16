#!/usr/bin/env node
/**
 * Regenerate the published scene map and the data the scenes page reads.
 *
 * Three steps, all derived. Each scene export is joined to an archive site by
 * the wall clock in its header, which gives each scene its site and position;
 * the map and the page data then derive from that. So a position corrected
 * once — in tools/s2-archive/map-marks.json, or as an override for a scene the
 * archive does not cover — moves the marker, the cell and the token together.
 *
 * Two files come out: the page data (src/_data/scenes.json) and the map SVG.
 * Nothing else is written, so nothing else can drift. The scene pages are
 * paginated from the page data (src/scenes/survey-pages.11ty.js), and the
 * scene records are held in memory between the steps rather than checked in
 * as a second copy of what scenes.json already carries.
 */
import { mkdir, readFile, readdir, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  LEVEL_AREA,
  LEVEL_NEIGHBOURHOOD,
  LEVEL_SITE,
  buildSceneMapModel,
  renderSceneMapSvg,
} from "./scene-map.mjs";
import {
  buildSceneSites,
  matchSite,
  sceneStartMs,
  unpublishedSites,
} from "./scene-sites.mjs";
import { DATASET_REPO, DATASET_URL } from "../s2-archive/capture-names.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(TOOL_DIRECTORY, "..", "..");
const OVERRIDES = path.join(REPO_ROOT, "public_html", "scene-overrides.json");
const SCENES_DIR = path.join(REPO_ROOT, "public_html", "src", "scenes");
const SITE_INDEX = path.join(
  REPO_ROOT,
  "tools",
  "s2-archive",
  "site-index.json",
);
const MARKS = path.join(REPO_ROOT, "tools", "s2-archive", "map-marks.json");

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
 * Move a published scene to the directory its site now names.
 *
 * A site's id is a slug of the intersection on its field mark, which makes it
 * meaningful and stable against the index being rebuilt — but not against the
 * mark itself being corrected. Renaming "Bush Street near Kearny" to "Bush at
 * Powell" renames the site, and the scene published under the old slug is then
 * unreachable: five scenes answered 404 at once that way.
 *
 * The clock join is what actually identifies a scene, so it can repair this.
 * A directory whose export lands on a site with a different id is renamed to
 * follow it, and the correction costs nothing but a rebuild.
 *
 * A scene mid-export is left alone: its assets are still being written.
 */
async function reconcileSceneDirectories(index) {
  const entries = await readdir(SCENES_DIR, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const dir = path.join(SCENES_DIR, entry.name);
    let header;
    try {
      header = JSON.parse(
        await readFile(
          path.join(dir, "assets", "part-000", "header.json"),
          "utf8",
        ),
      );
    } catch {
      continue; // nothing published here yet
    }
    try {
      await readdir(path.join(dir, "assets.new"));
      continue; // an export is in flight; leave it where it is
    } catch {
      // no in-flight export, so the directory is safe to move
    }
    const startMs = sceneStartMs(header);
    const joined = matchSite({ startMs }, index);
    if (!joined || joined.site.id === entry.name) continue;
    const target = path.join(SCENES_DIR, joined.site.id);
    try {
      await readdir(target);
      continue; // something already occupies the new name; do not clobber it
    } catch {
      await rename(dir, target);
      // Keep the manifest's copy of the id level with the directory, so a
      // reader of the published assets is not told the old name either.
      const manifestPath = path.join(target, "assets", "manifest.json");
      try {
        const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
        manifest.site = {
          ...manifest.site,
          id: joined.site.id,
          title: joined.site.where ?? joined.site.id,
        };
        await writeFile(
          manifestPath,
          `${JSON.stringify(manifest, null, 2)}\n`,
          "utf8",
        );
      } catch {
        // A scene without a readable manifest is not one we can correct.
      }
      process.stdout.write(
        `renamed scene ${entry.name} -> ${joined.site.id}\n`,
      );
    }
  }
}

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

/** Derive the published scene records from the exports, the index and the marks. */
export async function generateSceneSites() {
  const index = JSON.parse(await readFile(SITE_INDEX, "utf8"));
  // Follow any site whose mark was renamed before reading what is published.
  await reconcileSceneDirectories(index);
  const scenes = await discoverScenes();
  if (scenes.length === 0) {
    throw new Error(
      `${path.relative(REPO_ROOT, SCENES_DIR)} holds no published scene exports.`,
    );
  }
  const overrides = JSON.parse(await readFile(OVERRIDES, "utf8")).scenes ?? {};
  const { marks } = JSON.parse(await readFile(MARKS, "utf8"));
  return buildSceneSites({ scenes, index, marks, overrides });
}

/**
 * Group the sites by the level 10 cell that contains them.
 *
 * Sites with no position have no cell either, and go in a group of their own
 * at the end rather than being dropped: a recording nobody has placed yet is
 * still a recording, and hiding it would make the list disagree with the count
 * beside it.
 */
function groupByArea(sites) {
  const groups = new Map();
  for (const site of sites) {
    const area = site.cells?.area;
    const key = area?.token ?? "";
    if (!groups.has(key)) {
      groups.set(key, {
        token: area?.token ?? null,
        display: area?.display ?? null,
        level: area?.level ?? null,
        sites: [],
      });
    }
    groups.get(key).sites.push(site);
  }
  // Placed areas first, in token order so the listing is stable across
  // rebuilds; the unplaced group last.
  return [...groups.values()].sort((a, b) => {
    if (!a.token) return 1;
    if (!b.token) return -1;
    return a.token.localeCompare(b.token);
  });
}

export async function generateSceneMap() {
  const source = await generateSceneSites();
  const scenes = (source.sites ?? []).map((site) => ({
    ...site,
    published: true,
  }));
  if (scenes.length === 0) {
    throw new Error("no published scene joined to a site.");
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
    comment: source.comment,
    generated_by: "tools/s2-hilbert/generate-scene-map.mjs",
    levels: {
      area: LEVEL_AREA,
      neighbourhood: LEVEL_NEIGHBOURHOOD,
      site: LEVEL_SITE,
    },
    located_count: model.located.length,
    unlocated_count: model.sites.length - model.located.length,
    // Grouped by level 10 area, because the area token is the same for every
    // site inside it and repeating it on each card said nothing. As a heading
    // it says something: these are the places that share this cell.
    areas: groupByArea(model.sites),
    published_count: model.sites.filter((s) => s.published).length,
    recorded_count: model.sites.length,
    // Where each capture's raw file is published; sites[].capture names the file.
    dataset: { repo: DATASET_REPO, url: DATASET_URL },
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
      grid_azimuth_deg: site.grid_azimuth_deg ?? null,
      north_azimuth_deg: site.north_azimuth_deg ?? null,
      speed_limit_mph: site.speed_limit_mph ?? null,
      capture: site.capture ?? null,
      cells: site.cells ?? null,
      // The cell outlines the map draws. Tokens name a cell; only the vertices
      // let a viewer see where it actually falls on the street.
      cell_geometry: site.geometry
        ? {
            area: site.geometry.area?.vertices ?? null,
            neighbourhood: site.geometry.neighbourhood?.vertices ?? null,
          }
        : null,
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
