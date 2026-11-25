// Drive ground streets beneath their baked overhead scenery.
const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

const origin = process.env.SF_BASE_URL || "http://localhost:8090";
const candidate = process.env.SF_CANDIDATE_DIR;
const mode = process.argv[2] ?? "main";
assert.ok(mode === "main" || mode === "howard", "Expected main or howard");
const howard = mode === "howard";
const tile = howard ? "80858084" : "80858074";
const outputs = [
  "tiles/manifest.json",
  `tiles/${tile}.json.gz`,
  "quest-route.json",
];

function checkpoints() {
  if (howard) {
    const heading = -2.35725;
    return [-4, 0, 4].map((offset) => ({
      name: `howard-${offset}m`,
      x: 3411.907 - Math.sin(heading) * offset,
      y: 2709.407 + Math.cos(heading) * offset,
      heading,
      slope: 0,
      targetDistance: 132,
      timeLimit: 26,
      heightRange: [9, 10.2],
    }));
  }
  return [
    ["northwest", 4487, 3012.5, (3 * Math.PI) / 4, 0.032],
    ["southeast", 4429, 3070.7, -Math.PI / 4, -0.032],
  ].map(([name, x, y, heading, slope]) => ({
    name,
    x,
    y,
    heading,
    slope,
    targetDistance: 78,
    timeLimit: 22,
    heightRange: [3.8, 8],
  }));
}

async function controls(canvas, checkpoint) {
  return canvas.evaluate(async (element, route) => {
    const { x, y, heading, slope, targetDistance, timeLimit } = route;
    const start = performance.now();
    const direction = [Math.cos(heading), Math.sin(heading)];
    const heights = [];
    let throttle;
    let lastGrounded = start;
    let longestAirborneMs = 0;
    let maxHeightDeviation = 0;
    const initialZ = +element.dataset.altitude;
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
      const now = performance.now();
      const seconds = (now - start) / 1000;
      if (d.contact === "grounded") lastGrounded = now;
      longestAirborneMs = Math.max(longestAirborneMs, now - lastGrounded);
      // Terrain contact can briefly disengage at a mesh seam. The altitude
      // over the sampled street profile must still stay near lane level.
      maxHeightDeviation = Math.max(
        maxHeightDeviation,
        Math.abs(+d.altitude - initialZ - slope * distance),
      );
      if (d.surfaceId || Math.abs(lateral) > 3 || maxHeightDeviation > 0.55) {
        accelerate(false);
        return {
          distance,
          seconds,
          streetContact: false,
          lateral,
          surfaceId: d.surfaceId,
          contact: d.contact,
          longestAirborneMs,
          maxHeightDeviation,
        };
      }
      heights.push(+d.altitude);
      if (
        distance >= targetDistance ||
        seconds > timeLimit ||
        d.replay === "crash"
      ) {
        accelerate(false);
        return {
          distance,
          seconds,
          streetContact: true,
          longestAirborneMs,
          maxHeightDeviation,
          minZ: Math.min(...heights),
          maxZ: Math.max(...heights),
          contact: d.contact,
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
    for (const checkpoint of checkpoints()) {
      const { name, targetDistance, heightRange, x, y, heading } = checkpoint;
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
        if (howard) {
          const [east, north] = await canvas.evaluate(({ dataset }) => [
            Number(dataset.east),
            Number(dataset.north),
          ]);
          assert.ok(
            Math.hypot(east - x, north - y) < 1,
            `Wrong start for ${name}`,
          );
        }
        await canvas.focus();
        await confirmManualTakeover(page);
        const result = await controls(canvas, checkpoint);
        console.log(
          JSON.stringify({
            direction: name,
            manifest: quest.manifestSha256,
            ...result,
            errors,
            external,
          }),
        );
        assert.ok(result.distance >= targetDistance, name);
        assert.ok(result.streetContact, name);
        assert.ok(
          result.minZ > heightRange[0] && result.maxZ < heightRange[1],
          name,
        );
        if (howard) {
          assert.equal(result.contact, "grounded");
          assert.equal(result.longestAirborneMs, 0);
        }
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
