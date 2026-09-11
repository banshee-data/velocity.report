const SVG_NS = "http://www.w3.org/2000/svg";

const METRICS = [
  ["max", "Maximum", "#2d1e2f"],
  ["p98", "p98", "#f25f5c"],
  ["p85", "p85", "#f7b32b"],
  ["p50", "p50", "#fbd92f"],
];

/** Build the percentage geometry shared by the renderer and its tests. */
export function speedHistogramModel(summary) {
  const buckets = summary?.histogram ?? [];
  const total = buckets.reduce((sum, bucket) => sum + (bucket.count ?? 0), 0);
  const values = buckets.map((bucket) => ({
    start: bucket.start_mph,
    end: bucket.start_mph + (summary?.bucket_size ?? 5),
    count: bucket.count ?? 0,
    percentage: total > 0 ? ((bucket.count ?? 0) / total) * 100 : 0,
  }));
  const highest = Math.max(0, ...values.map((bucket) => bucket.percentage));
  const ceiling = Math.max(5, Math.ceil(highest / 5) * 5);
  return { total, values, ceiling };
}

function svgElement(name, attributes = {}) {
  const element = document.createElementNS(SVG_NS, name);
  for (const [key, value] of Object.entries(attributes)) {
    element.setAttribute(key, String(value));
  }
  return element;
}

function renderHistogram(target, summary) {
  const model = speedHistogramModel(summary);
  target.replaceChildren();
  if (model.total === 0) {
    target.textContent = "No car speeds were recorded.";
    return;
  }

  const width = 640;
  const height = 260;
  const plot = { left: 52, right: 626, top: 12, bottom: 205 };
  const plotWidth = plot.right - plot.left;
  const plotHeight = plot.bottom - plot.top;
  const slot = plotWidth / model.values.length;
  const svg = svgElement("svg", {
    viewBox: `0 0 ${width} ${height}`,
    role: "img",
    "aria-label": `Car track maximum speeds above 5 mph in 5 mph buckets; ${model.total} tracks`,
  });

  for (let value = 0; value <= model.ceiling; value += 5) {
    const y = plot.bottom - (value / model.ceiling) * plotHeight;
    svg.append(
      svgElement("line", {
        x1: plot.left,
        x2: plot.right,
        y1: y,
        y2: y,
        class: "scene-speed__grid",
      }),
    );
    const label = svgElement("text", {
      x: plot.left - 8,
      y: y + 4,
      "text-anchor": "end",
      class: "scene-speed__tick",
    });
    label.textContent = `${value}%`;
    svg.append(label);
  }

  model.values.forEach((bucket, index) => {
    const barHeight = (bucket.percentage / model.ceiling) * plotHeight;
    const x = plot.left + index * slot + slot * 0.14;
    const bar = svgElement("rect", {
      x,
      y: plot.bottom - barHeight,
      width: Math.max(1, slot * 0.72),
      height: barHeight,
      class: "scene-speed__bar",
    });
    const title = svgElement("title");
    title.textContent = `${bucket.start}-${bucket.end} mph: ${bucket.count} tracks (${bucket.percentage.toFixed(1)}%)`;
    bar.append(title);
    svg.append(bar);

    const label = svgElement("text", {
      x: plot.left + index * slot + slot / 2,
      y: plot.bottom + 17,
      "text-anchor": "end",
      transform: `rotate(-45 ${plot.left + index * slot + slot / 2} ${plot.bottom + 17})`,
      class: "scene-speed__tick",
    });
    label.textContent = `${bucket.start}-${bucket.end}`;
    svg.append(label);
  });

  const axis = svgElement("text", {
    x: (plot.left + plot.right) / 2,
    y: height - 4,
    "text-anchor": "middle",
    class: "scene-speed__axis-label",
  });
  axis.textContent = "Maximum track speed (mph)";
  svg.append(axis);
  target.append(svg);
}

function renderTable(target, summary) {
  target.replaceChildren();
  for (const [key, label, colour] of METRICS) {
    const row = document.createElement("tr");
    const heading = document.createElement("th");
    heading.scope = "row";
    const swatch = document.createElement("i");
    swatch.style.background = colour;
    swatch.setAttribute("aria-hidden", "true");
    heading.append(swatch, label);
    const value = document.createElement("td");
    value.textContent = Number.isFinite(summary[key])
      ? `${summary[key].toFixed(1)} mph`
      : "—";
    row.append(heading, value);
    target.append(row);
  }
}

/** Populate the report-aligned metrics table and histogram for one scene. */
export function renderSceneSpeedStats(root, timeline) {
  if (!root) return;
  const summary = timeline?.vehicle_speed;
  const status = root.querySelector("[data-speed-status]");
  if (!summary) {
    if (status)
      status.textContent = "No car track summary is available for this scene.";
    return;
  }
  renderTable(root.querySelector("[data-speed-table]"), summary);
  renderHistogram(root.querySelector("[data-speed-histogram]"), summary);
  if (status) status.hidden = true;
}
