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
  beginCrash,
  brakeLightsOn,
  buildCollisionIndex,
  collisionAt,
  createTrial,
  crashPose,
  formatBytes,
  formatTime,
  groundAt,
  intersectionsForTile,
  nearestRoad,
  questSeeker,
  tileAt,
  groundAtWorld,
  nextCrossing,
  streetAt,
  tilesAround,
} from "./sf-drive-core.js";

import { terrainTriangles, tileRegion } from "./sf-drive-surface.js";
import { createDownloadSizeTracker } from "./sf-download-size.js";
import { bindDrivingPad, drivingInput } from "./sf-touch-controls.js";
import { districtTrials, seekerDirection } from "./sf-district-trials.js";
import {
  DRIVE_VIEWS,
  DRIVE_VIEW_RANGE,
  MIRROR_VIEW_RANGE,
  driveCameraPose,
  incarRoll,
} from "./sf-drive-camera.js";
import { createCarReveal } from "./sf-car-reveal.js";
import { roadDashPolygons } from "./sf-road-markings.js";
import { createTerrainPainter } from "./sf-terrain-painter.js";
import { SCENE_COLOURS } from "./scene-colours.js";
import { mountSurveyReplay } from "./sf-survey-replay.js";
import {
  fetchJsonGz,
  loadRoadOverlay,
  openRoadRegion,
  readGameManifest,
} from "./sf-drive-assets.js";
import {
  BAY_BRIDGE_OVERLAY,
  REGIONAL_ROAD_CATALOG,
  mapSources,
} from "./sf-drive-settings.js";
import {
  BAY_BRIDGE_BASE,
  bayBridgeStarts,
  contactGeometry,
  driveOnRoadSurface,
  indexRoadContact,
} from "./sf-road-contact.js";
import {
  driveOnRoadNetwork,
  indexRoadNetwork,
  regionalDrives,
  regionalStart,
} from "./sf-road-network.js";
import { buildingWalls, validBuildingFace } from "./sf-building-walls.js";
import { createAirtimeHud } from "./sf-airtime.js";
import {
  fallFromRoad,
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
} from "./sf-highway-edge.js";

const css = (n) =>
  getComputedStyle(document.documentElement).getPropertyValue(n).trim();
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

function addBuildings(group, tile, C, reveal) {
  const [ox, oy] = tile.o;
  // Keep each building whole, but batch nearby buildings so the camera
  // rejects distant blocks before submitting their triangles and outlines.
  const batchSize = 1600; // decimetres; tile coordinates are local
  const batches = new Map();
  for (const b of tile.b) {
    const validFaces = b[1].filter(validBuildingFace);
    const firstRing = validFaces[0]?.[0];
    if (!firstRing?.length) continue;
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
      exposedWalls.push({ a, b, za, zb });
    });
    appendBuildingWallCorners(outlines, exposedWalls);
  }
  if (!batches.size) return;
  const facesMaterial = new THREE.MeshLambertMaterial({
    vertexColors: true,
    side: THREE.DoubleSide,
    transparent: true,
    opacity: 0.28,
    depthWrite: false,
    forceSinglePass: true,
  });
  const outlineMaterial = new THREE.LineBasicMaterial({
    color: SCENE_COLOURS.buildingOutline,
    opacity: 0.36,
    transparent: true,
    depthWrite: false,
  });
  reveal.enableOn(facesMaterial);
  reveal.enableOn(outlineMaterial);
  for (const { positions, colours, outlines } of batches.values()) {
    if (!positions.length) continue;
    const faces = new THREE.BufferGeometry();
    faces.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(positions, 3),
    );
    faces.setAttribute("color", new THREE.Float32BufferAttribute(colours, 3));
    faces.computeVertexNormals();
    const buildings = new THREE.Mesh(faces, facesMaterial);
    buildings.renderOrder = -2;
    const edges = new THREE.BufferGeometry();
    edges.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(outlines, 3),
    );
    const lines = new THREE.LineSegments(edges, outlineMaterial);
    lines.renderOrder = -1;
    group.add(buildings, lines);
  }
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
    tile.tr.forEach(([x, y, h, r], i) => {
      const wx = ox + x / 10;
      const wy = oy + y / 10;
      const z = gz(x / 10, y / 10);
      const ch = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
      p.set(...V(wx, wy, z + h - ch / 2));
      s.set(r, ch / 2, r);
      mtx.compose(p, qn, s);
      crown.setMatrixAt(i, mtx);
      const tw = Math.max(0.12, 0.04 * h);
      p.set(...V(wx, wy, z + (h - ch) / 2));
      s.set(tw, Math.max(0.5, h - ch), tw);
      mtx.compose(p, qn, s);
      trunk.setMatrixAt(i, mtx);
    });
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
  sources = {},
  hud,
  downloadAssets,
  surveySites = [],
}) {
  const params = new URLSearchParams(location.search);
  const at = params.get("at");
  const region = params.get("region");
  const reviewDrive = params.get("review");
  const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(
    location.hostname,
  );
  const pilotRequested = ["bay-bridge-deck", "bay-bridge-street"].includes(at);
  const settings = mapSources(sources);
  const tileBase = settings.tiles || base;
  const manifestUrl = new URL(`${tileBase}manifest.json`, location.href);
  const overlayUrl = region
    ? null
    : settings.overlay ||
      (loopback && pilotRequested ? BAY_BRIDGE_OVERLAY : null);
  const catalogUrl =
    settings.catalog || (loopback ? REGIONAL_ROAD_CATALOG : null);
  let overlayError = "";
  const [product, route, coverage] = await Promise.all([
    overlayUrl || (region && catalogUrl)
      ? Promise.resolve()
          .then(() => {
            const signal = AbortSignal.timeout(8000);
            return region
              ? openRoadRegion(new URL(catalogUrl), region, manifestUrl, signal)
              : loadRoadOverlay(new URL(overlayUrl), manifestUrl, signal);
          })
          .catch((error) => {
            overlayError = error.message;
            return null;
          })
      : null,
    fetchJsonGz(`${base}../quest-route.json`),
    fetchJsonGz(`${base}../coverage.json`),
  ]);
  if (product && product.overlay.base_manifest_sha256 !== route.manifestSha256)
    throw new Error(
      "This tile server needs a manifest matching the game's bundled routes.",
    );
  const manifest =
    product?.base ||
    (settings.tiles
      ? await readGameManifest(
          manifestUrl,
          route.manifestSha256,
          AbortSignal.timeout(8000),
        )
      : await fetchJsonGz(manifestUrl));
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
  let activeDistrict = null;
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
  const airtime = createAirtimeHud(
    hud.airtime,
    hud.airtimeReadout,
    hud.airtimePlot,
  );

  const roadSurfaces = new Map();
  const roadMeshes = new THREE.Group();
  scene.add(roadMeshes);
  const review = product?.review;
  const network = review && indexRoadNetwork(review.confirmed_connections);
  const ownerLoads = new Map();
  const roadMaterial = new THREE.MeshLambertMaterial({
    color: C.asphalt,
    side: THREE.DoubleSide,
    polygonOffset: true,
    polygonOffsetFactor: -1,
    polygonOffsetUnits: -1,
  });
  function addRoadTile(tile) {
    for (const surface of tile.surfaces) {
      if (!surface.contact || roadSurfaces.has(surface.id)) continue;
      const driveSurface = {
        ...surface,
        heightAt: indexRoadContact(surface.contact),
        boundary: roadBoundary(surface.contact.vertices),
        // This visual and physical rail is authored for the highway pilot.
        highwayRails:
          product.overlay.base_manifest_sha256 === BAY_BRIDGE_BASE &&
          surface.id === "gms:4YJKKX79HSSS0CF5P5H165ERHB",
      };
      driveSurface.railSegments = highwayRailSegments(driveSurface);
      roadSurfaces.set(surface.id, driveSurface);
      roadMeshes.add(
        new THREE.Mesh(contactGeometry(surface.contact, false), roadMaterial),
      );
      if (driveSurface.railSegments.length) {
        const vertices = driveSurface.railSegments.flatMap(({ a, b }) => [
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
        roadMeshes.add(
          new THREE.Mesh(
            geometry,
            new THREE.MeshLambertMaterial({
              color: 0xadb5ba,
              side: THREE.DoubleSide,
            }),
          ),
        );
      }
    }
    network?.add(tile);
  }
  function requestRoadOwner(owner) {
    if (!product?.overlay.tiles[owner]) return false;
    if (!ownerLoads.has(owner)) {
      const state = { pending: true };
      ownerLoads.set(owner, state);
      state.done = product
        .loadOwner(owner, AbortSignal.timeout(30000))
        .then(addRoadTile)
        .catch((error) => {
          hud.roadReviewStatus.textContent = `Road tile ${owner} unavailable · ${error.message}`;
        })
        .finally(() => {
          state.pending = false;
        });
    }
    return ownerLoads.get(owner).pending;
  }
  if (!review && product?.overlay.pilot === "bay-bridge-approach")
    for (const tile of product.tiles.values()) addRoadTile(tile);
  const drives = review ? regionalDrives(review) : [];
  const bridgeRegion = region === "embarcadero-bryant";
  if (review && bridgeRegion) {
    const owner = drives[0]?.start.owner;
    requestRoadOwner(owner);
    await ownerLoads.get(owner)?.done;
  }
  const bridgeStarts =
    !region || bridgeRegion ? bayBridgeStarts(roadSurfaces) : [];
  let reviewStart = bridgeStarts.find((p) => p.id === reviewDrive);
  let reviewError = "";
  if (review && !reviewStart) {
    const choices = reviewDrive
      ? drives.filter((drive) => drive.id === reviewDrive)
      : drives;
    if (!reviewDrive) reviewStart = bridgeStarts[0];
    for (const drive of choices) {
      if (reviewStart) break;
      const { owner, surface_id } = drive.start;
      requestRoadOwner(owner);
      await ownerLoads.get(owner)?.done;
      reviewStart = regionalStart(drive, roadSurfaces.get(surface_id));
    }
    if (!reviewStart)
      reviewError =
        "This review drive has no verified start that fits the car.";
  }
  const places = [...(manifest.places || []), ...bridgeStarts];
  if (reviewStart && !places.some((p) => p.id === reviewStart.id))
    places.push(reviewStart);
  const start = places.find((p) => p.id === at) || reviewStart || places[0];
  hud.regionalLink.hidden = !catalogUrl || !!review;
  if (review) {
    const leaveReview = (region, drive) => {
      if (region) params.set("region", region);
      else params.delete("region");
      if (drive) params.set("review", drive);
      else params.delete("review");
      params.delete("at");
      location.assign(`?${params}`);
    };
    hud.regionalReview.hidden = false;
    hud.roadRegion.replaceChildren(
      new Option("City map", ""),
      ...product.catalog.regions
        .filter((entry) => entry.state === "review")
        .map(
          (entry) =>
            new Option(entry.region.replaceAll("-", " "), entry.region),
        ),
    );
    hud.roadRegion.value = region;
    hud.roadRegion.addEventListener("change", () =>
      leaveReview(hud.roadRegion.value),
    );
    hud.roadReviewDrive.replaceChildren(
      ...[...bridgeStarts, ...drives].map(
        (drive) => new Option(drive.name, drive.id),
      ),
    );
    hud.roadReviewDrive.value = reviewDrive || reviewStart?.id || "";
    hud.roadReviewDrive.addEventListener("change", () =>
      leaveReview(region, hud.roadReviewDrive.value),
    );
    hud.roadReviewStatus.textContent =
      reviewError ||
      `Acceptance pending · ${review.confirmed_connections.length} confirmed · ${review.unresolved_connections.length} unresolved`;
  }
  const startingCar = ({ x, y, heading, surfaceId }) => ({
    x,
    y,
    heading,
    surfaceId,
    v: 0,
  });
  let car = startingCar(start);
  let carZ = 0;
  // The car waits until the ground under it has loaded, so it never starts in empty space.
  let grounded = false;
  const loaded = new Map(); // token -> {tile, grp}
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
  let cityCar = null;
  let cityPauseAt = 0;
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
  let trial = createTrial(route.targets);
  let selectedStart = start;
  let cityStart = start;
  let checkpoint = null;
  let crash = null;
  let crashUntil = 0;
  let accumulator = 0;
  let messageUntil = 0;
  const failures = new Map();
  const recordKey = () =>
    `sf-trial-v2:${route.manifestSha256}:${selectedStart.id}${activeDistrict ? `:district:${activeDistrict.recordScope}` : ""}`;
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
  function updateBridgeMode() {
    const active = !!selectedStart.surfaceId;
    document.body.dataset.bridge = String(active);
    hud.restartBtn.textContent = active ? "Restart drive" : "New trial";
    if (active) {
      hud.timer.textContent = review ? "Review" : "Pilot";
      hud.progress.textContent = review ? "Regional road" : "Measured approach";
    }
  }
  updateBridgeMode();
  hud.bridgeLink.hidden = !loopback || bridgeStarts.length > 0 || !!region;
  if (overlayError || reviewError || (region && !catalogUrl))
    announce(
      reviewError ||
        `${region ? "Road review" : "Bay Bridge"} unavailable · ${overlayError || "Set a Gerardus server in Map sources"}`,
      10,
    );
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

  function stream() {
    const { want, drop } = tilesAround(manifest.tiles, car.x, car.y);
    for (const t of drop) {
      const e = loaded.get(t);
      if (!e) continue;
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
      fetchJsonGz(`${tileBase}${t}.json.gz`)
        .then((tile) => {
          // The pinned base classified this bridge approach as one solid 3DEP-only building.
          // Gerardus supplies both road levels in its place; keep all other buildings.
          if (
            bridgeStarts.length &&
            t === "80858074" &&
            product.overlay.base_manifest_sha256 === BAY_BRIDGE_BASE
          ) {
            const inferredBridge = tile.b[42];
            if (
              inferredBridge?.[2] === 1 &&
              inferredBridge[0] === 32 &&
              inferredBridge[1].length === 195
            )
              tile.b.splice(42, 1);
          }
          tile.half = byToken.get(t).half;
          const grp = buildTile(tile, C, regions.get(t), reveal);
          scene.add(grp);
          const roads = roadShapes(tile);
          const entry = {
            tile,
            grp,
            roads,
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
            collisions: buildCollisionIndex(tile),
            intersections: intersectionsForTile(tile),
          };
          loaded.set(t, entry);
          const target = route.targets.find((q) => q.t === t);
          if (target) {
            entry.marker = markerFor(target);
            grp.add(entry.marker);
          }
          terrainPainter.add(t, entry);
          failures.delete(t);
        })
        .catch((e) => {
          failures.set(t, performance.now() + 5000);
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
    if (hud.menu.open) return;
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
    hud.viewMode.value = view;
    hud.mirrorBtn.hidden = view !== "incar";
    hud.mirrorFrame.hidden = view !== "incar" || !state.mirror;
    canvas.dataset.view = view;
    canvas.setAttribute(
      "aria-label",
      `${hud.viewMode.selectedOptions[0].textContent} 3D view of ${solo ? "the authored practice course" : "San Francisco"}`,
    );
  };
  const toggleView = () =>
    setView(
      DRIVE_VIEWS[(DRIVE_VIEWS.indexOf(state.view) + 1) % DRIVE_VIEWS.length],
    );
  const reset = (p = selectedStart) => {
    car = startingCar(p);
    grounded = false;
    camInit = false;
    accumulator = 0;
    crash = null;
    airtime.reset();
    updateBridgeMode();
    releaseInputs();
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
  function respawn(afterCrash = false) {
    if (solo) {
      restartSolo();
      return;
    }
    if (crash && !afterCrash) return;
    const kind = crash?.kind;
    if (selectedStart.surfaceId) {
      reset();
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
    reset({ ...point, heading });
    checkpoint = { ...point, heading };
    crashUntil = performance.now() + (afterCrash ? 0 : 750);
    announce(
      kind
        ? `Hit a ${kind} · back at the intersection`
        : "Back at the intersection · clock continues",
    );
  }
  function startCrash(kind, now) {
    if (crash) return;
    airtime.reset();
    crash = beginCrash(car, kind, now / 1000);
    car = { ...car, v: 0, steering: 0, lateralAcceleration: 0 };
    accumulator = 0;
    releaseInputs();
    if (trial.status === "running") trial.crashes++;
    const impact =
      kind === "ground"
        ? "Hard landing"
        : kind === "fall"
          ? "Fell off the road"
          : `Hit a ${kind}`;
    announce(`${impact} · spinning before reset`, 2);
  }
  function newTrial(p = selectedStart) {
    if (solo) {
      restartSolo();
      return;
    }
    selectedStart = p;
    trial = createTrial(activeDistrict?.targets || route.targets);
    checkpoint = null;
    crashUntil = 0;
    reset(p);
    announce(
      p.surfaceId
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
  hud.viewMode.addEventListener("change", () => {
    setView(hud.viewMode.value);
    hud.viewMode.blur();
  });
  hud.menuBtn.addEventListener("click", releaseInputs);
  hud.restartBtn.addEventListener("click", (e) => {
    newTrial();
    releaseFocus(e);
  });
  hud.respawnBtn.addEventListener("click", (e) => {
    respawn();
    releaseFocus(e);
  });
  hud.places.replaceChildren(...places.map((p) => new Option(p.name, p.id)));
  hud.places.value = start.id;
  hud.places.addEventListener("change", () => {
    const p = places.find((q) => q.id === hud.places.value);
    if (p) {
      cityStart = p;
      newTrial(p);
      params.set("at", p.id);
      if (overlayUrl) params.set("overlay", overlayUrl);
      history.replaceState(null, "", `?${params}`);
    }
    hud.places.blur();
  });
  hud.trialArea.replaceChildren(
    new Option("Whole city", "city"),
    ...districts.map((d) => new Option(d.name, d.id)),
  );
  if (!districts.length) {
    const unavailable = new Option(
      "District runs unavailable for this map",
      "unavailable",
    );
    unavailable.disabled = true;
    hud.trialArea.add(unavailable);
  }
  hud.trialArea.addEventListener("change", () => {
    activeDistrict =
      districts.find((d) => d.id === hud.trialArea.value) || null;
    hud.places.disabled = !!activeDistrict;
    hud.places.replaceChildren(
      ...(activeDistrict ? [activeDistrict.start] : places).map(
        (p) => new Option(p.name, p.id),
      ),
    );
    const p = activeDistrict?.start || cityStart;
    hud.places.value = p.id;
    hud.trialTitle.textContent = activeDistrict
      ? `${activeDistrict.name} emoji rally`
      : "San Francisco emoji rally";
    newTrial(p);
    hud.trialArea.blur();
  });
  drivingPad = bindDrivingPad(hud.touchButtons, (input) => {
    state.touch = input;
  });

  function restartSolo() {
    endSoloRun(soloRun, "aborted");
    hud.soloPrevious.textContent = soloSummary(soloRun);
    soloRun = createSoloRun();
    car = soloCar();
    airtime.reset();
    carZ = car.z;
    accumulator = 0;
    releaseInputs();
    camInit = false;
    hud.soloSummary.textContent = soloSummary(soloRun);
  }
  hud.gameMode.addEventListener("change", () => {
    releaseInputs();
    airtime.reset();
    solo = hud.gameMode.value === "solo";
    document.body.dataset.solo = String(solo);
    practice.visible = solo;
    roadMeshes.visible = !solo;
    if (solo) {
      surveyReplay.update(null, 0);
      cityCar = { car, carZ, grounded };
      cityPauseAt = performance.now();
      restartSolo();
    } else {
      endSoloRun(soloRun, "aborted");
      ({ car, carZ, grounded } = cityCar);
      if (trial.status === "running")
        trial.elapsed += (performance.now() - cityPauseAt) / 1000;
      for (const e of loaded.values()) e.grp.visible = true;
      accumulator = 0;
      camInit = false;
      updateBridgeMode();
    }
    if (solo) hud.restartBtn.textContent = "New solo run";
    hud.respawnBtn.textContent = solo ? "Restart solo route" : "Reset car";
    hud.respawnBtn.disabled = false;
    hud.mirrorFrame.hidden = state.view !== "incar" || !state.mirror;
    hud.timer.setAttribute(
      "aria-label",
      solo ? "Elapsed simulation time, no time reward" : "Elapsed trial time",
    );
    setView(state.view);
    soloHudAt = 0;
    hud.gameMode.blur();
  });
  function renderView(view) {
    const hx = Math.cos(car.heading),
      hy = Math.sin(car.heading);
    const W = renderer.domElement.width;
    const H = renderer.domElement.height;
    const pr = renderer.getPixelRatio();
    renderer.setScissorTest(false);
    renderer.setViewport(0, 0, W / pr, H / pr);
    carMesh.visible = !view.hideCar;
    if (carMesh.visible)
      reveal.update(camera, carMesh.position, car.heading, W, H, pr);
    else reveal.disable();
    renderer.render(scene, camera);
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
        airtime.step(
          before,
          car,
          1 / 120,
          {
            centreline: [min * 10, 0, max * 10, 0],
            origin: [0, 0],
            segment: 0,
            fraction: (before.x - min) / (max - min),
            heading: 0,
          },
          soloHeight,
        );
      }
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
        releaseInputs();
        hud.menu.showModal();
        hud.menuBtn.setAttribute("aria-expanded", "true");
        break;
      }
    }
    for (const e of loaded.values()) e.grp.visible = false;
    carZ = car.z;
    carMesh.position.set(...V(car.x, car.y, car.z));
    carMesh.rotation.set(0, car.heading, soloPitch(car), "YZX");
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
  let streamAt = 0;
  const camPos = new THREE.Vector3();
  const camLook = new THREE.Vector3();
  const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
  let cameraRoll = 0;
  let camInit = false;
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
  function drawMinimap(cv) {
    const w = cv.clientWidth,
      h = cv.clientHeight,
      dpr = Math.min(2, devicePixelRatio || 1);
    if (!w || !h) return; // The gameplay map is hidden on touch layouts.
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
    if (crash)
      hud.status.textContent = `Hit a ${crash.kind} · reset in ${crashPose(crash, performance.now() / 1000).remaining.toFixed(1)} s`;
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
    if (solo) {
      soloFrame(dt, now);
      return;
    }
    if (crash && crashPose(crash, now / 1000).done && now >= crashUntil)
      respawn(true);
    hud.respawnBtn.disabled = !!crash;
    const input = drivingInput(state.keys, state.touch);
    const tiles = [...loaded.values()].map((e) => e.tile);
    const indexes = [...loaded.values()].map((e) => e.collisions);
    let surface = roadSurfaces.get(car.surfaceId);
    const roadDrive = !!selectedStart.surfaceId;
    const terrainHeight = (x, y) => groundAtWorld(tiles, x, y, true);
    const heightAt = surface?.heightAt || terrainHeight;
    const landingBelow = (x, y, below) => {
      const terrain = terrainHeight(x, y);
      let landing =
        terrain !== null && terrain <= below ? { z: terrain } : null;
      for (const entry of roadSurfaces.values()) {
        const z = entry.heightAt(x, y);
        if (z !== null && z <= below && (!landing || z > landing.z))
          landing = { z, surfaceId: entry.id };
      }
      return landing;
    };
    if (!grounded) {
      const z0 = heightAt(car.x, car.y);
      const settled =
        review && surface
          ? loaded.has(tileAt(manifest.tiles, car.x, car.y).t) &&
            tilesAround(manifest.tiles, car.x, car.y, 120).want.every((t) =>
              loaded.has(t),
            )
          : loaded.size > 0 &&
            !loading.size &&
            tilesAround(manifest.tiles, car.x, car.y).want.every((t) =>
              loaded.has(t),
            );
      if (z0 !== null && settled) {
        grounded = true;
        if (!surface && !checkpoint) {
          const nearest = safeIntersections().sort(
            (a, b) =>
              Math.hypot(a.x - car.x, a.y - car.y) -
              Math.hypot(b.x - car.x, b.y - car.y),
          )[0];
          if (
            nearest &&
            Math.hypot(nearest.x - car.x, nearest.y - car.y) < 200
          ) {
            const heading =
              nearest.heading +
              (Math.cos(car.heading - nearest.heading) < 0 ? Math.PI : 0);
            car = { ...nearest, heading, v: 0, steering: 0 };
            checkpoint = { ...car };
            camInit = false;
          }
        }
        car.z = heightAt(car.x, car.y) ?? z0;
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
      accumulator += dt;
      while (accumulator >= 1 / 120) {
        accumulator -= 1 / 120;
        const controls =
          (surface ? !car.v : trial.status === "ready") && input.throttle === 0
            ? { ...input, brake: true }
            : input;
        let candidate = car.leavingRoad
          ? fallFromRoad(car, 1 / 120, landingBelow)
          : surface
            ? network
              ? driveOnRoadNetwork(
                  car,
                  controls,
                  1 / 120,
                  surface,
                  roadSurfaces,
                  network,
                  requestRoadOwner,
                )
              : driveOnRoadSurface(car, controls, 1 / 120, surface)
            : contactStep(
                car,
                controls,
                1 / 120,
                (x, y) => ({
                  height: terrainHeight(x, y),
                  drivable: !!road && road.distance <= road.width,
                }),
                1.35,
              );
        if (surface && candidate.blocked && !candidate.waiting)
          candidate =
            leaveRoadSide(car, controls, 1 / 120, surface) || candidate;
        const owner = tileAt(manifest.tiles, candidate.x, candidate.y);
        if (candidate.blocked || (!car.leavingRoad && !loaded.has(owner.t))) {
          car = { ...car, v: 0, vx: 0, vy: 0, vz: 0, lateralAcceleration: 0 };
          blocked = true;
          waitingForRoad =
            !!candidate.waiting || (!!surface && !loaded.has(owner.t));
          accumulator = 0;
          announce(
            waitingForRoad
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
        if (candidate.roadImpact) {
          car = { ...candidate, roadImpact: null };
          startCrash(candidate.roadImpact, now);
          break;
        }
        const hit = collisionAt(indexes, candidate, candidate.z);
        if (hit) {
          startCrash(hit, now);
          break;
        }
        if (candidate.grounded === false || car.grounded === false)
          airtime.step(
            car,
            candidate,
            1 / 120,
            !roadDrive && road && road.distance <= road.width ? road : null,
            roadDrive
              ? (x, y) => landingBelow(x, y, car.z)?.z ?? null
              : terrainHeight,
          );
        car = candidate;
        surface = roadSurfaces.get(car.surfaceId);
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
      if (!roadDrive && car.grounded && !crash && !blocked && now >= crashUntil)
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
          grounded && car.grounded && !crash && !blocked && now >= crashUntil,
        );
    for (const t of found)
      announce(
        `${t.emoji} Collected! ${trial.collected.size} / ${trial.targets.length}`,
      );
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
    for (const [t, e] of loaded)
      if (e.marker) {
        e.marker.visible =
          !roadDrive &&
          trial.targets.some((target) => target.t === t) &&
          !trial.collected.has(t);
        e.marker.userData.sprite.position.y =
          e.marker.userData.baseZ + Math.sin(now / 450) * 0.4;
      }
    surveyTerrain = tiles;
    surveyReplay.update(car, wallDt);
    const z = car.z ?? heightAt(car.x, car.y);
    if (z !== null) carZ = z;
    // Pitch the car with the ground under its axles, so it does not sink into steep hills.
    const ax = Math.cos(car.heading) * 1.5;
    const ay = Math.sin(car.heading) * 1.5;
    const zf = heightAt(car.x + ax, car.y + ay) ?? carZ;
    const zr = heightAt(car.x - ax, car.y - ay) ?? carZ;
    const pose = crash ? crashPose(crash, now / 1000) : null;
    carMesh.position.set(...V(car.x, car.y, carZ + (pose?.lift || 0)));
    carMesh.rotation.set(
      pose?.roll || 0,
      pose?.heading ?? car.heading,
      car.grounded === false
        ? Math.atan2(car.vz, Math.max(0.1, Math.abs(car.v)))
        : Math.atan2(zf - zr, 3),
      "YZX",
    );
    const braking = brakeLightsOn(
      car,
      input,
      !!crash || now < crashUntil || (!surface && trial.status === "ready"),
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
    const view = driveCameraPose(state.view, car, carZ, heightAt, cameraRoll);
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
    camera.lookAt(camLook);

    renderView(view);
    airtime.draw(now);

    if (now - streamAt > 400) {
      streamAt = now;
      stream();
    }
    if (now - hudAt > 250) {
      hudAt = now;
      const street = streetAt(tiles, car.x, car.y, car.heading);
      const next = nextCrossing(tiles, car.x, car.y, car.heading, street);
      hud.street.textContent = roadDrive
        ? selectedStart.name
        : street
          ? street.replace(/^Unnamed \d+$/, "Unnamed street")
          : "Off street";
      hud.next.textContent = roadDrive
        ? `${carZ.toFixed(1)} m NAVD88 · Gerardus ${review ? "review" : "approach"}`
        : next
          ? `${next.names.join(" · ")} in ${Math.round(next.distance / 10) * 10} m`
          : "";
      if (crash)
        hud.next.textContent = `Crash · reset in ${crashPose(crash, now / 1000).remaining.toFixed(1)} s`;
      else if (trial.status === "finished")
        hud.next.textContent = `Finished! ${trial.crashes} crashes`;
      else if (now < messageUntil)
        hud.next.textContent = hud.status.textContent;
      hud.next.title = hud.next.textContent;
      hud.driveBar.dataset.crash = String(!!crash);
      hud.speed.textContent = Math.round(Math.abs(car.v) * 2.23694);
      const deg = (((90 - (car.heading * 180) / Math.PI) % 360) + 360) % 360;
      hud.compass.textContent = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"][
        Math.round(deg / 45) % 8
      ];
      if (!grounded) hud.street.textContent = "Loading the city around you…";
      if (!roadDrive) updateTrialHud();
      drawMinimap(hud.menu.open ? hud.minimap : hud.desktopMap);
      canvas.dataset.contact = !grounded
        ? "loading"
        : car.grounded
          ? "grounded"
          : "airborne";
      canvas.dataset.altitude = carZ.toFixed(3);
      canvas.dataset.surface = surface?.kind || "city";
      canvas.dataset.surfaceId = surface?.id || "";
      canvas.dataset.roadWait = String(waitingForRoad);
      canvas.dataset.east = car.x.toFixed(3);
      canvas.dataset.north = car.y.toFixed(3);
    }
  }
  stream();
  requestAnimationFrame(frame);
}
