// Drive Main Street beneath the Bay Bridge in both directions.
const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

const origin = process.env.SF_BASE_URL || "http://localhost:8090";
const candidate = process.env.SF_CANDIDATE_DIR;
const outputs = [
  "tiles/manifest.json",
  "tiles/80858074.json.gz",
  "quest-route.json",
];

async function controls(canvas, checkpoint) {
  return canvas.evaluate(async (element, { x, y, heading }) => {
    const start = performance.now();
    const direction = [Math.cos(heading), Math.sin(heading)];
    const heights = [];
    let throttle;
    const accelerate = (enabled) => {
      if (throttle === enabled) return;
      throttle = enabled;
      element.dispatchEvent(
        new KeyboardEvent(enabled ? "keydown" : "keyup", {
          code: "ArrowUp",
          bubbles: true,
          cancelable: true,
        }),
      );
    };
    for (;;) {
      const d = element.dataset;
      const dx = +d.east - x;
      const dy = +d.north - y;
      const distance = dx * direction[0] + dy * direction[1];
      const lateral = dy * direction[0] - dx * direction[1];
      const seconds = (performance.now() - start) / 1000;
      if (d.contact !== "grounded" || d.surfaceId || Math.abs(lateral) > 3) {
        accelerate(false);
        return { distance, seconds, streetContact: false, lateral };
      }
      heights.push(+d.altitude);
      if (distance >= 78 || seconds > 22 || d.replay === "crash") {
        accelerate(false);
        return {
          distance,
          seconds,
          streetContact: true,
          minZ: Math.min(...heights),
          maxZ: Math.max(...heights),
          crashed: d.replay === "crash",
        };
      }
      accelerate(+d.speedMps < 9);
      await new Promise(requestAnimationFrame);
    }
  }, checkpoint);
}

async function main() {
  async function data(path) {
    if (candidate) return JSON.parse(readFileSync(join(candidate, path)));
    const response = await fetch(`${origin}/sf/${path}`);
    assert.ok(response.ok, path);
    return response.json();
  }
  const [manifest, quest] = await Promise.all([
    data("tiles/manifest.json"),
    data("quest-route.json"),
  ]);
  const key = `sf-progress-v1:sf-trial-v2:${manifest.terrainRebake.progressBaseSha256}:${quest.placementSha256}:downtown`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const [direction, x, y, heading] of [
      ["northwest", 4487, 3012.5, (3 * Math.PI) / 4],
      ["southeast", 4429, 3070.7, -Math.PI / 4],
    ]) {
      const page = await browser.newPage({
        viewport: { width: 840, height: 540 },
        serviceWorkers: "block",
      });
      try {
        const errors = [];
        const external = [];
        page.on("pageerror", (error) => errors.push(error.message));
        page.on("request", (request) => {
          const url = new URL(request.url());
          if (/^https?:$/.test(url.protocol) && url.origin !== origin)
            external.push(url.href);
        });
        if (candidate)
          await page.route("**/sf/**", (route) => {
            const path = new URL(route.request().url()).pathname.slice(4);
            if (!outputs.includes(path)) return route.continue();
            return route.fulfill({
              path: join(candidate, path),
              contentType: path.endsWith(".gz")
                ? "application/gzip"
                : "application/json",
            });
          });
        await page.addInitScript(
          ({ key, checkpoint }) => {
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            );
          },
          { key, checkpoint: { x, y, heading } },
        );
        await page.goto(`${origin}/sf/?at=downtown`);
        const canvas = page.locator('#drive[data-contact="grounded"]');
        await canvas.waitFor({ timeout: 60_000 });
        await canvas.focus();
        await confirmManualTakeover(page);
        const result = await controls(canvas, { x, y, heading });
        console.log(
          JSON.stringify({
            direction,
            manifest: quest.manifestSha256,
            ...result,
            errors,
            external,
          }),
        );
        assert.ok(result.distance >= 78, direction);
        assert.ok(result.streetContact, direction);
        assert.ok(result.minZ > 3.8 && result.maxZ < 8, direction);
        assert.equal(result.crashed, false);
        assert.deepEqual(errors, []);
        assert.deepEqual(external, []);
      } finally {
        await page.close();
      }
    }
  } finally {
    await browser.close();
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
