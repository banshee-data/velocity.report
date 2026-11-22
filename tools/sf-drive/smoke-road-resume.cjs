// Run against the built site with Playwright on NODE_PATH. Only the starting
// poses are injected. Keyboard, contact, recovery and the HUD are the live game.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 720, height: 480 },
      serviceWorkers: "block",
    });
    page.setDefaultTimeout(90_000);
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("**/sf-drive.js", async (request) => {
      const response = await request.fetch();
      const source = await response.text();
      const hook = "    const landingBelow = (x, y, below) =>";
      assert.ok(source.includes(hook));
      const body = source.replace(
        hook,
        `
    window.streetFixture = {
      read: () => ({x: car.x, y: car.y, speed: car.v, surface: car.surfaceId,
        onGuide: cityDriveOnRoute(roadSurfaces.get(car.surfaceId), car),
        grounded: car.grounded, crash: !!crash, progress: insurance.delivered,
        spent: insurance.spent, checkpoint: routeCheckpoint?.car}),
      place(backwards) {
        const span = gameDrive.spans.get("survey-streets-outbound");
        const index = span.start + 24;
        const [x, y] = gameDrive.points[index];
        const [nextX, nextY] = gameDrive.points[index + 1];
        const heading = Math.atan2(nextY-y, nextX-x);
        const east = x + Math.sin(heading) * 2.8;
        const north = y - Math.cos(heading) * 2.8;
        car = {x: east, y: north, heading: heading + (backwards ? Math.PI : -0.35),
          z: terrainHeight(east, north), v: backwards ? 0 : 7, grounded: true, surfaceId: span.id};
        if (car.z === null) throw new Error("Fixture pavement is not loaded");
        camInit = false;
      },
    };
${hook}`,
      );
      await request.fulfill({ response, body });
    });
    const origin = process.env.SF_BASE_URL || "http://localhost:8090";
    await page.goto(`${origin}/sf/?city=survey&at=fremont-exit`);
    await page.waitForFunction(
      () =>
        document.querySelector("#drive").dataset.contact === "grounded" &&
        window.streetFixture?.read().checkpoint,
    );
    await confirmManualTakeover(page);
    const read = () => page.evaluate(() => window.streetFixture.read());
    const resume = async () => {
      await page.locator("#menuBtn").click();
      await page.getByRole("button", { name: "Resume self-driving" }).click();
      assert.equal(
        await page.locator("#insuranceMode").textContent(),
        "Self-driving",
      );
      return (await read()).spent;
    };
    const start = await page.evaluate(() => {
      window.streetFixture.place(false);
      return window.streetFixture.read();
    });
    await page.waitForFunction(({ x, y }) => {
      const car = window.streetFixture.read();
      return !car.onGuide && Math.hypot(car.x - x, car.y - y) > 3;
    }, start);
    assert.ok(
      (await read()).speed > 5,
      "guide edge does not stop a moving car on pavement",
    );
    await page.keyboard.down("Space");
    await page.waitForFunction(
      () => Math.abs(window.streetFixture.read().speed) < 0.03,
    );
    await page.keyboard.up("Space");
    const stopped = await read();
    assert.equal(stopped.onGuide, false);
    assert.equal(stopped.crash, false);
    const spendAtResume = await resume();
    await page.waitForFunction(() => {
      const car = window.streetFixture.read();
      return car.onGuide && car.speed > 2;
    });
    const resumed = await read();
    assert.equal(resumed.surface, stopped.surface);
    assert.equal(resumed.grounded, true);
    assert.ok(
      Math.hypot(resumed.x - stopped.x, resumed.y - stopped.y) < 25,
      "nearby rejoin stays on the same street",
    );
    assert.ok(spendAtResume >= stopped.spent);
    assert.equal(resumed.spent, spendAtResume);
    console.log(
      "Street: manual coast crosses the guide edge, brakes to rest, and auto rejoins the same street",
    );

    await confirmManualTakeover(page);
    const stranded = await page.evaluate(() => {
      window.streetFixture.place(true);
      return window.streetFixture.read();
    });
    assert.ok(stranded.checkpoint);
    const spendAtRecovery = await resume();
    await page
      .getByText("Self-driving resumed from the last route checkpoint", {
        exact: true,
      })
      .first()
      .waitFor();
    await page.waitForFunction(({ x, y }) => {
      const car = window.streetFixture.read();
      return (
        car.grounded && car.speed > 2 && Math.hypot(car.x - x, car.y - y) > 1
      );
    }, stranded.checkpoint);
    const recovered = await read();
    assert.equal(recovered.progress, stranded.progress);
    assert.ok(spendAtRecovery >= stranded.spent);
    assert.equal(recovered.spent, spendAtRecovery);
    assert.deepEqual(errors, []);
    console.log(
      "Stranded car: self-driving announces checkpoint recovery and moves with progress preserved",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
