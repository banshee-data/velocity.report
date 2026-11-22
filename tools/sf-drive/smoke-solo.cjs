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
      const url = `${process.env.SF_BASE_URL || "http://127.0.0.1:8090"}/sf/`;
      await page.goto(`${url}?mode=solo`);
      const storage = await page.evaluate(() => ({ ...localStorage }));
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
        await page.locator("#respawnBtn").click();
        await page.locator("#viewMode").press("End");
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
        const afterLanding = await page.evaluate(() => ({ ...localStorage }));
        const {
          "sf-best-airtimes-v1": flights,
          "sf-replays-v1": savedReplays,
          ...otherRecords
        } = afterLanding;
        const [airtime] = JSON.parse(flights);
        const [clip] = JSON.parse(savedReplays).airtimes;
        assert.equal(clip.mode, "practice");
        assert.ok(clip.frames.length < clip.end / 2);
        assert.ok(
          airtime.seconds > 0 && airtime.peak > 0,
          "practice crest records a completed flight",
        );
        assert.deepEqual(
          otherRecords,
          storage,
          "solo leaves trial records and checkpoints intact",
        );
        console.log("desktop:", await page.locator("#soloSummary").innerText());
        await page.locator("#respawnBtn").click();
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
        await page.goto(`${url}?at=embarcadero`);
        await page.waitForFunction(() =>
          document.querySelector("#progress").textContent.includes("emojis"),
        );
        assert.deepEqual(
          await page.evaluate(() => ({ ...localStorage })),
          afterLanding,
        );
        assert.equal(await page.locator(".seeker").isVisible(), true);
        await page.locator("#menuBtn").click();
        await page.getByText("Saved replays", { exact: true }).click();
        const replay = page.locator("#airtimeReplays a");
        const href = await replay.getAttribute("href");
        assert.equal(await replay.getAttribute("target"), "_blank");
        await page.goto(new URL(href, page.url()).href);
        await page.locator('#drive[data-replay="airtime"]').waitFor();
        assert.match(
          await page.locator("#replayLocation").textContent(),
          /🏁 Practice finish · [\d.]+ m E/,
        );
        assert.equal(await page.locator("#replayRate").textContent(), "0.99×");
        assert.equal(
          await page
            .getByRole("img", { name: /Vehicle deceleration/ })
            .isVisible(),
          true,
        );
        await page.getByRole("button", { name: "Skip replay" }).click();
        await page.locator('#drive[data-solo-status="ready"]').waitFor();
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
