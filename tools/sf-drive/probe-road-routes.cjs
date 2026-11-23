// Exercise short reproductions through the game's browser controls and HUD.
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");
const plan = require("./road-audit-routes.json");

const probes = [
  ["20th Street, Dogpatch", 4210, 30.3, 0.06657, 6],
  ["I-280 south ramp at 5th Street", 3692.1, 1365.6, -1.50423, 4],
  ["Embarcadero centreline", 3993.5, 3933.9, 2.02, 4],
  ["Stockton south approach", 2912.5, 3260.3, 1.734, 4],
  ["Broadway west approach", 1743.7, 4005.3, 0.1685, 4],
];

async function measure(page, seconds) {
  const trace = page.evaluate(async (duration) => {
    const canvas = document.querySelector("#drive");
    const driveBar = document.querySelector("#driveBar");
    const samples = [];
    const end = performance.now() + duration * 1000;
    while (performance.now() < end) {
      await new Promise((next) => requestAnimationFrame(next));
      const d = canvas.dataset;
      samples.push({
        east: +d.east,
        north: +d.north,
        z: +d.altitude,
        speed: +d.speedMps,
        surface: d.surface,
        contact: d.contact,
        crash: driveBar.dataset.crash === "true" || d.replay === "crash",
        blocked: d.roadBlocked === "true",
      });
    }
    return samples;
  }, seconds);
  await page.keyboard.down("ArrowUp");
  const samples = await trace;
  await page.keyboard.up("ArrowUp");
  return samples;
}

async function main() {
  const base = process.env.SF_BASE_URL || "http://localhost:8090";
  const fetchFile = async (name) => {
    const response = await fetch(`${base}/sf/${name}`);
    assert.ok(response.ok, name);
    return response.text();
  };
  const [manifestBytes, questBytes] = await Promise.all([
    fetchFile("tiles/manifest.json"),
    fetchFile("quest-route.json"),
  ]);
  assert.equal(
    createHash("sha256").update(manifestBytes).digest("hex"),
    plan.manifestSha256,
  );
  const manifest = JSON.parse(manifestBytes);
  const quest = JSON.parse(questBytes);
  const { id } = manifest.places[0];
  const progress = manifest.terrainRebake.progressBaseSha256;
  const key = `sf-progress-v1:sf-trial-v2:${progress}:${quest.placementSha256}:${id}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const [name, x, y, heading, seconds] of probes) {
      const page = await browser.newPage({
        viewport: { width: 360, height: 260 },
      });
      page.setDefaultTimeout(60_000);
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        await page.addInitScript(
          ({ key, checkpoint }) => {
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            );
          },
          { key, checkpoint: { x, y, heading } },
        );
        await page.goto(`${base}/sf/?at=${id}`);
        await page.waitForFunction(
          () => document.querySelector("#drive").dataset.contact === "grounded",
        );
        const start = await page
          .locator("#drive")
          .evaluate(({ dataset: d }) => ({
            east: +d.east,
            north: +d.north,
            z: +d.altitude,
          }));
        assert.ok(Math.hypot(start.east - x, start.north - y) < 1, name);
        await page.locator("#drive").focus();
        await confirmManualTakeover(page);
        const samples = await measure(page, seconds);
        assert.ok(
          samples.some(({ speed }) => speed >= 0.5),
          name,
        );
        assert.deepEqual(errors, [], name);
        const z = samples.map(({ z }) => z);
        console.log(
          JSON.stringify({
            name,
            start,
            stop: samples.at(-1),
            minAltitude: Math.min(...z),
            maxAltitude: Math.max(...z),
            firstAirborne: samples.find(
              ({ contact }) => contact === "airborne",
            ),
            firstImpact: samples.find(({ crash }) => crash),
            firstBlock: samples.find(({ blocked }) => blocked),
          }),
        );
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
