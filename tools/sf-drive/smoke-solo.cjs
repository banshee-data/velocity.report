// Manual browser regression: build-docs, serve public_html/_site, then run with
// Playwright available via NODE_PATH (or installed locally). Uses a local Chromium.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    headless: true,
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    for (const mobile of [false, true]) {
      const context = await browser.newContext({
        viewport: mobile
          ? { width: 390, height: 690 }
          : { width: 1280, height: 800 },
        isMobile: mobile,
        hasTouch: mobile,
        deviceScaleFactor: 1,
      });
      const page = await context.newPage();
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.goto(
        `${process.env.SF_BASE_URL || "http://127.0.0.1:8090"}/sf/`,
      );
      await page.waitForFunction(
        () => document.querySelector("#trialArea").options.length > 0,
      );
      const storage = await page.evaluate(() => JSON.stringify(localStorage));
      await page.locator("#menuBtn").click();
      await page.locator("#gameMode").selectOption("solo");
      await page.locator('[aria-label="Close settings"]').click();
      await page.waitForFunction(
        () => document.querySelector("#drive").dataset.soloStatus === "ready",
      );
      assert.equal(await page.locator(".seeker").isVisible(), false);
      if (mobile) {
        const ratio = await page.evaluate(
          () =>
            ["#driveBar", ".steering", ".pedals"].reduce((sum, selector) => {
              const r = document
                .querySelector(selector)
                .getBoundingClientRect();
              if (r.left < 0 || r.right > innerWidth || r.bottom > innerHeight)
                throw new Error(`out of bounds: ${selector}`);
              return sum + r.width * r.height;
            }, 0) /
            (innerWidth * innerHeight),
        );
        assert.ok(ratio < 0.25, `mobile UI uses ${ratio}`);
        // Actual touch start/end through Chromium's input protocol.
        const cdp = await context.newCDPSession(page);
        const box = await page.locator('[data-act="up"]').boundingBox();
        const touch = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [touch],
        });
        await page.waitForFunction(
          () => Number(document.querySelector("#speed").textContent) > 0,
        );
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchEnd",
          touchPoints: [],
        });
        await page.locator("#menuBtn").click();
        assert.equal(
          await page.locator('[data-act="up"]').getAttribute("aria-pressed"),
          "false",
        );
        await page.locator("#restartBtn").click();
        await page.locator("#viewMode").selectOption("incar");
        await page.locator('[aria-label="Close settings"]').click();
        await page.waitForFunction(
          () => !document.querySelector("#mirrorFrame").hidden,
        );
        const mirrorRatio = await page.evaluate(
          () =>
            ["#driveBar", ".steering", ".pedals", "#mirrorFrame"].reduce(
              (sum, s) => {
                const r = document.querySelector(s).getBoundingClientRect();
                return sum + r.width * r.height;
              },
              0,
            ) /
            (innerWidth * innerHeight),
        );
        assert.ok(mirrorRatio < 0.25);
        console.log(
          `mobile: touch, input release, restart, in-car mirror; HUD ${(mirrorRatio * 100).toFixed(1)}%`,
        );
      } else {
        await page.keyboard.down("ArrowUp");
        await page.waitForFunction(
          () =>
            document.querySelector("#drive").dataset.soloStatus === "finished",
          {},
          { timeout: 90000 },
        );
        await page.keyboard.up("ArrowUp");
        assert.match(
          await page.locator("#soloSummary").innerText(),
          /finished.*unknown 1.2 s.*No weighted score/,
        );
        assert.equal(
          await page.evaluate(() => JSON.stringify(localStorage)),
          storage,
        );
        console.log("desktop:", await page.locator("#soloSummary").innerText());
        await page.locator("#restartBtn").click();
        await page.locator('[aria-label="Close settings"]').click();
        await page.keyboard.down("ArrowUp");
        await page.waitForFunction(
          () =>
            document.querySelector("#drive").dataset.soloStatus === "running",
        );
        await page.keyboard.up("ArrowUp");
        await page.keyboard.press("r");
        await page.waitForFunction(
          () => document.querySelector("#drive").dataset.soloStatus === "ready",
        );
        await page.locator("#menuBtn").click();
        await page.locator("#gameMode").selectOption("city");
        await page.locator('[aria-label="Close settings"]').click();
        await page.waitForFunction(() =>
          document.querySelector("#progress").textContent.includes("emojis"),
        );
        assert.equal(
          await page.evaluate(() => JSON.stringify(localStorage)),
          storage,
        );
        assert.equal(await page.locator(".seeker").isVisible(), true);
      }
      assert.deepEqual(errors, []);
      await context.close();
    }
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
