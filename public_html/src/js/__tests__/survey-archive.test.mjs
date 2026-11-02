import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { archiveStatus, fetchAll } from "../survey-archive.js";

const SurveyArchivePages = createRequire(import.meta.url)(
  "../../scenes/survey-archive-pages.11ty.js",
);

const origFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = origFetch;
});

const body = () => ({ ok: true, arrayBuffer: async () => new ArrayBuffer(0) });

test("every file is fetched and read to the end", async () => {
  const read = [];
  globalThis.fetch = async (url) => ({
    ok: true,
    arrayBuffer: async () => {
      read.push(url);
      return new ArrayBuffer(0);
    },
  });
  const urls = ["a", "b", "c", "d", "e", "f", "g", "h"];
  const results = [];
  await fetchAll(urls, (url, fetched) => results.push([url, fetched]), 3);
  assert.deepEqual(read.sort(), urls);
  assert.deepEqual(results.map(([url]) => url).sort(), urls);
  assert.ok(results.every(([, fetched]) => fetched));
});

test("no more than the limit are in flight at once", async () => {
  let inFlight = 0;
  let most = 0;
  globalThis.fetch = async () => {
    most = Math.max(most, ++inFlight);
    await new Promise((resolve) => setTimeout(resolve, 1));
    inFlight--;
    return body();
  };
  await fetchAll(["a", "b", "c", "d", "e", "f", "g"], () => {}, 3);
  assert.equal(most, 3);
});

test("a failed file is reported and the rest carry on", async () => {
  globalThis.fetch = async (url) => {
    if (url === "gone") return { ok: false };
    if (url === "unreachable") throw new TypeError("network");
    return body();
  };
  const results = new Map();
  await fetchAll(["gone", "unreachable", "ok"], (url, fetched) =>
    results.set(url, fetched),
  );
  assert.deepEqual(Object.fromEntries(results), {
    gone: false,
    unreachable: false,
    ok: true,
  });
});

test("the status says how far it got and what failed", () => {
  assert.equal(archiveStatus(3, 0, 26), "Fetched 3 of 26 files…");
  assert.equal(archiveStatus(26, 0, 26), "All 26 files fetched.");
  assert.equal(
    archiveStatus(26, 2, 26),
    "24 of 26 files fetched. 2 could not be.",
  );
});

test("each published survey with playback has an archive page", () => {
  const { pagination, permalink } = new SurveyArchivePages().data();
  const playback = { hasPlayback: true };
  const sites = [
    { id: "a", published: true, recordings: [playback] },
    { id: "b", published: false, recordings: [playback] },
    { id: "c", published: true, recordings: [{ hasPlayback: false }] },
  ];
  assert.deepEqual(
    pagination.before(sites).map((site) => site.id),
    ["a"],
  );
  assert.equal(
    permalink({ survey: { id: "a" } }),
    "/surveys/a/archive/index.html",
  );
});

test("the survey page links its archive page rather than each file", () => {
  const provenance = readFileSync(
    new URL("../../_includes/survey/provenance.njk", import.meta.url),
    "utf8",
  );
  assert.match(provenance, /page\.url \+ "archive\/"/);
  assert.doesNotMatch(provenance, /in rec\.files/);
});
