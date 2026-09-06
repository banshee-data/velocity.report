// Touch-aware orbit camera for the scene viewer.
//
// Written rather than pulled from three.js examples because only the core
// three module is vendored, and because a roadside scene needs a narrower set
// of gestures than a general-purpose orbit control: the ground plane is
// meaningful, so the camera stays above it and always looks at a target on it.
//
// Gestures are deliberately explicit on touch. Pinch always zooms, but a
// one-finger drag does whatever the on-screen mode toggle says, because
// guessing between orbit and pan from a single contact is how a viewer ends up
// somewhere it cannot get back from.

const MIN_POLAR = 0.05; // just above the horizon
const MAX_POLAR = Math.PI / 2 - 0.02; // never below the ground plane
const MIN_DISTANCE = 3;

/** Named viewpoints. Azimuth is the compass bearing the camera looks *from*. */
export const VANTAGE_PRESETS = [
  { id: "overview", label: "Overview", azimuth: 45, polar: 55, zoom: 1.0 },
  { id: "north", label: "From north", azimuth: 0, polar: 68, zoom: 0.85 },
  { id: "east", label: "From east", azimuth: 90, polar: 68, zoom: 0.85 },
  { id: "south", label: "From south", azimuth: 180, polar: 68, zoom: 0.85 },
  { id: "west", label: "From west", azimuth: 270, polar: 68, zoom: 0.85 },
  { id: "top", label: "Overhead", azimuth: 0, polar: 2, zoom: 0.95 },
];

const deg = (d) => (d * Math.PI) / 180;

/**
 * Drives a three.js camera in spherical coordinates around a ground target.
 *
 * @param {object} opts
 * @param {import('three').PerspectiveCamera} opts.camera
 * @param {HTMLElement} opts.element surface that receives the gestures
 * @param {object} opts.THREE the three module, passed in so this file imports nothing
 */
export function createSceneCamera({ camera, element, THREE }) {
  const target = new THREE.Vector3(0, 0, 0);

  const state = {
    azimuth: deg(45),
    polar: deg(55),
    distance: 60,
    baseDistance: 60,
    /** 'orbit' | 'pan' — what a one-finger drag does. */
    mode: "orbit",
  };

  let minDistance = MIN_DISTANCE;
  let maxDistance = 600;

  function apply() {
    state.polar = Math.min(MAX_POLAR, Math.max(MIN_POLAR, state.polar));
    state.distance = Math.min(
      maxDistance,
      Math.max(minDistance, state.distance),
    );

    const sinP = Math.sin(state.polar);
    camera.position.set(
      target.x + state.distance * sinP * Math.sin(state.azimuth),
      target.y + state.distance * Math.cos(state.polar),
      target.z + state.distance * sinP * Math.cos(state.azimuth),
    );
    camera.lookAt(target);
  }

  /**
   * Frames a bounding box, and makes that framing the reference the zoom
   * presets and limits are relative to.
   */
  function frame({ centerX, centerZ, groundY, span }) {
    target.set(centerX, groundY, centerZ);
    const fov = deg(camera.fov);
    // Pull back far enough that the span fits the narrower screen axis.
    const fit = span / 2 / Math.tan(fov / 2);
    state.baseDistance = Math.max(fit * 1.25, MIN_DISTANCE * 2);
    state.distance = state.baseDistance;
    minDistance = Math.max(MIN_DISTANCE, span * 0.05);
    maxDistance = state.baseDistance * 4;
    apply();
  }

  function applyPreset(id) {
    const preset =
      VANTAGE_PRESETS.find((p) => p.id === id) ?? VANTAGE_PRESETS[0];
    state.azimuth = deg(preset.azimuth);
    state.polar = deg(preset.polar);
    state.distance = state.baseDistance * preset.zoom;
    apply();
    return preset.id;
  }

  function orbit(dx, dy) {
    state.azimuth -= dx * 0.005;
    state.polar -= dy * 0.005;
    apply();
  }

  function pan(dx, dy) {
    // Pan across the ground plane, scaled by distance so the scene moves
    // with the finger at any zoom.
    const scale = state.distance * 0.0016;
    const sinA = Math.sin(state.azimuth);
    const cosA = Math.cos(state.azimuth);
    target.x -= (dx * cosA - dy * sinA) * scale;
    target.z += (dx * sinA + dy * cosA) * scale;
    apply();
  }

  function zoom(factor) {
    state.distance *= factor;
    apply();
  }

  // ---- gesture plumbing --------------------------------------------------

  const pointers = new Map();
  let lastPinch = 0;
  let lastCentroid = null;

  function centroid() {
    let x = 0;
    let y = 0;
    for (const p of pointers.values()) {
      x += p.x;
      y += p.y;
    }
    return { x: x / pointers.size, y: y / pointers.size };
  }

  function pinchDistance() {
    const [a, b] = [...pointers.values()];
    return Math.hypot(a.x - b.x, a.y - b.y);
  }

  function onPointerDown(e) {
    // Same reasoning as the timeline strip: capture helps a drag continue
    // outside the canvas, but must not be able to abort the gesture.
    try {
      element.setPointerCapture?.(e.pointerId);
    } catch {
      /* the drag simply stops tracking outside the canvas */
    }
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    lastCentroid = centroid();
    if (pointers.size === 2) lastPinch = pinchDistance();
  }

  function onPointerMove(e) {
    if (!pointers.has(e.pointerId)) return;
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });

    const c = centroid();
    const dx = c.x - (lastCentroid?.x ?? c.x);
    const dy = c.y - (lastCentroid?.y ?? c.y);
    lastCentroid = c;

    if (pointers.size >= 2) {
      // Two fingers: pinch to zoom, drag to pan. Orbit stays on one
      // finger so a two-handed gesture never tumbles the scene.
      const d = pinchDistance();
      if (lastPinch > 0 && d > 0) zoom(lastPinch / d);
      lastPinch = d;
      pan(dx, dy);
    } else if (state.mode === "pan") {
      pan(dx, dy);
    } else {
      orbit(dx, dy);
    }
    e.preventDefault();
  }

  function onPointerUp(e) {
    pointers.delete(e.pointerId);
    try {
      element.releasePointerCapture?.(e.pointerId);
    } catch {
      /* nothing to release */
    }
    lastCentroid = pointers.size ? centroid() : null;
    if (pointers.size < 2) lastPinch = 0;
  }

  function onWheel(e) {
    zoom(e.deltaY > 0 ? 1.1 : 0.9);
    e.preventDefault();
  }

  // touch-action:none is what stops the browser scrolling the page instead
  // of handing us the gesture.
  element.style.touchAction = "none";
  element.addEventListener("pointerdown", onPointerDown);
  element.addEventListener("pointermove", onPointerMove, { passive: false });
  element.addEventListener("pointerup", onPointerUp);
  element.addEventListener("pointercancel", onPointerUp);
  element.addEventListener("wheel", onWheel, { passive: false });

  /** Keyboard access, so the viewport is not mouse-and-touch only. */
  function onKeyDown(e) {
    const step = 24;
    switch (e.key) {
      case "ArrowLeft":
        orbit(-step, 0);
        break;
      case "ArrowRight":
        orbit(step, 0);
        break;
      case "ArrowUp":
        orbit(0, -step);
        break;
      case "ArrowDown":
        orbit(0, step);
        break;
      case "+":
      case "=":
        zoom(0.85);
        break;
      case "-":
      case "_":
        zoom(1.15);
        break;
      default:
        return;
    }
    e.preventDefault();
  }
  element.addEventListener("keydown", onKeyDown);

  apply();

  return {
    frame,
    applyPreset,
    setMode(mode) {
      state.mode = mode === "pan" ? "pan" : "orbit";
      return state.mode;
    },
    get mode() {
      return state.mode;
    },
    dispose() {
      element.removeEventListener("pointerdown", onPointerDown);
      element.removeEventListener("pointermove", onPointerMove);
      element.removeEventListener("pointerup", onPointerUp);
      element.removeEventListener("pointercancel", onPointerUp);
      element.removeEventListener("wheel", onWheel);
      element.removeEventListener("keydown", onKeyDown);
    },
  };
}
