// Traverse the baked Stockton tunnel in both directions using game controls.
const assert = require("node:assert/strict");
const { gunzipSync } = require("node:zlib");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

async function main() {
  const base = process.env.SF_BASE_URL || "http://localhost:8090";
  const fetchAsset = async (name) => {
    const response = await fetch(`${base}/sf/${name}`);
    assert.ok(response.ok, name);
    return Buffer.from(await response.arrayBuffer());
  };
  const [manifest, quest, tile] = await Promise.all([
    fetchAsset("tiles/manifest.json").then(JSON.parse),
    fetchAsset("quest-route.json").then(JSON.parse),
    fetchAsset("tiles/8085808c.json.gz").then(gunzipSync).then(JSON.parse),
  ]);
  const road = tile.playableRoads.find(({ kind }) => kind === "tunnel");
  assert.ok(road, "no baked Stockton tunnel");
  const key = `sf-progress-v1:sf-trial-v2:${manifest.terrainRebake.progressBaseSha256}:${quest.placementSha256}:downtown`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const [label, path] of [
      ["Northbound", road.points],
      ["Southbound", [...road.points].reverse()],
      ["Above the tunnel", null],
    ]) {
      const page = await browser.newPage({
        viewport: { width: 640, height: 420 },
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      page.setDefaultTimeout(60_000);
      try {
        const [x, y] = path?.[0] || [2879, 3457];
        const heading = path ? Math.atan2(path[1][1] - y, path[1][0] - x) : 0;
        const back = path ? 7 : 0;
        const checkpoint = {
          x: x - back * Math.cos(heading),
          y: y - back * Math.sin(heading),
          heading,
        };
        await page.addInitScript(
          ({ key, checkpoint }) =>
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            ),
          { key, checkpoint },
        );
        await page.goto(`${base}/sf/?at=downtown`);
        const drive = page.locator("#drive");
        await page.locator('#drive[data-contact="grounded"]').waitFor();
        const snapshot = () =>
          drive.evaluate(({ dataset: d }) => ({
            x: Number(d.east),
            y: Number(d.north),
            altitude: Number(d.altitude),
            speed: Number(d.speedMps),
            surface: d.surfaceId,
            camera: d.cameraContact,
            blocked: d.roadBlocked === "true",
            replay: d.replay,
          }));
        const start = await snapshot();
        assert.ok(
          Math.hypot(start.x - checkpoint.x, start.y - checkpoint.y) < 1,
        );
        assert.equal(start.surface, "");
        assert.equal(start.camera, "exterior");
        if (!path) {
          assert.ok(start.altitude > 47);
          assert.deepEqual(errors, []);
          console.log(label, start);
          continue;
        }
        await drive.focus();
        await confirmManualTakeover(page);
        let entry, interior, exit;
        let holding = false;
        const limit = Date.now() + 120_000;
        while (Date.now() < limit) {
          const sample = await snapshot();
          assert.ok(!sample.blocked && !sample.replay, JSON.stringify(sample));
          const accelerate = sample.speed < 13;
          if (accelerate !== holding) {
            if (accelerate) await page.keyboard.down("ArrowUp");
            else await page.keyboard.up("ArrowUp");
            holding = accelerate;
          }
          if (sample.surface === road.id) {
            entry ||= sample;
            assert.equal(sample.camera, "tunnel");
            if (!interior && Math.abs(sample.y - 3500) < 12) {
              interior = sample;
              if (process.env.SF_TUNNEL_SCREENSHOT && label === "Northbound")
                await page.screenshot({
                  path: process.env.SF_TUNNEL_SCREENSHOT,
                });
            }
          } else if (entry) {
            exit = sample;
            break;
          }
          await page.waitForTimeout(180);
        }
        await page.keyboard.up("ArrowUp");
        assert.ok(
          entry && interior && exit,
          JSON.stringify({ entry, interior, exit }),
        );
        assert.equal(exit.camera, "exterior");
        assert.ok(
          Math.abs(exit.altitude - (label === "Northbound" ? 42.2 : 28.2)) <
            0.4,
        );
        assert.deepEqual(errors, []);
        console.log(label, { entry, interior, exit });
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
