// Launches the pinned headless Chromium the capture harness renders in. The
// exact version is whatever "playwright" resolves to in package.json/the
// lockfile — recorded in the manifest so a capture can be traced back to the
// environment that produced it.

import { chromium } from "playwright";

/**
 * @param {{width: number, height: number}} viewport
 * @returns {Promise<{page: import('playwright').Page, version: string, close: () => Promise<void>}>}
 */
export async function launchCaptureBrowser({ width, height }) {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width, height },
    // Fixed at 1 regardless of host display: the recipe's viewport is the
    // exact pixel size of the output PNG, not a CSS size scaled by whatever
    // screen the script happens to run on.
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();
  const version = browser.version();
  return {
    page,
    version,
    close: () => browser.close(),
  };
}
