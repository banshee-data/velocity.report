// Drive the full westbound route using the game's automatic controller in Chrome.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

async function checkRoute() {
  const origin = process.env.SF_BASE_URL || "http://127.0.0.1:8090";
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 480, height: 320 },
    });
    const errors = [];
    const wrongOrigin = new Set();
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.includes("/city-tiles/") ||
        url.hostname.includes("arrow-worker")
      )
        wrongOrigin.add(url.href);
    });
    const start = "yerba-buena-westbound";
    await page.goto(`${origin}/sf/?at=${start}`);
    for (const [id, min, max] of [
      ["yerba-buena-east-approach", 55.9, 61.0],
      ["yerba-buena-upper-tunnel", 52.2, 56.0],
      ["yerba-buena-west-approach", 52.2, 55.1],
      ["bay-bridge-westbound", 54.9, 66.0],
      ["city-approach", 39.0, 57.0],
      ["fremont-exit", 7.0, 41.0],
      ["city-streets", 3.0, 8.0],
      ["fourth-street-entrance", 4.0, 12.0],
      ["westbound-i80", 10.0, 17.0],
    ]) {
      const tunnel = id === "yerba-buena-upper-tunnel";
      const handle = await page.waitForFunction(
        ({ id, tunnel }) => {
          const data = document.querySelector("#drive").dataset;
          if (
            data.surfaceId === id &&
            data.contact === "grounded" &&
            data.roadBlocked === "false" &&
            data.cameraContact === (tunnel ? "tunnel" : "exterior")
          )
            return { z: Number(data.altitude), speed: Number(data.speedMps) };
        },
        { id, tunnel },
        { timeout: 420000 },
      );
      const { z, speed } = await handle.jsonValue();
      await handle.dispose();
      assert.ok(z >= min && z <= max, `${id}: ${z} m`);
      assert.ok(speed > 0, id);
      if (id === "yerba-buena-east-approach") {
        assert.equal(await page.locator("#places").inputValue(), start);
        assert.ok(speed >= 45 * 0.44704, `Rolling start: ${speed} m/s`);
      }
      if (tunnel && process.env.SF_TUNNEL_SCREENSHOT)
        await page.screenshot({ path: process.env.SF_TUNNEL_SCREENSHOT });
      console.log(id, z, speed);
    }
    await page.waitForFunction(
      () => document.querySelector("#drive").dataset.routeComplete === "true",
      null,
      { timeout: 120000 },
    );
    assert.deepEqual(errors, []);
    assert.deepEqual([...wrongOrigin], []);
    console.log("Yerba Buena, Fremont and Fourth Street route completed");
  } finally {
    await browser.close();
  }
}

checkRoute().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
