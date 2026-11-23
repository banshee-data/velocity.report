// Run against the built SF site with Playwright available on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");

async function main() {
  const origin = process.env.SF_BASE_URL || "http://localhost:8090";
  const [manifest, route] = await Promise.all(
    ["tiles/manifest.json", "quest-route.json"].map(async (name) => {
      const response = await fetch(`${origin}/sf/${name}`);
      assert.ok(response.ok, name);
      return response.json();
    }),
  );
  const id = manifest.places[0].id;
  const placement = route.placementSha256 ? `:${route.placementSha256}` : "";
  const progressBase =
    manifest.terrainRebake?.progressBaseSha256 || manifest.roadBake.baseSha256;
  const key = `sf-progress-v1:sf-trial-v2:${progressBase}${placement}:${id}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    for (const [name, x, y, heading] of [
      ["21st Street approach", 4756, -63.421, 0.077],
      ["23rd Street approach", 4154, -542.75, 0.077],
      ["Old 23rd and Iowa checkpoint", 4234.658, -536.563, -3.075],
    ]) {
      const page = await browser.newPage({
        viewport: { width: 540, height: 390 },
      });
      page.setDefaultTimeout(75_000);
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        await page.addInitScript(
          ({ key, x, y, heading }) => {
            localStorage.setItem(
              key,
              JSON.stringify({
                collected: [],
                elapsed: 0,
                checkpoint: { x, y, heading },
              }),
            );
          },
          { key, x, y, heading },
        );
        await page.goto(`${origin}/sf/?at=${id}`);
        await page.waitForFunction(
          () => document.querySelector("#drive").dataset.contact === "grounded",
        );
        const snapshot = () =>
          page
            .locator("#drive")
            .evaluate(({ dataset: d }) =>
              Object.fromEntries(
                ["east", "north", "altitude"].map((field) => [
                  field,
                  Number(d[field]),
                ]),
              ),
            );
        const start = await snapshot();
        if (name.startsWith("Old")) {
          assert.ok(Math.hypot(start.east - x, start.north - y) > 1, name);
        } else {
          assert.ok(
            Math.hypot(start.east - x, start.north - y) < 1,
            `${name} restored at ${JSON.stringify(start)}`,
          );
        }
        if (name === "23rd Street approach") {
          await page.locator("#drive").focus();
          await page.keyboard.press("ArrowUp");
          await page
            .getByRole("button", { name: "Take manual control" })
            .click();
          await page.keyboard.down("ArrowUp");
          await page.waitForFunction(
            () =>
              document.querySelector("#drive").dataset.roadBlocked === "true",
          );
          await page.keyboard.up("ArrowUp");
          const stopped = await snapshot();
          assert.ok(stopped.east > 4165 && stopped.east < 4168);
          assert.ok(stopped.altitude > 14.7 && stopped.altitude < 15.4);
        }
        assert.deepEqual(errors, []);
        console.log(name, start);
      } finally {
        await page.close();
      }
    }
  } finally {
    await browser.close();
  }
}
main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
