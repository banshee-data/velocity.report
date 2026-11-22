import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";

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

function fixture({ bodyError, limit = Infinity } = {}) {
  const requests = [];
  const saved = new Set([tokens.at(-1)]);
  let onMessage;
  runInNewContext(worker, {
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
