// Build and serve public_html/_site on localhost, then run with Playwright on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { tiles } = require("../../public_html/src/sf/tiles/manifest.json");

(async () => {
  const origin = process.env.SF_BASE_URL || "http://localhost:8090";
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    const context = await browser.newContext({
      viewport: { width: 390, height: 690 },
      isMobile: true,
      hasTouch: true,
    });
    const page = await context.newPage();
    page.setDefaultTimeout(90_000);
    await page.goto(`${origin}/sf/`);
    await page.locator('#drive[data-contact="grounded"]').waitFor();
    await page.getByRole("button", { name: "Modes, map and settings" }).tap();
    const button = page
      .getByLabel("City minimap and assets")
      .getByRole("button", { name: "Download all tiles" });
    const status = page.locator("#tileDownloadStatus");
    const cached = () =>
      page.evaluate(async () => {
        const name = (await caches.keys()).find((key) =>
          key.startsWith("sf-tiles-"),
        );
        return (await (await caches.open(name)).keys()).map(
          (request) => request.url,
        );
      });
    const before = await cached();
    assert.ok(before.length > 0 && before.length < tiles.length);
    await page.waitForFunction((total) => {
      const count = document.querySelector("#offlineTileCount").textContent;
      const saved = Number(count.split("/")[0]);
      return saved > 0 && saved < total;
    }, tiles.length);

    await context.setOffline(true);
    await button.tap();
    await status
      .filter({ hasText: "Check your connection, then try again." })
      .waitFor();
    const partial = await cached();
    assert.ok(before.every((url) => partial.includes(url)));

    await context.setOffline(false);
    await button.tap();
    await status
      .filter({ hasText: `All ${tiles.length} tiles saved.` })
      .waitFor();
    await page
      .getByText(`${tiles.length}/${tiles.length} tiles available offline`, {
        exact: true,
      })
      .waitFor();
    const saved = new Set(await cached());
    for (const { t } of tiles)
      assert.ok(saved.has(`${origin}/sf/tiles/${t}.json.gz`));
    console.log(
      `Mobile Chrome retried an offline failure and saved all ${tiles.length} tiles (${before.length} already saved).`,
    );

    await context.setOffline(true);
    await page.goto(`${origin}/sf/?at=columbus`);
    await page.locator('#drive[data-contact="grounded"]').waitFor();
    const places = await page.locator("#places").inputValue();
    assert.equal(places, "columbus");
    console.log(
      "A city start away from the bridge loaded offline after navigation.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
