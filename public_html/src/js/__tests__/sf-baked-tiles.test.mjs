import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { gunzipSync } from "node:zlib";
import { Raycaster, Vector3 } from "../../vendor/three/three.module.js";
import { buildBakedRoadContact } from "../sf-baked-road-contact.js";
import {
  BAY_BRIDGE_CITY_DRIVE,
  buildCityGameDrive,
  buildRaisedRoad,
  cityDriveApproach,
} from "../sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  contactStep,
  groundAtWorld,
  tileAt,
} from "../sf-drive-core.js";
import {
  coveredRoadBlend,
  driveCameraPose,
  overheadDeckBlend,
} from "../sf-drive-camera.js";
import {
  highwayRailSegments,
  leaveRoadSurface,
  strikeHighwayRail,
} from "../sf-highway-edge.js";
import { driveThroughRoadPortal } from "../sf-road-portal.js";
import {
  contactGeometry,
  driveOnRoadSurface,
  roadSupportsCar,
} from "../sf-road-contact.js";
import { driveOnRoadNetwork, indexRoadNetwork } from "../sf-road-network.js";
import { deckClearanceIndex } from "../sf-tree-clearance.js";
import { tunnelMesh } from "../sf-tunnel-mesh.js";

const path = (file) => new URL(`../../sf/${file}`, import.meta.url);
const file = (name) => readFileSync(path(name));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

function assertRampWidthReceipt(roadBake, token, previous, raw) {
  const review =
    roadBake.bryantEastboundWidthSurvey.tiles[token] ??
    roadBake.sixthNorthExitWidthSurvey.tiles[token];
  if (review) assert.equal(review.parentSha256, previous);
  assertLaterRoadTileReceipt(roadBake, token, review?.sha256 ?? previous, raw);
}

function assertLaterRoadTileReceipt(roadBake, token, previous, raw) {
  let current = previous;
  for (const review of [
    roadBake.centralWestSplitSurvey,
    roadBake.i80FifthMappedSeamSurvey,
    roadBake.i280MariposaFifthCurveSurvey,
    roadBake.alemanyNorthboundConnectorSurvey,
    roadBake.i80HarrisonLoopPavementSurvey,
    roadBake.macallaWestboundExitSurvey,
  ]) {
    const tile = review.tiles[token];
    if (!tile) continue;
    assert.equal(tile.parentSha256, current);
    current = tile.sha256;
  }
  assert.equal(current, digest(raw));
}

function assertSouthgateReview(roadBake, token, raw) {
  const review = roadBake.southgateLegacyRoadReview.tiles[token];
  assert.equal(
    review.parentSha256,
    roadBake.ybiUpperWestSurvey.tiles[token].sha256,
  );
  assertLaterRoadTileReceipt(roadBake, token, review.sha256, raw);
}

test("the Yerba Buena bake records its surveyed tree clearance and keeps the road network", () => {
  const manifestBytes = file("tiles/manifest.json");
  const manifest = JSON.parse(manifestBytes);
  const route = JSON.parse(file("quest-route.json"));
  const token = "80858024";
  const release = manifest.roadBake.treeClassification;
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const receipt = release.tiles[token];
  assert.equal(receipt.removed, 30);
  const landing = manifest.provenance.oakland_touchdown_survey;
  assert.equal(receipt.sha256, landing.previousIslandTileSha256);
  const hillcrest = manifest.roadBake.hillcrestEastSpanSurvey.tiles[token];
  assert.equal(landing.islandTileSha256, hillcrest.parentSha256);
  const westbound = manifest.roadBake.ybiUpperWestSurvey.tiles[token];
  assert.equal(westbound.parentSha256, hillcrest.sha256);
  assertSouthgateReview(manifest.roadBake, token, raw);
  assert.equal(route.manifestSha256, digest(manifestBytes));
  assert.equal(manifest.tiles.find(({ t }) => t === token).bytes, raw.length);
  assert.ok(
    tile.playableRoads.some(({ id }) => id === "yerba-buena-upper-tunnel"),
  );
  assert.ok(!tile.tr.some(([x, y]) => x === 2417 && y === 1286));
  assert.ok(!tile.tr.some(([x, y]) => x === -2295 && y === -3445));
  assert.ok(!tile.tr.some(([x, y]) => x === -1623 && y === -3934));
  // This nearby row retained returns within the vehicle-height band.
  assert.ok(tile.tr.some(([x, y, h]) => x === 1497 && y === 1206 && h === 73));
});

test("South Gate keeps the distinct street branch and retires the obsolete painted connection", () => {
  const token = "80858024";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assertSouthgateReview(roadBake, token, raw);
  const review = roadBake.southgateLegacyRoadReview.tiles[token];
  assert.equal(roadBake.coveredGroundRoadPatches[token], 1);
  assert.equal(
    review.rowSha256,
    "cc2bd7a6633ae9e8010ef7af3b4ca96693ff52b7446126bfb9cb2ecfe7f4c4cd",
  );

  const streets = tile.r.filter(([name]) => tile.n[name] === "South Gate Rd");
  assert.equal(streets.length, 1);
  assert.equal(
    digest(JSON.stringify(streets[0]) + "\n"),
    "06a097f958b7331a5ad2d5a4e138fafb0b4b48dcd3b0bcc04969881f3af6da89",
  );
  for (const id of [
    "sf-contact/yerba-buena-eastbound-exit-to-southgate",
    "sf-contact/yerba-buena-southgate-to-macalla-shared",
    "sf-contact/yerba-buena-east-eastbound-on-ramp",
  ]) {
    assert.ok(tile.playableRoads.some((road) => road.id === id));
  }
});

test("the Fourth Street merge clears the capped face above its lower volume", () => {
  const token = "80858084";
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const review = manifest.roadBake.buildingClearanceReviews.findLast(
    ({ tiles }) => tiles[token],
  ).tiles[token];
  const fork = manifest.roadBake.i80FifthForkLaneWidthSurvey.tiles[token];
  assert.equal(fork.parentSha256, review.sha256);
  const divider = manifest.roadBake.i80FifthDividerSurvey.tiles[token];
  assert.equal(divider.parentSha256, fork.sha256);
  assertRampWidthReceipt(manifest.roadBake, token, divider.sha256, raw);

  const index = buildCollisionIndex(tile, token);
  const car = { x: 3382.15, y: 2037.35, heading: -2.37 };
  assert.equal(collisionAt([index], car, 10.82), null);
  assert.equal(collisionAt([index], car, 9.5), "building");
});

test("both Moscone spans leave Howard Street open below their overhead volumes", () => {
  const token = "80858084";
  const raw = file(`tiles/${token}.json.gz`);
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const receipts = manifest.roadBake.buildingClearanceReviews
    .map(({ tiles }) => tiles[token])
    .filter(Boolean);
  const howard = receipts.at(-1);
  assert.equal(howard.parentSha256, receipts.at(-2).sha256);
  const fork = manifest.roadBake.i80FifthForkLaneWidthSurvey.tiles[token];
  assert.equal(fork.parentSha256, howard.sha256);
  const divider = manifest.roadBake.i80FifthDividerSurvey.tiles[token];
  assert.equal(divider.parentSha256, fork.sha256);
  assertRampWidthReceipt(manifest.roadBake, token, divider.sha256, raw);

  const tile = JSON.parse(gunzipSync(raw));
  const index = buildCollisionIndex(tile, token);
  for (const [x, y, base] of [
    [3400.69, 2700.84, 19.3],
    [3345.3, 2642.85, 16.2],
  ]) {
    const car = { x, y, heading: -2.35725 };
    assert.equal(collisionAt([index], car, 9.6), null);
    assert.equal(collisionAt([index], car, base), "building");
  }
});

test("the Fifth Street divider covers both ramps and leaves the eastbound merge open", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const review = manifest.roadBake.i80FifthDividerSurvey;
  const roads = new Map();
  for (const [token, receipt] of Object.entries(review.tiles)) {
    const raw = file(`tiles/${token}.json.gz`);
    const climb = manifest.roadBake.i80LowerClimbWidthSurvey.tiles[token];
    if (climb) assert.equal(climb.parentSha256, receipt.sha256);
    const existingHash = climb?.sha256 ?? receipt.sha256;
    const westSpan = manifest.roadBake.i80WestSpanWidthSurvey.tiles[token];
    if (westSpan) assert.equal(westSpan.parentSha256, existingHash);
    const westSpanHash = westSpan?.sha256 ?? existingHash;
    const fremont = manifest.roadBake.i80FremontAlignmentSurvey.tiles[token];
    if (fremont) assert.equal(fremont.parentSha256, westSpanHash);
    const fremontHash = fremont?.sha256 ?? westSpanHash;
    const edge = manifest.roadBake.westboundCityApproachEdgeSurvey.tiles[token];
    if (edge) assert.equal(edge.parentSha256, fremontHash);
    let currentHash = edge?.sha256 ?? fremontHash;
    for (const { tiles } of manifest.roadBake.roadTreeClearanceReviews) {
      const clearance = tiles[token];
      if (!clearance) continue;
      assert.equal(clearance.parentSha256, currentHash);
      currentHash = clearance.sha256;
    }
    const northboundExit =
      manifest.roadBake.mariposaFifthExitSurvey.tiles[token];
    if (northboundExit) {
      assert.equal(northboundExit.parentSha256, currentHash);
      currentHash = northboundExit.sha256;
    }
    const sixthMerge = manifest.roadBake.sixthSouthMergeSurvey.tiles[token];
    if (sixthMerge) {
      assert.equal(sixthMerge.parentSha256, currentHash);
      currentHash = sixthMerge.sha256;
    }
    assertRampWidthReceipt(manifest.roadBake, token, currentHash, raw);
    assert.equal(manifest.tiles.find(({ t }) => t === token).bytes, raw.length);
    for (const road of JSON.parse(gunzipSync(raw)).playableRoads) {
      if (roads.has(road.id)) assert.deepEqual(roads.get(road.id), road);
      else roads.set(road.id, road);
    }
  }
  const entry = roads.get("sf-contact/i80-eastbound-fifth-bryant-entry");
  const exit = roads.get("sf-contact/i80-westbound-fifth-harrison-exit");
  const soma = roads.get("sf-contact/i80-eastbound-soma");
  const climb = roads.get("sf-contact/i80-eastbound-lower-climb");
  assert.deepEqual(entry.seams.end, climb.seams.start);
  assert.deepEqual(soma.seams.end, climb.seams.start);
  const [median, gore] = entry.railRanges;
  assert.equal(median.side, "left");
  assert.equal(gore.side, "right");
  assert.ok(median.endMetres - gore.endMetres > 40);
  assert.equal(exit.railRanges[0].side, "left");
  assert.ok(
    exit.railRanges[0].endMetres - exit.railRanges[0].startMetres > 230,
  );
  for (const road of [entry, exit]) {
    assert.ok(highwayRailSegments(buildBakedRoadContact(road).surface).length);
  }
  const ramp = buildBakedRoadContact(entry).surface;
  const deck = buildBakedRoadContact(climb).surface;
  for (const [surface, x, y] of [
    [ramp, 3683, 2299],
    [deck, 3703, 2318.8],
  ]) {
    assert.ok(roadSupportsCar(surface.heightAt, { x, y, heading: 0.78 }));
  }
});

test("the outside eastbound merge lane holds the car on the lower climb", () => {
  const token = "8085807c";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/i80-eastbound-lower-climb",
  );
  const { surface } = buildBakedRoadContact(road);
  const obstacles = buildCollisionIndex(tile, token);
  for (const [station, offset] of [
    [28, 21],
    [68, 21.5],
    [110, 21],
    [144, 19.5],
  ]) {
    const i = surface.profile.findIndex(({ s_m }) => s_m >= station);
    const {
      position: [x, y, z],
    } = surface.profile[i];
    const a = surface.profile[i - 1].position;
    const b = surface.profile[i + 1].position;
    const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
    const car = {
      x: x - offset * Math.sin(heading),
      y: y + offset * Math.cos(heading),
      z,
      heading,
    };
    assert.ok(roadSupportsCar(surface.heightAt, car));
    assert.equal(collisionAt([obstacles], car, z), null);
  }
});

test("The Embarcadero has street clearance beneath the retained Bay Bridge volume", () => {
  const token = "80858074";
  const raw = file(`tiles/${token}.json.gz`);
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const review = manifest.roadBake.buildingClearanceReviews.findLast(
    ({ tiles }) => tiles[token],
  );
  const westSpan = manifest.roadBake.i80WestSpanWidthSurvey.tiles[token];
  const fremont = manifest.roadBake.i80FremontAlignmentSurvey.tiles[token];
  assert.equal(review.tiles[token].sha256, westSpan.parentSha256);
  assert.equal(westSpan.sha256, fremont.parentSha256);
  const edge = manifest.roadBake.westboundCityApproachEdgeSurvey.tiles[token];
  assert.equal(edge.parentSha256, fremont.sha256);
  assert.equal(edge.sha256, digest(raw));

  const tile = JSON.parse(gunzipSync(raw));
  const obstacles = [buildCollisionIndex(tile, token)];
  for (const [x, y] of [
    [4563.4, 3185.567],
    [4567.975, 3161.65],
    [4569.225, 3156],
  ]) {
    const car = { x, y, heading: -1.375 };
    const ground = groundAtWorld([tile], x, y, true);
    assert.ok(ground >= 3.2 && ground <= 3.5);
    assert.equal(collisionAt(obstacles, car, ground), null);
    assert.equal(collisionAt(obstacles, car, 36.3), "building");
  }
  const besideLane = { x: 4569.4, y: 3186.5, heading: -1.375 };
  const ground = groundAtWorld([tile], besideLane.x, besideLane.y, true);
  assert.equal(collisionAt(obstacles, besideLane, ground), "building");
});

test("the northbound Embarcadero right lane clears both inferred volumes", () => {
  const token = "80858074";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const obstacles = [buildCollisionIndex(tile, token)];
  const [east, north] = tile.o;
  const [, , direction, points] = tile.r[15];
  assert.equal(tile.n[tile.r[15][0]], "The Embarcadero");
  assert.equal(direction, 1);
  let stations = 0;
  let adjacentObstructions = 0;
  for (let i = 0; i + 3 < points.length; i += 2) {
    const x = east + points[i] / 10;
    const y = north + points[i + 1] / 10;
    if (y < 3170 || y > 3220) continue;
    const heading = Math.atan2(
      points[i + 3] - points[i + 1],
      points[i + 2] - points[i],
    );
    const right = {
      x: x + 3 * Math.sin(heading),
      y: y - 3 * Math.cos(heading),
      heading,
    };
    const street = groundAtWorld([tile], right.x, right.y, true);
    assert.ok(street > 3 && street < 4);
    assert.equal(collisionAt(obstacles, right, street), null);
    const center = groundAtWorld([tile], x, y, true);
    if (collisionAt(obstacles, { x, y, heading }, center) === "building")
      adjacentObstructions++;
    stations++;
  }
  assert.ok(stations >= 16);
  assert.ok(adjacentObstructions >= 6);
});

test("reviewed offset lanes clear the inferred trunks on the three ramps", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const review = manifest.roadBake.treeClassification.additionalReviews.find(
    ({ classificationSha256 }) =>
      classificationSha256 ===
      "2ddfa4f406782f4a10df5693508952f2b8ec84de849b9b423664aadc562942c6",
  );
  assert.ok(review);
  for (const [token, row, [x, y, z, heading]] of [
    ["808f7c2c", [4329, 4204], [-659.634, -4123.431, 64.917, 1.318]],
    ["808f7e64", [-3451, -3058], [560.155, -3018.053, 50.275, -2.359]],
    ["808f7f24", [-3572, 1222], [3544.39, -4604.596, 39.249, -1.469]],
  ]) {
    const raw = file(`tiles/${token}.json.gz`);
    const tile = JSON.parse(gunzipSync(raw));
    assert.equal(manifest.tiles.find(({ t }) => t === token).bytes, raw.length);
    assert.ok(
      !tile.tr.some(([tx, ty]) => tx === row[0] && ty === row[1]),
      token,
    );
    const index = buildCollisionIndex(tile, token);
    assert.equal(collisionAt([index], { x, y, heading }, z), null, token);
  }
  const bosworth = JSON.parse(gunzipSync(file("tiles/808f7e7c.json.gz")));
  assert.ok(bosworth.tr.some(([x, y]) => x === 4369 && y === -638));
});

test("San Jose Avenue is clear from the southbound approach into the Tingley undercrossing", () => {
  const token = "808f7e64";
  const raw = file(`tiles/${token}.json.gz`);
  const manifest = JSON.parse(file("tiles/manifest.json"));
  assert.equal(
    manifest.roadBake.tingleyPortalClearance.tiles[token].sha256,
    digest(raw),
  );
  const scenery = JSON.parse(gunzipSync(raw));
  const approachBytes = file("tiles/808f7e7c.json.gz");
  const approach = JSON.parse(gunzipSync(approachBytes));
  const review = manifest.roadBake.terrainDetailReviews.findLast(
    ({ tile }) => tile === approach.t,
  );
  assert.equal(review.sha256, digest(approachBytes));
  assert.equal(approach.g.detail.s, 1);
  assert.equal(approach.g.details[0].s, 1);
  const tiles = [scenery, approach];
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  for (const [x, y, measured] of [
    [462.9301, -3110.0556, 46.16],
    [459.1922, -3113.3765, 45.9],
    [455.4544, -3116.6975, 45.65],
  ]) {
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, false) - measured) < 0.2);
  }
  for (const [x, y] of [
    [450.358, -3119.004],
    [448.585, -3121.154],
    [443.4933, -3127.3244],
  ]) {
    const z = groundAtWorld(tiles, x, y, true);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt(indexes, { x, y, heading: -2.26 }, z), null);
  }
});

test("Corbett Avenue follows the surveyed pavement in both lanes", () => {
  const token = "808f7e0c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const review = manifest.roadBake.terrainDetailReviews.findLast(
    ({ tile }) => tile === token,
  );
  assert.equal(review.sha256, digest(raw));
  for (const [x, y, measured] of [
    [-342.356, -984.191, 167.73],
    [-345.402, -983.212, 167.73],
    [-343.547, -987.993, 167.74],
    [-346.603, -987.044, 167.75],
  ]) {
    const height = groundAtWorld([tile], x, y, true);
    assert.ok(Number.isFinite(height));
    assert.ok(
      Math.abs(height - measured) < 0.035,
      `Corbett height at ${x}, ${y}`,
    );
  }
  assert.equal(tile.g.detail.s, 1);
  assert.equal(tile.g.details.at(-1).s, 1);
});

test("the Alemany connector leaves lower pavement clear beneath the crossing", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const token = "808f7c4c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const viaduct = manifest.roadBake.alemanyWestViaductSurvey.tiles[token];
  const review = manifest.roadBake.roadTreeClearanceReviews.find(
    ({ tiles }) => tiles[token]?.sha256 === viaduct.parentSha256,
  ).tiles[token];
  assert.equal(review.removed, 3);
  const index = buildCollisionIndex(tile, token);
  for (const [tx, ty] of [
    [3250, -2732],
    [2950, -3152],
    [2950, -3272],
  ]) {
    assert.ok(!tile.tr.some(([x, y]) => x === tx && y === ty));
    const x = tile.o[0] + tx / 10;
    const y = tile.o[1] + ty / 10;
    const z = groundAtWorld([tile], x, y, false);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt([index], { x, y, heading: -2 }, z), null);
  }
});

test("both Brotherhood Way street lanes clear the reviewed collider positions", () => {
  const token = "808f7c4c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const { sha256: parent } = roadBake.alemanyWestViaductSurvey.tiles[token];
  const clearance = roadBake.roadTreeClearanceReviews.find(
    ({ tiles }) => tiles[token]?.parentSha256 === parent,
  ).tiles[token];
  assert.equal(clearance.sha256, digest(raw));
  assert.equal(clearance.removed, 3);
  const index = buildCollisionIndex(tile, token);
  for (const [tx, ty, x, y, heading] of [
    [3130, 648, -2777.06, -5252.73, 0],
    [3110, 788, -2774.6, -5237.15, Math.PI],
    [3490, 808, -2736.6, -5236.65, Math.PI],
  ]) {
    assert.ok(!tile.tr.some(([e, n]) => e === tx && n === ty));
    const z = groundAtWorld([tile], x, y, true);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt([index], { x, y, heading }, z), null);
  }
});

test("Mason Street and JFK Drive clear both surveyed obstruction positions", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  for (const [token, tx, ty, x, y, heading, pavement] of [
    ["808586d4", -1452, -620, -1234.122, 4913.584, -2.98, 3.018],
    ["808587bc", -552, 1942, -6135.856, 1120.512, -0.11, 11.969],
  ]) {
    const raw = file(`tiles/${token}.json.gz`);
    const tile = JSON.parse(gunzipSync(raw));
    const review = roadBake.roadTreeClearanceReviews.findLast(
      ({ tiles }) => tiles[token],
    ).tiles[token];
    assert.equal(review.sha256, digest(raw));
    assert.equal(review.removed, 1);
    assert.ok(!tile.tr.some(([east, north]) => east === tx && north === ty));
    const z = groundAtWorld([tile], x, y, true);
    assert.ok(Math.abs(z - pavement) < 0.02);
    assert.equal(
      collisionAt([buildCollisionIndex(tile, token)], { x, y, heading }, z),
      null,
    );
  }
});

test("the southbound Third Street Bridge approach keeps its outside lane clear", () => {
  const token = "808f7fd4";
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const receipt = manifest.roadBake.roadTreeClearanceReviews.findLast(
    ({ tiles }) => tiles[token],
  ).tiles[token];
  const next = manifest.roadBake.mariposaFifthExitSurvey.tiles[token];
  assert.equal(next.parentSha256, receipt.sha256);
  const sixthMerge = manifest.roadBake.sixthSouthMergeSurvey.tiles[token];
  assert.equal(sixthMerge.parentSha256, next.sha256);
  assertRampWidthReceipt(manifest.roadBake, token, sixthMerge.sha256, raw);
  assert.equal(receipt.removed, 1);
  assert.ok(!tile.tr.some(([x, y]) => x === 4638 && y === 2534));
  assert.ok(tile.tr.some(([x, y]) => x === 4548 && y === 2645));
  const car = { x: 4361.8, y: 1876.4, heading: -0.99 };
  const z = groundAtWorld([tile], car.x, car.y, false);
  assert.ok(Math.abs(z - 4.06) < 0.02);
  assert.equal(collisionAt([buildCollisionIndex(tile, token)], car, z), null);
});

test("the Third Street deck over Islais Creek supports all four surveyed lanes", () => {
  const token = "808f7fa4";
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const review = roadBake.islaisThirdStreetLaneWidthSurvey;
  const dogpatch = roadBake.dogpatchNorthMergeSurvey.tiles[token];
  assert.equal(dogpatch.parentSha256, review.tiles[token].sha256);
  assert.equal(dogpatch.sha256, digest(raw));
  assert.equal(review.stations_per_band, 110);

  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/islais-creek-third-street",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  const offset = (x, y, heading, lane) => ({
    x: x - lane * Math.sin(heading),
    y: y + lane * Math.cos(heading),
    heading,
  });
  for (const [i, [x, y, z]] of road.points.entries()) {
    if (i === 0 || i === road.points.length - 1) continue;
    const [previousX, previousY] = road.points[i - 1];
    const heading = Math.atan2(y - previousY, x - previousX);
    for (const lane of [10.4, 14.15, -2.1, -5.5]) {
      const car = offset(x, y, heading, lane);
      assert.ok(
        roadSupportsCar(surface.heightAt, car),
        `${lane} m at point ${i}`,
      );
      assert.ok(Math.abs(surface.heightAt(car.x, car.y) - z) < 0.02);
    }
    const outside = offset(x, y, heading, 18.5);
    assert.equal(surface.heightAt(outside.x, outside.y), null);
  }
  assert.equal(portals.length, 2);
});

test("local bridges carry the car above the ground grid and meet street pavement", () => {
  const specifications = [
    ["8085876c", "crossover-drive-over-jfk", [-3462, 1194.7], 6],
    ["808f7fa4", "islais-creek-third-street", [4638, -1397], 1],
    ["808f7fa4", "islais-creek-illinois-street", [4737, -1359], 3.5],
    ["808f7fd4", "mission-creek-fourth-street", [4203, 1664], 0.9],
    ["808f7f1c", "williams-avenue-caltrain-bridge", [3875, -3356.3], 7],
    ["808f7f1c", "paul-avenue-caltrain-bridge", [3762.7, -4128.5], 5.7],
    ["808580ec", "hyde-street-over-broadway", [1896.74, 4027.87], 1.8],
    ["8085879c", "lincoln-way-sunset-westbound", [-4938.166, 520.029], 6],
    ["8085879c", "lincoln-way-sunset-eastbound", [-4938.596, 508.064], 6],
    [
      "808f7d74",
      "sloat-boulevard-sunset-westbound",
      [-4753.976, -2886.979],
      5.5,
    ],
    [
      "808f7d74",
      "sloat-boulevard-sunset-eastbound",
      [-4753.307, -2906.121],
      5.5,
    ],
    ["808f7f04", "oakdale-avenue-caltrain-bridge", [3969.599, -2526.18], 3],
    [
      "808f7c4c",
      "junipero-serra-brotherhood-southbound",
      [-2763.873, -5240.111],
      5.8,
    ],
    [
      "808f7c4c",
      "junipero-serra-brotherhood-northbound",
      [-2745.942, -5246.241],
      5,
    ],
    ["808f7e64", "mission-street-i280-viaduct", [1045.132, -3117.971], 8],
    ["808f7e64", "mission-street-i280-viaduct", [988.167, -3190.214], 8],
    ["808f7e7c", "baden-street-i280-bridge", [58.551, -3333.969], 7.5],
    ["808f7e7c", "paulding-street-i280-bridge", [-287.432, -3518.962], 7.5],
  ];
  for (const [token, slug, [x, y], gap] of specifications) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/${slug}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(surface.kind, "bridge_deck");
    assert.equal(portals.filter((portal) => !portal.sideAccess).length, 2);
    for (const portal of portals) {
      assert.ok(
        Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
          0.025,
        slug,
      );
    }
    const separation =
      surface.heightAt(x, y) - groundAtWorld([tile], x, y, true);
    assert.ok(separation > gap, `${slug}: ${separation} m`);
  }
  const park = JSON.parse(gunzipSync(file("tiles/8085876c.json.gz")));
  const surfaceFor = (id) =>
    buildBakedRoadContact(
      park.playableRoads.find((road) => road.id === `sf-contact/${id}`),
    );
  const bridge = surfaceFor("crossover-drive-over-jfk").surface;
  const lower = surfaceFor("jfk-drive-under-crossover");
  const crossing = [-3461.12, 1188.53];
  const floor = lower.surface.heightAt(...crossing);
  assert.ok(floor > 72 && floor < 74);
  assert.ok(bridge.heightAt(...crossing) - floor > 5.5);
  for (const portal of lower.portals) {
    assert.ok(
      Math.abs(groundAtWorld([park], portal.x, portal.y, true) - portal.z) <
        0.025,
    );
  }
});

test("Oakdale joins Dunshee at deck level while retaining the lower rail cut", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7f04.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/oakdale-avenue-caltrain-bridge",
  );
  const { portals, surface } = buildBakedRoadContact(road);
  const [access] = portals.filter((portal) => portal.sideAccess);
  assert.deepEqual(access.sideAccess, {
    side: "left",
    startMetres: 38,
    endMetres: 50,
  });
  assert.ok(
    Math.abs(groundAtWorld([tile], access.x, access.y, true) - access.z) < 0.01,
  );
  const lowPoint = [3977, -2540];
  const railCut = groundAtWorld([tile], ...lowPoint, true);
  assert.equal(railCut, 13.5875);
  assert.ok(surface.heightAt(...lowPoint) - railCut > 4.5);
});

test("Lincoln Way selects the eastbound deck across both approach lanes", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/8085879c.json.gz")));
  const contacts = tile.playableRoads
    .filter(({ id }) => id.startsWith("sf-contact/lincoln-way-sunset-"))
    .map(buildBakedRoadContact);
  assert.equal(contacts.length, 2);
  const eastbound = contacts.find(({ surface }) =>
    surface.id.endsWith("eastbound"),
  );
  const { x, y, nx, ny } = eastbound.portals[1];
  const ground = (east, north) => groundAtWorld([tile], east, north, true);

  for (const offset of [-2.8, 0, 2.8]) {
    const east = x - nx * 0.04 - ny * offset;
    const north = y - ny * 0.04 + nx * offset;
    const car = {
      x: east,
      y: north,
      z: ground(east, north),
      heading: Math.atan2(ny, nx),
      v: 11,
      grounded: true,
    };
    const next = driveThroughRoadPortal(
      car,
      { throttle: 1 },
      1 / 120,
      contacts.map(({ portals }) => portals),
      ground,
    );
    assert.equal(
      next?.surfaceId,
      eastbound.surface.id,
      `lane offset ${offset}`,
    );
  }
});

test("street underpasses keep their lower pavement and the freeway above it", () => {
  const tests = [
    [
      "808f7e54",
      "cortland-avenue-under-us101",
      "us101-north-alemany-to-silver",
      [2833.711, -2261.374],
      7.7,
      -0.077,
    ],
    [
      "808f7ee4",
      "bacon-street-under-us101",
      "us101-north-paul-to-silliman",
      [3288.208, -3565.776],
      18.4,
      -2.825,
    ],
    [
      "808f7ee4",
      "paul-avenue-under-us101",
      "us101-south-paul-mainline",
      [3414.601, -4024.334],
      20.4,
      2.84,
    ],
    [
      "808f7fb4",
      "dogpatch-twenty-second-under-i280",
      "i280-dogpatch-approach",
      [4190.7401, -254.3311],
      14.03,
      -3.065,
    ],
    [
      "808f7c24",
      "sickles-avenue-under-i280",
      "i280-south-geneva-to-sr82",
      [-1372.5463, -5435.5138],
      91.63,
      2.353,
    ],
    [
      "808f7c24",
      "san-jose-avenue-first-under-i280-at-sickles",
      "i280-south-geneva-to-sr82",
      [-1423.0909, -5456.3241],
      92.26,
      -2.29,
    ],
    [
      "808f7c24",
      "san-jose-avenue-second-under-i280-at-sickles",
      "i280-south-geneva-to-sr82",
      [-1450.3537, -5466.171],
      92.44,
      -2.282,
    ],
  ];
  for (const [token, underpass, freeway, [x, y], elevation, heading] of tests) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    const road = (slug) =>
      buildBakedRoadContact(
        tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
      );
    const { surface, portals } = road(underpass);
    const ground = (east, north) => groundAtWorld([tile], east, north, true);
    const floor = surface.heightAt(x, y);
    const deck = road(freeway).surface.heightAt(x, y);
    assert.ok(Math.abs(floor - elevation) < 0.2, underpass);
    assert.ok(Math.abs(ground(x, y) - floor) < 0.1, underpass);
    assert.ok(deck - floor > 5, underpass);
    assert.equal(portals.length, 2, underpass);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(ground(x, y) - z) < 0.08, underpass);
    }
    const indexes = [buildCollisionIndex(tile, token)];
    for (const offset of [-1.6, 0, 1.6]) {
      const car = {
        x: x - Math.sin(heading) * offset,
        y: y + Math.cos(heading) * offset,
        heading,
      };
      assert.equal(collisionAt(indexes, car, floor), null, underpass);
    }
    if (underpass.startsWith("cortland")) {
      assert.equal(
        collisionAt(indexes, { x, y, heading }, 13.4),
        "building",
        "the volume above the open lane remains in the scene",
      );
    }
  }
});

test("the tile adjoining Bacon Street carries the complete lower contact", () => {
  const contact = (token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))).playableRoads.find(
      ({ id }) => id === "sf-contact/bacon-street-under-us101",
    );
  const original = contact("808f7ee4");
  assert.ok(original);
  assert.deepEqual(contact("808f7f1c"), original);
});

test("Hyde Street keeps a separate deck above both Broadway Tunnel bores", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808580ec.json.gz")));
  const surface = (slug) =>
    buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
    ).surface;
  const deck = surface("hyde-street-over-broadway");
  for (const [direction, x, y] of [
    ["eastbound", 1898.1333, 4025.05],
    ["westbound", 1896.815, 4033.5483],
  ]) {
    const lower = surface(`broadway-${direction}`).heightAt(x, y);
    const upper = deck.heightAt(x, y);
    assert.ok(Math.abs(lower - 55.14) < 0.1, direction);
    assert.ok(upper - lower > 6, direction);
  }
});

test("Bryant Street carries the complete viaduct across the tile seam", () => {
  const tokens = ["80858074", "8085807c"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const [road, copy] = tiles.map((tile) =>
    tile.playableRoads.find(
      ({ id }) => id === "sf-contact/bryant-street-delancey-viaduct",
    ),
  );
  assert.ok(road);
  assert.deepEqual(copy, road);
  assert.deepEqual(road.owners, tokens);
  const { surface, portals } = buildBakedRoadContact(road);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.025);
  const [x, y] = [4320.6678, 2803.8757];
  assert.ok(surface.heightAt(x, y) - groundAtWorld(tiles, x, y, true) > 4);
});

test("Delancey follows the lower pavement and joins both adjoining streets", () => {
  const tiles = ["80858074", "8085807c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const [northeast, southwest] = tiles.map(({ playableRoads }) =>
    playableRoads.find(
      ({ id }) => id === "sf-contact/delancey-street-beside-bryant",
    ),
  );
  assert.ok(northeast);
  assert.deepEqual(northeast, southwest);
  assert.deepEqual(northeast.owners, ["80858074", "8085807c"]);
  const { surface, portals } = buildBakedRoadContact(northeast);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.05);
  const [x, y] = [4350.2121, 2817.5611];
  const floor = surface.heightAt(x, y);
  assert.ok(Math.abs(floor - 5.1) < 0.1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - floor) < 0.08);
  const approachFloor = surface.heightAt(4403.675, 2870.971);
  assert.ok(Math.abs(approachFloor - 4.04) < 0.06);
});

test("the Congdon crossing retains separate I-280 surfaces below its deck", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const get = (name) => {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/${name}`,
    );
    assert.ok(road, name);
    return buildBakedRoadContact(road);
  };
  const deck = get("justin-congdon-over-i280");
  assert.equal(deck.portals.length, 2);
  for (const [road, x, y] of [
    ["i280-north-monterey-to-alemany", 1219.2632, -3167.6042],
    ["i280-south-beyond-alemany", 1225.7041, -3146.8427],
  ]) {
    const upper = deck.surface.heightAt(x, y);
    const lower = get(road).surface.heightAt(x, y);
    assert.ok(Number.isFinite(upper) && Number.isFinite(lower));
    assert.ok(upper - lower > 6 && upper - lower < 8);
  }
});

test("both Geneva Avenue carriageways remain above both I-280 mainlines", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7c2c.json.gz")));
  const get = (id) => {
    const road = tile.playableRoads.find(
      ({ id: key }) => key === `sf-contact/${id}`,
    );
    assert.ok(road, id);
    return buildBakedRoadContact(road).surface;
  };
  const northbound = get("i280-north-geneva-to-ocean");
  const southbound = get("i280-south-ocean-to-geneva");
  for (const [direction, north, south] of [
    ["eastbound", [-705.8155, -4308.3693], [-723.5587, -4300.055]],
    ["westbound", [-704.5614, -4292.7347], [-722.2676, -4283.6408]],
  ]) {
    const avenue = get(`geneva-avenue-${direction}-over-i280`);
    for (const [highway, point] of [
      [northbound, north],
      [southbound, south],
    ]) {
      const upper = avenue.heightAt(...point);
      const lower = highway.heightAt(...point);
      assert.ok(Number.isFinite(upper) && Number.isFinite(lower));
      assert.ok(upper - lower > 6 && upper - lower < 8);
    }
  }
});

test("Mount Vernon joins all three streets above separate I-280 carriageways", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7c2c.json.gz")));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const get = (name) => roads.get(`sf-contact/${name}`);
  const approaches = {
    north: [get("san-jose-avenue-north-over-i280"), "end"],
    south: [get("san-jose-avenue-south-at-mount-vernon"), "start"],
    west: [get("mount-vernon-avenue-over-i280"), "start"],
  };
  const endpoint = (road, end) => road.points.at(end === "end" ? -1 : 0);
  for (const [road, end] of Object.values(approaches)) {
    const [portal] = buildBakedRoadContact(road).portals;
    assert.deepEqual(road.groundEnds, [end === "end" ? "start" : "end"]);
    assert.ok(
      Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
        0.03,
    );
  }
  for (const [first, second] of [
    ["north", "south"],
    ["north", "west"],
    ["south", "west"],
  ]) {
    const [incoming, from] = approaches[first];
    const [outgoing, to] = approaches[second];
    const curve = get(`mount-vernon-junction-${first}-${second}`);
    assert.deepEqual(curve.points[0], endpoint(incoming, from));
    assert.deepEqual(curve.points.at(-1), endpoint(outgoing, to));
    assert.deepEqual(curve.seams, {
      start: incoming.seams[from],
      end: outgoing.seams[to],
    });
    for (const [a, b] of [
      [`${incoming.id}:${from}`, `${curve.id}:start`],
      [`${curve.id}:end`, `${outgoing.id}:${to}`],
    ])
      assert.ok(
        roadBake.playableTransitions.some(
          ({ from, to, direction }) =>
            from === a && to === b && direction === "both",
        ),
      );
  }
  for (const [high, low, x, y] of [
    [
      "san-jose-avenue-north-over-i280",
      "i280-north-sr82-to-geneva",
      -731.4237,
      -4595.0466,
    ],
    [
      "mount-vernon-junction-north-west",
      "i280-south-beyond-geneva",
      -755.6345,
      -4611.0114,
    ],
  ]) {
    const upper = buildBakedRoadContact(get(high)).surface.heightAt(x, y);
    const lower = buildBakedRoadContact(get(low)).surface.heightAt(x, y);
    assert.ok(Number.isFinite(upper) && Number.isFinite(lower));
    assert.ok(upper - lower > 8 && upper - lower < 10);
  }
});

test("Saint Charles passes over I-280 and western Alemany passes below it", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7c3c.json.gz")));
  const get = (name) =>
    buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`),
    );
  const bridge = get("saint-charles-avenue-over-i280");
  const northbound = get("i280-north-edge-to-sr82");
  const southbound = get("i280-south-geneva-to-sr82");
  for (const [road, x, y] of [
    [northbound, -2538.346, -5766.955],
    [get("i280-south-sr82-to-edge"), -2556.202, -5751.768],
    [get("i280-south-edge-exit"), -2568.861, -5737.422],
  ]) {
    const pavement = bridge.surface.heightAt(x, y);
    const freeway = road.surface.heightAt(x, y);
    assert.ok(Number.isFinite(pavement) && Number.isFinite(freeway));
    assert.ok(pavement - freeway > 6 && pavement - freeway < 8);
  }
  const eastbound = get("western-alemany-eastbound-under-i280");
  const westbound = get("western-alemany-westbound-under-i280");
  for (const [street, road, x, y] of [
    [eastbound, northbound, -1828.53, -5507.642],
    [eastbound, southbound, -1866.084, -5484.74],
    [westbound, northbound, -1808.54, -5507.494],
    [westbound, southbound, -1838.108, -5484.533],
  ]) {
    const pavement = street.surface.heightAt(x, y);
    const freeway = road.surface.heightAt(x, y);
    assert.ok(Number.isFinite(pavement) && Number.isFinite(freeway));
    assert.ok(freeway - pavement > 6 && freeway - pavement < 8);
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - pavement) < 0.25);
  }
  for (const contact of [bridge, eastbound, westbound]) {
    assert.equal(contact.portals.length, 2);
    for (const { x, y, z } of contact.portals)
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.05);
  }
});

test("both Hester approaches stay above the terrain and join Third Street", () => {
  const token = "808f7f24";
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const raw = file(`tiles/${token}.json.gz`);
  const clearance = roadBake.roadTreeClearanceReviews.findLast(
    ({ tiles }) => tiles[token],
  ).tiles[token];
  assert.equal(
    roadBake.hesterThirdApproachSurvey.tiles[token].sha256,
    clearance.parentSha256,
  );
  assert.equal(clearance.sha256, digest(raw));
  const tile = JSON.parse(gunzipSync(raw));
  assert.ok(!tile.tr.some(([x, y]) => x === -2632 && y === -2318));
  const [bayshore, third, through] = [
    "bayshore-hester-frontage-lead",
    "hester-third-northbound-bridge-approach",
    "bayshore-hester-frontage-to-third",
  ].map((name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`),
  );
  assert.deepEqual(bayshore.source_cnn, ["2820000", "233101"]);
  for (const approach of [bayshore, third]) {
    assert.deepEqual(approach.points.at(-1), through.points[0]);
    assert.deepEqual(approach.seams.end, through.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to }) =>
          from === `${approach.id}:end` && to === `${through.id}:start`,
      ),
    );
  }

  const pavement = (x, y) => groundAtWorld([tile], x, y, true);
  for (const [road, x, y] of [
    [bayshore, 3660.595, -4885.528],
    [third, 3658.23, -4904.96],
  ]) {
    const contact = buildBakedRoadContact(road);
    assert.equal(contact.portals.length, 1);
    const upper = contact.surface.heightAt(x, y);
    assert.ok(upper >= 48.8 && upper < 49.4);
    assert.ok(upper - pavement(x, y) > 8.5 && upper - pavement(x, y) < 9.5);
  }

  const obstacles = [buildCollisionIndex(tile, tile.t)];
  for (const road of [bayshore, third, through]) {
    const { portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, 1);
    for (const { x, y, z } of portals)
      assert.ok(Math.abs(pavement(x, y) - z) < 0.05);
    for (const [i, [x, y, z]] of road.points.entries()) {
      const before = road.points[Math.max(0, i - 1)];
      const after = road.points[Math.min(road.points.length - 1, i + 1)];
      const heading = Math.atan2(after[1] - before[1], after[0] - before[0]);
      for (const offset of [-1.5, 0, 1.5]) {
        const pose = {
          x: x - Math.sin(heading) * offset,
          y: y + Math.cos(heading) * offset,
          heading,
        };
        assert.equal(
          collisionAt(obstacles, pose, z),
          null,
          `${x}, ${y}, ${offset}`,
        );
      }
    }
  }
});

test("Geary stays below the Fillmore bridge in both directions", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808580bc.json.gz")));
  const load = (name) =>
    buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`),
    );
  const bridge = load("fillmore-geary-bridge");
  const lower = [
    ["geary-under-fillmore-eastbound", [624, 2697]],
    ["geary-under-fillmore-westbound", [624, 2710]],
  ];
  for (const [name, crossing] of lower) {
    const road = load(name);
    const floor = road.surface.heightAt(...crossing);
    assert.ok(floor > 32.8 && floor < 33.4, `${name}: ${floor} m`);
    assert.ok(bridge.surface.heightAt(...crossing) - floor > 5.8);
    assert.ok(groundAtWorld([tile], ...crossing, true) < floor);
    assert.equal(road.portals.length, 2);
    for (const { x, y, z } of road.portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.04);
    }
  }
  const upper = [
    ["eastbound", [624, 2697]],
    ["westbound", [624, 2710]],
  ];
  for (const [direction, crossing] of upper) {
    const frontage = load(`geary-fillmore-${direction}-frontage`);
    const surface = frontage.surface.heightAt(...crossing);
    assert.ok(surface > 39.1 && surface < 39.5, `${direction}: ${surface} m`);
    assert.ok(Math.abs(bridge.surface.heightAt(...crossing) - surface) < 0.1);
    for (const { x, y, z } of frontage.portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.04);
    }
  }
  for (const { x, y, z } of bridge.portals) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.04);
  }
});

test("the Music Concourse Garage joins open pavement below JFK Drive", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858744.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/music-concourse-garage",
  );
  const { surface, portals, appearance } = buildBakedRoadContact(road);
  const ground = (x, y) => groundAtWorld([tile], x, y, true);
  assert.equal(surface.kind, "tunnel");
  assert.ok(appearance.interior[1] - appearance.interior[0] > 480);
  assert.equal(portals.length, 2);
  for (const portal of portals) {
    assert.ok(Math.abs(ground(portal.x, portal.y) - portal.z) < 0.025);
  }
  const crossing = [-2474.64, 1390.23];
  const upper = ground(...crossing);
  const lower = surface.heightAt(...crossing);
  assert.ok(upper > 74.1 && upper < 74.4);
  assert.ok(lower > 68.9 && lower < 69.6);
  assert.ok(upper - lower > appearance.clearance);
  const approachPaint = tile.r.find(
    ([name, , , points]) =>
      tile.n[name] === "10th Ave" &&
      Math.abs(tile.o[1] + points[1] / 10 - 1476.7) < 0.1,
  );
  assert.ok(tile.o[1] + approachPaint[3].at(-1) / 10 > 1448);
});

test("Lyell Street retains smooth contact beneath both I-280 carriageways", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const surfaceFor = (slug) =>
    buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
    );
  const lower = surfaceFor("lyell-street-under-i280");
  assert.equal(lower.portals.length, 2);
  for (const portal of lower.portals) {
    assert.ok(
      Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
        0.025,
    );
  }
  for (const [slug, crossing] of [
    ["i280-north-monterey-to-alemany", [728.19, -3098.15]],
    ["i280-south-monterey-mainline", [724.13, -3078.01]],
  ]) {
    const floor = lower.surface.heightAt(...crossing);
    const deck = surfaceFor(slug).surface.heightAt(...crossing);
    assert.ok(floor > 36 && floor < 40, `${slug}: ${floor}`);
    assert.ok(deck - floor > 9, `${slug}: ${deck - floor}`);
  }
  const before = lower.surface.heightAt(727.15, -3093.03);
  const after = lower.surface.heightAt(725.13, -3083.23);
  assert.ok(after - before > 0 && after - before < 0.9);
  const terrain = groundAtWorld([tile], 725.13, -3083.23, true);
  assert.ok(terrain < after && after - terrain < 0.08);
});

test("San Jose Avenue rises through the Branch East ground handoff", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/san-jose-avenue-shared-deck",
  );
  const { portals } = buildBakedRoadContact(road);
  assert.equal(portals.length, 1);
  const { x, y, z, nx, ny } = portals[0];
  const ground = (s) => groundAtWorld([tile], x - s * nx, y - s * ny, true);
  assert.ok(Math.abs(ground(0) - z) < 0.025);
  assert.ok(ground(4) > ground(2));
  assert.ok(Math.abs(ground(4) - 59.94) < 0.035);
  assert.ok(ground(8) > ground(4));
});

test("Beale stays at street level under Harrison's independently baked deck", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/8085807c.json.gz")));
  const surfaceFor = (slug) =>
    buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
    );
  const lower = surfaceFor("beale-street-under-harrison");
  const upper = surfaceFor("harrison-beale-bridge");
  assert.equal(lower.portals.length, 2);
  for (const portal of lower.portals) {
    assert.ok(
      Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
        0.025,
    );
  }
  const crossing = [4300.061, 3048.916];
  const elevation = lower.surface.heightAt(...crossing);
  assert.ok(elevation > 4.9 && elevation < 5.4);
  assert.ok(upper.surface.heightAt(...crossing) - elevation > 10);
  assert.ok(
    Math.abs(groundAtWorld([tile], ...crossing, true) - elevation) < 0.08,
  );
  for (const [name, , , points] of tile.r) {
    if (tile.n[name] !== "Harrison St" && tile.n[name] !== "Beale St") continue;
    for (let i = 0; i < points.length; i += 2) {
      const x = tile.o[0] + points[i] / 10;
      const y = tile.o[1] + points[i + 1] / 10;
      assert.ok(Math.hypot(x - crossing[0], y - crossing[1]) > 20);
    }
  }
});

test("Dogpatch northbound runs below 18th Street and keeps 20th Street on its own deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fb4", "808f7fbc", "808f7fcc", "808f7fd4"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const names = [
    "i280-dogpatch-east-north-entry",
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
    "i280-mariposa-northbound-approach",
    "i280-mariposa-northbound",
  ].map((id) => `sf-contact/${id}`);
  for (const [index, id] of names.slice(1).entries()) {
    const a = roads.get(names[index]),
      b = roads.get(id);
    assert.deepEqual(a.points.at(-1), b.points[0], id);
    assert.deepEqual(a.seams.end, b.seams.start, id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${a.id}:end` &&
          to === `${b.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const northbound = buildBakedRoadContact(roads.get(names[2])).surface;
  const eighteenth = buildBakedRoadContact(
    roads.get("sf-contact/eighteenth-west-viaduct"),
  ).surface;
  const dogpatchZ = northbound.heightAt(4184.1, 312);
  const viaductZ = eighteenth.heightAt(4184.1, 312);
  assert.ok(dogpatchZ > 20 && dogpatchZ < 22);
  assert.ok(viaductZ > 25 && viaductZ < 28);
  const twentieth = roads.get("sf-contact/dogpatch-twentieth-viaduct");
  const { surface, portals } = buildBakedRoadContact(twentieth);
  assert.ok(surface.heightAt(4226.9, 31.34) > 31);
  const freeway = buildBakedRoadContact(roads.get(names[1])).surface;
  assert.ok(surface.heightAt(4209, 30.1) - freeway.heightAt(4209, 30.1) > 6);
  assert.equal(portals.length, 2);
  assert.ok(roadBake.coveredGroundRoadPatches["808f7fb4"] >= 1);
});

test("northbound I-280 branches onto the lower Mariposa exit and hands off to street pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fb4", "808f7fcc"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const incoming = roads.get("sf-contact/i280-dogpatch-northbound");
  const ramp = roads.get("sf-contact/i280-north-mariposa-exit");
  const through = roads.get("sf-contact/i280-south-of-eighteenth-northbound");
  for (const branch of [ramp, through]) {
    assert.deepEqual(incoming.points.at(-1), branch.points[0]);
    assert.deepEqual(incoming.seams.end, branch.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${branch.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, ["end"]);
  assert.deepEqual(ramp.widthTransitionMetres, { start: 24 });
  const { surface, portals } = buildBakedRoadContact(ramp);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const floor = surface.heightAt(4201.4533, 303.8024);
  assert.ok(floor > 18 && floor < 19, floor);
});

test("the westbound Harrison loop hands off below both Bay Bridge approaches", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/8085807c.json.gz")));
  const find = (name) => tile.playableRoads.find(({ id }) => id === name);
  const incoming = find("sf-contact/i80-westbound-fremont-split");
  const exit = find("sf-contact/i80-westbound-harrison-loop");
  const through = find("sf-contact/i80-westbound-upper-soma");
  for (const road of [exit, through]) {
    assert.deepEqual(road.points[0], incoming.points.at(-1));
    assert.deepEqual(road.seams.start, incoming.seams.end);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${road.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);

  const overcrossing = [4280.5062, 2843.2714];
  const heightAt = (name) =>
    buildBakedRoadContact(find(name)).surface.heightAt(...overcrossing);
  const ramp = surface.heightAt(...overcrossing);
  const lowerDeck = heightAt("sf-eastbound-lower-approach");
  const upperDeck = heightAt("city-approach");
  assert.ok(ramp > 25 && ramp < 26);
  assert.ok(lowerDeck > 33 && lowerDeck < 34);
  assert.ok(upperDeck > 42 && upperDeck < 43);
});

test("the Harrison loop supports the surveyed lane on the outside of its curve", () => {
  const token = "8085807c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const receipt = roadBake.i80HarrisonLoopPavementSurvey;
  assert.equal(receipt.source_osm_way, 31129372);
  assert.equal(receipt.tiles[token].sha256, digest(raw));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/i80-westbound-harrison-loop",
  );
  const { surface } = buildBakedRoadContact(road);
  for (const [x, y, height] of [
    [4220.58, 2714.23, 29.9],
    [4233.61, 2729.41, 29.4],
    [4246.63, 2744.58, 28.6],
  ]) {
    const heading = 0.8615;
    assert.ok(roadSupportsCar(surface.heightAt, { x, y, heading }));
    assert.ok(Math.abs(surface.heightAt(x, y) - height) < 0.25);
  }
});

test("the westbound Macalla ramp follows the surveyed lane down to the island road", () => {
  const token = "80858024";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const receipt = roadBake.macallaWestboundExitSurvey;
  assert.equal(receipt.source_osm_way, 322962944);
  assert.equal(receipt.tiles[token].sha256, digest(raw));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/yerba-buena-east-westbound-off-ramp",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  for (const [x, y, z, heading] of [
    [6705.62, 5818.4, 58.08, 2.98],
    [6681.05, 5816.22, 56.98, -2.8],
    [6658.13, 5797.65, 55.35, -2.16],
  ]) {
    assert.ok(roadSupportsCar(surface.heightAt, { x, y, heading }));
    assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.3);
  }
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(z - groundAtWorld([tile], x, y, true)) < 0.03);
});

test("the 18th Street entrance stays on its own deck until the measured merge", () => {
  const token = "808f7fcc";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const { tiles, merge_station_metres: station } =
    roadBake.eighteenthMariposaMergeSurvey;
  const next = roadBake.mariposaFifthExitSurvey.tiles[token];
  assert.equal(next.parentSha256, tiles[token].sha256);
  const sixthMerge = roadBake.sixthSouthMergeSurvey.tiles[token];
  assert.equal(sixthMerge.parentSha256, next.sha256);
  const dogpatch = roadBake.dogpatchNorthMergeSurvey.tiles[token];
  assert.equal(dogpatch.parentSha256, sixthMerge.sha256);
  assertRampWidthReceipt(roadBake, token, dogpatch.sha256, raw);
  assert.ok(station > 91 && station < 93);
  const road = (id) =>
    tile.playableRoads.find((row) => row.id === `sf-contact/${id}`);
  const ramp = road("i280-eighteenth-north-entry");
  const approach = road("i280-mariposa-northbound-approach");
  const main = road("i280-mariposa-northbound");
  const links = roadBake.playableTransitions
    .filter(({ from }) => from === `${ramp.id}:end`)
    .map(({ to, direction }) => [to, direction]);
  assert.deepEqual(links, [[`${main.id}:start`, "forward"]]);

  for (const incoming of [ramp, approach]) {
    assert.deepEqual(incoming.points.at(-1), main.points[0]);
    assert.deepEqual(incoming.seams.end, main.seams.start);
  }

  const rampContact = buildBakedRoadContact(ramp).surface;
  const approachContact = buildBakedRoadContact(approach).surface;
  const mainContact = buildBakedRoadContact(main).surface;
  for (const [x, y, z] of [
    [4151.2, 531.5, 24.0],
    [4139.4, 559.1, 23.5],
    [4134.8, 567.9, 23.4],
  ]) {
    const height = rampContact.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - z) < 0.3);
    assert.equal(approachContact.heightAt(x, y), null);
    assert.equal(mainContact.heightAt(x, y), null);
  }
});

test("western Alemany Boulevard follows the elevated lane to street pavement", () => {
  const token = "808f7c4c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const viaduct = roadBake.alemanyWestViaductSurvey.tiles[token];
  const clearance = roadBake.roadTreeClearanceReviews.find(
    ({ tiles }) => tiles[token]?.parentSha256 === viaduct.sha256,
  ).tiles[token];
  assert.equal(clearance.sha256, digest(raw));
  assert.equal(roadBake.coveredGroundRoadPatches[token], 1);
  const deck = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/alemany-west-viaduct",
  );
  const { surface, portals } = buildBakedRoadContact(deck);
  assert.equal(portals.length, 2);
  for (const portal of portals)
    assert.ok(
      Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
        0.005,
    );

  const [x, y] = [-2775.5, -5609.1];
  const height = surface.heightAt(x, y);
  assert.ok(
    Number.isFinite(height) && height - groundAtWorld([tile], x, y, true) > 9.5,
  );

  const [raisedApproach, lowerStreet] = tile.r.slice(6, 8);
  assert.equal(tile.n[raisedApproach[0]], "Alemany Blvd");
  const maxSouth = Math.min(...raisedApproach[3].filter((_, i) => i % 2));
  assert.ok(tile.o[1] + maxSouth / 10 > -5495);
  assert.equal(tile.n[lowerStreet[0]], "Alemany Blvd");
  assert.equal(lowerStreet[2], 2);
});

test("Market Street meets its approaches on both surveyed carriageways", () => {
  const token = "808f7e0c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const corbett = roadBake.terrainDetailReviews.findLast(
    ({ tile }) => tile === token,
  );
  assert.equal(
    corbett.parentSha256,
    roadBake.marketWestViaductSurvey.tiles[token].sha256,
  );
  assert.equal(corbett.sha256, digest(raw));
  assert.equal(roadBake.coveredGroundRoadPatches[token], 2);

  for (const [direction, x, y, minimumClearance] of [
    ["northbound", -280.07, -1036.29, 14],
    ["southbound", -289.58, -1039.56, 10.8],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/market-west-viaduct-${direction}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.deepEqual(road.groundEnds, ["start", "end"]);
    for (const portal of portals)
      assert.ok(
        Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
          0.005,
      );
    const deck = surface.heightAt(x, y);
    assert.ok(
      Number.isFinite(deck) &&
        deck - groundAtWorld([tile], x, y, true) > minimumClearance,
    );
  }

  const [east, north] = tile.o;
  for (const [street, , , positions] of tile.r) {
    if (tile.n[street] !== "Market St") continue;
    for (let i = 0; i < positions.length; i += 2) {
      const x = east + positions[i] / 10;
      const y = north + positions[i + 1] / 10;
      assert.ok(x < -310 || x > -242 || y < -1050 || y > -955);
    }
  }
});

test("the southbound Sixth Street deck joins after the separate Fifth Street approach", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7fcc", "808f7fd4"];
  const tiles = tokens.map((token) => {
    const raw = file(`tiles/${token}.json.gz`);
    const sixth = roadBake.sixthSouthMergeSurvey.tiles[token];
    const dogpatch = roadBake.dogpatchNorthMergeSurvey.tiles[token];
    if (dogpatch) assert.equal(dogpatch.parentSha256, sixth.sha256);
    assertRampWidthReceipt(roadBake, token, (dogpatch ?? sixth).sha256, raw);
    return JSON.parse(gunzipSync(raw));
  });
  const road = (id, tile = tiles[0]) =>
    tile.playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const sixth = road("i280-sixth-southbound");
  assert.deepEqual(sixth, road("i280-sixth-southbound", tiles[1]));
  const fifth = road("i280-fifth-south-entry");
  assert.deepEqual(fifth, road("i280-fifth-south-entry", tiles[1]));
  const lead = road("i280-fifth-south-to-sixth-merge");
  const main = road("i280-king-to-mariposa");

  for (const [from, target] of [
    [fifth, lead],
    [lead, main],
    [sixth, main],
  ]) {
    assert.deepEqual(from.points.at(-1), target.points[0]);
    assert.deepEqual(from.seams.end, target.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` && edge.to === `${target.id}:start`,
      ),
    );
  }

  const upper = buildBakedRoadContact(sixth).surface;
  const lower = buildBakedRoadContact(lead).surface;
  const entry = buildBakedRoadContact(fifth).surface;
  for (const [x, y] of [
    [3699.9, 1102.3],
    [3718.2, 1034.8],
    [3729.6, 1007.1],
  ]) {
    const height = upper.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - 19) < 0.25);
    assert.equal(lower.heightAt(x, y), null);
  }
  for (const [surface, x, y, z, heading] of [
    [entry, 3724.6, 1075.6, 17.35, -1.39],
    [lower, 3744.6, 1008.6, 17.75, -1.17],
  ]) {
    const height = surface.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - z) < 0.15);
    assert.ok(roadSupportsCar(surface.heightAt, { x, y, heading }));
    assert.equal(upper.heightAt(x, y), null);
  }
});

test("the northbound I-280 fork carries the surveyed lane to Fifth Street", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7fcc", "808f7fd4"];
  const raws = tokens.map((t) => file(`tiles/${t}.json.gz`));
  const tiles = raws.map((raw) => JSON.parse(gunzipSync(raw)));
  const review = roadBake.mariposaFifthExitSurvey;
  for (const [i, t] of tokens.entries()) {
    const sixthMerge = roadBake.sixthSouthMergeSurvey.tiles[t];
    assert.equal(sixthMerge.parentSha256, review.tiles[t].sha256);
    const dogpatch = roadBake.dogpatchNorthMergeSurvey.tiles[t];
    if (dogpatch) assert.equal(dogpatch.parentSha256, sixthMerge.sha256);
    assertRampWidthReceipt(
      roadBake,
      t,
      (dogpatch ?? sixthMerge).sha256,
      raws[i],
    );
  }

  const road = (tile, name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const incoming = road(tiles[0], "i280-mariposa-northbound");
  const through = road(tiles[0], "i280-mariposa-sixth-approach");
  const fifth = road(tiles[1], "i280-north-fifth-exit");
  const sixth = road(tiles[0], "i280-sixth-northbound");
  assert.deepEqual(fifth, road(tiles[0], "i280-north-fifth-exit"));
  assert.deepEqual(fifth.owners, tokens);
  for (const branch of [through, fifth]) {
    assert.deepEqual(branch.points[0], incoming.points.at(-1));
    assert.deepEqual(branch.seams.start, incoming.seams.end);
  }
  assert.deepEqual(through.points.at(-1), sixth.points[0]);
  assert.deepEqual(through.seams.end, sixth.seams.start);

  const entries = [incoming, through, fifth].map(buildBakedRoadContact);
  const surfaces = new Map(entries.map(({ surface }) => [surface.id, surface]));
  const graph = indexRoadNetwork(roadBake.playableTransitions);
  graph.add({
    endpoints: entries.flatMap(({ endpoints }) => endpoints),
    transitions: roadBake.playableTransitions,
  });
  const [east, north, z] = incoming.points.at(-1);
  const heading = incoming.seams.end.bearing;
  const dx = Math.cos(heading);
  const dy = Math.sin(heading);
  for (const [lateral, expected] of [
    [0, through],
    [4.2, fifth],
  ]) {
    let car = {
      x: east - 7 * dx - lateral * dy,
      y: north - 7 * dy + lateral * dx,
      z,
      v: 12,
      heading,
      surfaceId: incoming.id,
    };
    for (let tick = 0; tick < 120 && car.surfaceId === incoming.id; tick++) {
      car = driveOnRoadNetwork(
        car,
        { throttle: 0.1, brake: false, steer: 0 },
        1 / 120,
        surfaces.get(incoming.id),
        surfaces,
        graph,
        () => false,
      );
    }
    assert.equal(car.surfaceId, expected.id);
  }

  const height = surfaces.get(fifth.id).heightAt(3775.1, 977.3);
  assert.ok(Number.isFinite(height) && Math.abs(height - 16.5) < 0.25);
  assert.equal(surfaces.get(through.id).heightAt(3775.1, 977.3), null);
  assert.deepEqual(fifth.groundEnds, ["end"]);
  const [x, y, finishZ] = fifth.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - finishZ) < 0.1);
});

test("Alemany northbound stays on its own deck and clears the surveyed Dogpatch pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = [
    "808f7ef4",
    "808f7efc",
    "808f7f04",
    "808f7fa4",
    "808f7fac",
    "808f7fb4",
  ];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const lookup = (name) => roads.get(`sf-contact/${name}`);
  const chain = [
    "i280-north-alemany-entry",
    "i280-north-alemany-main",
    "i280-north-alemany-climb",
    "i280-north-alemany-before-connector-merge",
    "i280-north-lower-viaduct",
    "i280-north-south-dogpatch",
    "i280-dogpatch-northbound",
  ].map(lookup);
  assert.deepEqual(chain[0].groundEnds, ["start"]);
  const [x, y, z] = chain[0].points[0];
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  for (const [index, next] of chain.slice(1).entries()) {
    const previous = chain[index];
    assert.deepEqual(previous.points.at(-1), next.points[0], next.id);
    assert.deepEqual(previous.seams.end, next.seams.start, next.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${previous.id}:end` &&
          to === `${next.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const lower = buildBakedRoadContact(chain[3]).surface;
  const upper = buildBakedRoadContact(
    lookup("i280-south-beyond-dogpatch"),
  ).surface;
  const crossing = [3379.018, -2458.532];
  assert.ok(Math.abs(lower.heightAt(...crossing) - 21.77) < 0.03);
  assert.ok(Math.abs(upper.heightAt(...crossing) - 29.58) < 0.03);

  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  assert.ok(roadBake.overheadBuildingPatches["808f7fac"] >= 3);
  assert.equal(roadBake.overheadBuildingPatches["808f7fb4"], 1);
  const road = buildBakedRoadContact(
    lookup("i280-north-south-dogpatch"),
  ).surface;
  for (const station of [945, 975, 1129, 1155]) {
    const i = road.profile.findIndex(({ s_m }) => s_m >= station);
    assert.ok(i > 0);
    const [x, y, z] = road.profile[i].position;
    const [a, b] = road.profile[i - 1].position;
    const heading = Math.atan2(y - b, x - a);
    assert.equal(collisionAt(obstacles, { x, y, z, heading }, z), null);
  }
});

test("Cesar Chavez exit branches from the lower viaduct and returns to ground", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7efc", "808f7f04", "808f7fa4", "808f7fac", "808f7fb4"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const incoming = roads.get("sf-contact/i280-north-lower-viaduct");
  const ramp = roads.get("sf-contact/i280-north-cesar-chavez-exit");
  const through = roads.get("sf-contact/i280-north-south-dogpatch");
  for (const branch of [ramp, through]) {
    assert.deepEqual(incoming.points.at(-1), branch.points[0], branch.id);
    assert.deepEqual(incoming.seams.end, branch.seams.start, branch.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${incoming.id}:end` &&
          to === `${branch.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, ["end"]);
  const [endX, endY, endZ] = ramp.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, endX, endY, true) - endZ) < 0.03);

  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const { surface } = buildBakedRoadContact(ramp);
  const i = surface.profile.findIndex(({ s_m }) => s_m >= 112);
  assert.ok(i > 0);
  const [x, y, z] = surface.profile[i].position;
  const [a, b] = surface.profile[i - 1].position;
  assert.equal(
    collisionAt(indexes, { x, y, z, heading: Math.atan2(y - b, x - a) }, z),
    null,
  );
  assert.equal(roadBake.overheadBuildingPatches["808f7fac"], 4);
});

test("Alemany connects to US 101 and leaves the original I-280 branch available", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ef4", "808f7efc", "808f7e54", "808f7e4c"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const byId = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (id) => byId.get(`sf-contact/${id}`);
  const approach = road("i280-north-alemany-main");
  const connector = road("i280-to-101-north-connector");
  const silver = road("us101-north-silver-approach");
  const exit = road("us101-north-bayshore-exit");
  const through = road("us101-north-bayshore-through");
  for (const [before, after] of [
    [approach, road("i280-north-alemany-climb")],
    [approach, connector],
    [connector, silver],
    [silver, exit],
    [silver, through],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.deepEqual(through.groundEnds, []);
  const [endX, endY, endZ] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, endX, endY, true) - endZ) < 0.03);
  const { surface } = buildBakedRoadContact(through);
  const crossing = [3199.64, -1212.11];
  assert.ok(
    surface.heightAt(...crossing) - groundAtWorld(tiles, ...crossing, true) > 7,
  );

  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  const [x, y, z] = connector.points.at(-1);
  assert.equal(collisionAt(obstacles, { x, y, z, heading: 1.67 }, z), null);
  assert.ok(roadBake.overheadBuildingPatches["808f7e54"] >= 1);
});

test("the Central Freeway connector stays on separate pavement to eastbound I-80", () => {
  const token = "808f7e2c";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const receipt = roadBake.centralEastMergeSurvey.tiles[token];
  assertRampWidthReceipt(roadBake, token, receipt.sha256, raw);
  const road = (slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const ramp = road("central-freeway-i80-east-connector");
  const lead = road("i80-eastbound-us101-approach");
  const eastbound = road("i80-eastbound-interchange");
  for (const approach of [ramp, lead]) {
    assert.deepEqual(approach.points.at(-1), eastbound.points[0]);
    assert.deepEqual(approach.seams.end, eastbound.seams.start);
  }

  const central = buildBakedRoadContact(ramp).surface;
  const us101 = buildBakedRoadContact(lead).surface;
  for (const [x, y, z] of [
    [3043.1, 1135, 17],
    [3037.8, 1154.3, 16.85],
    [3034.6, 1163.8, 16.5],
  ]) {
    const height = central.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - z) < 0.25);
    assert.equal(us101.heightAt(x, y), null);
  }
});

test("westbound I-80 keeps both US 101 choices on their surveyed branches", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const [west, southTile] = ["808f7e2c", "808f7e34"].map((token) => {
    const raw = file(`tiles/${token}.json.gz`);
    const receipt = roadBake.centralWestSplitSurvey.tiles[token];
    assert.equal(receipt.sha256, digest(raw));
    return JSON.parse(gunzipSync(raw));
  });
  const road = (slug, tile = west) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const approach = road("i80-westbound-interchange");
  const north = road("i80-westbound-central-connector");
  const south = road("i80-westbound-to-us101-south");
  assert.deepEqual(south, road("i80-westbound-to-us101-south", southTile));
  for (const [target, lanes] of [
    [north, [-5.5, 1]],
    [south, [1, 5.5]],
  ]) {
    assert.deepEqual(approach.points.at(-1), target.points[0]);
    assert.deepEqual(approach.seams.end, target.seams.start);
    const join = roadBake.playableTransitions.find(
      ({ from, to }) =>
        from === `${approach.id}:end` && to === `${target.id}:start`,
    );
    assert.deepEqual(join.from_lateral_range_m, lanes);
  }

  const northSurface = buildBakedRoadContact(north).surface;
  const southSurface = buildBakedRoadContact(south).surface;
  for (const [x, y, z] of [
    [3000.6, 1163, 15.4],
    [3004.3, 1146.1, 15.06],
    [2998.3, 1044.3, 17.13],
  ]) {
    const height = northSurface.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - z) < 0.25);
    assert.equal(southSurface.heightAt(x, y), null);
  }
});

test("US 101 reaches Potrero and joins the existing eastbound I-80 deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7e2c", "808f7e34", "808f7e4c", "808f7e54"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const byId = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (id) => byId.get(`sf-contact/${id}`);
  const incoming = road("us101-north-bayshore-through");
  const potrero = road("us101-north-potrero-approach");
  const exit = road("us101-north-potrero-exit");
  const i80Approach = road("us101-north-i80-approach");
  const ramp = road("us101-north-i80-east-connector");
  const i80 = road("i80-eastbound-interchange");
  const us101Lead = road("i80-eastbound-us101-approach");
  for (const [before, after] of [
    [incoming, potrero],
    [potrero, exit],
    [potrero, i80Approach],
    [i80Approach, ramp],
    [ramp, us101Lead],
    [us101Lead, i80],
    [road("central-freeway-i80-east-connector"), i80],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  const [x, y, z] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const [rampX, rampY, rampZ] = ramp.points[0];
  assert.ok(rampZ - groundAtWorld(tiles, rampX, rampY, true) > 9);
});

test("US 101 viaduct contact spans both tiles and forks to Ninth Street and Octavia", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = tiles.flatMap((tile) => tile.playableRoads);
  const road = (id) => roads.find(({ id: key }) => key === `sf-contact/${id}`);
  const viaduct = road("us101-north-central-viaduct");
  const exit = road("us101-north-ninth-bryant-exit");
  const merge = road("us101-north-central-merge");

  assert.deepEqual(
    tiles[1].playableRoads.find(({ id }) => id === viaduct.id),
    viaduct,
  );
  for (const [before, after] of [
    [road("us101-north-i80-approach"), viaduct],
    [viaduct, exit],
    [viaduct, merge],
    [merge, road("central-freeway-east-northbound")],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }

  const { surface } = buildBakedRoadContact(viaduct);
  const crossing = [2922.0769, 994.6148];
  assert.ok(surface.heightAt(...crossing) < 14.6);
  assert.ok(
    surface.heightAt(...crossing) - groundAtWorld(tiles, ...crossing, true) > 8,
  );
  assert.deepEqual(exit.groundEnds, ["end"]);
  const [x, y, z] = exit.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
});

test("SoMa ramps join the correct I-80 lanes and clear the surveyed Fifth Street pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["8085807c", "80858084", "808f7e2c", "808f7fd4"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const source = (name) => roads.get(`sf-contact/${name}`);
  const exit = source("i80-eastbound-fourth-bryant-exit");
  const fifth = source("i80-eastbound-fifth-bryant-entry");
  const bryant = source("i80-eastbound-bryant-entry");
  const harrison = source("i80-westbound-harrison-entry");
  const fourth = source("i80-eastbound-fourth");
  for (const [before, after] of [
    [fourth, exit],
    [bryant, fourth],
    [fifth, source("i80-eastbound-lower-climb")],
    [harrison, source("i80-westbound-interchange")],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const [ramp, end] of [
    [exit, "end"],
    [fifth, "start"],
    [bryant, "start"],
    [harrison, "start"],
  ]) {
    assert.deepEqual(ramp.groundEnds, [end]);
    const [x, y, z] = ramp.points.at(end === "start" ? 0 : -1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, ramp.id);
  }

  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const { surface } = buildBakedRoadContact(fifth);
  const [x, y] = [3516.59, 2126.49];
  const z = surface.heightAt(x, y);
  assert.ok(z > 5.6 && z < 5.9);
  assert.equal(collisionAt(indexes, { x, y, z, heading: 0.79 }, z), null);
  assert.ok(roadBake.overheadBuildingPatches["8085807c"] >= 1);
});

test("Seventh Street and southbound US 101 meet ordinary pavement only at their exits", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34", "808f7e4c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const seventh = road("i80-eastbound-seventh-off-ramp");
  const west = road("i80-eastbound-seventh-west-exit");
  const east = road("i80-eastbound-seventh-east-exit");
  const south = road("i80-westbound-to-us101-south");
  const main = road("us101-south-potrero-main");
  const fork = road("us101-south-potrero-exit");
  const approach = road("us101-south-cesar-potrero-approach");
  const cesar = road("us101-south-cesar-chavez-exit");
  const potrero = road("us101-south-potrero-avenue-exit");
  for (const [before, after] of [
    [road("i80-eastbound-interchange"), seventh],
    [seventh, west],
    [seventh, east],
    [road("i80-westbound-interchange"), south],
    [south, main],
    [main, fork],
    [fork, approach],
    [approach, cesar],
    [approach, potrero],
  ]) {
    assert.deepEqual(before.groundEnds, [], before.id);
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const exit of [west, east, cesar, potrero]) {
    assert.deepEqual(exit.groundEnds, ["end"], exit.id);
    const [x, y, z] = exit.points.at(-1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, exit.id);
  }
  for (const owner of south.owners) {
    const tile = tiles.find(({ t }) => t === owner);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === south.id),
      south,
    );
  }
});

test("Alemany keeps both I-280 merges connected and their city portals at street level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = [
    "808f7e4c",
    "808f7e54",
    "808f7e64",
    "808f7ef4",
    "808f7efc",
    "808f7f04",
  ];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const dogpatch = road("i280-south-beyond-dogpatch");
  const southMainline = road("i280-south-to-alemany-mainline");
  const tail = road("i280-south-alemany-merge-tail");
  const approach = road("us101-south-to-i280-south-approach");
  const merge = road("us101-south-to-i280-south-merge");
  const through = road("i280-south-alemany-through");
  const main = road("i280-south-alemany-main");
  const entry = road("i280-south-alemany-entry");
  const beyond = road("i280-south-beyond-alemany");
  const eastExit = road("us101-south-alemany-east-exit");
  const westExit = road("i280-south-alemany-exit");
  for (const [before, after] of [
    [dogpatch, southMainline],
    [southMainline, tail],
    [approach, merge],
    [approach, eastExit],
    [merge, through],
    [tail, through],
    [through, main],
    [through, westExit],
    [main, beyond],
    [entry, beyond],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0], after.id);
    assert.deepEqual(before.seams.end, after.seams.start, after.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(dogpatch.groundEnds, []);
  assert.deepEqual(entry.widthTransitionMetres, { end: 24 });
  for (const [ramp, end] of [
    [eastExit, "end"],
    [westExit, "end"],
    [entry, "start"],
  ]) {
    assert.deepEqual(ramp.groundEnds, [end]);
    const [x, y, z] = ramp.points.at(end === "start" ? 0 : -1);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, ramp.id);
  }
});

test("a narrow ramp tapers into the shared cross-section of a wider deck", () => {
  const seam = { bearing: 0, width: 12 };
  const ramp = {
    id: "narrow-ramp",
    kind: "ramp",
    width: 8,
    owners: ["8085807c", "8085807c"],
    points: Array.from({ length: 21 }, (_, i) => [i * 2, 0, 10]),
    seams: { end: seam },
    widthTransitionMetres: { end: 24 },
  };
  const deck = {
    ...ramp,
    id: "wide-deck",
    kind: "bridge_deck",
    width: 12,
    points: ramp.points.map(([x, y, z]) => [x + 40, y, z]),
    seams: { start: seam },
    widthTransitionMetres: {},
  };
  const { surface } = buildBakedRoadContact(ramp);
  const vertices = surface.contact.vertices;
  assert.equal(vertices[16][1], 4);
  assert.equal(vertices[28][1], 5);
  assert.equal(vertices.at(-2)[1], 6);
  assert.deepEqual(
    vertices.slice(-2),
    buildBakedRoadContact(deck).surface.contact.vertices.slice(0, 2),
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: {} }),
    null,
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: { start: 24 } }),
    null,
  );
  assert.equal(
    buildBakedRoadContact({ ...ramp, widthTransitionMetres: { end: 41 } }),
    null,
  );
});

test("measured road widths extend contact and move the matching highway rail", () => {
  const road = {
    id: "measured-fork",
    kind: "ramp",
    width: 8,
    owners: ["8085807c", "8085807c"],
    points: Array.from({ length: 21 }, (_, i) => [i * 2, 0, 10]),
    widthSections: [
      [0, 4, 4],
      [20, 7, 3],
      [40, 7, 3],
    ],
    seams: { end: { bearing: 0, width: 12, leftWidth: 8 } },
    widthTransitionMetres: { end: 12 },
    railSides: ["left"],
  };
  const { surface } = buildBakedRoadContact(road);
  assert.equal(surface.heightAt(10, 5.2), 10);
  assert.equal(surface.heightAt(10, 6), null);
  assert.equal(surface.heightAt(20, 6.5), 10);
  assert.equal(surface.heightAt(20, -3.5), null);
  assert.deepEqual(surface.contact.vertices.slice(-2), [
    [40, 8, 10],
    [40, -4, 10],
  ]);
  assert.deepEqual(surface.terminalHalfWidths, [4, 8]);
  assert.ok(
    highwayRailSegments(surface).some(({ a, b }) =>
      [a, b].some(([x, y]) => x === 20 && Math.abs(y - 6.65) < 0.001),
    ),
  );
  assert.equal(
    buildBakedRoadContact({
      ...road,
      widthSections: [
        [0, 4, 4],
        [41, 7, 3],
      ],
    }),
    null,
  );
  assert.equal(
    buildBakedRoadContact({
      ...road,
      widthSections: [
        [1, 4, 4],
        [1, 7, 3],
      ],
    }),
    null,
  );
});

test("tiled right-only guard rail bounces on the right and leaves the left edge open", () => {
  const road = {
    id: "test-highway-exit",
    kind: "ramp",
    owners: ["8085807c", "8085807c"],
    width: 6,
    points: Array.from({ length: 31 }, (_, x) => [x, 0, 10]),
    railSides: ["right"],
  };
  const { surface } = buildBakedRoadContact(road);
  surface.railSegments = highwayRailSegments(surface);
  assert.ok(surface.railSegments.length);
  assert.ok(surface.railSegments.every(({ a, b }) => a[1] < 0 && b[1] < 0));
  const left = { x: 15, y: 0.39, z: 10, v: 7, heading: Math.PI / 2 };
  const right = { ...left, y: -left.y, heading: -left.heading };
  assert.equal(strikeHighwayRail(left, {}, 1 / 120, surface), null);
  assert.equal(
    strikeHighwayRail(right, {}, 1 / 120, surface).edgeContact,
    "bounce",
  );
  assert.equal(
    leaveRoadSurface({ ...left, y: 2.99, grounded: true }, {}, 1 / 120, surface)
      .leavingRoad,
    true,
  );
  assert.equal(buildBakedRoadContact({ ...road, railSides: ["center"] }), null);
});

test("the upper West Span shares its Fremont fork with the westbound freeway continuation", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = [
    "80858014",
    "80858024",
    "8085803c",
    "8085806c",
    "80858074",
    "8085807c",
    "80858084",
  ];
  const roads = new Map(
    tokens.flatMap((token) => {
      const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
      return tile.playableRoads.map((road) => [road.id, road]);
    }),
  );
  const upper = roads.get("yerba-buena-upper-tunnel");
  const lower = roads.get("yerba-buena-lower-tunnel");
  assert.ok(buildBakedRoadContact(upper).appearance);
  assert.ok(buildBakedRoadContact(lower).appearance);
  assert.ok(upper.points[0][2] > lower.points[0][2] + 10);
  for (const ids of [
    ["city-approach", "fremont-exit"],
    [
      "bay-bridge-westbound",
      "city-approach",
      "sf-contact/i80-westbound-fremont-split",
      "sf-contact/i80-westbound-upper-soma",
      "sf-contact/i80-westbound-fourth-merge",
      "westbound-i80",
    ],
  ]) {
    for (const [index, next] of ids.slice(1).entries()) {
      const before = ids[index];
      const a = roads.get(before);
      const b = roads.get(next);
      assert.deepEqual(a.points.at(-1), b.points[0], next);
      assert.deepEqual(a.seams.end, b.seams.start, next);
      assert.ok(
        roadBake.playableTransitions.some(
          ({ from, to, direction }) =>
            from === `${before}:end` &&
            to === `${next}:start` &&
            direction === "forward",
        ),
      );
    }
  }
  const exit = roads.get("fremont-exit");
  assert.deepEqual(exit.groundEnds, ["end"]);
  assert.equal(exit.railRanges.length, 1);
  const [rail] = exit.railRanges;
  assert.equal(rail.side, "right");
  assert.equal(rail.startMetres, 90);
  const remaining =
    buildBakedRoadContact(exit).surface.profile.at(-1).s_m - rail.endMetres;
  assert.ok(remaining > 0 && remaining < 5);
});

test("the Fremont exit uses the measured lane in both free drive and the named Bay Bridge drive", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roadRecords = ["80858074", "8085807c"].flatMap(
    (token) =>
      JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))).playableRoads,
  );
  const roads = new Map(roadRecords.map((road) => [road.id, road]));
  const {
    drives: [drive],
  } = JSON.parse(file("gamedrives.json"));
  for (const definition of [
    drive,
    cityDriveApproach(drive, "yerba-buena-westbound"),
  ]) {
    const named = buildCityGameDrive(definition, () => 0);
    for (const id of [
      "bay-bridge-westbound",
      "city-approach",
      "fremont-exit",
    ]) {
      const measured = buildBakedRoadContact(roads.get(id)).surface;
      const surface = named.surfaces.find((candidate) => candidate.id === id);
      assert.deepEqual(surface.contact, measured.contact, id);
      assert.deepEqual(surface.railRanges, measured.railRanges, id);
    }
  }
  const exit = buildBakedRoadContact(roads.get("fremont-exit")).surface;
  for (const [x, y, observed] of [
    [4213.89, 2821.37, 40.13],
    [4113.51, 2785.36, 35.4],
    [4038.94, 2796.74, 31.02],
    [4003.31, 2814.7, 29.27],
    [3969.63, 2842.89, 27.57],
  ]) {
    const height = exit.heightAt(x, y);
    assert.ok(Number.isFinite(height) && Math.abs(height - observed) < 0.08);
  }
  const branches = new Map(
    roadBake.playableTransitions
      .filter(({ from }) => from === "city-approach:end")
      .map(({ to, from_lateral_range_m }) => [to, from_lateral_range_m]),
  );
  for (const [lane, target] of [
    [-4.5, "fremont-exit:start"],
    [1.3, "sf-contact/i80-westbound-fremont-split:start"],
  ]) {
    assert.deepEqual(
      [...branches]
        .filter(([, [left, right]]) => lane >= left && lane <= right)
        .map(([id]) => id),
      [target],
    );
  }
});

test("the westbound deck holds the sampled outside lanes above their lower levels", () => {
  const { roadBake, tiles: entries } = JSON.parse(file("tiles/manifest.json"));
  const review = roadBake.i80WestSpanWidthSurvey;
  const roads = new Map();
  const collision = [];
  for (const [token, receipt] of Object.entries(review.tiles)) {
    const raw = file(`tiles/${token}.json.gz`);
    const fremont = roadBake.i80FremontAlignmentSurvey.tiles[token];
    if (fremont) assert.equal(fremont.parentSha256, receipt.sha256);
    const previous = fremont?.sha256 ?? receipt.sha256;
    const edge = roadBake.westboundCityApproachEdgeSurvey.tiles[token];
    if (edge) assert.equal(edge.parentSha256, previous);
    assertLaterRoadTileReceipt(roadBake, token, edge?.sha256 ?? previous, raw);
    assert.equal(raw.length, entries.find(({ t }) => t === token).bytes);
    const tile = JSON.parse(gunzipSync(raw));
    collision.push(buildCollisionIndex(tile, token));
    for (const road of tile.playableRoads) {
      if (roads.has(road.id)) assert.deepEqual(roads.get(road.id), road);
      roads.set(road.id, road);
    }
  }

  const lower = buildBakedRoadContact(
    roads.get("bay-bridge-eastbound-lower"),
  ).surface;
  for (const [id, station, offset] of [
    ["bay-bridge-westbound", 325, -9],
    ["bay-bridge-westbound", 1230, -9],
    ["bay-bridge-westbound", 1476, -9],
    ["bay-bridge-westbound", 2460, -9],
    ["city-approach", 6, -10],
    ["city-approach", 163, -10],
    ["city-approach", 290, -10],
    ["city-approach", 390, -10],
    ["sf-contact/i80-westbound-fremont-split", 58, -10],
    ["sf-contact/i80-westbound-upper-soma", 124, -10],
  ]) {
    const { surface } = buildBakedRoadContact(roads.get(id));
    const i = surface.profile.findIndex(({ s_m }) => s_m >= station);
    const {
      position: [x, y, z],
    } = surface.profile[i];
    const a = surface.profile[i - 1].position;
    const b = surface.profile[i + 1].position;
    const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
    const car = {
      x: x - offset * Math.sin(heading),
      y: y + offset * Math.cos(heading),
      z,
      heading,
    };
    assert.ok(roadSupportsCar(surface.heightAt, car), `${id} at ${station} m`);
    assert.equal(collisionAt(collision, car, z), null);
    if (id === "bay-bridge-westbound") {
      const below = lower.heightAt(car.x, car.y);
      assert.ok(Number.isFinite(below) && z - below > 8);
    }
  }
});

test("the westbound outside lane stays on the city approach deck before Fremont", () => {
  const tiles = ["80858074", "8085807c"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const road = tiles[0].playableRoads.find(({ id }) => id === "city-approach");
  assert.deepEqual(
    tiles[1].playableRoads.find(({ id }) => id === road.id),
    road,
  );
  const { surface } = buildBakedRoadContact(road);
  const indices = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  for (const [x, y, height] of [
    [4315.615, 2893.522, 44.243],
    [4303.072, 2882.353, 43.846],
    [4296.864, 2877.573, 43.6],
  ]) {
    const car = { x, y, heading: -2.44 };
    const z = surface.heightAt(x, y);
    assert.ok(Number.isFinite(z) && Math.abs(z - height) < 0.5);
    assert.ok(roadSupportsCar(surface.heightAt, { ...car, z }));
    assert.equal(collisionAt(indices, car, z), null);
  }
  assert.equal(
    collisionAt(indices, { x: 4303.072, y: 2882.353, heading: -2.44 }, 30),
    "building",
  );
});

test("the east island ramp joins the upper bore without changing the through tunnel level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["80858024", "8085803c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/yerba-buena-east-westbound-on-ramp");
  const entranceBore = roads.get(
    "sf-contact/yerba-buena-east-westbound-entrance-tunnel",
  );
  const mainBore = roads.get("yerba-buena-upper-tunnel");
  const sharedBore = roads.get("yerba-buena-upper-tunnel-west-of-merge");
  const west = roads.get("yerba-buena-west-approach");
  assert.deepEqual(ramp.groundEnds, ["start"]);
  assert.equal(buildBakedRoadContact(ramp).appearance, null);
  assert.ok(buildBakedRoadContact(entranceBore).appearance);
  assert.ok(buildBakedRoadContact(sharedBore).appearance);
  for (const [before, after] of [
    [ramp, entranceBore],
    [entranceBore, sharedBore],
    [mainBore, sharedBore],
    [sharedBore, west],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const shells = [mainBore, entranceBore, sharedBore].map((road) => {
    const { surface, appearance } = buildBakedRoadContact(road);
    return tunnelMesh(surface, appearance);
  });
  const originalMainBore = buildBakedRoadContact(mainBore);
  const uncut = tunnelMesh(originalMainBore.surface, {
    ...originalMainBore.appearance,
    wallOpenings: {},
  });
  const eye = ([x, y, z]) => new Vector3(x, z + 1.0, -y);
  let uncutHits = 0;
  for (const { points } of [entranceBore, mainBore]) {
    for (let i = 1; i < points.length; i++) {
      const start = eye(points[i - 1]);
      const delta = eye(points[i]).sub(start);
      const end = delta.length();
      const ray = new Raycaster(start, delta.normalize(), 0, end);
      assert.equal(
        ray.intersectObjects(shells).length,
        0,
        `wall across tunnel at ${points[i]}`,
      );
      uncutHits += ray.intersectObject(uncut).length;
    }
  }
  assert.ok(uncutHits > 0, "the earlier wall would cross the entrance lane");
  const viewpoint = eye(mainBore.points[5]).lerp(eye(mainBore.points[6]), 0.5);
  const vertices = originalMainBore.surface.contact.vertices;
  const outerWall = eye(vertices[10]).lerp(eye(vertices[12]), 0.5);
  const sideways = outerWall.sub(viewpoint).normalize();
  const wall = new Raycaster(viewpoint, sideways, 0, 12);
  assert.ok(
    wall.intersectObject(shells[0]).length,
    "outside the merge, the outer wall remains",
  );
  const product = JSON.parse(file("gamedrives.json"));
  const route = cityDriveApproach(product.drives[0], "yerba-buena-westbound");
  for (const road of [mainBore, sharedBore]) {
    const guide = route.sections.find(({ id }) => id === road.id);
    assert.deepEqual(guide.positions, road.points);
    assert.deepEqual(guide.tunnel_wall_openings_m, road.tunnel_wall_openings_m);
  }
});

test("the lower island exit stays below the upper bore and reaches the island street", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const entry = roads.get("yerba-buena-lower-tunnel");
  const through = roads.get("yerba-buena-lower-tunnel-east-of-exit");
  const exit = roads.get("sf-contact/yerba-buena-east-eastbound-exit-tunnel");
  const ramp = roads.get("sf-contact/yerba-buena-east-eastbound-off-ramp");
  const feeder = roads.get(
    "sf-contact/yerba-buena-eastbound-exit-to-southgate",
  );
  const street = roads.get(
    "sf-contact/yerba-buena-southgate-to-macalla-shared",
  );
  const departure = feeder ? street : ramp;
  for (const [before, after] of [
    [entry, through],
    [entry, exit],
    [exit, ramp],
    ...(feeder
      ? [
          [ramp, feeder],
          [feeder, street],
        ]
      : []),
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.ok(exit.points[0][2] < 49);
  assert.deepEqual(departure.groundEnds, ["end"]);
  if (feeder) assert.deepEqual(ramp.groundEnds, []);
  const [x, y, z] = departure.points.at(-1);
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  const shells = [through, exit].map((road) => {
    const { surface, appearance } = buildBakedRoadContact(road);
    return tunnelMesh(surface, appearance);
  });
  for (const { points } of [through, exit]) {
    for (let i = 1; i < points.length; i++) {
      const eye = ([x, y, z]) => new Vector3(x, z + 1, -y);
      const start = eye(points[i - 1]);
      const delta = eye(points[i]).sub(start);
      const length = delta.length();
      const ray = new Raycaster(start, delta.normalize(), 0, length);
      assert.equal(
        ray.intersectObjects(shells).length,
        0,
        `wall across lower tunnel at ${points[i]}`,
      );
    }
  }
});

test("the upper westbound Yerba Buena deck reaches Macalla on surveyed pavement", () => {
  const token = "80858024";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assertSouthgateReview(roadBake, token, raw);
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const early = roads.get("sf-contact/yerba-buena-upper-east-through");
  const late = roads.get("yerba-buena-east-approach");
  const entrance = roads.get("sf-contact/macalla-road-westbound-i80-entrance");
  const common = roads.get("sf-contact/yerba-buena-upper-after-macalla-merge");
  assert.deepEqual(early.points.at(-1), late.points[0]);
  assert.deepEqual(early.seams.end, late.seams.start);
  for (const road of [late, entrance]) {
    assert.deepEqual(road.points.at(-1), common.points[0]);
    assert.deepEqual(road.seams.end, common.seams.start);
  }

  const east = buildBakedRoadContact(early).surface;
  const west = buildBakedRoadContact(late).surface;
  const nearBend =
    east.heightAt(6757.1, 5768.1) ?? west.heightAt(6757.1, 5768.1);
  assert.ok(Number.isFinite(nearBend) && Math.abs(nearBend - 60.82) < 0.2);
  const merge = buildBakedRoadContact(common).surface;
  assert.ok(
    roadSupportsCar((x, y) => west.heightAt(x, y) ?? merge.heightAt(x, y), {
      x: 6628.435,
      y: 5606.129,
      heading: common.seams.start.bearing,
    }),
  );

  const driveBytes = file("gamedrives.json");
  assert.equal(digest(driveBytes), roadBake.driveSha256);
  const { drives } = JSON.parse(driveBytes);
  const sections = drives.flatMap(({ approaches }) =>
    (approaches ?? []).flatMap(({ sections }) => sections),
  );
  for (const road of [early, late]) {
    const named = sections.filter(({ id }) => id === road.id);
    assert.equal(named.length, 2);
    for (const section of named)
      assert.deepEqual(section.positions, road.points);
  }
});

test("Hillcrest stays in its own surveyed lane to the East Span merge", () => {
  const token = "80858024";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const receipt = roadBake.hillcrestEastSpanSurvey.tiles[token];
  const westbound = roadBake.ybiUpperWestSurvey.tiles[token];
  assert.equal(westbound.parentSha256, receipt.sha256);
  assertSouthgateReview(roadBake, token, raw);
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const entry = roads.get("sf-contact/yerba-buena-east-eastbound-on-ramp");
  const lower = roads.get("yerba-buena-lower-east-after-entry");
  const span = roads.get("sf-contact/bay-bridge-east-span-eastbound");
  for (const road of [entry, lower]) {
    assert.deepEqual(road.points.at(-1), span.points[0]);
    assert.deepEqual(road.seams.end, span.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${road.id}:end` &&
          edge.to === `${span.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }

  const { surface, portals } = buildBakedRoadContact(entry);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  const witnesses = [
    [6754.1, 5707.1, 51.4],
    [6783.7, 5747.4, 51.9],
    [6837.3, 5819.7, 52.5],
  ];
  const lowerContact = buildBakedRoadContact(lower).surface;
  for (const [x, y, z] of witnesses) {
    assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.3);
    assert.equal(lowerContact.heightAt(x, y), null);
    assert.ok(z - groundAtWorld([tile], x, y, true) > 8);
  }

  const scenery = [buildCollisionIndex(tile, token)];
  for (const [i, { position, s_m }] of surface.profile.slice(1).entries()) {
    const previous = surface.profile[i];
    const [x, y, z] = position;
    const heading = Math.atan2(
      y - previous.position[1],
      x - previous.position[0],
    );
    assert.ok(Math.abs(z - previous.position[2]) / (s_m - previous.s_m) < 0.04);
    for (const lane of [-1.4, 0, 1.4]) {
      const car = {
        x: x - lane * Math.sin(heading),
        y: y + lane * Math.cos(heading),
        heading,
      };
      assert.equal(collisionAt(scenery, car, z), null, `Hillcrest ${s_m} m`);
    }
  }
});

test("23rd Street crosses both US 101 carriageways at its own level", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const other = JSON.parse(gunzipSync(file("tiles/808f7e34.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/potrero-23rd-us101-overpass",
  );
  const { surface } = buildBakedRoadContact(road);
  assert.deepEqual(road.groundEnds, ["start", "end"]);
  for (const [x, y, z] of [road.points[0], road.points.at(-1)]) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  }
  for (const id of [
    "sf-contact/us101-north-potrero-approach",
    "sf-contact/us101-south-potrero-main",
  ]) {
    const lower = tile.playableRoads.find((entry) => entry.id === id);
    assert.deepEqual(
      lower,
      other.playableRoads.find((entry) => entry.id === id),
    );
    const [x, y, z] = lower.points.reduce((best, point) =>
      Math.abs(point[1] + 611) < Math.abs(best[1] + 611) ? point : best,
    );
    const level = surface.heightAt(x, y);
    assert.ok(level - z > 6.5 && level - z < 8, id);
  }
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.ok(roadBake.coveredGroundRoadPatches[tile.t]);
});

test("Evans Avenue retains its measured grade across the rebaked terrain strip", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7fac.json.gz")));
  const point = [3797.04, -1228.795];
  const end = [3760.65, -1133.32];
  const length = Math.hypot(end[0] - point[0], end[1] - point[1]);
  for (const offset of [-6, 0, 6]) {
    const heights = Array.from({ length: 102 }, (_, station) => {
      const x =
        point[0] +
        (station * (end[0] - point[0]) + offset * (end[1] - point[1])) / length;
      const y =
        point[1] +
        (station * (end[1] - point[1]) - offset * (end[0] - point[0])) / length;
      return groundAtWorld([tile], x, y, true);
    });
    assert.ok(heights.every((z) => z !== null));
    assert.ok(Math.min(...heights.slice(44, 65)) > 13.5);
    assert.ok(
      heights.slice(1).every((z, i) => Math.abs(z - heights[i]) < 0.12),
    );
  }
});

test("Central Freeway joins the lower Bay Bridge through contiguous eastbound contact", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7e2c",
    "80858084",
    "808f7fd4",
    "8085807c",
    "80858074",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const chain = [
    "sf-contact/i80-eastbound-interchange",
    "sf-contact/i80-eastbound-midtown",
    "sf-contact/i80-eastbound-fourth",
    "sf-contact/i80-eastbound-soma",
    "sf-contact/i80-eastbound-lower-climb",
    "sf-contact/i80-eastbound-essex-merge",
    "sf-eastbound-lower-approach",
  ].map((id) => roads.get(id));
  for (const [index, road] of chain.slice(1).entries()) {
    const previous = chain[index];
    assert.ok(buildBakedRoadContact(road), road.id);
    assert.deepEqual(road.groundEnds, [], road.id);
    assert.deepEqual(previous.points.at(-1), road.points[0], road.id);
    assert.deepEqual(previous.seams.end, road.seams.start, road.id);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${previous.id}:end` &&
          to === `${road.id}:start` &&
          direction === "forward",
      ),
      road.id,
    );
  }
  const merge = chain.at(-2);
  const upper = buildBakedRoadContact(
    roads.get("sf-contact/i80-westbound-upper-soma"),
  ).surface;
  const [x, y, lowerZ] = merge.points[10];
  const levelGap = upper.heightAt(x, y) - lowerZ;
  assert.ok(levelGap > 8 && levelGap < 10);
});

test("Yerba Buena connects continuously through the upper tunnel to the original bridge drive", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const entry = roadBake.driveApproaches.find(
    ({ id }) => id === "yerba-buena-westbound",
  );
  const selected = cityDriveApproach(drive, entry.id);
  assert.equal(selected.label, entry.label);
  assert.equal(selected.initial_speed_mps, 50 * 0.44704);
  assert.equal(cityDriveApproach(drive, "fremont-exit"), drive);
  assert.equal(drive.sections[0].id, "bay-bridge-westbound");
  assert.deepEqual(selected.sections.slice(8), drive.sections);
  const [
    outerSpan,
    span,
    through,
    approach,
    merged,
    firstBore,
    sharedBore,
    exit,
    bridge,
  ] = selected.sections;
  assert.equal(outerSpan.id, "sf-contact/bay-bridge-east-span-westbound");
  assert.equal(span.id, "sf-contact/yerba-buena-upper-east-span");
  assert.equal(through.id, "sf-contact/yerba-buena-upper-east-through");
  const tunnels = [firstBore, sharedBore];
  for (const tunnel of tunnels) {
    assert.equal(tunnel.kind, "tunnel");
    assert.equal(tunnel.game_clearance_m, 4.4);
    assert.equal(tunnel.width_m, 16);
  }
  for (const [before, after] of [
    [outerSpan, span],
    [span, through],
    [through, approach],
    [approach, merged],
    [merged, firstBore],
    [firstBore, sharedBore],
    [sharedBore, exit],
    [exit, bridge],
  ]) {
    assert.deepEqual(before.positions.at(-1), after.positions[0]);
  }
  let length = 0;
  for (const { positions } of tunnels) {
    for (let i = 1; i < positions.length; i++) {
      const [ax, ay, az] = positions[i - 1];
      const [bx, by, bz] = positions[i];
      const ds = Math.hypot(bx - ax, by - ay);
      assert.ok(Math.abs(bz - az) / ds < 0.045);
      length += ds;
    }
  }
  assert.ok(length > 164 && length < 166);
});

test("both East Span carriageways connect to Yerba Buena on separate surveyed contacts", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const east = roads.get("sf-contact/bay-bridge-east-span-eastbound");
  const west = roads.get("sf-contact/bay-bridge-east-span-westbound");
  const lower = roads.get("yerba-buena-lower-east-after-entry");
  const upper = roads.get("sf-contact/yerba-buena-upper-east-span");

  for (const [before, after] of [
    [lower, east],
    [west, upper],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }

  assert.ok(lower.points.at(-1)[2] > 53.6 && lower.points.at(-1)[2] < 54);
  assert.ok(east.points.at(-1)[0] >= 7630);
  assert.ok(west.points[0][0] >= 7630);
  assert.ok(west.points[0][1] - east.points.at(-1)[1] > 40);
  for (const [road, opposing] of [
    [east, west],
    [west, east],
  ]) {
    assert.equal(road.width, 20);
    assert.deepEqual(road.groundEnds, []);
    assert.ok(
      road.profiles.every((id) => roadBake.playableReplaces.includes(id)),
    );
    const { surface } = buildBakedRoadContact(road);
    assert.ok(surface.profile.at(-1).s_m > 780);
    const [x, y, z] = road.points[Math.floor(road.points.length / 2)];
    const [a, b] = road.points[Math.floor(road.points.length / 2) - 1];
    const heading = Math.atan2(y - b, x - a);
    for (const offset of [-8, 0, 8]) {
      const height = surface.heightAt(
        x - offset * Math.sin(heading),
        y + offset * Math.cos(heading),
      );
      assert.ok(Math.abs(height - z) < 0.1);
    }
    const [oppositeX, oppositeY] =
      opposing.points[Math.floor(opposing.points.length / 2)];
    assert.equal(surface.heightAt(oppositeX, oppositeY), null);
    for (let i = 1; i < road.points.length; i++) {
      const [ax, ay, az] = road.points[i - 1];
      const [bx, by, bz] = road.points[i];
      assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.025);
    }
  }
});

test("both East Span carriageways connect to Oakland with four complete cross-tile copies", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const coverage = JSON.parse(file("coverage.json"));
  const owners = ["80857f8c", "80857f94", "80857fbc", "80857fb4"];
  const tiles = ["80858024", ...owners].map((token) => {
    assert.ok(manifest.tiles.some(({ t }) => t === token));
    assert.equal(coverage.cells[token].length, 4);
    return JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  });
  const contact = (tile, suffix) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${suffix}`);
  const drive = JSON.parse(file("gamedrives.json")).drives[0];
  for (const direction of ["eastbound", "westbound"]) {
    const fromOakland = direction === "westbound";
    const name = `bay-bridge-oakland-${direction}`;
    const road = contact(tiles.at(-1), name);
    const inner = contact(tiles[0], `bay-bridge-east-span-${direction}`);
    const [before, after] = fromOakland ? [road, inner] : [inner, road];
    assert.deepEqual(
      inner.owners,
      fromOakland ? ["80857f8c", "80858024"] : ["80858024", "80857f8c"],
    );
    assert.deepEqual(
      contact(tiles[1], `bay-bridge-east-span-${direction}`),
      inner,
    );
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      manifest.roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
    for (const tile of tiles.slice(1)) {
      assert.deepEqual(contact(tile, name), road);
    }
    const end = fromOakland ? "start" : "end";
    const [east, north, height] = road.points.at(fromOakland ? 0 : -1);
    assert.deepEqual(road.groundEnds, [end]);
    assert.ok(
      Math.abs(groundAtWorld(tiles, east, north, true) - height) < 0.02,
    );
    if (fromOakland) {
      const route = cityDriveApproach(drive, "oakland-westbound");
      assert.deepEqual(route.sections[0].positions, road.points);
      assert.equal(route.initial_speed_mps, 22.352);
      assert.ok(route.sections.some(({ id }) => id === "fremont-exit"));
      assert.ok(
        route.sections.some(({ id }) => id === "fourth-street-entrance"),
      );
      assert.equal(route.sections.at(-1).id, "westbound-i80");
    } else {
      for (const start of ["yerba-buena-eastbound", "essex-eastbound"]) {
        const route = cityDriveApproach(drive, start);
        assert.deepEqual(route.sections.at(-1).positions, road.points);
      }
    }
  }
});

test("the upper East Span forks to Macalla Road without crossing the lower deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/80858024.json.gz")));
  const roads = new Map(tile.playableRoads.map((road) => [road.id, road]));
  const road = (suffix) => roads.get(`sf-contact/${suffix}`);
  const stem = road("yerba-buena-upper-east-span");
  const through = road("yerba-buena-upper-east-through");
  const ramp = road("yerba-buena-east-westbound-off-ramp");
  const upper = roads.get("yerba-buena-east-approach");
  for (const [before, after] of [
    [stem, through],
    [stem, ramp],
    [through, upper],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface: approach } = buildBakedRoadContact(stem);
  assert.ok(approach.profile.at(-1).s_m >= 140);
  const index = buildCollisionIndex(tile, tile.t);
  for (const [
    i,
    {
      position: [x, y, z],
    },
  ] of approach.profile.slice(1).entries()) {
    const [priorX, priorY] = approach.profile[i].position;
    const heading = Math.atan2(y - priorY, x - priorX);
    for (const lateral of [-2.2, 0, 2.2]) {
      const car = {
        x: x - lateral * Math.sin(heading),
        y: y + lateral * Math.cos(heading),
        heading,
      };
      assert.equal(
        collisionAt([index], car, z),
        null,
        `East Span at ${x}, ${y}`,
      );
    }
  }
  const overhead = { x: 6937.2, y: 5993.1, heading: -2.4 };
  assert.equal(collisionAt([index], overhead, 65), "building");
  const { surface: exit, portals } = buildBakedRoadContact(ramp);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.005);
  assert.ok(exit.profile[0].position[2] > 60);
  assert.ok(exit.profile.at(-1).position[2] > 54);
  assert.ok(exit.profile.at(-1).s_m > 310);
});

test("the eastbound bridge start connects First Street to Oakland through the lower bore", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const entry = roadBake.driveApproaches.find(
    ({ id }) => id === "yerba-buena-eastbound",
  );
  const selected = cityDriveApproach(drive, entry.id);
  assert.equal(selected.label, entry.label);
  assert.equal(selected.sections[0].id, "first-street-eastbound-entry");
  assert.equal(drive.sections[0].id, "bay-bridge-westbound");
  const tunnels = selected.sections.filter(({ kind }) => kind === "tunnel");
  assert.deepEqual(
    tunnels.map(({ id }) => id),
    ["yerba-buena-lower-tunnel", "yerba-buena-lower-tunnel-east-of-exit"],
  );
  let length = 0;
  for (const tunnel of tunnels) {
    assert.equal(tunnel.game_clearance_m, 4.9);
    for (let i = 1; i < tunnel.positions.length; i++) {
      const [ax, ay, az] = tunnel.positions[i - 1];
      const [bx, by, bz] = tunnel.positions[i];
      const ds = Math.hypot(bx - ax, by - ay);
      assert.ok(Math.abs(bz - az) / ds < 0.05);
      length += ds;
    }
  }
  assert.ok(length > 286 && length < 288);
  assert.ok(
    tunnels[0].positions[0][2] < 44 && tunnels.at(-1).positions.at(-1)[2] > 51,
  );
  for (let i = 1; i < selected.sections.length; i++) {
    assert.deepEqual(
      selected.sections[i - 1].positions.at(-1),
      selected.sections[i].positions[0],
    );
  }
  assert.equal(
    selected.sections.at(-2).id,
    "sf-contact/bay-bridge-east-span-eastbound",
  );
  assert.equal(
    selected.sections.at(-1).id,
    "sf-contact/bay-bridge-oakland-eastbound",
  );
});

test("Essex and First Street enter the same lower deck from ordinary city pavement", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const drive = JSON.parse(file("gamedrives.json")).drives.find(
    ({ id }) => id === BAY_BRIDGE_CITY_DRIVE,
  );
  const essex = cityDriveApproach(drive, "essex-eastbound");
  const first = cityDriveApproach(drive, "yerba-buena-eastbound");
  assert.equal(essex.sections[0].id, "essex-street-eastbound-entry");
  assert.deepEqual(essex.sections.slice(1), first.sections.slice(1));
  const city = JSON.parse(gunzipSync(file("tiles/8085807c.json.gz")));
  const deck = city.playableRoads.find(
    ({ id }) => id === "sf-eastbound-lower-approach",
  );
  for (const route of [essex, first]) {
    const ramp = city.playableRoads.find(
      ({ id }) => id === route.sections[0].id,
    );
    assert.deepEqual(ramp.points.at(-1), deck.points[0]);
    assert.deepEqual(ramp.seams.end, deck.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${ramp.id}:end` && edge.to === `${deck.id}:start`,
      ),
    );
    const { portals } = buildBakedRoadContact(ramp);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld([city], x, y, true) - z) < 0.15);
    if (route === first) {
      const [startX, startY] = first.sections[0].positions[0];
      assert.ok(Math.hypot(x - startX, y - startY) > 7);
    }
  }
});

test("ordinary city tiles carry the complete lower West Span and Yerba Buena bore", () => {
  const readRoad = (token, id) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))).playableRoads.find(
      (road) => road.id === id,
    );
  const westId = "bay-bridge-eastbound-lower";
  const west = readRoad("80858074", westId);
  for (const owner of ["80858014", "8085803c", "8085806c"])
    assert.deepEqual(readRoad(owner, westId), west);
  const boreId = "yerba-buena-lower-tunnel";
  const bore = readRoad("80858024", boreId);
  assert.deepEqual(readRoad("8085803c", boreId), bore);
  const { appearance, portals } = buildBakedRoadContact(bore);
  assert.equal(appearance.clearance, 4.9);
  const continuation = readRoad(
    "80858024",
    "yerba-buena-lower-tunnel-east-of-exit",
  );
  assert.deepEqual(bore.points.at(-1), continuation.points[0]);
  assert.ok(
    appearance.interior[1] +
      buildBakedRoadContact(continuation).appearance.interior[1] >
      286,
  );
  assert.deepEqual(portals, []);
});

test("bundled road runs have identical geometry across tile owners", () => {
  const manifestBytes = file("tiles/manifest.json");
  const { tiles, roadBake, terrainReview } = JSON.parse(manifestBytes);
  const driveBytes = file("gamedrives.json");
  assert.equal(digest(driveBytes), roadBake.driveSha256);
  assert.equal(
    JSON.parse(driveBytes).base_manifest_sha256,
    roadBake.baseSha256,
  );
  assert.equal(
    JSON.parse(file("quest-route.json")).manifestSha256,
    digest(manifestBytes),
  );
  const runs = new Map();
  const seamKinds = new Set();
  for (const { t, bytes } of tiles) {
    const tile = file(`tiles/${t}.json.gz`);
    assert.equal(tile.length, bytes, t);
    for (const run of JSON.parse(gunzipSync(tile)).raisedRoads || []) {
      assert.ok(run.points.length > 1, run.id);
      assert.ok(
        run.points.every(
          (point) => point.length === 3 && point.every(Number.isFinite),
        ),
        run.id,
      );
      if (runs.has(run.id)) {
        assert.deepEqual(run, runs.get(run.id), run.id);
        seamKinds.add(run.kind);
      }
      runs.set(run.id, run);
    }
  }
  const reviewedRoads = new Set(terrainReview?.addedRoads);
  for (const id of reviewedRoads) assert.ok(runs.has(id), id);
  assert.equal(runs.size, roadBake.runs + reviewedRoads.size);
  assert.deepEqual(seamKinds, new Set(["bridge", "overpass"]));
});

test("the reviewed decks replace the provisional spans and clear the lower I-280 camera", () => {
  const { tiles, roadBake } = JSON.parse(file("tiles/manifest.json"));
  const replaced = new Set(roadBake.playableReplaces);
  const previews = new Map();
  const roads = new Map();
  for (const { t } of tiles) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`)));
    for (const preview of tile.raisedRoads ?? [])
      previews.set(preview.id, preview);
    for (const road of tile.playableRoads ?? []) roads.set(road.id, road);
  }
  const decks = deckClearanceIndex();
  for (const road of roads.values()) {
    if (road.kind === "tunnel") continue;
    const { surface } = buildBakedRoadContact(road);
    const geometry = contactGeometry(surface.contact, false);
    decks.add(geometry);
    geometry.dispose();
  }
  // The surveyed westbound deck follows a different footprint from these previews.
  for (const run of ["14214000/forward/0", "14244000/forward/0"]) {
    const preview = previews.get(`overpass/${run}`);
    assert.ok(preview, run);
    assert.equal(buildRaisedRoad(preview, replaced), null, run);
  }
  for (const run of [
    "14241000/forward/0",
    "14245000/forward/0",
    "19365000/forward/0",
    "19304000/forward/0",
    "19304000/forward/1",
    "19337000/forward/0",
    "19342000/forward/0",
  ]) {
    const preview = previews.get(`overpass/${run}`);
    assert.ok(preview, run);
    assert.equal(buildRaisedRoad(preview, replaced), null, run);
    for (const [x, y, z] of preview.points) {
      assert.ok(Math.abs(decks.ceiling(x, y, z - 1.2) - z) < 0.8, run);
    }
  }
  const { surface } = buildBakedRoadContact(
    roads.get("sf-contact/i280-north-lower-viaduct"),
  );
  const car = { x: 3675.605, y: -2069.69, heading: 0.9486 };
  const z = surface.heightAt(car.x, car.y);
  assert.ok(Number.isFinite(z));
  assert.equal(overheadDeckBlend(car, z, surface.heightAt, decks.ceiling), 0);
  const lowerBay = buildBakedRoadContact(
    roads.get("yerba-buena-lower-east-approach"),
  ).surface;
  const islandCar = { x: 6676.4267, y: 5658.1896, heading: 0.8536 };
  const islandZ = lowerBay.heightAt(islandCar.x, islandCar.y);
  assert.ok(Number.isFinite(islandZ));
  assert.ok(decks.ceiling(islandCar.x, islandCar.y, islandZ) - islandZ > 7);
  assert.ok(
    overheadDeckBlend(islandCar, islandZ, lowerBay.heightAt, decks.ceiling) >
      0.7,
  );
  const obsolete = buildRaisedRoad(
    previews.get("overpass/19304000/forward/1"),
    new Set(),
  );
  decks.add(obsolete.geometry);
  obsolete.geometry.dispose();
  obsolete.material.dispose();
  assert.ok(overheadDeckBlend(car, z, surface.heightAt, decks.ceiling) > 0.7);
});

test("the rebake keeps tunnel alignments off the aerial ground while retaining Tunnel Avenue", () => {
  const { tiles, terrainRebake } = JSON.parse(file("tiles/manifest.json"));
  const tunnelNames = new Set([
    "Robert C Levy Tunnel",
    "Stockton Tunnel",
    "Toll Plaza Tunnel",
  ]);
  assert.equal(
    Object.values(terrainRebake.removedTunnelRows).reduce((a, b) => a + b),
    4,
  );
  let tunnelAvenue = false;
  for (const { t } of tiles) {
    const { n, r } = JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`)));
    for (const [index] of r) {
      assert.ok(!tunnelNames.has(n[index]), t);
      if (n[index] === "Tunnel Ave") tunnelAvenue = true;
    }
  }
  assert.ok(tunnelAvenue);
});

test("baked roads agree across tile owners and reach their declared ground portals", () => {
  const { tiles, roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.match(roadBake.playableSha256, /^[0-9a-f]{64}$/);
  const roads = new Map();
  const duplicated = new Set();
  const terminals = new Map();
  const ownerCopies = new Map();
  const loaded = tiles.map(({ t }) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  for (const { t, playableRoads = [] } of loaded) {
    for (const road of playableRoads) {
      if (roads.has(road.id)) {
        assert.deepEqual(road, roads.get(road.id), road.id);
        duplicated.add(road.id);
      } else {
        ownerCopies.set(road.id, new Set());
      }
      roads.set(road.id, road);
      ownerCopies.get(road.id).add(t);
      terminals.set(`${road.id}:start`, [road.points[0], road.seams?.start]);
      terminals.set(`${road.id}:end`, [road.points.at(-1), road.seams?.end]);
    }
  }
  const entry = roads.get("sf-contact/i280-fifth-south-entry");
  assert.deepEqual(entry.groundEnds, ["start"]);
  assert.ok(duplicated.has(entry.id));
  const joinedEnds = new Set();
  for (const { from, to } of roadBake.playableTransitions) {
    const terminal = terminals.get(from);
    assert.ok(terminal?.[1], from);
    assert.deepEqual(terminal, terminals.get(to));
    joinedEnds.add(from);
    joinedEnds.add(to);
  }
  const unconnected = [];
  for (const road of roads.values()) {
    assert.ok(buildBakedRoadContact(road), road.id);
    const copies = ownerCopies.get(road.id);
    for (const owner of road.owners)
      assert.ok(
        copies.has(owner),
        `${road.id} missing from its terminal owner ${owner}`,
      );
    for (const [x, y] of road.points) {
      const { t } = tileAt(tiles, x, y);
      assert.ok(copies.has(t), `${road.id} missing from crossed tile ${t}`);
    }
    for (const end of ["start", "end"]) {
      const id = `${road.id}:${end}`;
      if (!joinedEnds.has(id) && !road.groundEnds?.includes(end))
        unconnected.push(id);
    }
    for (const end of road.groundEnds ?? []) {
      const [x, y, z] =
        road.points[end === "start" ? 0 : road.points.length - 1];
      const ground = groundAtWorld(loaded, x, y, true);
      assert.ok(
        ground !== null && Math.abs(ground - z) <= 0.22,
        `${road.id}:${end}`,
      );
    }
  }
  assert.deepEqual(unconnected, []);
  const fork = "sf-contact/i280-18th-off-ramp:end";
  const exits = [
    "sf-contact/i280-18th-exit",
    "sf-contact/i280-pennsylvania-exit",
  ];
  assert.deepEqual(
    roadBake.playableTransitions
      .filter(({ from }) => from === fork)
      .map(({ to }) => to)
      .sort(),
    exits.map((exit) => `${exit}:start`),
  );
  assert.deepEqual(roads.get(exits[0]).groundEnds, []);
  assert.deepEqual(roads.get(exits[1]).groundEnds, ["end"]);
  const stockton = roads.get("sf-contact/stockton-tunnel");
  assert.deepEqual(stockton.groundEnds, ["start", "end"]);
  assert.equal(stockton.kind, "tunnel");
  assert.ok(
    Math.abs(
      stockton.interior_range_m[1] - stockton.interior_range_m[0] - 276.44,
    ) < 0.01,
  );
});

test("southbound I-280 retains both 18th Street fork branches and levels", () => {
  const t = "808f7fcc";
  const tile = JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const fork = "sf-contact/i280-king-to-mariposa:end";
  const through = "sf-contact/i280-south-through-18th";
  assert.deepEqual(
    roadBake.playableTransitions
      .filter(({ from }) => from === fork)
      .map(({ to }) => to)
      .sort(),
    ["sf-contact/i280-18th-off-ramp:start", `${through}:start`],
  );
  const road = tile.playableRoads.find(({ id }) => id === through);
  const { surface } = buildBakedRoadContact(road);
  const underRamp = surface.heightAt(4161.917, 311.135);
  assert.ok(underRamp > 20.3 && underRamp < 20.6);
  for (let i = 1; i < road.points.length; i++) {
    const [ax, ay, az] = road.points[i - 1];
    const [bx, by, bz] = road.points[i];
    assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.04);
  }
});

test("Dogpatch keeps four ramp portals and the southbound through lane connected", () => {
  const tokens = ["808f7fb4", "808f7fac", "808f7efc"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const prefix = "sf-contact/i280-";
  const road = (slug) =>
    tiles
      .flatMap(({ playableRoads }) => playableRoads)
      .find(({ id }) => id === prefix + slug);
  const connections = (slug) =>
    roadBake.playableTransitions
      .filter(({ from }) => from === `${prefix}${slug}:end`)
      .map(({ to }) => to.slice(prefix.length, -6))
      .sort();
  assert.deepEqual(connections("dogpatch-approach"), [
    "dogpatch-off-ramp",
    "south-through-dogpatch",
  ]);
  assert.deepEqual(connections("dogpatch-off-ramp"), [
    "dogpatch-pennsylvania-north",
    "dogpatch-pennsylvania-south",
  ]);
  for (const approach of [
    "dogpatch-pennsylvania-on-north",
    "dogpatch-pennsylvania-on-south",
  ])
    assert.deepEqual(connections(approach), ["dogpatch-on-ramp"]);
  for (const approach of ["dogpatch-on-ramp", "south-through-dogpatch"])
    assert.deepEqual(connections(approach), ["south-beyond-dogpatch"]);
  for (const slug of [
    "dogpatch-pennsylvania-north",
    "dogpatch-pennsylvania-south",
    "dogpatch-pennsylvania-on-north",
    "dogpatch-pennsylvania-on-south",
  ]) {
    const { portals } = buildBakedRoadContact(road(slug));
    assert.equal(portals.length, 1, slug);
    const [{ x, y, z }] = portals;
    const pavement = groundAtWorld(tiles, x, y, true);
    assert.ok(Number.isFinite(pavement) && Math.abs(pavement - z) < 0.03, slug);
  }
  const { surface: approach } = buildBakedRoadContact(
    road("dogpatch-approach"),
  );
  const [x, y] = [4192.36, -173.02];
  assert.ok(Math.abs(approach.heightAt(x, y) - 23.48) < 0.05);
  assert.ok(groundAtWorld(tiles, x, y, true) < 11);
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const { surface } = buildBakedRoadContact(road("south-through-dogpatch"));
  for (const [x, y, heading] of [
    [4265.17, -839.73, -1.57],
    [4267.09, -846.3, -1.58],
    [4262.41, -875.6, -1.67],
    [4230.53, -1027.3, -1.92],
  ]) {
    const z = surface.heightAt(x, y);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt(indexes, { x, y, heading }, z), null);
  }
});

test("Alemany keeps its overpass complete in both tiles above northbound I-280", () => {
  const tokens = ["808f7e64", "808f7ef4"];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/alemany-i280-overpass"),
  );
  assert.deepEqual(roads[0], roads[1]);
  const { surface, portals } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const lower = groundAtWorld(tiles, 1293.207, -3176.618, true);
  const deck = surface.heightAt(1293.207, -3176.618);
  assert.ok(lower > 28.7 && lower < 29.2);
  assert.ok(deck > 36.35 && deck < 36.55);
  for (let i = 1; i < roads[0].points.length; i++) {
    const [ax, ay, az] = roads[0].points[i - 1];
    const [bx, by, bz] = roads[0].points[i];
    assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.03);
  }
});

test("Geary's bidirectional tunnel joins the ground and stays below Masonic", () => {
  const tiles = ["808580b4", "80858734"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/geary-masonic-tunnel"),
  );
  assert.deepEqual(roads[0], roads[1]);
  assert.equal(roads[0].source_cnn, "6105000");
  const { surface, portals, appearance } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z, halfWidth } of portals) {
    assert.ok(Math.abs(halfWidth - 8) < 0.002);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const above = groundAtWorld(tiles, -643.5, 2489.8, true);
  const bore = surface.heightAt(-643.5, 2489.8);
  assert.ok(above > 83.5 && above < 84.5);
  assert.ok(bore > 76.4 && bore < 77.5);
  assert.ok(above - bore > appearance.clearance);
  const length = appearance.interior[1] - appearance.interior[0];
  assert.ok(length > 160 && length < 162);
});

test("MacArthur keeps one complete bore across tiles and only paints the open approaches", () => {
  const readTile = (token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const owners = ["80858724", "808586e4"];
  const tiles = owners.map(readTile);
  const roads = tiles.map(({ playableRoads }) =>
    playableRoads.find(({ id }) => id === "sf-contact/macarthur-tunnel"),
  );
  assert.deepEqual(roads[0], roads[1]);
  assert.deepEqual(roads[0].owners, owners);
  const { surface, portals, appearance } = buildBakedRoadContact(roads[0]);
  assert.equal(portals.length, 2);
  for (const { x, y, z, halfWidth } of portals) {
    assert.ok(Math.abs(halfWidth - 6.7) < 0.002);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const length = appearance.interior[1] - appearance.interior[0];
  assert.ok(length > 394 && length < 397);
  const centre = [-2602.5, 3800];
  const pavement = surface.heightAt(...centre);
  assert.ok(pavement > 69 && pavement < 74.3);
  assert.ok(groundAtWorld(tiles, ...centre, true) > pavement + 13);

  const source = readTile("8085871c");
  for (const direction of ["Northbound", "Southbound"]) {
    const rows = source.r.filter(
      ([name]) => source.n[name] === `Hwy 1 ${direction}`,
    );
    assert.equal(rows.length, 2);
    for (const row of rows) assert.equal(row[3].length, row[4].length);
    const northing = rows.map((row) =>
      row[3].filter((_, i) => i % 2).map((v) => source.o[1] + v / 10),
    );
    assert.ok(northing.some((stations) => Math.max(...stations) === 3617));
    assert.ok(northing.some((stations) => Math.min(...stations) === 4012));
  }
});

test("Kobbe Avenue remains below both SR 1 decks, which join the highway at each end", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586e4.json.gz")));
  for (const [direction, x] of [
    ["northbound", -2594.4],
    ["southbound", -2603.3],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/kobbe-viaduct-${direction}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, 2);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    }
    const street = groundAtWorld([tile], x, 4200, true);
    const deck = surface.heightAt(x, 4200);
    assert.ok(street > 55 && street < 59);
    assert.ok(deck > 65 && deck < 66);
    assert.ok(deck - street > 6);
    for (let i = 1; i < road.points.length; i++) {
      const [ax, ay, az] = road.points[i - 1];
      const [bx, by, bz] = road.points[i];
      assert.ok(Math.abs(bz - az) / Math.hypot(bx - ax, by - ay) < 0.08);
    }
  }
});

test("Ruckman keeps both SR 1 decks above the street and branches at the northbound fork", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808586dc.json.gz")));
  const road = (slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const north = road("ruckman-viaduct-northbound");
  const goldenGate = road("sr1-north-to-golden-gate-approach");
  const city = road("sr1-north-to-presidio-southbound");
  const doyle = road("presidio-sb-entry");
  for (const [from, to] of [
    [north, goldenGate],
    [north, city],
    [city, doyle],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: start, to: end, direction }) =>
          start === `${from.id}:end` &&
          end === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(north.groundEnds, ["start"]);
  assert.deepEqual(goldenGate.groundEnds, []);
  for (const [direction, x] of [
    ["northbound", -2589],
    ["southbound", -2596.4],
  ]) {
    const decks =
      direction === "northbound"
        ? [north, goldenGate]
        : [road("ruckman-viaduct-southbound")];
    const contact = decks.map(buildBakedRoadContact);
    const portals = contact.flatMap((deck) => deck.portals);
    assert.equal(portals.length, 1);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    }
    const street = groundAtWorld([tile], x, 4547, true);
    const { surface } = contact.at(-1);
    const deck = surface.heightAt(x, 4547);
    assert.ok(street > 35 && street < 39);
    assert.ok(deck > 44 && deck < 45);
    assert.ok(deck - street > 5);
  }
});

test("Sixth Street reaches both I-280 levels through continuous paved approaches", () => {
  const tiles = ["808f7fcc", "808f7fd4"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (slug, tile = tiles[0]) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const south = road("i280-sixth-southbound");
  const north = road("i280-sixth-northbound");
  const east = road("eighteenth-east-viaduct");
  const west = road("eighteenth-west-viaduct");
  const junction = road("eighteenth-viaduct-junction");
  const landing = road("eighteenth-west-landing");
  const exit = road("i280-18th-exit");
  const exitEast = road("i280-18th-exit-east-turn");
  const exitWest = road("i280-18th-exit-west-turn");
  const ramp = road("i280-eighteenth-north-entry");
  const mariposa = road("i280-mariposa-northbound");
  const sixthApproach = road("i280-mariposa-sixth-approach");
  assert.deepEqual(south, road("i280-sixth-southbound", tiles[1]));
  assert.deepEqual(north, road("i280-sixth-northbound", tiles[1]));
  assert.equal(roadBake.coveredGroundRoadPatches["808f7fcc"], 3);
  for (const [from, to] of [
    [south, road("i280-king-to-mariposa")],
    [east, west],
    [west, junction],
    [junction, landing],
    [exit, exitEast],
    [exit, exitWest],
    [exitWest, landing],
    [east, ramp],
    [ramp, mariposa],
    [mariposa, sixthApproach],
    [sixthApproach, north],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` && edge.to === `${to.id}:start`,
      ),
    );
  }
  assert.deepEqual(exitEast.points.at(-1), west.points.at(-1));
  assert.deepEqual(exitEast.seams.end, west.seams.end);
  assert.ok(
    roadBake.playableTransitions.some(
      ({ from, to, direction }) =>
        from === `${exitEast.id}:end` &&
        to === `${west.id}:end` &&
        direction === "forward",
    ),
  );
  for (const deck of [north, south, east, landing]) {
    const { portals } = buildBakedRoadContact(deck);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const indiana = [4259.48, 320.06];
  assert.ok(groundAtWorld(tiles, ...indiana, true) < 12);
  assert.ok(buildBakedRoadContact(east).surface.heightAt(...indiana) > 21);
});

test("Central Freeway connects Fourth Street, 13th, Octavia and South Van Ness", () => {
  const tokens = [
    "8085807c",
    "80858084",
    "8085809c",
    "808580a4",
    "808f7e24",
    "808f7e2c",
  ];
  const tiles = tokens.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const west = "westbound-i80";
  const north = "sf-contact/central-freeway-east-northbound";
  const south = "sf-contact/central-freeway-east-southbound";
  for (const [fromId, toId] of [
    ["fourth-street-entrance", west],
    [west, "sf-contact/i80-westbound-fifth"],
    [west, "sf-contact/i80-westbound-fifth-exit"],
    [north, "sf-contact/central-freeway-octavia-exit"],
    [north, "sf-contact/central-freeway-13th-exit"],
    ["sf-contact/central-freeway-octavia-entry", south],
    ["sf-contact/central-freeway-van-ness-merge", south],
    [south, "sf-contact/central-freeway-i80-east-connector"],
  ]) {
    const from = roads.get(fromId);
    const to = roads.get(toId);
    assert.ok(buildBakedRoadContact(from));
    assert.ok(buildBakedRoadContact(to));
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) => edge.from === `${fromId}:end` && edge.to === `${toId}:start`,
      ),
    );
  }
  const exit = roads.get("sf-contact/central-freeway-octavia-exit");
  const [x, y, z] = exit.points[Math.floor(exit.points.length / 2)];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 4);
  assert.equal(roadBake.overheadBuildingPatches["8085809c"], 1);
  assert.ok(!tiles[2].playableRoads?.some(({ id }) => id === exit.id));
});

test("southbound US 101 keeps the Silver fork, Industrial merge and Silliman street handoffs", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e54", "808f7efc", "808f7ee4", "808f7f24"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (id) => roads.get(`sf-contact/us101-south-${id}`);
  const assertJoin = (source, destination) => {
    const from = road(source);
    const to = road(destination);
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  };
  for (const [from, to] of [
    ["silver-through", "to-i280-south-approach"],
    ["silver-through", "silver-to-industrial"],
    ["silver-to-industrial", "industrial-to-silliman"],
    ["industrial-entry", "industrial-to-silliman"],
    ["industrial-to-silliman", "silliman-exit"],
    ["industrial-to-silliman", "silliman-mainline"],
    ["silliman-mainline", "silliman-to-paul"],
    ["silliman-entry", "silliman-to-paul"],
    ["silliman-to-paul", "paul-approach"],
    ["paul-approach", "san-bruno-exit"],
    ["paul-approach", "paul-mainline"],
  ]) {
    assertJoin(from, to);
  }
  const exit = road("silliman-exit");
  const entry = road("silliman-entry");
  assert.deepEqual(exit.points.at(-1), entry.points[0]);
  assert.ok(
    !roadBake.playableTransitions.some(
      (edge) => edge.from === `${exit.id}:end`,
    ),
  );
  for (const [id, groundEnd] of [
    ["industrial-entry", "start"],
    ["silliman-entry", "start"],
    ["silliman-exit", "end"],
    ["san-bruno-exit", "end"],
  ]) {
    const surface = road(id);
    assert.deepEqual(surface.groundEnds, [groundEnd]);
    const { portals } = buildBakedRoadContact(surface);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, id);
  }
  const acrossTiles = road("silliman-to-paul");
  assert.deepEqual(acrossTiles.owners, ["808f7efc", "808f7ee4"]);
  for (const token of acrossTiles.owners) {
    const tile = tiles.find(({ t }) => t === token);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === acrossTiles.id),
      acrossTiles,
    );
  }
});

test("southbound US 101 stays on its level through the Hester fork and collector merge", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7ee4", "808f7f24", "808f7f2c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (name) => roads.get(`sf-contact/us101-south-${name}`);
  const paul = road("paul-mainline");
  const main = road("hester-mainline");
  const entry = road("hester-collector-entry");
  const exit = road("hester-bayshore-exit");
  const collector = road("hester-collector-return");
  const edge = road("hester-to-city-edge");
  const through = road("past-candlestick-exit");
  for (const [from, to] of [
    [paul, main],
    [paul, entry],
    [entry, exit],
    [entry, collector],
    [main, edge],
    [collector, edge],
    [edge, through],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  for (const ramp of [paul, main, entry, collector, edge]) {
    assert.deepEqual(ramp.groundEnds, []);
    assert.equal(buildBakedRoadContact(ramp).portals.length, 0);
  }
  for (const ramp of [exit, through]) {
    const [{ x, y, z }] = buildBakedRoadContact(ramp).portals;
    assert.deepEqual(ramp.groundEnds, ["end"]);
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  assert.deepEqual(edge.owners, ["808f7f24", "808f7f2c"]);
  assert.deepEqual(
    tiles[1].playableRoads.find(({ id }) => id === edge.id),
    tiles[2].playableRoads.find(({ id }) => id === edge.id),
  );
  assert.equal(roadBake.overheadBuildingPatches["808f7f24"], 4);
});

test("Potrero and Cesar Chavez entrances merge above the street on southbound US 101", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e4c", "808f7e54"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (id) => roads.get(`sf-contact/us101-south-${id}`);
  const potrero = road("potrero-avenue-approach");
  const potreroRamp = road("potrero-entry");
  const chavez = road("cesar-chavez-entry");
  const merge = road("city-entrance-merge");
  const highway = road("silver-through");
  for (const [from, to] of [
    [potrero, potreroRamp],
    [potreroRamp, merge],
    [chavez, merge],
    [merge, highway],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  for (const entrance of [potrero, chavez]) {
    assert.deepEqual(entrance.groundEnds, ["start"]);
    const { portals } = buildBakedRoadContact(entrance);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  assert.deepEqual(potreroRamp.groundEnds, []);
  assert.deepEqual(merge.groundEnds, []);
  const [x, y, z] = merge.points[0];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 5);
  for (const token of new Set(merge.owners)) {
    const owner = tiles.find(({ t }) => t === token);
    assert.deepEqual(
      owner.playableRoads.find(({ id }) => id === merge.id),
      merge,
    );
  }
});

test("the Bayshore exit passes below both freeway directions and keeps its city handoff", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const road = (id) =>
    tile.playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const upstream = road("us101-south-potrero-exit");
  const approach = road("us101-south-bayshore-exit-approach");
  const exit = road("us101-south-bayshore-north-turn-exit");
  for (const [from, to] of [
    [upstream, approach],
    [approach, exit],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(surface.coveredRange, [136, 170]);
  assert.equal(
    buildBakedRoadContact({ ...exit, covered_range_m: [136, 200] }),
    null,
  );
  const deckNames = [
    ["us101-south-cesar-through", 145],
    ["us101-north-bayshore-through", 164],
  ];
  const decks = [];
  for (const [id, crossing] of deckNames) {
    const upper = buildBakedRoadContact(road(id)).surface;
    decks.push(upper);
    const {
      position: [x, y, z],
    } = surface.profile.find(({ s_m }) => s_m >= crossing);
    const above = upper.heightAt(x, y);
    assert.ok(above - z > 7, id);
    assert.equal(surface.heightAt(x, y), z);
    assert.equal(upper.coveredRange, undefined);
  }
  const profile = surface.profile;
  for (let i = 1; i < profile.length; i++) {
    const {
      s_m,
      position: [x, y, z],
    } = profile[i];
    if (s_m < 134) continue;
    const [px, py] = profile[i - 1].position;
    const car = { x, y, heading: Math.atan2(y - py, x - px) };
    const { position } = driveCameraPose(
      "zoomed",
      car,
      z,
      surface.heightAt,
      0,
      0,
      coveredRoadBlend(surface, car),
    );
    const [cx, cy, cz] = position;
    const samples = Math.ceil(Math.hypot(cx - x, cy - y) / 0.25);
    for (let step = 0; step <= samples; step++) {
      const t = step / samples;
      const sx = cx + (x - cx) * t;
      const sy = cy + (y - cy) * t;
      const sightline = cz + (z + 0.8 - cz) * t;
      for (const deck of decks) {
        const height = deck.heightAt(sx, sy);
        if (height === null) continue;
        assert.ok(
          height - sightline > 0.5,
          `Camera under ${deck.id} at ${s_m.toFixed(1)} m`,
        );
      }
    }
  }
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
  for (const [px, py] of [
    [2972, -1960],
    [3012, -3560],
  ]) {
    assert.ok(!tile.tr.some(([tx, ty]) => tx === px && ty === py));
  }
});

test("Monterey keeps its ramp mouths separate and Ocean Avenue reaches city pavement", () => {
  const tokens = ["808f7e64", "808f7e7c", "808f7ef4", "808f7e84", "808f7c2c"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const road = (slug) => roads.get(`sf-contact/${slug}`);
  const north = road("i280-north-monterey-entry");
  const south = road("i280-south-monterey-exit");
  const northContact = buildBakedRoadContact(north);
  const southContact = buildBakedRoadContact(south);
  const [entry] = northContact.portals;
  const [exit] = southContact.portals;
  assert.ok(Math.hypot(entry.x - exit.x, entry.y - exit.y) > 8.9);
  const ground = (x, y) => groundAtWorld(tiles, x, y, true);
  for (const { x, y, z } of [entry, exit]) {
    assert.ok(Math.abs(ground(x, y) - z) < 0.03);
  }
  const x = entry.x - 0.05 * entry.nx;
  const y = entry.y - 0.05 * entry.ny;
  const car = {
    x,
    y,
    z: ground(x, y),
    heading: Math.atan2(entry.ny, entry.nx),
    v: 10,
    grounded: true,
  };
  const arrived = driveThroughRoadPortal(
    car,
    { throttle: 1 },
    1 / 120,
    [southContact.portals, northContact.portals],
    ground,
  );
  assert.ok(arrived?.grounded);
  assert.equal(arrived.surfaceId, north.id);

  const southbound = road("i280-south-beyond-alemany");
  const ocean = road("i280-south-ocean-exit");
  for (const chain of [
    [
      north,
      road("i280-north-monterey-to-alemany"),
      road("i280-north-alemany-west-approach"),
      road("i280-north-alemany-main"),
    ],
    [southbound, south],
    [
      southbound,
      road("i280-south-monterey-mainline"),
      road("i280-south-monterey-to-ocean"),
      road("i280-south-ocean-approach"),
      ocean,
    ],
  ]) {
    for (const [i, to] of chain.slice(1).entries()) {
      const from = chain[i];
      assert.deepEqual(from.points.at(-1), to.points[0]);
      assert.deepEqual(from.seams.end, to.seams.start);
      assert.ok(
        roadBake.playableTransitions.some(
          (edge) =>
            edge.from === `${from.id}:end` &&
            edge.to === `${to.id}:start` &&
            edge.direction === "forward",
        ),
      );
    }
  }
  const [oceanExit] = buildBakedRoadContact(ocean).portals;
  assert.ok(Math.abs(ground(oceanExit.x, oceanExit.y) - oceanExit.z) < 0.03);
  const fromBothTiles = tiles
    .slice(0, 2)
    .map(({ playableRoads }) =>
      playableRoads.find(({ id }) => id === north.id),
    );
  assert.deepEqual(fromBothTiles[0], fromBothTiles[1]);
});

test("Central Freeway and Tenth Street join southbound US 101 while I-80 stays connected", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e2c", "808f7e34"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = (tile, slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const central = road(tiles[0], "central-freeway-east-southbound");
  const i80 = road(tiles[0], "central-freeway-i80-east-connector");
  const viaduct = road(tiles[0], "us101-south-central-viaduct");
  const entry = road(tiles[0], "us101-south-tenth-bryant-entry");
  const merge = road(tiles[0], "us101-south-central-merge");
  const south = road(tiles[1], "us101-south-potrero-main");
  assert.deepEqual(merge, road(tiles[1], "us101-south-central-merge"));
  assert.deepEqual(merge.owners, ["808f7e2c", "808f7e34"]);
  for (const [from, to] of [
    [central, i80],
    [central, viaduct],
    [viaduct, merge],
    [entry, merge],
    [merge, south],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { portals } = buildBakedRoadContact(entry);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  assert.equal(buildBakedRoadContact(merge).portals.length, 0);
});

test("the Branch East exit and northbound I-280 keep their own road levels", () => {
  const tiles = ["808f7e7c", "808f7e64"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const matching = (tile, slug) =>
    tile.playableRoads.find(
      ({ id }) => id === `sf-contact/branch-east-${slug}`,
    );
  const bore = matching(tiles[0], "off-ramp");
  const mainline = matching(tiles[0], "i280-northbound");
  const viaduct = matching(tiles[1], "san-jose-viaduct");
  for (const [road, slug] of [
    [bore, "off-ramp"],
    [mainline, "i280-northbound"],
  ]) {
    assert.deepEqual(road, matching(tiles[1], slug));
  }
  const shared = tiles[1].playableRoads.find(
    ({ id }) => id === "sf-contact/san-jose-avenue-shared-deck",
  );
  for (const road of [bore, mainline, viaduct, shared]) {
    const { portals } = buildBakedRoadContact(road);
    assert.equal(portals.length, road === viaduct ? 0 : 1);
    for (const { x, y, z } of portals)
      assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  const crossing = [451.516, -3185.15];
  const { surface, appearance } = buildBakedRoadContact(bore);
  const lower = surface.heightAt(...crossing);
  const upper = buildBakedRoadContact(mainline).surface.heightAt(...crossing);
  assert.ok(lower > 48 && lower < 48.25);
  assert.ok(upper > 53.8 && upper < 54.1);
  assert.ok(upper - lower > appearance.clearance);
  const farCrossing = [680.986, -2997.639];
  const farDeck = buildBakedRoadContact(viaduct).surface.heightAt(
    ...farCrossing,
  );
  assert.ok(farDeck > 56.5 && farDeck < 56.8);
  assert.ok(farDeck - groundAtWorld(tiles, ...farCrossing, true) > 9);
});

test("Broadway's lower lanes clear the building while its upper surfaces stay solid", () => {
  const owners = ["808580ec", "808580f4"];
  const tiles = owners.map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  assert.deepEqual(
    owners.map((t) => roadBake.overheadBuildingPatches[t]),
    [1, 2],
  );
  const bores = tiles.map(({ playableRoads }) =>
    playableRoads.filter(({ id }) => id.startsWith("sf-contact/broadway-")),
  );
  assert.deepEqual(bores[0], bores[1]);
  assert.deepEqual(
    bores[0].map(({ id }) => id),
    ["sf-contact/broadway-westbound", "sf-contact/broadway-eastbound"],
  );
  for (const road of bores[0])
    assert.deepEqual(road.groundEnds, ["start", "end"]);

  const { surface } = buildBakedRoadContact(bores[0][0]);
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const entry = { x: 2529.5, y: 4132.1, heading: -2.917 };
  const floor = surface.heightAt(entry.x, entry.y);
  assert.ok(floor > 40 && floor < 41);
  assert.equal(collisionAt(indexes, entry, floor), null);
  assert.equal(collisionAt(indexes, entry, 55), "building");

  const east = 2191.961;
  const north = 4077.38;
  assert.ok(Math.abs(surface.heightAt(east, north) - 52.91) < 0.1);
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - 93.81) < 0.1);
});

test("the Broadway portal lanes clear inferred trunks at their supported road levels", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const owners = ["808580ec", "808580f4"];
  const tiles = owners.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const bores = tiles[0].playableRoads.filter(({ id }) =>
    id.startsWith("sf-contact/broadway-"),
  );
  assert.equal(bores.length, 2);
  for (const { id, points } of bores) {
    for (const [i, [x, y, z]] of points.slice(1).entries()) {
      const [priorX, priorY] = points[i];
      const heading = Math.atan2(y - priorY, x - priorX);
      for (const offset of [-0.8, 0, 0.8]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(collisionAt(indexes, car, z), null, `${id} at ${x}, ${y}`);
      }
    }
  }
  assert.deepEqual(
    owners.map((token) => roadBake.inferredTreePatches[token]),
    [4, 2],
  );
});

test("surveyed ground approaches and carriageways stay clear of inferred trunks", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const review = manifest.roadBake.treeClassification.additionalReviews.find(
    ({ classificationSha256 }) =>
      classificationSha256 ===
      "f39e9cebecdd8c24af56d67350edcca6588caa9fa83d7f0d36571c9c3062a6db",
  );
  assert.ok(review);
  for (const [token, row, [x, y, z, heading]] of [
    [
      "808580f4",
      [-3334, -2679],
      [2566.646533, 4140.084305, 39.350755, -2.915933],
    ],
    [
      "808f7c3c",
      [2890, 4607],
      [-1799.651503, -5528.700417, 89.519388, 1.945773],
    ],
    ["808f7e2c", [-2501, -844], [2650.740653, 1152.678111, 5.3, -0.211633]],
    ["808f7dac", [924, -3946], [-3992.333, -5041.133, 15.493, -1.742]],
  ]) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    assert.ok(
      !tile.tr.some(([tx, ty]) => tx === row[0] && ty === row[1]),
      token,
    );
    const index = buildCollisionIndex(tile, token);
    assert.equal(collisionAt([index], { x, y, heading }, z), null, token);
  }
  const dogpatch = JSON.parse(gunzipSync(file("tiles/808f7fb4.json.gz")));
  assert.ok(dogpatch.tr.some(([x, y]) => x === 2662 && y === -3926));
  const crossover = JSON.parse(gunzipSync(file("tiles/8085876c.json.gz")));
  for (const [x, y] of [
    [-2152, -3278],
    [-2132, -3378],
  ])
    assert.ok(crossover.tr.some(([u, v]) => u === x && v === y));
});

test("Presidio's through lanes stay continuous across the Battery and Main Post portals", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808586d4", "808586dc", "808586ec"].map((t) =>
    JSON.parse(gunzipSync(file(`tiles/${t}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) => playableRoads.map((r) => [r.id, r])),
  );
  const parkwayThrough = new Map([
    ["sf-contact/presidio-nb-viaduct:end", "sf-contact/presidio-nb-exit:start"],
    [
      "sf-contact/main-post-southbound-west:end",
      "sf-contact/main-post-southbound:start",
    ],
  ]);
  const transitions = new Map(
    roadBake.playableTransitions
      .filter(
        ({ from, to }) =>
          !parkwayThrough.has(from) || parkwayThrough.get(from) === to,
      )
      .map(({ from, to }) => [from, to]),
  );
  for (const [direction, bores] of [
    ["nb", ["main-post-northbound", "battery-northbound"]],
    [
      "sb",
      [
        "battery-southbound",
        "main-post-southbound-west",
        "main-post-southbound",
      ],
    ],
  ]) {
    const start = `sf-contact/presidio-${direction}-entry`;
    const end =
      direction === "nb"
        ? "sf-contact/ruckman-viaduct-southbound"
        : "sf-contact/presidio-sb-exit";
    let id = start;
    const tunnels = [];
    const visited = new Set();
    while (id) {
      assert.ok(!visited.has(id), `Road cycle at ${id}`);
      visited.add(id);
      const road = roads.get(id);
      assert.ok(road, id);
      const baked = buildBakedRoadContact(road);
      assert.ok(baked, id);
      if (road.kind === "tunnel") {
        assert.ok(baked.appearance, id);
        tunnels.push(id);
      }
      for (const terminal of road.groundEnds) {
        const [x, y, z] = road.points.at(terminal === "start" ? 0 : -1);
        assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03, id);
      }
      for (let i = 1; i < road.points.length; i++) {
        const [x, y, z] = road.points[i];
        const [px, py, pz] = road.points[i - 1];
        assert.ok(Math.abs(z - pz) / Math.hypot(x - px, y - py) < 0.075, id);
      }
      id = transitions.get(`${id}:end`)?.replace(/:start$/, "");
    }
    assert.deepEqual(
      tunnels,
      bores.map((bore) => `sf-contact/${bore}`),
    );
    assert.deepEqual(roads.get(start).groundEnds, ["start"]);
    assert.deepEqual(roads.get(end).groundEnds, ["end"]);
    assert.equal([...visited].at(-1), end);
  }

  const plaza = tiles[2];
  const { surface, appearance, portals } = buildBakedRoadContact(
    roads.get("sf-contact/toll-plaza-tunnel"),
  );
  assert.ok(appearance);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  const entry = { x: -3169.55, y: 5207.47, heading: 0.625 };
  const floor = surface.heightAt(entry.x, entry.y);
  assert.ok(Number.isFinite(floor));
  const indexes = [buildCollisionIndex(plaza, plaza.t)];
  assert.equal(collisionAt(indexes, entry, floor), null);
  assert.equal(collisionAt(indexes, entry, 55.4), "building");
});

test("Main Post bores underpass the park and leave the measured causeway open", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586d4.json.gz")));
  const contact = (id) =>
    buildBakedRoadContact(
      tile.playableRoads.find((road) => road.id === `sf-contact/${id}`),
    );
  for (const [bore, causeway, inside, outside, minimumElevation] of [
    [
      "main-post-northbound",
      "presidio-nb-causeway",
      [-1390, 4784],
      [-1100, 4825],
      8,
    ],
    [
      "main-post-southbound-west",
      "presidio-sb-causeway",
      [-1390, 4764],
      [-1100, 4802],
      7.5,
    ],
  ]) {
    const tunnel = contact(bore);
    assert.ok(tunnel.appearance);
    const floor = tunnel.surface.heightAt(...inside);
    assert.ok(floor > 0 && floor < 3, bore);
    assert.ok(groundAtWorld([tile], ...inside, true) - floor > 6, bore);
    const exposed = contact(causeway);
    assert.equal(exposed.surface.kind, "bridge_deck");
    assert.ok(!exposed.appearance);
    assert.ok(
      exposed.surface.heightAt(...outside) > minimumElevation,
      causeway,
    );
  }
  const { surface } = contact("presidio-sb-causeway");
  const entry = { x: -1047.5, y: 4805.6, heading: 0.1 };
  const floor = surface.heightAt(entry.x, entry.y);
  assert.ok(floor > 8.4);
  assert.equal(
    collisionAt([buildCollisionIndex(tile, tile.t)], entry, floor),
    null,
  );
});

test("Marina Boulevard and the northbound causeway join at the Main Post portal", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808586d4.json.gz")));
  const road = (slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const marina = road("presidio-marina-northbound-entry");
  const causeway = road("presidio-nb-causeway");
  const northbound = road("main-post-northbound");
  assert.deepEqual(marina.groundEnds, ["start"]);
  assert.deepEqual(marina.widthTransitionMetres, { end: 24 });
  for (const entrance of [marina, causeway]) {
    assert.deepEqual(entrance.points.at(-1), northbound.points[0]);
    assert.deepEqual(entrance.seams.end, northbound.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${entrance.id}:end` &&
          edge.to === `${northbound.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { portals, surface } = buildBakedRoadContact(marina);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
  const [east, north] = [-1116.81, 4840.67];
  assert.ok(
    surface.heightAt(east, north) - groundAtWorld([tile], east, north, true) >
      4,
  );
});

test("Presidio Parkway keeps its mainline and SR 1 on distinct branches across tiles", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tileOwners = ["808586dc", "808586ec"];
  const tiles = tileOwners.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = (tile, slug) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const mainlineSouth = road(tiles[0], "presidio-doyle-southbound-entry");
  const mainlineNorth = road(tiles[0], "presidio-doyle-northbound-exit");
  const viaductSouth = road(tiles[0], "presidio-sb-viaduct");
  const viaductNorth = road(tiles[0], "presidio-nb-viaduct");
  const originalExit = road(tiles[0], "presidio-nb-exit");
  const ruckman = road(tiles[0], "ruckman-viaduct-southbound");
  const northLoop = road(tiles[1], "presidio-nb-loop-to-sr1-south");
  const southExit = road(tiles[1], "presidio-southbound-to-sr1-exit");
  const sr1SouthMerge = road(tiles[0], "presidio-to-sr1-south-merge");
  const sr1NorthEntry = road(tiles[0], "sr1-north-to-golden-gate-approach");
  const sr1NorthMerge = road(tiles[1], "sr1-north-to-doyle-merge");
  const goldenNorth = road(tiles[1], "golden-gate-northbound-1");
  const goldenSouth = road(tiles[1], "golden-gate-southbound-6");
  assert.deepEqual(originalExit.groundEnds, []);
  assert.deepEqual(ruckman.groundEnds, ["end"]);
  const replaced = new Set(roadBake.playableReplaces);
  for (const id of [
    "overpass/19187000/forward/1",
    "overpass/19187000/forward/2",
    "overpass/15224000/forward/1",
  ]) {
    const preview = tiles
      .flatMap((tile) => tile.raisedRoads)
      .find((road) => road.id === id);
    assert.ok(preview, id);
    assert.equal(buildRaisedRoad(preview, replaced), null, id);
  }
  for (const crossing of [northLoop, sr1NorthMerge]) {
    assert.deepEqual([...new Set(crossing.owners)].sort(), tileOwners);
    for (const tile of tiles) {
      assert.deepEqual(
        tile.playableRoads.find(({ id }) => id === crossing.id),
        crossing,
      );
    }
  }
  for (const mainline of [mainlineSouth, mainlineNorth]) {
    assert.deepEqual(
      mainline,
      tiles[1].playableRoads.find(({ id }) => id === mainline.id),
    );
    assert.deepEqual([...new Set(mainline.owners)].sort(), tileOwners);
    const { portals } = buildBakedRoadContact(mainline);
    assert.equal(portals.length, 1);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.03);
  }
  for (const [from, to] of [
    [mainlineSouth, viaductSouth],
    [viaductNorth, mainlineNorth],
    [viaductNorth, originalExit],
    [originalExit, northLoop],
    [northLoop, sr1SouthMerge],
    [goldenSouth, southExit],
    [southExit, sr1SouthMerge],
    [sr1SouthMerge, ruckman],
    [sr1NorthEntry, sr1NorthMerge],
    [sr1NorthMerge, goldenNorth],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: start, to: end, direction }) =>
          start === `${from.id}:end` &&
          end === `${to.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const [x, y, z] = mainlineNorth.points[20];
  assert.ok(z - groundAtWorld(tiles, x, y, true) > 5);
  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tileOwners[i]),
  );
  const north = buildBakedRoadContact(mainlineNorth).surface;
  const treeSite = { x: -2897.4, y: 4993.2, heading: 2.35 };
  assert.equal(
    collisionAt(obstacles, treeSite, north.heightAt(treeSite.x, treeSite.y)),
    null,
  );
  assert.ok(roadBake.inferredTreePatches["808586ec"] >= 1);
  assert.ok(
    Math.hypot(
      mainlineNorth.points.at(-1)[0] - originalExit.points.at(-1)[0],
      mainlineNorth.points.at(-1)[1] - originalExit.points.at(-1)[1],
    ) > 250,
  );
});

test("Harrison and Silver Avenue decks span the lower roads at independent heights", () => {
  const cases = [
    [
      "8085807c",
      "harrison-beale-bridge",
      4300.061,
      3048.916,
      15.58,
      5.13,
      0.075,
    ],
    [
      "808f7efc",
      "silver-avenue-bridge",
      3058.179,
      -3041.466,
      22.69,
      16.21,
      0.03,
    ],
  ];
  for (const [token, id, x, y, upper, lower, maxGrade] of cases) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    const road = tile.playableRoads.find(
      (entry) => entry.id === `sf-contact/${id}`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(surface.kind, "bridge_deck", id);
    assert.equal(portals.length, 2, id);
    for (const portal of portals)
      assert.ok(
        Math.abs(groundAtWorld([tile], portal.x, portal.y, true) - portal.z) <
          0.03,
        id,
      );
    assert.ok(Math.abs(surface.heightAt(x, y) - upper) < 0.03, id);
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - lower) < 0.03, id);
    for (let i = 1; i < road.points.length; i++) {
      const a = road.points[i - 1],
        b = road.points[i];
      assert.ok(
        Math.abs(b[2] - a[2]) / Math.hypot(b[0] - a[0], b[1] - a[1]) < maxGrade,
        id,
      );
    }
  }
});

test("Toll Plaza joins the west street before rounding the bend into the lower bore", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808586ec.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/toll-plaza-tunnel",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  const [west] = portals;
  assert.ok(Math.hypot(west.x + 3161.95, west.y - 5193.32) < 0.03);
  assert.ok(
    Math.abs(groundAtWorld([tile], west.x, west.y, true) - west.z) < 0.03,
  );
  const bearings = surface.profile
    .slice(1, 35)
    .map(({ position: [x, y] }, i) => {
      const [px, py] = surface.profile[i].position;
      return Math.atan2(y - py, x - px);
    });
  assert.ok(Math.abs(bearings[0] - 2.303) < 0.01);
  assert.ok(Math.abs(bearings.at(-1) - 0.627) < 0.01);
  for (let i = 1; i < bearings.length; i++)
    assert.ok(Math.abs(bearings[i] - bearings[i - 1]) < 0.17);
});

test("the southbound Bayshore turn stays above its refined ground and joins the city street", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const baked = (id) =>
    tile.playableRoads.find((road) => road.id === `sf-contact/${id}`);
  const stem = baked("us101-south-bayshore-exit-approach");
  const south = baked("us101-south-bayshore-south-turn-exit");
  const north = baked("us101-south-bayshore-north-turn-exit");
  const lower = baked("bayshore-boulevard-under-us101");
  for (const branch of [south, north]) {
    assert.deepEqual(stem.points.at(-1), branch.points[0]);
    assert.deepEqual(stem.seams.end, branch.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${stem.id}:end` &&
          edge.to === `${branch.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  assert.equal(tile.g.s, 8);
  assert.equal(tile.g.detail.s, 1);
  assert.ok(roadBake.terrainDetailPatches["808f7e4c"]);
  for (const road of [south, lower]) {
    const { surface } = buildBakedRoadContact(road);
    const [coverStart, coverEnd] = surface.coveredRange;
    for (const {
      position: [x, y, z],
      s_m,
    } of surface.profile) {
      if (s_m < coverStart || s_m > coverEnd) continue;
      assert.ok(
        groundAtWorld([tile], x, y, true) - z < 0.15,
        `Ground over ${road.id} at ${s_m} m`,
      );
    }
  }
  const [{ x, y, z }] = buildBakedRoadContact(lower).portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
});

test("the Potrero connector joins lower Bayshore pavement without losing the US 101 fork", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const road = (id) =>
    tile.playableRoads.find((candidate) => candidate.id === `sf-contact/${id}`);
  const potrero = road("us101-south-potrero-avenue-approach");
  const freeway = road("us101-south-potrero-entry");
  const connector = road("potrero-to-bayshore-boulevard");
  const offRamp = road("us101-south-bayshore-south-turn-exit");
  const lower = road("bayshore-boulevard-under-us101");
  for (const [from, to] of [
    [potrero, freeway],
    [potrero, connector],
    [connector, lower],
    [offRamp, lower],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  assert.deepEqual(connector.groundEnds, []);
  assert.deepEqual(offRamp.groundEnds, []);
  assert.deepEqual(lower.groundEnds, ["end"]);
  assert.deepEqual(lower.groundExitOnly, ["end"]);
  const upperRoad = road("us101-south-cesar-through");
  const upper = buildBakedRoadContact(upperRoad).surface;
  const [x, y, z] = lower.points[0];
  assert.ok(upper.heightAt(x, y) - z > 7);
});

test("lower Bayshore follows the measured street grade beyond the portal", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e4c.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/bayshore-boulevard-under-us101",
  );
  const [{ x, y, z }] = buildBakedRoadContact(road).portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.01);

  // The old terrain formed two humps at these recorded airborne starts.
  const samples = [
    [3193.35, -1357.21, 6.84],
    [3193.158, -1362.521, 6.87],
    [3193.025, -1364.131, 6.89],
    [3192.651, -1370.438, 6.94],
    [3192.574, -1371.995, 6.95],
  ];
  let previous = -Infinity;
  for (const [east, north, measured] of samples) {
    const elevation = groundAtWorld([tile], east, north, true);
    assert.ok(Math.abs(elevation - measured) < 0.04);
    assert.ok(elevation > previous);
    previous = elevation;
  }
});

test("both SR 1 approaches carry the same contact across the tile boundary", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["8085871c", "80858724"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const cases = [
    ["northbound", -2653.332, 3396.3535],
    ["southbound", -2661.0729, 3400.5867],
  ];
  const deckIndex = deckClearanceIndex();
  const surfaces = cases.map(([direction, x, y]) => {
    const id = `sf-contact/sr1-south-of-macarthur-${direction}`;
    const [road, neighbour] = tiles.map((tile) =>
      tile.playableRoads.find((candidate) => candidate.id === id),
    );
    assert.deepEqual(road, neighbour);
    assert.equal(new Set(road.owners).size, 2);
    const { surface, portals } = buildBakedRoadContact(road);
    assert.deepEqual(
      portals.map(({ exitOnly }) => exitOnly),
      [false, true],
    );
    for (const { x, y, z } of portals)
      assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.01);
    assert.ok(surface.heightAt(x, y) - groundAtWorld(tiles, x, y, true) > 5);
    deckIndex.add(contactGeometry(surface.contact, false));
    return road.points;
  });
  assert.equal(roadBake.overheadBuildingPatches["8085871c"], 1);
  const collisions = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), deckIndex.ceiling),
  );
  for (const points of surfaces) {
    for (const [i, [x, y, z]] of points.entries()) {
      if (!i) continue;
      const [a, b] = points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      for (const offset of [0, -2.2, 2.2]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(collisionAt(collisions, car, z), null);
      }
    }
  }
});

test("the westbound Fifth exit crosses below the continuing freeway and reaches Harrison Street", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["8085807c", "80858084"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const route = (id) =>
    tiles[0].playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const approach = route("i80-westbound-upper-soma");
  const exit = route("i80-westbound-fifth-harrison-exit");
  const through = route("i80-westbound-fourth-merge");
  for (const [branch, approachLane] of [
    [exit, [-8.8, 2]],
    [through, [-17.5, -8.8]],
  ]) {
    assert.deepEqual(approach.points.at(-1), branch.points[0]);
    assert.deepEqual(approach.seams.end, branch.seams.start);
    const edge = roadBake.playableTransitions.find(
      (edge) =>
        edge.from === `${approach.id}:end` &&
        edge.to === `${branch.id}:start` &&
        edge.direction === "forward",
    );
    assert.deepEqual(edge?.from_lateral_range_m, approachLane);
  }
  assert.deepEqual(exit.owners, tokens);
  assert.deepEqual(
    tiles[1].playableRoads.find((r) => r.id === exit.id),
    exit,
  );
  const { surface, portals } = buildBakedRoadContact(exit);
  assert.deepEqual(surface.coveredRange, [410.369, 445.369]);
  const upper = buildBakedRoadContact(through).surface;
  const {
    position: [x, y, z],
  } = surface.profile.find((point) => point.s_m >= 421);
  assert.ok(upper.heightAt(x, y) - z > 6);
  const [{ x: east, y: north, z: ground }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - ground) < 0.03);
  const indexes = tiles.map((tile, i) => buildCollisionIndex(tile, tokens[i]));
  const {
    position: [px, py, pz],
  } = upper.profile.find((point) => point.s_m >= 159);
  const heading = -2.435;
  const offset = -2;
  const pose = {
    x: px - Math.sin(heading) * offset,
    y: py + Math.cos(heading) * offset,
    z: pz,
    heading,
  };
  assert.equal(collisionAt(indexes, pose, pz), null);
  assert.equal(roadBake.overheadBuildingPatches["8085807c"], 5);
});

test("the surveyed SoMa I-80 lanes stay on their own decks and clear the capped face", () => {
  const token = "8085807c";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const obstacles = [buildCollisionIndex(tile, token)];
  for (const [slug, chainage, lateral] of [
    ["i80-eastbound-lower-climb", 147, 10],
    ["i80-westbound-upper-soma", 365, -10],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/${slug}`,
    );
    const { surface } = buildBakedRoadContact(road);
    const i = surface.profile.findIndex(({ s_m }) => s_m >= chainage);
    const [east, north, z] = surface.profile[i].position;
    const a = surface.profile[i - 1].position;
    const b = surface.profile[i + 1].position;
    const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
    const car = {
      x: east - lateral * Math.sin(heading),
      y: north + lateral * Math.cos(heading),
      heading,
      z,
    };
    assert.ok(Math.abs(surface.heightAt(car.x, car.y) - z) < 0.1);
    assert.ok(roadSupportsCar(surface.heightAt, car));
    assert.equal(collisionAt(obstacles, car, z), null);
    if (lateral < 0) assert.equal(collisionAt(obstacles, car, 23), "building");
  }
});

test("both I-280 Mariposa approaches support the surveyed lanes on either side", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7fcc.json.gz")));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const { merge_station_metres: split } =
    roadBake.eighteenthMariposaMergeSurvey;
  for (const [slug, station, offsets] of [
    ["i280-fifth-south-to-sixth-merge", 283, [-7, 9]],
    ["i280-mariposa-northbound", 352 - split, [-5.5, 9]],
  ]) {
    const road = tile.playableRoads.find(
      ({ id }) => id === `sf-contact/${slug}`,
    );
    const { surface } = buildBakedRoadContact(road);
    const i = surface.profile.findIndex(({ s_m }) => s_m >= station);
    const [east, north, z] = surface.profile[i].position;
    const a = surface.profile[i - 1].position;
    const b = surface.profile[i + 1].position;
    const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
    for (const lateral of offsets) {
      const car = {
        x: east - lateral * Math.sin(heading),
        y: north + lateral * Math.cos(heading),
        heading,
        z,
      };
      assert.ok(Math.abs(surface.heightAt(car.x, car.y) - z) < 0.15);
      assert.ok(roadSupportsCar(surface.heightAt, car));
    }
  }
});

test("the Alemany fork carries both I-280 through traffic and the ramp to southbound US 101", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ef4", "808f7efc", "808f7ee4"];
  const tiles = new Map(
    tokens.map((token) => [
      token,
      JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
    ]),
  );
  const route = (token, id) =>
    tiles
      .get(token)
      .playableRoads.find((road) => road.id === `sf-contact/${id}`);
  const approach = route(tokens[0], "i280-north-alemany-west-approach");
  const entry = route(tokens[0], "i280-north-alemany-entry");
  const through = route(tokens[0], "i280-north-alemany-main");
  const connector = route(tokens[0], "i280-north-to-us101-south-connector");
  const descent = route(tokens[1], "i280-to-us101-south-descent");
  const onward = route(tokens[2], "us101-south-paul-approach");
  for (const [before, after] of [
    [approach, through],
    [approach, connector],
    [entry, connector],
    [connector, descent],
    [descent, onward],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${before.id}:end` &&
          edge.to === `${after.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  assert.deepEqual(
    route(tokens[1], "i280-north-to-us101-south-connector"),
    connector,
  );
  assert.deepEqual(route(tokens[2], "i280-to-us101-south-descent"), descent);
  assert.equal(connector.points.at(-1)[2], 38.86);
  assert.equal(descent.points.at(-1)[2], 21.94);
  const x = 2336.03;
  const y = -3057.44;
  const z = buildBakedRoadContact(connector).surface.heightAt(x, y);
  assert.ok(Math.abs(z - 11.29) < 0.05);
  const index = buildCollisionIndex(tiles.get(tokens[0]), tokens[0]);
  assert.equal(collisionAt([index], { x, y, heading: 0.45 }, z), null);
});

test("the southbound I-280 left exit stays on the upper deck and joins southbound US 101", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ef4", "808f7efc", "808f7f04"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const approach = road("i280-south-beyond-dogpatch");
  const mainline = road("i280-south-to-alemany-mainline");
  const exit = road("i280-south-to-us101-south-exit");
  const descent = road("i280-to-us101-south-descent");
  const lower = road("i280-north-alemany-before-connector-merge");
  for (const [before, after] of [
    [approach, mainline],
    [approach, exit],
    [exit, descent],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, []);
  assert.ok(exit.profiles.length > 0);
  assert.ok(
    exit.profiles.every((id) => roadBake.playableReplaces.includes(id)),
  );
  const copies = tiles.flatMap(({ playableRoads }) =>
    playableRoads.filter(({ id }) => id === exit.id),
  );
  assert.ok(copies.length >= 2);
  for (const copy of copies) assert.deepEqual(copy, exit);
  const lowerSurface = buildBakedRoadContact(lower).surface;
  assert.ok(
    exit.points.some(([east, north, upper]) => {
      const height = lowerSurface.heightAt(east, north);
      return height !== null && upper - height > 6;
    }),
  );
  assert.ok(
    roadBake.playableTransitions.every(
      ({ from, to }) =>
        ![from, to].some((end) => end.startsWith(`${exit.id}:`)) ||
        [from, to].every((end) => !end.startsWith(`${lower.id}:`)),
    ),
  );
  const upperSurface = buildBakedRoadContact(approach).surface;
  const i = upperSurface.profile.findIndex(({ s_m }) => s_m >= 1230);
  const {
    position: [x, y],
  } = upperSurface.profile[i];
  const [a, b] = upperSurface.profile[i + 1].position;
  const heading = Math.atan2(b - y, a - x);
  const leftLane = {
    x: x - 7.25 * Math.sin(heading),
    y: y + 7.25 * Math.cos(heading),
    heading,
  };
  assert.ok(roadSupportsCar(upperSurface.heightAt, leftLane));
});

test("the Geary east portal meets street pavement across both travel lanes", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808580b4.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/geary-masonic-tunnel",
  );
  const [[x, y, z], [u, v]] = road.points;
  const span = Math.hypot(u - x, v - y);
  const east = (u - x) / span;
  const north = (v - y) / span;
  for (const lane of [-6, -4.5, 0, 4.5, 6]) {
    const trail = Array.from({ length: 31 }, (_, i) =>
      groundAtWorld(
        [tile],
        x + (i - 30) * east - lane * north,
        y + (i - 30) * north + lane * east,
        true,
      ),
    );
    assert.ok(trail.every(Number.isFinite));
    assert.ok(Math.abs(trail.at(-1) - z) < 0.01);
    assert.ok(
      trail.slice(1).every((height, i) => Math.abs(height - trail[i]) < 0.06),
    );
  }
});

test("outer lanes keep momentum across the 12 m Dogpatch seam", async () => {
  const { driveOnRoadNetwork, indexRoadNetwork } =
    await import("../sf-road-network.js");
  const tile = JSON.parse(gunzipSync(file("tiles/808f7fcc.json.gz")));
  const pieces = [
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
  ].map((name) =>
    buildBakedRoadContact(
      tile.playableRoads.find((r) => r.id === "sf-contact/" + name),
    ),
  );
  const [from, to] = pieces.map((p) => p.surface);
  const graph = indexRoadNetwork([{ id: "fixture", status: "joined" }]);
  graph.add({
    endpoints: pieces.flatMap((p) => p.endpoints),
    transitions: [
      {
        id: "fixture",
        from: from.end_ids[1],
        to: to.end_ids[0],
        support: "observed",
        direction: "both",
      },
    ],
  });
  const surfaces = new Map(pieces.map((p) => [p.surface.id, p.surface]));
  const [x, y, z] = from.profile.at(-1).position,
    bearing = from.terminalBearings[1];
  for (const side of [-4.4, 4.4])
    for (const direction of [-1, 1]) {
      const heading = bearing + (direction < 0 ? Math.PI : 0);
      let c = {
        x: x - 3 * Math.cos(heading) - side * Math.sin(bearing),
        y: y - 3 * Math.sin(heading) + side * Math.cos(bearing),
        z,
        v: 12,
        heading,
        grounded: true,
        surfaceId: (direction > 0 ? from : to).id,
      };
      for (let n = 0; n < 65; n++) {
        c = driveOnRoadNetwork(
          c,
          {},
          1 / 120,
          surfaces.get(c.surfaceId),
          surfaces,
          graph,
          () => false,
        );
        assert.equal(
          c.blocked,
          false,
          `side ${side}, direction ${direction}, step ${n}`,
        );
      }
      assert.equal(c.surfaceId, (direction > 0 ? to : from).id);
      assert.ok(c.v > 10);
    }
});

test("Marin and Cesar Chavez climb from street level into the northbound Bayshore merge", () => {
  const token = "808f7e4c";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const route = (id) =>
    tile.playableRoads.find((r) => r.id === `sf-contact/${id}`);
  const ramps = [
    route("us101-north-marin-entry"),
    route("us101-north-cesar-chavez-entry"),
  ];
  const merge = route("us101-north-bayshore-entry-merge");
  const through = route("us101-north-potrero-approach");
  for (const ramp of ramps) {
    const { portals } = buildBakedRoadContact(ramp);
    const [{ x, y, z }] = portals;
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.03);
    assert.ok(ramp.points.at(-1)[2] - z > 9);
  }
  for (const [from, to] of [
    ...ramps.map((ramp) => [ramp, merge]),
    [merge, through],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (edge) =>
          edge.from === `${from.id}:end` &&
          edge.to === `${to.id}:start` &&
          edge.direction === "forward",
      ),
    );
  }
  const { surface } = buildBakedRoadContact(merge);
  const x = 3284.69;
  const y = -851.62;
  const z = surface.heightAt(x, y);
  assert.ok(Math.abs(z - 19.91) < 0.1);
  assert.equal(
    collisionAt([buildCollisionIndex(tile, token)], { x, y, heading: 1.69 }, z),
    null,
  );
  assert.ok(roadBake.inferredTreePatches[token] >= 8);
});

test("Mariposa Street stays at the lower level through the viaduct and connects to southbound I-280", () => {
  const token = "808f7fcc";
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [token, "808f7fb4", "808f7fd4"].map((owner) =>
    JSON.parse(gunzipSync(file(`tiles/${owner}.json.gz`))),
  );
  const [tile] = tiles;
  const roads = new Map(
    tiles.flatMap((entry) =>
      entry.playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/i280-south-mariposa-entry");
  const through = roads.get("sf-contact/i280-dogpatch-approach");
  assert.deepEqual(ramp.points.at(-1), through.points[0]);
  assert.deepEqual(ramp.seams.end, through.seams.start);
  assert.ok(
    roadBake.playableTransitions.some(
      (edge) =>
        edge.from === `${ramp.id}:end` &&
        edge.to === `${through.id}:start` &&
        edge.direction === "forward",
    ),
  );
  const { surface, portals } = buildBakedRoadContact(ramp);
  const heightAt = (x, y) => groundAtWorld([tile], x, y, true);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(heightAt(x, y) - z) < 0.03);
  assert.deepEqual(surface.coveredRange, [0, 152]);
  const upper = buildBakedRoadContact(roads.get("sf-contact/i280-18th-exit"));
  const {
    position: [lowerX, lowerY, lowerZ],
  } = surface.profile.find((point) => point.s_m >= 104);
  assert.ok(upper.surface.heightAt(lowerX, lowerY) - lowerZ > 7.5);
  assert.ok(heightAt(lowerX, lowerY) <= lowerZ);

  const index = buildCollisionIndex(tile, token);
  let car = {
    x: 4082,
    y: 448.3,
    z: heightAt(4082, 448.3),
    heading: Math.atan(0.078),
    v: 0,
    grounded: true,
  };
  let airborneSteps = 0;
  for (let frame = 0; frame < 120 * 25 && car.x < 4185; frame++) {
    car = contactStep(
      car,
      { steer: 0, throttle: car.v < 5.5 ? 1 : 0, brake: 0 },
      1 / 120,
      (x, y) => ({ height: heightAt(x, y), drivable: true }),
      1.35,
    );
    if (!car.grounded) {
      airborneSteps++;
      assert.ok(car.z - heightAt(car.x, car.y) < 0.001);
    }
    assert.ok(car.z > 11.25 && car.z < 13.25);
    assert.equal(collisionAt([index], car, car.z), null);
  }
  assert.ok(
    airborneSteps <= 2,
    `Lost ground contact for ${airborneSteps} steps`,
  );
  assert.ok(car.x >= 4185);
  assert.ok(roadBake.terrainDetailPatches[token]);
  assert.ok(!tile.tr.some(([x, y]) => x === 2480 && y === -2627));
});

test("southern northbound US 101 joins the existing Silver approach across tiles", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const owners = [
    "808f7e54",
    "808f7ee4",
    "808f7efc",
    "808f7f1c",
    "808f7f24",
    "808f7f2c",
  ];
  const tiles = new Map(
    owners.map((owner) => [
      owner,
      JSON.parse(gunzipSync(file(`tiles/${owner}.json.gz`))),
    ]),
  );
  const roads = new Map(
    [...tiles.values()].flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const mainline = [
    "us101-north-city-edge-to-hester",
    "us101-north-hester-to-bayshore",
    "us101-north-bayshore-to-paul",
    "us101-north-paul-to-silliman",
    "us101-north-silliman-to-alemany",
    "us101-north-alemany-to-silver",
    "us101-north-silver-approach",
  ].map((name) => roads.get(`sf-contact/${name}`));
  const ramp = roads.get("sf-contact/us101-north-alemany-entry");
  for (const [from, to] of [
    ...mainline.slice(0, -1).map((road, i) => [road, mainline[i + 1]]),
    [ramp, mainline[5]],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0], from.id);
    assert.deepEqual(from.seams.end, to.seams.start, from.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      from.id,
    );
    for (const owner of new Set(from.owners)) {
      assert.deepEqual(
        tiles.get(owner).playableRoads.find(({ id }) => id === from.id),
        from,
      );
    }
    assert.ok(
      from.points.every(
        (point, i) =>
          i === 0 ||
          Math.abs(point[2] - from.points[i - 1][2]) /
            Math.hypot(
              point[0] - from.points[i - 1][0],
              point[1] - from.points[i - 1][1],
            ) <
            (from === ramp ? 0.095 : 0.055),
      ),
    );
  }
  const hesterTile = tiles.get("808f7f24");
  const hester = buildBakedRoadContact(mainline[1]).surface;
  const x = 3657.92,
    y = -4875.17;
  const z = hester.heightAt(x, y);
  assert.ok(z > 40 && z < 41);
  assert.deepEqual(hester.coveredRange, [133, 177]);
  assert.equal(
    collisionAt(
      [buildCollisionIndex(hesterTile, "808f7f24")],
      { x, y, heading: 1.936 },
      z,
    ),
    null,
  );
  assert.ok(
    hesterTile.b.some(
      ([bottom, faces]) =>
        bottom === 480 &&
        faces.some(
          ([roof]) =>
            roof[2] === 488 && roof[0] + hesterTile.o[0] * 10 === 36598,
        ),
    ),
  );
  const { portals } = buildBakedRoadContact(ramp);
  const [entrance] = portals;
  assert.ok(
    Math.abs(
      groundAtWorld([tiles.get(ramp.owners[0])], entrance.x, entrance.y, true) -
        entrance.z,
    ) < 0.05,
  );
});

test("the northbound Silliman fork hands off to Bayshore Boulevard below the deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7efc.json.gz")));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const main = road("us101-north-paul-to-silliman");
  const through = road("us101-north-silliman-to-alemany");
  const exit = road("us101-north-silliman-exit");
  assert.deepEqual(exit.points[0], main.points.at(-1));
  assert.deepEqual(exit.seams.start, through.seams.start);
  for (const fork of [through, exit]) {
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${main.id}:end` &&
          to === `${fork.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(exit.groundEnds, ["end"]);
  const { portals, surface } = buildBakedRoadContact(exit);
  const { x, y, z } = portals[0];
  assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.01);
  assert.ok(z > 22 && z < 23);
  assert.deepEqual(surface.coveredRange, [185, 212]);
  const shadow = surface.profile.find(({ s_m }) => s_m >= 200).position;
  assert.ok(shadow[2] > 22 && shadow[2] < 23);
  assert.equal(
    collisionAt(
      [buildCollisionIndex(tile, "808f7efc")],
      { x: shadow[0], y: shadow[1], heading: 0.5 },
      shadow[2],
    ),
    null,
  );
});

test("the Alemany connector keeps both stacked crossings and the US 101 through fork", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7ee4", "808f7efc", "808f7f1c", "808f7f04", "808f7fac"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const approach = road("us101-north-bayshore-to-paul");
  const shared = road("us101-north-alemany-connector-approach");
  const connector = road("us101-north-to-i280-south");
  const north = road("us101-north-to-i280-north");
  const west = road("i280-north-alemany-before-connector-merge");
  const lowerMainline = road("i280-north-lower-viaduct");
  const through = road("us101-north-paul-to-silliman");
  const merge = road("i280-south-alemany-through");
  for (const [from, to] of [
    [approach, shared],
    [approach, through],
    [shared, connector],
    [shared, north],
    [north, lowerMainline],
    [west, lowerMainline],
    [connector, merge],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  for (const ramp of [shared, connector, north]) {
    for (const token of new Set(ramp.owners)) {
      const tile = tiles[tokens.indexOf(token)];
      assert.deepEqual(
        tile.playableRoads.find(({ id }) => id === ramp.id),
        ramp,
      );
    }
  }
  assert.deepEqual(connector.groundEnds, []);
  const { surface } = buildBakedRoadContact(connector);
  const sharedSurface = buildBakedRoadContact(shared).surface;
  const northSurface = buildBakedRoadContact(north).surface;
  assert.equal(sharedSurface.coveredRange[0], 675);
  assert.equal(surface.coveredRange[0], 0);
  assert.equal(northSurface.coveredRange[0], 0);
  const obstacles = tiles.map((tile, index) =>
    buildCollisionIndex(tile, tokens[index]),
  );
  for (const [upperName, x, y, heading] of [
    ["i280-north-to-us101-south-connector", 3125.926, -3079.465, 1.997],
    ["i280-north-alemany-climb", 2815.336, -2802.036, 3.046],
  ]) {
    const lower = surface.heightAt(x, y);
    const upper = buildBakedRoadContact(road(upperName)).surface.heightAt(x, y);
    assert.ok(Number.isFinite(lower) && Number.isFinite(upper));
    assert.ok(upper - lower > 7 && upper - lower < 8);
    assert.equal(collisionAt(obstacles, { x, y, heading }, lower), null);
  }
  const measuredLane = { x: 3118.432, y: -3041.715, heading: 1.95 };
  assert.ok(roadSupportsCar(northSurface.heightAt, measuredLane));
  assert.ok(
    Math.abs(northSurface.heightAt(measuredLane.x, measuredLane.y) - 31.1) <
      0.15,
  );
});

test("Bayshore stays on the upper street through the Hester fork and merges onto US 101", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const approach = road("bayshore-north-hester-approach");
  const street = road("bayshore-north-hester-through");
  const ramp = road("us101-north-bayshore-hester-entry");
  const highway = road("us101-north-bayshore-to-paul");
  for (const [from, to] of [
    [approach, street],
    [approach, ramp],
    [ramp, highway],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(ramp.groundEnds, []);
  const contact = buildBakedRoadContact(approach);
  for (const { x, y, z } of [
    ...contact.portals,
    ...buildBakedRoadContact(street).portals,
  ]) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const x = 3641.582;
  const y = -4906.127;
  const z = contact.surface.heightAt(x, y);
  const ground = groundAtWorld([tile], x, y, true);
  assert.ok(Number.isFinite(z) && Number.isFinite(ground) && z - ground > 7);
  assert.equal(
    collisionAt([buildCollisionIndex(tile, token)], { x, y, heading: 1.2 }, z),
    null,
  );
});

test("both southbound Hester entrances stay below US 101 and join the collector fork", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const third = road("us101-south-hester-third-entry");
  const salinas = road("us101-south-hester-salinas-entry");
  const collector = road("us101-south-hester-entrance-collector");
  const south = road("us101-south-hester-collector-return");
  const street = road("us101-south-hester-bayshore-exit");
  for (const [from, to] of [
    [third, collector],
    [salinas, collector],
    [collector, south],
    [collector, street],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  for (const ramp of [third, salinas]) {
    const [{ x, y, z }] = buildBakedRoadContact(ramp).portals;
    assert.deepEqual(ramp.groundEnds, ["start"]);
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  assert.ok(Math.abs(third.points[0][0] - 3809.5041) < 0.01);
  assert.deepEqual(collector.groundEnds, []);
  assert.ok(Math.abs(collector.points[0][2] - 34.2) < 0.01);
  const upper = road("us101-north-hester-to-bayshore");
  const [x, y] = [3569.7184, -4516.3596];
  const upperHeight = buildBakedRoadContact(upper).surface.heightAt(x, y);
  const lowerHeight = buildBakedRoadContact(third).surface.heightAt(x, y);
  assert.ok(Number.isFinite(upperHeight) && Number.isFinite(lowerHeight));
  assert.ok(upperHeight - lowerHeight > 8 && upperHeight - lowerHeight < 9);
});

test("northbound Hester retains the mainline and both exits reach city pavement", () => {
  const token = "808f7f24";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tile.playableRoads.find(({ id }) => id === `sf-contact/${name}`);
  const approach = road("us101-north-city-edge-to-hester");
  const mainline = road("us101-north-hester-to-bayshore");
  const fork = road("us101-north-hester-off-ramp");
  const third = road("us101-north-hester-third-off-ramp");
  const bayshore = road("us101-north-hester-bayshore-off-ramp");
  for (const [from, to] of [
    [approach, mainline],
    [approach, fork],
    [fork, third],
    [fork, bayshore],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(fork.groundEnds, []);
  for (const exit of [third, bayshore]) {
    assert.deepEqual(exit.groundEnds, ["end"]);
    const [{ x, y, z }] = buildBakedRoadContact(exit).portals;
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const obstacles = [buildCollisionIndex(tile, token)];
  const throughPose = { x: 3728.98, y: -5012.988, heading: 2.0099 };
  const throughHeight = buildBakedRoadContact(mainline).surface.heightAt(
    throughPose.x,
    throughPose.y,
  );
  assert.ok(Number.isFinite(throughHeight));
  assert.equal(collisionAt(obstacles, throughPose, throughHeight), null);
  const reentry = buildBakedRoadContact(road("us101-south-hester-third-entry"));
  for (const [x, y, heading] of [
    [3678.8, -4457.4, 3.46],
    [3582.029, -4503.235, -2.365],
  ]) {
    const z = reentry.surface.heightAt(x, y);
    assert.ok(Number.isFinite(z));
    assert.equal(collisionAt(obstacles, { x, y, heading }, z), null);
  }
  for (const [ramp, x, y] of [
    [fork, 3670.3473, -4817.6891],
    [third, 3704.4769, -4632.0553],
  ]) {
    const z = buildBakedRoadContact(ramp).surface.heightAt(x, y);
    const separation = z - groundAtWorld([tile], x, y, true);
    assert.ok(separation > 0.08 && separation < 0.17);
    assert.equal(collisionAt(obstacles, { x, y, heading: 1.4 }, z), null);
  }
});

test("west Yerba Buena ramps join the island to the correct Bay Bridge decks", () => {
  const tokens = ["8085803c", "80858024"];
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const road = (name) =>
    tiles[0].playableRoads.find(
      ({ id }) => id === name || id === `sf-contact/${name}`,
    );
  const fromCity = road("bay-bridge-eastbound-lower");
  const lowerThrough = road("yerba-buena-lower-west-approach");
  const toIsland = road("yerba-buena-eastbound-off-ramp");
  const fromIsland = road("yerba-buena-westbound-on-ramp");
  const upperThrough = road("yerba-buena-west-approach");
  const toCity = road("bay-bridge-westbound");
  for (const [from, to] of [
    [fromCity, lowerThrough],
    [fromCity, toIsland],
    [fromIsland, toCity],
    [upperThrough, toCity],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0]);
    assert.deepEqual(from.seams.end, to.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from: a, to: b, direction, status }) =>
          a === `${from.id}:end` &&
          b === `${to.id}:start` &&
          direction === "forward" &&
          status === "joined",
      ),
    );
  }
  assert.deepEqual(toIsland.groundEnds, ["end"]);
  assert.deepEqual(fromIsland.groundEnds, ["start"]);
  const lower = buildBakedRoadContact(toIsland);
  const upper = buildBakedRoadContact(fromIsland);
  for (const { x, y, z } of [...lower.portals, ...upper.portals]) {
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  }
  const obstacles = tiles.map((tile, i) =>
    buildCollisionIndex(tile, tokens[i]),
  );
  for (const [x, y] of [
    [6420.6852, 5368.4154],
    [6440, 5402],
    [6329.863, 5420.266],
  ]) {
    const bottom = lower.surface.heightAt(x, y);
    const top = upper.surface.heightAt(x, y);
    assert.ok(Number.isFinite(top) && Number.isFinite(bottom));
    assert.ok(top - bottom > 7 && top - bottom < 9);
    assert.ok(groundAtWorld(tiles, x, y, true) - bottom < 0.04);
    assert.equal(collisionAt(obstacles, { x, y, heading: 1.01 }, bottom), null);
    assert.equal(collisionAt(obstacles, { x, y, heading: -2.1 }, top), null);
  }
  const upperPoints = fromIsland.points;
  let station = 0;
  let probes = 0;
  for (let i = 1; i < upperPoints.length; i++) {
    const [a, b] = upperPoints[i - 1];
    const [x, y] = upperPoints[i];
    const distance = Math.hypot(x - a, y - b);
    station += distance;
    if (station < 220 || station > 308) continue;
    for (const offset of [-2.65, 2.65]) {
      const east = x - (offset * (y - b)) / distance;
      const north = y + (offset * (x - a)) / distance;
      const deck = upper.surface.heightAt(east, north);
      assert.ok(Number.isFinite(deck));
      assert.ok(groundAtWorld(tiles, east, north, true) - deck < 0.1);
      probes++;
    }
  }
  assert.ok(probes > 60);
});

test("Minna Street remains drivable under the retained building bridge", () => {
  const token = "80858084";
  const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  const obstacles = [buildCollisionIndex(tile, token)];
  const [startX, startY] = [3000.9, 2475.6];
  const heading = Math.atan2(2407.2 - startY, 2932.2 - startX);
  const along = Math.cos(heading);
  const north = Math.sin(heading);
  function pose(station, lateral) {
    return {
      x: startX + station * along - lateral * north,
      y: startY + station * north + lateral * along,
      heading,
    };
  }
  for (const lateral of [-1.5, 0, 1.5]) {
    let previous;
    for (let station = 13; station <= 85; station++) {
      const car = pose(station, lateral);
      const z = groundAtWorld([tile], car.x, car.y, true);
      assert.ok(z >= 8 && z <= 9.1, `pavement at ${station} m`);
      if (previous !== undefined) assert.ok(Math.abs(z - previous) < 0.12);
      previous = z;
      assert.equal(
        collisionAt(obstacles, car, z),
        null,
        `ground clearance at ${station} m, lateral ${lateral} m`,
      );
    }
  }
  assert.equal(collisionAt(obstacles, pose(42, 0), 13), "building");
  for (const lateral of [-5.5, 5.5]) {
    const car = pose(42, lateral);
    const z = groundAtWorld([tile], car.x, car.y, true);
    assert.equal(collisionAt(obstacles, car, z), "building");
  }
});

test("the Macalla loop merges into westbound I-80 at the upper level", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["8085801c", "80858024", "8085803c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads = [] }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const ramp = roads.get("sf-contact/macalla-road-westbound-i80-entrance");
  const near = roads.get("yerba-buena-east-approach");
  const through = roads.get("sf-contact/yerba-buena-upper-after-macalla-merge");
  const tunnel = roads.get("yerba-buena-upper-tunnel");
  assert.ok(ramp, "the signed entrance has its own playable contact");
  for (const [before, after] of [
    [near, through],
    [ramp, through],
    [through, tunnel],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }
  const { surface, portals } = buildBakedRoadContact(ramp);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  assert.ok(surface.profile.at(-1).s_m > 300);
  assert.ok(groundAtWorld(tiles, 6710.14, 5802.76, true) < 40);
  assert.ok(surface.heightAt(6710.14, 5802.76) > 56);
  const index = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  for (let i = 1; i < ramp.points.length; i++) {
    const [a, b, h] = ramp.points[i - 1];
    const [x, y, z] = ramp.points[i];
    const heading = Math.atan2(y - b, x - a);
    assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.12);
    for (const lateral of [-2.25, 0, 2.25]) {
      const car = {
        x: x - lateral * Math.sin(heading),
        y: y + lateral * Math.cos(heading),
        heading,
      };
      assert.equal(collisionAt(index, car, z), null, `Macalla at ${x}, ${y}`);
    }
  }
  const drive = JSON.parse(file("gamedrives.json")).drives[0];
  const named = cityDriveApproach(drive, "yerba-buena-westbound");
  for (const road of [near, through]) {
    assert.deepEqual(
      named.sections.find(({ id }) => id === road.id).positions,
      road.points,
    );
  }
});

test("the CA 1 North connector has its own road level between two ground portals", () => {
  const token = "808f7c44";
  const raw = file(`tiles/${token}.json.gz`);
  const tile = JSON.parse(gunzipSync(raw));
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const { tiles } = roadBake.dalyCa1NorthConnectorSurvey;
  assert.deepEqual(Object.keys(tiles), [token]);
  assert.equal(tiles[token].sha256, digest(raw));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/daly-ca1-north-connector",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  assert.deepEqual(road.owners, [token, token]);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals)
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);

  const index = buildCollisionIndex(tile, token);
  for (const [x, y, observed] of [
    [-2715.7695, -5965.4002, 76.29],
    [-2717.6977, -5927.4494, 76.55],
  ]) {
    const car = { x, y, heading: Math.PI / 2 };
    const z = surface.heightAt(x, y);
    const ground = groundAtWorld([tile], x, y, true);
    assert.ok(Number.isFinite(ground) && z - ground > 10);
    assert.ok(Number.isFinite(z) && Math.abs(z - observed) < 0.02);
    assert.ok(roadSupportsCar(surface.heightAt, { ...car, z }));
    assert.equal(collisionAt([index], car, z), null);
  }
});

test("John Daly exit and lower I-280 approach rejoin at the southbound entrance", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7c3c", "808f7c44"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const exit = road("i280-south-john-daly-exit");
  const upper = road("john-daly-westbound-overpass");
  const entry = road("john-daly-southbound-i280-entry");
  const tunnel = road("i280-south-daly-separation");
  const through = road("i280-daly-tunnel-to-return-merge");
  const merge = road("i280-south-after-john-daly-merge");
  for (const tile of tiles) {
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === exit.id),
      exit,
    );
  }
  const transitions = manifest.roadBake.playableTransitions;
  for (const [before, after] of [
    [road("i280-south-edge-exit"), exit],
    [exit, upper],
    [road("john-daly-westbound"), upper],
    [upper, entry],
    [tunnel, through],
    [entry, merge],
    [through, merge],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      transitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
    assert.ok(!before.groundEnds.includes("end"));
  }
  const crossing = [-2799.381, -6099.975];
  const top = buildBakedRoadContact(upper).surface.heightAt(...crossing);
  const bottom = buildBakedRoadContact(tunnel).surface.heightAt(...crossing);
  assert.ok(
    Number.isFinite(top) && Number.isFinite(bottom) && top - bottom > 14,
  );
  const [x, y, z] = merge.points.at(-1);
  assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
});

test("both Golden Gate carriageways connect the Presidio and Marin above terrain", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const coverage = JSON.parse(file("coverage.json"));
  const tokens = [
    "808586dc",
    "808586e4",
    "808586ec",
    "80858694",
    "8085869c",
    "80858424",
    "8085842c",
    "80858434",
    "808585cc",
  ];
  const tiles = tokens.map((token) => {
    assert.ok(
      manifest.tiles.some(({ t }) => t === token),
      token,
    );
    assert.equal(coverage.cells[token].length, 4, token);
    return JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
  });
  const indexes = tiles.map((tile) => buildCollisionIndex(tile, tile.t));
  const roads = new Map(
    tiles.flatMap(({ playableRoads = [] }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const road = (name) => roads.get(`sf-contact/${name}`);
  const assertForwardJoin = (previous, next, departure = "end") => {
    assert.ok(previous && next, "Missing Golden Gate contact");
    const terminal =
      departure === "end" ? previous.points.at(-1) : previous.points[0];
    assert.deepEqual(terminal, next.points[0], next.id);
    assert.deepEqual(previous.seams[departure], next.seams.start, next.id);
    assert.ok(
      manifest.roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${previous.id}:${departure}` &&
          to === `${next.id}:start` &&
          direction === "forward",
      ),
      next.id,
    );
  };
  const visitorMerge = road("golden-gate-northbound-from-visitor");
  const merchantFork = road("golden-gate-southbound-to-merchant");
  const vistaExit = road("golden-gate-vista-exit");
  const vistaApproach = road("golden-gate-northbound-vista-mainline");
  const northFork = road("us101-north-before-alexander-fork");
  const northApproach = road("golden-gate-northbound-past-vista");
  const northOverpass = road("us101-north-over-alexander");
  const northMerge = road("us101-north-after-alexander-merge");
  const waldoNorth = road("robin-williams-nb");
  const waldoSouth = road("robin-williams-sb");
  const southFork = road("us101-south-alexander-fork");
  const southOverpass = road("us101-south-over-alexander");
  const southApproach = road("us101-south-from-alexander-to-golden-gate");
  const southMerge = road("golden-gate-southbound-to-alexander-merge");
  const assertGroundPortal = (contact) => {
    const portals = buildBakedRoadContact(contact).portals;
    assert.equal(portals.length, 1, contact.id);
    const [{ x, y, z }] = portals;
    assert.ok(
      Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.025,
      contact.id,
    );
  };
  for (const direction of ["northbound", "southbound"]) {
    const crossing = Array.from({ length: 6 }, (_, i) =>
      road(`golden-gate-${direction}-${i + 1}`),
    );
    const northbound = direction === "northbound";
    const doyle = road(
      `presidio-doyle-${direction}-${northbound ? "exit" : "entry"}`,
    );
    const chain = northbound
      ? [
          doyle,
          crossing[0],
          visitorMerge,
          ...crossing.slice(1),
          vistaApproach,
          northFork,
          northApproach,
          northOverpass,
          northMerge,
          road("101-north-alexander-to-waldo"),
          waldoNorth,
        ]
      : [
          waldoSouth,
          road("101-south-waldo-to-alexander"),
          southFork,
          southOverpass,
          southApproach,
          southMerge,
          ...crossing.slice(0, -1),
          merchantFork,
          crossing.at(-1),
          doyle,
        ];
    for (let i = 1; i < chain.length; i++)
      assertForwardJoin(chain[i - 1], chain[i]);

    let span = 0;
    for (const [section, road] of crossing.entries()) {
      const { surface } = buildBakedRoadContact(road);
      if (section > 0 && section < crossing.length - 1) {
        assert.equal(road.width, 9.4);
        assert.deepEqual(surface.railSides, ["left", "right"]);
        span += surface.profile.at(-1).s_m;
      }
      for (let i = 1; i < road.points.length; i++) {
        const [a, b, h] = road.points[i - 1];
        const [x, y, z] = road.points[i];
        const heading = Math.atan2(y - b, x - a);
        assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05, road.id);
        for (const lateral of [-2.9, 0, 2.9]) {
          const car = {
            x: x - lateral * Math.sin(heading),
            y: y + lateral * Math.cos(heading),
            heading,
          };
          assert.equal(
            collisionAt(indexes, car, z),
            null,
            `${road.id} ${i} lateral ${lateral}`,
          );
        }
      }
    }
    assert.ok(span > 2700 && span < 2800, `${direction} span ${span}`);
    assertGroundPortal(northbound ? waldoNorth : waldoSouth);
    const [deckX, deckY, deckZ] = crossing[2].points[190];
    assert.ok(groundAtWorld(tiles, deckX, deckY, true) < deckZ - 65);
  }
  const vistaEntry = road("golden-gate-vista-entry");
  const eastAlexander = road("alexander-avenue-east-approach");
  const westAlexander = road("alexander-avenue-west-to-conzelman");
  const northExit = road("us101-north-alexander-exit");
  const northEntry = road("us101-north-alexander-entry");
  const southExit = road("us101-south-alexander-exit");
  const southEntry = road("us101-south-alexander-entry");
  for (const [from, to] of [
    [road("golden-gate-visitor-entry"), visitorMerge],
    [merchantFork, road("golden-gate-merchant-exit")],
    [road("golden-gate-northbound-6"), vistaExit],
    [vistaEntry, northFork],
    [northFork, northExit],
    [northExit, eastAlexander],
    [northEntry, northMerge],
    [southFork, southExit],
    [westAlexander, southEntry],
    [southEntry, road("golden-gate-southbound-1")],
  ])
    assertForwardJoin(from, to);
  assertForwardJoin(eastAlexander, northEntry, "start");
  for (const road of [vistaEntry, vistaExit, westAlexander, southExit]) {
    assertGroundPortal(road);
  }

  const underpass = buildBakedRoadContact(
    road("alexander-avenue-under-us101"),
  ).surface;
  for (const upper of [northMerge, southOverpass]) {
    const gaps = upper.points.flatMap(([x, y, z]) => {
      const floor = underpass.heightAt(x, y);
      return floor === null ? [] : [z - floor];
    });
    assert.ok(gaps.length > 0, upper.id);
    assert.ok(
      gaps.every((gap) => gap > 6.5),
      upper.id,
    );
  }
});

test("Main Street stays open beneath both Bay Bridge decks", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/80858074.json.gz")));
  const obstacles = [buildCollisionIndex(tile, tile.t)];
  const heading = (3 * Math.PI) / 4;
  const pose = (x, offset) => ({
    x: x - offset * Math.sin(heading),
    y: 7499.4 - x + offset * Math.cos(heading),
    heading,
  });
  const hit = (car) =>
    collisionAt(obstacles, car, groundAtWorld([tile], car.x, car.y, true));
  for (let x = 4439; x <= 4478; x += 0.5) {
    for (const offset of [-3, 0, 3]) {
      assert.equal(
        hit(pose(x, offset)),
        null,
        `Main Street at ${x}, ${offset}`,
      );
    }
  }
  assert.equal(hit(pose(4467, -9)), "building");
  assert.equal(hit(pose(4467, 9)), "building");
  const { x, y } = pose(4461, 0);
  const height = (id) =>
    buildBakedRoadContact(
      tile.playableRoads.find((road) => road.id === id),
    ).surface.heightAt(x, y);
  assert.ok(groundAtWorld([tile], x, y, true) < 6);
  assert.ok(Math.abs(height("sf-eastbound-lower-approach") - 41.39) < 0.02);
  assert.ok(Math.abs(height("city-approach") - 50.18) < 0.02);
});

test("Harrison traffic crosses the Fourth Street ramp mouth without taking it", () => {
  const tiles = ["8085807c", "80858084"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const road = tiles[0].playableRoads.find(
    ({ id }) => id === "fourth-street-entrance",
  );
  const { surface, portals } = buildBakedRoadContact(road);
  const [{ x, y, nx, ny }] = portals;
  const ground = (east, north) => groundAtWorld(tiles, east, north, true);
  for (const [east, north, heading, wantsRamp] of [
    [3570.3, 2321.8, (-3 * Math.PI) / 4, false],
    [x - 2 * nx, y - 2 * ny, Math.atan2(ny, nx), true],
  ]) {
    let car = {
      x: east,
      y: north,
      z: ground(east, north),
      heading,
      v: 10,
      grounded: true,
    };
    for (let step = 0; step < 480; step++) {
      const input = { throttle: 0.1 };
      car =
        driveThroughRoadPortal(car, input, 1 / 120, [portals], ground) ??
        contactStep(
          car,
          input,
          1 / 120,
          (px, py) => ({
            height: ground(px, py),
            drivable: true,
          }),
          1.35,
        );
      assert.equal(car.blocked, false);
      assert.ok(Math.abs(car.z - ground(car.x, car.y)) < 0.3);
      if (car.surfaceId || car.x < 3543) break;
    }
    assert.equal(car.surfaceId, wantsRamp ? surface.id : undefined);
    if (!wantsRamp)
      assert.ok(car.x < 3543, "through traffic cleared the mouth");
  }
});

test("Richland and Highland carry traffic above the San Jose Avenue cut", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const obstacles = [buildCollisionIndex(tile, tile.t)];
  const ground = (x, y) => groundAtWorld([tile], x, y, true);
  for (const [id, length, clearance] of [
    ["richland", 85, 13],
    ["highland", 80, 10],
  ]) {
    const road = tile.playableRoads.find(
      ({ id: name }) => name === `sf-contact/${id}-avenue-bridge`,
    );
    const { surface, portals } = buildBakedRoadContact(road);
    assert.equal(road.width, 7.2);
    assert.equal(portals.length, 2);
    assert.ok(surface.profile.at(-1).s_m > length);
    for (const { x, y, z } of portals) {
      assert.ok(Math.abs(ground(x, y) - z) < 0.02);
    }
    const [east, north, height] =
      road.points[Math.floor(road.points.length / 2)];
    assert.ok(height - ground(east, north) > clearance);
    for (let i = 1; i < road.points.length; i++) {
      const [x, y, z] = road.points[i];
      const [a, b, h] = road.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05);
      for (const offset of [-2.4, 0, 2.4]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(collisionAt(obstacles, car, z), null, `${id} point ${i}`);
      }
    }
  }
  for (const [x, y] of [
    [1178.9, -2628.1],
    [1308.1, -2488.5],
  ]) {
    const z = ground(x, y);
    assert.ok(z > 55 && z < 60);
    assert.equal(collisionAt(obstacles, { x, y, heading: -2.3 }, z), null);
  }
});

test("Ocean Avenue crosses above I-280 and the northbound entrance joins at Monterey", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c2c",
    "808f7e84",
    "808f7e7c",
    "808f7e64",
    "808f7ef4",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const crossing = roads.get("sf-contact/ocean-avenue-bridge");
  const survey = roadBake.oceanAvenueLaneWidthSurvey;
  assert.equal(survey.stations_per_band, 98);
  assert.deepEqual(crossing.widthSections, [[0, 9.35, 12.4]]);
  for (const owner of crossing.owners) {
    assert.equal(
      survey.tiles[owner].sha256,
      digest(file(`tiles/${owner}.json.gz`)),
    );
    const tile = tiles.find(({ t }) => t === owner);
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === crossing.id),
      crossing,
    );
  }
  const { surface, portals } = buildBakedRoadContact(crossing);
  assert.equal(portals.length, 2);
  const [startX, startY] = crossing.points[0];
  const [endX, endY] = crossing.points.at(-1);
  for (const x of [-650, -665, -690, -710, -728]) {
    const y = startY + ((x - startX) / (endX - startX)) * (endY - startY);
    for (const lane of [-10, 4.6, 7.7]) {
      assert.ok(
        roadSupportsCar(surface.heightAt, { x, y: y - lane, heading: Math.PI }),
      );
    }
    assert.equal(surface.heightAt(x, y + 13), null);
    assert.equal(surface.heightAt(x, y - 10), null);
  }
  assert.ok(
    surface.heightAt(-690, -4111.4) -
      groundAtWorld(tiles, -690, -4111.4, true) >
      6.5,
  );
  for (const { x, y, z } of portals) {
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
  }

  const chain = [
    "i280-north-ocean-entry",
    "i280-north-ocean-connector",
    "i280-north-ocean-mainline",
    "i280-north-ocean-monterey",
    "branch-east-i280-northbound",
    "i280-north-monterey-merge",
    "i280-north-monterey-to-alemany",
  ].map((slug) => roads.get(`sf-contact/${slug}`));
  const [entry] = chain;
  const [east, north, height] = entry.points[0];
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - height) < 0.02);
  const [streetX, streetY] = crossing.points[0];
  assert.ok(Math.hypot(east - streetX, north - streetY) > 12);
  assert.deepEqual(entry.groundEnds, ["start"]);
  assert.deepEqual(chain[4].groundEnds, ["start"]);
  for (let i = 1; i < chain.length; i++) {
    const a = chain[i - 1];
    const b = chain[i];
    assert.deepEqual(a.points.at(-1), b.points[0]);
    assert.deepEqual(a.seams.end, b.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${a.id}:end` &&
          to === `${b.id}:start` &&
          direction === "forward",
      ),
      b.id,
    );
  }

  const decks = deckClearanceIndex();
  for (const road of roads.values()) {
    if (road.kind === "tunnel") continue;
    const contact = buildBakedRoadContact(road).surface.contact;
    const geometry = contactGeometry(contact, false);
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  for (const road of [crossing, ...chain.slice(0, -1)]) {
    for (let i = 1; i < road.points.length; i++) {
      const [x, y, z] = road.points[i];
      const [a, b, h] = road.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.1, road.id);
      const lane = road.width < 10 ? 2.2 : 3.6;
      for (const offset of [-lane, 0, lane]) {
        const car = {
          x: x - offset * Math.sin(heading),
          y: y + offset * Math.cos(heading),
          heading,
        };
        assert.equal(
          collisionAt(obstacles, car, z),
          null,
          `${road.id} point ${i}, offset ${offset}`,
        );
      }
    }
  }
});

test("Geneva keeps both I-280 carriageways and their four ramps connected", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c24",
    "808f7c2c",
    "808f7e64",
    "808f7e7c",
    "808f7e84",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) => tile.playableRoads.map((road) => [road.id, road])),
  );
  const road = (slug) => roads.get(`sf-contact/i280-${slug}`);
  const north = road("north-geneva-to-ocean");
  const south = road("south-ocean-to-geneva");
  for (const [a, b] of [
    ["north-geneva-approach", "north-geneva-to-ocean"],
    ["north-geneva-approach", "north-geneva-exit"],
    ["north-geneva-to-ocean", "north-ocean-mainline"],
    ["north-geneva-entry", "north-ocean-connector"],
    ["south-monterey-to-ocean", "south-ocean-to-geneva"],
    ["south-ocean-approach", "south-geneva-exit"],
    ["south-ocean-to-geneva", "south-beyond-geneva"],
    ["south-geneva-entry", "south-beyond-geneva"],
  ]) {
    const from = road(a);
    const to = road(b);
    assert.deepEqual(from.points.at(-1), to.points[0], to.id);
    assert.deepEqual(from.seams.end, to.seams.start, to.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      to.id,
    );
  }
  for (const direction of ["north", "south"]) {
    const exit = road(`${direction}-geneva-exit`);
    const entry = road(`${direction}-geneva-entry`);
    const exitLane = direction === "north" ? "westbound" : "eastbound";
    const through = roads.get(`sf-contact/geneva-avenue-${exitLane}-over-i280`);
    const oppositeEntry = road(
      `${direction === "north" ? "south" : "north"}-geneva-entry`,
    );
    assert.deepEqual(exit.groundExitOnly, ["end"]);
    assert.deepEqual(entry.groundEnds, ["start"]);
    assert.deepEqual(exit.points.at(-1), entry.points[0]);
    const [x, y, z] = entry.points[0];
    assert.ok(Math.abs(groundAtWorld(tiles, x, y, true) - z) < 0.02);
    for (const [first, second] of [
      [exit, through],
      [through, oppositeEntry],
    ]) {
      assert.deepEqual(first.points.at(-1), second.points[0]);
      assert.deepEqual(first.seams.end, second.seams.start);
      const joins = roadBake.playableTransitions.filter(
        ({ from }) => from === `${first.id}:end`,
      );
      assert.deepEqual(
        joins.map(({ to, direction }) => [to, direction]),
        [[`${second.id}:start`, "forward"]],
      );
    }
  }

  const decks = deckClearanceIndex();
  for (const item of roads.values()) {
    if (item.kind === "tunnel") continue;
    const contact = buildBakedRoadContact(item).surface.contact;
    const geometry = contactGeometry(contact, false);
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  for (const item of [
    north,
    south,
    road("north-geneva-approach"),
    road("south-beyond-geneva"),
    ...["north", "south"].flatMap((direction) => [
      road(`${direction}-geneva-entry`),
      road(`${direction}-geneva-exit`),
    ]),
  ]) {
    for (const owner of item.owners) {
      assert.deepEqual(
        tiles
          .find(({ t }) => t === owner)
          .playableRoads.find(({ id }) => id === item.id),
        item,
      );
    }
    const { surface } = buildBakedRoadContact(item);
    for (const [i, [x, y, z]] of item.points.entries()) {
      if (!i) continue;
      const [a, b, h] = item.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.1, item.id);
      assert.equal(collisionAt(obstacles, { x, y, heading }, z), null, item.id);
    }
    const [x, y, z] = item.points[Math.floor(item.points.length / 2)];
    assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.005, item.id);
  }
  for (const [item, x, y, wanted] of [
    [north, -705.46, -4308.72, 68.89],
    [south, -724.16, -4297.39, 69.44],
    [road("north-geneva-entry"), -657.73, -4110.48, 64.79],
    [road("south-geneva-exit"), -713.81, -4107.39, 66.5],
  ]) {
    const z = buildBakedRoadContact(item).surface.heightAt(x, y);
    assert.ok(Math.abs(z - wanted) < 0.08, item.id);
    assert.ok(groundAtWorld(tiles, x, y, true) - z > 1.5, item.id);
  }
});

test("Blanken Avenue crosses below US 101 and releases at both street portals", () => {
  const tile = JSON.parse(gunzipSync(file("tiles/808f7f2c.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/blanken-avenue-underpass",
  );
  const { surface, portals, appearance } = buildBakedRoadContact(road);
  assert.equal(appearance.clearance, 4.5);
  assert.equal(portals.length, 2);
  assert.ok(appearance.interior[1] - appearance.interior[0] > 55);
  for (const { x, y, z } of portals) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const [x, y, z] = road.points[Math.floor(road.points.length / 2)];
  assert.ok(groundAtWorld([tile], x, y, true) - z > 10);
  assert.ok(Math.abs(surface.heightAt(x, y) - z) < 0.01);
  for (const [slug, east, north] of [
    ["us101-north-city-edge-to-hester", 3930, -5446.5],
    ["us101-south-hester-to-city-edge", 3907, -5447.5],
  ]) {
    const upper = buildBakedRoadContact(
      tile.playableRoads.find(({ id }) => id === `sf-contact/${slug}`),
    ).surface.heightAt(east, north);
    const lower = surface.heightAt(east, north);
    assert.ok(Number.isFinite(upper) && Number.isFinite(lower));
    assert.ok(upper - lower > 11, slug);
  }
  const paint = tile.r.filter(([name]) => tile.n[name] === "Blanken Ave");
  assert.ok(paint.length > 1);
  const tileEast = Math.round((x - tile.o[0]) * 10);
  for (const [, , , line] of paint) {
    const east = line.filter((_, i) => i % 2 === 0);
    assert.ok(Math.max(...east) < tileEast || Math.min(...east) > tileEast);
  }

  const obstacles = [buildCollisionIndex(tile, tile.t)];
  for (const [i, [x, y, z]] of road.points.entries()) {
    if (!i) continue;
    const [a, b, h] = road.points[i - 1];
    const heading = Math.atan2(y - b, x - a);
    assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.05);
    for (const offset of [-2.5, 0, 2.5]) {
      const car = {
        x: x - offset * Math.sin(heading),
        y: y + offset * Math.cos(heading),
        heading,
      };
      assert.equal(collisionAt(obstacles, car, z), null, `point ${i}`);
    }
  }
});

test("Candlestick carries the exit above Alana Way and preserves the through lane", () => {
  const manifest = JSON.parse(file("tiles/manifest.json"));
  const owner = "808f7f2c";
  const adjacent = "808f7f24";
  const load = (token) => {
    const raw = file(`tiles/${token}.json.gz`);
    const original = manifest.roadBake.candlestickExitSurvey.tiles[token];
    const hester = manifest.roadBake.hesterThirdApproachSurvey.tiles[token];
    if (hester) assert.equal(hester.parentSha256, original.sha256);
    const tree = manifest.roadBake.roadTreeClearanceReviews.findLast(
      ({ tiles }) => hester && tiles[token]?.parentSha256 === hester.sha256,
    );
    assert.equal(
      tree?.tiles[token].sha256 ?? hester?.sha256 ?? original.sha256,
      digest(raw),
    );
    return JSON.parse(gunzipSync(raw));
  };
  const tile = load(owner);
  const approachId = "sf-contact/us101-south-hester-to-city-edge";
  const throughId = "sf-contact/us101-south-past-candlestick-exit";
  const exitId = "sf-contact/us101-south-candlestick-exit";
  const find = (id) => tile.playableRoads.find((road) => road.id === id);
  const approach = find(approachId);
  const through = find(throughId);
  const exit = find(exitId);
  assert.deepEqual(
    load(adjacent).playableRoads.find(({ id }) => id === approachId),
    approach,
  );
  for (const road of [through, exit]) {
    assert.deepEqual(road.points[0], approach.points.at(-1));
    assert.deepEqual(road.seams.start, approach.seams.end);
  }
  const branches = manifest.roadBake.playableTransitions.filter(
    ({ from }) => from === `${approachId}:end`,
  );
  assert.deepEqual(
    branches.map(({ to }) => to),
    [`${throughId}:start`, `${exitId}:start`],
  );
  assert.deepEqual(branches[1].from_lateral_range_m, [-6.5, -2.2]);

  const { surface, portals } = buildBakedRoadContact(exit);
  const lower = buildBakedRoadContact(
    find("sf-contact/alana-way-under-us101"),
  ).surface;
  const crossing = [3932.003, -5686.35];
  assert.ok(Math.abs(surface.heightAt(...crossing) - 12.704) < 0.03);
  assert.ok(Math.abs(lower.heightAt(...crossing) - 5.947) < 0.03);
  assert.equal(portals.length, 1);
  const [{ x, y, z }] = portals;
  assert.ok(Math.abs(groundAtWorld([tile], x, y, false) - z) < 0.02);
});

test("I-280 follows the surveyed carriageways and directed SR 82 ramps south of Geneva", () => {
  const { tiles: owners, roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = [
    "808f7c24",
    "808f7c2c",
    "808f7c34",
    "808f7c3c",
    "808f7e84",
  ].map((token) => JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))));
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const road = (slug) => roads.get(`sf-contact/i280-${slug}`);
  const northFirst = road("north-edge-to-sr82");
  const northNext = road("north-sr82-to-geneva");
  const northApproach = road("north-geneva-approach");
  const northEntry = road("north-sr82-entry");
  const southApproach = road("south-beyond-geneva");
  const southNext = road("south-geneva-to-sr82");
  const southLast = road("south-sr82-to-edge");
  const southExit = road("south-sr82-exit");
  const edgeExit = road("south-edge-exit");

  for (const [from, to] of [
    [northFirst, northNext],
    [northEntry, northNext],
    [northNext, northApproach],
    [southApproach, southNext],
    [southApproach, southExit],
    [southNext, southLast],
    [southNext, edgeExit],
  ]) {
    assert.deepEqual(from.points.at(-1), to.points[0], to.id);
    assert.deepEqual(from.seams.end, to.seams.start, to.id);
    assert.ok(
      roadBake.playableTransitions.some(
        (join) =>
          join.from === `${from.id}:end` &&
          join.to === `${to.id}:start` &&
          join.direction === "forward",
      ),
      to.id,
    );
  }
  for (const item of [northApproach, southApproach])
    assert.deepEqual(item.groundEnds, [], item.id);
  assert.deepEqual(southExit.groundEnds, ["end"]);
  assert.deepEqual(southExit.groundExitOnly, ["end"]);
  assert.deepEqual(edgeExit.groundEnds, []);
  assert.equal(edgeExit.groundExitOnly, undefined);
  assert.deepEqual(northEntry.groundEnds, ["start"]);
  const [east, north, height] = northEntry.points[0];
  const lead = Math.hypot(east + 1504.46, north + 5537.32);
  assert.ok(lead >= 14 && lead < 18);
  assert.ok(Math.abs(groundAtWorld(tiles, east, north, true) - height) < 0.02);

  const decks = deckClearanceIndex();
  for (const item of roads.values()) {
    if (item.kind === "tunnel") continue;
    const geometry = contactGeometry(
      buildBakedRoadContact(item).surface.contact,
      false,
    );
    decks.add(geometry);
    geometry.dispose();
  }
  const obstacles = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), decks.ceiling),
  );
  const tileByToken = new Map(tiles.map((tile) => [tile.t, tile]));
  for (const item of [
    northFirst,
    northNext,
    northEntry,
    southNext,
    southLast,
    southExit,
    edgeExit,
  ]) {
    const seen = new Set();
    for (const [i, [x, y, z]] of item.points.entries()) {
      const closest = owners.reduce((a, b) =>
        (x - a.x) ** 2 + (y - a.y) ** 2 < (x - b.x) ** 2 + (y - b.y) ** 2
          ? a
          : b,
      );
      seen.add(closest.t);
      if (!i) continue;
      const [a, b, h] = item.points[i - 1];
      const heading = Math.atan2(y - b, x - a);
      assert.ok(Math.abs(z - h) / Math.hypot(x - a, y - b) < 0.08, item.id);
      assert.equal(collisionAt(obstacles, { x, y, heading }, z), null, item.id);
    }
    for (const token of seen) {
      const copy = tileByToken
        .get(token)
        .playableRoads.find(({ id }) => id === item.id);
      assert.deepEqual(copy, item, `${item.id} in ${token}`);
    }
  }
  for (const item of [northFirst, southNext]) {
    const differences = item.points.flatMap(([x, y, z]) => {
      const ground = groundAtWorld(tiles, x, y, true);
      return Number.isFinite(ground) ? [z - ground] : [];
    });
    assert.ok(Math.max(...differences) > 6, item.id);
  }
});

test("San Jose Avenue stays on its measured upper road above the Bosworth ramps", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = JSON.parse(gunzipSync(file("tiles/808f7e64.json.gz")));
  const road = tile.playableRoads.find(
    ({ id }) => id === "sf-contact/san-jose-bosworth-upper-street",
  );
  assert.equal(road.width, 8);
  assert.deepEqual(road.groundEnds, ["start", "end"]);
  assert.ok(
    road.profiles.every((id) => roadBake.playableReplaces.includes(id)),
  );
  const { surface, portals } = buildBakedRoadContact(road);
  assert.equal(portals.length, 2);
  for (const { x, y, z } of portals) {
    assert.ok(Math.abs(groundAtWorld([tile], x, y, true) - z) < 0.02);
  }
  const [x, y] = [664.64, -2965.1];
  assert.ok(groundAtWorld([tile], x, y, true) < 49);
  assert.ok(surface.heightAt(x, y) > 56.9);
  assert.ok(surface.heightAt(x, y) < 57.3);
  for (let i = 1; i < road.points.length; i++) {
    const [a, b, az] = road.points[i - 1];
    const [c, d, bz] = road.points[i];
    assert.ok(Math.abs(bz - az) / Math.hypot(c - a, d - b) < 0.035);
  }
});

test("street turns clear the Diamond and César Chávez forks without entering the ramp", () => {
  for (const [token, id, end] of [
    ["808f7e64", "san-jose-bosworth-upper-street", "end"],
    ["808f7e4c", "us101-south-cesar-chavez-entry", "start"],
  ]) {
    const tile = JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`)));
    const road = tile.playableRoads.find(
      ({ id: roadId }) => roadId === `sf-contact/${id}`,
    );
    const portals = buildBakedRoadContact(road).portals;
    const gate = portals[road.groundEnds.indexOf(end)];
    const fallback = buildBakedRoadContact({
      ...road,
      groundEntryProbeMetres: {},
    });
    const pavement = (x, y) => groundAtWorld([tile], x, y, true);
    const entry = Math.atan2(gate.entryY, gate.entryX);
    const x = gate.x - 0.02 * gate.nx;
    const y = gate.y - 0.02 * gate.ny;
    const cross = (heading, mouths) =>
      driveThroughRoadPortal(
        { x, y, z: pavement(x, y), heading, v: 6, grounded: true },
        { throttle: 1 },
        1 / 120,
        [mouths],
        pavement,
      );

    assert.equal(gate.entryProbe, 12, id);
    assert.equal(cross(entry, portals)?.surfaceId, road.id, id);
    assert.equal(cross(entry + 0.62, fallback.portals)?.surfaceId, road.id, id);
    assert.equal(cross(entry + 0.62, portals), null, id);
  }
});

test("both Bosworth entrances join southbound I-280 below the San Jose Avenue deck", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e64", "808f7e7c", "808f7e8c", "808f7e84"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const contact = (name) => roads.get(`sf-contact/${name}`);
  const bosworth = contact("bosworth-southbound-entry");
  const sanJose = contact("san-jose-southbound-entry");
  const merge = contact("bosworth-i280-southbound-merge");
  const onward = contact("i280-south-monterey-to-ocean");
  for (const [before, after] of [
    [bosworth, merge],
    [sanJose, merge],
    [merge, onward],
  ]) {
    assert.deepEqual(before.points.at(-1), after.points[0]);
    assert.deepEqual(before.seams.end, after.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${before.id}:end` &&
          to === `${after.id}:start` &&
          direction === "forward",
      ),
    );
  }

  for (const road of [bosworth, sanJose]) {
    assert.deepEqual(road.groundEnds, ["start"]);
    for (const tile of tiles.slice(0, 2)) {
      assert.deepEqual(
        tile.playableRoads.find(({ id }) => id === road.id),
        road,
      );
    }
  }
  const x = 569.56;
  const y = -3010.53;
  const upper = buildBakedRoadContact(
    contact("san-jose-bosworth-upper-street"),
  ).surface;
  const lower = buildBakedRoadContact(bosworth).surface;
  assert.ok(lower.heightAt(x, y) > 49.8 && lower.heightAt(x, y) < 50.4);
  assert.ok(upper.heightAt(x, y) - lower.heightAt(x, y) > 8);

  const clearance = deckClearanceIndex();
  for (const road of roads.values()) {
    const { surface } = buildBakedRoadContact(road);
    if (surface.kind !== "tunnel") {
      clearance.add(contactGeometry(surface.contact, false));
    }
  }
  const indexes = tiles.map((tile) =>
    buildCollisionIndex(tile, tile.t, new Map(), clearance.ceiling),
  );
  for (const road of [bosworth, sanJose, merge]) {
    for (let i = 1; i < road.points.length; i++) {
      const [a, b] = road.points[i - 1];
      const [east, north, z] = road.points[i];
      const car = {
        x: east,
        y: north,
        heading: Math.atan2(north - b, east - a),
      };
      assert.equal(
        collisionAt(indexes, car, z),
        null,
        `${road.id} at ${east}, ${north}`,
      );
    }
  }
});

test("Monterey Boulevard and the Branch East exit share one upper deck into San Jose Avenue", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7e64", "808f7e7c"].map((token) =>
    JSON.parse(gunzipSync(file(`tiles/${token}.json.gz`))),
  );
  const roads = new Map(
    tiles.flatMap((tile) =>
      (tile.playableRoads ?? []).map((road) => [road.id, road]),
    ),
  );
  const contact = (name) => roads.get(`sf-contact/${name}`);
  const monterey = contact("monterey-san-jose-approach");
  const exit = contact("branch-east-san-jose-viaduct");
  const bore = contact("branch-east-off-ramp");
  const shared = contact("san-jose-avenue-shared-deck");

  assert.deepEqual(monterey.groundEnds, ["start"]);
  assert.deepEqual(bore.groundEnds, ["start"]);
  assert.deepEqual(exit.groundEnds, []);
  assert.deepEqual(shared.groundEnds, ["end"]);
  assert.deepEqual(bore.points.at(-1), exit.points[0]);
  assert.deepEqual(bore.seams.end, exit.seams.start);
  assert.ok(
    roadBake.playableTransitions.some(
      (link) =>
        link.from === `${bore.id}:end` &&
        link.to === `${exit.id}:start` &&
        link.direction === "forward",
    ),
  );
  for (const [approach, direction] of [
    [monterey, "both"],
    [exit, "forward"],
  ]) {
    assert.deepEqual(approach.points.at(-1), shared.points[0]);
    assert.deepEqual(approach.seams.end, shared.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        (link) =>
          link.from === `${approach.id}:end` &&
          link.to === `${shared.id}:start` &&
          link.direction === direction,
      ),
    );
  }
  for (const tile of tiles) {
    assert.deepEqual(
      tile.playableRoads.find(({ id }) => id === monterey.id),
      monterey,
    );
  }

  const upper = buildBakedRoadContact(monterey).surface;
  for (const [x, y, lowerName, minClearance] of [
    [493.47, -3071.69, "bosworth-southbound-entry", 9.8],
    [552.27, -3090.69, "branch-east-off-ramp", 7.2],
  ]) {
    const lower = buildBakedRoadContact(contact(lowerName)).surface;
    const ground = lower.heightAt(x, y);
    assert.ok(ground > 48 && ground < 51);
    assert.ok(upper.heightAt(x, y) - ground > minClearance);
  }

  const departure = buildBakedRoadContact(shared).portals[0];
  assert.ok(
    Math.abs(
      groundAtWorld(tiles, departure.x, departure.y, true) - departure.z,
    ) < 0.02,
  );
});

test("the outside Dogpatch entrance lane stays elevated across the northbound merge", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tokens = ["808f7fa4", "808f7fac", "808f7fb4", "808f7fcc"];
  const tiles = tokens.map((token) => {
    const raw = file(`tiles/${token}.json.gz`);
    assertRampWidthReceipt(
      roadBake,
      token,
      roadBake.dogpatchNorthMergeSurvey.tiles[token].sha256,
      raw,
    );
    return JSON.parse(gunzipSync(raw));
  });
  const roads = new Map(
    tiles.flatMap(({ playableRoads }) =>
      playableRoads.map((road) => [road.id, road]),
    ),
  );
  const [entry, through, merged] = [
    "i280-dogpatch-east-north-entry",
    "i280-north-south-dogpatch",
    "i280-dogpatch-northbound",
  ].map((slug) => roads.get(`sf-contact/${slug}`));
  for (const approach of [entry, through]) {
    assert.deepEqual(approach.points.at(-1), merged.points[0]);
    assert.deepEqual(approach.seams.end, merged.seams.start);
    assert.ok(
      roadBake.playableTransitions.some(
        ({ from, to, direction }) =>
          from === `${approach.id}:end` &&
          to === `${merged.id}:start` &&
          direction === "forward",
      ),
    );
  }
  assert.deepEqual(entry.groundEnds, ["start"]);
  assert.deepEqual(merged.groundEnds, []);
  const ramp = buildBakedRoadContact(entry).surface;
  const mainline = buildBakedRoadContact(merged).surface;
  const heading = merged.seams.start.bearing;
  const [x, y, height] = merged.points[0];
  const travel = (station, offset) => ({
    x: x + station * Math.cos(heading) + offset * Math.sin(heading),
    y: y + station * Math.sin(heading) - offset * Math.cos(heading),
    heading,
  });
  const car = travel(0, 9.25);
  assert.ok(
    roadSupportsCar(
      (east, north) =>
        ramp.heightAt(east, north) ?? mainline.heightAt(east, north),
      car,
    ),
  );
  assert.ok(height - groundAtWorld(tiles, car.x, car.y, true) > 6);
  const after = travel(20, 8.25);
  assert.ok(roadSupportsCar(mainline.heightAt, after));
  const outside = travel(20, 16);
  assert.equal(mainline.heightAt(outside.x, outside.y), null);
});

test("the surveyed Bryant entry lane stays on the deck and joins eastbound I-80", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tile = (token) => {
    const raw = file(`tiles/${token}.json.gz`);
    assertLaterRoadTileReceipt(
      roadBake,
      token,
      roadBake.bryantEastboundWidthSurvey.tiles[token].sha256,
      raw,
    );
    return JSON.parse(gunzipSync(raw));
  };
  const [south, north] = ["808f7e2c", "80858084"].map(tile);
  const road = (owner, slug) =>
    owner.playableRoads.find(({ id }) => id === `sf-contact/${slug}`);
  const bryant = road(south, "i80-eastbound-bryant-entry");
  const next = road(north, "i80-eastbound-fourth");
  assert.deepEqual(bryant, road(north, "i80-eastbound-bryant-entry"));
  assert.deepEqual(bryant.points.at(-1), next.points[0]);
  assert.deepEqual(bryant.seams.end, next.seams.start);
  const { surface } = buildBakedRoadContact(bryant);
  for (const [x, y, heading] of [
    [3020.3, 1694, 0.93],
    [3085.69, 1769.62, 0.79],
  ]) {
    assert.ok(roadSupportsCar(surface.heightAt, { x, y, heading }));
    assert.ok(
      surface.heightAt(x, y) - groundAtWorld([south, north], x, y, true) > 5,
    );
  }
});

test("the surveyed northbound Sixth Street lane stays above the street across the seam", () => {
  const { roadBake } = JSON.parse(file("tiles/manifest.json"));
  const tiles = ["808f7fcc", "808f7fd4"].map((token) => {
    const raw = file(`tiles/${token}.json.gz`);
    assertLaterRoadTileReceipt(
      roadBake,
      token,
      roadBake.sixthNorthExitWidthSurvey.tiles[token].sha256,
      raw,
    );
    return JSON.parse(gunzipSync(raw));
  });
  const road = (slug) =>
    tiles
      .flatMap(({ playableRoads }) => playableRoads)
      .find(({ id }) => id === `sf-contact/${slug}`);
  const lead = road("i280-mariposa-sixth-approach");
  const exit = road("i280-sixth-northbound");
  assert.deepEqual(lead.points.at(-1), exit.points[0]);
  assert.deepEqual(lead.seams.end, exit.seams.start);
  const { surface: approach } = buildBakedRoadContact(lead);
  const { surface: ramp } = buildBakedRoadContact(exit);
  const pavement = (x, y) => approach.heightAt(x, y) ?? ramp.heightAt(x, y);
  for (const [x, y, heading] of [
    [3826.41, 936.49, 2.2],
    [3817.81, 948.77, 2.13],
    [3809.85, 961.48, 2.1],
  ]) {
    assert.ok(roadSupportsCar(pavement, { x, y, heading }));
    assert.ok(pavement(x, y) - groundAtWorld(tiles, x, y, true) > 7);
  }
});
