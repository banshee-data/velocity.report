/**
 * Survey charts as SVG strings. Used at build time (Eleventy filters) and in
 * the browser (compare view), so a chart looks the same either way. Colours
 * are presentation attributes rather than CSS classes so a chart serialised
 * for "Save chart" keeps them.
 */
import { validLimit } from "./survey-model.js";

export const C = {
  bg: "#07090c",
  line: "#1b2632",
  line2: "#243140",
  text: "#e6ebf2",
  soft: "#a7b2c0",
  mute: "#7c8898",
  dim: "#4a5563",
  under: "#38bdf8",
  over: "#f87171",
  p85: "#fbbf24",
  latest: "#6ee7b7",
};
const MONO = "JetBrains Mono, ui-monospace, Menlo, monospace";

/** Colours for recordings, oldest first; the latest always takes C.latest. */
const RECORDING_COLOURS = ["#a78bfa", "#a7b2c0", "#f0abfc", "#fdba74"];
export function recordingColour(index, count) {
  return index === count - 1
    ? C.latest
    : RECORDING_COLOURS[index % RECORDING_COLOURS.length];
}

export const PERCENTILE_MARKS = [
  { key: "p50", label: "P50", colour: C.soft },
  { key: "p85", label: "P85", colour: C.p85 },
  { key: "p98", label: "P98", colour: C.over },
  { key: "max", label: "MAX", colour: C.over },
];

const esc = (s) =>
  String(s).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const n = (v) => Math.round(v * 10) / 10;
const text = (x, y, body, attrs = "") =>
  `<text x="${n(x)}" y="${n(y)}" font-family="${MONO}" ${attrs}>${esc(body)}</text>`;

/** A shared mph axis for one or more speed summaries: 5 mph steps, at least 5–40. */
export function speedScale(speeds, limit) {
  const list = speeds.filter(Boolean);
  const lo = Math.min(5, ...list.map((s) => s.bins[0]?.from ?? 5));
  const top = Math.max(
    40,
    ...list.map((s) => s.bins.at(-1)?.to ?? 40),
    (validLimit(limit) ?? 0) + 5,
  );
  return { lo, hi: Math.ceil(top / 5) * 5 };
}

/** Y ceiling in percent, rounded up to 5, shared across small multiples. */
export function shareCeiling(speeds) {
  const highest = Math.max(
    0,
    ...speeds.filter(Boolean).flatMap((s) => s.bins.map((b) => b.share * 100)),
  );
  return Math.max(5, Math.ceil(highest / 5) * 5);
}

export function histogramSvg(speed, opts = {}) {
  if (!speed) return "";
  const {
    width = 720,
    height = 240,
    compact = false,
    label = "Vehicle speed distribution",
  } = opts;
  const limit = validLimit(opts.limit);
  const { lo, hi } = opts.scale ?? speedScale([speed], limit);
  const ymax = opts.ymax ?? shareCeiling([speed]);
  const pl = compact ? 6 : 36;
  const pr = 8;
  const pt = compact ? 16 : 22;
  const pb = compact ? 18 : 30;
  const iw = width - pl - pr;
  const ih = height - pt - pb;
  const x = (v) => pl + ((v - lo) / (hi - lo)) * iw;
  const y = (share) => pt + ih - ((share * 100) / ymax) * ih;
  const out = [];
  out.push(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="${width}" height="${height}" role="img" aria-label="${esc(
      `${label}: ${speed.count} vehicle tracks, P50 ${speed.p50}, P85 ${speed.p85}, P98 ${speed.p98}, fastest ${speed.max} mph${limit ? `, posted limit ${limit} mph` : ""}`,
    )}">`,
  );
  if (limit !== null)
    out.push(
      `<rect x="${n(x(limit))}" y="${pt}" width="${n(x(hi) - x(limit))}" height="${ih}" fill="${C.over}" opacity=".06"/>`,
    );
  if (!compact) {
    for (const f of [0, 0.25, 0.5, 0.75, 1]) {
      const yy = n(pt + ih * f);
      out.push(
        `<line x1="${pl}" x2="${width - pr}" y1="${yy}" y2="${yy}" stroke="${C.line}" stroke-width="1"/>`,
      );
    }
  }
  for (const b of speed.bins) {
    const isOver = limit !== null && b.from >= limit;
    const top = y(b.share);
    out.push(
      `<rect x="${n(x(b.from) + 1)}" y="${n(top)}" width="${n(Math.max(1, x(b.to) - x(b.from) - 2))}" height="${n(pt + ih - top)}" rx="1.5" fill="${isOver ? C.over : C.under}" opacity="${isOver ? 0.85 : 0.7}"><title>${esc(
        `${b.from}–${b.to} mph: ${b.count} tracks (${(b.share * 100).toFixed(1)}%)`,
      )}</title></rect>`,
    );
  }
  if (limit !== null) {
    out.push(
      `<line x1="${n(x(limit))}" x2="${n(x(limit))}" y1="${pt - 8}" y2="${pt + ih}" stroke="${C.text}" stroke-width="1.5"/>`,
    );
    if (!compact)
      out.push(
        text(
          x(limit) + 5,
          pt - 1,
          `LIMIT ${limit}`,
          `font-size="10" fill="${C.text}"`,
        ),
      );
  }
  for (const m of PERCENTILE_MARKS.slice(0, 3)) {
    const v = speed[m.key];
    if (!Number.isFinite(v)) continue;
    out.push(
      `<line x1="${n(x(v))}" x2="${n(x(v))}" y1="${pt + ih}" y2="${pt + ih + 6}" stroke="${m.colour}" stroke-width="2"/>`,
    );
    if (!compact)
      out.push(
        text(
          x(v),
          pt + ih + 17,
          m.label,
          `font-size="9.5" fill="${m.colour}" text-anchor="middle"`,
        ),
      );
  }
  if (!compact) {
    for (let t = lo; t <= hi; t += 5)
      out.push(
        text(
          x(t),
          height - 1,
          t,
          `font-size="9" fill="${C.dim}" text-anchor="middle"`,
        ),
      );
    out.push(text(2, pt + 4, `${ymax}%`, `font-size="9" fill="${C.dim}"`));
  }
  out.push("</svg>");
  return out.join("");
}

/** One horizontal mph scale with the percentile markers and the limit. */
export function percentileStripSvg(speed, opts = {}) {
  if (!speed) return "";
  const { width = 420, height = 54 } = opts;
  const limit = validLimit(opts.limit);
  const lo = opts.lo ?? Math.max(0, Math.floor((speed.p50 - 5) / 5) * 5);
  const hi =
    opts.hi ??
    Math.max(lo + 15, Math.ceil((Math.max(speed.max, limit ?? 0) + 2) / 5) * 5);
  const x = (v) => 8 + ((v - lo) / (hi - lo)) * (width - 16);
  const out = [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="${width}" height="${height}" role="img" aria-label="${esc(
      `Percentile speeds on one scale: P50 ${speed.p50}, P85 ${speed.p85}, P98 ${speed.p98}, fastest ${speed.max} mph`,
    )}">`,
    `<line x1="8" x2="${width - 8}" y1="24" y2="24" stroke="${C.line2}" stroke-width="2"/>`,
  ];
  if (limit !== null) {
    out.push(
      `<rect x="${n(x(limit))}" y="18" width="${n(x(hi) - x(limit))}" height="12" fill="${C.over}" opacity=".08"/>`,
    );
    out.push(
      `<line x1="${n(x(limit))}" x2="${n(x(limit))}" y1="10" y2="38" stroke="${C.text}" stroke-width="1.5"/>`,
    );
    out.push(
      text(
        x(limit),
        8,
        `${limit} LIMIT`,
        `font-size="9.5" fill="${C.text}" text-anchor="middle"`,
      ),
    );
  }
  for (const m of PERCENTILE_MARKS) {
    const v = speed[m.key];
    if (!Number.isFinite(v)) continue;
    out.push(`<circle cx="${n(x(v))}" cy="24" r="4.5" fill="${m.colour}"/>`);
    out.push(
      text(
        x(v),
        48,
        m.label,
        `font-size="9.5" fill="${m.colour}" text-anchor="middle"`,
      ),
    );
  }
  out.push("</svg>");
  return out.join("");
}

/**
 * Percentile dot plot, one row per recording on a shared mph axis.
 * rows: [{ label, colour, speed }]
 */
export function percentileRowsSvg(rows, opts = {}) {
  const { width = 640, rowHeight = 38, labelWidth = 96 } = opts;
  const limit = validLimit(opts.limit);
  const values = rows.flatMap((r) =>
    PERCENTILE_MARKS.map((m) => r.speed?.[m.key]).filter(Number.isFinite),
  );
  const lo =
    opts.lo ?? Math.max(0, Math.floor((Math.min(...values) - 3) / 5) * 5);
  const hi =
    opts.hi ?? Math.ceil((Math.max(...values, limit ?? 0) + 3) / 5) * 5;
  const pl = labelWidth;
  const pr = 12;
  const top = 22;
  const body = rows.length * rowHeight;
  const height = top + body + 24;
  const x = (v) => pl + ((v - lo) / (hi - lo)) * (width - pl - pr);
  const out = [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="${width}" height="${height}" role="img" aria-label="${esc(
      `Percentile speeds per recording${limit ? ` against the ${limit} mph limit` : ""}: ${rows
        .map((r) => `${r.label} P85 ${r.speed?.p85 ?? "n/a"}`)
        .join("; ")}`,
    )}">`,
  ];
  if (limit !== null)
    out.push(
      `<rect x="${n(x(limit))}" y="${top - 6}" width="${n(x(hi) - x(limit))}" height="${body + 6}" fill="${C.over}" opacity=".06"/>`,
    );
  for (let t = lo; t <= hi; t += 5) {
    out.push(
      `<line x1="${n(x(t))}" x2="${n(x(t))}" y1="${top - 6}" y2="${top + body}" stroke="${C.line}" stroke-width="1"/>`,
    );
    out.push(
      text(
        x(t),
        height - 6,
        t,
        `font-size="9.5" fill="${C.dim}" text-anchor="middle"`,
      ),
    );
  }
  if (limit !== null) {
    out.push(
      `<line x1="${n(x(limit))}" x2="${n(x(limit))}" y1="${top - 12}" y2="${top + body}" stroke="${C.text}" stroke-width="1.5"/>`,
    );
    out.push(
      text(
        x(limit) + 5,
        top - 10,
        `LIMIT ${limit}`,
        `font-size="10" fill="${C.text}"`,
      ),
    );
  }
  rows.forEach((r, i) => {
    const cy = top + i * rowHeight + rowHeight / 2;
    out.push(text(0, cy + 4, r.label, `font-size="11" fill="${r.colour}"`));
    if (!r.speed) return;
    out.push(
      `<line x1="${n(x(r.speed.p50))}" x2="${n(x(r.speed.max))}" y1="${n(cy)}" y2="${n(cy)}" stroke="${C.line2}" stroke-width="2"/>`,
    );
    for (const m of PERCENTILE_MARKS) {
      const v = r.speed[m.key];
      if (!Number.isFinite(v)) continue;
      out.push(
        `<circle cx="${n(x(v))}" cy="${n(cy)}" r="${m.key === "p85" ? 6 : 4.5}" fill="${m.colour}" stroke="${C.bg}" stroke-width="1.5"><title>${esc(`${r.label} ${m.label} ${v} mph`)}</title></circle>`,
      );
      if (i === 0)
        out.push(
          text(
            x(v),
            cy - 11,
            m.label,
            `font-size="9" fill="${m.colour}" text-anchor="middle"`,
          ),
        );
    }
  });
  out.push("</svg>");
  return out.join("");
}

/** Tiny distribution for listings. Bars at or above the limit are red. */
export function sparklineSvg(speed, opts = {}) {
  if (!speed) return "";
  const { width = 90, height = 22 } = opts;
  const limit = validLimit(opts.limit);
  const { lo, hi } = opts.scale ?? { lo: 5, hi: 45 };
  const peak = Math.max(...speed.bins.map((b) => b.share)) || 1;
  const x = (v) => ((v - lo) / (hi - lo)) * width;
  const out = [
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} ${height}" width="${width}" height="${height}" aria-hidden="true" focusable="false">`,
  ];
  for (const b of speed.bins) {
    if (b.from >= hi) continue;
    const h = Math.max(b.count > 0 ? 1 : 0, (b.share / peak) * height);
    const isOver = limit !== null && b.from >= limit;
    out.push(
      `<rect x="${n(x(b.from) + 0.5)}" y="${n(height - h)}" width="${n(Math.max(0.5, x(Math.min(b.to, hi)) - x(b.from) - 1))}" height="${n(h)}" fill="${isOver ? C.over : C.under}" opacity="${isOver ? 0.8 : 0.6}"/>`,
    );
  }
  if (limit !== null && limit < hi)
    out.push(
      `<line x1="${n(x(limit))}" x2="${n(x(limit))}" y1="0" y2="${height}" stroke="${C.text}" stroke-width="1"/>`,
    );
  out.push("</svg>");
  return out.join("");
}
