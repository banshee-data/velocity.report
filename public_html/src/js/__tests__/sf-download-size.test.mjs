import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { gzipSync } from "node:zlib";
import { gzipBytes, sfDownloadAssets } from "../../../sf-asset-sizes.cjs";
import { createDownloadSizeTracker } from "../sf-download-size.js";

const origin = "https://example.test";
const tiles = [
  { t: "a", bytes: 400 },
  { t: "b", bytes: 600 },
];
const assets = [
  { base: "tiles", url: "manifest.json", bytes: 100 },
  { base: "modules", url: "./sf-drive.js", bytes: 200 },
];
const tracker = (prefix = "") =>
  createDownloadSizeTracker({
    tiles,
    assets,
    tileBase: `${origin}${prefix}/sf/tiles/`,
    moduleBase: `${origin}${prefix}/modules/version/js/sf-download-size.js`,
  });

test("build estimates gzip text and retain already-gzipped tile sizes", () => {
  const text = Buffer.from("a repeated script statement;\n".repeat(1000));
  const encoded = gzipSync(text, { level: 9 });
  assert.equal(gzipBytes(text), encoded.length);
  assert.equal(
    gzipBytes(encoded),
    encoded.length,
    "do not gzip tile payloads twice",
  );
  assert.ok(gzipBytes(text) < text.length / 10);
});

test("game download fallback counts compressed renderer files rather than their source sizes", () => {
  const entries = sfDownloadAssets();
  const core = entries.find((e) => e.url.endsWith("three.core.js"));
  const source = fs.readFileSync(
    new URL("../../../node_modules/three/build/three.core.js", import.meta.url),
  );
  assert.equal(core.bytes, gzipSync(source, { level: 9 }).length);
  assert.ok(core.bytes < source.length / 3);
  assert.ok(
    entries.some((e) => e.url === "./sf-download-size.js"),
    "include the accounting module itself",
  );
});

test("encoded response bodies replace estimates, excluding headers and expanded bytes", () => {
  const download = tracker();
  assert.deepEqual(download.sizes(["a"]), {
    total: 1300,
    tiles: 1000,
    resident: 400,
    estimated: true,
  });
  download.observe([
    {
      name: `${origin}/sf/tiles/manifest.json`,
      encodedBodySize: 75,
      decodedBodySize: 750,
      transferSize: 375,
    },
    {
      name: `${origin}/modules/version/js/sf-drive.js`,
      encodedBodySize: 25,
      decodedBodySize: 200,
      transferSize: 325,
    },
    { name: `${origin}/unrelated.js`, encodedBodySize: 9000 },
  ]);
  assert.deepEqual(download.sizes(["a"]), {
    total: 1100,
    tiles: 1000,
    resident: 400,
    estimated: false,
  });
});

test("cache hits retain full download sizes, and missing timing data keeps the marked estimate", () => {
  const download = tracker();
  download.observe([
    {
      name: `${origin}/sf/tiles/manifest.json`,
      encodedBodySize: 75,
      decodedBodySize: 750,
      transferSize: 0,
    },
    {
      name: `${origin}/modules/version/js/sf-drive.js`,
      encodedBodySize: 0,
      decodedBodySize: 0,
      transferSize: 0,
    },
  ]);
  assert.deepEqual(download.sizes([]), {
    total: 1275,
    tiles: 1000,
    resident: 0,
    estimated: true,
  });
  download.observe([
    {
      name: `${origin}/modules/version/js/sf-drive.js`,
      encodedBodySize: 25,
      transferSize: 0,
    },
  ]);
  assert.equal(download.sizes([]).total, 1100);
  assert.equal(download.sizes([]).estimated, false);
  download.observe([
    { name: `${origin}/modules/version/js/sf-drive.js`, encodedBodySize: 0 },
  ]);
  assert.equal(
    download.sizes([]).total,
    1100,
    "unavailable timing must not erase a measurement",
  );
});

test("tile retries and unloads do not inflate or erase the complete download inventory", () => {
  const download = tracker();
  const entry = { name: `${origin}/sf/tiles/a.json.gz`, encodedBodySize: 350 };
  download.observe([entry, entry]);
  assert.deepEqual(download.sizes(["a"]), {
    total: 1250,
    tiles: 950,
    resident: 350,
    estimated: true,
  });
  assert.deepEqual(download.sizes([]), {
    total: 1250,
    tiles: 950,
    resident: 0,
    estimated: true,
  });
});

test("error pages and partial responses do not replace complete asset download sizes", () => {
  const download = tracker();
  download.observe([
    {
      name: `${origin}/sf/tiles/a.json.gz`,
      encodedBodySize: 15,
      responseStatus: 404,
    },
    {
      name: `${origin}/modules/version/js/sf-drive.js`,
      encodedBodySize: 20,
      responseStatus: 206,
    },
  ]);
  assert.equal(download.sizes([]).total, 1300);
});

test("offline path prefixes resolve both tile and module response URLs", () => {
  const download = tracker("/public_html");
  download.observe([
    {
      name: `${origin}/public_html/sf/tiles/manifest.json`,
      encodedBodySize: 50,
    },
    {
      name: `${origin}/public_html/modules/version/js/sf-drive.js`,
      encodedBodySize: 150,
    },
  ]);
  assert.deepEqual(download.sizes(["b"]), {
    total: 1200,
    tiles: 1000,
    resident: 600,
    estimated: false,
  });
});
