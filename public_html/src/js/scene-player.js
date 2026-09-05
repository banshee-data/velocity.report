// Three.js player for velocity.report scene exports.
//
// Renders tracked road users as oriented boxes with client-side trails, and
// drives playback from the recorded frame timestamps. It owns rendering only;
// fetching, decoding and seeking live in scene-reader.js.

import * as THREE from "three";
import { SceneSession, SceneError } from "./scene-reader.js";

// Sensor data is ENU: X east, Y north, Z up. Three.js is Y-up, so east stays
// X, up becomes Y, and north becomes -Z to keep the frame right-handed.
const toSceneX = (x) => x;
const toSceneY = (z) => z;
const toSceneZ = (y) => -y;

const MPS_TO_MPH = 2.236936;

/** Class colours. Anything unrecognised falls back to `default`. */
const CLASS_COLOUR = {
  car: 0x4bc0d9,
  bus: 0xf2a65a,
  truck: 0xf2a65a,
  cyclist: 0x9ad14b,
  motorcyclist: 0x9ad14b,
  pedestrian: 0xf25f5c,
  bird: 0xb08bd4,
  dynamic: 0x8899a6,
  noise: 0x55606b,
  default: 0x8899a6,
};

const TRAIL_MAX_POINTS = 40;

function colourFor(cls) {
  return CLASS_COLOUR[cls] ?? CLASS_COLOUR.default;
}

/** One rendered track: a box, its edges, and a trail line. */
class TrackVisual {
  constructor(scene, colour) {
    const geo = new THREE.BoxGeometry(1, 1, 1);
    this.edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geo),
      new THREE.LineBasicMaterial({ color: colour, transparent: true, opacity: 0.95 }),
    );
    this.fill = new THREE.Mesh(
      geo,
      new THREE.MeshBasicMaterial({ color: colour, transparent: true, opacity: 0.22 }),
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
      new THREE.LineBasicMaterial({ color: colour, transparent: true, opacity: 0.7 }),
    );
    scene.add(this.trail);
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

  dispose(scene) {
    for (const o of [this.edges, this.fill, this.trail]) {
      scene.remove(o);
      o.geometry.dispose();
      o.material.dispose();
    }
  }
}

/**
 * Mounts a scene player on a canvas.
 *
 * @param {object} opts
 * @param {HTMLCanvasElement} opts.canvas
 * @param {string} opts.manifestURL relative or absolute; kept configurable so
 *   assets can later move to object storage without touching this code.
 * @param {object} opts.ui element references for the transport controls
 */
export async function mountScenePlayer({ canvas, manifestURL, ui }) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75));
  renderer.setClearColor(0x0b1013, 1);

  const scene = new THREE.Scene();
  scene.fog = new THREE.Fog(0x0b1013, 60, 190);

  const camera = new THREE.PerspectiveCamera(52, 1, 0.5, 1200);

  const grid = new THREE.GridHelper(160, 32, 0x3f5a66, 0x24363f);
  scene.add(grid);

  // A ring at the sensor origin gives the viewer a fixed reference point. The
  // sensor is the coordinate origin, mounted above the carriageway.
  const origin = new THREE.Mesh(
    new THREE.RingGeometry(0.7, 0.9, 32),
    new THREE.MeshBasicMaterial({ color: 0x4bc0d9, side: THREE.DoubleSide }),
  );
  origin.rotation.x = -Math.PI / 2;
  scene.add(origin);

  /**
   * Frames the camera and ground plane from the data rather than assuming a
   * layout. Sensor height, mounting angle and the span of the observed area
   * differ per site, so hard-coded values only ever suit one recording.
   */
  function fitToObservations(frames) {
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
    if (!bases.length) return;

    // Percentiles, not extremes. A stray bird or a distant noise cluster would
    // otherwise pull the camera so far back that the street itself is a few
    // pixels across.
    const pct = (arr, p) => {
      const sorted = [...arr].sort((a, b) => a - b);
      return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
    };
    const ground = pct(bases, 0.05);
    const x0 = pct(xs, 0.05);
    const x1 = pct(xs, 0.95);
    const z0 = pct(zs, 0.05);
    const z1 = pct(zs, 0.95);

    const cx = (x0 + x1) / 2;
    const cz = (z0 + z1) / 2;
    const span = Math.max(x1 - x0, z1 - z0, 20);

    grid.position.set(cx, ground, cz);
    origin.position.y = ground + 0.02;

    camera.position.set(cx + span * 0.28, ground + span * 0.42, cz + span * 0.62);
    camera.lookAt(cx, ground + 1, cz);
    // Fog starts beyond the framed area so it adds depth without dimming the
    // objects the viewer came to see.
    scene.fog = new THREE.Fog(0x0b1013, span * 1.6, span * 4.5);
  }

  function resize() {
    const w = canvas.clientWidth || 960;
    const h = canvas.clientHeight || 540;
    if (canvas.width !== w || canvas.height !== h) {
      renderer.setSize(w, h, false);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    }
  }
  window.addEventListener("resize", resize);
  resize();

  const session = await new SceneSession(manifestURL).open();

  // Fit from the opening chunk, which is already fetched for the first frame.
  try {
    const first = session.parts[0];
    fitToObservations(await first.loadChunk(first.chunks[0].c));
  } catch {
    // A scene that cannot be measured still renders; the default view applies.
    camera.position.set(60, 45, 75);
    camera.lookAt(0, 0, 0);
  }

  const visuals = new Map();
  let currentPart = -1;

  function renderFrame(frame, partIndex) {
    // Track identifiers are export-local, so they carry no meaning across a
    // part boundary. Drop all visuals rather than let a trail jump sites.
    if (partIndex !== currentPart) {
      for (const v of visuals.values()) v.dispose(scene);
      visuals.clear();
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
      v.update(t);
    }
    for (const [id, v] of visuals) {
      if (!seen.has(id)) {
        v.dispose(scene);
        visuals.delete(id);
      }
    }
    renderer.render(scene, camera);
    return seen.size;
  }

  // ---- transport ----------------------------------------------------------

  const state = {
    seconds: 0,
    playing: false,
    rate: 1,
    lastWall: 0,
    pending: false,
  };

  function formatClock(sec) {
    const s = Math.max(0, Math.floor(sec));
    return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
  }

  async function show(seconds) {
    if (state.pending) return;
    state.pending = true;
    try {
      const { frame, partIndex } = await session.frameAt(seconds);
      if (!frame) return;
      const n = renderFrame(frame, partIndex);
      if (ui.stats) {
        const speeds = (frame.tr ?? []).map((t) => t.spd ?? 0);
        const fastest = speeds.length ? Math.max(...speeds) : 0;
        ui.stats.textContent =
          `${n} tracked ${n === 1 ? "object" : "objects"}` +
          (fastest > 0 ? ` · fastest ${(fastest * MPS_TO_MPH).toFixed(0)} mph` : "");
      }
    } catch (err) {
      reportError(err);
    } finally {
      state.pending = false;
    }
  }

  function syncUI() {
    if (ui.slider && document.activeElement !== ui.slider) {
      ui.slider.value = String(state.seconds);
    }
    if (ui.clock) ui.clock.textContent = formatClock(state.seconds);
    if (ui.playToggle) {
      ui.playToggle.textContent = state.playing ? "Pause" : "Play";
      ui.playToggle.setAttribute("aria-label", state.playing ? "Pause" : "Play");
    }
  }

  function reportError(err) {
    const msg = err instanceof SceneError ? err.message : "Something went wrong loading this scene.";
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
  // The longest step playback will take in one frame. requestAnimationFrame
  // stops while a tab is hidden, so the first frame after returning can carry
  // an arbitrarily large delta; without a clamp the playhead would jump.
  const MAX_STEP_SEC = 0.25;

  function loop(now) {
    if (state.playing) {
      const raw = state.lastWall ? (now - state.lastWall) / 1000 : 0;
      const dt = Math.min(Math.max(raw, 0), MAX_STEP_SEC);
      state.lastWall = now;
      state.seconds += dt * state.rate;
      if (state.seconds >= session.duration) {
        state.seconds = session.duration;
        state.playing = false;
      }
      void show(state.seconds);
      syncUI();
    }
    requestAnimationFrame(loop);
  }

  if (ui.slider) {
    ui.slider.min = "0";
    ui.slider.max = String(session.duration);
    ui.slider.step = "0.1";
    ui.slider.addEventListener("input", () => {
      state.seconds = Number(ui.slider.value);
      void show(state.seconds);
      if (ui.clock) ui.clock.textContent = formatClock(state.seconds);
    });
  }
  if (ui.playToggle) {
    ui.playToggle.addEventListener("click", () => {
      state.playing = !state.playing;
      state.lastWall = 0;
      syncUI();
    });
  }
  if (ui.rate) {
    ui.rate.addEventListener("change", () => {
      state.rate = Number(ui.rate.value) || 1;
    });
  }
  // Returning to a hidden tab should resume, not skip ahead.
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") state.lastWall = 0;
  });

  if (ui.duration) ui.duration.textContent = formatClock(session.duration);
  if (ui.title) ui.title.textContent = session.title;

  await show(0);
  syncUI();
  if (ui.status) ui.status.hidden = true;
  if (ui.loading) ui.loading.hidden = true;

  // Respect a reduced-motion preference by not auto-playing.
  const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  if (!reduced) {
    state.playing = true;
    syncUI();
  }
  requestAnimationFrame(loop);

  return session;
}
