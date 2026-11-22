// Build and serve public_html/_site, then run with Playwright on NODE_PATH.
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

// Use the real drive draw path with an opaque wall inserted directly between
// the camera and the marker. Compare pixels with the HUD hidden and glyph removed.
async function wallFixture(page) {
  await page.route("**/sf-drive.js", async (route) => {
    const response = await route.fetch();
    const source = await response.text();
    const hook = "    camera.updateMatrixWorld();\n    emojiScreen.copy";
    assert.ok(source.includes(hook));
    await route.fulfill({
      response,
      body: source.replace(
        hook,
        `
      window.markerSpriteVisible = () => marker.userData.sprite.visible;
      if (window.emojiWall) {
        let wall = scene.getObjectByName("emoji-wall-fixture");
        if (!wall) {
          wall = new THREE.Mesh(new THREE.PlaneGeometry(80, 80),
            new THREE.MeshBasicMaterial({ color: 0xff00ff, fog: false, toneMapped: false }));
          wall.name = "emoji-wall-fixture";
          scene.add(wall);
        }
        wall.position.copy(camera.position).lerp(marker.userData.sprite.position, 0.65);
        wall.lookAt(camera.position);
      }
${hook}`,
      ),
    });
  });
}

async function verifyOccludedGlyph(page, emoji) {
  await page.evaluate(() => {
    window.emojiWall = true;
  });
  assert.equal(await page.evaluate(() => window.markerSpriteVisible()), false);
  const bounds = await emoji.boundingBox();
  const clip = {
    x: Math.round(bounds.x + 7),
    y: Math.round(bounds.y + 7),
    width: 30,
    height: 30,
  };
  const pixels = async () => {
    const png = await page.screenshot({ clip });
    return page.evaluate(
      async (bytes) => {
        const bitmap = await createImageBitmap(
          new Blob([new Uint8Array(bytes)], { type: "image/png" }),
        );
        const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
        const context = canvas.getContext("2d");
        context.drawImage(bitmap, 0, 0);
        return Array.from(
          context.getImageData(0, 0, bitmap.width, bitmap.height).data,
        );
      },
      [...png],
    );
  };
  await emoji.evaluate((button) => {
    button.style.visibility = "hidden";
  });
  const wall = await pixels();
  const centre = (15 * 30 + 15) * 4;
  assert.deepEqual(
    wall.slice(centre, centre + 3),
    [255, 0, 255],
    "opaque wall covers the marker's projected centre",
  );
  const glyph = await emoji.evaluate((button) => {
    button.style.visibility = "";
    const text = button.textContent;
    button.textContent = "";
    return text;
  });
  const empty = await pixels();
  await emoji.evaluate((button, text) => {
    button.textContent = text;
  }, glyph);
  const visible = await pixels();
  const changed = visible.filter(
    (channel, i) => Math.abs(channel - empty[i]) > 25,
  ).length;
  assert.ok(
    changed > 70,
    `visible glyph must change pixels over the wall (${changed})`,
  );
  console.log(
    `Opaque wall covers the 3D sprite; the interactive glyph changed ${changed} foreground channels`,
  );
}

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
    args: ["--no-sandbox", "--enable-unsafe-swiftshader"],
  });
  const url = `${process.env.SF_BASE_URL || "http://localhost:8090"}/sf/`;
  try {
    for (const mobile of [false, true]) {
      const page = await browser.newPage({
        viewport: mobile
          ? { width: 390, height: 690 }
          : { width: 1000, height: 700 },
        serviceWorkers: "block",
        hasTouch: mobile,
        isMobile: mobile,
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      if (!mobile) await wallFixture(page);
      await page.goto(`${url}?at=embarcadero`);
      const emoji = page.locator("#emojiFocus");
      await emoji.waitFor({ timeout: 90000 });
      await page.locator("#menuBtn").click();
      const menu = page.locator("#gameMenu");
      assert.equal(
        await menu
          .locator(".tools")
          .first()
          .locator("input,select")
          .first()
          .getAttribute("id"),
        "viewMode",
      );
      assert.equal(
        await menu.evaluate((el) => el.lastElementChild.id),
        "mapSources",
      );
      assert.equal(
        await menu
          .locator("#mapSources input, #mapSources select, #mapSources button")
          .count(),
        0,
      );
      assert.ok((await emoji.textContent()).trim());
      assert.equal(
        await menu
          .locator("#gameMode, #trialArea, #restartBtn:visible")
          .count(),
        0,
      );
      const slider = menu.getByRole("slider", { name: /^Camera/ });
      await slider.press("ArrowRight");
      assert.equal(await slider.getAttribute("aria-valuetext"), "Overhead");
      await slider.press("End");
      assert.equal(await slider.getAttribute("aria-valuetext"), "In car");
      await slider.press("Home");
      await menu.getByRole("button", { name: "Close settings" }).click();

      await confirmManualTakeover(page);
      const position = () =>
        page
          .locator("#drive")
          .evaluate((el) => [el.dataset.east, el.dataset.north]);
      if (!mobile) await verifyOccludedGlyph(page, emoji);
      const before = await position();
      if (mobile) await emoji.tap();
      else await emoji.press("Enter");
      assert.equal(await emoji.getAttribute("aria-pressed"), "true");
      await page.waitForFunction(() => {
        const r = document.querySelector("#emojiFocus").getBoundingClientRect();
        return (
          Math.abs(r.x + r.width / 2 - innerWidth / 2) < 1 &&
          Math.abs(r.y + r.height / 2 - innerHeight / 2) < 1
        );
      });
      assert.deepEqual(await position(), before);
      if (mobile) {
        const cdp = await page.context().newCDPSession(page);
        const r = await page
          .getByRole("button", { name: "Accelerate" })
          .boundingBox();
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchStart",
          touchPoints: [{ x: r.x + r.width / 2, y: r.y + r.height / 2 }],
        });
        await page.waitForFunction(
          () => Number(document.querySelector("#speed").textContent) > 0,
        );
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchEnd",
          touchPoints: [],
        });
      } else {
        await page.keyboard.down("ArrowUp");
        await page.waitForFunction(
          () => Number(document.querySelector("#speed").textContent) > 0,
        );
        await page.keyboard.up("ArrowUp");
      }
      assert.equal(await emoji.getAttribute("aria-pressed"), "false");
      await page.keyboard.press("r");
      assert.equal(await emoji.getAttribute("aria-pressed"), "false");
      await page.goto(`${url}?city=bay-bridge`);
      await page
        .locator('#drive[data-surface-id="bay-bridge-westbound"]')
        .waitFor();
      assert.equal(await emoji.isVisible(), false);
      await page.goto(`${url}?mode=solo`);
      await page.waitForFunction(
        () => document.querySelector("#drive").dataset.soloStatus === "ready",
      );
      assert.equal(await emoji.isVisible(), false);
      assert.deepEqual(errors, []);
      console.log(
        `${mobile ? "390×690 touch" : "Desktop keyboard"}: menu order, camera slider, emoji centring, drive release, reset and solo route passed`,
      );
      await page.close();
    }
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
