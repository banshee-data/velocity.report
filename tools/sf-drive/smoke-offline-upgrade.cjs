// Two unmodified built sites, one localhost origin, one disposable Chromium profile.
// Usage: node tools/sf-drive/smoke-offline-upgrade.cjs OLD_SITE NEW_SITE
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { createServer } = require("node:http");
const { readFileSync, statSync } = require("node:fs");
const path = require("node:path");
const { gunzipSync } = require("node:zlib");
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
const payloadHash = (root, token) =>
  createHash("sha256")
    .update(
      gunzipSync(
        readFileSync(path.join(root, "sf", "tiles", `${token}.json.gz`)),
      ),
    )
    .digest("hex");
const tileHashes = new Map(
  manifests[1].tiles.map(({ t }) => [t, payloadHash(roots[1], t)]),
);
const previousTokens = new Set(manifests[0].tiles.map(({ t }) => t));
const changed = [...tileHashes]
  .filter(
    ([token, hash]) =>
      previousTokens.has(token) && payloadHash(roots[0], token) !== hash,
  )
  .map(([token]) => token);
assert.ok(
  changed.length,
  "Use an upgrade that changes at least one existing tile",
);

async function cachedHashes(page, tokens) {
  return page.evaluate(async (tokens) => {
    const hashes = {};
    let next = 0;
    async function check() {
      while (next < tokens.length) {
        const token = tokens[next++];
        const response = await fetch(`/sf/tiles/${token}.json.gz`);
        if (!response.ok)
          throw new Error(`${token} returned ${response.status}`);
        const raw = await response.arrayBuffer();
        const head = new Uint8Array(raw);
        const json =
          head[0] === 0x1f && head[1] === 0x8b
            ? await new Response(
                new Blob([raw])
                  .stream()
                  .pipeThrough(new DecompressionStream("gzip")),
              ).arrayBuffer()
            : raw;
        const hash = await crypto.subtle.digest("SHA-256", json);
        hashes[token] = Array.from(new Uint8Array(hash), (byte) =>
          byte.toString(16).padStart(2, "0"),
        ).join("");
      }
    }
    await Promise.all(Array.from({ length: 8 }, check));
    return hashes;
  }, tokens);
}

(async () => {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    const context = await browser.newContext({
      viewport: { width: 800, height: 560 },
    });
    context.setDefaultTimeout(30_000);
    const errors = [];
    context.on("page", (page) =>
      page.on("pageerror", (error) => errors.push(error.message)),
    );
    const old = await context.newPage();
    await old.goto(origin + "/sf/?at=embarcadero");
    await grounded(old);
    await old.waitForFunction(() => navigator.serviceWorker.controller);
    assert.match(await download(old), /^All /);
    await old.evaluate(
      ({ key, target }) => {
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
        );
      },
      { key, target },
    );
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
    const checkpointBefore = (await saved(old)).checkpoint;

    // Keep the old game tab open while its replacement prepares the new tile cache.
    revision = 1;
    await old.evaluate(async () => {
      const registration = await navigator.serviceWorker.getRegistration();
      const prepared = new Promise((resolve, reject) => {
        const timeout = setTimeout(
          () => reject(new Error("Candidate worker installation timed out")),
          30_000,
        );
        registration.addEventListener(
          "updatefound",
          () => {
            const worker = registration.installing;
            worker.addEventListener("statechange", () => {
              if (!["installed", "redundant"].includes(worker.state)) return;
              clearTimeout(timeout);
              if (worker.state === "installed") resolve();
              else reject(new Error("Candidate worker installation failed"));
            });
          },
          { once: true },
        );
      });
      await registration.update();
      await prepared;
    });
    const status = await inventory(old);
    assert.equal(
      status.find(({ name }) => name === `sf-tiles-/sf/-${versions[1]}`)?.count,
      [...tileHashes.keys()].filter((token) => previousTokens.has(token))
        .length,
    );
    await context.setOffline(true);
    const before = await cachedHashes(old, changed);
    for (const token of changed)
      assert.equal(before[token], payloadHash(roots[0], token), token);
    await context.setOffline(false);

    // This page is outside the game worker scope, so it can observe activation
    // without keeping the outgoing worker alive.
    const monitor = await context.newPage();
    await monitor.goto(origin + "/rpi.json");
    await monitor.evaluate(async () => {
      const { waiting } = await navigator.serviceWorker.getRegistration("/sf/");
      if (!waiting)
        throw new Error("The candidate did not wait for the old game tab");
      window.mapActivation = new Promise((resolve, reject) => {
        const timeout = setTimeout(
          () => reject(new Error("Candidate worker activation timed out")),
          30_000,
        );
        waiting.addEventListener("statechange", () => {
          if (!["activated", "redundant"].includes(waiting.state)) return;
          clearTimeout(timeout);
          if (waiting.state === "activated") resolve();
          else reject(new Error("Candidate worker activation failed"));
        });
      });
    });
    await old.close();
    await monitor.evaluate(() => window.mapActivation);
    await context.setOffline(true);
    const fresh = await context.newPage();
    await fresh.goto(origin + "/sf/?at=embarcadero");
    await grounded(fresh);
    assert.match(await fresh.locator("#progress").textContent(), /^1\//);
    assert.deepEqual((await saved(fresh)).checkpoint, checkpointBefore);
    const copied = [...tileHashes.keys()].filter((token) =>
      previousTokens.has(token),
    );
    const received = await cachedHashes(fresh, copied);
    for (const token of copied)
      assert.equal(received[token], tileHashes.get(token), token);

    await fresh.reload();
    await grounded(fresh);
    assert.ok((await saved(fresh)).collected.includes(target.t));
    assert.deepEqual((await saved(fresh)).checkpoint, checkpointBefore);
    const requested = requests
      .filter(
        ({ revision, pathname }) =>
          revision === 1 && pathname.endsWith(".json.gz"),
      )
      .map(({ pathname }) => path.basename(pathname, ".json.gz"));
    assert.deepEqual(requested.sort(), changed.sort());
    assert.deepEqual(errors, []);
    console.log(
      JSON.stringify({
        chromium: browser.version(),
        versions,
        tilesReadyOffline: copied.length,
        fetchedChangedTiles: changed.length,
        point: await state(fresh),
        caches: await inventory(fresh),
        pageErrors: errors,
      }),
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
