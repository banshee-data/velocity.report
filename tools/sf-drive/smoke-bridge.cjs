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
    const waitForStart = (id, surface, speed) =>
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
        { timeout: 60000 },
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
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
