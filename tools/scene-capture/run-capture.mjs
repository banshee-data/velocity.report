// Orchestrates one capture run: load and validate the recipe, serve its
// export locally alongside the shared player modules, drive a pinned
// headless Chromium through every requested view, and write PNGs plus a
// provenance manifest. Milestone 1's script, in full.

import {
  mkdir,
  rm,
  writeFile,
  readFile,
  access,
  readdir,
} from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadRecipe } from "./recipe.mjs";
import { startServer } from "./server.mjs";
import { launchCaptureBrowser } from "./browser.mjs";
import { captureView } from "./capture-view.mjs";
import { sha256File, codeRevision, nowIso } from "./manifest.mjs";
import { buildContactSheetHtml, renderContactSheet } from "./contact-sheet.mjs";
import { expandBulletTime, resolveBulletTimeTarget } from "./bullet-time.mjs";
import { sphericalFromEnu } from "../../public_html/src/js/scene-coords.js";
import { serializeCaptureParams } from "../../public_html/src/js/scene-capture.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "../..");
const PUBLIC_JS_DIR = path.join(REPO_ROOT, "public_html/src/js");
const THREE_VENDOR_DIR = path.join(__dirname, "node_modules/three/build");

export class CaptureError extends Error {}

async function exists(p) {
  try {
    await access(p);
    return true;
  } catch {
    return false;
  }
}

async function sourceFiles(currentDir) {
  const entries = await readdir(currentDir, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(currentDir, entry.name);
      if (entry.isDirectory()) return sourceFiles(entryPath);
      return entry.isFile() ? [entryPath] : [];
    }),
  );
  return nested.flat();
}

function isInside(parent, child) {
  const relative = path.relative(parent, child);
  return (
    relative === "" ||
    (!relative.startsWith(`..${path.sep}`) && relative !== "..")
  );
}

/** Hashes every source asset that can affect a captured scene, not metadata alone. */
async function readSourceProvenance(sourceDir) {
  const manifestPath = path.join(sourceDir, "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const partUrl = manifest.parts?.[0]?.url;
  if (typeof partUrl !== "string") {
    throw new CaptureError(`${manifestPath} lists no usable parts`);
  }
  const partDir = path.resolve(sourceDir, partUrl);
  if (!isInside(sourceDir, partDir)) {
    throw new CaptureError(
      `${manifestPath} refers to a part outside the source export`,
    );
  }
  const headerPath = path.join(partDir, "header.json");
  const files = [];
  for (const filePath of (await sourceFiles(sourceDir)).sort((a, b) =>
    a.localeCompare(b),
  )) {
    files.push({
      path: path.relative(sourceDir, filePath),
      sha256: await sha256File(filePath),
    });
  }

  const header = JSON.parse(await readFile(headerPath, "utf8"));
  return {
    files,
    header: {
      source_vrlog_sha256: header.source_vrlog_sha256 ?? null,
      build_version: header.build_version ?? null,
      generated_at: header.generated_at ?? null,
      sensor_id: header.sensor_id ?? null,
      coordinate_frame: header.coordinate_frame ?? null,
    },
  };
}

function viewSpecFor(recipe, view) {
  return {
    camera: view.camera,
    target: view.target,
    fovDeg: view.fovDeg,
    layers: {
      lidar: recipe.layers.lidar,
      boxes: recipe.layers.boxes,
      trails: recipe.layers.trails,
      trailHistorySec: recipe.layers.trailHistorySec,
    },
    frame: recipe.frame,
  };
}

/**
 * Runs one capture: `recipePath` is a path to a recipe JSON file.
 * `overwrite` allows replacing an existing output directory; without it, an
 * existing output directory refuses the run outright.
 *
 * @returns {Promise<{manifest: object, allStable: boolean}>}
 */
export async function runCapture(recipePath, { overwrite = false } = {}) {
  const recipe = await loadRecipe(recipePath);

  if (isInside(recipe.source, recipe.output.dir)) {
    throw new CaptureError("output.dir must not be inside the source export");
  }

  if (await exists(recipe.output.dir)) {
    if (!overwrite) {
      throw new CaptureError(
        `${recipe.output.dir} already exists; pass --overwrite to replace it`,
      );
    }
    await rm(recipe.output.dir, { recursive: true, force: true });
  }
  await mkdir(recipe.output.dir, { recursive: true });
  const diagnosticsDir = path.join(recipe.output.dir, "diagnostics");

  const provenance = await readSourceProvenance(recipe.source);

  const server = await startServer([
    { prefix: "/export/", root: recipe.source },
    { prefix: "/js/", root: PUBLIC_JS_DIR },
    { prefix: "/vendor/three/", root: THREE_VENDOR_DIR },
    { prefix: "/output/", root: recipe.output.dir },
    { prefix: "/", root: __dirname },
  ]);

  const {
    page,
    version: chromiumVersion,
    close: closeBrowser,
  } = await launchCaptureBrowser(recipe.viewport);

  const viewManifests = [];
  let allStable = true;

  try {
    await page.goto(server.url + "/harness.html");
    await page.waitForFunction(
      () => window.__sceneCapture != null || window.__sceneCaptureError != null,
      { timeout: 30_000 },
    );
    const loadError = await page.evaluate(
      () => window.__sceneCaptureError ?? null,
    );
    if (loadError) {
      throw new CaptureError(
        `capture harness failed to load the export: ${loadError}`,
      );
    }

    let viewsToCapture = recipe.views;
    if (recipe.bulletTime) {
      const baseView = recipe.views.find(
        (v) => v.name === recipe.bulletTime.baseView,
      );
      const { tracks } = await page.evaluate(
        (selector) => window.__sceneCapture.resolveFrame(selector),
        recipe.frame,
      );
      let target;
      try {
        target = resolveBulletTimeTarget(recipe.bulletTime.target, tracks);
      } catch (err) {
        throw new CaptureError(err.message);
      }
      const generated = expandBulletTime(
        recipe.bulletTime,
        baseView,
        target,
      ).map((v) => ({ ...v, bulletTime: true }));
      viewsToCapture = generated;
    }

    for (const view of viewsToCapture) {
      const spec = viewSpecFor(recipe, view);
      const reproductionQuery = serializeCaptureParams(spec).toString();
      let outcome;
      try {
        outcome = await captureView(page, "#scene-canvas", spec);
      } catch (err) {
        throw new CaptureError(`view "${view.name}" failed: ${err.message}`);
      }

      const { applyResult, pngs, stable } = outcome;
      const spherical = sphericalFromEnu({
        camera: view.camera,
        target: view.target,
      });

      if (stable) {
        await writeFile(
          path.join(recipe.output.dir, `${view.name}.png`),
          pngs[0],
        );
      } else {
        allStable = false;
        const viewDiagDir = path.join(diagnosticsDir, view.name);
        await mkdir(viewDiagDir, { recursive: true });
        await Promise.all(
          pngs.map((buf, i) =>
            writeFile(path.join(viewDiagDir, `attempt-${i + 1}.png`), buf),
          ),
        );
      }

      // Bullet-time views get an angle+frame caption; plain views get a
      // name+capture-time one — a formatting difference only, since every
      // view already carries the same azimuth/elevation/radius/resolved
      // frame fields regardless of how it was produced.
      const caption = view.bulletTime
        ? `az ${spherical.azimuthDeg.toFixed(0)}° el ${spherical.elevationDeg.toFixed(0)}° · frame ${applyResult.resolvedFrame.frameId}`
        : `${view.name} · ${nowIso()}`;

      viewManifests.push({
        name: view.name,
        file: stable ? `${view.name}.png` : null,
        camera: view.camera,
        target: view.target,
        fov_deg: applyResult.appliedFovDeg,
        azimuth_deg: spherical.azimuthDeg,
        elevation_deg: spherical.elevationDeg,
        radius_m: spherical.radius,
        resolved_frame: applyResult.resolvedFrame,
        effective_trail_history_sec: applyResult.effectiveTrailHistorySec,
        reproduction: {
          query: reproductionQuery,
          harness_url: `${server.url}/harness.html?${reproductionQuery}`,
          viewer_url: recipe.viewerURL
            ? new URL(`?${reproductionQuery}`, recipe.viewerURL).href
            : null,
        },
        caption,
        stability: {
          stable,
          attempts: pngs.length,
          diagnostics_dir: stable ? null : `diagnostics/${view.name}`,
        },
      });
    }

    if (recipe.output.contactSheet && viewManifests.some((v) => v.file)) {
      const items = viewManifests
        .filter((v) => v.file)
        .map((v) => ({ fileName: v.file, caption: v.caption }));
      const html = buildContactSheetHtml(
        items,
        recipe.viewport.width,
        recipe.viewport.height,
      );
      await writeFile(path.join(recipe.output.dir, "contact-sheet.html"), html);
      await renderContactSheet(
        page,
        server.url + "/output/contact-sheet.html",
        items.length,
        recipe.viewport.width,
        recipe.viewport.height,
        path.join(recipe.output.dir, "contact-sheet.png"),
      );
    }
  } finally {
    await closeBrowser();
    await server.close();
  }

  const revision = await codeRevision(REPO_ROOT);
  const manifest = {
    schema_version: 1,
    generated_at: nowIso(),
    source: {
      path: recipe.source,
      files: provenance.files,
      export_header: provenance.header,
    },
    frame_selector: recipe.frame,
    layers: {
      lidar: recipe.layers.lidar,
      boxes: recipe.layers.boxes,
      trails: recipe.layers.trails,
      trail_history_sec: recipe.layers.trailHistorySec,
    },
    viewport: recipe.viewport,
    code_revision: revision,
    browser: { chromium_version: chromiumVersion },
    views: viewManifests,
  };
  await writeFile(
    path.join(recipe.output.dir, "manifest.json"),
    JSON.stringify(manifest, null, 2) + "\n",
  );

  return { manifest, allStable };
}
