// Run with Playwright available via NODE_PATH and Chromium on the Hydra dev host.
const assert = require("node:assert/strict");
const { gunzipSync } = require("node:zlib");
const { chromium } = require("playwright");

const host = process.env.SF_BASE_URL || "http://localhost:8090";
const state = () => {
  const { dataset } = document.querySelector("#drive");
  return {
    id: dataset.surfaceId,
    kind: dataset.surface,
    waiting: dataset.roadWait === "true",
    contact: dataset.contact,
    x: Number(dataset.east),
    y: Number(dataset.north),
    z: Number(dataset.altitude),
    status: document.querySelector("#next").textContent,
  };
};

const accelerationReady = () =>
  Number(document.querySelector("#speed").textContent) >= 10;

async function visit(page, region, review) {
  const params = new URLSearchParams({ region });
  if (review) params.set("review", review);
  await page.goto(`${host}/sf/?${params}`);
  try {
    await page.waitForFunction(
      () => document.querySelector("#drive").dataset.contact === "grounded",
      null,
      { timeout: 60000 },
    );
  } catch (error) {
    throw new Error(
      `${region}: ${JSON.stringify(await page.evaluate(state))}`,
      { cause: error },
    );
  }
  const car = await page.evaluate(state);
  assert.ok(car.id, JSON.stringify({ region, review, car }));
  return car;
}

async function delayedJoin(page, { region, join, token, from, to }) {
  const path = `**/geometry/${token}.json`;
  let release;
  const hold = new Promise((resolve) => {
    release = resolve;
  });
  let requested = false;
  await page.route(path, async (route) => {
    requested = true;
    await hold;
    await route.continue();
  });
  await page.keyboard.up("ArrowUp");
  try {
    const start = await visit(page, region, join);
    assert.equal(start.id, from);
    assert.equal(requested, false);
    await page.keyboard.down("ArrowUp");
    await page.waitForFunction(
      () => document.querySelector("#drive").dataset.roadWait === "true",
      null,
      { timeout: 90000 },
    );
    assert.ok(requested);
    const stopped = await page.evaluate(state);
    assert.equal(stopped.id, from);
    assert.equal(stopped.contact, "grounded");
    assert.match(stopped.status, /Waiting for the road/);
    assert.ok(Math.hypot(stopped.x - start.x, stopped.y - start.y) > 10);
    await page.waitForTimeout(650);
    const held = await page.evaluate(state);
    assert.equal(held.id, from);
    assert.equal(held.contact, "grounded");
    assert.ok(Math.hypot(held.x - stopped.x, held.y - stopped.y) < 0.15);
    release();
    await page.waitForFunction(
      (id) => document.querySelector("#drive").dataset.surfaceId === id,
      to,
      { timeout: 60000 },
    );
    const crossed = await page.evaluate(state);
    assert.equal(crossed.kind, start.kind);
    assert.equal(crossed.contact, "grounded");
    assert.ok(Math.abs(crossed.z - stopped.z) < 0.5);
    console.log(
      `${region}: held at ${stopped.z} m and resumed on ${to} at ${crossed.z} m`,
    );
  } catch (error) {
    throw new Error(
      `${region}: ${JSON.stringify(await page.evaluate(state))}`,
      { cause: error },
    );
  } finally {
    release();
    await page.keyboard.up("ArrowUp");
    await page.unroute(path);
  }
}

async function checkHighwayEdges(page) {
  for (const steer of ["ArrowLeft", "ArrowRight"]) {
    const start = await visit(page, "embarcadero-bryant", "bay-bridge-deck");
    await page.keyboard.down("ArrowUp");
    try {
      await page.waitForFunction(accelerationReady, null, { timeout: 45000 });
      await page.keyboard.down(steer);
      await page.waitForFunction(
        () =>
          /Bounced off|Scraping highway barrier/.test(
            document.querySelector("#next").textContent,
          ),
        null,
        { timeout: 60000 },
      );
      const onDeck = await page.evaluate(state);
      assert.equal(onDeck.id, start.id);
      assert.equal(onDeck.contact, "grounded");
      assert.equal(
        await page.locator("#driveBar").getAttribute("data-crash"),
        "false",
      );
      console.log(`Bay Bridge ${steer}: ${onDeck.status} at ${onDeck.z} m`);
    } finally {
      await page.keyboard.up(steer);
      await page.keyboard.up("ArrowUp");
    }
  }

  const deck = await visit(page, "embarcadero-bryant", "bay-bridge-deck");
  await page.keyboard.down("ArrowUp");
  try {
    await page.waitForFunction(
      ({ x, y }) => {
        const { east, north } = document.querySelector("#drive").dataset;
        return Math.hypot(Number(east) - x, Number(north) - y) >= 55;
      },
      deck,
      { timeout: 90000 },
    );
    await page.keyboard.down("ArrowLeft");
    await page.waitForFunction(
      () =>
        document.querySelector("#driveBar").dataset.crash === "true" &&
        document
          .querySelector("#status")
          .textContent.includes("highway barrier"),
      null,
      { timeout: 45000 },
    );
    console.log("Bay Bridge: higher-speed strike triggered crash recovery");
  } finally {
    await page.keyboard.up("ArrowLeft");
    await page.keyboard.up("ArrowUp");
  }

  const { groundAtWorld } =
    await import("../../public_html/src/js/sf-drive-core.js");
  const reply = await page.request.get(`${host}/sf/tiles/808f7fd4.json.gz`);
  assert.ok(reply.ok());
  const bytes = await reply.body();
  const pavement = JSON.parse(
    bytes[0] === 0x1f && bytes[1] === 0x8b ? gunzipSync(bytes) : bytes,
  );
  for (const steer of ["ArrowLeft", "ArrowRight"]) {
    const openDeck = await visit(
      page,
      "6th-brannan",
      "gms:40FXJKCA78QRCR5YNVX5TV8TVN",
    );
    await page.keyboard.down("ArrowUp");
    try {
      await page.waitForFunction(accelerationReady, null, { timeout: 45000 });
      await page.keyboard.down(steer);
      await page.waitForFunction(
        () => document.querySelector("#drive").dataset.contact === "airborne",
        null,
        { timeout: 60000 },
      );
      const airborne = await page.evaluate(state);
      assert.equal(airborne.id, openDeck.id);
      await page.keyboard.up(steer);
      await page.waitForFunction(
        () => {
          const { contact, surface } = document.querySelector("#drive").dataset;
          return contact === "grounded" && surface === "city";
        },
        null,
        { timeout: 45000 },
      );
      const landing = await page.evaluate(state);
      const terrain = groundAtWorld([pavement], landing.x, landing.y, true);
      assert.ok(airborne.z - landing.z > 0.5);
      assert.ok(terrain !== null && Math.abs(terrain - landing.z) < 0.02);
      assert.equal(
        await page.locator("#driveBar").getAttribute("data-crash"),
        "false",
      );
      console.log(
        `6th–Brannan ${steer}: fell from ${airborne.z} m and landed on terrain at ${landing.z} m`,
      );
    } finally {
      await page.keyboard.up(steer);
      await page.keyboard.up("ArrowUp");
    }
  }
}

async function checkLongitudinalGap(page) {
  const gapId = "gms:40FXJKCA78QRCR5YNVX5TV8TVN";
  const start = await visit(page, "6th-brannan", gapId);
  assert.equal(start.kind, "bridge_deck");
  await page.keyboard.down("ArrowUp");
  try {
    await page.waitForFunction(
      () =>
        document
          .querySelector("#next")
          .textContent.includes("End of supported surface"),
      null,
      { timeout: 120000 },
    );
    const end = await page.evaluate(state);
    assert.equal(end.id, gapId);
    assert.equal(end.contact, "grounded");
    assert.ok(Math.hypot(end.x - start.x, end.y - start.y) > 3);
    await page.waitForTimeout(500);
    const held = await page.evaluate(state);
    assert.equal(held.id, gapId);
    assert.equal(held.contact, "grounded");
    assert.ok(Math.hypot(held.x - end.x, held.y - end.y) < 0.15);
    console.log(`6th-brannan: deck gap stopped contact at ${end.z} m`);
  } catch (error) {
    throw new Error(
      `6th-brannan deck gap: ${JSON.stringify(await page.evaluate(state))}`,
      { cause: error },
    );
  } finally {
    await page.keyboard.up("ArrowUp");
  }
}

async function checkRampApproach(page) {
  const params = new URLSearchParams({
    region: "6th-brannan",
    review: "sixth-brannan-ramp-approach",
  });
  await page.goto(`${host}/sf/?${params}`);
  await page
    .locator('#drive[data-contact="grounded"][data-surface="city"]')
    .waitFor({ timeout: 30000 });
  const street = await page.evaluate(state);
  assert.equal(street.id, "");
  await page.keyboard.down("ArrowUp");
  try {
    const target = "gms:55M0AW4H11A0P40ZHJBKQ76ZX7";
    await page.waitForFunction(
      (id) => {
        const data = document.querySelector("#drive").dataset;
        return data.surfaceId === id && data.contact === "grounded";
      },
      target,
      { timeout: 60000 },
    );
    const joined = await page.evaluate(state);
    assert.ok(joined.z - street.z < 0.6);
    await page.waitForFunction(
      ({ id, z }) => {
        const data = document.querySelector("#drive").dataset;
        return (
          data.surfaceId === id &&
          data.contact === "grounded" &&
          Number(data.altitude) > z + 1.5
        );
      },
      { id: target, z: street.z },
      { timeout: 60000 },
    );
    await page.waitForFunction(
      () =>
        document
          .querySelector("#next")
          .textContent.includes("End of supported surface"),
      null,
      { timeout: 60000 },
    );
    const stopped = await page.evaluate(state);
    assert.equal(stopped.id, target);
    assert.equal(stopped.contact, "grounded");
    assert.ok(stopped.z > 5.8 && stopped.z < 6.2);
    console.log(
      `6th–Brannan: climbed from city terrain at ${street.z} m via ${joined.z} m onto the strip and stopped at ${stopped.z} m`,
    );
  } catch (error) {
    throw new Error(
      `6th–Brannan ramp: ${JSON.stringify(await page.evaluate(state))}`,
      { cause: error },
    );
  } finally {
    await page.keyboard.up("ArrowUp");
  }
}

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 360, height: 280 },
    });
    await page.route("**/.11ty/reload-client.js", (route) => route.abort());
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    if (process.argv.includes("--ramp")) {
      await checkRampApproach(page);
      assert.deepEqual(errors, []);
      return;
    }
    if (process.argv.includes("--edges")) {
      await checkHighwayEdges(page);
      assert.deepEqual(errors, []);
      return;
    }
    if (process.argv.includes("--gap")) {
      await checkLongitudinalGap(page);
      assert.deepEqual(errors, []);
      return;
    }
    await page.goto(`${host}/sf/`);
    await page.locator("#menuBtn").click();
    await page.getByRole("link", { name: "Open regional road review" }).click();
    await page.waitForFunction(
      () => document.querySelector("#roadRegion").options.length === 11,
    );
    const regions = await page
      .locator("#roadRegion option")
      .evaluateAll((options) =>
        options.map((option) => option.value).filter(Boolean),
      );
    for (const region of regions) {
      const car = await visit(page, region);
      console.log(`${region}: ${car.kind} start at ${car.z} m`);
    }
    await delayedJoin(page, {
      region: "6th-brannan",
      join: "gms:5SXB9CCZQT47D11VRWG0K43QS2",
      token: "808f7fd4",
      from: "gms:21R41RZ4Q8HAHQA50TVGD0ZQZ3",
      to: "gms:4AGWQJGFG2Z4RJ1M6VP9CJFW85",
    });
    await delayedJoin(page, {
      region: "geary-masonic",
      join: "gms:2RFR8G9NH7E5JTW81PWJ5J6CY3",
      token: "8085874c",
      from: "gms:6H607XR50DH9TZFY6GM78MA6FW",
      to: "gms:16Y7WN0QZHATVJMP02XXGQBKDQ",
    });
    await checkLongitudinalGap(page);
    await checkRampApproach(page);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
