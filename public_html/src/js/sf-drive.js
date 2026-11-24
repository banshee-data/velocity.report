// San Francisco driving view (/sf/): drive anywhere in the city over measured
// geometry. Tiles (one per S2 level-13 cell, about 1.1 km across) stream in
// around the car: ground, buildings, trees, roads with kerbs, and street-name
// signposts. Start zoomed out; overhead and in-car views are available in the menu.
import {
  SOLO,
  soloHeight,
  soloPitch,
  soloSurface,
  soloCar,
  contactStep,
  createSoloRun,
  startSoloRun,
  endSoloRun,
  accountSolo,
  soloSample,
  soloSummary,
} from "./sf-solo.js";
import * as THREE from "../vendor/three/three.module.js";
import {
  advanceTrial,
  brakeLightsOn,
  buildCollisionIndex,
  collisionAt,
  createTrial,
  DRIVE_CAR,
  formatBytes,
  formatTime,
  groundAt,
  intersectionsForTile,
  nearestRoad,
  questSeeker,
  nearestQuestTarget,
  tileAt,
  groundAtWorld,
  nextCrossing,
  streetAt,
  tilesAround,
} from "./sf-drive-core.js";

import { drivingHull, obstacleContact } from "./sf-chassis-contact.js";
import {
  beginCrash,
  crashCar,
  seriousImpact,
  closingSpeed,
  scrapeObstacle,
  stepCrash,
  hardLanding,
} from "./sf-crash.js";

import { terrainTriangles, tileRegion } from "./sf-drive-surface.js";
import { createDownloadSizeTracker } from "./sf-download-size.js";
import { bindDrivingPad, drivingInput } from "./sf-touch-controls.js";
import { districtTrials, seekerDirection } from "./sf-district-trials.js";
import {
  DRIVE_VIEWS,
  DRIVE_VIEW_RANGE,
  MIRROR_VIEW_RANGE,
  coveredRoadBlend,
  driveCameraPose,
  incarRoll,
} from "./sf-drive-camera.js";
import { createCarReveal } from "./sf-car-reveal.js";
import { roadDashPolygons } from "./sf-road-markings.js";
import { createTerrainPainter } from "./sf-terrain-painter.js";
import { SCENE_COLOURS } from "./scene-colours.js";
import { mountSurveyReplay } from "./sf-survey-replay.js";
import { surveyBeacons } from "./sf-survey-beacons.js";
import {
  BONUS_RADIUS,
  advanceInsurance,
  gameInsuranceLimit,
  recordCrash,
  createInsurance,
  insuranceRate,
  formatInsuranceCents,
  formatInsuranceDollars,
  visitSurvey,
} from "./sf-insurance.js";
import {
  BRIDGE_SURVEY_DRIVE,
  BRIDGE_SURVEY_NAME,
  bridgeSurveyDrive,
  SOMA_SURVEY_DRIVE,
  SOMA_SURVEY_NAME,
  SOMA_SURVEY_STOPS,
  somaSurveyDrive,
} from "./sf-soma-drive.js";
import { fetchJsonGz, readBundledDrive } from "./sf-drive-assets.js";
import {
  BAY_BRIDGE_APPROACH,
  BAY_BRIDGE_CITY_DRIVE,
  applyCityDriveScenery,
  buildCityGameDrive,
  cityDriveApproach,
  cityDriveControls,
  cityDriveCanResume,
  cityDriveOnRoute,
  driveOnCityRoute,
  buildRaisedRoad,
} from "./sf-city-road-drive.js";
import {
  contactGeometry,
  driveOnRoadSurface,
  indexRoadContact,
} from "./sf-road-contact.js";
import { driveOnRoadNetwork, indexRoadNetwork } from "./sf-road-network.js";
import { buildBakedRoadContact } from "./sf-baked-road-contact.js";
import { driveThroughRoadPortal } from "./sf-road-portal.js";
import { tunnelMesh } from "./sf-tunnel-mesh.js";
import { buildingWalls, validBuildingFace } from "./sf-building-walls.js";
import {
  crashHistory,
  encodeReplay,
  replayLibrary,
  FAVOURITE_LIMIT,
  replayPose,
  REPLAY_POSITION_SCALE,
  CRASH_HOLD_TICKS,
} from "./sf-replay.js";
import {
  advanceReplay,
  replayRate,
  replayLandmark,
  replayTelemetry,
  replayCharts,
} from "./sf-replay-telemetry.js";
import {
  treeRadius,
  treeDamageForImpact,
  treeFallAngle,
  treeFallProgress,
  replayTreeIndex,
} from "./sf-tree-damage.js";
import { createAirtimeHud } from "./sf-airtime.js";
import {
  bestAirtimes,
  readProgress,
  restoreTrial,
  writeProgress,
} from "./sf-progress.js";
import {
  fallFromRoad,
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
  strikeHighwayRail,
  crashRoadSupport,
  indexCrashRails,
} from "./sf-highway-edge.js";

const css = (n) =>
  getComputedStyle(document.documentElement).getPropertyValue(n).trim();

// Downtown, the waterfront and the neighbouring SoMa skyline in the SF base.
const BAY_SKYLINE_TILES = ["80858064", "80858074", "8085807c", "8085808c"];
const col = (n) => new THREE.Color(css(n));

function mapPath(lines, close = false) {
  const path = new Path2D();
  for (const line of lines) {
    line.forEach(([x, y], i) => (i ? path.lineTo(x, y) : path.moveTo(x, y)));
    if (close) path.closePath();
  }
  return path;
}

// World (x east, y north, z up) to three.js (x, y up, -z north).
const V = (x, y, z) => [x, z, -y];

function groundFn(tile) {
  return (lx, ly) =>
    groundAt(
      tile.g,
      Math.max(tile.g.x0, Math.min(tile.g.x0 + (tile.g.n - 1) * tile.g.s, lx)),
      Math.max(tile.g.y0, Math.min(tile.g.y0 + (tile.g.n - 1) * tile.g.s, ly)),
    ) ?? tile.zfill;
}

function roadShapes(tile) {
  return tile.r.map((r) => {
    const p = r[3],
      w = r[4],
      left = [],
      right = [],
      centre = [];
    for (let i = 0; i < p.length; i += 2) {
      const a = Math.max(0, i - 2),
        b = Math.min(p.length - 2, i + 2);
      const dx = p[b] - p[a],
        dy = p[b + 1] - p[a + 1],
        len = Math.hypot(dx, dy) || 1;
      const x = tile.o[0] + p[i] / 10,
        y = tile.o[1] + p[i + 1] / 10;
      const wl = w.length ? w[i] / 10 : 1.2,
        wr = w.length ? w[i + 1] / 10 : 1.2;
      left.push([x - (dy / len) * wl, y + (dx / len) * wl]);
      right.push([x + (dy / len) * wr, y - (dx / len) * wr]);
      centre.push([x, y]);
    }
    return {
      polygon: [...left, ...right.reverse()],
      centre,
      road: !!w.length,
      dashed: !!w.length && r[2] === 0,
    };
  });
}

function paintTerrain(entry, entries, C, rasterMarkings = false) {
  const texture = entry.grp.userData.terrain.material.map;
  const cv = texture.image,
    ctx = cv.getContext("2d"),
    { g, o } = entry.tile;
  ctx.resetTransform();
  ctx.fillStyle = `#${C.land.getHexString()}`;
  ctx.fillRect(0, 0, cv.width, cv.height);
  const scale = cv.width / ((g.n - 1) * g.s);
  ctx.setTransform(
    scale,
    0,
    0,
    -scale,
    -(o[0] + g.x0) * scale,
    (o[1] + g.y0 + (g.n - 1) * g.s) * scale,
  );
  const shapes = entries.flatMap((e) => e.roads);
  const trace = (points) => {
    points.forEach(([x, y], i) => (i ? ctx.lineTo(x, y) : ctx.moveTo(x, y)));
  };
  const path = (points) => {
    ctx.beginPath();
    trace(points);
  };
  // Paths first, then carriageways. Both colours share one depth surface.
  for (const road of [false, true])
    for (const shape of shapes) {
      if (shape.road !== road) continue;
      path(shape.polygon);
      ctx.closePath();
      ctx.fillStyle = `#${(road ? C.asphalt : C.path).getHexString()}`;
      ctx.fill();
    }
  // The baked kerb lines can span many terrain cells. Paint them on the
  // surface so they follow every slope and stay out of missing ground.
  ctx.beginPath();
  for (const source of entries) for (const line of source.kerbs) trace(line);
  ctx.strokeStyle = `#${C.kerb.getHexString()}`;
  ctx.lineWidth = 0.3;
  ctx.lineJoin = "round";
  ctx.stroke();
  if (rasterMarkings) {
    ctx.strokeStyle = `#${C.dash.getHexString()}`;
    ctx.lineWidth = 0.3;
    ctx.setLineDash([3, 3]);
    for (const shape of shapes)
      if (shape.dashed) {
        path(shape.centre);
        ctx.stroke();
      }
    ctx.setLineDash([]);
  }
  texture.needsUpdate = true;
}

function applyRoadPaint(entry, positions, C) {
  // Vector paint stays sharp without increasing the mobile terrain texture.
  // Clip to the very same terrain planes; offset only in depth-buffer space.
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.Float32BufferAttribute(positions, 3),
  );
  let paint = entry.grp.userData.markings;
  if (paint) paint.geometry.dispose();
  else {
    paint = new THREE.Mesh(
      geometry,
      new THREE.MeshBasicMaterial({
        color: C.dash,
        side: THREE.DoubleSide,
        depthWrite: false,
        polygonOffset: true,
        polygonOffsetFactor: -1,
        polygonOffsetUnits: -1,
      }),
    );
    paint.renderOrder = 1;
    entry.grp.userData.markings = paint;
    entry.grp.add(paint);
  }
  paint.geometry = geometry;
}

function appendBuildingWallCorners(outlines, edges) {
  const corners = new Map();
  for (const { a, b, za, zb } of edges) {
    for (const [point, neighbour, floor] of [
      [a, b, za],
      [b, a, zb],
    ]) {
      const key = point.join(",");
      const corner = corners.get(key) ?? { point, neighbours: [], floor };
      corner.neighbours.push(neighbour);
      corner.floor = Math.min(corner.floor, floor);
      corners.set(key, corner);
    }
  }
  for (const { point, neighbours, floor } of corners.values()) {
    // Only draw wall posts at real turns in the eaves. Straight edge
    // subdivisions and the ends of interior roof ridges need no posts.
    if (neighbours.length !== 2) continue;
    const [a, b] = neighbours;
    const ax = a[0] - point[0],
      ay = a[1] - point[1];
    const bx = b[0] - point[0],
      by = b[1] - point[1];
    if (
      Math.abs(ax * by - ay * bx) >
      0.5 * Math.hypot(ax, ay) * Math.hypot(bx, by)
    )
      outlines.push(...V(...point), ...V(point[0], point[1], floor));
  }
}

function addBuildings(group, tile, C, reveal, distant = false) {
  const [ox, oy] = tile.o;
  // Keep each building whole, but batch nearby buildings so the camera
  // rejects distant blocks before submitting their triangles and outlines.
  const batchSize = 1600; // decimetres; tile coordinates are local
  const batches = new Map();
  for (const b of tile.b) {
    const validFaces = b[1].filter(validBuildingFace);
    const firstRing = validFaces[0]?.[0];
    if (!firstRing?.length) continue;
    if (
      distant &&
      !validFaces.some((face) =>
        face.some((ring) =>
          ring.some((z, index) => index % 3 === 2 && z - b[0] >= 420),
        ),
      )
    )
      continue;
    const key = `${Math.floor(firstRing[0] / batchSize)},${Math.floor(firstRing[1] / batchSize)}`;
    let batch = batches.get(key);
    if (!batch) {
      batch = { positions: [], colours: [], outlines: [] };
      batches.set(key, batch);
    }
    const { positions, colours, outlines } = batch;
    const zb = b[0] / 10;
    const roof = b[2] ? C.extra : C.roof;
    const wall = b[2] ? C.extra : C.wall;
    const roofEdges = new Map();
    const faces = validFaces.map((face) =>
      face.map((r) => {
        const a = [];
        for (let i = 0; i < r.length; i += 3)
          a.push([ox + r[i] / 10, oy + r[i + 1] / 10, r[i + 2] / 10]);
        // Drop a closing point that repeats the first: the triangulator drops it
        // too, and its indices must address the same vertices as `all` below.
        const f = a[0];
        const l = a.at(-1);
        if (a.length > 3 && f[0] === l[0] && f[1] === l[1]) a.pop();
        return a;
      }),
    );
    for (const rings of faces) {
      const outer = rings[0].map((v) => new THREE.Vector2(v[0], v[1]));
      const holes = rings
        .slice(1)
        .map((h) => h.map((v) => new THREE.Vector2(v[0], v[1])));
      let tris = [];
      try {
        tris = THREE.ShapeUtils.triangulateShape(outer, holes);
      } catch {
        tris = [];
      }
      const all = [].concat(...rings);
      for (const t of tris) {
        for (const k of t) {
          positions.push(...V(all[k][0], all[k][1], all[k][2]));
          colours.push(roof.r, roof.g, roof.b);
        }
      }
      if (distant) continue;
      for (const r of rings) {
        for (let i = 0; i < r.length; i++) {
          const a = r[i];
          const c = r[(i + 1) % r.length];
          const edge = [a.join(","), c.join(",")].sort().join("|");
          roofEdges.set(edge, [a, c]);
        }
      }
    }
    for (const [a, b] of roofEdges.values())
      outlines.push(...V(...a), ...V(...b));
    const exposedWalls = [];
    buildingWalls(faces, zb, (a, b, za, zb) => {
      const bottomA = [a[0], a[1], za];
      const bottomB = [b[0], b[1], zb];
      for (const point of [bottomA, bottomB, b, bottomA, b, a]) {
        positions.push(...V(...point));
        colours.push(wall.r, wall.g, wall.b);
      }
      if (!distant) exposedWalls.push({ a, b, za, zb });
    });
    if (!distant) appendBuildingWallCorners(outlines, exposedWalls);
  }
  if (!batches.size) return;
  const facesMaterial = distant
    ? new THREE.MeshBasicMaterial({
        vertexColors: true,
        side: THREE.DoubleSide,
        transparent: true,
        forceSinglePass: true,
      })
    : new THREE.MeshLambertMaterial({
        vertexColors: true,
        side: THREE.DoubleSide,
        transparent: true,
        opacity: 0.28,
        depthWrite: false,
        forceSinglePass: true,
      });
  const outlineMaterial = distant
    ? null
    : new THREE.LineBasicMaterial({
        color: SCENE_COLOURS.buildingOutline,
        opacity: 0.36,
        transparent: true,
        depthWrite: false,
      });
  if (!distant) {
    reveal.enableOn(facesMaterial);
    reveal.enableOn(outlineMaterial);
  }
  for (const { positions, colours, outlines } of batches.values()) {
    if (!positions.length) continue;
    const faces = new THREE.BufferGeometry();
    faces.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(positions, 3),
    );
    faces.setAttribute("color", new THREE.Float32BufferAttribute(colours, 3));
    if (!distant) faces.computeVertexNormals();
    const buildings = new THREE.Mesh(faces, facesMaterial);
    buildings.renderOrder = -2;
    if (distant) {
      group.add(buildings);
      continue;
    }
    const edges = new THREE.BufferGeometry();
    edges.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(outlines, 3),
    );
    const lines = new THREE.LineSegments(edges, outlineMaterial);
    lines.renderOrder = -1;
    group.add(buildings, lines);
  }
  return facesMaterial;
}

function buildTile(tile, C, region, reveal) {
  const ox = tile.o[0];
  const oy = tile.o[1];
  const gz = groundFn(tile);
  const grp = new THREE.Group();
  // ground
  const g = tile.g;
  const vals = g.z.filter((v) => v !== -32768);
  tile.zfill = vals.length
    ? vals.reduce((a, b) => a + b, 0) / vals.length / 10
    : 0;
  const surface = terrainTriangles(tile, region);
  const gg = new THREE.BufferGeometry();
  gg.setAttribute(
    "position",
    new THREE.Float32BufferAttribute(surface.positions, 3),
  );
  gg.setAttribute("uv", new THREE.Float32BufferAttribute(surface.uv, 2));
  gg.computeVertexNormals();
  const cv = document.createElement("canvas");
  cv.width = cv.height = matchMedia("(pointer: coarse)").matches ? 1024 : 2048;
  const ctx = cv.getContext("2d");
  ctx.fillStyle = `#${C.land.getHexString()}`;
  ctx.fillRect(0, 0, cv.width, cv.height);
  const texture = new THREE.CanvasTexture(cv);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.anisotropy = 8;
  const terrain = new THREE.Mesh(
    gg,
    new THREE.MeshLambertMaterial({ map: texture }),
  );
  grp.userData.terrain = terrain;
  grp.add(terrain);

  addBuildings(grp, tile, C, reveal);

  // trees: crown and trunk, instanced
  const nt = tile.tr.length;
  if (nt) {
    const crown = new THREE.InstancedMesh(
      new THREE.IcosahedronGeometry(1, 1),
      new THREE.MeshLambertMaterial({ color: C.tree }),
      nt,
    );
    const trunk = new THREE.InstancedMesh(
      new THREE.CylinderGeometry(1, 1, 1, 6),
      new THREE.MeshLambertMaterial({ color: C.trunk }),
      nt,
    );
    reveal.enableOn(crown.material);
    reveal.enableOn(trunk.material);
    const mtx = new THREE.Matrix4();
    const qn = new THREE.Quaternion();
    const s = new THREE.Vector3();
    const p = new THREE.Vector3();
    const axis = new THREE.Vector3();
    const prior = new THREE.Quaternion();
    const base = new THREE.Vector3();
    function setTreePose(i, event, elapsed = 1) {
      const [x, y, h, r] = tile.tr[i];
      base.set(...V(ox + x / 10, oy + y / 10, gz(x / 10, y / 10)));
      qn.identity();
      if (event) {
        // Rotate the trunk and canopy about the same ground anchor, away from impact.
        axis.set(-event.north, 0, -event.east).normalize();
        if (event.priorLean) {
          qn.setFromAxisAngle(axis, treeFallAngle(event, 1));
          const [east, north] = event.priorLean;
          axis.set(-north, 0, -east).normalize();
          prior.setFromAxisAngle(axis, treeFallAngle({ stage: "cracked" }, 1));
          qn.copy(prior.slerp(qn, treeFallProgress(elapsed)));
        } else {
          qn.setFromAxisAngle(axis, treeFallAngle(event, elapsed));
        }
      }
      const ch = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
      p.set(0, h - ch / 2, 0)
        .applyQuaternion(qn)
        .add(base);
      s.set(r, ch / 2, r);
      mtx.compose(p, qn, s);
      crown.setMatrixAt(i, mtx);
      const tw = treeRadius(h);
      p.set(0, (h - ch) / 2, 0)
        .applyQuaternion(qn)
        .add(base);
      s.set(tw, Math.max(0.5, h - ch), tw);
      mtx.compose(p, qn, s);
      trunk.setMatrixAt(i, mtx);
    }
    tile.tr.forEach((_, i) => setTreePose(i));
    grp.userData.drawTree = (i, event, seconds) => {
      if (!tile.tr[i]) return;
      setTreePose(i, event, seconds);
      crown.instanceMatrix.needsUpdate = true;
      trunk.instanceMatrix.needsUpdate = true;
    };
    grp.add(crown, trunk);
  }

  // signposts: a pole on the corner, with a blade per street lined up with that street
  if (tile.s.length) grp.add(buildSigns(tile, C, ox, oy, gz));
  return grp;
}

// Street-name blades as on San Francisco poles: a green blade per street, white
// letters on both faces, the lower blade on the first street and the upper on
// the second. One texture atlas and one mesh per tile, so a tile of several
// hundred posts costs three draw calls.
const BLADE_H = 0.3; // m; a little larger than the city's 0.2 m so it reads from the car
const POLE_H = 3.3;
function buildSigns(tile, C, ox, oy, gz) {
  const ids = [];
  for (const sg of tile.s)
    for (let i = 2; i < sg.length; i += 2)
      if (!ids.includes(sg[i]) && tile.n[sg[i]]) ids.push(sg[i]);
  // shelf-pack the names into one canvas
  const font = "600 34px system-ui, -apple-system, Segoe UI, sans-serif";
  const rowH = 48;
  const atlasW = 1024;
  const probe = document.createElement("canvas").getContext("2d");
  probe.font = font;
  const cells = new Map();
  let x = 0;
  let y = 0;
  for (const id of ids) {
    const w = Math.min(
      atlasW,
      Math.ceil(probe.measureText(tile.n[id]).width + 28),
    );
    if (x + w > atlasW) {
      x = 0;
      y += rowH;
    }
    cells.set(id, { x, y, w });
    x += w;
  }
  const atlasH = y + rowH;
  const cv = document.createElement("canvas");
  cv.width = atlasW;
  cv.height = atlasH;
  const ctx = cv.getContext("2d");
  ctx.font = font;
  ctx.textBaseline = "middle";
  for (const [id, c] of cells) {
    ctx.fillStyle = `#${C.sign.getHexString()}`;
    ctx.fillRect(c.x, c.y, c.w, rowH);
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth = 2;
    ctx.strokeRect(c.x + 3, c.y + 3, c.w - 6, rowH - 6);
    ctx.fillStyle = "#ffffff";
    ctx.fillText(tile.n[id], c.x + 14, c.y + rowH / 2 + 1);
  }
  const tex = new THREE.CanvasTexture(cv);
  tex.colorSpace = THREE.SRGBColorSpace;
  tex.anisotropy = 4;

  const P = [];
  const UV = [];
  const quad = (a, b, z0, z1, u0, u1, v0, v1) => {
    // a and b are the blade's two ends; u0 at a, u1 at b
    const c = [
      [a, z0, u0, v0],
      [b, z0, u1, v0],
      [b, z1, u1, v1],
      [a, z0, u0, v0],
      [b, z1, u1, v1],
      [a, z1, u0, v1],
    ];
    for (const [p, z, u, v] of c) {
      P.push(...V(p[0], p[1], z));
      UV.push(u, v);
    }
  };
  const poles = [];
  for (const sg of tile.s) {
    const lx = sg[0] / 10;
    const ly = sg[1] / 10;
    const z = gz(lx, ly) ?? tile.zfill;
    const px = ox + lx;
    const py = oy + ly;
    poles.push([px, py, z]);
    let level = 0;
    for (let i = 2; i + 1 < sg.length; i += 2) {
      const c = cells.get(sg[i]);
      if (!c) continue;
      const a = (sg[i + 1] * Math.PI) / 180;
      const ux = Math.cos(a);
      const uy = Math.sin(a);
      const half = (BLADE_H * c.w) / rowH / 2;
      const nx = -uy * 0.012;
      const ny = ux * 0.012;
      const z0 = z + POLE_H - 0.55 + level * (BLADE_H + 0.04);
      const z1 = z0 + BLADE_H;
      const u0 = c.x / atlasW;
      const u1 = (c.x + c.w) / atlasW;
      const v1 = 1 - c.y / atlasH;
      const v0 = 1 - (c.y + rowH) / atlasH;
      const e1 = [px + ux * half, py + uy * half];
      const e2 = [px - ux * half, py - uy * half];
      // seen from the +n side the words run toward -u; from the -n side toward +u
      quad(
        [e1[0] + nx, e1[1] + ny],
        [e2[0] + nx, e2[1] + ny],
        z0,
        z1,
        u0,
        u1,
        v0,
        v1,
      );
      quad(
        [e2[0] - nx, e2[1] - ny],
        [e1[0] - nx, e1[1] - ny],
        z0,
        z1,
        u0,
        u1,
        v0,
        v1,
      );
      level += 1;
    }
  }
  const out = new THREE.Group();
  const geo = new THREE.BufferGeometry();
  geo.setAttribute("position", new THREE.Float32BufferAttribute(P, 3));
  geo.setAttribute("uv", new THREE.Float32BufferAttribute(UV, 2));
  const blades = new THREE.Mesh(
    geo,
    new THREE.MeshBasicMaterial({ map: tex, side: THREE.DoubleSide }),
  );
  out.add(blades);
  const pole = new THREE.InstancedMesh(
    new THREE.CylinderGeometry(0.045, 0.05, POLE_H, 8),
    new THREE.MeshLambertMaterial({ color: C.pole }),
    poles.length,
  );
  const m = new THREE.Matrix4();
  poles.forEach(([px, py, z], i) => {
    m.makeTranslation(...V(px, py, z + POLE_H / 2));
    pole.setMatrixAt(i, m);
  });
  out.add(pole);
  return out;
}

function makeCar() {
  const car = new THREE.Group();
  const body = new THREE.Mesh(
    new THREE.BoxGeometry(4.4, 0.9, 1.85),
    new THREE.MeshLambertMaterial({ color: SCENE_COLOURS.vehicleShade }),
  );
  body.position.y = 0.7;
  const cab = new THREE.Mesh(
    new THREE.BoxGeometry(2.2, 0.7, 1.65),
    new THREE.MeshLambertMaterial({ color: SCENE_COLOURS.vehicleShade }),
  );
  cab.position.set(-0.3, 1.45, 0);
  const outline = new THREE.LineBasicMaterial({ color: SCENE_COLOURS.vehicle });
  for (const part of [body, cab])
    part.add(
      new THREE.LineSegments(new THREE.EdgesGeometry(part.geometry), outline),
    );
  car.add(body, cab);
  // Rear lamps face the chase camera; a high centre lamp sits on the cabin.
  const brakeMaterial = new THREE.MeshBasicMaterial({ color: 0x661018 });
  for (const z of [-0.62, 0.62]) {
    const lamp = new THREE.Mesh(
      new THREE.BoxGeometry(0.07, 0.24, 0.48),
      brakeMaterial,
    );
    lamp.position.set(-2.23, 0.83, z);
    car.add(lamp);
  }
  const centreLamp = new THREE.Mesh(
    new THREE.BoxGeometry(0.04, 0.1, 0.56),
    brakeMaterial,
  );
  centreLamp.position.set(-1.42, 1.6, 0);
  car.add(centreLamp);
  car.userData.brakeMaterial = brakeMaterial;
  const wheels = [];
  const tyreGeometry = new THREE.CylinderGeometry(0.36, 0.36, 0.22, 12);
  tyreGeometry.rotateX(Math.PI / 2);
  const tyreMaterial = new THREE.MeshLambertMaterial({ color: 0x20262a });
  for (const x of [-1.35, 1.35])
    for (const z of [-0.91, 0.91]) {
      const pivot = new THREE.Group();
      pivot.position.set(x, 0.36, z);
      const tyre = new THREE.Mesh(tyreGeometry, tyreMaterial);
      pivot.add(tyre);
      car.add(pivot);
      wheels.push({ pivot, tyre, front: x > 0 });
    }
  car.userData.wheels = wheels;
  return car;
}

export async function startDrive({
  canvas,
  base,
  hud,
  downloadAssets,
  tileCacheName,
  surveySites = [],
}) {
  const params = new URLSearchParams(location.search);
  const at = params.get("at");
  const tileBase = base;
  const [manifest, route, coverage] = await Promise.all([
    fetchJsonGz(`${base}manifest.json`),
    fetchJsonGz(`${base}../quest-route.json`),
    fetchJsonGz(`${base}../coverage.json`),
  ]);
  const bridgeApproaches = manifest.roadBake?.driveApproaches || [];
  const cityMode =
    params.get("city") ||
    (bridgeApproaches.some(({ id }) => id === at)
      ? "bay-bridge"
      : !at &&
          !params.has("district") &&
          !params.has("region") &&
          params.get("mode") !== "solo"
        ? "survey"
        : null);
  let bakedDrive;
  if (cityMode === "bay-bridge" || cityMode === "survey") {
    const bridge = await readBundledDrive(
      new URL(`${base}../gamedrives.json`, location.href),
      manifest.roadBake,
      BAY_BRIDGE_CITY_DRIVE,
    );
    bakedDrive =
      cityMode === "survey"
        ? bridgeSurveyDrive(bridge)
        : cityDriveApproach(bridge, at);
  }
  const coverageCorners = Object.values(coverage.cells).flat();
  const cellPaths = new Map(
    Object.entries(coverage.cells).map(([t, ring]) => [
      t,
      mapPath([ring], true),
    ]),
  );
  const landPath = mapPath(coverage.coast.flat(), true);
  const parkPath = mapPath(coverage.parks.flat(), true);
  const arterialPath = mapPath(coverage.arterials);
  const downloads = createDownloadSizeTracker({
    tiles: manifest.tiles,
    assets: downloadAssets,
    tileBase: new URL(tileBase, location.href),
    assetBase: new URL(base, location.href),
  });
  let districts = [];
  try {
    districts = districtTrials(route, manifest);
  } catch (e) {
    console.warn("District trials unavailable", e);
  }
  const activeDistrict =
    !params.has("city") && !params.has("region")
      ? districts.find((d) => d.id === params.get("district"))
      : null;
  const C = {
    sky: col("--sky"),
    land: col("--land"),
    roof: new THREE.Color(SCENE_COLOURS.buildingRoof),
    wall: new THREE.Color(SCENE_COLOURS.buildingWall),
    extra: new THREE.Color(SCENE_COLOURS.buildingInferred),
    asphalt: new THREE.Color(SCENE_COLOURS.asphalt),
    path: new THREE.Color(SCENE_COLOURS.path),
    dash: col("--dash"),
    kerb: new THREE.Color(SCENE_COLOURS.kerb),
    tree: new THREE.Color(SCENE_COLOURS.treeCrown),
    trunk: new THREE.Color(SCENE_COLOURS.treeTrunk),
    pole: col("--pole"),
    sign: col("--sign"),
  };
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(2, window.devicePixelRatio || 1));
  const scene = new THREE.Scene();
  scene.background = C.sky;
  const driveFog = new THREE.Fog(
    C.sky,
    DRIVE_VIEW_RANGE.fogNear,
    DRIVE_VIEW_RANGE.fogFar,
  );
  const mirrorFog = new THREE.Fog(
    C.sky,
    MIRROR_VIEW_RANGE.fogNear,
    MIRROR_VIEW_RANGE.fogFar,
  );
  scene.fog = driveFog;
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8a8f80, 1.9));
  const sun = new THREE.DirectionalLight(0xffffff, 1.7);
  sun.position.set(-0.4, 1, 0.3);
  scene.add(sun);
  const camera = new THREE.PerspectiveCamera(
    62,
    1,
    DRIVE_VIEW_RANGE.near,
    DRIVE_VIEW_RANGE.far,
  );
  const mirrorCam = new THREE.PerspectiveCamera(
    48,
    3.2,
    MIRROR_VIEW_RANGE.near,
    MIRROR_VIEW_RANGE.far,
  );
  const reveal = createCarReveal();
  const carMesh = makeCar();
  scene.add(carMesh);
  const airtimeKey = "sf-best-airtimes-v1";
  let airtimeRecords = bestAirtimes(readProgress(airtimeKey));
  function showAirtimeRecords() {
    hud.airtimeRecords.replaceChildren(
      ...airtimeRecords.map((entry) => {
        const row = document.createElement("li");
        row.textContent = `${entry.seconds.toFixed(2)} s · ${entry.peak.toFixed(1)} m clearance${entry.ending === "impact" ? " · Impact ending" : ""}`;
        return row;
      }),
    );
    if (!airtimeRecords.length) {
      const empty = document.createElement("li");
      empty.textContent = "No landings yet";
      hud.airtimeRecords.append(empty);
    }
  }
  showAirtimeRecords();
  const airtime = createAirtimeHud(
    hud.airtime,
    hud.airtimeCard,
    (landing, poses, takeoff) => {
      airtimeRecords = bestAirtimes(airtimeRecords, landing);
      writeProgress(airtimeKey, airtimeRecords);
      showAirtimeRecords();
      if (poses?.length > 1)
        saveReplay({
          ...encodeReplay(poses, null, CRASH_HOLD_TICKS),
          takeoff,
          touchdown: poses.length - 1,
          ...landing,
          telemetry: replayTelemetry(poses),
        });
    },
  );

  const roadSurfaces = new Map();
  const roadMeshes = new THREE.Group();
  scene.add(roadMeshes);
  const selectedDrive =
    bakedDrive || (cityMode === "soma" ? somaSurveyDrive() : null);
  const gameDrive =
    selectedDrive &&
    buildCityGameDrive(selectedDrive, (x, y) =>
      groundAtWorld(
        [...loaded.values()].map((entry) => entry.tile),
        x,
        y,
        true,
      ),
    );
  const skylineScene = new THREE.Scene();
  skylineScene.background = C.sky;
  skylineScene.fog = new THREE.Fog(C.sky, 1200, 5800);
  const skylineBuildings = new THREE.Group();
  const beacons = surveyBeacons(surveySites, camera);
  skylineScene.add(skylineBuildings);
  const beaconScene = new THREE.Scene();
  const beaconCamera = camera.clone();
  beaconCamera.far = 6500;
  beaconCamera.updateProjectionMatrix();
  beaconScene.add(beacons);
  const skylineCamera = camera.clone();
  skylineCamera.near = 30;
  skylineCamera.far = 6000;
  skylineCamera.updateProjectionMatrix();
  const skylineTiles = new Map();
  const skylineRequests = new Map();
  const shoreline =
    gameDrive?.points[gameDrive.spans.get("bay-bridge-westbound")?.end];
  const network = gameDrive && indexRoadNetwork(gameDrive.topology.transitions);
  const bakedTransitions = manifest.roadBake?.playableTransitions || [];
  const bakedNetwork = indexRoadNetwork(bakedTransitions);
  bakedNetwork.add({ endpoints: [], transitions: bakedTransitions });
  const bakedRoads = new Map();
  const bakedPortals = new Map();
  const replacedPreviews = new Set([
    ...(gameDrive?.references || []),
    ...(manifest.roadBake?.playableReplaces || []),
  ]);
  const raisedRoads = new Map();
  const roadMaterial = new THREE.MeshLambertMaterial({
    color: C.asphalt,
    side: THREE.DoubleSide,
    polygonOffset: true,
    polygonOffsetFactor: -1,
    polygonOffsetUnits: -1,
  });
  const deckMaterial = roadMaterial.clone();
  deckMaterial.color.lerp(C.kerb, 0.33);
  function addGuardRails(surface, mesh) {
    surface.railSegments = highwayRailSegments(surface);
    if (!surface.railSegments.length) return;
    const vertices = surface.railSegments.flatMap(({ a, b }) => [
      ...V(...a),
      ...V(...b),
      ...V(a[0], a[1], a[2] + 0.65),
      ...V(...b),
      ...V(b[0], b[1], b[2] + 0.65),
      ...V(a[0], a[1], a[2] + 0.65),
    ]);
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(vertices, 3),
    );
    geometry.computeVertexNormals();
    mesh.add(
      new THREE.Mesh(
        geometry,
        new THREE.MeshLambertMaterial({
          color: 0xadb5ba,
          side: THREE.DoubleSide,
        }),
      ),
    );
  }

  function addRoadTile(tile) {
    for (const surface of tile.surfaces) {
      if (!surface.contact || roadSurfaces.has(surface.id)) continue;
      const driveSurface = {
        ...surface,
        heightAt: surface.heightAt || indexRoadContact(surface.contact),
        boundary:
          surface.kind === "ground" ? [] : roadBoundary(surface.contact),
      };
      roadSurfaces.set(surface.id, driveSurface);
      if (!gameDrive || surface.kind !== "ground")
        roadMeshes.add(
          new THREE.Mesh(
            contactGeometry(surface.contact, false),
            surface.kind === "ground" || surface.kind === "tunnel"
              ? roadMaterial
              : deckMaterial,
          ),
        );
      if (surface.appearance)
        roadMeshes.add(tunnelMesh(surface, surface.appearance));
      if (gameDrive) {
        const points = surface.profile.flatMap(({ position: [x, y, z] }) =>
          V(x, y, z + 0.06),
        );
        const line = new THREE.BufferGeometry();
        line.setAttribute(
          "position",
          new THREE.Float32BufferAttribute(points, 3),
        );
        roadMeshes.add(
          new THREE.Line(
            line,
            new THREE.LineBasicMaterial({ color: 0xffd072 }),
          ),
        );
      }
      addGuardRails(driveSurface, roadMeshes);
    }
    network?.add(tile);
  }
  if (gameDrive) addRoadTile(gameDrive.topology);
  const places = [...(manifest.places || [])];
  if (gameDrive?.exitStart) places.unshift(gameDrive.exitStart);
  if (gameDrive) places.unshift(gameDrive.start);
  const options = [
    { id: SOMA_SURVEY_DRIVE, name: SOMA_SURVEY_NAME, city: "soma" },
    ...(manifest.roadBake
      ? [
          { id: BRIDGE_SURVEY_DRIVE, name: BRIDGE_SURVEY_NAME, city: "survey" },
          {
            id: BAY_BRIDGE_CITY_DRIVE,
            name: BAY_BRIDGE_APPROACH,
            city: "bay-bridge",
          },
          ...bridgeApproaches.map(({ id, label }) => ({
            id,
            name: label,
            city: "bay-bridge",
          })),
        ]
      : []),
  ];
  const startChoices = [
    ...places,
    ...options.filter(({ id }) => id !== gameDrive?.start.id),
  ];
  const start =
    (at && at === gameDrive?.exitStart?.id ? gameDrive.exitStart : null) ||
    gameDrive?.start ||
    activeDistrict?.start ||
    places.find((p) => p.id === at) ||
    places[0];
  const startingCar = ({ x, y, heading, surfaceId, initialSpeed = 0 }) => ({
    x,
    y,
    heading,
    surfaceId,
    v: initialSpeed,
  });
  let car = startingCar(start);
  let carZ = 0;
  // The car waits until the ground under it has loaded, so it never starts in empty space.
  let grounded = false;
  const loaded = new Map(); // token -> {tile, grp}
  const treeDamage = new Map(); // token -> damage by baked tree row
  let completedCityDrive = false;
  let routeCheckpoint = null;
  let resumeRecovery = false;
  const cityRoadOwners = new Set();
  const loading = new Set();
  let surveyTerrain = [];
  const surveyReplay = mountSurveyReplay({
    sites: surveySites,
    root: new URL("../../", new URL(base, location.href)),
    scene,
    hud: hud.survey,
    groundAt: (x, y) => groundAtWorld(surveyTerrain, x, y, true),
  });
  const byToken = new Map(manifest.tiles.map((t) => [t.t, t]));
  const regions = new Map(
    manifest.tiles.map((t) => [t.t, tileRegion(t, manifest.tiles)]),
  );
  let solo = false;
  let soloRun = createSoloRun();
  let soloHudAt = 0;
  const practice = new THREE.Group();
  practice.visible = false;
  scene.add(practice);
  // Explicitly authored surfaces, isolated from the unchanged city assets.
  for (const [left, right, colour, lift] of [
    [-40, 40, 0x6b8152, -0.02],
    [-6, 6, 0x40454a, 0],
    [-6.15, -6, 0xd4d4cf, 0.15],
    [6, 6.15, 0xd4d4cf, 0.15],
  ]) {
    const positions = [];
    for (let x = SOLO.start - 20; x < SOLO.end + 40; x++) {
      const points = [
        [x, left],
        [x + 1, left],
        [x, right],
        [x + 1, right],
      ];
      for (const i of [0, 1, 2, 1, 3, 2])
        positions.push(
          ...V(points[i][0], points[i][1], soloHeight(points[i][0]) + lift),
        );
    }
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(positions, 3),
    );
    practice.add(
      new THREE.Mesh(
        geometry,
        new THREE.MeshBasicMaterial({ color: colour, side: THREE.DoubleSide }),
      ),
    );
  }
  for (const x of [SOLO.start, 100, SOLO.end]) {
    const geometry = new THREE.BoxGeometry(0.5, 0.03, 12);
    const marker = new THREE.Mesh(
      geometry,
      new THREE.MeshBasicMaterial({ color: x === 100 ? 0xfbbf24 : 0xffffff }),
    );
    marker.position.set(...V(x, 0, soloHeight(x) + 0.02));
    practice.add(marker);
  }
  let trial = createTrial(activeDistrict?.targets || route.targets);
  let selectedStart = start;
  const deliveryDrive = () =>
    [SOMA_SURVEY_DRIVE, BRIDGE_SURVEY_DRIVE].includes(selectedStart.id);
  const insuranceFor = (point) =>
    createInsurance(
      gameDrive?.spans.has(point.surfaceId) === true,
      [SOMA_SURVEY_DRIVE, BRIDGE_SURVEY_DRIVE].includes(point.id)
        ? SOMA_SURVEY_STOPS
        : [],
    );
  let insurance = insuranceFor(start);
  let emojiTarget = null;
  let emojiLocked = false;
  const emojiScreen = new THREE.Vector3();
  let checkpoint = null;
  let crash = null;
  let playback = null;
  let replayId = 0;
  const library = replayLibrary();
  const leadIn = crashHistory();
  const poseEuler = new THREE.Euler();
  const poseRotation = new THREE.Quaternion();
  const replayMode = () => (solo ? "practice" : gameDrive ? cityMode : "city");
  const replayDay = new Intl.DateTimeFormat(undefined, {
    dateStyle: "short",
    timeStyle: "medium",
  });
  let replaySaved = true;
  let crashUntil = 0;
  let accumulator = 0;
  let messageUntil = 0;
  const failures = new Map();
  window.addEventListener("online", () => failures.clear());
  const baseRevision =
    manifest.terrainRebake?.progressBaseSha256 ||
    manifest.roadBake?.baseSha256 ||
    route.manifestSha256;
  const placementRevision = route.placementSha256
    ? `:${route.placementSha256}`
    : "";
  const recordKey = () =>
    `sf-trial-v2:${baseRevision}${placementRevision}:${selectedStart.id}${activeDistrict ? `:district:${activeDistrict.recordScope}` : ""}`;
  const progressKey = () => `sf-progress-v1:${recordKey()}`;
  function loadTrialProgress() {
    const saved = readProgress(progressKey());
    const restored = restoreTrial(trial, saved);
    if (restored) car = startingCar(restored);
    return restored;
  }
  function saveTrialProgress() {
    if (isRoadReview(selectedStart) || solo || trial.status === "ready") return;
    writeProgress(progressKey(), {
      collected: [...trial.collected],
      elapsed: trial.elapsed,
      crashes: trial.crashes,
      checkpoint,
    });
  }
  const recordsByStart = new Map();
  function records() {
    if (!recordsByStart.has(recordKey())) {
      let saved = [];
      try {
        saved = JSON.parse(localStorage.getItem(recordKey()) || "[]");
      } catch {
        /* Storage can be disabled. */
      }
      recordsByStart.set(
        recordKey(),
        Array.isArray(saved)
          ? saved
              .filter((r) => r && Number.isFinite(r.seconds) && r.seconds > 0)
              .slice(0, 5)
          : [],
      );
    }
    return recordsByStart.get(recordKey());
  }
  function announce(text, seconds = 3) {
    hud.status.textContent = text;
    messageUntil = performance.now() + seconds * 1000;
  }
  const isRoadReview = (point) => !!(point.surfaceId || point.rampApproach);
  if (!isRoadReview(selectedStart)) checkpoint = loadTrialProgress();
  window.addEventListener("pagehide", saveTrialProgress);
  function updateBridgeMode() {
    const active = isRoadReview(selectedStart);
    document.body.dataset.bridge = String(active);
    updateDriveMode();
    hud.restartBtn.hidden = !active && trial.status !== "finished";
    hud.restartBtn.textContent = active
      ? "Restart drive"
      : "Start a new emoji trial";
    if (active) {
      hud.timer.textContent = deliveryDrive() ? "Delivery run" : "Game route";
      hud.progress.textContent = deliveryDrive() ? "" : "Authored connections";
    }
  }
  function updateInsuranceMeter() {
    hud.insuranceCost.textContent = formatInsuranceDollars(insurance.spent);
    hud.insuranceLimit.textContent =
      insurance.mode === "manual"
        ? insurance.limit == null
          ? "Limit unavailable"
          : `${insurance.limit} mph game limit · ${insurance.multiplier}×`
        : "";
    hud.insuranceRate.textContent =
      insurance.mode === "manual"
        ? `${formatInsuranceCents(insuranceRate(insurance))}/s · ${insurance.manualSeconds.toFixed(1)} s`
        : "Meter off";
  }
  function updateDriveMode() {
    hud.followCityDrive.hidden = !gameDrive?.spans.has(selectedStart.surfaceId);
    hud.followCityDrive.textContent = completedCityDrive
      ? "Restart self-driving"
      : insurance.mode === "automatic"
        ? "Take manual control"
        : "Resume self-driving";
    hud.insuranceHud.dataset.mode = insurance.mode;
    hud.insuranceMeter.hidden = insurance.mode !== "manual";
    hud.selfDrivingIcon.hidden = insurance.mode !== "automatic";
    updateInsuranceMeter();
    hud.insuranceMode.textContent =
      insurance.mode === "automatic"
        ? "Self-driving"
        : insurance.mode === "manual"
          ? "Meat mode"
          : "Manual ready";
  }
  updateBridgeMode();

  function markerFor(target) {
    const cv = document.createElement("canvas");
    cv.width = cv.height = 128;
    const ctx = cv.getContext("2d");
    ctx.font = "84px system-ui";
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(target.emoji, 64, 68);
    const tex = new THREE.CanvasTexture(cv);
    tex.colorSpace = THREE.SRGBColorSpace;
    const sprite = new THREE.Sprite(
      new THREE.SpriteMaterial({ map: tex, depthTest: true }),
    );
    sprite.scale.set(6, 6, 1);
    const z =
      groundAtWorld(
        [...loaded.values()].map((e) => e.tile),
        target.x,
        target.y,
      ) || 0;
    sprite.position.set(...V(target.x, target.y, z + 6));
    const ring = new THREE.Mesh(
      new THREE.TorusGeometry(4.5, 0.18, 6, 40),
      new THREE.MeshBasicMaterial({ color: 0xffc857 }),
    );
    ring.rotation.x = Math.PI / 2;
    ring.position.set(...V(target.x, target.y, z + 0.3));
    const group = new THREE.Group();
    group.add(sprite, ring);
    group.userData.sprite = sprite;
    group.userData.baseZ = z + 6;
    return group;
  }
  const state = {
    mirror: true,
    view: DRIVE_VIEWS[0],
    keys: new Set(),
    touch: { throttle: 0, steer: 0, brake: false },
  };

  let roadWorker = null;
  try {
    roadWorker = new Worker(
      new URL("./sf-road-markings-worker.js", import.meta.url),
      { type: "module" },
    );
  } catch (error) {
    console.warn(
      "Vector road paint unavailable; using texture markings.",
      error,
    );
  }
  const terrainPainter = createTerrainPainter({
    worker: roadWorker,
    paint: (entry, sources, fallback) =>
      paintTerrain(entry, sources, C, fallback),
    apply: (entry, positions) => applyRoadPaint(entry, positions, C),
    onError: (error) =>
      console.warn(
        "Vector road paint unavailable; using texture markings.",
        error,
      ),
  });

  function stream(position = car, required) {
    const { want, drop } = required
      ? { want: required, drop: [] }
      : tilesAround(manifest.tiles, position.x, position.y);
    for (const t of drop) {
      const e = loaded.get(t);
      if (!e) continue;
      for (const id of e.bakedRoadIds) {
        const entry = bakedRoads.get(id);
        if (--entry.owners) continue;
        scene.remove(entry.mesh);
        entry.mesh.traverse((object) => {
          object.geometry?.dispose();
          if (
            object.material &&
            object.material !== deckMaterial &&
            object.material !== roadMaterial
          )
            object.material.dispose();
        });
        for (const terminal of entry.surface.end_ids)
          bakedNetwork.endpoints.delete(terminal);
        bakedPortals.delete(id);
        roadSurfaces.delete(id);
        bakedRoads.delete(id);
      }
      for (const road of e.tile.raisedRoads || []) {
        const cached = raisedRoads.get(road.id);
        if (--cached.owners) continue;
        if (cached.mesh) {
          scene.remove(cached.mesh);
          cached.mesh.geometry.dispose();
          cached.mesh.material.dispose();
        }
        raisedRoads.delete(road.id);
      }
      canvas.dataset.raisedRoads = String(raisedRoads.size);
      terrainPainter.remove(t);
      scene.remove(e.grp);
      const materials = new Set();
      e.grp.traverse((o) => {
        o.geometry?.dispose();
        if (o.material) materials.add(o.material);
      });
      for (const material of materials) {
        material.map?.dispose();
        material.dispose();
      }
      loaded.delete(t);
    }
    for (const t of want) {
      if (loaded.has(t) || loading.has(t) || loading.size >= 3) continue;
      if ((failures.get(t) || 0) > performance.now()) continue;
      loading.add(t);
      (skylineRequests.get(t) || fetchJsonGz(`${tileBase}${t}.json.gz`))
        .then((data) => {
          const tile = { ...data };
          applyCityDriveScenery(tile, gameDrive?.sceneExclusions?.[t]);
          tile.half = byToken.get(t).half;
          const grp = buildTile(tile, C, regions.get(t), reveal);
          scene.add(grp);
          const roads = roadShapes(tile);
          const entry = {
            tile,
            grp,
            roads,
            bakedRoadIds: [],
            kerbs: tile.k.map((line) => {
              const points = [];
              for (let i = 0; i + 1 < line.length; i += 2)
                points.push([
                  tile.o[0] + line[i] / 10,
                  tile.o[1] + line[i + 1] / 10,
                ]);
              return points;
            }),
            region: regions.get(t),
            markings: roadDashPolygons(roads),
            collisions: buildCollisionIndex(tile, t, treeDamage.get(t)),
            intersections: intersectionsForTile(tile),
          };
          loaded.set(t, entry);
          for (const [index, damage] of treeDamage.get(t) || [])
            grp.userData.drawTree?.(index, damage);
          for (const road of tile.playableRoads || []) {
            const existing = bakedRoads.get(road.id);
            if (existing) existing.owners++;
            else {
              const contact = buildBakedRoadContact(road);
              if (!contact || roadSurfaces.has(road.id)) continue;
              const { surface, endpoints, portals, appearance } = contact;
              if (surface.kind === "tunnel" && !appearance) continue;
              if (surface.kind !== "tunnel")
                surface.boundary = roadBoundary(surface.contact);
              const mesh = new THREE.Group();
              mesh.add(
                new THREE.Mesh(
                  contactGeometry(surface.contact, false),
                  surface.kind === "tunnel" ? roadMaterial : deckMaterial,
                ),
              );
              if (appearance) mesh.add(tunnelMesh(surface, appearance));
              addGuardRails(surface, mesh);
              scene.add(mesh);
              bakedRoads.set(road.id, { surface, mesh, owners: 1 });
              bakedPortals.set(road.id, portals);
              roadSurfaces.set(road.id, surface);
              bakedNetwork.add({ endpoints, transitions: [] });
            }
            entry.bakedRoadIds.push(road.id);
          }
          for (const road of tile.raisedRoads || []) {
            const cached = raisedRoads.get(road.id);
            if (cached) {
              cached.owners++;
            } else {
              const mesh = buildRaisedRoad(road, replacedPreviews);
              raisedRoads.set(road.id, { mesh, owners: 1 });
              if (mesh) scene.add(mesh);
            }
          }
          canvas.dataset.raisedRoads = String(raisedRoads.size);
          const target = route.targets.find((q) => q.t === t);
          if (target) {
            entry.marker = markerFor(target);
            grp.add(entry.marker);
          }
          terrainPainter.add(t, entry);
          failures.delete(t);
        })
        .catch((e) => {
          const offline = !navigator.onLine;
          failures.set(t, offline ? Infinity : performance.now() + 5000);
          if (!offline && tileAt(manifest.tiles, car.x, car.y).t === t)
            announce("A city tile could not load. Retrying…");
          console.warn("tile", t, e);
        })
        .finally(() => loading.delete(t));
    }
    hud.tiles.textContent = `${loaded.size} / ${manifest.tiles.length} tiles`;
  }

  function resize() {
    const r = canvas.getBoundingClientRect();
    renderer.setSize(r.width, r.height, false);
    camera.aspect = r.width / r.height;
    camera.updateProjectionMatrix();
  }
  window.addEventListener("resize", resize);
  resize();

  const keyMap = {
    ArrowUp: "up",
    KeyW: "up",
    ArrowDown: "down",
    KeyS: "down",
    ArrowLeft: "left",
    KeyA: "left",
    ArrowRight: "right",
    KeyD: "right",
    Space: "brake",
  };
  window.addEventListener("keydown", (e) => {
    if (playback) {
      if (e.code === "Escape") {
        e.preventDefault();
        finishPlayback();
      } else if (keyMap[e.code] && !e.target.closest?.("button, a, summary"))
        e.preventDefault();
      return;
    }
    if (hud.menu.open) return;
    // A held key that opened the dialog is not a new driving command.
    if (e.repeat && keyMap[e.code]) {
      e.preventDefault();
      return;
    }
    if (hud.takeover.open) return;
    if (e.target.closest?.("select, input, textarea")) return;
    if (e.target.closest?.("button, a") && ["Space", "Enter"].includes(e.code))
      return;
    if (keyMap[e.code]) {
      state.keys.add(keyMap[e.code]);
      e.preventDefault();
    }
    if (!e.repeat && e.code === "KeyM") toggleMirror();
    if (!e.repeat && e.code === "KeyV") toggleView();
    if (!e.repeat && e.code === "KeyR") respawn();
  });
  window.addEventListener(
    "keyup",
    (e) => keyMap[e.code] && state.keys.delete(keyMap[e.code]),
  );
  // A key released in another window never sends its keyup here: drop held
  // inputs when the page loses focus so the car does not drive on by itself.
  let drivingPad;
  const releaseInputs = () => {
    state.keys.clear();
    drivingPad?.clear();
    state.touch = { throttle: 0, steer: 0, brake: false };
  };
  window.addEventListener("blur", releaseInputs);
  document.addEventListener("visibilitychange", () => {
    last = performance.now();
    if (document.hidden) releaseInputs();
  });
  const toggleMirror = () => {
    if (state.view !== "incar") return;
    state.mirror = !state.mirror;
    hud.mirrorFrame.hidden = !state.mirror;
    hud.mirrorBtn.setAttribute("aria-pressed", String(state.mirror));
  };
  const setView = (view) => {
    state.view = view;
    camInit = false;
    clearEmojiFocus();
    const index = DRIVE_VIEWS.indexOf(view);
    const name = ["Zoomed out", "Overhead", "In car"][index];
    hud.viewMode.value = index;
    hud.viewMode.setAttribute("aria-valuetext", name);
    hud.cameraName.value = name;
    hud.mirrorBtn.hidden = view !== "incar";
    hud.mirrorFrame.hidden = view !== "incar" || !state.mirror;
    canvas.dataset.view = view;
    canvas.setAttribute(
      "aria-label",
      `${name} 3D view of ${solo ? "the authored practice course" : "San Francisco"}`,
    );
  };
  const toggleView = () =>
    setView(
      DRIVE_VIEWS[(DRIVE_VIEWS.indexOf(state.view) + 1) % DRIVE_VIEWS.length],
    );
  const reset = (p = selectedStart, keepAirtime = false) => {
    cameraCoveredRoad = null;
    clearEmojiFocus();
    car = startingCar(p);
    grounded = false;
    camInit = false;
    accumulator = 0;
    crash = null;
    leadIn.clear();
    completedCityDrive = false;
    resumeRecovery = false;
    routeCheckpoint = null;
    airtime.reset(keepAirtime);
    updateBridgeMode();
    releaseInputs();
    if (document.hidden) saveTrialProgress();
  };
  function safeIntersections() {
    const entries = [...loaded.values()],
      tiles = entries.map((e) => e.tile),
      indexes = entries.map((e) => e.collisions);
    return [
      ...entries.flatMap((e) => e.intersections),
      ...route.targets.filter((t) => loaded.has(t.t)),
    ].filter((p) => {
      const z = groundAtWorld(tiles, p.x, p.y, true);
      return z !== null && !collisionAt(indexes, p, z);
    });
  }
  function recoverCityDrive() {
    const saved = routeCheckpoint;
    reset(selectedStart, true);
    if (saved) car = { ...saved.car };
    announce(
      saved
        ? "Self-driving resumed from the last route checkpoint"
        : "Self-driving resumed from the route start",
      6,
    );
  }
  function respawn(afterCrash = false) {
    if (solo) {
      restartSolo();
      return;
    }
    if (crash && !afterCrash) return;
    const kind = crash?.kind;
    if (isRoadReview(selectedStart)) {
      if (afterCrash && resumeRecovery) {
        recoverCityDrive();
        return;
      }
      reset(selectedStart, afterCrash);
      crashUntil = performance.now() + (afterCrash ? 0 : 750);
      announce("Back at the start of this surface");
      return;
    }
    const safe = safeIntersections();
    const point =
      (checkpoint &&
        safe.find(
          (p) => Math.hypot(p.x - checkpoint.x, p.y - checkpoint.y) < 1,
        )) ||
      safe.sort(
        (a, b) =>
          Math.hypot(a.x - car.x, a.y - car.y) -
          Math.hypot(b.x - car.x, b.y - car.y),
      )[0];
    if (!point) {
      crashUntil = performance.now() + 500;
      announce("Waiting for a clear intersection to load…");
      return;
    }
    let heading = point.heading;
    if (Math.cos(car.heading - heading) < 0) heading += Math.PI;
    reset({ ...point, heading }, afterCrash);
    checkpoint = { ...point, heading };
    crashUntil = performance.now() + (afterCrash ? 0 : 750);
    announce(
      kind
        ? `Hit a ${kind} · back at the intersection`
        : "Back at the intersection · clock continues",
    );
  }
  function startCrash(kind, incoming, hit, heightAt, now, supportAt, indexes) {
    if (crash) return;
    airtime.finish(now, kind === "fall" ? "Out of map" : "Impact");
    let treeEvent;
    if (hit?.tree && Math.hypot(hit.nx, hit.ny) > 0) {
      const tree = hit.tree;
      const damage = treeDamage.get(tree.tile) || new Map();
      treeEvent = treeDamageForImpact(
        tree,
        DRIVE_CAR.mass,
        closingSpeed(incoming, hit),
        hit.nx,
        hit.ny,
        damage.get(tree.index),
      );
      if (treeEvent) {
        damage.set(tree.index, treeEvent);
        treeDamage.set(tree.tile, damage);
        if (treeEvent.stage === "broken") {
          tree.broken = true;
          hit = { ...hit, yielded: treeEvent.absorbed };
        }
      }
    }
    crash = beginCrash(
      incoming,
      kind,
      hit,
      heightAt,
      kind === "ground" ? { previous: car, supportAt } : null,
    );
    const poses = leadIn.read();
    const crashRails = indexCrashRails(roadSurfaces.values());
    const impactTick = poses.length;
    do {
      car = crashCar(crash, incoming);
      const q = crash.rotation;
      poses.push({
        x: car.x,
        y: car.y,
        z: car.z,
        q: [q.x, q.z, -q.y, q.w],
        heading: crash.initialHeading,
        velocity: crash.velocity.toArray(),
      });
      if (crash.outcome) break;
      stepCrash(crash, supportAt, indexes, crashRails);
    } while (true);
    const inbound = capturePose(incoming, heightAt).velocity;
    const peakJerk = Math.max(
      0,
      ...(replayTelemetry(poses.slice(impactTick), 0, inbound) ?? []).map(
        (bin) => bin[2],
      ),
    );
    const fee = recordCrash(insurance, peakJerk);
    const clip = saveReplay({
      ...encodeReplay(
        poses,
        impactTick,
        crash.outcome === "stopped" ? CRASH_HOLD_TICKS : 0,
      ),
      kind,
      outcome: crash.outcome,
      fee,
      ...(treeEvent && { treeDamage: treeEvent }),
      telemetry: replayTelemetry(poses, impactTick, inbound),
    });
    if (hud.takeover.open) hud.takeover.close("cancel");
    startPlayback(clip, true);
    clearEmojiFocus();
    releaseInputs();
    if (trial.status === "running") trial.crashes++;
    const impact =
      kind === "ground"
        ? "Hard landing"
        : kind === "fall"
          ? "Fell off the road"
          : `Hit a ${kind}`;
    announce(
      `${impact} · ${fee > 0 ? formatInsuranceCents(fee) + " crash charge" : "crash replay"}`,
      2,
    );
  }
  function newTrial(p, fresh = false) {
    if (solo) {
      restartSolo();
      return;
    }
    if (!fresh) saveTrialProgress();
    selectedStart = p;
    insurance = insuranceFor(p);
    trial = createTrial(activeDistrict?.targets || route.targets);
    checkpoint = null;
    crashUntil = 0;
    reset(p);
    if (!isRoadReview(p)) {
      if (fresh) writeProgress(progressKey(), null);
      else checkpoint = loadTrialProgress();
    }
    announce(
      isRoadReview(p)
        ? "Drive the supported approach · R resets this level"
        : `Collect every marked emoji${activeDistrict ? ` in ${activeDistrict.name}` : ""}. Drive to start the clock.`,
      4,
    );
  }
  // A clicked control would otherwise keep focus and swallow the driving keys
  // (the keydown handler ignores keys aimed at buttons and selects).
  const releaseFocus = (e) => {
    if (e.detail !== 0) e.currentTarget.blur(); // pointer clicks only; keyboard users keep their place
  };
  hud.mirrorBtn.addEventListener("click", (e) => {
    toggleMirror();
    releaseFocus(e);
  });
  hud.viewMode.addEventListener("input", () => {
    setView(DRIVE_VIEWS[hud.viewMode.valueAsNumber]);
  });
  hud.menuBtn.addEventListener("click", () => {
    showReplayLibrary();
    releaseInputs();
    clearEmojiFocus();
  });
  function requestTakeover() {
    if (crash || insurance.mode === "manual" || hud.takeover.open) return;
    releaseInputs();
    if (hud.menu.open) hud.menu.close();
    hud.takeover.returnValue = "";
    hud.takeover.showModal();
  }
  hud.takeover.querySelector("form").addEventListener("submit", (event) => {
    event.preventDefault();
    const decision = event.submitter?.value;
    if (decision === "manual") {
      // Ownership changes on confirmation, without resetting vehicle state or
      // feeding the input used to open the modal into the physics loop.
      releaseInputs();
      insurance.mode = "manual";
      resumeRecovery = false;
      updateDriveMode();
    }
    hud.takeover.close(decision || "cancel");
    if (decision === "manual") canvas.focus({ preventScroll: true });
  });
  hud.followCityDrive.addEventListener("click", () => {
    if (completedCityDrive) {
      newTrial(selectedStart, true);
      hud.menu.close();
    } else if (insurance.mode === "automatic") requestTakeover();
    else {
      insurance.mode = "automatic";
      resumeRecovery = true;
      releaseInputs();
      updateDriveMode();
      hud.menu.close();
      canvas.focus({ preventScroll: true });
    }
  });
  hud.restartBtn.addEventListener("click", (e) => {
    newTrial(selectedStart, true);
    hud.menu.close();
    releaseFocus(e);
  });
  hud.respawnBtn.addEventListener("click", (e) => {
    respawn();
    releaseFocus(e);
  });
  hud.places.replaceChildren(
    ...(activeDistrict ? [activeDistrict.start] : startChoices).map(
      (p) => new Option(p.name, p.id),
    ),
  );
  hud.places.value = start.id;
  hud.places.disabled = !!activeDistrict;
  if (activeDistrict)
    hud.trialTitle.textContent = `${activeDistrict.name} emoji rally`;
  hud.places.addEventListener("change", () => {
    const p = startChoices.find((q) => q.id === hud.places.value);
    if (p) {
      params.set("at", p.id);
      if (p.city) {
        params.set("city", p.city);
        location.assign(`?${params}`);
        return;
      }
      if (gameDrive && !p.surfaceId) {
        params.delete("city");
        location.assign(`?${params}`);
        return;
      }
      if (gameDrive) params.set("city", cityMode);
      newTrial(p);
      history.replaceState(null, "", `?${params}`);
    }
    hud.places.blur();
  });
  drivingPad = bindDrivingPad(hud.touchButtons, (input) => {
    if (!playback) state.touch = input;
  });

  function restartSolo() {
    endSoloRun(soloRun, "aborted");
    hud.soloPrevious.textContent = soloSummary(soloRun);
    soloRun = createSoloRun();
    car = soloCar();
    airtime.reset();
    leadIn.clear();
    carZ = car.z;
    accumulator = 0;
    releaseInputs();
    camInit = false;
    hud.soloSummary.textContent = soloSummary(soloRun);
  }
  function enterSolo() {
    solo = true;
    hud.insuranceHud.hidden = true;
    document.body.dataset.solo = "true";
    practice.visible = true;
    roadMeshes.visible = false;
    surveyReplay.update(null, 0);
    restartSolo();
    hud.restartBtn.hidden = true;
    hud.respawnBtn.textContent = "Restart solo route";
    hud.timer.setAttribute(
      "aria-label",
      "Elapsed simulation time, no time reward",
    );
  }

  function clearEmojiFocus() {
    emojiLocked = false;
    hud.emojiFocus.setAttribute("aria-pressed", "false");
  }
  hud.emojiFocus.addEventListener("click", (event) => {
    if (
      !emojiTarget ||
      state.keys.size ||
      state.touch.throttle ||
      state.touch.steer ||
      state.touch.brake
    )
      return;
    emojiLocked = !emojiLocked;
    hud.emojiFocus.setAttribute("aria-pressed", String(emojiLocked));
    releaseFocus(event);
  });
  function drawEmojiFocus(marker) {
    const button = hud.emojiFocus;
    if (!marker?.visible) {
      button.hidden = true;
      return;
    }
    camera.updateMatrixWorld();
    emojiScreen.copy(marker.userData.sprite.position).project(camera);
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;
    const x = ((emojiScreen.x + 1) * width) / 2;
    const y = ((1 - emojiScreen.y) * height) / 2;
    // Keep the button clear of the status bar, safe-area edges and driving pads.
    const visible =
      emojiScreen.z >= -1 &&
      emojiScreen.z <= 1 &&
      x >= 32 &&
      x <= width - 32 &&
      y >= 105 &&
      y <= height - 100;
    button.hidden = !visible;
    marker.userData.sprite.visible = !visible;
    if (!visible) return;
    button.style.left = `${x}px`;
    button.style.top = `${y}px`;
  }
  function vehicleVelocity(source, pitch) {
    if (source.grounded === false) return [source.vx, source.vy, source.vz];
    const h = source.heading,
      slip = source.slip || 0;
    return [
      Math.cos(h) * source.v - Math.sin(h) * slip,
      Math.sin(h) * source.v + Math.cos(h) * slip,
      Math.tan(pitch) * source.v,
    ];
  }

  function capturePose(source, heightAt, soloPitchValue) {
    const { x, y, z, heading } = source;
    const airborne = source.grounded === false;
    const ax = Math.cos(heading) * 1.5,
      ay = Math.sin(heading) * 1.5;
    const pitch = airborne
      ? source.airAttitude?.pitch || 0
      : (soloPitchValue ??
        Math.atan2(
          (heightAt(x + ax, y + ay) ?? z) - (heightAt(x - ax, y - ay) ?? z),
          3,
        ));
    poseEuler.set(
      airborne ? source.airAttitude?.roll || 0 : 0,
      heading,
      pitch,
      "YZX",
    );
    return {
      x,
      y,
      z,
      heading,
      q: poseRotation.setFromEuler(poseEuler).toArray(),
      velocity: vehicleVelocity(source, pitch),
    };
  }

  function saveReplay(data) {
    const created = Date.now();
    const clip = {
      ...data,
      mode: replayMode(),
      created,
      id: `${created.toString(36)}-${performance.timeOrigin.toString(36)}-${++replayId}`,
    };
    replaySaved = library.save(clip);
    return clip;
  }

  function replayUrl(clip) {
    const query = new URLSearchParams({ replay: clip.id });
    if (clip.mode === "practice") query.set("mode", "solo");
    else if (clip.mode === "city") query.set("at", "embarcadero");
    else query.set("city", clip.mode);
    return `?${query}`;
  }

  function replayLocation(clip) {
    const targets =
      clip.mode === "practice"
        ? [{ x: SOLO.end, y: 0, emoji: "🏁", names: ["Practice finish"] }]
        : route.targets;
    return replayLandmark(clip, targets);
  }

  function showReplayLibrary() {
    const { crashes, airtimes } = library.read();
    for (const [list, clips] of [
      [hud.crashReplays, crashes],
      [hud.airtimeReplays, airtimes],
    ]) {
      list.replaceChildren(
        ...clips.map((clip) => {
          const row = document.createElement("li");
          const label =
            clip.impact === null
              ? `${clip.seconds.toFixed(2)} s · ${clip.peak.toFixed(1)} m · ${clip.mode === "practice" ? "Practice" : "City"}${clip.ending === "impact" ? " · Impact ending" : ""}`
              : `${clip.kind} · ${replayDay.format(clip.created)}`;
          const sameMode = clip.mode === replayMode();
          const action = document.createElement(sameMode ? "button" : "a");
          action.textContent = `Play ${label}`;
          if (sameMode) {
            action.type = "button";
            action.addEventListener("click", () => startPlayback(clip, false));
          } else {
            action.href = replayUrl(clip);
            action.target = "_blank";
            action.rel = "noopener";
            action.setAttribute("aria-label", `Play ${label} in a new tab`);
          }
          const location = document.createElement("small");
          location.textContent = replayLocation(clip);
          row.append(action, location);
          const controls = document.createElement("div");
          controls.className = "replay-actions";
          const button = (name, handle) => {
            const control = document.createElement("button");
            control.type = "button";
            control.textContent = name;
            control.addEventListener("click", handle);
            controls.append(control);
            return control;
          };
          if (clip.impact !== null) {
            const favourite = button("Favourite", () => {
              const updated = library.setFavourite(
                clip.id,
                clip.favourite !== true,
              );
              if (updated !== null) replaySaved = updated;
              showReplayLibrary();
              hud.replayLibrary.querySelector("summary").focus();
            });
            favourite.setAttribute("aria-label", `Favourite ${label}`);
            favourite.setAttribute(
              "aria-pressed",
              String(clip.favourite === true),
            );
            if (
              !clip.favourite &&
              crashes.filter((record) => record.favourite === true).length >=
                FAVOURITE_LIMIT
            ) {
              favourite.disabled = true;
              favourite.title = "Remove a favourite to save another";
            }
          }
          const remove = button("Delete", () => {
            if (!window.confirm(`Delete the replay “${label}”?`)) return;
            replaySaved = library.remove(clip.id);
            showReplayLibrary();
            hud.replayLibrary.querySelector("summary").focus();
          });
          remove.setAttribute("aria-label", `Delete ${label}`);
          row.append(controls);
          return row;
        }),
      );
      if (!clips.length) {
        const empty = document.createElement("li");
        empty.textContent = "No saved replays yet";
        list.append(empty);
      }
    }
    hud.replayStorage.textContent = replaySaved
      ? ""
      : "Browser storage is unavailable. New replays may only be available in this session.";
  }

  function startPlayback(clip, automatic) {
    if (playback) return;
    const required =
      clip.mode === "practice"
        ? []
        : [
            ...new Set(
              clip.frames.map(
                ([, x, y]) =>
                  tileAt(
                    manifest.tiles,
                    x / REPLAY_POSITION_SCALE,
                    y / REPLAY_POSITION_SCALE,
                  ).t,
              ),
            ),
          ];
    const tree = clip.treeDamage;
    if (tree && byToken.has(tree.tile) && !required.includes(tree.tile))
      required.push(tree.tile);
    releaseInputs();
    clearEmojiFocus();
    if (hud.menu.open) hud.menu.close();
    playback = {
      clip,
      automatic,
      required,
      tick: 0,
      streamAt: -Infinity,
      charts: replayCharts(hud.replayGraphs, clip),
    };
    hud.replayLocation.textContent = [
      replayLocation(clip),
      clip.fee > 0 ? `${formatInsuranceCents(clip.fee)} crash charge` : "",
      tree
        ? `${Math.round(tree.diameter * 100)} cm trunk · tree ${tree.stage === "broken" ? "fell" : "cracked"}`
        : "",
    ]
      .filter(Boolean)
      .join(" · ");
    hud.replayPanel.hidden = false;
    hud.replayGraphs.hidden = false;
    hud.replayRate.textContent = `${replayRate(clip, 0)}×`;
    hud.menuBtn.disabled = true;
    hud.respawnBtn.disabled = true;
    hud.replayStatus.textContent =
      clip.impact === null ? "Airtime replay" : "Crash replay";
    hud.skipReplay.focus({ preventScroll: true });
  }

  function finishPlayback() {
    if (!playback) return;
    const { automatic, clip, treeIndex } = playback;
    const tree = clip.treeDamage;
    if (tree && treeIndex >= 0)
      loaded
        .get(tree.tile)
        ?.grp.userData.drawTree?.(
          treeIndex,
          treeDamage.get(tree.tile)?.get(treeIndex),
        );
    else if (tree && automatic)
      loaded.get(tree.tile)?.grp.userData.drawTree?.(tree.index, tree);
    playback = null;
    hud.replayPanel.hidden = true;
    hud.replayGraphs.hidden = true;
    hud.menuBtn.disabled = false;
    hud.respawnBtn.disabled = false;
    delete canvas.dataset.replay;
    releaseInputs();
    camInit = false;
    accumulator = 0;
    if (automatic) respawn(true);
    else if (!solo) stream();
    if (automatic) canvas.focus({ preventScroll: true });
    else hud.menuBtn.focus({ preventScroll: true });
  }

  hud.skipReplay.addEventListener("click", finishPlayback);

  function replayFrame(wallDt, now) {
    const { clip, automatic, required } = playback;
    if (automatic) {
      if (!isRoadReview(selectedStart)) {
        advanceTrial(trial, wallDt, car, false, false);
        hud.timer.textContent = formatTime(trial.elapsed);
      }
      updateInsuranceMeter();
    }
    if (
      now - playback.streamAt >= 400 &&
      required.some((t) => !loaded.has(t))
    ) {
      playback.streamAt = now;
      stream(car, required);
    }
    if (required.some((t) => !loaded.has(t))) {
      const failed = required.some((t) => failures.has(t) && !loaded.has(t));
      hud.replayStatus.textContent = failed
        ? "Some replay tiles are unavailable"
        : "Loading replay tiles…";
      return;
    }
    if (!playback.started) playback.started = true;
    else if (!document.hidden)
      playback.tick = advanceReplay(clip, playback.tick, wallDt);
    const rateText = `${replayRate(clip, playback.tick)}×`;
    if (hud.replayRate.textContent !== rateText)
      hud.replayRate.textContent = rateText;
    if (hud.replayGraphs.open) playback.charts(playback.tick);
    const tree = clip.treeDamage;
    const treeEntry = tree && loaded.get(tree.tile);
    if (treeEntry && playback.treeIndex === undefined)
      playback.treeIndex = replayTreeIndex(treeEntry.tile, tree);
    if (playback.treeIndex >= 0 && !playback.treeFallen) {
      const elapsed = Math.max(0, (playback.tick - clip.impact) / 120);
      treeEntry.grp.userData.drawTree?.(playback.treeIndex, tree, elapsed);
      playback.treeFallen = treeFallProgress(elapsed) === 1;
    }
    const pose = replayPose(clip, playback.tick);
    canvas.dataset.replay = clip.impact === null ? "airtime" : "crash";
    let status = clip.impact === null ? "Airtime replay" : "Crash replay";
    if (
      clip.outcome === "stopped" &&
      playback.tick >= clip.end - CRASH_HOLD_TICKS
    )
      status = "Car stopped";
    else if (clip.impact !== null && playback.tick < clip.impact)
      status = "Before impact";
    else if (clip.takeoff !== undefined && playback.tick < clip.takeoff)
      status = "Before take-off";
    else if (clip.touchdown !== undefined && playback.tick >= clip.touchdown)
      status = clip.ending === "impact" ? "Impact ending" : "Touchdown";
    else if (playback.tick === clip.end && clip.outcome !== "stopped")
      status = "Recovery";
    if (status !== hud.replayStatus.textContent)
      hud.replayStatus.textContent = status;
    hud.emojiFocus.hidden = true;
    hud.airtime.hidden = true;
    carMesh.position.set(...V(pose.x, pose.y, pose.z));
    carMesh.quaternion.fromArray(pose.q);
    carMesh.userData.brakeMaterial.color.setHex(0x661018);
    for (const wheel of carMesh.userData.wheels) wheel.pivot.rotation.y = 0;
    const tiles = [...loaded.values()].map((e) => e.tile);
    const ground = solo
      ? soloHeight
      : (x, y) => groundAtWorld(tiles, x, y, true);
    const view = driveCameraPose(
      state.view === "overhead" ? "overhead" : "zoomed",
      pose,
      pose.z,
      ground,
    );
    camera.position.set(...V(...view.position));
    camera.up.set(...V(...view.up));
    camera.lookAt(...V(...view.look));
    renderView(view, pose);
    if (playback.tick >= clip.end) finishPlayback();
  }

  function renderView(view, subject = car) {
    const hx = Math.cos(subject.heading),
      hy = Math.sin(subject.heading);
    const W = renderer.domElement.width;
    const H = renderer.domElement.height;
    const pr = renderer.getPixelRatio();
    renderer.setScissorTest(false);
    renderer.setViewport(0, 0, W / pr, H / pr);
    carMesh.visible = !view.hideCar;
    if (carMesh.visible)
      reveal.update(camera, carMesh.position, subject.heading, W, H, pr);
    else reveal.disable();
    let drawSkyline = false;
    if (
      skylineTiles.size &&
      !solo &&
      state.view !== "overhead" &&
      ["bay-bridge-westbound", "city-approach"].includes(car.surfaceId)
    ) {
      const shoreDistance = shoreline
        ? Math.hypot(car.x - shoreline[0], car.y - shoreline[1])
        : Infinity;
      const distantOpacity =
        car.surfaceId === "bay-bridge-westbound"
          ? THREE.MathUtils.smoothstep(shoreDistance, 190, 650)
          : 0;
      let local = 0,
        distant = 0;
      for (const [token, { buildings, material }] of skylineTiles) {
        const ready = loaded.has(token);
        local += Number(ready);
        material.opacity = ready ? distantOpacity : 1;
        buildings.visible = material.opacity > 0.01;
        distant += Number(buildings.visible);
      }
      if (canvas.dataset.skylineLocalTiles !== String(local))
        canvas.dataset.skylineLocalTiles = String(local);
      if (canvas.dataset.skylineFarTiles !== String(distant))
        canvas.dataset.skylineFarTiles = String(distant);
      drawSkyline = distant > 0;
    }
    skylineBuildings.visible = drawSkyline;
    if (drawSkyline) {
      skylineCamera.position.copy(camera.position);
      skylineCamera.quaternion.copy(camera.quaternion);
      if (skylineCamera.aspect !== camera.aspect) {
        skylineCamera.aspect = camera.aspect;
        skylineCamera.updateProjectionMatrix();
      }
      renderer.render(skylineScene, skylineCamera);
      // Reuse the colour buffer but start a fresh depth buffer for local
      // roads and buildings, which still use their normal clip and fog range.
      scene.background = null;
      renderer.autoClear = false;
      renderer.clearDepth();
      renderer.render(scene, camera);
      renderer.autoClear = true;
      scene.background = C.sky;
    } else {
      renderer.render(scene, camera);
    }
    if (!solo && beacons.count) {
      beaconCamera.position.copy(camera.position);
      beaconCamera.quaternion.copy(camera.quaternion);
      if (beaconCamera.aspect !== camera.aspect) {
        beaconCamera.aspect = camera.aspect;
        beaconCamera.updateProjectionMatrix();
      }
      // Keep the local depth buffer so buildings can obscure the nearby
      // columns. The larger far plane covers surveys beyond streamed tiles.
      renderer.autoClear = false;
      renderer.render(beaconScene, beaconCamera);
      renderer.autoClear = true;
    }
    carMesh.visible = true;
    const showMirror = view.mirror && state.mirror;
    hud.mirrorFrame.hidden = !showMirror;
    if (showMirror) {
      // the mirror: from the cabin roof, looking back over the boot
      mirrorCam.position.set(
        ...V(car.x - hx * 0.4, car.y - hy * 0.4, carZ + 1.9),
      );
      mirrorCam.lookAt(...V(car.x - hx * 30, car.y - hy * 30, carZ + 1.2));
      // Match the scissor rectangle to the centred CSS frame.
      const frame = hud.mirrorFrame.getBoundingClientRect();
      const mw = frame.width * pr,
        mh = frame.height * pr;
      const mx = frame.left * pr,
        my = H - frame.bottom * pr;
      mirrorCam.aspect = mw / mh;
      mirrorCam.updateProjectionMatrix();
      renderer.setScissorTest(true);
      renderer.setScissor(mx / pr, my / pr, mw / pr, mh / pr);
      renderer.setViewport(mx / pr, my / pr, mw / pr, mh / pr);
      carMesh.visible = false; // the view behind, not the inside of the cabin
      scene.fog = mirrorFog;
      renderer.render(scene, mirrorCam);
      scene.fog = driveFog;
      carMesh.visible = true;
      renderer.setScissorTest(false);
    }
  }
  function soloFrame(dt, now) {
    const input = drivingInput(state.keys, state.touch);
    const paused = hud.menu.open || document.hidden || !document.hasFocus();
    if (!paused && input.throttle !== 0) startSoloRun(soloRun);
    accumulator += dt;
    while (accumulator >= 1 / 120) {
      accumulator -= 1 / 120;
      if (soloRun.status !== "running") continue;
      if (paused) {
        accountSolo(soloRun, 1 / 120, { excluded: true });
        continue;
      }
      const before = car;
      car = contactStep(car, input, 1 / 120, soloSurface);
      if (!car.blocked && (!car.grounded || !before.grounded)) {
        const min = SOLO.start - 20,
          max = SOLO.end + 40;
        airtime.step(before, car, 1 / 120, now, {
          road: {
            centreline: [min * 10, 0, max * 10, 0],
            origin: [0, 0],
            segment: 0,
            fraction: (before.x - min) / (max - min),
            heading: 0,
          },
          heightAt: soloHeight,
          leadIn: leadIn.read,
          pose: (source) => capturePose(source, soloHeight, soloPitch(source)),
        });
      }
      if (!car.blocked)
        leadIn.push(capturePose(before, soloHeight, soloPitch(before)));
      accountSolo(
        soloRun,
        1 / 120,
        car.blocked ? { excluded: true } : soloSample(before, car),
      );
      if (
        car.x >= SOLO.end &&
        Math.abs(car.y) <= SOLO.halfWidth &&
        car.grounded
      ) {
        endSoloRun(soloRun);
        showReplayLibrary();
        releaseInputs();
        hud.menu.showModal();
        hud.menuBtn.setAttribute("aria-expanded", "true");
        break;
      }
    }
    for (const e of loaded.values()) e.grp.visible = false;
    carZ = car.z;
    carMesh.position.set(...V(car.x, car.y, car.z));
    carMesh.rotation.set(
      car.grounded === false ? car.airAttitude?.roll || 0 : 0,
      car.heading,
      soloPitch(car),
      "YZX",
    );
    carMesh.visible = state.view !== "incar";
    carMesh.userData.brakeMaterial.color.setHex(
      brakeLightsOn(car, input, soloRun.status !== "running")
        ? 0xff3038
        : 0x661018,
    );
    for (const wheel of carMesh.userData.wheels) {
      wheel.pivot.rotation.y = wheel.front ? car.steering || 0 : 0;
      if (!paused) wheel.tyre.rotation.z -= (car.v * dt) / 0.36;
    }
    const view = driveCameraPose(
      state.view,
      car,
      carZ,
      (x) => soloHeight(x),
      0,
    );
    camera.position.set(...V(...view.position));
    camera.up.set(...V(...view.up));
    camera.lookAt(...V(...view.look));
    renderView(view);
    airtime.draw(now);
    const statusChanged = canvas.dataset.soloStatus !== soloRun.status;
    canvas.dataset.soloStatus = soloRun.status;
    canvas.dataset.contact = car.grounded ? "grounded" : "airborne";
    if (now - soloHudAt < 250 && !statusChanged) return;
    soloHudAt = now;
    const sample = soloSample(car, car);
    hud.street.textContent = "Solo · authored crest";
    hud.speed.textContent = Math.round(sample.mph);
    hud.timer.textContent = formatTime(soloRun.elapsed);
    hud.progress.textContent = sample.limit
      ? "20 mph practice limit"
      : "Limit unknown";
    const cue = car.blocked
      ? "Course boundary · restart in menu"
      : soloRun.status === "finished"
        ? "Complete · evidence in menu"
        : sample.drivable === false
          ? "Off road · return gently"
          : sample.curbContact
            ? "Kerb contact"
            : !car.grounded
              ? "Airborne · momentum carries"
              : `${Math.max(0, Math.round(SOLO.end - car.x))} m east to finish · no time reward`;
    if (hud.next.textContent !== cue) hud.next.textContent = cue;
    hud.driveBar.dataset.crash = "false";
    if (hud.menu.open) hud.soloSummary.textContent = soloSummary(soloRun);
    if (hud.menu.open)
      hud.soloTrace.textContent = JSON.stringify(
        {
          limits: soloRun.limits,
          excursions: soloRun.excursions,
          strikes: soloRun.strikes,
          incidents: soloRun.incidents,
        },
        null,
        2,
      );
  }

  let last = performance.now();
  let hudAt = 0;
  let offlineTiles = new Set();
  let coverageReadAt = -Infinity;
  let coverageReading = false;
  let streamAt = 0;
  const camPos = new THREE.Vector3();
  const camLook = new THREE.Vector3();
  const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
  let cameraRoll = 0;
  let camInit = false;
  let cameraCoveredRoad = null;
  if (params.get("mode") === "solo") enterSolo();
  setView(state.view);
  let roadAt = 0,
    road = null;
  const seekerColours = {
    muted: "#b8c5cc",
    accent: "#fbbf24",
  };
  function drawSeeker() {
    const seeker = questSeeker(trial, manifest.tiles, car);
    const cv = hud.seekerCompass,
      ctx = cv.getContext("2d");
    const w = cv.clientWidth,
      dpr = Math.min(2, devicePixelRatio || 1),
      c = w / 2;
    if (cv.width !== Math.round(w * dpr))
      cv.width = cv.height = Math.round(w * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, w);
    ctx.beginPath();
    ctx.arc(c, c, w * 0.48, 0, Math.PI * 2);
    ctx.strokeStyle = seekerColours.muted;
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.font = `600 ${w * 0.13}px system-ui`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    for (const [name, bearing] of [
      ["N", 0],
      ["E", 90],
      ["S", 180],
      ["W", 270],
    ]) {
      const angle = ((bearing - seeker.heading) * Math.PI) / 180;
      ctx.fillStyle = name === "N" ? seekerColours.accent : seekerColours.muted;
      ctx.fillText(
        name,
        c + Math.sin(angle) * w * 0.43,
        c - Math.cos(angle) * w * 0.43,
      );
    }
    if (seeker.target) {
      const angle = (seeker.relative * Math.PI) / 180;
      ctx.save();
      ctx.translate(c, c);
      ctx.rotate(angle);
      ctx.strokeStyle = "#ffffff";
      ctx.fillStyle = seekerColours.accent;
      ctx.lineWidth = 2;
      ctx.lineJoin = "round";
      ctx.shadowColor = "#000";
      ctx.shadowBlur = 4;
      ctx.beginPath();
      ctx.moveTo(0, -w * 0.35);
      ctx.lineTo(w * 0.22, -w * 0.04);
      ctx.lineTo(w * 0.09, -w * 0.04);
      ctx.lineTo(w * 0.09, w * 0.28);
      ctx.lineTo(-w * 0.09, w * 0.28);
      ctx.lineTo(-w * 0.09, -w * 0.04);
      ctx.lineTo(-w * 0.22, -w * 0.04);
      ctx.closePath();
      ctx.fill();
      ctx.stroke();
      ctx.restore();
    }
    // Your heading is always up; the compass ring and target turn around it.
    if (!seeker.target) {
      ctx.fillStyle = seekerColours.accent;
      ctx.font = `600 ${w * 0.3}px system-ui`;
      ctx.fillText("✓", c, c);
    }
    hud.seekerScope.textContent = !seeker.target
      ? "All emojis"
      : seeker.onTile
        ? "This tile"
        : "Next tile";
    hud.seekerDistance.textContent = seeker.target
      ? `${seeker.target.emoji} ${seeker.distance >= 1000 ? `${(seeker.distance / 1000).toFixed(1)} km` : `${Math.round(seeker.distance)} m`}`
      : "✓";
    hud.seekerTurn.textContent = seeker.target
      ? seekerDirection(seeker.relative)
      : "Complete";
    hud.seekerHeading.textContent = `${seeker.cardinal} · ${String(Math.round(seeker.heading) % 360).padStart(3, "0")}°`;
    cv.setAttribute(
      "aria-label",
      `Heading ${seeker.cardinal}, ${Math.round(seeker.heading) % 360} degrees. ${seeker.target ? `${seeker.target.emoji}, ${seeker.onTile ? "this tile" : "next tile"}, ${Math.round(seeker.distance)} metres away, bearing ${Math.round(seeker.bearing) % 360} degrees.` : "All emojis collected."}`,
    );
  }
  async function refreshOfflineCoverage(now) {
    if (
      coverageReading ||
      now - coverageReadAt < 2000 ||
      !window.caches ||
      !tileCacheName
    )
      return;
    coverageReadAt = now;
    coverageReading = true;
    try {
      const entries = (await caches.has(tileCacheName))
        ? await (await caches.open(tileCacheName)).keys()
        : [];
      const prefix = new URL(tileBase, location.href).href;
      offlineTiles = new Set(
        entries.flatMap(({ url }) => {
          if (!url.startsWith(prefix)) return [];
          const match = /^([a-f0-9]{8})\.json\.gz$/.exec(
            url.slice(prefix.length),
          );
          return match && byToken.has(match[1]) ? [match[1]] : [];
        }),
      );
      hud.offlineTileCount.textContent = `${offlineTiles.size}/${manifest.tiles.length} tiles available offline`;
    } catch {
      offlineTiles = new Set();
      hud.offlineTileCount.textContent = "Offline coverage unavailable";
    } finally {
      coverageReading = false;
    }
  }

  function drawMinimap(cv) {
    const w = cv.clientWidth,
      h = cv.clientHeight,
      dpr = Math.min(2, devicePixelRatio || 1);
    if (!w || !h) return; // The gameplay map is hidden on touch layouts.
    refreshOfflineCoverage(performance.now());
    const ctx = cv.getContext("2d");
    if (cv.width !== Math.round(w * dpr) || cv.height !== Math.round(h * dpr)) {
      cv.width = Math.round(w * dpr);
      cv.height = Math.round(h * dpr);
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const styles = getComputedStyle(cv);
    const colour = (name) => styles.getPropertyValue(name).trim();
    ctx.fillStyle = colour("--map-water");
    ctx.fillRect(0, 0, w, h);
    const polygons = activeDistrict
      ? [...activeDistrict.polygons.flat(2), [car.x, car.y]]
      : coverageCorners;
    const minX = Math.min(...polygons.map((p) => p[0])),
      maxX = Math.max(...polygons.map((p) => p[0]));
    const minY = Math.min(...polygons.map((p) => p[1])),
      maxY = Math.max(...polygons.map((p) => p[1]));
    const scale = Math.min((w - 24) / (maxX - minX), (h - 24) / (maxY - minY));
    const point = (x, y) => [
      (x - (minX + maxX) / 2) * scale + w / 2,
      h / 2 - (y - (minY + maxY) / 2) * scale,
    ];
    ctx.save();
    ctx.translate(...point(0, 0));
    ctx.scale(scale, -scale);
    ctx.fillStyle = colour("--map-land");
    ctx.fill(landPath, "evenodd");
    ctx.strokeStyle = colour("--map-coast");
    ctx.lineWidth = 0.8 / scale;
    ctx.stroke(landPath);
    ctx.lineWidth = 0.55 / scale;
    ctx.strokeStyle = colour("--map-grid");
    for (const row of manifest.tiles) {
      ctx.globalAlpha =
        activeDistrict && !activeDistrict.overlapTiles.has(row.t) ? 0.25 : 1;
      const cell = cellPaths.get(row.t);
      ctx.fillStyle = trial.collected.has(row.t)
        ? "#d79927a6"
        : loaded.has(row.t)
          ? "#0f93839e"
          : loading.has(row.t)
            ? "#73c7d199"
            : "#8c99a51a";
      ctx.fill(cell);
      ctx.stroke(cell);
    }
    ctx.globalAlpha = 1;
    ctx.fillStyle = colour("--map-park");
    ctx.fill(parkPath, "evenodd");
    ctx.strokeStyle = colour("--map-arterial");
    ctx.lineWidth = 0.75 / scale;
    ctx.lineJoin = "round";
    ctx.lineCap = "round";
    ctx.stroke(arterialPath);
    ctx.strokeStyle = colour("--map-offline");
    ctx.lineWidth = 1.35 / scale;
    for (const token of offlineTiles) ctx.stroke(cellPaths.get(token));
    ctx.restore();
    if (activeDistrict) {
      for (const polygon of activeDistrict.polygons) {
        ctx.beginPath();
        for (const ring of polygon) {
          ring.forEach(([x, y], i) =>
            i ? ctx.lineTo(...point(x, y)) : ctx.moveTo(...point(x, y)),
          );
          ctx.closePath();
        }
        ctx.fillStyle = "#f59e0b22";
        ctx.fill("evenodd");
        ctx.strokeStyle = "#d38b00";
        ctx.lineWidth = 2.5;
        ctx.stroke();
      }
    }
    ctx.fillStyle = colour("--scene-vehicle");
    for (const site of surveySites) {
      const [x, y] = point(site.x, site.y);
      ctx.beginPath();
      ctx.moveTo(x, y - 2.8);
      ctx.lineTo(x + 2.8, y);
      ctx.lineTo(x, y + 2.8);
      ctx.lineTo(x - 2.8, y);
      ctx.fill();
    }
    const [cx, cy] = point(car.x, car.y);
    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate(-car.heading);
    ctx.beginPath();
    ctx.moveTo(4, 0);
    ctx.lineTo(-2.5, -2);
    ctx.lineTo(-1.5, 0);
    ctx.lineTo(-2.5, 2);
    ctx.closePath();
    ctx.fillStyle = "#fff";
    ctx.strokeStyle = "#17202a";
    ctx.lineWidth = 0.8;
    ctx.fill();
    ctx.stroke();
    ctx.restore();
    ctx.fillStyle = colour("--ink");
    ctx.font = "600 11px system-ui";
    ctx.textAlign = "right";
    ctx.fillText("N ↑", w - 6, 14);
  }
  function updateTrialHud() {
    const nearest = questSeeker(trial, manifest.tiles, car).target;
    downloads.observe(performance.getEntriesByType("resource"));
    const size = downloads.sizes(loaded.keys());
    hud.assets.textContent = `${formatBytes(size.resident)} loaded · ${formatBytes(size.tiles)} city tiles`;
    hud.assets.title =
      "Compressed response-body sizes of the loaded tiles and complete city tile set.";
    hud.gameSize.textContent = `${size.estimated ? "≈ " : ""}${formatBytes(size.total)} asset download`;
    hud.gameSize.title = size.estimated
      ? "Compressed network payload: gzip estimates where browser response sizes are unavailable. Excludes HTTP headers and on-demand survey recordings."
      : "Network payload using encoded response-body sizes, including compressed responses. Excludes HTTP headers; cached assets count at their download size. Excludes on-demand survey recordings.";
    hud.coverage.textContent = `${manifest.tiles.length} L13 tiles · ${route.skipped.length} without a playable intersection`;
    hud.restartBtn.hidden = trial.status !== "finished";
    hud.progress.textContent = `${trial.collected.size}/${trial.targets.length} emojis`;
    hud.progressBar.max = trial.targets.length;
    hud.progressBar.value = trial.collected.size;
    hud.target.textContent = nearest
      ? `${nearest.emoji} ${nearest.names.join(" & ")} · ${Math.round(Math.hypot(nearest.x - car.x, nearest.y - car.y))} m`
      : "All intersection emojis collected!";
    hud.target.title = nearest ? `Tile ${nearest.t}` : "";
    const best = records();
    hud.best.textContent = best.length
      ? `Personal best ${formatTime(best[0].seconds)} · ${selectedStart.name}`
      : "Personal best — · saved on this device";
    hud.records.textContent = best
      .map((r, i) => `${i + 1}. ${formatTime(r.seconds)}`)
      .join("   ");
    if (crash) hud.status.textContent = `Hit a ${crash.kind} · recovering`;
    else if (performance.now() > messageUntil)
      hud.status.textContent =
        trial.status === "ready"
          ? "Drive to start · collect the marked emojis in any order"
          : trial.status === "finished"
            ? `Finished! ${trial.crashes} crashes · try to beat your time`
            : `${trial.crashes} crashes · R returns to your last intersection`;
  }
  function frame(now) {
    requestAnimationFrame(frame);
    const wallDt = Math.max(0, (now - last) / 1000);
    const dt = Math.min(0.05, wallDt);
    last = now;
    if (playback) {
      replayFrame(wallDt, now);
      return;
    }
    if (solo) {
      soloFrame(dt, now);
      return;
    }
    if (crash && now >= crashUntil) respawn(true);
    hud.respawnBtn.disabled = !!crash;
    let input = drivingInput(state.keys, state.touch);
    if (
      emojiLocked &&
      (input.throttle || input.steer || input.brake || crash || hud.menu.open)
    )
      clearEmojiFocus();
    if (insurance.mode !== "manual") {
      if (input.throttle || input.steer || input.brake) requestTakeover();
      input = { throttle: 0, steer: 0, brake: false };
    }
    const tiles = [...loaded.values()].map((e) => e.tile);
    const indexes = [...loaded.values()].map((e) => e.collisions);
    let surface = roadSurfaces.get(car.surfaceId);
    const roadDrive = isRoadReview(selectedStart);
    const terrainHeight = (x, y) => groundAtWorld(tiles, x, y, true);
    const heightAt = surface?.heightAt || terrainHeight;
    const crashSupport = crashRoadSupport(
      terrainHeight,
      roadSurfaces,
      surface?.kind === "tunnel" ? surface : undefined,
    );
    if (!grounded) {
      let z0 = heightAt(car.x, car.y);
      const owner = tileAt(manifest.tiles, car.x, car.y).t;
      const nearby = tilesAround(
        manifest.tiles,
        car.x,
        car.y,
        gameDrive && roadDrive ? 120 : undefined,
      ).want;
      const settled =
        loaded.has(owner) &&
        (!navigator.onLine ||
          nearby.some((token) => failures.has(token)) ||
          nearby.every((token) => loaded.has(token)));
      if (settled && !roadDrive && (!checkpoint || z0 === null)) {
        const nearest = safeIntersections().sort(
          (a, b) =>
            Math.hypot(a.x - car.x, a.y - car.y) -
            Math.hypot(b.x - car.x, b.y - car.y),
        )[0];
        if (
          nearest &&
          (checkpoint || Math.hypot(nearest.x - car.x, nearest.y - car.y) < 200)
        ) {
          const heading =
            nearest.heading +
            (Math.cos(car.heading - nearest.heading) < 0 ? Math.PI : 0);
          car = { ...nearest, heading, v: 0, steering: 0 };
          checkpoint = { ...car };
          camInit = false;
          z0 = heightAt(car.x, car.y);
        } else if (checkpoint && z0 === null) {
          // A tile revision may remove contact at a saved intersection.
          checkpoint = null;
          car = startingCar(selectedStart);
          camInit = false;
          stream(car);
        }
      }
      if (z0 !== null && settled) {
        grounded = true;
        car.z = z0;
        car.grounded = true;
      }
    }
    if (now - roadAt > 150) {
      road = nearestRoad(tiles, car.x, car.y);
      roadAt = now;
    }
    let blocked = false;
    let waitingForRoad = false;
    if (grounded && !crash && now >= crashUntil) {
      if (
        resumeRecovery &&
        car.grounded &&
        !cityDriveCanResume(gameDrive, car)
      ) {
        recoverCityDrive();
        return;
      }
      accumulator += dt;
      while (accumulator >= 1 / 120) {
        accumulator -= 1 / 120;
        const assisted =
          insurance.mode === "automatic" && gameDrive
            ? cityDriveControls(gameDrive, car)
            : null;
        if (assisted?.finished && !completedCityDrive) {
          completedCityDrive = true;
          resumeRecovery = false;
          updateDriveMode();
        }
        const controls =
          assisted ||
          ((surface ? !car.v : trial.status === "ready") && input.throttle === 0
            ? { ...input, brake: true }
            : input);
        let candidate = bakedPortals.size
          ? driveThroughRoadPortal(
              car,
              controls,
              1 / 120,
              surface
                ? [bakedPortals.get(surface.id) || []]
                : bakedPortals.values(),
              terrainHeight,
            )
          : null;
        if (!candidate && car.leavingRoad) {
          candidate = fallFromRoad(car, 1 / 120, (x, y) =>
            crashSupport(x, y, car),
          );
        }
        if (!candidate && !surface) {
          candidate = contactStep(
            car,
            controls,
            1 / 120,
            (x, y) => ({
              height: terrainHeight(x, y),
              drivable: !!road && road.distance <= road.width,
            }),
            1.35,
          );
        } else if (!candidate && bakedRoads.has(surface.id)) {
          candidate =
            strikeHighwayRail(car, controls, 1 / 120, surface) ||
            driveOnRoadNetwork(
              car,
              controls,
              1 / 120,
              surface,
              roadSurfaces,
              bakedNetwork,
              (token) => {
                if (!byToken.has(token) || loaded.has(token)) return false;
                stream(car, [token]);
                return !failures.has(token);
              },
            );
          if (
            candidate.blocked &&
            !candidate.waiting &&
            surface.kind !== "tunnel"
          )
            candidate =
              leaveRoadSide(car, controls, 1 / 120, surface) || candidate;
        } else if (!candidate && network) {
          candidate = driveOnCityRoute(
            car,
            controls,
            1 / 120,
            surface,
            roadSurfaces,
            network,
            insurance.mode === "manual" || resumeRecovery
              ? terrainHeight
              : undefined,
          );
        } else if (!candidate) {
          candidate = driveOnRoadSurface(car, controls, 1 / 120, surface);
          if (candidate.blocked && surface.kind !== "ground")
            candidate =
              leaveRoadSide(car, controls, 1 / 120, surface) || candidate;
        }
        const owner = tileAt(manifest.tiles, candidate.x, candidate.y);
        if (!loaded.has(owner.t) && car.grounded === false) {
          waitingForRoad = true;
          accumulator = 0;
          announce("Waiting for ground below to load…", 1);
          break;
        }
        if (candidate.blocked || !loaded.has(owner.t)) {
          blocked = true;
          waitingForRoad = !!candidate.waiting || !loaded.has(owner.t);
          if (resumeRecovery && !waitingForRoad && !completedCityDrive) {
            recoverCityDrive();
            return;
          }
          // A streaming pause holds the last supported pose and momentum.
          // Only a confirmed end of support stops the car.
          if (!waitingForRoad)
            car = {
              ...car,
              v: 0,
              slip: 0,
              vx: 0,
              vy: 0,
              vz: 0,
              lateralAcceleration: 0,
            };
          accumulator = 0;
          announce(
            (!navigator.onLine || failures.has(owner.t)) && !loaded.has(owner.t)
              ? "This tile is unavailable · turn back or reconnect"
              : waitingForRoad
                ? "Waiting for the road ahead to load…"
                : surface
                  ? "End of supported surface · reverse or press R"
                  : loaded.has(owner.t)
                    ? "No drivable ground · R returns to an intersection"
                    : "Waiting for the road ahead to load…",
            1,
          );
          break;
        }
        const contactHeight =
          roadSurfaces.get(candidate.surfaceId)?.heightAt || terrainHeight;
        const landing =
          car.grounded === false && hardLanding(car, candidate, crashSupport);
        const hit =
          candidate.hit ||
          obstacleContact(
            indexes,
            drivingHull(candidate),
            car.grounded === false ? drivingHull(car) : undefined,
          );
        const impact =
          candidate.roadImpact ||
          (landing && "ground") ||
          (hit && seriousImpact(candidate, hit) && hit.kind);

        if (candidate.grounded === false || car.grounded === false) {
          const takeoffRoad =
            car.grounded !== false && (!surface || surface.kind === "ground")
              ? nearestRoad(tiles, car.x, car.y)
              : null;
          airtime.step(car, landing || candidate, 1 / 120, now, {
            road:
              takeoffRoad && takeoffRoad.distance <= takeoffRoad.width
                ? takeoffRoad
                : null,
            heightAt: terrainHeight,
            deckAt: surface?.kind === "ground" ? null : surface?.heightAt,
            impact,
            tiles,
            leadIn: leadIn.read,
            pose: (source) =>
              capturePose(
                source,
                roadSurfaces.get(source.surfaceId)?.heightAt || terrainHeight,
              ),
          });
        }
        leadIn.push(capturePose(car, surface?.heightAt || terrainHeight));
        if (!document.hidden && !hud.takeover.open)
          advanceInsurance(
            insurance,
            1 / 120,
            Math.hypot(candidate.v, candidate.slip || 0),
            gameInsuranceLimit(
              candidate,
              roadSurfaces.get(candidate.surfaceId),
            ),
          );
        if (impact) {
          // A landing candidate has already discarded its inbound vertical velocity.
          const incoming =
            impact === "ground"
              ? landing || {
                  ...car,
                  x: candidate.x,
                  y: candidate.y,
                  z: candidate.z,
                  surfaceId: candidate.surfaceId,
                  vz: car.vz - 9.81 / 120,
                }
              : candidate;
          startCrash(
            impact,
            incoming,
            impact === "ground" ? null : hit,
            contactHeight,
            now,
            crashSupport,
            indexes,
          );
          // Freeze the live state while the recorded trajectory is shown.
          if (!roadDrive) advanceTrial(trial, wallDt, car, false, false);
          return;
        }
        car = hit ? scrapeObstacle(candidate, hit) : candidate;
        if (hit) announce(`Scraped a ${hit.kind}`, 1);
        surface = roadSurfaces.get(car.surfaceId);
        if (
          assisted &&
          !completedCityDrive &&
          car.v > 2 &&
          (!routeCheckpoint || now - routeCheckpoint.at >= 3000) &&
          cityDriveOnRoute(surface, car)
        )
          routeCheckpoint = { car: { ...car }, at: now };
        if (candidate.edgeContact) {
          announce(
            candidate.edgeContact === "bounce"
              ? "Bounced off highway barrier"
              : "Scraping highway barrier",
            1.2,
          );
          car.edgeContact = "";
        }
      }
      // Remember reached, clear intersections; a crash never erases quest progress.
      if (
        !roadDrive &&
        !bakedRoads.has(car.surfaceId) &&
        car.grounded &&
        !crash &&
        !blocked &&
        now >= crashUntil
      )
        for (const e of loaded.values())
          for (const p of e.intersections) {
            if (Math.hypot(car.x - p.x, car.y - p.y) < 12) {
              const z = groundAtWorld(tiles, p.x, p.y, true);
              if (z !== null && !collisionAt(indexes, p, z)) checkpoint = p;
            }
          }
    }
    const before = trial.status;
    const found = roadDrive
      ? []
      : advanceTrial(
          trial,
          wallDt,
          car,
          grounded && !crash && now >= crashUntil && input.throttle !== 0,
          grounded &&
            car.grounded &&
            !bakedRoads.has(car.surfaceId) &&
            !crash &&
            !blocked &&
            now >= crashUntil,
        );
    for (const t of found)
      announce(
        `${t.emoji} Collected! ${trial.collected.size} / ${trial.targets.length}`,
      );
    if (found.length) saveTrialProgress();
    if (before !== "finished" && trial.status === "finished") {
      const best = records();
      best.push({ seconds: trial.elapsed, crashes: trial.crashes });
      best.sort((a, b) => a.seconds - b.seconds);
      best.splice(5);
      try {
        localStorage.setItem(recordKey(), JSON.stringify(best));
      } catch {
        /* Session records still work. */
      }
      announce(
        `All ${trial.targets.length} emojis! Finished in ${formatTime(trial.elapsed)}`,
        8,
      );
    }
    if (!roadDrive) {
      hud.timer.textContent = formatTime(trial.elapsed);
      drawSeeker();
    }
    const nearest =
      roadDrive || crash || !grounded || surface?.kind === "tunnel"
        ? null
        : nearestQuestTarget(trial, car);
    const nextEmoji = nearest && loaded.get(nearest.t)?.marker ? nearest : null;
    if (nextEmoji !== emojiTarget) {
      emojiTarget = nextEmoji;
      clearEmojiFocus();
      if (emojiTarget) {
        hud.emojiFocus.textContent = emojiTarget.emoji;
        hud.emojiFocus.setAttribute(
          "aria-label",
          `Centre emoji at ${emojiTarget.names.join(" and ")}`,
        );
      }
    }
    const emojiMarker = emojiTarget ? loaded.get(emojiTarget.t).marker : null;
    for (const [t, e] of loaded)
      if (e.marker) {
        e.marker.visible =
          !roadDrive &&
          trial.targets.some((target) => target.t === t) &&
          !trial.collected.has(t);
        e.marker.userData.sprite.visible = true;
        e.marker.userData.sprite.position.y =
          e.marker.userData.baseZ +
          (e.marker === emojiMarker ? 0 : Math.sin(now / 450) * 0.4);
      }
    surveyTerrain = tiles;
    surveyReplay.update(car, wallDt);
    beacons.material.uniforms.time.value = reducedMotion.matches
      ? 0
      : now / 1000;
    if (grounded && !crash && !document.hidden) {
      for (const [index, site] of surveySites.entries()) {
        if (Math.hypot(site.x - car.x, site.y - car.y) > BONUS_RADIUS) continue;
        const message = visitSurvey(
          insurance,
          site,
          car,
          terrainHeight(site.x, site.y),
          index,
        );
        if (message) announce(message, 3);
      }
    }
    const currentHeightAt = surface?.heightAt || terrainHeight;
    const z = car.z ?? currentHeightAt(car.x, car.y);
    if (z !== null) carZ = z;
    // Pitch the car with the ground under its axles, so it does not sink into steep hills.
    const ax = Math.cos(car.heading) * 1.5;
    const ay = Math.sin(car.heading) * 1.5;
    const zf = currentHeightAt(car.x + ax, car.y + ay) ?? carZ;
    const zr = currentHeightAt(car.x - ax, car.y - ay) ?? carZ;
    carMesh.position.set(...V(car.x, car.y, carZ));
    if (crash) {
      // Conjugating ENU by the world-to-scene basis maps the quaternion axis.
      const q = crash.rotation;
      carMesh.quaternion.set(q.x, q.z, -q.y, q.w);
    } else {
      carMesh.rotation.set(
        car.grounded === false ? car.airAttitude?.roll || 0 : 0,
        car.heading,
        car.grounded === false
          ? car.airAttitude?.pitch || 0
          : Math.atan2(zf - zr, 3),
        "YZX",
      );
    }
    const braking =
      !crash &&
      brakeLightsOn(
        car,
        input,
        now < crashUntil || (!surface && trial.status === "ready"),
      );
    carMesh.userData.brakeMaterial.color.setHex(braking ? 0xff3038 : 0x661018);
    for (const wheel of carMesh.userData.wheels) {
      wheel.pivot.rotation.y = wheel.front ? car.steering || 0 : 0;
      wheel.tyre.rotation.z -= (car.v * dt) / 0.36;
    }

    const hx = Math.cos(car.heading);
    const hy = Math.sin(car.heading);
    cameraRoll =
      state.view === "incar" && camInit && !reducedMotion.matches
        ? incarRoll(cameraRoll, car.lateralAcceleration || 0, dt)
        : 0;
    // Keep the horizon and view direction steady while the chassis tumbles.
    if (surface) cameraCoveredRoad = surface.coveredRange ? surface : null;
    const crossingBlend = coveredRoadBlend(cameraCoveredRoad, car);
    if (!surface && crossingBlend === 0) cameraCoveredRoad = null;
    const coveredBlend = state.view === "zoomed" ? crossingBlend : 0;
    const view = driveCameraPose(
      surface?.kind === "tunnel"
        ? "incar"
        : crash && state.view === "incar"
          ? "zoomed"
          : state.view,
      crash ? { ...car, heading: crash.initialHeading } : car,
      carZ,
      currentHeightAt,
      cameraRoll,
      shoreline && skylineTiles.size && surface?.id === "bay-bridge-westbound"
        ? 8 *
            THREE.MathUtils.smoothstep(
              Math.hypot(car.x - shoreline[0], car.y - shoreline[1]),
              350,
              1150,
            )
        : 0,
      coveredBlend,
    );
    const target = new THREE.Vector3(...V(...view.position));
    const look = new THREE.Vector3(...V(...view.look));
    // Snap rather than glide after a jump, before the ground loads, or when far behind (a slow frame rate
    // on a hill would otherwise leave the camera under the slope); the glide rate does not depend on frame rate.
    if (
      view.attached ||
      !camInit ||
      !grounded ||
      camPos.distanceTo(target) > 40
    ) {
      camPos.copy(target);
      camLook.copy(look);
      camInit = true;
    } else {
      camPos.lerp(target, 1 - Math.pow(0.88, dt * 60));
      camLook.lerp(look, 1 - Math.pow(0.8, dt * 60));
    }
    camera.position.copy(camPos);
    camera.up.set(...V(...view.up));
    camera.lookAt(
      emojiLocked && emojiMarker
        ? emojiMarker.userData.sprite.position
        : camLook,
    );
    drawEmojiFocus(emojiMarker);

    renderView(view);
    airtime.draw(now);

    if (now - streamAt > 400) {
      streamAt = now;
      stream();
    }
    if (now - hudAt > 250) {
      hudAt = now;
      const layered = bakedRoads.has(car.surfaceId);
      const street = layered
        ? surface?.label
        : streetAt(tiles, car.x, car.y, car.heading);
      const next = layered
        ? null
        : nextCrossing(tiles, car.x, car.y, car.heading, street);
      hud.street.textContent = roadDrive
        ? car.leavingRoad
          ? "Airborne · road edge"
          : !surface
            ? selectedStart.rampApproach
              ? selectedStart.name
              : "City terrain"
            : surface.label ||
              (surface.id === selectedStart.surfaceId
                ? selectedStart.name
                : surface.kind.replaceAll("_", " "))
        : street
          ? street.replace(/^Unnamed \d+$/, "Unnamed street")
          : "Off street";
      hud.next.textContent = roadDrive
        ? `${carZ.toFixed(1)} m NAVD88 · ${car.leavingRoad ? "Airborne" : surface ? "Game route" : "Base map"}`
        : layered
          ? `${carZ.toFixed(1)} m NAVD88 · ${surface.kind.replaceAll("_", " ")}`
          : next
            ? `${next.names.join(" · ")} in ${Math.round(next.distance / 10) * 10} m`
            : "";
      if (crash) hud.next.textContent = "Crash · recovering";
      else if (trial.status === "finished")
        hud.next.textContent = `Finished! ${trial.crashes} crashes`;
      else if (completedCityDrive)
        hud.next.textContent = deliveryDrive()
          ? `Route complete${selectedStart.id === BRIDGE_SURVEY_DRIVE ? " · back on I-80" : ""} · ${insurance.delivered}/${insurance.stops.length} deliveries`
          : "Route complete · back on I-80";
      else if (now < messageUntil)
        hud.next.textContent = hud.status.textContent;
      hud.next.title = hud.next.textContent;
      hud.driveBar.dataset.crash = String(!!crash);
      hud.speed.textContent = grounded
        ? Math.round(Math.abs(car.v) * 2.23694)
        : 0;
      hud.insuranceHud.hidden = false;
      updateInsuranceMeter();
      const nextBonus = insurance.free || insurance.discount;
      const activeBonus = insurance.free > 0 ? "Free" : "Half rate";
      hud.insuranceBonus.textContent = nextBonus
        ? `${activeBonus} · ${Math.ceil(nextBonus)} s left`
        : "";
      const nextStop = surveySites.find(
        (site) => site.id === insurance.stops[insurance.delivered],
      );
      hud.insuranceDeliveries.textContent = insurance.stops.length
        ? `${insurance.delivered}/${insurance.stops.length} deliveries${nextStop ? ` · ${nextStop.title}` : ""}`
        : "";
      hud.insuranceInfractions.textContent = `${insurance.infractions} infractions`;
      const deg = (((90 - (car.heading * 180) / Math.PI) % 360) + 360) % 360;
      hud.compass.textContent = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"][
        Math.round(deg / 45) % 8
      ];
      if (!grounded) hud.street.textContent = "Loading the city around you…";
      if (!grounded && !navigator.onLine)
        hud.next.textContent = "Offline · only downloaded tiles are available";
      else if (!grounded && failures.size)
        hud.next.textContent = "Waiting for an unavailable city tile";
      if (
        !roadDrive &&
        trial.status === "running" &&
        Math.floor(now / 5000) !== Math.floor((now - wallDt * 1000) / 5000)
      )
        saveTrialProgress();
      if (!roadDrive) updateTrialHud();
      drawMinimap(hud.menu.open ? hud.minimap : hud.desktopMap);
      canvas.dataset.contact = !grounded
        ? "loading"
        : car.grounded
          ? "grounded"
          : "airborne";
      canvas.dataset.altitude = carZ.toFixed(3);
      canvas.dataset.surface = surface?.kind || "city";
      canvas.dataset.cameraContact =
        surface?.kind === "tunnel"
          ? "tunnel"
          : coveredBlend > 0
            ? "covered"
            : "exterior";
      canvas.dataset.surfaceId = surface?.id || "";
      canvas.dataset.routeComplete = String(completedCityDrive);
      canvas.dataset.roadWait = String(waitingForRoad);
      canvas.dataset.speedMps = car.v.toFixed(3);
      canvas.dataset.heading = car.heading.toFixed(4);
      canvas.dataset.roadBlocked = String(blocked && !waitingForRoad);
      canvas.dataset.east = car.x.toFixed(3);
      canvas.dataset.north = car.y.toFixed(3);
    }
  }
  if (gameDrive) {
    for (const token of BAY_SKYLINE_TILES) {
      if (!byToken.has(token)) continue;
      const request = fetchJsonGz(`${tileBase}${token}.json.gz`);
      skylineRequests.set(token, request);
      request
        .then((tile) => {
          // Match the local route's structure exceptions without mutating a
          // decoded tile that may already be shared with the local streamer.
          const displayTile = { ...tile };
          applyCityDriveScenery(
            displayTile,
            gameDrive.sceneExclusions?.[token],
          );
          const buildings = new THREE.Group();
          const material = addBuildings(buildings, displayTile, C, null, true);
          if (!buildings.children.length) return;
          skylineBuildings.add(buildings);
          skylineTiles.set(token, { buildings, material });
          canvas.dataset.skylineTiles = String(skylineTiles.size);
        })
        .catch((error) => {
          skylineRequests.delete(token);
          console.warn("Skyline tile", token, error);
        });
    }
  }
  const pendingReplay = params.get("replay");
  if (pendingReplay) {
    const { crashes, airtimes } = library.read();
    const clip = [...crashes, ...airtimes].find(
      (row) => row.id === pendingReplay && row.mode === replayMode(),
    );
    params.delete("replay");
    history.replaceState(
      null,
      "",
      params.size ? `?${params}` : location.pathname,
    );
    if (clip) startPlayback(clip, false);
  }
  if (!solo && !playback) stream();
  requestAnimationFrame(frame);
}
