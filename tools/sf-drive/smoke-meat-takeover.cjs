// Run against the built site with Playwright available on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    for (const mobile of [false, true]) {
      const page = await browser.newPage({
        viewport: mobile
          ? { width: 390, height: 690 }
          : { width: 800, height: 560 },
        isMobile: mobile,
        hasTouch: mobile,
      });
      page.setDefaultTimeout(75_000);
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      const origin = process.env.SF_BASE_URL || "http://localhost:8090";
      await page.goto(`${origin}/sf/`);
      const snapshot = () =>
        page.locator("#drive").evaluate(({ dataset: d }) => ({
          time: performance.now(),
          v: Number(d.speedMps),
          x: Number(d.east),
          y: Number(d.north),
          heading: Number(d.heading),
          contact: d.contact,
          surface: d.surfaceId,
        }));
      await page.waitForFunction(() => {
        const d = document.querySelector("#drive").dataset;
        return (
          d.surfaceId === "bay-bridge-westbound" &&
          d.contact === "grounded" &&
          Number(d.speedMps) >= 22
        );
      });
      assert.match(
        await page.locator("#insuranceDeliveries").textContent(),
        /^0\/3/,
      );
      const dialog = page.getByRole("dialog", { name: "Switch to Meat mode?" });
      const confirm = dialog.getByRole("button", {
        name: "Take manual control",
      });
      const cancel = dialog.getByRole("button", { name: "Cancel" });
      const cdp = mobile ? await page.context().newCDPSession(page) : null;
      const pedal = page.getByRole("button", {
        name: "Brake or hold to reverse",
      });
      const startBrake = async () => {
        if (!mobile) return page.keyboard.down("Space");
        const r = await pedal.boundingBox();
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [{ x: r.x + r.width / 2, y: r.y + r.height / 2, id: 0 }],
        });
      };
      const stopBrake = () =>
        mobile
          ? cdp.send("Input.dispatchTouchEvent", {
              type: "touchEnd",
              touchPoints: [],
            })
          : page.keyboard.up("Space");
      for (const takeControl of [false, true]) {
        await startBrake();
        await dialog.waitFor();
        const before = await snapshot();
        assert.ok(before.v >= 20 && before.contact === "grounded");
        assert.match(
          await page.locator("#insuranceRate").textContent(),
          /Meter off/,
        );
        assert.match(
          await page.locator("#insuranceLimit").textContent(),
          /^50 mph game limit · (?:1|2|4|8)×$/,
        );
        assert.equal(await page.locator("#insuranceMeter").isVisible(), false);
        assert.equal(await page.locator("#selfDrivingIcon").isVisible(), true);
        const action = takeControl ? confirm : cancel;
        if (mobile) {
          // The modal clears the pedal even before the opening finger lifts.
          assert.equal(await pedal.getAttribute("aria-pressed"), "false");
          await stopBrake();
          await action.tap();
        } else {
          await action.press("Enter");
          // A browser repeat from the still-held opening key must not brake.
          await page.keyboard.down("Space");
        }
        await dialog.waitFor({ state: "hidden" });
        await page.waitForFunction(({ x, y }) => {
          const d = document.querySelector("#drive").dataset;
          return Math.hypot(Number(d.east) - x, Number(d.north) - y) > 1;
        }, before);
        const after = await snapshot();
        assert.equal(after.contact, "grounded");
        assert.equal(after.surface, before.surface);
        const dx = after.x - before.x;
        const dy = after.y - before.y;
        assert.ok(
          dx * Math.cos(before.heading) + dy * Math.sin(before.heading) > 0,
          "keeps moving forward",
        );
        assert.ok(
          Math.hypot(dx, dy) < 65 * ((after.time - before.time) / 1000 + 0.5),
          "does not jump along the route",
        );
        assert.ok(
          Math.abs(after.heading - before.heading) < 0.08,
          JSON.stringify({ before, after }),
        );
        assert.ok(after.v >= before.v - 1.5, JSON.stringify({ before, after }));
        if (!mobile) await stopBrake();
        if (!takeControl) {
          assert.equal(
            await page.locator("#insuranceMode").textContent(),
            "Self-driving",
          );
          assert.equal(
            await page.locator("#insuranceCost").textContent(),
            "$0.0000",
          );
        }
      }
      await page.waitForFunction(
        () =>
          document
            .querySelector("#insuranceRate")
            .textContent.match(/^0\.00[1248]0¢\/s/) &&
          document.querySelector("#insuranceCost").textContent !== "$0.0000",
      );
      assert.match(
        await page.locator("#insuranceLimit").textContent(),
        /^50 mph game limit · (?:1|2|4|8)×$/,
      );
      assert.equal(await page.locator("#insuranceMeter").isVisible(), true);
      assert.equal(await page.locator("#selfDrivingIcon").isVisible(), false);
      assert.equal(
        await page.locator("#insuranceHud").getAttribute("data-mode"),
        "manual",
      );
      const meter = await page.locator("#insuranceHud").boundingBox();
      assert.ok(
        Math.abs(
          meter.x +
            meter.width / 2 -
            (await page.evaluate(() => innerWidth)) / 2,
        ) < 2,
      );
      if (mobile) {
        const pads = await page
          .locator(".pad")
          .evaluateAll((pads) =>
            pads.map((x) => x.getBoundingClientRect().toJSON()),
          );
        assert.ok(
          pads.every(
            (p) =>
              p.right <= meter.x ||
              p.left >= meter.x + meter.width ||
              p.top >= meter.y + meter.height,
          ),
          JSON.stringify({ meter, pads }),
        );
      }
      const coast = await snapshot();
      await startBrake();
      await page.waitForFunction(
        (v) =>
          Number(document.querySelector("#drive").dataset.speedMps) < v - 2,
        coast.v,
      );
      await stopBrake();
      assert.deepEqual(errors, []);
      console.log(
        `${mobile ? "Touch" : "Keyboard"}: bridge start, cancel, confirm, retained contact and speed, fresh brake, manual meter and authored highway limit`,
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
