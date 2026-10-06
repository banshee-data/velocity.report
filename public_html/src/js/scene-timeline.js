// Timeline annotation strip.
//
// Draws where something is happening, so a viewer can find the interesting
// parts of a recording instead of scrubbing blindly through eleven minutes of
// mostly-quiet street.
//
// Traffic diverges from a zero line: vehicles and speed above it, people and
// bikes below.
// A road busy with people reads nothing like one busy with cars, and stacking
// them into one total hides exactly that. Peak speed is overlaid as a line,
// since the busiest moment and the fastest one are rarely the same one.
//
// Unclassified returns are deliberately not drawn. They outnumber everything
// else at some sites, and plotting them would bury the modes the strip exists
// to compare.

import { SCENE_CSS_COLOURS } from "./scene-colours.js";

/** Matches the class colours used for the boxes, so the strip reads as the same scene. */
export const MODE_COLOURS = {
  vehicle: SCENE_CSS_COLOURS.vehicle,
  person: SCENE_CSS_COLOURS.walking,
  cycle: SCENE_CSS_COLOURS.cycle,
  other: "#3d4a52",
};

const SPEED_COLOUR = "#f2a65a";

/**
 * Renders a timeline summary into a canvas and reports scrub positions.
 *
 * @param {object} opts
 * @param {HTMLCanvasElement} opts.canvas
 * @param {object} opts.summary timeline.json contents
 * @param {number} opts.duration scene length in seconds
 * @param {(seconds:number)=>void} opts.onSeek
 */
export function createTimelineStrip({ canvas, summary, duration, onSeek }) {
  const buckets = summary?.buckets ?? [];
  const bucketSeconds = summary?.bucket_seconds ?? 5;
  // A near-empty scene would otherwise scale its noise to full height.
  // The taller of the two directions sets the scale, so neither half clips and
  // a bar means the same count whichever way it points.
  const recordingSpan = duration || buckets.length * bucketSeconds || 1;
  let span = recordingSpan;

  let playhead = 0;
  let hover = null;

  function draw() {
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const ratio = Math.min(globalThis.devicePixelRatio || 1, 2);
    const w = canvas.clientWidth || 600;
    const h = canvas.clientHeight || 72;
    if (canvas.width !== w * ratio || canvas.height !== h * ratio) {
      canvas.width = w * ratio;
      canvas.height = h * ratio;
    }
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const visibleBuckets = buckets.slice(
      0,
      Math.max(1, Math.ceil(span / bucketSeconds)),
    );
    const peakSide = Math.max(
      1,
      ...visibleBuckets.map((b) =>
        Math.max((b.ped ?? 0) + (b.cyc ?? 0), b.veh ?? 0),
      ),
    );
    const maxSpeed = Math.max(1, ...visibleBuckets.map((b) => b.spd ?? 0));
    const barW = Math.max(1, w / Math.max(visibleBuckets.length, 1));
    // Zero sits in the middle. Vehicles grow upwards; people and bikes grow
    // downwards, so the two halves of a street's traffic can be compared
    // against each other rather than stacked into one indistinct total.
    const mid = Math.round(h / 2);
    const halfH = mid - 4;
    // One shared scale for both directions, so a bar's length means the same
    // number of objects whichever way it points.
    const scale = halfH / peakSide;

    visibleBuckets.forEach((b, i) => {
      const x = i * barW;
      const barWidth = Math.max(barW - 0.5, 0.5);

      // Upward: vehicles.
      const veh = b.veh ?? 0;
      if (veh > 0) {
        ctx.fillStyle = MODE_COLOURS.vehicle;
        ctx.fillRect(x, mid - veh * scale, barWidth, veh * scale);
      }

      // Downward: people, then bikes stacked below them.
      let down = mid;
      for (const [key, mode] of [
        ["ped", "person"],
        ["cyc", "cycle"],
      ]) {
        const v = b[key] ?? 0;
        if (v <= 0) continue;
        const barH = v * scale;
        ctx.fillStyle = MODE_COLOURS[mode];
        ctx.fillRect(x, down, barWidth, barH);
        down += barH;
      }
    });

    // Peak speed over the vehicle half.
    ctx.beginPath();
    ctx.strokeStyle = SPEED_COLOUR;
    ctx.lineWidth = 1.5;
    visibleBuckets.forEach((b, i) => {
      const x = i * barW + barW / 2;
      const y = mid - 3 - ((b.spd ?? 0) / maxSpeed) * (halfH - 4);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();

    // The zero line last, so it reads on top of everything.
    ctx.strokeStyle = "rgba(207, 227, 232, 0.45)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(0, mid + 0.5);
    ctx.lineTo(w, mid + 0.5);
    ctx.stroke();

    const px = (playhead / span) * w;
    const isDarkMode =
      window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
    ctx.strokeStyle = isDarkMode ? "#ffffff" : "#111827";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(px, 0);
    ctx.lineTo(px, h);
    ctx.stroke();

    if (hover != null) {
      ctx.strokeStyle = isDarkMode
        ? "rgba(255,255,255,0.35)"
        : "rgba(17, 24, 39, 0.35)";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(hover, 0);
      ctx.lineTo(hover, h);
      ctx.stroke();
    }
  }
  function secondsAt(clientX) {
    const rect = canvas.getBoundingClientRect();
    // A hidden or collapsed strip has no width; dividing by it would send
    // the playhead to NaN, from which nothing recovers.
    if (!(rect.width > 0)) return 0;
    const frac = Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
    return frac * span;
  }

  /** The bucket under a given time, for the caller's readout. */
  function bucketAt(seconds) {
    const i = Math.floor(seconds / bucketSeconds);
    return buckets[Math.min(Math.max(i, 0), buckets.length - 1)] ?? null;
  }

  /**
   * Steps the playhead from the keyboard.
   *
   * The strip replaced a range input, which was the only way to scrub without
   * a pointer. A canvas answers to nothing by default, so the keys have to be
   * put back by hand or the transport becomes mouse-only.
   */
  function onKeyDown(e) {
    const fine = 1;
    const coarse = Math.max(10, span / 20);
    let next = playhead;
    switch (e.key) {
      case "ArrowLeft":
        next -= e.shiftKey ? coarse : fine;
        break;
      case "ArrowRight":
        next += e.shiftKey ? coarse : fine;
        break;
      case "PageUp":
        next -= coarse;
        break;
      case "PageDown":
        next += coarse;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = span;
        break;
      default:
        return;
    }
    e.preventDefault();
    onSeek?.(Math.min(span, Math.max(0, next)));
  }
  canvas.addEventListener("keydown", onKeyDown);

  canvas.setAttribute("aria-valuemin", "0");
  canvas.setAttribute("aria-valuemax", String(Math.round(span)));

  canvas.style.touchAction = "none";
  canvas.addEventListener("pointerdown", (e) => {
    // Capture is an optimisation for dragging past the strip's edge, not a
    // precondition for seeking. It throws when the pointer is not active,
    // so a failure here must not swallow the seek itself.
    try {
      canvas.setPointerCapture?.(e.pointerId);
    } catch {
      /* dragging outside the strip simply will not track */
    }
    onSeek?.(secondsAt(e.clientX));
    e.preventDefault();
  });
  canvas.addEventListener("pointermove", (e) => {
    hover = e.clientX - canvas.getBoundingClientRect().left;
    if (e.buttons > 0) onSeek?.(secondsAt(e.clientX));
    draw();
  });
  canvas.addEventListener("pointerleave", () => {
    hover = null;
    draw();
  });
  // Guarded rather than assumed: the strip is unit-tested outside a browser,
  // and a module that only works next to a global is a module that cannot be
  // tested without one.
  globalThis.addEventListener?.("resize", draw);

  draw();

  return {
    draw,
    bucketAt,
    setDuration(seconds) {
      span = seconds > 0 ? Math.min(seconds, recordingSpan) : recordingSpan;
      playhead = Math.min(playhead, span);
      canvas.setAttribute("aria-valuemax", String(Math.round(span)));
      draw();
    },
    setPlayhead(seconds) {
      playhead = seconds;
      draw();
    },
  };
}
