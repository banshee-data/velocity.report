// Run against the built site. Inject approach poses while production physics,
// billing, save/reload and the renderer handle everything after the approach.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

async function setup(browser, suffix) {
  const page = await browser.newPage({
    viewport: { width: 800, height: 560 },
    serviceWorkers: "block",
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.setDefaultTimeout(90000);
  await page.route("**/sf-drive.js", async (route) => {
    const response = await route.fetch();
    const source = await response.text();
    const hook =
      "    const crashSupport = crashRoadSupport(terrainHeight, roadSurfaces);";
    assert.ok(source.includes(hook));
    await route.fulfill({
      response,
      body: source.replace(
        hook,
        `
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
              const distance=2.23 + treeRadius(height);
              const prior=pose(distance+0.06), next=pose(distance-0.09);
              if (!Number.isFinite(prior.z) || !Number.isFinite(next.z) ||
                  Math.abs(prior.z-ground) > 0.12 || Math.abs(next.z-prior.z) > 0.07) continue;
              const hit=obstacleContact(indexes,drivingHull(next));
              if (hit?.tree?.tile !== token || hit.tree.index !== index ||
                  obstacleContact(indexes,drivingHull(prior))) continue;
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
      treeUp() {
        const event=playback?.clip.treeDamage;
        const trunk=loaded.get(event?.tile)?.grp.children.find(
          mesh=>mesh.isInstancedMesh && mesh.geometry.type==="CylinderGeometry");
        if (!trunk) return null;
        const m=new THREE.Matrix4();
        trunk.getMatrixAt(event.index,m);
        return m.elements[5]/Math.hypot(...m.elements.slice(4,7));
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
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
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
    assert.match(
      await deck.page.locator("#insuranceLimit").textContent(),
      /^50 mph game limit/,
    );
    assert.equal(
      await deck.page.getByRole("button", { name: "Skip replay" }).isVisible(),
      false,
    );
    assert.deepEqual(deck.errors, []);
    await deck.page.close();

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
    await page.getByRole("button", { name: "Map and game settings" }).click();
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
      "Chromium verified recoverable highway touchdown, continued acceleration, baked tree falling in live and archived replay, and a one-time crash charge.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
