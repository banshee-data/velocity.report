// Check the 5th Street ramp through the real game's normal manual controls.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

(async () => {
  const base = process.env.SF_BASE_URL || "http://localhost:8090";
  const [manifest, quest] = await Promise.all(
    ["tiles/manifest.json", "quest-route.json"].map(async (path) => {
      const response = await fetch(`${base}/sf/${path}`);
      assert.ok(response.ok, path);
      return response.json();
    }),
  );
  assert.ok(manifest.roadBake.playableSha256, "structure trial is not baked");
  const id = "downtown";
  const key = `sf-progress-v1:sf-trial-v2:${manifest.terrainRebake.progressBaseSha256}:${quest.placementSha256}:${id}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const probe of [
      { x: 3823.18, y: 1606.19, heading: -2.35715, isEntry: true },
      { x: 3692.74, y: 1355.95, heading: 0, isEntry: false },
    ]) {
      const page = await browser.newPage({
        viewport: { width: 420, height: 320 },
      });
      const errors = [];
      const external = [];
      page.setDefaultTimeout(60_000);
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("request", (request) => {
        const url = new URL(request.url());
        if (/^https?:$/.test(url.protocol) && url.origin !== base)
          external.push(url.href);
      });
      try {
        await page.addInitScript(
          ({ key, checkpoint }) => {
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            );
          },
          { key, checkpoint: probe },
        );
        await page.goto(`${base}/sf/?at=${id}`);
        const canvas = page.locator("#drive");
        await page.locator('#drive[data-contact="grounded"]').waitFor();
        const snapshot = () =>
          canvas.evaluate(({ dataset: d }) => ({
            x: +d.east,
            y: +d.north,
            z: +d.altitude,
            surface: d.surfaceId,
          }));
        const start = await snapshot();
        assert.ok(
          Math.hypot(start.x - probe.x, start.y - probe.y) < 1,
          JSON.stringify(start),
        );
        assert.equal(start.surface, "");
        if (probe.isEntry) {
          await canvas.focus();
          await confirmManualTakeover(page);
          await page.keyboard.down("ArrowUp");
          try {
            await page.waitForFunction(
              () => {
                const d = document.querySelector("#drive").dataset;
                return (
                  d.surfaceId === "sf-contact/i280-fifth-south-entry" &&
                  Number(d.altitude) >= 3.25
                );
              },
              null,
              { timeout: 18_000 },
            );
          } finally {
            await page.keyboard.up("ArrowUp");
          }
          console.log(
            "5th Street entry climbed onto layered contact",
            await snapshot(),
          );
        } else {
          assert.ok(start.z >= 3 && start.z < 3.3, JSON.stringify(start));
          console.log(
            "The street under the new ramp retained its original level",
            start,
          );
        }
        assert.deepEqual(errors, []);
        assert.deepEqual(external, []);
      } finally {
        await page.close();
      }
    }
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
