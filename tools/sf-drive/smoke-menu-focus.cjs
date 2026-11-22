// Build and serve public_html/_site, then run with Playwright on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  const url = `${process.env.SF_BASE_URL || "http://localhost:8090"}/sf/`;
  try {
    for (const mobile of [false, true]) {
      const page = await browser.newPage({
        viewport: mobile
          ? { width: 390, height: 690 }
          : { width: 1000, height: 700 },
        hasTouch: mobile,
        isMobile: mobile,
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.goto(`${url}?at=embarcadero`);
      const emoji = page.locator("#emojiFocus");
      await emoji.waitFor({ timeout: 90000 });
      await page.locator("#menuBtn").click();
      const menu = page.locator("#gameMenu");
      assert.equal(
        await menu
          .locator(".tools")
          .first()
          .locator("input,select")
          .first()
          .getAttribute("id"),
        "viewMode",
      );
      assert.equal(await menu.locator("#mapSources").count(), 0);
      assert.equal(await emoji.textContent(), "");
      assert.equal(
        await menu
          .locator("#gameMode, #trialArea, #restartBtn:visible")
          .count(),
        0,
      );
      const slider = menu.getByRole("slider", { name: /^Camera/ });
      await slider.press("ArrowRight");
      assert.equal(await slider.getAttribute("aria-valuetext"), "Overhead");
      await slider.press("End");
      assert.equal(await slider.getAttribute("aria-valuetext"), "In car");
      await slider.press("Home");
      await menu.getByRole("button", { name: "Close settings" }).click();

      await confirmManualTakeover(page);
      const position = () =>
        page
          .locator("#drive")
          .evaluate((el) => [el.dataset.east, el.dataset.north]);
      const before = await position();
      if (mobile) await emoji.tap();
      else await emoji.press("Enter");
      assert.equal(await emoji.getAttribute("aria-pressed"), "true");
      await page.waitForFunction(() => {
        const r = document.querySelector("#emojiFocus").getBoundingClientRect();
        return (
          Math.abs(r.x + r.width / 2 - innerWidth / 2) < 1 &&
          Math.abs(r.y + r.height / 2 - innerHeight / 2) < 1
        );
      });
      assert.deepEqual(await position(), before);
      if (mobile) {
        const cdp = await page.context().newCDPSession(page);
        const r = await page
          .getByRole("button", { name: "Accelerate" })
          .boundingBox();
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [{ x: r.x + r.width / 2, y: r.y + r.height / 2 }],
        });
        await page.waitForFunction(
          () => Number(document.querySelector("#speed").textContent) > 0,
        );
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchEnd",
          touchPoints: [],
        });
      } else {
        await page.keyboard.down("ArrowUp");
        await page.waitForFunction(
          () => Number(document.querySelector("#speed").textContent) > 0,
        );
        await page.keyboard.up("ArrowUp");
      }
      assert.equal(await emoji.getAttribute("aria-pressed"), "false");
      await page.keyboard.press("r");
      assert.equal(await emoji.getAttribute("aria-pressed"), "false");
      await page.goto(`${url}?city=bay-bridge`);
      await page
        .locator('#drive[data-surface-id="bay-bridge-westbound"]')
        .waitFor();
      assert.equal(await emoji.isVisible(), false);
      await page.goto(`${url}?mode=solo`);
      await page.waitForFunction(
        () => document.querySelector("#drive").dataset.soloStatus === "ready",
      );
      assert.equal(await emoji.isVisible(), false);
      assert.deepEqual(errors, []);
      console.log(
        `${mobile ? "390×690 touch" : "Desktop keyboard"}: menu order, camera slider, emoji centring, drive release, reset and solo route passed`,
      );
      await page.close();
    }
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
