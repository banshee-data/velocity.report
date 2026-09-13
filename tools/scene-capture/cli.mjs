#!/usr/bin/env node
// Usage: node cli.mjs <recipe.json> [--overwrite]

import { runCapture, CaptureError } from "./run-capture.mjs";
import { RecipeError } from "./recipe.mjs";

function parseArgs(argv) {
  const args = argv.slice(2);
  const overwrite = args.includes("--overwrite");
  const recipePath = args.find((a) => !a.startsWith("--"));
  return { recipePath, overwrite };
}

async function main() {
  const { recipePath, overwrite } = parseArgs(process.argv);
  if (!recipePath) {
    console.error("Usage: node cli.mjs <recipe.json> [--overwrite]");
    process.exitCode = 2;
    return;
  }

  let result;
  try {
    result = await runCapture(recipePath, { overwrite });
  } catch (err) {
    if (err instanceof RecipeError || err instanceof CaptureError) {
      console.error(err.message);
      process.exitCode = 1;
      return;
    }
    throw err;
  }

  const { manifest, allStable } = result;
  console.log(
    `Captured ${manifest.views.length} view(s) from ${manifest.source.path}:`,
  );
  for (const v of manifest.views) {
    console.log(
      `  ${v.name}: ${v.stability.stable ? v.file : `UNSTABLE (see ${v.stability.diagnostics_dir})`}`,
    );
  }
  if (!allStable) {
    console.error(
      "One or more views did not render to a stable state; see diagnostics/ in the output directory.",
    );
    process.exitCode = 1;
  }
}

main().catch((err) => {
  console.error(err.stack ?? String(err));
  process.exitCode = 1;
});
