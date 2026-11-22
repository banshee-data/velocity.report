import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { test } from "node:test";
import { contactGeometry } from "../sf-road-contact.js";
import { loadRoadOverlay } from "../sf-drive-assets.js";

const digest = (value) => createHash("sha256").update(value).digest("hex");

test("the overlay uses the SF frame and leaves missing contact as a gap", () => {
  const geometry = contactGeometry({
    vertices: [
      [4, 5, 6],
      [7, 8, 9],
      [10, 11, 12],
      [13, 14, 15],
      [16, 17, null],
    ],
    support: ["observed", "observed", "observed", "interpolated", "missing"],
    triangles: [
      [0, 1, 2],
      [2, 1, 3],
      [1, 3, 4],
    ],
  });
  const positions = geometry.getAttribute("position");
  assert.equal(positions.count, 6);
  assert.deepEqual(Array.from(positions.array.slice(0, 3)), [4, 6, -5]);
  assert.deepEqual(geometry.groups, [
    { start: 0, count: 3, materialIndex: 0 },
    { start: 3, count: 3, materialIndex: 1 },
  ]);
  geometry.dispose();
});

test("the viewer binds the remote tile to the raw game manifest and checks its hash", async (t) => {
  const origin = [48881.186, 25109.973];
  const base = JSON.stringify({ origin_epsg7131: origin, tiles: [] });
  const tile = JSON.stringify({ surfaces: [] });
  const revisionUrl = new URL(
    "http://gerardus.invalid/city-tiles/revision/manifest.json",
  );
  const baseUrl = new URL("http://localhost:8090/sf/tiles/manifest.json");
  const tileUrl = new URL("80858074.json", revisionUrl);
  const overlay = {
    compatibility_version: 1,
    base_manifest_sha256: digest(base),
    horizontal_crs: "EPSG:7131",
    vertical_crs: "EPSG:5703",
    units: "m",
    origin_epsg7131: origin,
    tiles: { 80858074: { sha256: digest(tile), bytes: tile.length } },
  };
  const responses = new Map([
    [baseUrl.href, base],
    [revisionUrl.href, JSON.stringify(overlay)],
    [tileUrl.href, tile],
  ]);
  const fetch = t.mock.method(
    globalThis,
    "fetch",
    async (url) => new Response(responses.get(url.href)),
  );
  const loaded = await loadRoadOverlay(revisionUrl, baseUrl);
  assert.deepEqual(loaded.tiles.get("80858074"), { surfaces: [] });
  responses.set(tileUrl.href, tile.replace("surfaces", "altered!"));
  await assert.rejects(
    loadRoadOverlay(revisionUrl, baseUrl),
    /Tile 80858074 failed verification/,
  );
  responses.set(baseUrl.href, `${base}\n`);
  fetch.mock.resetCalls();
  await assert.rejects(
    loadRoadOverlay(revisionUrl, baseUrl),
    /does not match the SF game manifest/,
  );
  assert.equal(fetch.mock.callCount(), 2);
});
