import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { gunzipSync } from "node:zlib";
import { sfSurveyData } from "../../../sf-survey-data.cjs";
import { loadSurveys } from "../../_data/surveys.js";
import {
  buildCityGameDrive,
  applyCityDriveScenery,
  cityDriveControls,
} from "../sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
  tileAt,
} from "../sf-drive-core.js";
import {
  createInsurance,
  advanceInsurance,
  insuranceRate,
  visitSurvey,
} from "../sf-insurance.js";
import { driveOnRoadNetwork, indexRoadNetwork } from "../sf-road-network.js";
import { somaSurveyDrive, SOMA_SURVEY_STOPS } from "../sf-soma-drive.js";

test("insurance charges only manual time and accounts for both credit boundaries", () => {
  const run = createInsurance(true, ["a", "b"]);
  const car = { x: 0, y: 0, z: 7, grounded: true };
  const a = { id: "a", x: 0, y: 0 };
  const b = { ...a, id: "b" };
  assert.equal(visitSurvey(run, a, { ...car, z: 16 }, 7, 0), null);
  assert.equal(visitSurvey(run, a, { ...car, grounded: false }, 7, 0), null);
  assert.match(visitSurvey(run, a, car, 7, 0), /Free insurance/);
  assert.equal(visitSurvey(run, a, car, 7, 0), null);
  assert.match(visitSurvey(run, b, car, 7, 1), /Half-rate insurance/);
  advanceInsurance(run, 100);
  assert.equal(run.delivered, 2);
  assert.equal(run.manualSeconds, 0);
  assert.equal(run.free, 10);
  assert.equal(run.discount, 20);
  run.mode = "manual";
  advanceInsurance(run, 11);
  assert.equal(insuranceRate(run), 0.06);
  assert.equal(run.spent, 0.06);
  advanceInsurance(run, 20);
  assert.equal(insuranceRate(run), 0.12);
  assert.ok(Math.abs(run.spent - 1.32) < 1e-9);
  run.mode = "automatic";
  advanceInsurance(run, 100);
  assert.equal(run.manualSeconds, 31);
  assert.equal(insuranceRate(run), 0);
});

test("the default SoMa route completes every named delivery on the bundled city tiles", async () => {
  const files = new URL("../../sf/tiles/", import.meta.url);
  const manifest = JSON.parse(readFileSync(new URL("manifest.json", files)));
  const city = JSON.parse(
    readFileSync(new URL("../../city/sites.json", import.meta.url)),
  );
  const sites = sfSurveyData(await loadSurveys(), city, manifest);
  const published = somaSurveyDrive();
  const cached = new Map();
  function tile(x, y) {
    const token = tileAt(manifest.tiles, x, y).t;
    if (!cached.has(token)) {
      const world = JSON.parse(
        gunzipSync(readFileSync(new URL(`${token}.json.gz`, files))),
      );
      applyCityDriveScenery(world, published.scene_exclusions[token]);
      cached.set(token, { world, index: buildCollisionIndex(world) });
    }
    return cached.get(token);
  }
  const height = (x, y) => groundAtWorld([tile(x, y).world], x, y, true);
  const drive = buildCityGameDrive(published, height);
  const graph = indexRoadNetwork(drive.topology.transitions);
  graph.add(drive.topology);
  const surfaces = new Map(
    drive.surfaces.map((surface) => [surface.id, surface]),
  );
  const run = createInsurance(true, SOMA_SURVEY_STOPS);
  let car = {
    ...drive.start,
    v: 0,
    grounded: true,
    z: height(drive.start.x, drive.start.y),
  };
  let finished = false;
  for (let i = 0; i < 360 * 120; i++) {
    const controls = cityDriveControls(drive, car);
    if (controls.finished) {
      finished = true;
      break;
    }
    car = driveOnRoadNetwork(
      car,
      controls,
      1 / 120,
      surfaces.get(car.surfaceId),
      surfaces,
      graph,
      () => false,
    );
    assert.ok(car.grounded && !car.blocked);
    assert.equal(collisionAt([tile(car.x, car.y).index], car, car.z), null);
    const next = sites.find(({ id }) => run.stops[run.delivered] === id);
    if (next) visitSurvey(run, next, car, car.z, sites.indexOf(next));
  }
  assert.ok(finished);
  assert.equal(run.delivered, SOMA_SURVEY_STOPS.length);
  assert.equal(run.spent, 0);
});
