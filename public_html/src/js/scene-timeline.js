// Timeline annotation strip.
//
// Draws where something is happening, so a viewer can find the interesting
// parts of a recording instead of scrubbing blindly through eleven minutes of
// mostly-quiet street.
//
// Traffic is split by mode because a road busy with people reads nothing like
// one busy with cars, and a single "objects" curve hides exactly that. Peak
// speed is overlaid as a line, since the busiest moment and the fastest one are
// rarely the same moment.

/** Matches the class colours used for the boxes, so the strip reads as the same scene. */
export const MODE_COLOURS = {
  vehicle: "#4bc0d9",
  person: "#f25f5c",
  cycle: "#9ad14b",
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
  const maxTotal = Math.max(summary?.max_total ?? 0, 1);
  const maxSpeed = Math.max(summary?.max_speed ?? 0, 1);
  const span = duration || buckets.length * bucketSeconds || 1;

  let playhead = 0;
  let hover = null;

  function draw() {
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const ratio = Math.min(window.devicePixelRatio || 1, 2);
    const w = canvas.clientWidth || 600;
    const h = canvas.clientHeight || 64;
    if (canvas.width !== w * ratio || canvas.height !== h * ratio) {
      canvas.width = w * ratio;
      canvas.height = h * ratio;
    }
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, w, h);

    const barW = Math.max(1, w / Math.max(buckets.length, 1));

    // Stacked volume, quietest mode at the bottom so the modes a reader
    // cares about sit against the baseline where they are easiest to compare.
    const order = ["other", "cycle", "person", "vehicle"];
    const keyFor = {
      other: "oth",
      cycle: "cyc",
      person: "ped",
      vehicle: "veh",
    };

    buckets.forEach((b, i) => {
      const x = i * barW;
      let y = h;
      for (const mode of order) {
        const v = b[keyFor[mode]] ?? 0;
        if (v <= 0) continue;
        const barH = (v / maxTotal) * (h - 12);
        ctx.fillStyle = MODE_COLOURS[mode];
        ctx.globalAlpha = mode === "other" ? 0.35 : 0.9;
        ctx.fillRect(x, y - barH, Math.max(barW - 0.5, 0.5), barH);
        y -= barH;
      }
    });
    ctx.globalAlpha = 1;

    // Peak speed as a line: the fastest moment is rarely the busiest one.
    ctx.beginPath();
    ctx.strokeStyle = SPEED_COLOUR;
    ctx.lineWidth = 1.5;
    buckets.forEach((b, i) => {
      const x = i * barW + barW / 2;
      const y = h - 10 - ((b.spd ?? 0) / maxSpeed) * (h - 20);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();

    // Playhead
    const px = (playhead / span) * w;
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(px, 0);
    ctx.lineTo(px, h);
    ctx.stroke();

    if (hover != null) {
      ctx.strokeStyle = "rgba(255,255,255,0.35)";
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
  window.addEventListener("resize", draw);

  draw();

  return {
    draw,
    bucketAt,
    setPlayhead(seconds) {
      playhead = seconds;
      draw();
    },
  };
}
