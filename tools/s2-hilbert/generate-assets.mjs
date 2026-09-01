#!/usr/bin/env node

import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  buildS2HilbertModel,
  renderHilbertSvg,
  renderOrientationLegendSvg,
} from "./index.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const GENERATED_DIRECTORY = path.join(TOOL_DIRECTORY, "generated");
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

async function writeGenerated(filename, content) {
  const destination = path.join(GENERATED_DIRECTORY, filename);
  await writeFile(destination, content, "utf8");
  return destination;
}

export async function generateAssets() {
  await mkdir(GENERATED_DIRECTORY, { recursive: true });
  const written = [];

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
          description: `Sixteen actual S2 L12 descendants of canonical L10 parent ${parent}, connected in ascending CellID order.`,
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

  return { written };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  generateAssets()
    .then(({ written }) => {
      process.stdout.write(
        `${written.map((filename) => path.relative(process.cwd(), filename)).join("\n")}\n`,
      );
    })
    .catch((error) => {
      process.stderr.write(`s2-hilbert assets: ${error.message}\n`);
      process.exitCode = 1;
    });
}
