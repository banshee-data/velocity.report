// Run against the built site. Inject approach poses while production physics,
// billing, save/reload and the renderer handle everything after the approach.
const assert = require("node:assert/strict");
const { chromium, webkit, devices } = require("playwright");
const useWebKit = process.env.SF_WEBKIT === "1";
const { confirmManualTakeover } = require("./manual-takeover.cjs");

async function setup(browser, suffix) {
  const page = await browser.newPage({
    ...(useWebKit
      ? devices["iPhone 15"]
      : { viewport: { width: 800, height: 560 } }),
    serviceWorkers: "block",
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("crash", () => errors.push("Browser page crashed"));
  page.setDefaultTimeout(90000);
  await page.route("**/sf-drive.js", async (route) => {
    const response = await route.fetch();
    const source = await response.text();
    const hook = "    const crashSupport = crashRoadSupport(";
    assert.ok(source.includes(hook));
    await route.fulfill({
      response,
      body: source.replace(
        hook,
        `
    let observedTreeUp = 1;
    const watched = new WeakMap();
    function watchTree(entry, row) {
      if (!entry || watched.get(entry) === row) return;
      const draw = entry.grp.userData.drawTree;
      entry.grp.userData.drawTree = (index, event, seconds = 1) => {
        draw(index, event, seconds);
        if (index === row)
          observedTreeUp = event ? Math.cos(Math.PI * 0.48 * treeFallProgress(seconds)) : 1;
      };
      watched.set(entry, row);
    }
    window.contactFixture = {
      drop() {
        const floor = surface.heightAt(car.x, car.y);
        if (!Number.isFinite(floor)) throw Error("Missing highway support");
        car = {...car, z:floor + 8, v:19, vx:19*Math.cos(car.heading),
          vy:19*Math.sin(car.heading), vz:0, grounded:false, leavingRoad:true};
      },
      strike() {
        for (const [token, {tile}] of loaded) {
          for (const [index, [lx,ly,height]] of tile.tr.entries()) {
            if (height > 4.75 || height < 2.5) continue;
            const x=tile.o[0]+lx/10, y=tile.o[1]+ly/10, ground=terrainHeight(x,y);
            if (!Number.isFinite(ground) || ground < 1 ||
                Math.hypot(x-car.x,y-car.y) > 400) continue;
            for (const heading of [0,Math.PI/2,Math.PI,3*Math.PI/2]) {
              const pose = distance => {
                const wx=x-distance*Math.cos(heading), wy=y-distance*Math.sin(heading);
                return {x:wx,y:wy,z:terrainHeight(wx,wy),heading,v:22,grounded:true};
              };
              const distance=2.23 + Math.max(0.12, 0.04 * height);
              const prior=pose(distance+0.06), next=pose(distance-0.09);
              if (!Number.isFinite(prior.z) || !Number.isFinite(next.z) ||
                  Math.abs(prior.z-ground) > 0.12 || Math.abs(next.z-prior.z) > 0.07) continue;
              const hit=obstacleContact(indexes,drivingHull(next));
              if (hit?.tree?.tile !== token || hit.tree.index !== index ||
                  obstacleContact(indexes,drivingHull(prior))) continue;
              watchTree(loaded.get(token), index);
              car=prior;
              return {token,index};
            }
          }
        }
        throw Error("No clear approach to a thin baked tree found");
      },
      read() {
        const {clip,tick}=playback || {};
        return {x:car.x,y:car.y,v:car.v,grounded:car.grounded,leaving:car.leavingRoad,
          history:leadIn.read().length, limit:insurance.limit, multiplier:insurance.multiplier,
          spent:insurance.spent, elapsed:insurance.manualSeconds, clip,tick};
      },
      treeUp() { return observedTreeUp; },
      watchReplayTree() {
        const {clip,treeIndex} = playback || {};
        const event=clip?.treeDamage;
        if (event && treeIndex >= 0) watchTree(loaded.get(event.tile), treeIndex);
      },
    };
${hook}`,
      ),
    });
  });
  const origin = process.env.SF_BASE_URL || "http://localhost:8090";
  await page.goto(`${origin}/sf/${suffix}`);
  await page.waitForFunction(
    () => window.contactFixture?.read().history === 240,
  );
  return { page, errors };
}

(async () => {
  const browser = useWebKit
    ? await webkit.launch()
    : await chromium.launch({
        executablePath: process.env.CHROMIUM_PATH || undefined,
        args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
      });
  try {
    if (process.env.SF_TREE_ONLY !== "1") {
      const deck = await setup(browser, "");
      await confirmManualTakeover(deck.page);
      const start = await deck.page.evaluate(() => {
        window.contactFixture.drop();
        return window.contactFixture.read();
      });
      await deck.page.waitForFunction(() => {
        const car = window.contactFixture.read();
        return car.grounded && !car.leaving && !car.clip;
      });
      await deck.page.keyboard.down("ArrowUp");
      await deck.page.waitForFunction((before) => {
        const car = window.contactFixture.read();
        return (
          car.v > 19.15 &&
          car.limit === 50 &&
          car.multiplier === 1 &&
          Math.hypot(car.x - before.x, car.y - before.y) > 12
        );
      }, start);
      await deck.page.keyboard.up("ArrowUp");
      await deck.page.waitForFunction(() =>
        document
          .querySelector("#insuranceLimit")
          .textContent.startsWith("50 mph game limit"),
      );
      assert.equal(
        await deck.page
          .getByRole("button", { name: "Skip replay" })
          .isVisible(),
        false,
      );
      assert.deepEqual(deck.errors, []);
      await deck.page.close();
    }

    const city = await setup(browser, "?at=embarcadero");
    const { page } = city;
    await confirmManualTakeover(page);
    const target = await page.evaluate(() => window.contactFixture.strike());
    const skip = page.getByRole("button", { name: "Skip replay" });
    await skip.waitFor();
    const read = () => page.evaluate(() => window.contactFixture.read());
    async function readFallenTree() {
      const sample = await page.waitForFunction(() => {
        const fixture = window.contactFixture;
        fixture.watchReplayTree();
        const upright = fixture.treeUp();
        if (!Number.isFinite(upright) || upright >= 0.2) return false;
        const { elapsed, spent } = fixture.read();
        return { elapsed, spent };
      });
      return sample.jsonValue();
    }
    const crash = await read();
    assert.equal(crash.clip.kind, "tree");
    assert.equal(crash.clip.treeDamage.stage, "broken");
    assert.equal(crash.clip.treeDamage.tile, target.token);
    assert.equal(crash.clip.treeDamage.index, target.index);
    assert.ok(crash.clip.fee > 0 && crash.spent >= crash.clip.fee);
    assert.match(
      await page.locator("#replayLocation").textContent(),
      /cm trunk · tree fell/,
    );
    await page.waitForFunction(() => window.contactFixture.treeUp() > 0.99);
    const replaying = await readFallenTree();
    assert.equal(replaying.elapsed, crash.elapsed);
    assert.equal(replaying.spent, crash.spent);
    await page.keyboard.press("Escape");
    await skip.waitFor({ state: "hidden" });
    await page.reload();
    await page.waitForFunction(
      () => window.contactFixture?.read().history === 240,
    );
    await page.locator("#menuBtn").click();
    await page.getByText("Saved replays", { exact: true }).click();
    await page
      .locator("#crashReplays")
      .getByRole("button", { name: /^Play .*tree/i })
      .click();
    await skip.waitFor();
    assert.equal((await readFallenTree()).spent, 0);
    await page.keyboard.press("Escape");
    await skip.waitFor({ state: "hidden" });
    assert.deepEqual(city.errors, []);
    console.log(
      browser.browserType().name(),
      "verified recoverable highway touchdown, continued acceleration, baked tree falling in live and archived replay, and a one-time crash charge.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
