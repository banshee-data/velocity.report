// Run against the built site. Set only the approach pose at a baked rail;
// use the production contact, controls, render loop, HUD and recovery.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    const runs = [];
    for (const mobile of [false, true]) {
      const page = await browser.newPage({
        viewport: mobile
          ? { width: 390, height: 690 }
          : { width: 800, height: 560 },
        isMobile: mobile,
        hasTouch: mobile,
        serviceWorkers: "block",
      });
      page.setDefaultTimeout(60000);
      const errors = [];
      page.on("pageerror", (err) => errors.push(err.message));
      await page.route("**/sf-drive.js", async (route) => {
        const response = await route.fetch();
        const hook = "    const landingBelow = (x, y, below) =>";
        const trace = "        stepCrash(";
        const source = await response.text();
        assert.ok(source.includes(hook) && source.includes(trace));
        const body = source
          .replace(
            hook,
            `
    window.crashFixture = {
      strike() {
        const rail=surface.railSegments[10];
        const {a,b,nx,ny}=rail;
        const x=(a[0]+b[0])/2-3*nx,y=(a[1]+b[1])/2-3*ny;
        car={x,y,z:surface.heightAt(x,y),heading:Math.atan2(0.8*ny+0.6*nx,0.8*nx-0.6*ny),
          surfaceId:surface.id,grounded:true,v:32};
        window.crashTrace=[];
      },
      read:()=>({kind:crash?.kind,inputs:[...state.keys],touch:state.touch,inf:insurance.infractions,
        mesh:carMesh.quaternion.toArray(), steering:car.steering}),
    };
${hook}`,
          )
          .replace(
            trace,
            `
        if (crash.ticks % 30 === 0) window.crashTrace.push({
          q:crash.rotation.toArray(),p:crash.position.toArray(),v:crash.velocity.toArray()
        });
${trace}`,
          );
        await route.fulfill({ response, body });
      });
      await page.goto(
        (process.env.SF_BASE_URL || "http://localhost:8090") + "/sf/",
      );
      await page.waitForFunction(
        () =>
          window.crashFixture &&
          document.querySelector("#drive").dataset.contact === "grounded",
      );
      await confirmManualTakeover(page);
      await page.evaluate(() => window.crashFixture.strike());
      await page.waitForFunction(
        () => window.crashFixture.read().kind === "highway barrier",
      );
      let cdp;
      if (mobile) {
        cdp = await page.context().newCDPSession(page);
        const pedal = page.getByRole("button", { name: "Accelerate" }),
          r = await pedal.boundingBox();
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [{ x: r.x + r.width / 2, y: r.y + r.height / 2 }],
        });
      } else {
        await page.keyboard.down("ArrowLeft");
        await page.keyboard.down("ArrowUp");
        await page.keyboard.press("KeyR");
      }
      await page.waitForFunction(() => window.crashTrace.length >= 3);
      assert.equal(await page.locator("#respawnBtn").isDisabled(), true);
      const active = await page.evaluate(() => window.crashFixture.read());
      assert.equal(active.inf, 1);
      assert.equal(active.steering, 0);
      assert.ok(
        Math.abs(active.mesh[0]) + Math.abs(active.mesh[2]) > 0.005,
        "rendered chassis tips",
      );
      await page.waitForFunction(
        () =>
          window.crashTrace.length === 8 &&
          !window.crashFixture.read().kind &&
          document.querySelector("#drive").dataset.contact === "grounded",
      );
      const end = await page.evaluate(() => window.crashFixture.read());
      assert.equal(end.inf, 1);
      assert.deepEqual(end.inputs, []);
      assert.equal(end.touch.throttle, 0);
      assert.equal(await page.locator("#respawnBtn").isEnabled(), true);
      const trace = await page.evaluate(() => window.crashTrace);
      assert.ok(
        Math.hypot(...trace.at(-1).p.map((v, i) => v - trace[0].p[i])) > 1,
      );
      assert.deepEqual(errors, []);
      runs.push(trace);
      if (cdp)
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchEnd",
          touchPoints: [],
        });
      else {
        await page.keyboard.up("ArrowLeft");
        await page.keyboard.up("ArrowUp");
      }
      await page.close();
    }
    assert.deepEqual(
      runs[0],
      runs[1],
      "held keyboard and touch inputs cannot steer or accelerate the wreck",
    );
    console.log(
      "Desktop and touch Chromium: identical eight-sample 3-D rail crash, one infraction, locked controls and clean recovery.",
    );
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
