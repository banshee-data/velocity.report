// Run against the built site with Playwright on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");
const manifest = require("../../public_html/src/sf/tiles/manifest.json");
const {
  targets,
  placementSha256,
} = require("../../public_html/src/sf/quest-route.json");

(async () => {
  const origin = process.env.SF_BASE_URL || "http://localhost:8090";
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const context = await browser.newContext({
      viewport: { width: 800, height: 560 },
    });
    const page = await context.newPage();
    page.setDefaultTimeout(60_000);
    const errors = [];
    const external = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => {
      const url = request.url();
      if (/^https?:/.test(url) && new URL(url).origin !== origin)
        external.push(url);
    });
    const grounded = () =>
      page.locator('#drive[data-contact="grounded"]').waitFor();
    const bridgeUrl = `${origin}/sf/?city=bay-bridge`;
    await page.goto(bridgeUrl);
    await page
      .locator(
        '#drive[data-surface-id="bay-bridge-westbound"][data-contact="grounded"]',
      )
      .waitFor();
    await page.waitForFunction(
      () => Number(document.querySelector("#drive").dataset.raisedRoads) > 0,
    );
    await context.setOffline(true);
    await page.reload();
    await page
      .locator(
        '#drive[data-surface-id="bay-bridge-westbound"][data-contact="grounded"]',
      )
      .waitFor();
    await context.setOffline(false);

    const url = `${origin}/sf/?at=embarcadero`;
    const target = targets.find(({ t }) => t === "8085806c");
    const progressBase =
      manifest.terrainRebake?.progressBaseSha256 ||
      manifest.roadBake.baseSha256;
    const placement = placementSha256 ? `:${placementSha256}` : "";
    const key = `sf-progress-v1:sf-trial-v2:${progressBase}${placement}:embarcadero`;
    const airtimes = [
      { seconds: 4.2, peak: 6 },
      { seconds: 2.1, peak: 2 },
      { seconds: 1.4, peak: 1 },
    ];
    await page.goto(url);
    await grounded();
    await page.waitForFunction(() => navigator.serviceWorker.controller);
    const warmedTiles = await page.evaluate(async () => {
      const name = (await caches.keys()).find((key) =>
        key.startsWith("sf-tiles-"),
      );
      return name && (await (await caches.open(name)).keys()).length;
    });
    assert.ok(warmedTiles > 0 && warmedTiles < manifest.tiles.length);
    // Begin a saved trial just before the pickup so the offline drive is reproducible.
    await page.evaluate(
      ({ key, target, airtimes }) => {
        localStorage.setItem(
          key,
          JSON.stringify({
            elapsed: 8,
            crashes: 0,
            collected: [],
            checkpoint: {
              x: target.x - 10 * Math.cos(target.heading),
              y: target.y - 10 * Math.sin(target.heading),
              heading: target.heading,
            },
          }),
        );
        localStorage.setItem("sf-best-airtimes-v1", JSON.stringify(airtimes));
      },
      { key, target, airtimes },
    );
    await context.setOffline(true);
    await page.reload();
    await grounded();
    assert.match(await page.locator("#progress").textContent(), /^0\//);
    await confirmManualTakeover(page);
    await page.keyboard.down("ArrowUp");
    await page.waitForFunction(
      ({ key, token }) =>
        JSON.parse(localStorage.getItem(key)).collected.includes(token),
      { key, token: target.t },
    );
    await page.keyboard.up("ArrowUp");
    await page.reload();
    await grounded();
    assert.match(await page.locator("#progress").textContent(), /^1\//);
    await page.locator("#menuBtn").click();
    assert.deepEqual(
      await page.locator("#airtimeRecords li").allTextContents(),
      [
        "4.20 s · 6.0 m clearance",
        "2.10 s · 2.0 m clearance",
        "1.40 s · 1.0 m clearance",
      ],
    );
    // Restore a completed trial while offline, then explicitly start another.
    await page.addInitScript(
      ({ key, tokens }) => {
        const url = new URL(location.href);
        if (!url.searchParams.has("completedTrial")) return;
        const saved = JSON.parse(localStorage.getItem(key));
        localStorage.setItem(
          key,
          JSON.stringify({ ...saved, collected: tokens }),
        );
        url.searchParams.delete("completedTrial");
        history.replaceState(null, "", url);
      },
      { key, tokens: targets.map(({ t }) => t) },
    );
    await page.goto(`${url}&completedTrial=1`);
    await grounded();
    await page.locator("#menuBtn").click();
    await page.getByRole("button", { name: "Start a new emoji trial" }).click();
    await grounded();
    await page.waitForFunction(() =>
      document.querySelector("#progress").textContent.startsWith("0/"),
    );
    await page.reload();
    await grounded();
    assert.match(await page.locator("#progress").textContent(), /^0\//);

    // Emulate Wi-Fi with no internet. The browser still reports online, but
    // uncached adjacent tiles fail. Columbus can use its cached owner tile.
    const unavailable = [];
    page.on("requestfailed", (request) => {
      if (request.url().endsWith(".json.gz")) unavailable.push(request.url());
    });
    await page.addInitScript(() => {
      Object.defineProperty(navigator, "onLine", { value: true });
    });
    await page.goto(`${origin}/sf/?at=columbus`);
    await grounded();
    assert.equal(await page.evaluate(() => navigator.onLine), true);
    assert.ok(unavailable.length);
    console.log(
      "Chrome restarted a completed trial offline and opened a cached start with its online flag set and neighbouring tile requests failing.",
    );
    assert.deepEqual(external, []);
    assert.deepEqual(errors, []);
    console.log(
      `Chrome reopened the Bay Bridge and Embarcadero offline, collected an emoji and restored the pickup and all three airtimes after an offline reload (${warmedTiles} cached tiles).`,
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
