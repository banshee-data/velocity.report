// Two unmodified built sites, one localhost origin, one disposable Chromium profile.
// Usage: node tools/sf-drive/smoke-offline-upgrade.cjs OLD_SITE NEW_SITE
// Add --require-continuity to turn the observed upgrade loss into a failing guard.
const assert = require("node:assert/strict");
const { createServer } = require("node:http");
const { readFileSync, statSync } = require("node:fs");
const path = require("node:path");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

const roots = process.argv.slice(2, 4).map((p) => path.resolve(p));
assert.equal(
  roots.length,
  2,
  "Provide OLD_SITE and NEW_SITE built with make build-docs",
);
const json = (root, file) => JSON.parse(readFileSync(path.join(root, file)));
const manifests = roots.map((root) => json(root, "sf/tiles/manifest.json"));
const routes = roots.map((root) => json(root, "sf/quest-route.json"));
const workers = roots.map((root) =>
  readFileSync(path.join(root, "sf/offline.js"), "utf8"),
);
const versions = workers.map(
  (worker) => /const version = "([^"]+)"/.exec(worker)[1],
);
assert.notEqual(versions[0], versions[1]);
const progressKeys = manifests.map(
  (m, i) =>
    `sf-progress-v1:sf-trial-v2:${m.terrainRebake?.progressBaseSha256 || m.roadBake.baseSha256}${routes[i].placementSha256 ? ":" + routes[i].placementSha256 : ""}:embarcadero`,
);
assert.equal(
  progressKeys[0],
  progressKeys[1],
  "This scenario isolates map upgrades with compatible progress identity",
);
const key = progressKeys[0];
const target = routes[0].targets.find((t) => t.t === "8085806c");
assert.ok(target);
let revision = 0;
const requests = [];
const server = createServer((req, res) => {
  try {
    const pathname = decodeURIComponent(
      new URL(req.url, "http://localhost").pathname,
    );
    let file = path.resolve(roots[revision], "." + pathname);
    if (!file.startsWith(roots[revision] + path.sep))
      throw new Error("Outside site");
    if (statSync(file).isDirectory()) file = path.join(file, "index.html");
    const types = {
      ".html": "text/html",
      ".js": "text/javascript",
      ".css": "text/css",
      ".json": "application/json",
      ".woff2": "font/woff2",
    };
    requests.push({ revision, pathname });
    res.writeHead(200, {
      "Content-Type": types[path.extname(file)] || "application/octet-stream",
      "Cache-Control": "no-store",
    });
    res.end(readFileSync(file));
  } catch {
    console.error("HTTP404", revision, req.url);
    res.writeHead(404);
    res.end();
  }
});
const inventory = (page) =>
  page.evaluate(async () =>
    Promise.all(
      (await caches.keys()).map(async (name) => ({
        name,
        count: (await (await caches.open(name)).keys()).length,
      })),
    ),
  );
const saved = (page) =>
  page.evaluate((key) => JSON.parse(localStorage.getItem(key)), key);
const state = (page) =>
  page.evaluate(() => ({
    progress: document.querySelector("#progress")?.textContent,
    contact: document.querySelector("#drive")?.dataset.contact,
    east: document.querySelector("#drive")?.dataset.east,
    north: document.querySelector("#drive")?.dataset.north,
    status: document.querySelector("#tileDownloadStatus")?.textContent,
    coverage: document.querySelector("#offlineTileCount")?.textContent,
  }));
const grounded = (page) =>
  page.locator('#drive[data-contact="grounded"]').waitFor();
async function download(page) {
  if (!(await page.locator("#downloadTiles").isVisible()))
    await page.locator("#menuBtn").click();
  await page.locator("#downloadTiles").click();
  await page.waitForFunction(() =>
    document
      .querySelector("#tileDownloadStatus")
      .textContent.startsWith("All "),
  );
  return page.locator("#tileDownloadStatus").textContent();
}
async function probe(page, token) {
  return page.evaluate(async (token) => {
    try {
      const response = await fetch(`/sf/tiles/${token}.json.gz`);
      return {
        ok: response.ok,
        bytes: (await response.arrayBuffer()).byteLength,
      };
    } catch (error) {
      return { ok: false, error: error.message };
    }
  }, token);
}

(async () => {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  const evidence = { chromium: browser.version(), versions };
  const emit = (phase, data) => {
    evidence[phase] = data;
    console.log(JSON.stringify({ phase, ...data }));
  };
  try {
    const context = await browser.newContext({
      viewport: { width: 800, height: 560 },
    });
    context.setDefaultTimeout(90_000);
    const old = await context.newPage();
    const errors = [];
    old.on("console", (msg) => {
      if (/INSTALLING|WORKERSTATE/.test(msg.text())) console.log(msg.text());
    });
    context.on("page", (page) =>
      page.on("pageerror", (error) => errors.push(error.message)),
    );
    old.on("pageerror", (error) => errors.push(error.message));
    await old.goto(origin + "/sf/?at=embarcadero");
    await grounded(old);
    emit("oldReady", { state: await state(old) });
    await old.waitForFunction(() => navigator.serviceWorker.controller);
    emit("oldDownload", {
      message: await download(old),
      caches: await inventory(old),
    });
    await old.evaluate(
      ({ key, target }) =>
        localStorage.setItem(
          key,
          JSON.stringify({
            elapsed: 8,
            crashes: 0,
            collected: [],
            checkpoint: {
              x: target.x - 10 * Math.cos(target.heading),
              y: target.y - 10 * Math.sin(target.heading),
              heading: target.heading,
            },
          }),
        ),
      { key, target },
    );
    await context.setOffline(true);
    await old.reload();
    await grounded(old);
    await confirmManualTakeover(old);
    await old.keyboard.down("ArrowUp");
    try {
      await old.waitForFunction(
        ({ key, token }) =>
          JSON.parse(localStorage.getItem(key)).collected.includes(token),
        { key, token: target.t },
      );
    } finally {
      await old.keyboard.up("ArrowUp");
    }
    await old.reload();
    await grounded(old);
    assert.match(await old.locator("#progress").textContent(), /^1\//);
    const checkpointBefore = (await saved(old)).checkpoint;
    emit("oldOfflineReload", {
      state: await state(old),
      saved: await saved(old),
    });
    // Keep this fully loaded old document open while its controller is replaced.
    await old.locator("#menuBtn").click();
    await context.setOffline(false);
    revision = 1;
    await old.evaluate(async () => {
      const changed = new Promise((resolve, reject) => {
        const timer = setTimeout(
          () => reject(new Error("Worker activation timed out")),
          60000,
        );
        navigator.serviceWorker.addEventListener(
          "controllerchange",
          () => {
            clearTimeout(timer);
            resolve();
          },
          { once: true },
        );
      });
      const registration = await navigator.serviceWorker.getRegistration();
      registration.addEventListener("updatefound", () => {
        const worker = registration.installing;
        console.log("INSTALLING", worker.state);
        worker.addEventListener("statechange", () =>
          console.log("WORKERSTATE", worker.state),
        );
      });
      await registration.update();
      await changed;
    });
    await context.setOffline(true);
    const postActivationCaches = await inventory(old);
    const newCacheName = `sf-tiles-/sf/-${versions[1]}`;
    const absent = await old.evaluate(
      async ({ name, tokens }) => {
        const cache = await caches.open(name);
        for (const token of tokens)
          if (
            !(await cache.match(
              new URL(`/sf/tiles/${token}.json.gz`, location.href),
            ))
          )
            return token;
      },
      { name: newCacheName, tokens: manifests[0].tiles.map((t) => t.t) },
    );
    assert.ok(absent);
    const oldCacheHasTile = await old.evaluate(
      async ({ name, token }) =>
        !!(await (
          await caches.open(name)
        ).match(new URL(`/sf/tiles/${token}.json.gz`, location.href))),
      { name: `sf-tiles-/sf/-${versions[0]}`, token: absent },
    );
    assert.equal(oldCacheHasTile, true);
    const missingResponse = await probe(old, absent);
    emit("oldTabAfterUpgrade", {
      state: await state(old),
      caches: postActivationCaches,
      probeToken: absent,
      oldCacheHasTile,
      response: missingResponse,
      saved: await saved(old),
    });
    const fresh = await context.newPage();
    const tileFailures = [];
    fresh.on("requestfailed", (request) => {
      if (request.url().endsWith(".json.gz")) tileFailures.push(request.url());
    });
    await fresh.goto(origin + "/sf/?at=embarcadero");
    // Wait for observable tile failures, not a fixed rendering delay.
    await fresh.waitForFunction(() =>
      document.querySelector("#progress")?.textContent.startsWith("1/"),
    );
    await fresh.waitForFunction(
      () => document.querySelector("#drive")?.dataset.contact !== undefined,
    );
    emit("freshOfflineBeforeRedownload", {
      state: await state(fresh),
      saved: await saved(fresh),
      tileFailures: tileFailures.length,
      response: await probe(fresh, absent),
    });
    assert.ok((await saved(fresh)).collected.includes(target.t));
    assert.deepEqual((await saved(fresh)).checkpoint, checkpointBefore);
    // Reload once more offline to exclude a warm document as the explanation.
    await fresh.reload();
    await fresh.waitForFunction(() =>
      document.querySelector("#progress")?.textContent.startsWith("1/"),
    );
    emit("freshOfflineSecondReload", {
      state: await state(fresh),
      saved: await saved(fresh),
    });
    await context.setOffline(false);
    await fresh.reload();
    await grounded(fresh);
    emit("newDownload", {
      message: await download(fresh),
      caches: await inventory(fresh),
    });
    await context.setOffline(true);
    await fresh.reload();
    await grounded(fresh);
    assert.match(await fresh.locator("#progress").textContent(), /^1\//);
    const after = await state(fresh);
    assert.ok(
      Math.hypot(
        Number(after.east) - checkpointBefore.x,
        Number(after.north) - checkpointBefore.y,
      ) < 2,
      "Restored car remains near the saved checkpoint",
    );
    const newResponse = await probe(fresh, absent);
    assert.equal(newResponse.ok, true);
    emit("freshOfflineAfterRedownload", {
      state: after,
      saved: await saved(fresh),
      response: newResponse,
      caches: await inventory(fresh),
    });
    emit("oldTabAfterRedownload", {
      state: await state(old),
      response: await probe(old, absent),
    });
    assert.deepEqual(errors, []);
    const finalCaches = await inventory(fresh);
    assert.equal(
      finalCaches.filter((c) => c.name.startsWith("sf-tiles-")).length,
      2,
    );
    assert.ok(
      finalCaches
        .filter((c) => c.name.startsWith("sf-tiles-"))
        .every((c) => c.count === manifests[0].tiles.length),
    );
    emit("summary", {
      chromium: evidence.chromium,
      versions,
      continuity: missingResponse.ok,
      retainedTileCaches: 2,
      oldTileCount: manifests[0].tiles.length,
      newTileCount: manifests[1].tiles.length,
      pageErrors: errors,
      networkTileRequests: requests.filter((r) =>
        r.pathname.endsWith(".json.gz"),
      ).length,
    });
    if (process.argv.includes("--require-continuity"))
      assert.equal(
        missingResponse.ok,
        true,
        "A completed offline download stopped serving tiles after worker activation",
      );
    else
      assert.equal(
        missingResponse.ok,
        false,
        "Known failure changed; review the expectations and enable the continuity guard",
      );
  } finally {
    await browser.close();
  }
})()
  .catch((error) => {
    console.error(error);
    process.exitCode = 1;
  })
  .finally(() => server.close());
