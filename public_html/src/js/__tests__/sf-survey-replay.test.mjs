import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { sfSurveyData } from "../../../sf-survey-data.cjs";
import { loadSurveys } from "../../_data/surveys.js";
import { ChunkUnavailableError } from "../scene-reader.js";
import {
  createSurveyReplay,
  nearbySurvey,
  projectSurveyTrack,
} from "../sf-survey-replay.js";

function json(path) {
  return JSON.parse(readFileSync(new URL(path, import.meta.url), "utf8"));
}

test("joins eligible game sites to the placement, histogram and link of the same recording", async () => {
  const city = json("../../city/sites.json");
  const manifest = json("../../sf/tiles/manifest.json");
  const surveys = await loadSurveys();
  const sites = sfSurveyData(surveys, city, manifest);
  assert.equal(sites.length, city.placed_count);
  assert.ok(!sites.some((site) => site.id === "soma1"));

  const site = sites.find(({ id }) => id === "embarcadero-folsom");
  const survey = surveys.byId[site.id];
  const [capture] = survey.recordings;
  assert.equal(site.manifest, `${capture.assetsUrl.slice(1)}/manifest.json`);
  assert.equal(site.page, `${survey.page.slice(1)}?recording=${capture.id}`);
  assert.equal(site.speed.count, capture.speed.count);
  assert.deepEqual(site.speed.bins, capture.speed.bins);
  assert.equal(site.limit, survey.speedLimitMph);

  // Measurements independently projected through city_frame.py and PROJ
  // 9.5 (6318 -> 7131), using the drive manifest's rounded origin.
  const cases = [
    [site, [120, -75, -1], [4436.9691, 3297.0667, 4.6423]],
    [
      sites.find(({ id }) => id === "ashbury-downey"),
      [-60, 100, 1],
      [-503.8259, 328.5866, 133.6917],
    ],
  ];
  for (const [placed, [x, y, z], expected] of cases) {
    const { anchor } = projectSurveyTrack(placed.transform, { x, y, z });
    const error = Math.hypot(...anchor.map((v, i) => v - expected[i]));
    assert.ok(error < 0.01, `local projection differs by ${error} m`);
  }

  // A more recent playback may have a different sensor pose.
  survey.recordings.unshift({
    ...capture,
    id: "later-visit",
    assetsUrl: `${survey.page}recordings/later-visit`,
  });
  const joined = sfSurveyData(surveys, city, manifest);
  assert.equal(joined.find(({ id }) => id === site.id).page, site.page);
});

const a = { id: "a", x: 0, y: 0 };
const b = { id: "b", x: 180, y: 0 };
const point = (x) => ({ x, y: 0 });
const turn = () => new Promise(setImmediate);

test("keeps one nearby recording, with hysteresis both at the edge and between sites", () => {
  assert.equal(nearbySurvey([a, b], point(-116)), null);
  assert.equal(nearbySurvey([a, b], point(-115)), a);
  assert.equal(nearbySurvey([a, b], point(-140), a), a);
  assert.equal(nearbySurvey([a, b], point(-146), a), null);
  assert.equal(nearbySurvey([a, b], point(94), a), a);
  assert.equal(nearbySurvey([a, b], point(101), a), b);
});

test("shows the nearest survey ahead before playback, then clears the cue on entry or exit from city mode", () => {
  const surveyed = [];
  const notices = [];
  const replay = createSurveyReplay({
    sites: [{ id: "between", x: 80, y: 0 }, a, b],
    open(site) {
      surveyed.push(site);
      return session([], site.id);
    },
    onSite() {},
    onStatus() {},
    onFrame() {},
    onApproach(notice) {
      notices.push(notice);
    },
  });
  const facingEast = (x) => ({ ...point(x), heading: 0 });
  replay.update(facingEast(-251), 0);
  assert.equal(notices.at(-1), null);
  replay.update({ ...facingEast(-180), heading: Math.PI }, 0);
  assert.equal(notices.at(-1), null);
  replay.update(facingEast(-180), 0);
  assert.deepEqual(notices.at(-1), { site: a, distance: 180 });
  assert.deepEqual(surveyed, []);
  replay.update(facingEast(-120), 0);
  assert.deepEqual(notices.at(-1), { site: a, distance: 120 });
  replay.update(facingEast(-115), 0);
  assert.equal(notices.at(-1), null);
  assert.deepEqual(surveyed, [a]);
  replay.update(facingEast(-150), 0);
  assert.deepEqual(notices.at(-1), { site: a, distance: 150 });
  replay.update(facingEast(-241), 0);
  assert.deepEqual(notices.at(-1), { site: a, distance: 241 });
  replay.update(null, 0);
  assert.equal(notices.at(-1), null);
});

test("projects the measured box centre and rotates the recorded heading into the city grid", () => {
  const transform = [
    [0, -1, 0, 25],
    [1, 0, 0, 15],
    [0, 0, 1, 12],
  ];
  assert.deepEqual(
    projectSurveyTrack(transform, { x: 1, y: 2, bx: 3, by: 5, z: -1, bh: 0 }),
    { anchor: [20, 18, 11], yaw: Math.PI / 2 },
  );
});

function session(samples, id) {
  return {
    duration: 1,
    parts: [{ header: {} }],
    async frameAt(seconds) {
      samples.push([id, seconds]);
      return { partIndex: 0, frame: { t: seconds * 1e6, tr: [id] } };
    },
  };
}

test("opens on approach, loops at the recorded duration, and discards late frames after departure", async () => {
  const opens = [],
    frames = [],
    samples = [];
  let completeA;
  const replay = createSurveyReplay({
    sites: [a, b],
    open(site) {
      opens.push(site.id);
      return site === a
        ? new Promise((resolve) => {
            completeA = resolve;
          })
        : session(samples, site.id);
    },
    onSite() {},
    onStatus() {},
    onFrame(site, tracks) {
      frames.push([site?.id, tracks]);
    },
  });
  replay.update(point(-500), 0);
  assert.deepEqual(opens, []);
  replay.update(point(0), 0);
  replay.update(point(180), 0);
  await turn();
  completeA(session(samples, "a"));
  await turn();
  assert.deepEqual(opens, ["a", "b"]);
  assert.deepEqual(samples, [["b", 0]]);
  replay.update(point(180), 0.75);
  await turn();
  replay.update(point(180), 0.5);
  await turn();
  assert.deepEqual(samples, [
    ["b", 0],
    ["b", 0.75],
    ["b", 0.25],
  ]);
  assert.deepEqual(frames.slice(-2), [
    ["b", []],
    ["b", ["b"]],
  ]);
  replay.update(null, 0);
  assert.deepEqual(frames.at(-1), [undefined, []]);
});

test("skips missing chunks and keeps the linked survey visible when all replay is unavailable", async () => {
  const sites = [],
    statuses = [],
    samples = [];
  const replay = createSurveyReplay({
    sites: [a],
    open: () => ({
      duration: 1,
      playableFrom: (seconds) =>
        seconds === 0 && samples.length === 1 ? 0.3 : null,
      async frameAt(seconds) {
        samples.push(seconds);
        throw new ChunkUnavailableError("Missing test chunk");
      },
    }),
    onSite: (site) => sites.push(site),
    onStatus: (status) => statuses.push(status),
    onFrame() {},
  });
  replay.update(point(0), 0);
  await turn();
  replay.update(point(0), 0.1);
  await turn();
  assert.deepEqual(samples, [0, 0.3]);
  assert.deepEqual(sites, [a]);
  assert.equal(statuses.at(-1), "Replay unavailable");
});
