import { CRASH_HOLD_TICKS, REPLAY_HZ, replayPose } from "./sf-replay.js";

const impactWindow = 0.3 * REPLAY_HZ;
const binTicks = REPLAY_HZ / 10;
const headings = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];

function playbackSegments(clip) {
  const hold =
    clip.outcome === "stopped" ? clip.end - CRASH_HOLD_TICKS : clip.end;
  if (clip.impact === null) return [[clip.end, 0.99]];
  return [
    [Math.max(0, clip.impact - impactWindow), 0.99],
    [Math.min(hold, clip.impact + impactWindow), 0.5],
    [hold, 0.99],
    [clip.end, 1],
  ];
}

export function replayRate(clip, tick) {
  return playbackSegments(clip).find(([end]) => tick < end)?.[1] ?? 1;
}

export function advanceReplay(clip, tick, wallSeconds) {
  let remaining = Math.max(0, wallSeconds) * REPLAY_HZ;
  for (const [end, rate] of playbackSegments(clip)) {
    if (tick >= end) continue;
    const needed = (end - tick) / rate;
    if (remaining < needed) return tick + remaining * rate;
    remaining -= needed;
    tick = end;
  }
  return tick;
}

export function replayLandmark(clip, targets) {
  const pose = replayPose(clip, clip.impact ?? 0);
  let nearest,
    distance = Infinity;
  for (const target of targets) {
    const gap = Math.hypot(target.x - pose.x, target.y - pose.y);
    if (gap >= distance) continue;
    nearest = target;
    distance = gap;
  }
  if (!nearest) return "";
  const bearing = Math.atan2(nearest.x - pose.x, nearest.y - pose.y);
  const direction = headings[(Math.round(bearing / (Math.PI / 4)) + 8) % 8];
  const gap =
    distance >= 1000
      ? `${(distance / 1000).toFixed(1)} km`
      : `${Math.round(distance)} m`;
  return `${nearest.emoji} ${nearest.names.join(" & ")} · ${gap}${gap === "0 m" ? "" : " " + direction}`;
}

// Accumulate peak values per 100 ms from the full-rate simulated velocities.
// Each bin preserves one-tick braking and jerk events even when pose keys are sparse.
export function replayTelemetry(poses, impact = null, inbound = null) {
  const bins = [];
  let previous, lastAcceleration;
  for (const [tick, pose] of poses.entries()) {
    const v = pose.velocity;
    if (!v || !v.every(Number.isFinite)) return null;
    const speed = Math.hypot(...v);
    const bin = (bins[Math.floor(tick / binTicks)] ??= [0, 0, 0]);
    bin[0] = Math.max(bin[0], Math.round(speed * 2.236936));
    const approach = tick === impact && inbound ? inbound : previous;
    if (approach) {
      const delta = v.map((value, i) => (value - approach[i]) * REPLAY_HZ);
      const decel = (Math.hypot(...approach) - speed) * REPLAY_HZ;
      const baseline = lastAcceleration ?? (tick === impact ? [0, 0, 0] : null);
      bin[1] = Math.max(bin[1], Math.round(decel));
      if (baseline) {
        const jerk =
          Math.hypot(...delta.map((value, i) => value - baseline[i])) *
          REPLAY_HZ;
        bin[2] = Math.max(bin[2], Math.round(jerk));
      }
      lastAcceleration = delta;
    }
    previous = v;
  }
  return bins;
}

export function replayCharts(panel, clip) {
  const charts = [...panel.querySelectorAll("canvas")];
  const data = clip.telemetry;
  const available =
    Array.isArray(data) &&
    data.length > 0 &&
    data.length <= Math.floor(clip.end / binTicks) + 1 &&
    data.every(
      (bin) =>
        Array.isArray(bin) &&
        bin.length === charts.length &&
        bin.every((value) => Number.isSafeInteger(value) && value >= 0),
    );
  panel.querySelector(".telemetry-missing").hidden = available;
  const locale = new Intl.NumberFormat();
  const graphs = charts
    .map((canvas, channel) => {
      const figure = canvas.closest("figure");
      figure.hidden = !available;
      if (!available) return null;
      const points = data.map((bin) => bin[channel]);
      const peak = Math.max(...points);
      const peakText = `${locale.format(peak)} ${canvas.dataset.unit} peak`;
      figure.querySelector("output").textContent = peakText;
      canvas.setAttribute("aria-label", `${canvas.dataset.label}, ${peakText}`);
      return { canvas, ctx: canvas.getContext("2d"), points, peak };
    })
    .filter(Boolean);
  return (tick) => {
    for (const { canvas, ctx, points, peak } of graphs) {
      const w = canvas.width,
        h = canvas.height;
      const x = (t) => 1 + (t / clip.end) * (w - 2);
      ctx.clearRect(0, 0, w, h);
      ctx.fillStyle = "#4c6a7c";
      ctx.fillRect(0, h - 2, w, 1);
      ctx.strokeStyle = "#7dd3fc";
      ctx.lineWidth = 2;
      ctx.beginPath();
      for (const [i, value] of points.entries()) {
        const y = h - 3 - ((h - 6) * value) / (peak || 1);
        const start = x(i * binTicks);
        if (i === 0) ctx.moveTo(start, y);
        else ctx.lineTo(start, y);
        ctx.lineTo(x(Math.min(clip.end, (i + 1) * binTicks)), y);
      }
      ctx.stroke();
      if (clip.impact !== null) {
        ctx.fillStyle = "#fb923c";
        ctx.fillRect(x(clip.impact), 0, 1, h);
      }
      ctx.fillStyle = "#fff";
      ctx.fillRect(x(tick), 0, 1, h);
    }
  };
}
