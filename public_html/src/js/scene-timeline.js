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

import { gapBand } from "./scene-following.js";
import { SCENE_CSS_COLOURS } from "./scene-colours.js";

/** Matches the class colours used for the boxes, so the strip reads as the same scene. */
export const MODE_COLOURS = {
  vehicle: SCENE_CSS_COLOURS.vehicle,
  person: SCENE_CSS_COLOURS.walking,
  cycle: SCENE_CSS_COLOURS.cycle,
  other: "#3d4a52",
};

const SPEED_COLOUR = "#f2a65a";

/** Maximum recorded bucket speed in each recording-relative five-minute interval. */
export function fiveMinuteSpeeds(buckets, bucketSeconds, duration) {
  const result = Array.from({ length: Math.ceil(duration / 300) }, (_, i) => ({
    start: i * 300,
    end: Math.min(duration, (i + 1) * 300),
    speed: 0,
  }));
  buckets.forEach((b, i) => {
    const time = Number.isFinite(b.t) ? b.t : i * bucketSeconds;
    const bin = result[Math.floor(time / 300)];
    if (bin && time < duration && Number.isFinite(b.spd))
      bin.speed = Math.max(bin.speed, b.spd);
  });
  return result;
}

/**
 * Renders a timeline summary into a canvas and reports scrub positions.
 *
 * @param {object} opts
 * @param {HTMLCanvasElement} opts.canvas
 * @param {object} opts.summary timeline.json contents
 * @param {number} opts.duration scene length in seconds
 * @param {(seconds:number)=>void} opts.onSeek
 */
export function createTimelineStrip({
  canvas,
  summary,
  duration,
  onSeek,
  mode = "traffic",
  reviewPeriods,
}) {
  const lifecycle = new AbortController();
  const options = { signal: lifecycle.signal };
  if (mode === "following")
    canvas.setAttribute(
      "aria-label",
      "Experimental DRAC and minimum net time gap over time. Click or drag to seek; arrow keys step, Home and End jump.",
    );
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
    if (mode === "following") {
      const values = visibleBuckets
        .map((b) => b.maximum_experimental_drac_mps2)
        .filter((v) => Number.isFinite(v) && v >= 0);
      const actualPeak = Math.max(0, ...values),
        peak = Math.max(1, actualPeak);
      const width = w / Math.max(1, visibleBuckets.length);
      visibleBuckets.forEach((b, i) => {
        const value = b.maximum_experimental_drac_mps2;
        if (Number.isFinite(value) && value >= 0) {
          const height = (value / peak) * (h - 24);
          ctx.fillStyle = "#ffae62";
          ctx.fillRect(
            i * width,
            h - 18 - height,
            Math.max(1, width - 0.5),
            Math.max(1, height),
          );
        }
        ctx.fillStyle = gapBand(b.minimum_time_gap_s)?.colour ?? "#34404b";
        ctx.fillRect(i * width, h - 13, Math.max(1, width - 0.5), 10);
      });
      ctx.fillStyle = "#dce6ed";
      ctx.font = "11px sans-serif";
      ctx.fillText(
        values.length
          ? `DRAC peak ${actualPeak.toFixed(2)} m/s²`
          : "DRAC unavailable",
        5,
        12,
      );
    } else {
      const peakSide = Math.max(
        1,
        ...visibleBuckets.map((b) =>
          Math.max((b.ped ?? 0) + (b.cyc ?? 0), b.veh ?? 0),
        ),
      );
      const speeds = fiveMinuteSpeeds(buckets, bucketSeconds, span);
      const maxSpeed = Math.max(1, ...speeds.map((b) => b.speed));
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

      // Hold each five-minute maximum through its interval; only vertical steps
      // join bins, with the final partial interval ending at the recording edge.
      ctx.beginPath();
      ctx.strokeStyle = SPEED_COLOUR;
      ctx.lineWidth = 1.5;
      speeds.forEach((b, i) => {
        const x = (b.start / span) * w;
        const y = mid - 3 - (b.speed / maxSpeed) * (halfH - 4);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
        ctx.lineTo((b.end / span) * w, y);
      });
      ctx.stroke();

      // The zero line last, so it reads on top of everything.
      ctx.strokeStyle = "rgba(207, 227, 232, 0.45)";
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(0, mid + 0.5);
      ctx.lineTo(w, mid + 0.5);
      ctx.stroke();
    }

    if (mode === "following" && Array.isArray(reviewPeriods)) {
      let coveredUntil = 0;
      ctx.fillStyle = "#4b3e4a";
      for (const p of reviewPeriods) {
        const first = Math.max(0, Math.min(span, p.start));
        if (first > coveredUntil)
          ctx.fillRect(
            (coveredUntil / span) * w,
            0,
            ((first - coveredUntil) / span) * w,
            h,
          );
        coveredUntil = Math.max(coveredUntil, Math.min(span, p.end));
      }
      if (coveredUntil < span)
        ctx.fillRect(
          (coveredUntil / span) * w,
          0,
          ((span - coveredUntil) / span) * w,
          h,
        );
    }

    const px = (playhead / span) * w;
    // Read through globalThis, as with the pixel ratio above: a bare `window`
    // throws where there is none, and draw() runs on construction, so the
    // strip could not be built outside a browser. No scheme to ask means dark.
    const isDarkMode =
      globalThis.matchMedia?.("(prefers-color-scheme: dark)").matches ?? true;
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
  canvas.addEventListener("keydown", onKeyDown, options);

  canvas.setAttribute("aria-valuemin", "0");
  canvas.setAttribute("aria-valuemax", String(Math.round(span)));

  canvas.style.touchAction = "none";
  canvas.addEventListener(
    "pointerdown",
    (e) => {
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
    },
    options,
  );
  canvas.addEventListener(
    "pointermove",
    (e) => {
      hover = e.clientX - canvas.getBoundingClientRect().left;
      if (e.buttons > 0) onSeek?.(secondsAt(e.clientX));
      draw();
    },
    options,
  );
  canvas.addEventListener(
    "pointerleave",
    () => {
      hover = null;
      draw();
    },
    options,
  );
  // Guarded rather than assumed: the strip is unit-tested outside a browser,
  // and a module that only works next to a global is a module that cannot be
  // tested without one.
  globalThis.addEventListener?.("resize", draw, options);

  draw();

  return {
    dispose() {
      lifecycle.abort();
    },
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
