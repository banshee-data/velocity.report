// Run with Playwright available via NODE_PATH and Chromium on the Hydra dev host.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

(async () => {
  const { indexRoadContact } =
    await import("../../public_html/src/js/sf-road-contact.js");
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 640, height: 480 },
    });
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
    for (const [at, kind] of [
      ["bay-bridge-deck", "bridge_deck"],
      ["bay-bridge-street", "ground"],
    ]) {
      const tile = page.waitForResponse((res) => {
        const url = new URL(res.url());
        return (
          url.hostname === "arrow-worker" &&
          url.pathname.endsWith("/80858074.json") &&
          res.ok()
        );
      });
      await page.goto(
        `${process.env.SF_BASE_URL || "http://localhost:8090"}/sf/?at=${at}`,
      );
      const product = await (await tile).json();
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
      const deadline = Date.now() + 60000;
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
            .querySelector("#airtimeReadout")
            .textContent.startsWith("Airborne"),
      );
      await page.waitForFunction(() =>
        document
          .querySelector("#airtimeReadout")
          .textContent.startsWith("Touchdown"),
      );
    } finally {
      await page.keyboard.up("ArrowUp");
    }
    console.log(await page.locator("#airtimeReadout").textContent());
    const host = process.env.SF_BASE_URL || "http://localhost:8090";
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
    await page
      .getByLabel("Gerardus address or overlay manifest URL")
      .fill("http://arrow-worker:50061");
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
      await page
        .getByLabel("Gerardus address or overlay manifest URL")
        .inputValue(),
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
