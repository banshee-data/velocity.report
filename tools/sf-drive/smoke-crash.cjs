// Run against the built site. Inject only the approach pose to a baked rail.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

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
        serviceWorkers: "block",
      });
      page.setDefaultTimeout(60000);
      const errors = [];
      page.on("pageerror", (err) => errors.push(err.message));
      await page.route("**/sf-drive.js", async (route) => {
        const response = await route.fetch();
        const hook =
          "    const crashSupport = crashRoadSupport(terrainHeight, roadSurfaces);";
        const source = await response.text();
        assert.ok(source.includes(hook));
        await route.fulfill({
          response,
          body: source.replace(
            hook,
            `
    window.crashFixture = {
      strike() {
        const {a,b,nx,ny}=surface.railSegments[10];
        const x=(a[0]+b[0])/2-3*nx,y=(a[1]+b[1])/2-3*ny;
        car={x,y,z:surface.heightAt(x,y),heading:Math.atan2(0.97*ny+0.24*nx,0.97*nx-0.24*ny),
          surfaceId:surface.id,grounded:true,v:22};
      },
      read:()=>({kind:crash?.kind,inputs:[...state.keys],touch:state.touch,inf:insurance.infractions,
        mesh:carMesh.quaternion.toArray(), steering:car.steering, tick:playback?.tick,
        clip:playback?.clip, history:leadIn.read().length}),
    };
${hook}`,
          ),
        });
      });
      await page.goto(
        (process.env.SF_BASE_URL || "http://localhost:8090") + "/sf/",
      );
      await page.waitForFunction(
        () => window.crashFixture?.read().history === 240,
      );
      await confirmManualTakeover(page);
      await page.evaluate(() => window.crashFixture.strike());
      const skip = page.getByRole("button", { name: "Skip replay" });
      await skip.waitFor();
      const read = () => page.evaluate(() => window.crashFixture.read());
      const active = await read();
      assert.equal(active.kind, "highway barrier");
      assert.equal(active.clip.impact, 240);
      assert.equal(active.clip.outcome, "stopped");
      assert.equal(active.clip.end - active.clip.frames.at(-1)[0], 240);
      assert.equal(active.inf, 1);
      assert.equal(active.steering, 0);
      assert.ok(
        active.clip.telemetry.some(([, decel, jerk]) => decel > 0 && jerk > 0),
      );
      assert.match(
        await page.locator("#replayLocation").textContent(),
        /\d+(?:\.\d+)? (?:km|m) [NSEW]{1,2}$/,
      );
      assert.equal(
        await page.getByRole("img", { name: /Vehicle jerk/ }).isVisible(),
        true,
      );
      assert.equal(await page.locator("#replayRate").textContent(), "0.99×");
      if (mobile) {
        await skip.tap();
      } else {
        await page.keyboard.down("ArrowLeft");
        await page.keyboard.down("ArrowUp");
        await page.keyboard.press("KeyR");
        await page.getByText("0.5×", { exact: true }).waitFor();
        const graphs = page.locator("#replayGraphs summary");
        await graphs.press("Space");
        assert.equal(
          await page.locator("#replayGraphs").getAttribute("open"),
          null,
        );
        await graphs.press("Space");
        assert.notEqual(
          await page.locator("#replayGraphs").getAttribute("open"),
          null,
        );
        await page.waitForFunction(() => {
          const { tick, clip, mesh } = window.crashFixture.read();
          return (
            clip &&
            tick > clip.impact &&
            Math.abs(mesh[0]) + Math.abs(mesh[2]) > 0.005
          );
        });
        await page.getByText("Car stopped", { exact: true }).waitFor();
        await skip.waitFor({ state: "hidden" });
        await page.keyboard.up("ArrowLeft");
        await page.keyboard.up("ArrowUp");
      }
      await page.waitForFunction(() => !window.crashFixture.read().kind);
      const end = await read();
      assert.equal(end.inf, 1);
      assert.deepEqual(end.inputs, []);
      assert.equal(end.touch.throttle, 0);
      const lastSaved = () =>
        page.evaluate(
          () => JSON.parse(localStorage.getItem("sf-replays-v1")).crashes[0],
        );
      assert.deepEqual(await lastSaved(), active.clip);

      await page.reload();
      await page.locator('#drive[data-contact="grounded"]').waitFor();
      await page.getByRole("button", { name: "Map and game settings" }).click();
      await page.getByText("Saved replays", { exact: true }).click();
      await page.getByRole("button", { name: /Play highway barrier/ }).click();
      await page.locator('#drive[data-replay="crash"]').waitFor();
      const archived = await read();
      assert.equal(
        archived.inf,
        0,
        "viewing a replay does not count as another crash",
      );
      assert.deepEqual(archived.clip, active.clip);
      if (mobile) await skip.tap();
      else await page.keyboard.press("Escape");
      await skip.waitFor({ state: "hidden" });
      assert.deepEqual(errors, []);
      await page.close();
    }
    console.log(
      "Desktop played the lead-in, impact and full stop hold. Mobile skipped and retained the clip. Both replayed the saved crash after reload.",
    );
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
