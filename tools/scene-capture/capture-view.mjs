// Applies one view and checks it rendered to a stable state before trusting
// the pixels. Scene time is frozen throughout (applyView already froze it);
// the only thing that can still change between attempts is something that
// should not exist in a capture-ready page — an animation that was not
// actually stopped, a texture still streaming in, a font swap.

import { PNG } from "pngjs";

const STABILITY_GAP_MS = 500;
const STABILITY_ATTEMPTS = 3;

function buffersEqual(a, b) {
  return Buffer.compare(a, b) === 0;
}

/**
 * @param {import('playwright').Page} page
 * @param {string} canvasSelector
 * @param {object} viewSpec see scene-capture.js's ViewSpec shape
 * @returns {Promise<{applyResult: object, pngs: Buffer[], stable: boolean}>}
 *   pngs holds every attempt in capture order; on failure the caller should
 *   keep all of them for diagnosis rather than just the first.
 */
export async function captureView(page, canvasSelector, viewSpec) {
  const applyResult = await page.evaluate(
    (spec) => window.__sceneCapture.applyView(spec),
    viewSpec,
  );

  const canvas = page.locator(canvasSelector);
  const pngs = [];
  for (let i = 0; i < STABILITY_ATTEMPTS; i++) {
    if (i > 0) await page.waitForTimeout(STABILITY_GAP_MS);
    pngs.push(await canvas.screenshot({ type: "png" }));
  }

  const decoded = pngs.map((buf) => PNG.sync.read(buf));
  const [first, ...rest] = decoded;
  const stable = rest.every(
    (d) =>
      d.width === first.width &&
      d.height === first.height &&
      buffersEqual(d.data, first.data),
  );

  return { applyResult, pngs, stable };
}
