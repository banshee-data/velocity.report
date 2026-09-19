#!/usr/bin/env node
/**
 * Place static PCAPNG release pairs below their S2 L13 display-token directory.
 *
 * This is intentionally a move, not a copy: the release set is large, and a
 * pair retains one identity while its filename gains the recording day.
 */
import { cp, mkdir, readFile, rename, stat, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { s2 } from "s2js";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_INDEX = path.join(HERE, "site-index.json");
const DEFAULT_SCENE_ROOT = path.join(
  HERE,
  "..",
  "..",
  "public_html",
  "src",
  "scenes",
);

function displayToken(token) {
  return token.length > 5 ? `${token.slice(0, 5)}-${token.slice(5)}` : token;
}

function l13DisplayToken(site) {
  if (!Number.isFinite(site.lat) || !Number.isFinite(site.lon)) {
    throw new Error(`${site.id}: no usable latitude/longitude`);
  }
  const leaf = s2.cellid.fromLatLng(s2.LatLng.fromDegrees(site.lat, site.lon));
  return displayToken(s2.cellid.toToken(s2.cellid.parent(leaf, 13)));
}

function parseArgs(args) {
  const options = {
    index: DEFAULT_INDEX,
    output: null,
    sceneRoot: DEFAULT_SCENE_ROOT,
    copyVRLOGs: false,
    apply: false,
  };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--index" || arg === "--output" || arg === "--scene-root") {
      const value = args[index + 1];
      if (!value) throw new Error(`${arg} needs a path`);
      if (arg === "--scene-root") options.sceneRoot = value;
      else options[arg.slice(2)] = value;
      index += 1;
    } else if (arg === "--copy-vrlogs") {
      options.copyVRLOGs = true;
    } else if (arg === "--apply") {
      options.apply = true;
    } else if (arg === "--help") {
      console.log(
        "Usage: organise-static-pcaps.mjs --output DIR [--index FILE] [--copy-vrlogs] --apply",
      );
      process.exit(0);
    } else {
      throw new Error(`unknown option: ${arg}`);
    }
  }
  if (!options.output) throw new Error("--output is required");
  return options;
}

async function plan(indexPath, output, sceneRoot) {
  const sites = JSON.parse(await readFile(indexPath, "utf8"));
  const seenTargets = new Set();
  return sites.map((site) => {
    const token = l13DisplayToken(site);
    const stem = `${site.day.replaceAll("-", "")}_${site.id}`;
    const destinationDir = path.join(output, token);
    const targetPCAP = path.join(destinationDir, `${stem}.pcapng`);
    const targetJSON = path.join(destinationDir, `${stem}.json`);
    for (const target of [targetPCAP, targetJSON]) {
      if (seenTargets.has(target))
        throw new Error(`duplicate destination: ${target}`);
      seenTargets.add(target);
    }
    return {
      site,
      sourcePCAP: path.join(output, `${site.id}.pcapng`),
      sourceJSON: path.join(output, `${site.id}.json`),
      targetPCAP,
      targetJSON,
      sourceVRLOG: path.join(sceneRoot, site.published_as, "assets"),
      targetVRLOG: path.join(destinationDir, `${stem}_vrlog`),
    };
  });
}

async function requireAbsent(target, siteID) {
  try {
    await stat(target);
    throw new Error(`${siteID}: destination already exists: ${target}`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
}

async function movePair(item) {
  const [pcapStats, sidecarData] = await Promise.all([
    stat(item.sourcePCAP),
    readFile(item.sourceJSON, "utf8"),
  ]);
  await Promise.all(
    [item.targetPCAP, item.targetJSON].map((target) =>
      requireAbsent(target, item.site.id),
    ),
  );
  const sidecar = JSON.parse(sidecarData);
  if (sidecar.site_id !== item.site.id) {
    throw new Error(`${item.site.id}: sidecar site_id does not match`);
  }
  if (sidecar.output_file !== path.basename(item.sourcePCAP)) {
    throw new Error(`${item.site.id}: sidecar output_file does not match`);
  }
  if (sidecar.output_bytes !== pcapStats.size) {
    throw new Error(
      `${item.site.id}: PCAPNG size no longer matches its sidecar`,
    );
  }
  for (const retiredField of ["site", "source_index", "position_confidence"]) {
    delete sidecar[retiredField];
  }
  sidecar.output_file = path.basename(item.targetPCAP);
  await mkdir(path.dirname(item.targetPCAP), { recursive: true });
  const stagedJSON = `${item.sourceJSON}.new`;
  await writeFile(stagedJSON, `${JSON.stringify(sidecar, null, 2)}\n`);
  await rename(item.sourcePCAP, item.targetPCAP);
  await rename(item.sourceJSON, item.targetJSON);
  await rename(stagedJSON, item.targetJSON);
}

async function copyVRLOG(item) {
  if (!item.site.published_as) {
    throw new Error(`${item.site.id}: no associated published scene`);
  }
  const source = await stat(item.sourceVRLOG);
  if (!source.isDirectory()) {
    throw new Error(`${item.site.id}: web VRLOG export is not a directory`);
  }
  await requireAbsent(item.targetVRLOG, item.site.id);
  await cp(item.sourceVRLOG, item.targetVRLOG, {
    recursive: true,
    force: false,
    errorOnExist: true,
    preserveTimestamps: true,
  });
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const items = await plan(options.index, options.output, options.sceneRoot);
  if (options.copyVRLOGs) {
    for (const item of items) {
      console.log(`${item.sourceVRLOG} -> ${item.targetVRLOG}`);
    }
    if (!options.apply) {
      console.log(
        `dry run: ${items.length} VRLOG exports; pass --apply to copy them`,
      );
      return;
    }
    for (const item of items) {
      await copyVRLOG(item);
    }
    console.log(`copied ${items.length} web VRLOG exports`);
    return;
  }
  for (const item of items) {
    console.log(`${item.sourcePCAP} -> ${item.targetPCAP}`);
  }
  if (!options.apply) {
    console.log(`dry run: ${items.length} pairs; pass --apply to move them`);
    return;
  }
  for (const item of items) {
    await movePair(item);
  }
  console.log(`moved ${items.length} PCAPNG/JSON pairs`);
}

main().catch((error) => {
  console.error(`organise static PCAPs: ${error.message}`);
  process.exit(1);
});
