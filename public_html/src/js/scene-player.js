// Three.js player for velocity.report scene exports.
//
// Renders tracked road users as oriented boxes with client-side trails, and
// drives playback from the recorded frame timestamps. It owns rendering only;
// fetching, decoding and seeking live in scene-reader.js.

import { northOverhead, updateCompass } from "./scene-compass.js";
import { mountSceneDev } from "./scene-dev.js";
import * as THREE from "three";
import { SceneSession, SceneError } from "./scene-reader.js";
import { createSceneCamera } from "./scene-camera.js";
import { renderSceneSpeedStats } from "./scene-speed-stats.js";
import { createTimelineStrip } from "./scene-timeline.js";
import { createSceneFlight } from "./scene-flight.js";
import {
  advanceSceneClock,
  autoplayScene,
  pointCloudOverlayOpacity,
  sceneGeneratorVersions,
  sceneLoopDuration,
  sceneSourcesAlign,
} from "./scene-playback.js";
import { SCENE_COLOURS } from "./scene-colours.js";

// Sensor data is ENU: X east, Y north, Z up. Three.js is Y-up, so east stays
// X, up becomes Y, and north becomes -Z to keep the frame right-handed.
const toSceneX = (x) => x;
const toSceneY = (z) => z;
const toSceneZ = (y) => -y;

const MPS_TO_MPH = 2.236936;

/** Class colours. Anything unrecognised falls back to `default`. */
const CLASS_COLOUR = {
  // These read across the scene and timeline strip, so a colour means one thing.
  car: SCENE_COLOURS.vehicle,
  bus: SCENE_COLOURS.vehicle,
  truck: SCENE_COLOURS.vehicle,
  pedestrian: SCENE_COLOURS.walking,
  cyclist: SCENE_COLOURS.cycle,
  motorcyclist: SCENE_COLOURS.cycle,
  bird: SCENE_COLOURS.bird,
  dynamic: SCENE_COLOURS.dynamic,
  noise: SCENE_COLOURS.noise,
  default: SCENE_COLOURS.dynamic,
};

const TRAIL_MAX_POINTS = 40;

function colourFor(cls) {
  return CLASS_COLOUR[cls] ?? CLASS_COLOUR.default;
}

/** One rendered track: a box, its edges, and a trail line. */
class TrackVisual {
  constructor(scene, colour) {
    this.colour = colour;
    const geo = new THREE.BoxGeometry(1, 1, 1);
    this.edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geo),
      new THREE.LineBasicMaterial({
        color: colour,
        transparent: true,
        opacity: 0.95,
      }),
    );
    this.fill = new THREE.Mesh(
      geo,
      new THREE.MeshBasicMaterial({
        color: colour,
        transparent: true,
        opacity: 0.22,
      }),
    );
    scene.add(this.edges);
    scene.add(this.fill);

    this.trailPoints = [];
    this.trailGeo = new THREE.BufferGeometry();
    this.trailGeo.setAttribute(
      "position",
      new THREE.BufferAttribute(new Float32Array(TRAIL_MAX_POINTS * 3), 3),
    );
    this.trailGeo.setDrawRange(0, 0);
    this.trail = new THREE.Line(
      this.trailGeo,
      new THREE.LineBasicMaterial({
        color: colour,
        transparent: true,
        opacity: 0.7,
      }),
    );
    scene.add(this.trail);
  }

  /**
   * Repaints the box, its fill and its trail together.
   *
   * The classifier refines a track as observations accumulate, so an object
   * often starts unclassified and only later becomes a car. Colouring once at
   * creation left those vehicles grey for their whole life, which is most of
   * them at a site where unclassified returns outnumber cars five to one.
   */
  setColour(colour) {
    if (colour === this.colour) return;
    this.colour = colour;
    this.edges.material.color.setHex(colour);
    this.fill.material.color.setHex(colour);
    this.trail.material.color.setHex(colour);
  }

  update(t) {
    const x = toSceneX(t.x);
    const y = toSceneY(t.z);
    const z = toSceneZ(t.y);

    // A floor on the rendered extent. Many real clusters are a few tens of
    // centimetres across, which at street scale is under a pixel; the box is a
    // marker for a measurement, so it stays legible rather than true to size.
    const MIN_EXTENT = 0.9;
    const l = Math.max(t.l || 0, MIN_EXTENT);
    const w = Math.max(t.w || 0, MIN_EXTENT);
    const h = Math.max(t.h || 0, MIN_EXTENT);

    for (const obj of [this.edges, this.fill]) {
      obj.visible = true;
      // Track position is the oriented box centre, so it is used directly.
      obj.position.set(x, y, z);
      obj.scale.set(l, h, w);
      // ENU yaw is counter-clockwise about up; three.js rotates about +Y the
      // other way, hence the negation.
      obj.rotation.set(0, -(t.bh ?? t.hdg ?? 0), 0);
    }

    const pts = this.trailPoints;
    const last = pts[pts.length - 1];
    if (!last || Math.abs(last[0] - x) > 0.02 || Math.abs(last[2] - z) > 0.02) {
      pts.push([x, y - h / 2 + 0.05, z]);
      if (pts.length > TRAIL_MAX_POINTS) pts.shift();
      const arr = this.trailGeo.attributes.position.array;
      for (let i = 0; i < pts.length; i++) {
        arr[i * 3] = pts[i][0];
        arr[i * 3 + 1] = pts[i][1];
        arr[i * 3 + 2] = pts[i][2];
      }
      this.trailGeo.attributes.position.needsUpdate = true;
      this.trailGeo.setDrawRange(0, pts.length);
    }
    this.trail.visible = pts.length > 1;
  }

  hide() {
    this.edges.visible = false;
    this.fill.visible = false;
    this.trail.visible = false;
  }

  setOpacity(opacity) {
    this.edges.material.opacity = 0.95 * opacity;
    this.fill.material.opacity = 0.22 * opacity;
    this.trail.material.opacity = 0.7 * opacity;
  }

  setVisible(visible) {
    this.edges.visible = visible;
    this.fill.visible = visible;
    this.trail.visible = visible && this.trailPoints.length > 1;
  }

  dispose(scene) {
    for (const o of [this.edges, this.fill, this.trail]) {
      scene.remove(o);
      o.geometry.dispose();
      o.material.dispose();
    }
  }
}

/** One reusable geometry for the recorded foreground returns in clip exports. */
class PointCloudVisual {
  constructor(scene) {
    this.geometry = new THREE.BufferGeometry();
    this.capacity = 0;
    this.material = new THREE.PointsMaterial({
      size: 0.11,
      vertexColors: true,
      sizeAttenuation: true,
      transparent: true,
      opacity: 1,
    });
    this.baseColour = new THREE.Color(SCENE_COLOURS.primary);
    this.points = new THREE.Points(this.geometry, this.material);
    this.points.visible = false;
    scene.add(this.points);
  }

  ensureCapacity(count) {
    if (count <= this.capacity) return;
    this.capacity = count;
    this.geometry.setAttribute(
      "position",
      new THREE.BufferAttribute(new Float32Array(count * 3), 3),
    );
    this.geometry.setAttribute(
      "color",
      new THREE.BufferAttribute(new Float32Array(count * 3), 3),
    );
  }

  update(points, opacity) {
    if (!points?.length) {
      this.points.visible = false;
      this.geometry.setDrawRange(0, 0);
      return;
    }
    this.ensureCapacity(points.length);
    const positions = this.geometry.attributes.position.array;
    const colours = this.geometry.attributes.color.array;
    points.forEach((point, i) => {
      positions[i * 3] = toSceneX(point[0]);
      positions[i * 3 + 1] = toSceneY(point[2]);
      positions[i * 3 + 2] = toSceneZ(point[1]);
      const light = 0.42 + 0.58 * Math.min(1, (point[3] ?? 0) / 255);
      colours[i * 3] = this.baseColour.r * light;
      colours[i * 3 + 1] = this.baseColour.g * light;
      colours[i * 3 + 2] = this.baseColour.b * light;
    });
    this.geometry.attributes.position.needsUpdate = true;
    this.geometry.attributes.color.needsUpdate = true;
    this.geometry.setDrawRange(0, points.length);
    this.material.opacity = opacity;
    this.points.visible = true;
  }
}

/**
 * Mounts a scene player on a canvas.
 *
 * @param {object} opts
 * @param {HTMLCanvasElement} opts.canvas
 * @param {string} opts.manifestURL relative or absolute; kept configurable so
 *   assets can later move to object storage without touching this code.
 * @param {string} [opts.pointCloudManifestURL] optional point overlay aligned
 *   to the beginning of the primary recording
 * @param {object} opts.ui element references for the transport controls
 */
export async function mountScenePlayer({
  canvas,
  manifestURL,
  pointCloudManifestURL,
  ui,
}) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
  renderer.setClearColor(SCENE_COLOURS.canvas, 1);

  const scene = new THREE.Scene();
  scene.fog = new THREE.Fog(SCENE_COLOURS.canvas, 60, 190);

  const camera = new THREE.PerspectiveCamera(52, 1, 0.5, 2000);

  const grid = new THREE.GridHelper(160, 32, 0x2c4049, 0x1c2b32);
  // Turn the grid onto the street.
  //
  // The grid is drawn square to the sensor, because that is the coordinate
  // frame the points arrive in — and a sensor is set down facing whatever the
  // kerb allowed, not facing down the road. So the squares cut across the
  // carriageway at whatever angle the tripod happened to sit at, and a viewer
  // reads that as the street being skewed rather than the grid.
  //
  // gridAzimuthDeg is the sensor's zero azimuth measured against this
  // junction's street grid. Turning the grid by it lines the squares up with
  // the kerbs. It changes nothing about the data: the points, the tracks and
  // their headings are all still in the sensor's frame.
  if (Number.isFinite(ui.gridAzimuthDeg)) {
    grid.rotation.y = (-ui.gridAzimuthDeg * Math.PI) / 180;
  }
  scene.add(grid);

  // A ring at the sensor origin gives the viewer a fixed reference point. The
  // sensor is the coordinate origin, mounted above the carriageway.
  const origin = new THREE.Mesh(
    new THREE.RingGeometry(0.7, 0.9, 32),
    new THREE.MeshBasicMaterial({ color: SCENE_COLOURS.primary, side: THREE.DoubleSide }),
  );
  origin.rotation.x = -Math.PI / 2;
  scene.add(origin);

  let backgroundCloud = null;

  /**
   * Loads the static background: one settled snapshot of the street with
   * nothing moving in it.
   *
   * Boxes floating in empty space are hard to read as a place. The background
   * is what makes a trajectory legible as "along the kerb" rather than "over
   * there somewhere". It is optional: a scene without one still plays.
   */
  async function loadBackground(url) {
    const res = await fetch(url);
    if (!res.ok) return null;

    const raw = new Uint8Array(await res.arrayBuffer());
    const isGzip = raw.length > 1 && raw[0] === 0x1f && raw[1] === 0x8b;
    const text = isGzip
      ? await new Response(
          new Blob([raw]).stream().pipeThrough(new DecompressionStream("gzip")),
        ).text()
      : new TextDecoder().decode(raw);
    const bg = JSON.parse(text);
    if (!Array.isArray(bg.points) || bg.points.length === 0) return null;

    const positions = new Float32Array(bg.points.length * 3);
    const colours = new Float32Array(bg.points.length * 3);
    let maxConf = 1;
    for (const p of bg.points) if (p[3] > maxConf) maxConf = p[3];

    bg.points.forEach((p, i) => {
      positions[i * 3] = toSceneX(p[0]);
      positions[i * 3 + 1] = toSceneY(p[2]);
      positions[i * 3 + 2] = toSceneZ(p[1]);
      // Confidence is how often the cell was seen. Fading the least certain
      // returns keeps transient clutter from reading as solid structure.
      // Dim and near-neutral. Dim because the background should lose every
      // contrast fight with the coloured boxes; neutral because it used to be
      // blue, which is now the colour of a pedestrian - a tinted background
      // would have people disappearing into the street they walk on.
      const c = 0.1 + 0.26 * Math.min(1, (p[3] || 0) / maxConf);
      colours[i * 3] = c * 0.94;
      colours[i * 3 + 1] = c * 0.96;
      colours[i * 3 + 2] = c;
    });

    const geo = new THREE.BufferGeometry();
    geo.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    geo.setAttribute("color", new THREE.BufferAttribute(colours, 3));
    const cloud = new THREE.Points(
      geo,
      new THREE.PointsMaterial({
        size: 0.11,
        vertexColors: true,
        sizeAttenuation: true,
      }),
    );
    scene.add(cloud);
    backgroundCloud = cloud;
    return bg;
  }

  /**
   * Frames the view from the background where there is one, and from the
   * tracks otherwise. Sensor height, mounting angle and the span of the
   * observed area differ per site, so hard-coded values only ever suit one
   * recording.
   */
  function boundsFromBackground(bg) {
    const span = Math.max(bg.max_x - bg.min_x, bg.max_y - bg.min_y, 20);
    return {
      centerX: toSceneX((bg.min_x + bg.max_x) / 2),
      centerZ: toSceneZ((bg.min_y + bg.max_y) / 2),
      groundY: toSceneY(bg.ground_z ?? bg.min_z),
      span: Math.min(span, 140),
    };
  }

  function boundsFromFrames(frames) {
    const xs = [];
    const zs = [];
    const bases = [];
    for (const f of frames) {
      for (const t of f.tr ?? []) {
        xs.push(toSceneX(t.x));
        zs.push(toSceneZ(t.y));
        bases.push(toSceneY(t.z) - (t.h || 0) / 2);
      }
    }
    if (!bases.length) return { centerX: 0, centerZ: 0, groundY: 0, span: 60 };

    // Percentiles, not extremes. A stray bird or a distant noise cluster would
    // otherwise pull the camera so far back that the street itself is a few
    // pixels across.
    const pct = (arr, p) => {
      const sorted = [...arr].sort((a, b) => a - b);
      return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
    };
    const x0 = pct(xs, 0.05);
    const x1 = pct(xs, 0.95);
    const z0 = pct(zs, 0.05);
    const z1 = pct(zs, 0.95);
    return {
      centerX: (x0 + x1) / 2,
      centerZ: (z0 + z1) / 2,
      groundY: pct(bases, 0.05),
      span: Math.max(x1 - x0, z1 - z0, 20),
    };
  }

  function applyBounds(b) {
    grid.position.set(b.centerX, b.groundY, b.centerZ);
    origin.position.y = b.groundY + 0.02;
    // Fog starts beyond the framed area so it adds depth without dimming the
    // objects the viewer came to see.
    scene.fog = new THREE.Fog(SCENE_COLOURS.canvas, b.span * 1.8, b.span * 5);
    sceneCamera.frame(b);
  }

  // Drone state. Declared up here because the camera's own input callback
  // lands the drone, and that callback is wired before the flight is built.
  let flight = null;
  let flying = false;
  let flightSeconds = 0;
  let setFlying = () => {};

  // The scene is drawn on demand, not every animation frame, so anything that
  // changes what the view should look like has to say so. Camera moves are the
  // main one: without this a drag on a paused scene would move the camera and
  // never be seen.
  let viewDirty = false;
  const markDirty = () => {
    viewDirty = true;
  };

  function resize() {
    const w = canvas.clientWidth || 960;
    const h = canvas.clientHeight || 540;
    if (canvas.width !== w || canvas.height !== h) {
      renderer.setSize(w, h, false);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
      markDirty();
    }
  }
  window.addEventListener("resize", resize);
  resize();

  let northAzimuthDeg = ui.northAzimuthDeg;
  const sceneCamera = createSceneCamera({
    camera,
    element: canvas,
    THREE,
    onChange: markDirty,
    // Touching the camera lands the drone. Anything else means a drag is
    // undone by the next animation frame.
    onUserInput: () => setFlying(false),
  });

  const session = await new SceneSession(manifestURL).open();
  const generatorVersions = sceneGeneratorVersions(session.parts);
  if (ui.generatorVersion && generatorVersions.length) {
    ui.generatorVersion.textContent = generatorVersions.join(", ");
    ui.generatorVersion.closest("[data-scene-generator]")?.removeAttribute("hidden");
  }
  let pointCloudSession = null;
  if (pointCloudManifestURL) {
    try {
      const candidate = await new SceneSession(pointCloudManifestURL).open();
      if (sceneSourcesAlign(session.parts[0]?.header, candidate.parts[0]?.header)) {
        pointCloudSession = candidate;
      } else {
        console.warn("Point-cloud overlay does not align with the tracked scene; hiding it.");
      }
    } catch (err) {
      console.warn("Point-cloud overlay could not be loaded; continuing with tracks.", err);
    }
  }

  /**
   * Reads the scene's vantages.json.
   *
   * This one file is where a scene's viewpoints live — not the part header,
   * not the recording, not the scene index. One writable copy means there is
   * never a question of which one the publisher meant, and an edit here is the
   * whole of the change. Accepts a bare array or an object with a "vantages"
   * key, matching `velocity scene vantages`.
   *
   * A scene that ships no file, or a file that cannot be read, falls back to
   * compass bearings: worth less than "Eastbound Howard", but still true.
   */
  async function loadVantages(url) {
    try {
      // no-cache, because this is the file someone edits to adjust an angle.
      // A cached copy would make every edit look like it did nothing.
      const res = await fetch(url, { cache: "no-cache" });
      if (!res.ok) return null;
      const body = await res.json();
      const list = Array.isArray(body) ? body : body?.vantages;
      return Array.isArray(list) && list.length ? list : null;
    } catch {
      return null;
    }
  }

  // A scene's own vantages win; the camera falls back to its compass views,
  // labelled truthfully where the operator measured where north is.
  const authoredVantages = ui.vantagesURL
    ? await loadVantages(ui.vantagesURL)
    : null;
  const vantages = sceneCamera.setVantages(
    authoredVantages,
    ui.northAzimuthDeg,
  );

  const background = ui.backgroundURL
    ? await loadBackground(ui.backgroundURL).catch(() => null)
    : null;

  let bounds;
  try {
    const first = session.parts[0];
    bounds = background
      ? boundsFromBackground(background)
      : boundsFromFrames(await first.loadChunk(first.chunks[0].c));
  } catch {
    bounds = { centerX: 0, centerZ: 0, groundY: 0, span: 60 };
  }
  applyBounds(bounds);

  // One constant-speed orbit fitted to the scene's vantages, on its own clock
  // so it keeps turning while playback is paused. A scene with fewer than two
  // eligible vantages gets a still view and no switch: one point does not
  // describe a circuit anybody asked for.
  flight = createSceneFlight({ vantages });
  const canFly = flight.path.length > 1;

  // Open on the orbit when there is one, rather than on the first vantage.
  // Placing the camera at a named viewpoint and then handing it to the drone
  // shows a jump on the first frame, which is a poor thing to open with.
  sceneCamera.applyVantage(canFly ? flight.sample(0) : vantages[0]);

  const visuals = new Map();
  const pointCloud = new PointCloudVisual(scene);
  let currentPart = -1;
  let strip = null;

  function render() {
    updateCompass(ui.compass, sceneCamera.currentVantage(), northAzimuthDeg);
    renderer.render(scene, camera);
    positionLabels();
  }

  // Speed labels are HTML positioned over the canvas rather than sprites: text
  // stays crisp at any zoom, and no texture is allocated per track.
  const labels = new Map();

  function labelFor(id) {
    let el = labels.get(id);
    if (!el) {
      el = document.createElement("span");
      el.className = "scene-label";
      ui.labelLayer.appendChild(el);
      labels.set(id, el);
    }
    return el;
  }

  const projected = new THREE.Vector3();

  /** Vertical gap to keep between two labels before one is nudged. */
  const LABEL_ROW_PX = 15;

  function positionLabels() {
    if (!ui.labelLayer) return;
    const w = canvas.clientWidth;
    const h = canvas.clientHeight;

    const placed = [];
    for (const [id, el] of labels) {
      const v = visuals.get(id);
      if (!v || !v.edges.visible) {
        el.style.display = "none";
        continue;
      }
      projected.copy(v.edges.position);
      projected.y += (v.edges.scale.y ?? 1) / 2 + 0.6;
      projected.project(camera);

      // Behind the camera, or off screen: nothing useful to place.
      if (
        projected.z > 1 ||
        Math.abs(projected.x) > 1.1 ||
        Math.abs(projected.y) > 1.1
      ) {
        el.style.display = "none";
        continue;
      }
      placed.push({
        el,
        x: ((projected.x + 1) / 2) * w,
        y: ((1 - projected.y) / 2) * h,
        // Depth decides who keeps the true position when two labels collide.
        depth: projected.z,
      });
    }

    // Vehicles queued at a junction project to nearly the same point, so their
    // labels land on top of each other and none can be read. Nearest first,
    // then push any later label that would overlap up into a free row.
    placed.sort((a, b) => a.depth - b.depth);
    const taken = [];
    for (const p of placed) {
      let y = p.y;
      let guard = 0;
      while (
        guard++ < 12 &&
        taken.some(
          (t) => Math.abs(t.y - y) < LABEL_ROW_PX && Math.abs(t.x - p.x) < 46,
        )
      ) {
        y -= LABEL_ROW_PX;
      }
      taken.push({ x: p.x, y });
      p.el.style.display = "block";
      p.el.style.left = `${p.x}px`;
      p.el.style.top = `${y}px`;
    }
  }

  /** Drops every box, trail and label. Used at a part change and at a wrap. */
  function resetVisuals() {
    for (const v of visuals.values()) v.dispose(scene);
    visuals.clear();
    for (const el of labels.values()) el.remove();
    labels.clear();
    pointCloud.update([], 0);
  }

  function renderFrame(frame, partIndex, pointFrame = null, pointOpacity = 0) {
    // Track identifiers are export-local, so they carry no meaning across a
    // part boundary. Drop all visuals rather than let a trail jump sites.
    if (partIndex !== currentPart) {
      resetVisuals();
      currentPart = partIndex;
    }

    const seen = new Set();
    for (const t of frame.tr ?? []) {
      seen.add(t.id);
      let v = visuals.get(t.id);
      if (!v) {
        v = new TrackVisual(scene, colourFor(t.c));
        visuals.set(t.id, v);
      }
      // Re-applied every frame: a track's class can change under it.
      v.setColour(colourFor(t.c));
      v.update(t);
      v.setOpacity(1);
      v.setVisible(state.boxesVisible);

      // Only vehicles are labelled. A number over every pedestrian would bury
      // the scene, and speed is the thing a street asks about cars.
      if (
        ui.labelLayer &&
        (t.c === "car" || t.c === "bus" || t.c === "truck")
      ) {
        // The track's peak so far, not this instant's, which may already have
        // passed by the time the box is on screen.
        const mph = (t.mspd ?? t.spd ?? 0) * MPS_TO_MPH;
        if (mph >= 1) {
          labelFor(t.id).textContent = `${Math.round(mph)} mph`;
        }
      }
    }
    for (const [id, v] of visuals) {
      if (!seen.has(id)) {
        v.dispose(scene);
        visuals.delete(id);
        labels.get(id)?.remove();
        labels.delete(id);
      }
    }
    pointCloud.update(pointFrame?.p, pointOpacity);
    render();
    return seen.size;
  }

  // ---- transport ----------------------------------------------------------

  const state = {
    seconds: 0,
    playing: autoplayScene(),
    // Read from the markup so the selector is the one place the default lives.
    rate: Number(ui.rate?.value) || 1,
    pending: false,
  };
  if (backgroundCloud) backgroundCloud.visible = state.backgroundVisible;
  grid.visible = state.gridVisible;
  const playbackDuration = () =>
    sceneLoopDuration(
      session.duration,
      pointCloudSession?.duration,
      state.loopOpening,
    );

  function formatClock(sec) {
    const s = Math.max(0, Math.floor(sec));
    return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
  }

  async function show(seconds) {
    if (state.pending) {
      state.refreshRequested = true;
      return;
    }
    state.pending = true;
    try {
      const { frame, partIndex } = await session.frameAt(seconds);
      if (!frame) return;
      let pointFrame = null;
      let pointOpacity = 0;
      if (
        state.lidarVisible &&
        pointCloudSession &&
        partIndex === 0 &&
        seconds < pointCloudSession.duration
      ) {
        pointOpacity = pointCloudOverlayOpacity(seconds, pointCloudSession.duration, {
          fadeOutSeconds: pointCloudSession.manifest?.fade_out_seconds,
          fadeInSeconds: pointCloudSession.manifest?.loop_fade_in_seconds,
          loopingIn: state.pointCloudLoopingIn,
        });
        if (pointOpacity > 0) {
          // Use the tracked frame's timestamp, not the animation clock. Both
          // exports contain this frame, so the boxes and returns cannot drift.
          pointFrame = (await pointCloudSession.frameAt(frame.t / 1e6)).frame;
        }
        if (
          state.pointCloudLoopingIn &&
          seconds >= (pointCloudSession.manifest?.loop_fade_in_seconds ?? 2)
        ) {
          state.pointCloudLoopingIn = false;
        }
      }
      if (!state.lidarVisible) {
        pointFrame = null;
        pointOpacity = 0;
      }
      const n = renderFrame(frame, partIndex, pointFrame, pointOpacity);
      if (ui.stats) {
        const tracks = frame.tr ?? [];
        const speeds = tracks.map((t) => t.spd ?? 0);
        const fastest = speeds.length ? Math.max(...speeds) : 0;
        // Named by mode rather than totalled, because "14 on foot" says
        // something about a street that "14 objects" does not.
        let vehicle = 0;
        let person = 0;
        let cycle = 0;
        for (const t of tracks) {
          if (t.c === "car" || t.c === "bus" || t.c === "truck") vehicle++;
          else if (t.c === "pedestrian") person++;
          else if (t.c === "cyclist" || t.c === "motorcyclist") cycle++;
        }
        const parts = [];
        if (vehicle) parts.push(`${vehicle} vehicle${vehicle > 1 ? "s" : ""}`);
        if (person) parts.push(`${person} walking`);
        if (cycle) parts.push(`${cycle} cycling`);
        ui.stats.textContent =
          (parts.length ? parts.join(" · ") : `${n} tracked`) +
          (fastest > 0
            ? ` · fastest ${(fastest * MPS_TO_MPH).toFixed(0)} mph`
            : "");
      }
    } catch (err) {
      reportError(err);
    } finally {
      state.pending = false;
      if (state.refreshRequested) {
        state.refreshRequested = false;
        void show(state.seconds);
      }
    }
  }

  function syncUI() {
    strip?.setPlayhead(state.seconds);
    // The strip is a slider to a screen reader, so it has to say where it is.
    if (ui.timelineCanvas) {
      ui.timelineCanvas.setAttribute(
        "aria-valuenow",
        String(Math.round(state.seconds)),
      );
      ui.timelineCanvas.setAttribute(
        "aria-valuetext",
        `${formatClock(state.seconds)} of ${formatClock(playbackDuration())}`,
      );
    }
    if (ui.clock) ui.clock.textContent = formatClock(state.seconds);
    if (ui.playToggle) {
      ui.playToggle.textContent = state.playing ? "Pause" : "Play";
      ui.playToggle.setAttribute(
        "aria-label",
        state.playing ? "Pause" : "Play",
      );
    }
  }

  function reportError(err) {
    const msg =
      err instanceof SceneError
        ? err.message
        : "The scene could not be loaded.";
    if (ui.status) {
      ui.status.textContent = msg;
      ui.status.hidden = false;
    }
    state.playing = false;
    syncUI();
  }

  // Playback advances on wall-clock delta scaled by rate, and the frame shown
  // is whichever the recording says belongs at that instant. Frame intervals
  // are not uniform, so no fixed tick is assumed anywhere.
  // The most *wall* time one frame may account for. requestAnimationFrame
  // stops while a tab is hidden, so the first frame after returning can carry
  // an arbitrarily large delta; without a clamp the playhead would jump.
  // The scene-time step is this times the rate, so at 16x a stalled frame
  // advances four seconds — bounded, and what 16x means.
  const MAX_WALL_STEP_SEC = 0.25;

  /** Wall clock of the previous animation frame; 0 means "no delta yet". */
  let lastWall = 0;

  function loop(now) {
    const raw = lastWall ? (now - lastWall) / 1000 : 0;
    const wallDt = Math.min(Math.max(raw, 0), MAX_WALL_STEP_SEC);
    lastWall = now;

    // The drone runs on wall time, not scene time, so it keeps circling while
    // playback is paused and does not fly at sixteen times the speed when the
    // recording does.
    if (flying && flight) {
      flightSeconds += wallDt;
      sceneCamera.applyVantage(flight.sample(flightSeconds));
    }

    if (state.playing) {
      const next = advanceSceneClock(
        state.seconds,
        wallDt,
        state.rate,
        playbackDuration(),
      );
      state.seconds = next.seconds;
      // Wrap rather than stop. A scene is a loop of street, not a film with an
      // ending, and eleven minutes in, whoever is still watching wants the
      // next pass rather than a dead playhead and a button to press.
      if (next.wrapped) {
        // Trails are per-track state built up frame by frame. Carrying them
        // over the wrap would draw a line from the last vehicle of the
        // recording to the first.
        resetVisuals();
        state.pointCloudLoopingIn = state.lidarVisible;
      }
      void show(state.seconds);
      syncUI();
    }
    // Redraw for anything that changed the view without changing the frame:
    // an orbit, a pan, a zoom, a resize. This is what keeps the viewport live
    // while playback is paused.
    if (viewDirty) {
      viewDirty = false;
      render();
    }
    requestAnimationFrame(loop);
  }

  if (ui.playToggle) {
    ui.playToggle.addEventListener("click", () => {
      state.playing = !state.playing;
      syncUI();
    });
  }
  if (ui.rate) {
    ui.rate.addEventListener("change", () => {
      state.rate = Number(ui.rate.value) || 1;
    });
  }
  if (ui.lidarToggle) {
    ui.lidarToggle.addEventListener("change", () => {
      state.lidarVisible = ui.lidarToggle.checked;
      state.pointCloudLoopingIn = false;
      if (!state.lidarVisible) pointCloud.update([], 0);
      void show(state.seconds);
      render();
    });
  }
  if (ui.boxesToggle) {
    ui.boxesToggle.addEventListener("change", () => {
      state.boxesVisible = ui.boxesToggle.checked;
      for (const visual of visuals.values()) {
        visual.setVisible(state.boxesVisible);
      }
      render();
    });
  }
  if (ui.backgroundToggle) {
    ui.backgroundToggle.addEventListener("change", () => {
      state.backgroundVisible = ui.backgroundToggle.checked;
      if (backgroundCloud) backgroundCloud.visible = state.backgroundVisible;
      render();
    });
  }
  if (ui.gridToggle) {
    ui.gridToggle.addEventListener("change", () => {
      state.gridVisible = ui.gridToggle.checked;
      grid.visible = state.gridVisible;
      render();
    });
  }
  if (ui.openingLoopToggle) {
    ui.openingLoopToggle.addEventListener("change", () => {
      state.loopOpening = ui.openingLoopToggle.checked;
      if (state.loopOpening && state.seconds >= playbackDuration()) {
        resetVisuals();
        state.seconds = 0;
        state.pointCloudLoopingIn = state.lidarVisible;
        void show(0);
      }
      strip?.setDuration(playbackDuration());
      if (ui.duration) {
        ui.duration.textContent = formatClock(playbackDuration());
      }
      syncUI();
    });
  }
  // Returning to a hidden tab should resume, not skip ahead.
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") lastWall = 0;
  });

  // --- viewport controls ---------------------------------------------------

  /** Marks one chip as the active viewpoint, or none while the drone flies. */
  function markPreset(id) {
    for (const el of ui.presets?.querySelectorAll("[data-preset]") ?? []) {
      el.setAttribute("aria-pressed", String(el.dataset.preset === id));
    }
  }

  /**
   * Starts or lands the drone.
   *
   * Landing leaves the camera exactly where it is: the point of touching a
   * control is to keep what you were looking at. Taking off resumes at the
   * bearing already on screen rather than wherever the flight clock had
   * drifted to, so the view carries on turning instead of cutting elsewhere.
   */
  setFlying = (next) => {
    if (next === flying) return;
    flying = next && canFly;
    if (flying) {
      flightSeconds = flight.phaseFor(sceneCamera.currentVantage().azimuth_deg);
      markPreset(null);
    }
    if (ui.flyToggle) ui.flyToggle.setAttribute("aria-checked", String(flying));
  };

  if (ui.presets) {
    ui.presets.replaceChildren();
    for (const vantage of vantages) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "scene-chip";
      btn.textContent = vantage.label;
      btn.dataset.preset = vantage.id;
      btn.setAttribute("aria-pressed", "false");
      btn.addEventListener("click", () => {
        // Picking a viewpoint is a request to stay there, so the drone lands.
        setFlying(false);
        markPreset(sceneCamera.applyPreset(vantage.id));
        // No render() here: moving the camera marks the view dirty and the
        // next animation frame draws it, paused or not.
      });
      ui.presets.appendChild(btn);
    }
  }

  if (ui.flyToggle) {
    // A scene with fewer than two eligible vantages has nowhere to fly, and a
    // switch that cannot do anything is worse than no switch.
    ui.flyToggle.hidden = !canFly;
    if (ui.flyTip) ui.flyTip.hidden = !canFly;
    ui.flyToggle.addEventListener("click", () => setFlying(!flying));
  }

  // The drone is on by default: a still three-quarter view of a junction says
  // less than a slow circle of it, and nobody has to find a control to get it.
  if (canFly) {
    flying = true;
    flightSeconds = 0;
    ui.flyToggle?.setAttribute("aria-checked", "true");
  } else {
    markPreset(vantages[0]?.id);
  }

  // Framing a useful angle is easy; describing it to whoever edits the scene
  // is not. This hands over the exact numbers a vantage is stored in.
  ui.compass?.addEventListener("click", () => {
    setFlying(false);
    sceneCamera.applyVantage(
      northOverhead(sceneCamera.currentVantage(), northAzimuthDeg),
    );
    markPreset(null);
  });
  mountSceneDev({
    panel: ui.devPanel,
    captureView: ui.captureView,
    siteId: ui.siteId,
    gridAzimuthDeg: ui.gridAzimuthDeg,
    northAzimuthDeg: ui.northAzimuthDeg,
    onTop: () => {
      setFlying(false);
      sceneCamera.applyVantage({ azimuth_deg: 0, polar_deg: 0, zoom: 0.85 });
      markPreset(null);
    },
    onChange: (angles) => {
      const updatedVantages = sceneCamera.setVantages(
        authoredVantages,
        angles.north_azimuth_deg,
      );
      for (const button of ui.presets?.querySelectorAll("[data-preset]") ??
        []) {
        const vantage = updatedVantages.find(
          (v) => v.id === button.dataset.preset,
        );
        if (vantage) button.textContent = vantage.label;
      }
      grid.rotation.y = -((angles.grid_azimuth_deg ?? 0) * Math.PI) / 180;
      northAzimuthDeg = angles.north_azimuth_deg;
      render();
    },
  });

  if (ui.captureView) {
    ui.captureView.addEventListener("click", async () => {
      const v = sceneCamera.currentVantage();
      const text = JSON.stringify(v);
      let copied = false;
      try {
        if (!navigator.clipboard?.writeText)
          throw new Error("Clipboard unavailable");
        await navigator.clipboard.writeText(text);
        copied = true;
      } catch {
        // Clipboard access is often refused; the numbers are shown either way.
      }
      if (ui.captureOutput) {
        ui.captureOutput.textContent = text;
        ui.captureOutput.hidden = false;
      }
      ui.captureView.textContent = copied
        ? "Copied"
        : "Copy failed — shown below";
      setTimeout(() => {
        ui.captureView.textContent = "Copy this view";
      }, 2200);
    });
  }

  if (ui.modeToggle) {
    const setMode = (mode) => {
      const active = sceneCamera.setMode(mode);
      for (const el of ui.modeToggle.querySelectorAll("[data-mode]")) {
        el.setAttribute("aria-pressed", String(el.dataset.mode === active));
      }
    };
    for (const el of ui.modeToggle.querySelectorAll("[data-mode]")) {
      el.addEventListener("click", () => setMode(el.dataset.mode));
    }
    setMode("orbit");
  }

  // --- timeline annotation -------------------------------------------------

  if (ui.timelineCanvas && ui.timelineURL) {
    try {
      const res = await fetch(ui.timelineURL);
      if (res.ok) {
        const summary = await res.json();
        renderSceneSpeedStats(ui.speedStats, summary);
        strip = createTimelineStrip({
          canvas: ui.timelineCanvas,
          summary,
          duration: session.duration,
          onSeek: (seconds) => {
            // A jump is not continuous motion, so the trails leading up to the
            // old position describe nothing about the new one.
            resetVisuals();
            state.pointCloudLoopingIn = false;
            state.seconds = seconds;
            void show(seconds);
            syncUI();
          },
        });
        strip.setDuration(playbackDuration());
      } else {
        renderSceneSpeedStats(ui.speedStats, null);
      }
    } catch {
      renderSceneSpeedStats(ui.speedStats, null);
      // A scene without a summary still plays; these annotations are aids,
      // not prerequisites.
    }
  }

  if (ui.duration) ui.duration.textContent = formatClock(playbackDuration());
  if (ui.title) ui.title.textContent = session.title;

  await show(0);
  syncUI();
  if (ui.status) ui.status.hidden = true;
  if (ui.loading) ui.loading.hidden = true;

  requestAnimationFrame(loop);

  return session;
}
