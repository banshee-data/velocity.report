#!/usr/bin/env node
/**
 * Regenerate the published scene map and the data the scenes page reads.
 *
 * Input is public_html/scene-sites.json, where a position is hand-entered. Both
 * outputs are derived from it, so a position corrected in one place moves the
 * marker, the cell and the token together.
 */
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { buildSceneMapModel, renderSceneMapSvg, LEVEL_AREA, LEVEL_NEIGHBOURHOOD, LEVEL_SITE } from "./scene-map.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(TOOL_DIRECTORY, "..", "..");
const SOURCE = path.join(REPO_ROOT, "public_html", "scene-sites.json");
const DATA_OUTPUT = path.join(REPO_ROOT, "public_html", "src", "_data", "scenes.json");
const SVG_OUTPUT = path.join(REPO_ROOT, "public_html", "src", "_includes", "scene-map.svg");

export async function generateSceneMap() {
  const source = JSON.parse(await readFile(SOURCE, "utf8"));
  const sites = source.sites ?? [];
  if (sites.length === 0) {
    throw new Error(`${path.relative(REPO_ROOT, SOURCE)} lists no sites.`);
  }

  const model = buildSceneMapModel({ sites });
  const svg = renderSceneMapSvg(model);

  // The page reads only this file, so it carries the derived tokens rather than
  // asking a template to do S2 arithmetic in Nunjucks.
  const data = {
    generated_by: "tools/s2-hilbert/generate-scene-map.mjs",
    levels: { area: LEVEL_AREA, neighbourhood: LEVEL_NEIGHBOURHOOD, site: LEVEL_SITE },
    located_count: model.located.length,
    unlocated_count: model.sites.length - model.located.length,
    sites: model.sites.map((site) => ({
      id: site.id,
      title: site.title,
      page: site.page,
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

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
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
