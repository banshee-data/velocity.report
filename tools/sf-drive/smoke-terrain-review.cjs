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
  const key = `sf-progress-v1:sf-trial-v2:${manifest.roadBake.baseSha256}${placement}:${id}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  try {
    for (const [name, x, y, heading] of [
      ["21st Street approach", 4746, -64.19, 0.077],
      ["23rd Street approach", 4154, -542.75, 0.077],
      ["Old 23rd and Iowa checkpoint", 4234.658, -536.563, -3.075],
    ]) {
      const page = await browser.newPage({
        viewport: { width: 700, height: 480 },
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        await page.addInitScript(
          ({ key, x, y, heading }) => {
            localStorage.setItem(
              key,
              JSON.stringify({
                collected: [],
                elapsed: 1,
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
          assert.ok(Math.hypot(start.east - x, start.north - y) > 1);
        } else {
          assert.ok(Math.abs(start.east - x) < 0.01);
          assert.ok(Math.abs(start.north - y) < 0.01);
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
