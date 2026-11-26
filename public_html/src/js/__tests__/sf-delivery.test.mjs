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
  driveOnCityRoute,
} from "../sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
  tileAt,
  tilesAround,
} from "../sf-drive-core.js";
import {
  createInsurance,
  advanceInsurance,
  insuranceRate,
  formatInsuranceCents,
  gameInsuranceLimit,
  recordCrash,
  speedMultiplier,
  visitSurvey,
} from "../sf-insurance.js";
import { highwayRailSegments, strikeHighwayRail } from "../sf-highway-edge.js";
import { indexRoadNetwork } from "../sf-road-network.js";
import { bridgeSurveyDrive, SOMA_SURVEY_STOPS } from "../sf-soma-drive.js";

test("insurance charges only manual time and accounts for both credit boundaries", () => {
  const run = createInsurance(true, ["a", "b"]);
  const car = { x: 0, y: 0, z: 7, grounded: true };
  const a = { id: "a", x: 0, y: 0 };
  const b = { ...a, id: "b" };
  assert.equal(visitSurvey(run, a, { ...car, z: 16 }, 7, 0), null);
  assert.equal(run.delivered, 0);
  assert.equal(run.collected.size, 0);
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
  assert.equal(formatInsuranceCents(insuranceRate(run)), "0.0005¢");
  assert.equal(run.spent, 0.0005);
  advanceInsurance(run, 20);
  assert.equal(formatInsuranceCents(insuranceRate(run)), "0.0010¢");
  assert.ok(Math.abs(run.spent - 0.011) < 1e-9);
  assert.equal(formatInsuranceCents(run.spent), "0.0110¢");
  assert.equal(formatInsuranceCents(0), "0.00¢");
  run.mode = "automatic";
  advanceInsurance(run, 100);
  assert.equal(run.manualSeconds, 31);
  assert.equal(insuranceRate(run), 0);
});

test("speeding doubles the manual rate per five mph game band and unknown limits use base", () => {
  const pavement = { grounded: true, z: 10 };
  assert.equal(gameInsuranceLimit(pavement, { kind: "bridge_deck" }), 50);
  assert.equal(gameInsuranceLimit(pavement, { kind: "ramp" }), 30);
  assert.equal(gameInsuranceLimit(pavement, null), 25);
  assert.equal(
    gameInsuranceLimit({ ...pavement, grounded: false }, null),
    null,
  );
  assert.equal(gameInsuranceLimit({ ...pavement, z: undefined }, null), null);
  for (const [mph, expected] of [
    [24, 1],
    [25, 1],
    [25.01, 2],
    [30, 2],
    [30.01, 4],
    [35, 4],
    [40, 8],
    [45, 16],
    [-45, 16],
    [45.01, 32],
  ])
    assert.equal(speedMultiplier(mph, 25), expected, `${mph} mph`);
  assert.equal(speedMultiplier(70, null), 1);

  const run = createInsurance(false);
  run.mode = "manual";
  const speedMps = (45 * 1609.344) / 3600;
  for (let tick = 0; tick < 120; tick++)
    advanceInsurance(run, 1 / 120, speedMps, 25);
  assert.ok(Math.abs(run.spent - 0.016) < 1e-10);
  assert.equal(insuranceRate(run), 0.016);
  assert.equal(formatInsuranceCents(insuranceRate(run)), "0.0160¢");
  assert.equal(run.limit, 25);
  advanceInsurance(run, 1, speedMps, null);
  assert.ok(Math.abs(run.spent - 0.017) < 1e-10);
  assert.equal(insuranceRate(run), 0.001);
});

test("one-off crash charges are proportional to jerk in manual mode", () => {
  const run = createInsurance(true);
  assert.equal(recordCrash(run, 2000), 0);
  assert.equal(run.spent, 0);
  run.mode = "manual";
  assert.equal(recordCrash(run, 1000), 0.01);
  assert.ok(Math.abs(recordCrash(run, 12000) - 0.12) < 1e-10);
  assert.ok(Math.abs(run.spent - 0.13) < 1e-10);
  assert.equal(run.infractions, 3);
  assert.equal(recordCrash(run, Infinity), 0);
});
test("the default Bay Bridge route completes every named delivery on the bundled city tiles", async () => {
  const files = new URL("../../sf/tiles/", import.meta.url);
  const manifest = JSON.parse(readFileSync(new URL("manifest.json", files)));
  const city = JSON.parse(
    readFileSync(new URL("../../city/sites.json", import.meta.url)),
  );
  const sites = sfSurveyData(await loadSurveys(), city, manifest);
  const published = bridgeSurveyDrive(
    JSON.parse(readFileSync(new URL("../gamedrives.json", files))).drives[0],
  );
  const cached = new Map();
  function tile(token) {
    if (!cached.has(token)) {
      const world = JSON.parse(
        gunzipSync(readFileSync(new URL(`${token}.json.gz`, files))),
      );
      applyCityDriveScenery(world, published.scene_exclusions?.[token]);
      cached.set(token, { world, index: buildCollisionIndex(world) });
    }
    return cached.get(token);
  }
  const height = (x, y) =>
    groundAtWorld([tile(tileAt(manifest.tiles, x, y).t).world], x, y, true);
  const drive = buildCityGameDrive(published, height);
  const graph = indexRoadNetwork(drive.topology.transitions);
  graph.add(drive.topology);
  const surfaces = new Map(
    drive.surfaces.map((surface) => [
      surface.id,
      { ...surface, railSegments: highwayRailSegments(surface) },
    ]),
  );
  const run = createInsurance(true, SOMA_SURVEY_STOPS);
  let car = {
    ...drive.start,
    v: drive.start.initialSpeed,
    grounded: true,
    z: surfaces
      .get(drive.start.surfaceId)
      .heightAt(drive.start.x, drive.start.y),
  };
  const spans = [car.surfaceId];
  const deliveries = [];
  assert.ok(car.v >= 22);
  let finished = false;
  let finishTick = 0;
  let collisionIndexes = [];
  const surveyHeights = new Map(
    sites.map((site) => [site.id, height(site.x, site.y)]),
  );
  for (let i = 0; i < 600 * 120; i++) {
    if (i % 25 === 0)
      collisionIndexes = tilesAround(manifest.tiles, car.x, car.y, 50).want.map(
        (token) => tile(token).index,
      );
    const controls = cityDriveControls(drive, car);
    if (controls.finished) {
      finished = true;
      finishTick = i;
      break;
    }
    const surface = surfaces.get(car.surfaceId);
    const nextCar = driveOnCityRoute(
      car,
      controls,
      1 / 120,
      surface,
      surfaces,
      graph,
      height,
      undefined,
      true,
    );
    if (nextCar.surfaceId !== car.surfaceId) spans.push(nextCar.surfaceId);
    car = nextCar;
    assert.ok(
      car.grounded && !car.blocked && !car.roadImpact,
      `Contact at ${car.surfaceId} ${car.x},${car.y} at tick ${i}`,
    );
    assert.equal(
      collisionAt(collisionIndexes, car, car.z),
      null,
      `Collision at ${car.surfaceId} ${car.x},${car.y} at tick ${i}`,
    );
    for (const [index, site] of sites.entries()) {
      const before = run.delivered;
      visitSurvey(run, site, car, surveyHeights.get(site.id), index);
      if (run.delivered > before) {
        assert.equal(surfaces.get(car.surfaceId).kind, "ground");
        deliveries.push(site.id);
      }
    }
  }
  assert.ok(finished, JSON.stringify({ car, spans, deliveries }));
  assert.deepEqual(spans, [
    "bay-bridge-westbound",
    "city-approach",
    "fremont-exit",
    "survey-streets-outbound",
    "survey-streets-return",
    "fourth-street-entrance",
    "westbound-i80",
  ]);
  assert.deepEqual(deliveries, SOMA_SURVEY_STOPS);
  assert.equal(run.delivered, SOMA_SURVEY_STOPS.length);
  assert.equal(run.spent, 0);
  assert.equal(surfaces.get(car.surfaceId).kind, "bridge_deck");
  console.log(
    `Bridge survey replay ${(finishTick / 120).toFixed(2)} s: ${deliveries.join(" → ")} → ${car.surfaceId}`,
  );
});
