// Run with Playwright available via NODE_PATH and Chromium on the Hydra dev host.
const assert = require("node:assert/strict");
const { gunzipSync } = require("node:zlib");
const { chromium } = require("playwright");

(async () => {
  const { driveOnStreetContact, indexRoadContact, roadSupportsCar } =
    await import("../../public_html/src/js/sf-road-contact.js");
  const { contactStep, groundAtWorld } =
    await import("../../public_html/src/js/sf-drive-core.js");
  const host = process.env.SF_BASE_URL || "http://localhost:8090";
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
    page.on("console", (message) => {
      if (
        message.type() === "error" &&
        /THREE.WebGLProgram/.test(message.text())
      )
        errors.push(message.text());
    });
    const read = () =>
      page.locator("#drive").evaluate(({ dataset }) => ({
        x: Number(dataset.east),
        y: Number(dataset.north),
        z: Number(dataset.altitude),
        kind: dataset.surface,
        contact: dataset.contact,
      }));
    const drives = [
      ["bay-bridge-deck", "bridge_deck"],
      ["bay-bridge-street", "ground"],
    ];
    for (const [at, kind] of drives) {
      if (process.argv.includes("--street") && kind !== "ground") continue;
      const params = process.env.SF_BRIDGE_REGIONAL
        ? new URLSearchParams({ region: "embarcadero-bryant", review: at })
        : new URLSearchParams({ at });
      const tile = page.waitForResponse(
        (res) => {
          const url = new URL(res.url());
          return (
            url.hostname === "arrow-worker" &&
            url.pathname.endsWith("/80858074.json") &&
            res.ok()
          );
        },
        { timeout: 60000 },
      );
      await page.goto(`${host}/sf/?${params}`);
      const product = await tile
        .then((response) => response.json())
        .catch(async (cause) => {
          throw new Error(
            JSON.stringify({
              at,
              status: await page.locator("#next").textContent(),
              errors,
            }),
            { cause },
          );
        });
      const selected = product.surfaces.find(
        (surface) => surface.kind === kind,
      );
      const crossing = product.surfaces.find(
        (surface) => surface.kind !== kind,
      );
      const heightAt = indexRoadContact(selected.contact);
      const otherHeight = indexRoadContact(crossing.contact);
      await page.waitForFunction((kind) => {
        const { surface, contact } = document.querySelector("#drive").dataset;
        return surface === kind && contact === "grounded";
      }, kind);
      const start = await read();
      let crossed = false;
      let exited = false;
      let min = start.z,
        max = start.z;
      await page.keyboard.down("ArrowUp");
      const deadline = Date.now() + 120000;
      try {
        while (Date.now() < deadline && !exited) {
          const car = await read();
          const expected = heightAt(car.x, car.y);
          assert.equal(car.kind, kind);
          assert.equal(car.contact, "grounded");
          assert.ok(expected !== null && Math.abs(expected - car.z) < 0.035);
          min = Math.min(min, car.z);
          max = Math.max(max, car.z);
          const other = otherHeight(car.x, car.y);
          if (other !== null) {
            crossed = true;
            assert.ok(Math.abs(car.z - other) > 40);
          } else if (crossed) exited = true;
          if (!exited) await new Promise((resolve) => setTimeout(resolve, 55));
        }
      } finally {
        await page.keyboard.up("ArrowUp");
      }
      assert.ok(
        crossed && exited,
        JSON.stringify({
          at,
          crossed,
          start,
          last: await read(),
          speed: await page.locator("#speed").textContent(),
          status: await page.locator("#next").textContent(),
          errors,
        }),
      );
      if (kind === "ground") {
        const reply = await page.request.get(
          `${host}/sf/tiles/80858074.json.gz`,
        );
        assert.ok(reply.ok());
        const bytes = await reply.body();
        const pavement = JSON.parse(
          bytes[0] === 0x1f && bytes[1] === 0x8b ? gunzipSync(bytes) : bytes,
        );
        const terrainHeight = (x, y) => groundAtWorld([pavement], x, y, true);
        const passed = await read();
        const heading = Math.atan2(passed.y - start.y, passed.x - start.x);
        // Offsetting the underpass start exposed holes in the real contact.
        // Exercise each lane against the baked pavement served by the game.
        for (const side of [-0.5, 1, 1.5, 2]) {
          const x = start.x - side * Math.sin(heading);
          const y = start.y + side * Math.cos(heading);
          let car = {
            x,
            y,
            z: heightAt(x, y),
            heading,
            v: 5,
            grounded: true,
            surfaceId: selected.id,
          };
          assert.ok(
            roadSupportsCar(heightAt, car),
            `offset ${side} starts outside supported contact`,
          );
          let underneath = false;
          const input = { throttle: 1 };
          for (
            let i = 0;
            i < 500 && Math.hypot(car.x - x, car.y - y) < 28;
            i++
          ) {
            car = car.surfaceId
              ? driveOnStreetContact(
                  car,
                  input,
                  1 / 120,
                  { heightAt },
                  terrainHeight,
                )
              : contactStep(
                  car,
                  input,
                  1 / 120,
                  (x, y) => ({ height: terrainHeight(x, y) }),
                  1.35,
                );
            assert.equal(
              car.blocked,
              false,
              `underpass blocked at offset ${side}`,
            );
            assert.ok(
              car.z > 2.9 && car.z < 3.6,
              `wrong level at offset ${side}`,
            );
            underneath ||= otherHeight(car.x, car.y) !== null;
          }
          assert.ok(underneath && Math.hypot(car.x - x, car.y - y) >= 28);
        }
        await page.keyboard.down("ArrowUp");
        try {
          await page.waitForFunction(
            ({ x, y }) => {
              const data = document.querySelector("#drive").dataset;
              return (
                data.surface === "city" &&
                data.contact === "grounded" &&
                Math.hypot(Number(data.east) - x, Number(data.north) - y) > 83
              );
            },
            start,
            { timeout: 90000 },
          );
          const continuation = await read();
          assert.ok(
            Math.abs(
              continuation.z - terrainHeight(continuation.x, continuation.y),
            ) < 0.02,
          );
          console.log(
            "Embarcadero: crossed four lane offsets and continued past the clip on city pavement",
          );
        } finally {
          await page.keyboard.up("ArrowUp");
        }
      }
      await page.keyboard.press("KeyR");
      await page.waitForFunction(({ x, y, kind }) => {
        const data = document.querySelector("#drive").dataset;
        return (
          data.surface === kind &&
          data.contact === "grounded" &&
          Math.hypot(Number(data.east) - x, Number(data.north) - y) < 0.1
        );
      }, start);
      console.log(
        `${kind} crossed and reset on its own level (${min.toFixed(2)}–${max.toFixed(2)} m)`,
      );
    }
    if (process.env.SF_BRIDGE_REGIONAL || process.argv.includes("--street")) {
      assert.deepEqual(errors, []);
      return;
    }
    await page.goto(
      `${process.env.SF_BASE_URL || "http://localhost:8090"}/sf/`,
    );
    await page.waitForFunction(
      () => document.querySelector("#trialArea").options.length > 0,
    );
    await page.locator("#menuBtn").click();
    await page.locator("#gameMode").selectOption("solo");
    await page.getByRole("button", { name: "Close settings" }).click();
    await page.keyboard.down("ArrowUp");
    try {
      await page.waitForFunction(
        () =>
          !document.querySelector("#airtime").hidden &&
          document
            .querySelector(".airtime-card:last-child .airtime-readout")
            ?.textContent.startsWith("Airborne"),
      );
      await page.waitForFunction(() =>
        document
          .querySelector(".airtime-card:last-child .airtime-readout")
          ?.textContent.startsWith("Touchdown"),
      );
    } finally {
      await page.keyboard.up("ArrowUp");
    }
    console.log(await page.locator(".airtime-readout").last().textContent());
    await page.goto(
      `${host}/sf/?tiles=${encodeURIComponent(`${host}/missing-tiles`)}`,
    );
    await page.waitForFunction(() =>
      document.querySelector("#street").textContent.includes("could not"),
    );
    await page.locator("#menuBtn").click();
    await page
      .getByLabel("SF tile directory or manifest URL")
      .fill(`${host}/sf/tiles`);
    await page.getByLabel(/Gerardus address/).fill("http://arrow-worker:50061");
    const manifest = page.waitForResponse(
      (res) =>
        new URL(res.url()).hostname === "arrow-worker" &&
        res.url().endsWith("/manifest.json") &&
        res.ok(),
    );
    await page.getByRole("button", { name: "Apply map sources" }).click();
    await manifest;
    await page.waitForFunction(() =>
      [...document.querySelector("#places").options].some(
        (option) => option.value === "bay-bridge-deck",
      ),
    );
    await page.locator("#menuBtn").click();
    await page.locator("#mapSources").evaluate((details) => {
      details.open = true;
    });
    assert.equal(
      await page.getByLabel(/Gerardus address/).inputValue(),
      "http://arrow-worker:50061",
    );
    console.log(
      "Map sources recovered from a missing tile directory and loaded Gerardus.",
    );
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
