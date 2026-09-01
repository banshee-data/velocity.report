#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

import {
  buildOrderedChildrenDocument,
  buildS2HilbertModel,
  parseCliToken,
  renderHilbertSvg,
} from "./index.mjs";

const HELP = `Usage:
  npm run s2-hilbert -- --parent TOKEN --target-level LEVEL [options]

Required:
  --parent TOKEN          Canonical token, or an explicitly converted display token
  --target-level LEVEL    Descendant level, from the parent level to 30

Output:
  --output FILE           Write SVG to FILE; otherwise print it to stdout
  --json FILE             Write the ordered child data to FILE
  --land-mask FILE        Optional Polygon or MultiPolygon GeoJSON land mask
  --print-ids             Print the ordered child table to stdout

Geometry and presentation:
  --width NUMBER          SVG width (default: 1000)
  --height NUMBER         SVG height (default: 1000)
  --padding NUMBER        Padding in viewBox units (default: 40)
  --view-box "X Y W H"    SVG coordinate system (default: 0 0 WIDTH HEIGHT)
  --show-cells            Draw descendant boundaries (default)
  --hide-cells            Hide descendant boundaries
  --show-labels           Draw display tokens at descendant centres
  --title TEXT            Accessible SVG title
  --help                   Show this help
`;

function numberOption(name, value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) throw new Error(`${name} requires a finite number.`);
  return parsed;
}

function nextArgument(arguments_, index, name) {
  const value = arguments_[index + 1];
  if (value === undefined || value.startsWith("--")) {
    throw new Error(`${name} requires a value.`);
  }
  return value;
}

export function parseArguments(arguments_) {
  const options = {
    width: 1000,
    height: 1000,
    padding: 40,
    showCells: true,
    showLabels: false,
    printIds: false,
  };

  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    if (argument === "--help") return { help: true };
    if (["--show-cells", "--hide-cells", "--show-labels", "--print-ids"].includes(argument)) {
      if (argument === "--show-cells") options.showCells = true;
      if (argument === "--hide-cells") options.showCells = false;
      if (argument === "--show-labels") options.showLabels = true;
      if (argument === "--print-ids") options.printIds = true;
      continue;
    }

    const value = nextArgument(arguments_, index, argument);
    index += 1;
    if (argument === "--parent") options.parent = parseCliToken(value);
    else if (argument === "--target-level") options.targetLevel = numberOption(argument, value);
    else if (argument === "--width") options.width = numberOption(argument, value);
    else if (argument === "--height") options.height = numberOption(argument, value);
    else if (argument === "--padding") options.padding = numberOption(argument, value);
    else if (argument === "--output") options.output = value;
    else if (argument === "--json") options.json = value;
    else if (argument === "--land-mask") options.landMask = value;
    else if (argument === "--title") options.title = value;
    else if (argument === "--view-box") {
      options.viewBox = value
        .trim()
        .split(/[ ,]+/)
        .map((part) => numberOption(argument, part));
    } else throw new Error(`Unknown option: ${argument}.`);
  }

  if (!options.parent) throw new Error("--parent is required.");
  if (!Number.isInteger(options.targetLevel)) {
    throw new Error("--target-level is required and must be an integer.");
  }
  return options;
}

async function writeTextFile(filename, content) {
  const resolved = path.resolve(filename);
  await mkdir(path.dirname(resolved), { recursive: true });
  await writeFile(resolved, content, "utf8");
  return resolved;
}

function printChildren(model) {
  process.stdout.write("index\ttoken\tlatitude\tlongitude\n");
  for (const cell of model.cells) {
    process.stdout.write(
      `${cell.index}\t${cell.token}\t${cell.centre.lat.toFixed(9)}\t${cell.centre.lng.toFixed(9)}\n`,
    );
  }
}

export async function main(arguments_ = process.argv.slice(2)) {
  const options = parseArguments(arguments_);
  if (options.help) {
    process.stdout.write(HELP);
    return;
  }

  const landMask = options.landMask
    ? JSON.parse(await readFile(path.resolve(options.landMask), "utf8"))
    : undefined;
  const model = buildS2HilbertModel({
    parent: options.parent,
    targetLevel: options.targetLevel,
    width: options.width,
    height: options.height,
    padding: options.padding,
    viewBox: options.viewBox,
    landMask,
  });
  const svg = renderHilbertSvg(model, {
    showCells: options.showCells,
    showLabels: options.showLabels,
    title: options.title,
  });

  if (options.output) {
    const svgPath = await writeTextFile(options.output, svg);
    process.stderr.write(`Wrote ${model.cells.length} cells to ${svgPath}\n`);
  } else {
    process.stdout.write(svg);
  }
  if (options.json) {
    const jsonPath = await writeTextFile(
      options.json,
      `${JSON.stringify(buildOrderedChildrenDocument(model), null, 2)}\n`,
    );
    process.stderr.write(`Wrote ordered child data to ${jsonPath}\n`);
  }
  if (options.printIds) printChildren(model);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    process.stderr.write(`s2-hilbert: ${error.message}\n`);
    process.exitCode = 1;
  });
}
