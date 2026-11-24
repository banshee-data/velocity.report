// Run with Playwright available via NODE_PATH and Chromium on the Hydra dev host.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

(async () => {
  const host = process.env.SF_BASE_URL || "http://localhost:8090";
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 360, height: 280 },
    });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("console", (message) => {
      if (
        message.type() === "error" &&
        /THREE.WebGLProgram/.test(message.text())
      )
        errors.push(message.text());
    });
    const waitForStart = (id, surface, speed, timeout = 60_000) =>
      page.waitForFunction(
        ({ id, surface, speed }) => {
          const drive = document.querySelector("#drive").dataset;
          return (
            document.querySelector("#places").value === id &&
            drive.contact === "grounded" &&
            (drive.surfaceId || "") === surface &&
            (speed === null ||
              (speed === "rolling"
                ? Number(document.querySelector("#speed").textContent) >= 45
                : document.querySelector("#speed").textContent === speed))
          );
        },
        { id, surface, speed },
        { timeout },
      );
    const bridge = "bay-bridge-highway-drive";
    await page.goto(`${host}/sf/?city=bay-bridge`);
    await waitForStart(bridge, "bay-bridge-westbound", "rolling");
    await page.locator("#menuBtn").click();
    const picker = page.getByLabel("Start from", { exact: true });
    assert.equal(await picker.locator(`option[value="${bridge}"]`).count(), 1);
    assert.equal(await page.locator("#bridgeLink, #cityDriveLink").count(), 0);
    await picker.selectOption("fremont-exit");
    await waitForStart("fremont-exit", "fremont-exit", null);
    assert.equal(new URL(page.url()).searchParams.get("city"), "bay-bridge");
    const intersection = await picker
      .locator(`option:not([value="${bridge}"]):not([value="fremont-exit"])`)
      .first()
      .getAttribute("value");
    await picker.selectOption(intersection);
    await waitForStart(intersection, "", "0");
    await page.locator("#menuBtn").click();
    await picker.selectOption(bridge);
    await waitForStart(bridge, "bay-bridge-westbound", "rolling");
    assert.deepEqual(errors, []);
    console.log(
      "The start picker loads the baked Bay Bridge drive at speed, Fremont exit and a stationary intersection.",
    );
    if (process.argv.includes("--eastbound")) {
      const start = "yerba-buena-eastbound";
      await page.goto(`${host}/sf/?at=${start}`);
      for (const surface of [
        "first-street-eastbound-entry",
        "sf-eastbound-lower-approach",
        "bay-bridge-eastbound-lower",
        "yerba-buena-lower-west-approach",
        "yerba-buena-lower-tunnel",
        "yerba-buena-lower-tunnel-east-of-exit",
        "yerba-buena-lower-east-approach",
        "yerba-buena-lower-east-after-entry",
        "sf-contact/bay-bridge-east-span-eastbound",
      ]) {
        await waitForStart(start, surface, null, 220_000);
        if (surface.startsWith("yerba-buena-lower-tunnel")) {
          const { altitude, cameraContact } = await page
            .locator("#drive")
            .evaluate(({ dataset }) => ({ ...dataset }));
          assert.ok(Number(altitude) < 51.3);
          assert.equal(cameraContact, "tunnel");
        }
      }
      await page.waitForFunction(() => {
        const { north, contact } = document.querySelector("#drive").dataset;
        return Number(north) > 6410 && contact === "grounded";
      });
      assert.deepEqual(errors, []);
      console.log(
        "The eastbound drive reached the East Span map boundary from First Street.",
      );
    }
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
