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

/**
 * Fallback viewpoints, used only when a scene names none.
 *
 * Compass bearings say nothing about a street. A recording that knows its own
 * geometry ships labels like "Eastbound Howard" instead, and those arrive in
 * the export header rather than being hardcoded here.
 */
/**
 * The views a scene has when it ships no vantages of its own.
 *
 * Distances match the one scene that was framed by hand, soma1: the four
 * compass views sit at 0.25 of the framing distance, which is close enough to
 * read a vehicle's shape against the street. The defaults were 0.85 — three
 * and a half times further out — which framed the whole survey and left the
 * traffic as specks.
 *
 * The overview and the overhead plan are excluded from the flight, as they are
 * in soma1. An orbit is fitted to the vantages it flies, so leaving an
 * overhead view at 2 degrees of elevation in that set drags the circle up and
 * out: the flight was not only too far away, it was the wrong shape.
 */
export const DEFAULT_VANTAGES = [
  {
    id: "overview",
    label: "Overview",
    azimuth_deg: 45,
    polar_deg: 55,
    zoom: 0.5,
    fly: false,
  },
  {
    id: "north",
    label: "From north",
    azimuth_deg: 0,
    polar_deg: 68,
    zoom: 0.25,
  },
  {
    id: "east",
    label: "From east",
    azimuth_deg: 90,
    polar_deg: 68,
    zoom: 0.25,
  },
  {
    id: "south",
    label: "From south",
    azimuth_deg: 180,
    polar_deg: 68,
    zoom: 0.25,
  },
  {
    id: "west",
    label: "From west",
    azimuth_deg: 270,
    polar_deg: 68,
    zoom: 0.25,
  },
  {
    id: "top",
    label: "Overhead",
    azimuth_deg: 0,
    polar_deg: 2,
    zoom: 0.5,
    fly: false,
  },
];

const COMPASS = [
  "north",
  "north-east",
  "east",
  "south-east",
  "south",
  "south-west",
  "west",
  "north-west",
];

/**
 * The default vantages, labelled by true bearing where north is known.
 *
 * "From north" on a default vantage means the sensor's zero azimuth, which is
 * wherever the tripod was pointed when it was set down — a kerb, a railing,
 * whatever was there. Calling that north is only true by accident, and it is a
 * confident sort of wrong: a viewer has no reason to doubt a compass label.
 *
 * northAzimuthDeg is the sensor's zero measured clockwise from true north, so
 * a vantage sitting at sensor azimuth A stands at true bearing A + north. With
 * it the labels become true; without it they stay as they were, since a
 * scene's own azimuths are still a consistent way to move around it.
 */
export function compassVantages(northAzimuthDeg) {
  if (!Number.isFinite(northAzimuthDeg)) return DEFAULT_VANTAGES;
  return DEFAULT_VANTAGES.map((v) => {
    if (v.id === "overview" || v.id === "top") return v;
    const bearing =
      ((((v.azimuth_deg ?? 0) + northAzimuthDeg) % 360) + 360) % 360;
    const point = COMPASS[Math.round(bearing / 45) % 8];
    return {
      ...v,
      label: `From ${point}`,
      true_bearing_deg: Math.round(bearing),
    };
  });
}

const deg = (d) => (d * Math.PI) / 180;

/**
 * Cleans a hand-written vantage list, mirroring the exporter's Normalise.
 *
 * These arrive from a JSON file someone edits by hand, so an entry may be
 * missing a label or carry a bearing of -90. An entry with no id is dropped
 * rather than rendered, because the id is what marks a chip active and two
 * unnamed entries would both answer to `undefined`.
 */
export function normaliseVantages(list) {
  if (!Array.isArray(list)) return [];
  const seen = new Set();
  const out = [];
  for (const v of list) {
    const id = String(v?.id ?? "").trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    out.push({
      ...v,
      id,
      label: String(v.label ?? "").trim() || id,
      azimuth_deg: (((Number(v.azimuth_deg) || 0) % 360) + 360) % 360,
      polar_deg: Number(v.polar_deg) || 0,
      zoom: Number(v.zoom) || 1,
      // Absent means "join the flight"; only an explicit false opts out.
      fly: v.fly !== false,
    });
  }
  return out;
}

/**
 * Drives a three.js camera in spherical coordinates around a ground target.
 *
 * @param {object} opts
 * @param {import('three').PerspectiveCamera} opts.camera
 * @param {HTMLElement} opts.element surface that receives the gestures
 * @param {object} opts.THREE the three module, passed in so this file imports nothing
 * @param {() => void} [opts.onChange] fired whenever the camera moves. The
 *   player draws on demand rather than every frame, so without this a drag on
 *   a paused scene would change the camera and never be seen.
 * @param {() => void} [opts.onUserInput] fired when a person moves the camera,
 *   as opposed to the drone flight doing it. Something has to give way, and it
 *   is not going to be the person: a drag overwritten 16 ms later reads as a
 *   broken control.
 */
export function createSceneCamera({
  camera,
  element,
  THREE,
  onChange,
  onUserInput,
}) {
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

  // The framing a vantage's offsets are measured from. Offsets are relative so
  // a saved viewpoint survives the export being regenerated with a different
  // background or a shifted observed area.
  const home = { x: 0, z: 0, y: 0, span: 100 };
  let vantages = DEFAULT_VANTAGES;

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
    onChange?.();
  }

  /**
   * Frames a bounding box, and makes that framing the reference the zoom
   * presets and limits are relative to.
   */
  function frame({ centerX, centerZ, groundY, span }) {
    // Look at the sensor, not at the middle of what it happened to see.
    //
    // The observed area is whatever the street gave back — long down one
    // approach, short where a building stands — so its centre wanders away
    // from the sensor by an amount that says more about the geometry of the
    // reflections than about the junction. Orbiting that centre swings the
    // sensor around the edge of the view. The sensor is at the origin, it is
    // the one fixed thing in the scene, and it is what the scene is of.
    //
    // The span still comes from the observed area: what to look at and how
    // much of it to fit in are different questions.
    home.x = 0;
    home.z = 0;
    home.y = groundY;
    home.span = span;
    // centerX and centerZ are accepted and deliberately unused; callers pass
    // the observed bounds and the framing distance below is derived from them.
    void centerX;
    void centerZ;
    target.set(home.x, groundY, home.z);
    const fov = deg(camera.fov);
    // Pull back far enough that the span fits the narrower screen axis.
    const fit = span / 2 / Math.tan(fov / 2);
    state.baseDistance = Math.max(fit * 1.25, MIN_DISTANCE * 2);
    state.distance = state.baseDistance;
    minDistance = Math.max(MIN_DISTANCE, span * 0.05);
    maxDistance = state.baseDistance * 4;
    apply();
  }

  /**
   * Moves to a viewpoint. Takes a vantage-shaped object rather than an id,
   * because the drone flies through positions that lie between two named
   * vantages and so have no id of their own.
   */
  function applyVantage(v) {
    if (!v) return null;
    state.azimuth = deg(v.azimuth_deg ?? 0);
    state.polar = deg(v.polar_deg ?? 55);
    state.distance = state.baseDistance * (v.zoom || 1);
    // Offsets shift the look-at point across the ground, so a vantage can
    // centre on one approach rather than the middle of the junction.
    target.set(home.x + (v.offset_x ?? 0), home.y, home.z + (v.offset_y ?? 0));
    apply();
    return v.id ?? null;
  }

  function applyPreset(id) {
    const v = vantages.find((p) => p.id === id) ?? vantages[0];
    return v ? applyVantage(v) : null;
  }

  /**
   * Describes the current view in the same shape a vantage is stored in, so a
   * reader who has framed a useful angle can hand those numbers to whoever
   * edits the scene rather than describing it in prose.
   */
  function currentVantage() {
    const round = (n, p = 1) => Math.round(n * 10 ** p) / 10 ** p;
    return {
      azimuth_deg: round(((state.azimuth * 180) / Math.PI + 360) % 360),
      polar_deg: round((state.polar * 180) / Math.PI),
      zoom: round(state.distance / (state.baseDistance || 1), 2),
      offset_x: round(target.x - home.x),
      offset_y: round(target.z - home.z),
    };
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
    // Vertical drag moves the target along the ground away from or towards the
    // camera. Both axes are signed so the scene tracks the finger: drag up and
    // the ground goes up the screen, rather than the camera going up and the
    // ground appearing to fall.
    target.x -= (dx * cosA + dy * sinA) * scale;
    target.z -= (-dx * sinA + dy * cosA) * scale;
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
    onUserInput?.();
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
    onUserInput?.();
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
    onUserInput?.();
    e.preventDefault();
  }
  element.addEventListener("keydown", onKeyDown);

  apply();

  return {
    frame,
    applyPreset,
    applyVantage,
    currentVantage,
    /**
     * @param list a scene's own vantages, or null
     * @param northAzimuthDeg the sensor's zero measured from true north, if
     *   the operator recorded it; it labels the built-in views truthfully and
     *   is ignored when the scene brings its own, which are already named for
     *   the street rather than for a bearing.
     */
    setVantages(list, northAzimuthDeg) {
      const clean = normaliseVantages(list);
      vantages = clean.length ? clean : compassVantages(northAzimuthDeg);
      return vantages;
    },
    get vantages() {
      return vantages;
    },
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
