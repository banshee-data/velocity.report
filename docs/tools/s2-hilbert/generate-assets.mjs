#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  buildOrderedChildrenDocument,
  buildS2HilbertModel,
  getHilbertTraversal,
  renderHilbertSvg,
  renderOrientationLegendSvg,
  seedCellSelection,
} from "./index.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const GENERATED_DIRECTORY = path.join(TOOL_DIRECTORY, "generated");
const LAND_MASK_PATH = path.join(TOOL_DIRECTORY, "land", "sf-shoreline-and-islands.geojson");
// Hand-edited input, not generated output: the coastline seeds it once and
// every later run reads it back so manual opt-ins survive regeneration.
const SELECTION_DIRECTORY = path.join(TOOL_DIRECTORY, "selection");
const SELECTION_PATH = path.join(SELECTION_DIRECTORY, "80858-1-l13-cells.json");
const COARSE_PARENTS = ["808581", "808587", "808f7d", "808f7f"];
const ORIENTATION_EXAMPLES = [
  { orientation: 0, label: "canonical", parent: "808587" },
  { orientation: 1, label: "swapped", parent: "808f7f" },
  { orientation: 2, label: "inverted", parent: "808581" },
  { orientation: 3, label: "swapped + inverted", parent: "808585" },
];

function displayFilename(token) {
  return token.length > 5 ? `${token.slice(0, 5)}-${token.slice(5)}` : token;
}

/**
 * Read the committed selection, or seed one from the coastline the first time.
 * An existing file is never overwritten; that is where the manual opt-ins live.
 */
async function loadOrSeedSelection(landMask) {
  try {
    return JSON.parse(await readFile(SELECTION_PATH, "utf8"));
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }

  const seeded = seedCellSelection(getHilbertTraversal("808581", 13), landMask);
  await mkdir(SELECTION_DIRECTORY, { recursive: true });
  await writeFile(SELECTION_PATH, `${JSON.stringify(seeded, null, 2)}\n`, "utf8");
  process.stderr.write(
    `Seeded ${seeded.filter((cell) => cell.enabled).length}/${seeded.length} enabled cells to ${SELECTION_PATH}\n`,
  );
  return seeded;
}

async function writeGenerated(filename, content) {
  const destination = path.join(GENERATED_DIRECTORY, filename);
  await writeFile(destination, content, "utf8");
  return destination;
}

export async function generateAssets() {
  const landMask = JSON.parse(await readFile(LAND_MASK_PATH, "utf8"));
  await mkdir(GENERATED_DIRECTORY, { recursive: true });
  const written = [];

  const cellSelection = await loadOrSeedSelection(landMask);
  const detailed = buildS2HilbertModel({
    parent: "808581",
    targetLevel: 13,
    width: 1200,
    height: 1200,
    padding: 52,
    cellSelection,
  });
  written.push(
    await writeGenerated(
      "80858-1-l13-detailed.svg",
      renderHilbertSvg(detailed, {
        showCells: true,
        title: "80858-1: 64 S2 L13 descendants in Hilbert order",
        description:
          "The actual S2 L13 cells inside canonical parent 808581, projected in Web Mercator. Segments through enabled cells are strong; the rest stay visible at reduced opacity. Chevrons mark the direction of travel.",
      }),
    ),
  );
  written.push(
    await writeGenerated(
      "80858-1-l13.json",
      `${JSON.stringify(buildOrderedChildrenDocument(detailed), null, 2)}\n`,
    ),
  );

  for (const parent of COARSE_PARENTS) {
    const model = buildS2HilbertModel({
      parent,
      targetLevel: 12,
      width: 720,
      height: 720,
      padding: 52,
    });
    written.push(
      await writeGenerated(
        `${displayFilename(parent)}-l12-coarse.svg`,
        renderHilbertSvg(model, {
          showCells: true,
          showLabels: false,
          title: `${displayFilename(parent)}: S2 L12 Hilbert orientation`,
          description:
            `Sixteen actual S2 L12 descendants of canonical L10 parent ${parent}, connected in ascending CellID order.`,
        }),
      ),
    );
  }

  const legendEntries = ORIENTATION_EXAMPLES.map((example) => {
    const model = buildS2HilbertModel({
      parent: example.parent,
      targetLevel: 12,
      width: 190,
      height: 190,
      padding: 13,
      viewBox: [0, 0, 190, 190],
    });
    if (model.parent.orientation !== example.orientation) {
      throw new Error(
        `Expected ${example.parent} to have S2 orientation ${example.orientation}; received ${model.parent.orientation}.`,
      );
    }
    return { ...example, model };
  });
  written.push(
    await writeGenerated("s2-hilbert-orientations.svg", renderOrientationLegendSvg(legendEntries)),
  );

  return { detailed, written };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  generateAssets()
    .then(({ written }) => {
      process.stdout.write(`${written.map((filename) => path.relative(process.cwd(), filename)).join("\n")}\n`);
    })
    .catch((error) => {
      process.stderr.write(`s2-hilbert assets: ${error.message}\n`);
      process.exitCode = 1;
    });
}
