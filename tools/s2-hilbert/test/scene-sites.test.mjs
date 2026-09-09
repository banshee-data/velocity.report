import assert from "node:assert/strict";
import test from "node:test";

import { buildSceneSites, matchSite, sceneStartMs } from "../scene-sites.mjs";

const INDEX = [
  {
    site: "s10",
    start: "2026-09-02T10:58:30.258816-07:00",
    where: "Union Street near Van Ness",
    lat: 37.7985,
    lon: -122.424,
  },
  {
    site: "s14",
    start: "2026-09-02T13:37:20.000000-07:00",
    where: null,
    lat: null,
    lon: null,
  },
  {
    site: "s23",
    start: "2026-09-03T15:50:43.000000-07:00",
    where: "SoMa, Mission near 8th",
    lat: 37.779,
    lon: -122.413,
  },
];

const at = (iso, durationSec = 1200, id = "scene", title = "scene") => ({
  id,
  title,
  startMs: Date.parse(iso),
  durationSec,
});

test("a scene start_ns survives the conversion without losing its clock", () => {
  // The value exceeds 2^53 nanoseconds, which is why the header stores a string.
  assert.equal(
    sceneStartMs({ start_ns: "1788371950124819330" }),
    1788371950124,
  );
});

test("a scene joins the site whose static period it opens in", () => {
  const joined = matchSite(at("2026-09-02T10:59:10-07:00"), INDEX);
  assert.equal(joined.site.site, "s10");
  // The export skips the opening frames while the background converges, so it
  // starts a little after the site does.
  assert.ok(joined.gapSeconds > 0 && joined.gapSeconds < 60);
});

test("a recording made off the archive volume joins nothing", () => {
  assert.equal(matchSite(at("2025-12-06T13:42:21-08:00"), INDEX), null);
});

test("a scene takes its position from the site, so correcting a mark moves it", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2026-09-02T10:59:10-07:00", 2004, "s2-sf-2")],
    index: INDEX,
  });
  const [scene] = sites;
  assert.equal(scene.archive_site, "s10");
  assert.deepEqual(scene.position, {
    lat: 37.7985,
    lon: -122.424,
    source: "operator",
  });
  assert.equal(scene.position_source, "archive-index");
  assert.match(scene.summary, /^33 minutes/);
  assert.equal(scene.page, "/scenes/s2-sf-2/");
});

test("an override beats the site, so a survey is not overwritten by a map reading", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2026-09-03T15:51:23-07:00", 1164, "s2-sf-8")],
    index: INDEX,
    overrides: {
      "s2-sf-8": {
        title: "Mission & 8th",
        position: { lat: 37.7791, lon: -122.4128, source: "surveyed" },
        position_note: "Measured.",
      },
    },
  });
  const [scene] = sites;
  assert.equal(
    scene.archive_site,
    "s23",
    "the join still records which site it is",
  );
  assert.deepEqual(scene.position, {
    lat: 37.7791,
    lon: -122.4128,
    source: "surveyed",
  });
  assert.equal(scene.position_source, "override");
  assert.equal(scene.title, "Mission & 8th");
});

test("a site whose mark could not be read leaves the scene unpositioned, not guessed", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2026-09-02T13:41:32-07:00", 780, "s2-sf-3")],
    index: INDEX,
  });
  const [scene] = sites;
  assert.equal(scene.archive_site, "s14");
  assert.equal(scene.position, null);
  assert.match(scene.position_note, /could not be read/);
});

test("a scene with no site at all is unsurveyed rather than unlocated", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2025-12-06T13:42:21-08:00", 662, "soma1")],
    index: INDEX,
  });
  assert.equal(sites[0].archive_site, null);
  assert.match(sites[0].position_note, /Not surveyed/);
});

test("a scene is named for its place once it is joined, not for the capture prefix", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2026-09-02T10:59:10-07:00", 2004, "s2-sf-2", "s2-sf-2")],
    index: INDEX,
  });
  assert.equal(sites[0].title, "Union Street near Van Ness");
});

test("a scene whose site has no readable mark keeps the prefix rather than inventing a name", () => {
  const { sites } = buildSceneSites({
    scenes: [at("2026-09-02T13:41:32-07:00", 780, "s2-sf-3", "s2-sf-3")],
    index: INDEX,
  });
  assert.equal(sites[0].title, "s2-sf-3");
});
