import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";
import { gzipSync } from "node:zlib";

const scope = "https://example.test/sf/";
const tokens = Array.from({ length: 12 }, (_, i) => String(i).padStart(8, "0"));
const manifest = { tiles: tokens.map((t) => ({ t })) };
const worker = readFileSync(
  new URL("../../sf/offline.njk", import.meta.url),
  "utf8",
)
  .replace(/^---\n[\s\S]*?\n---\n/, "")
  .replace("{{ (modulesBase + '/js/') | url }}", "/modules/test/js/")
  .replace("{{ sfTileVersion }}", "test")
  .replace("{{ sfDownloadAssets | safe }}", "[]");
const workerWithHashes = (entries = {}) =>
  worker.replace("{{ sfTileHashes | safe }}", JSON.stringify(entries));

function fixture({ bodyError, limit = Infinity } = {}) {
  const requests = [];
  const saved = new Set([tokens.at(-1)]);
  let onMessage;
  runInNewContext(workerWithHashes(), {
    URL,
    AbortSignal,
    self: {
      registration: { scope },
      addEventListener(type, handler) {
        if (type === "message") onMessage = handler;
      },
    },
    caches: {
      async open(name) {
        return name.startsWith("sf-shell-")
          ? { match: async () => Response.json(manifest) }
          : {
              async match(url) {
                if (saved.has(tokenOf(url))) return new Response("cached");
              },
              async put(url, response) {
                await response.arrayBuffer();
                if (saved.size >= limit)
                  throw new DOMException("Cache is full", "QuotaExceededError");
                saved.add(tokenOf(url));
              },
            };
      },
    },
    async fetch(url) {
      const token = tokenOf(url);
      requests.push(token);
      if (token === tokens[0] && bodyError) {
        const error = bodyError;
        bodyError = undefined;
        // HTTP 200 resolves before reading the body encounters the error.
        const stream = new ReadableStream({
          start(controller) {
            controller.enqueue(new Uint8Array([1]));
          },
          pull(controller) {
            controller.error(error);
          },
        });
        return new Response(stream);
      }
      return new Response("complete");
    },
  });

  function tokenOf(url) {
    return url.pathname.split("/").at(-1).split(".")[0];
  }

  return {
    requests,
    saved,
    async download() {
      const messages = [];
      let done;
      onMessage({
        data: { type: "download-all-tiles" },
        ports: [
          {
            postMessage(message) {
              messages.push(structuredClone(message));
            },
            close() {},
          },
        ],
        waitUntil(promise) {
          done = promise;
        },
      });
      await done;
      return messages.at(-1);
    },
    freeStorage() {
      limit = Infinity;
    },
  };
}

for (const error of [
  new TypeError("Connection dropped"),
  new DOMException("Timed out", "AbortError"),
]) {
  test(`a response body ${error.name} leaves later tiles running and retries only the missing tile`, async () => {
    const { download, requests, saved } = fixture({ bodyError: error });
    assert.deepEqual(await download(), {
      type: "complete",
      saved: 11,
      total: 12,
      storageError: false,
    });
    assert.deepEqual([...requests].sort(), tokens.slice(0, -1));
    assert.deepEqual([...saved].sort(), tokens.slice(1));

    requests.length = 0;
    assert.deepEqual(await download(), {
      type: "complete",
      saved: 12,
      total: 12,
      storageError: false,
    });
    assert.deepEqual(requests, [tokens[0]]);
  });
}

test("a real cache quota error stops new transfers and retains cached tiles for retry", async () => {
  const { download, requests, saved, freeStorage } = fixture({ limit: 3 });
  assert.deepEqual(await download(), {
    type: "complete",
    saved: 3,
    total: 12,
    storageError: true,
  });
  assert.ok(requests.length < tokens.length - 1);
  assert.ok(
    saved.has(tokens.at(-1)),
    "counts cached tiles after the quota error",
  );
  const missing = tokens.filter((t) => !saved.has(t));
  requests.length = 0;
  freeStorage();
  assert.deepEqual(await download(), {
    type: "complete",
    saved: 12,
    total: 12,
    storageError: false,
  });
  assert.deepEqual([...requests].sort(), missing);
});

test("a map upgrade reuses matching offline tiles and fetches only changed downloaded tiles", async () => {
  const [same, changed, stale, untouched] = tokens;
  const published = {
    [same]: '{"road":"retained"}',
    [changed]: '{"road":"new join"}',
    [stale]: '{"road":"replacement"}',
    [untouched]: '{"road":"new coverage"}',
  };
  const hashes = Object.fromEntries(
    Object.entries(published).map(([token, json]) => [
      token,
      createHash("sha256").update(json).digest("hex"),
    ]),
  );
  const stored = new Map();
  const older = new Map([
    [same, new Response(gzipSync(published[same]))],
    [changed, new Response('{"road":"old join"}')],
    [stale, new Response('{"road":"old pavement"}')],
  ]);
  const tokenOf = (url) => url.pathname.split("/").at(-1).slice(0, 8);
  const requests = [];
  let install;
  runInNewContext(workerWithHashes(hashes), {
    AbortSignal,
    Blob,
    DecompressionStream,
    Request,
    Response,
    URL,
    crypto: globalThis.crypto,
    self: {
      registration: { scope },
      addEventListener(type, handler) {
        if (type === "install") install = handler;
      },
    },
    caches: {
      async keys() {
        return [
          "sf-tiles-/sf/-older",
          "sf-tiles-/other/-ignored",
          "sf-tiles-/sf/-test",
        ];
      },
      async open(name) {
        if (name.startsWith("sf-shell-")) return { async addAll() {} };
        const revision = name.endsWith("-test") ? stored : older;
        return {
          async match(url) {
            return revision.get(tokenOf(url))?.clone();
          },
          async put(url, response) {
            revision.set(tokenOf(url), response);
          },
        };
      },
    },
    async fetch(url, options) {
      const token = tokenOf(url);
      assert.equal(options.cache, "reload");
      requests.push(token);
      return new Response(
        token === stale ? "stale network response" : published[token],
      );
    },
  });
  let done;
  install({
    waitUntil(promise) {
      done = promise;
    },
  });
  await assert.rejects(done, /offline download could not be carried forward/);
  assert.deepEqual([...requests].sort(), [changed, stale]);
  assert.deepEqual([...stored.keys()].sort(), [same, changed]);
  assert.equal(await stored.get(changed).text(), published[changed]);
  assert.equal(await older.get(changed).text(), '{"road":"old join"}');
});
