#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  buildS2CompositeModel,
  compositeAdjacency,
  orientationName,
  renderCompositeSvg,
} from "./index.mjs";

const TOOL_DIRECTORY = path.dirname(fileURLToPath(import.meta.url));
const GENERATED_DIRECTORY = path.join(TOOL_DIRECTORY, "generated");
const SELECTION_PATH = path.join(
  TOOL_DIRECTORY,
  "selection",
  "80858-1-l13-cells.json",
);
const OUTPUT_NAME = "s2-l10-quad-composite.svg";

export const COMPOSITE_RENDER_OPTIONS = {
  title: "Four adjacent S2 L10 cells and their Hilbert orientations",
  description:
    "Canonical parents 808581, 808587, 808f7d and 808f7f drawn through one shared Web Mercator projection so they join along their real S2 edges. Every cell shows its full L13 traversal; 808581 draws its selected cells heavy and its L12 run light, while the other three keep a solid L12 run.",
};

// Listed in reading order; the shared projection puts each one where S2 says.
const HERO = "808581";
const CONTEXT = ["808587", "808f7d", "808f7f"];

export async function generateComposite() {
  const cellSelection = JSON.parse(await readFile(SELECTION_PATH, "utf8"));
  const model = buildS2CompositeModel({
    width: 1400,
    height: 1400,
    padding: 56,
    panels: [
      {
        parent: HERO,
        role: "hero",
        cellSelection,
        showOffMapContinuations: true,
      },
      ...CONTEXT.map((parent) => ({
        parent,
        role: "context",
        showOffMapContinuations: parent === "808587",
      })),
    ],
  });

  const adjacency = compositeAdjacency(model);
  const edges = adjacency.filter((pair) => pair.relation === "edge").length;
  if (edges !== 4) {
    throw new Error(
      `Expected the four L10 cells to share four edges; found ${edges}. Check the parent list.`,
    );
  }

  await mkdir(GENERATED_DIRECTORY, { recursive: true });
  const destination = path.join(GENERATED_DIRECTORY, OUTPUT_NAME);
  await writeFile(
    destination,
    renderCompositeSvg(model, COMPOSITE_RENDER_OPTIONS),
    "utf8",
  );

  return { model, adjacency, destination };
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  generateComposite()
    .then(({ model, adjacency, destination }) => {
      for (const panel of model.panels) {
        const kind =
          panel.role === "hero" ? "hero" : orientationName(panel.parent.token);
        const tiles =
          panel.selectedCount === null
            ? ""
            : `, ${panel.selectedCount} selected tiles`;
        process.stdout.write(`${panel.parent.displayToken}  ${kind}${tiles}\n`);
      }
      for (const pair of adjacency) {
        process.stdout.write(`  ${pair.a} ${pair.relation} ${pair.b}\n`);
      }
      process.stdout.write(`${path.relative(process.cwd(), destination)}\n`);
    })
    .catch((error) => {
      process.stderr.write(`s2-hilbert composite: ${error.message}\n`);
      process.exitCode = 1;
    });
}
