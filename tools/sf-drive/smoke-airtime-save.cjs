// Run against the built site. Only the initial airborne pose is injected.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 760, height: 520 },
      serviceWorkers: "block",
    });
    page.setDefaultTimeout(90000);
    const errors = [];
    page.on("pageerror", (err) => errors.push(err.message));
    await page.route("**/sf-drive.js", async (route) => {
      const response = await route.fetch();
      const text = await response.text();
      const hook =
        "    const crashSupport = crashRoadSupport(terrainHeight, roadSurfaces);";
      assert.ok(text.includes(hook));
      const body = text.replace(
        hook,
        `
    window.airtimeFixture = () => {
      const floor = surface.heightAt(car.x,car.y);
      if (!Number.isFinite(floor)) throw Error("Missing fixture deck");
      car = { ...car, z:floor + 65, v:0, vx:0, vy:0, vz:0, grounded:false, leavingRoad:true };
    };
${hook}`,
      );
      await route.fulfill({ response, body });
    });
    const origin = process.env.SF_BASE_URL || "http://localhost:8090";
    await page.goto(`${origin}/sf/`);
    const grounded = page.locator('#drive[data-contact="grounded"]');
    await grounded.waitFor();
    await page.evaluate(() => window.airtimeFixture());
    const skip = page.getByRole("button", { name: "Skip replay" });
    await skip.waitFor();
    const { airtimes, crashes } = await page.evaluate(() =>
      JSON.parse(localStorage.getItem("sf-replays-v1")),
    );
    assert.equal(airtimes.length, 1);
    assert.equal(crashes.length, 1);
    assert.equal(crashes[0].kind, "ground");
    const flight = airtimes[0];
    assert.equal(flight.ending, "impact");
    assert.ok(flight.seconds > 3.5 && flight.seconds < 3.8);
    assert.equal(flight.end, Math.round(flight.seconds * 120));
    assert.equal(flight.frames[0][0], 0);
    assert.equal(flight.frames.at(-1)[0], flight.end);
    const [score] = await page.evaluate(() =>
      JSON.parse(localStorage.getItem("sf-best-airtimes-v1")),
    );
    assert.equal(score.ending, "impact");
    assert.equal(score.seconds, flight.seconds);
    await skip.click();
    await page.reload();
    await grounded.waitFor();
    await page.getByRole("button", { name: "Map and game settings" }).click();
    await page.getByText("Saved replays", { exact: true }).click();
    await page
      .locator("#airtimeReplays")
      .getByRole("button", { name: /Impact ending/ })
      .click();
    await page.locator('#drive[data-replay="airtime"]').waitFor();
    assert.equal(
      await page.getByRole("img", { name: /Vehicle deceleration/ }).isVisible(),
      true,
    );
    await skip.click();
    assert.deepEqual(errors, []);
    console.log(
      `Chromium saved a ${flight.seconds.toFixed(2)} s hard landing as both airtime and crash. The full airtime replay played from the list after reload.`,
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
