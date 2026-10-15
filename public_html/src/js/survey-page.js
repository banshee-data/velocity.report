/**
 * Survey listing and location pages: recording switcher, compare view,
 * share and chart export, and the listing's search, filters and table.
 *
 * The pages are complete without this script: the latest recording and every
 * listing row are rendered at build time. This adds the interactive parts.
 * The pure functions (readSurveyState, renderCompare, filterRows, sortRows)
 * are exported for tests.
 */
import {
  FAMILIES,
  SENSORS,
  compareSelection,
  compareRows,
  compareFinding,
  comparabilityNotes,
  familyAvailability,
  mph,
  pct,
} from "./survey-model.js";
import {
  histogramSvg,
  percentileRowsSvg,
  recordingColour,
  shareCeiling,
  speedScale,
} from "./survey-charts.js";

const esc = (s) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );

/**
 * Read the survey JSON and the query string into the page state:
 * { data, recording, compare } where compare is a compareSelection or null.
 * ?recording=<id> picks a recording; ?compare=<id>,<id>&base=<id> compares.
 */
export function readSurveyState(source, search) {
  const text = typeof source === "string" ? source : source?.textContent;
  const data = JSON.parse(text || "{}");
  const recordings = data.recordings ?? [];
  const params = new URLSearchParams(search ?? "");
  const latest = recordings.at(-1) ?? null;
  const recording =
    recordings.find((r) => r.id === params.get("recording")) ?? latest;
  let compare = null;
  if (params.has("compare")) {
    const ids = params.get("compare").split(",").filter(Boolean);
    compare = compareSelection(
      recordings.filter((r) => r.speed),
      ids,
      params.get("base"),
    );
  }
  return { data, recording, compare };
}

/** The query string for a comparison. */
export function compareSearch(ids, baseId) {
  const params = new URLSearchParams();
  params.set("compare", ids.join(","));
  if (baseId) params.set("base", baseId);
  return `?${params.toString().replace(/%2C/g, ",")}`;
}

const sensorTag = (kind) =>
  `<span class="sv-sensor sv-sensor--${esc(kind)}">${esc((SENSORS[kind] ?? SENSORS.unknown).tag)}</span>`;

const findingHTML = (segments) =>
  segments
    .map((s) =>
      s.tone === "strong"
        ? `<b>${esc(s.text)}</b>`
        : s.tone === "over"
          ? `<span class="sv-over">${esc(s.text)}</span>`
          : esc(s.text),
    )
    .join("");

function signed(value, unit) {
  if (value === null || value === undefined)
    return `<span class="sv-muted">—</span>`;
  const cls = value < 0 ? "sv-em" : value > 0 ? "sv-over" : "sv-muted";
  const sign = value > 0 ? "+" : value < 0 ? "−" : "±";
  return `<span class="${cls}">${sign}${Math.abs(value).toFixed(1)}${unit === "pts" ? " pts" : ""}</span>`;
}

function chartPair(wide, narrow) {
  return `<div class="sv-chart"><div class="sv-chart__wide">${wide}</div><div class="sv-chart__narrow">${narrow}</div></div>`;
}

/** HTML for the compare view. Pure: same input, same string. */
export function renderCompare(data, selection) {
  const { recordings: recs, base, latest } = selection;
  const all = data.recordings;
  const colourOf = (r) =>
    recordingColour(
      all.findIndex((x) => x.id === r.id),
      all.length,
    );
  const limit = data.limit ?? null;
  const scale = speedScale(
    recs.map((r) => r.speed),
    limit,
  );
  const ymax = shareCeiling(recs.map((r) => r.speed));
  const rows = compareRows(selection, limit);
  const sensors = [...new Set(recs.map((r) => r.sensor))];
  const ids = recs.map((r) => r.id);

  const chips = recs
    .map(
      (r) =>
        `<span class="sv-recchip-c"><i class="sv-recchip-c__box" style="background:${colourOf(r)}"></i><b>${esc(r.label)}</b><span class="sv-mono">${esc(r.when.time)}</span>${sensorTag(r.sensor)}${r.id === base.id ? `<span class="sv-tag sv-tag--em">BASELINE</span>` : ""}</span>`,
    )
    .join("");

  const rowsData = recs.map((r) => ({
    label: r.label,
    colour: colourOf(r),
    speed: r.speed,
  }));
  const dots = chartPair(
    percentileRowsSvg(rowsData, { width: 560, limit }),
    percentileRowsSvg(rowsData, {
      width: 311,
      limit,
      labelWidth: 70,
      rowHeight: 34,
    }),
  );

  const table = `<table class="sv-ctable">
    <caption class="sv-sr">Speed percentiles per recording, with the change from ${esc(base.label)} to ${esc(latest.label)}</caption>
    <thead><tr><th scope="col"><span class="sv-mono">MPH</span></th>${recs
      .map(
        (r) =>
          `<th scope="col" class="${r.id === base.id ? "is-base" : ""}"><span class="sv-mono" style="color:${colourOf(r)}">${esc(r.label.toUpperCase())}</span></th>`,
      )
      .join(
        "",
      )}<th scope="col" class="sv-ctable__delta"><span class="sv-mono">Δ VS ${esc(base.label.slice(0, 3).toUpperCase())}</span></th></tr></thead>
    <tbody>${rows
      .map(
        (row) =>
          `<tr><th scope="row"><span class="sv-mono">${esc(row.label)}</span> <span class="sv-ctable__name">${esc(row.name)}</span></th>${row.values
            .map(
              (v, i) =>
                `<td class="${recs[i].id === base.id ? "is-base" : ""}${recs[i].id === latest.id ? " is-latest" : ""}">${row.share ? pct(v) : mph(v)}</td>`,
            )
            .join(
              "",
            )}<td class="sv-ctable__delta">${signed(row.delta, row.unit)}</td></tr>`,
      )
      .join("")}</tbody></table>`;

  const multiples = recs
    .map(
      (r) => `<figure class="sv-multiple${r.id === base.id ? " is-base" : ""}">
        <figcaption><span class="sv-mono" style="color:${colourOf(r)}">${esc(r.label.toUpperCase())}</span><span class="sv-mono sv-small">${esc(r.when.time)} · ${r.when.minutes} min</span></figcaption>
        ${histogramSvg(r.speed, { width: 320, height: 160, limit, scale, ymax, label: `${r.label} speed distribution` })}
        <p class="sv-mono sv-small">P85 <span class="sv-amber">${mph(r.speed.p85)}</span>${limit ? ` · at or above limit <span class="sv-over">${pct(r.over)}</span>` : ""} · n ${r.speed.count}</p>
      </figure>`,
    )
    .join("");

  const facts = [
    ["Sensor", (r) => `${sensorTag(r.sensor)}`],
    ["Day &amp; time", (r) => esc(r.when.time)],
    ["Context", (r) => esc(r.when.context)],
    ["Duration", (r) => `${r.when.minutes} min`],
    ["Pipeline", (r) => `<code>${esc(r.build ?? "—")}</code>`],
  ];
  const comparability = `<div class="sv-facts" style="--cols:${recs.length}">
      <span></span>${recs.map((r) => `<span class="sv-mono" style="color:${colourOf(r)}">${esc(r.label.toUpperCase())}</span>`).join("")}
      ${facts.map(([label, f]) => `<span class="sv-mono">${label}</span>${recs.map((r) => `<span>${f(r)}</span>`).join("")}`).join("")}
    </div>
    <ul class="sv-notes">${comparabilityNotes(selection)
      .map(
        (n) =>
          `<li class="sv-notes__${n.tone}"><span aria-hidden="true">${n.tone === "warn" ? "◐" : "–"}</span><span>${esc(n.text)}</span></li>`,
      )
      .join("")}</ul>`;

  const availability = familyAvailability(selection);
  const others = availability.length
    ? `<section class="sv-section"><div class="sv-section__head"><h2 class="sv-section__title">Other measurements across recordings</h2></div>
        <div class="sv-card"><table class="sv-avail"><thead><tr><th></th>${recs
          .map(
            (r) =>
              `<th scope="col"><span class="sv-mono" style="color:${colourOf(r)}">${esc(r.label.toUpperCase())}</span> ${sensorTag(r.sensor)}</th>`,
          )
          .join("")}</tr></thead><tbody>${availability
          .map(
            (a) =>
              `<tr><th scope="row">${esc(a.family.name)}</th>${a.cells
                .map(
                  (c, i) =>
                    `<td><span class="sv-status sv-status--${c}">${c === "measured" ? "● Measured" : c === "missing" ? "◌ Not reported" : `– Not recorded · ${esc((SENSORS[recs[i].sensor] ?? SENSORS.unknown).label.toLowerCase())}`}</span></td>`,
                )
                .join("")}</tr>`,
          )
          .join("")}</tbody></table></div></section>`
    : "";

  return `
  <div class="sv-comparebar">
    <span class="sv-mono sv-em">COMPARING ${recs.length}</span>
    ${chips}
    <span class="sv-spacer"></span>
    <code class="sv-mono sv-hide-narrow">${esc(compareSearch(ids, base.id))}</code>
    <button type="button" class="sv-btn sv-btn--small" data-open-compare>Edit</button>
    <a class="sv-btn sv-btn--small" href="?">✕ Exit compare</a>
  </div>
  <div class="sv-finding">
    <p class="sv-eyebrow sv-em">Change · vehicle speed · ${esc(base.label)} → ${esc(latest.label)}</p>
    <p class="sv-finding__text">${findingHTML(compareFinding(selection, limit))}</p>
  </div>
  <section class="sv-card sv-card--accent" id="family-speed" aria-labelledby="compare-speed-title">
    <div class="sv-card__head">
      <h2 class="sv-card__title" id="compare-speed-title">Vehicle speed</h2>
      <span class="sv-status sv-status--measured">● Measured</span>
      <span class="sv-mono">comparable in all ${recs.length} · ${sensors.map((s) => esc((SENSORS[s] ?? SENSORS.unknown).label)).join(" + ")}</span>
      <span class="sv-spacer"></span>
      ${limit ? `<span class="sv-reftag">LAW</span>` : ""}
      <span class="sv-mono sv-hide-narrow">${esc(FAMILIES[0].definition)}</span>
    </div>
    <div class="sv-compare__grid">
      <div class="sv-stack"><h3 class="sv-eyebrow">Percentiles per recording</h3>${dots}</div>
      <div class="sv-stack"><h3 class="sv-eyebrow">Values</h3>${table}</div>
    </div>
    <details class="sv-dists" data-dists open>
      <summary><span class="sv-eyebrow">Distribution · same scale</span> <span class="sv-mono sv-small">share of vehicles per 5 mph bin · y-axis fixed at ${ymax}%</span></summary>
      <div class="sv-multiples" style="--cols:${recs.length}">${multiples}</div>
    </details>
    <div class="sv-stack sv-divided"><h3 class="sv-eyebrow">Are these comparable?</h3>${comparability}</div>
    <div class="sv-card__foot">
      <button type="button" class="sv-btn sv-btn--small" data-open-compare>Baseline: ${esc(base.label)} ▾</button>
      <button type="button" class="sv-btn sv-btn--small" data-compare-csv>⤓ Comparison CSV</button>
      <button type="button" class="sv-btn sv-btn--small" data-share>↗ Share comparison</button>
      <span class="sv-spacer"></span>
      <span class="sv-mono sv-small">n = ${recs.map((r) => r.speed.count).join(" / ")} vehicle tracks</span>
    </div>
  </section>
  ${others}`;
}

/** CSV of the compare table. */
export function compareCSV(data, selection) {
  const rows = compareRows(selection, data.limit ?? null);
  const head = [
    "metric",
    ...selection.recordings.map((r) => r.id),
    `delta_vs_${selection.base.id}`,
  ];
  const lines = [head.join(",")];
  for (const row of rows) {
    lines.push(
      [
        row.key,
        ...row.values.map((v) =>
          v === null ? "" : row.share ? (v * 100).toFixed(2) : v,
        ),
        row.delta ?? "",
      ].join(","),
    );
  }
  return `${lines.join("\n")}\n`;
}

// --- listing ----------------------------------------------------------------

/** Rows (plain objects with the data-* fields) that pass the query and filters. */
export function filterRows(rows, query, filters) {
  const q = String(query ?? "")
    .trim()
    .toLowerCase();
  return rows.filter((row) => {
    if (q && !`${row.name} ${row.cell ?? ""}`.toLowerCase().includes(q))
      return false;
    for (const f of filters) {
      const [kind, value] = f.split(":");
      if (kind === "multi" && !(Number(row.recordings) > 1)) return false;
      if (
        kind === "year" &&
        !String(row.years ?? "")
          .split(" ")
          .includes(value)
      )
        return false;
      if (
        kind === "sensor" &&
        !String(row.sensors ?? "")
          .split(" ")
          .includes(value)
      )
        return false;
      if (
        kind === "family" &&
        !String(row.families ?? "")
          .split(" ")
          .includes(value)
      )
        return false;
    }
    return true;
  });
}

/** Sort rows by a field; missing values always sort last, never as zero. */
export function sortRows(rows, key, direction = 1) {
  return [...rows].sort((a, b) => {
    const av = a[key];
    const bv = b[key];
    const am = av === undefined || av === null || av === "";
    const bm = bv === undefined || bv === null || bv === "";
    if (am || bm) return am === bm ? 0 : am ? 1 : -1;
    const an = Number(av);
    const bn = Number(bv);
    const cmp =
      Number.isFinite(an) && Number.isFinite(bn)
        ? an - bn
        : String(av).localeCompare(String(bv));
    return cmp * direction;
  });
}

// --- browser wiring -----------------------------------------------------------

function setHidden(el, hidden) {
  if (el) el.hidden = hidden;
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

function download(name, blob) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

/** Rasterise a chart SVG with a caption line, for sharing in slides or posts. */
async function saveChartPNG(svg, name, caption) {
  const clone = svg.cloneNode(true);
  const w = Number(clone.getAttribute("width")) || svg.viewBox.baseVal.width;
  const h = Number(clone.getAttribute("height")) || svg.viewBox.baseVal.height;
  const scale = 2;
  const pad = 16;
  const footer = 28;
  const url = URL.createObjectURL(
    new Blob([new XMLSerializer().serializeToString(clone)], {
      type: "image/svg+xml",
    }),
  );
  try {
    const img = new Image();
    await new Promise((resolve, reject) => {
      img.onload = resolve;
      img.onerror = reject;
      img.src = url;
    });
    const canvas = document.createElement("canvas");
    canvas.width = (w + pad * 2) * scale;
    canvas.height = (h + pad * 2 + footer) * scale;
    const ctx = canvas.getContext("2d");
    ctx.scale(scale, scale);
    ctx.fillStyle = "#07090c";
    ctx.fillRect(0, 0, w + pad * 2, h + pad * 2 + footer);
    ctx.drawImage(img, pad, pad, w, h);
    ctx.fillStyle = "#a7b2c0";
    ctx.font = "11px JetBrains Mono, ui-monospace, Menlo, monospace";
    ctx.fillText(caption, pad, h + pad + footer - 6, w);
    const blob = await new Promise((resolve) =>
      canvas.toBlob(resolve, "image/png"),
    );
    if (blob) download(`${name}.png`, blob);
  } finally {
    URL.revokeObjectURL(url);
  }
}

function initLocation(root) {
  const dataEl = document.getElementById("survey-data");
  if (!dataEl) return;
  const state = readSurveyState(dataEl, location.search);
  const { data, recording, compare } = state;

  // Recording: show its block, its header meta and its provenance.
  if (recording) {
    for (const el of root.querySelectorAll("[data-recording]"))
      el.hidden = el.dataset.recording !== recording.id;
    for (const el of root.querySelectorAll("[data-recording-only]"))
      el.hidden = el.dataset.recordingOnly !== recording.id;
    const label = root.querySelector("[data-recselect-label]");
    if (label) label.textContent = recording.label;
    setHidden(
      root.querySelector("[data-recselect-latest]"),
      recording !== data.recordings.at(-1),
    );
    for (const a of root.querySelectorAll("[data-recording-link]")) {
      if (a.dataset.recordingLink === recording.id)
        a.setAttribute("aria-current", "true");
    }
    // The "On this page" links name the latest recording's cards at build time.
    for (const a of root.querySelectorAll("[data-family-link]"))
      a.href = compare
        ? `#family-${a.dataset.familyLink}`
        : `#family-${a.dataset.familyLink}-${recording.id}`;
  }

  // A recording without 3D playback (a radar survey) has no evidence section.
  if (recording && !recording.hasPlayback) {
    for (const el of root.querySelectorAll("#evidence, [data-evidence-link]"))
      el.hidden = true;
  }

  // Recording menu.
  const button = root.querySelector("[data-recselect-button]");
  const menu = root.querySelector("[data-recselect-menu]");
  if (button && menu) {
    const close = () => {
      menu.hidden = true;
      button.setAttribute("aria-expanded", "false");
    };
    button.addEventListener("click", () => {
      menu.hidden = !menu.hidden;
      button.setAttribute("aria-expanded", String(!menu.hidden));
    });
    document.addEventListener("click", (e) => {
      if (!menu.hidden && !e.target.closest("[data-recselect]")) close();
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && !menu.hidden) {
        close();
        button.focus();
      }
    });
  }

  // Expand / collapse the full distribution.
  root.addEventListener("click", (e) => {
    const expand = e.target.closest("[data-expand]");
    if (!expand) return;
    const panel = document.getElementById(expand.getAttribute("aria-controls"));
    const open = expand.getAttribute("aria-expanded") !== "true";
    expand.setAttribute("aria-expanded", String(open));
    panel.hidden = !open;
    panel.closest("[data-speed-body]")?.classList.toggle("is-expanded", open);
    const label = expand.querySelector("[data-expand-label]");
    if (label) label.textContent = open ? "− Collapse" : "+ Full distribution";
  });

  // Chart PNG.
  root.addEventListener("click", (e) => {
    const save = e.target.closest("[data-save-chart]");
    if (!save) return;
    const card = save.closest("[data-family]");
    const svg = [...card.querySelectorAll(".sv-chart--histogram svg")].find(
      (s) => s.getBoundingClientRect().width > 0,
    );
    if (!svg) return;
    const rec = recording;
    const caption = `${data.title} · ${rec.label} · n=${rec.speed?.count ?? "?"} tracks${data.limit ? ` · limit ${data.limit} mph` : ""}${rec.build ? ` · pipeline ${rec.build}` : ""} · velocity.report/surveys/${data.id}/`;
    void saveChartPNG(
      svg,
      save.dataset.chartName || `${data.id}-speed`,
      caption,
    );
  });

  // Share.
  const shareStatus = root.querySelector("[data-share-status]");
  root.addEventListener("click", async (e) => {
    if (!e.target.closest("[data-share]")) return;
    const url = new URL(location.href);
    if (!compare && recording && !url.searchParams.has("recording"))
      url.searchParams.set("recording", recording.id);
    const ok = await copyText(url.toString());
    if (shareStatus) {
      shareStatus.textContent = ok ? "Link copied" : url.toString();
      if (ok) setTimeout(() => (shareStatus.textContent = ""), 2500);
    }
  });

  // Compare picker.
  const dialog = root.querySelector("[data-compare-dialog]");
  if (dialog) {
    const form = dialog.querySelector("[data-compare-form]");
    const error = dialog.querySelector("[data-compare-error]");
    if (compare) {
      for (const box of form.querySelectorAll('input[name="rec"]'))
        box.checked = compare.recordings.some((r) => r.id === box.value);
      for (const radio of form.querySelectorAll('input[name="base"]'))
        radio.checked = radio.value === compare.base.id;
    }
    root.addEventListener("click", (e) => {
      if (!e.target.closest("[data-open-compare]")) return;
      if (menu) menu.hidden = true;
      if (typeof dialog.showModal === "function") dialog.showModal();
      else dialog.setAttribute("open", "");
    });
    dialog
      .querySelector("[data-compare-cancel]")
      .addEventListener("click", () => dialog.close());
    form.addEventListener("submit", (e) => {
      const ids = [...form.querySelectorAll('input[name="rec"]:checked')].map(
        (b) => b.value,
      );
      if (ids.length < 2 || ids.length > 4) {
        e.preventDefault();
        error.textContent =
          ids.length < 2
            ? "Pick at least two recordings."
            : "Pick at most four recordings.";
        error.hidden = false;
        return;
      }
      const base = form.querySelector('input[name="base"]:checked')?.value;
      location.assign(compareSearch(ids, ids.includes(base) ? base : null));
    });
  }

  // Compare view.
  const view = root.querySelector("[data-compare-view]");
  if (compare && view) {
    for (const el of root.querySelectorAll(
      "[data-recording], [data-single-only], .sv-loc-meta",
    ))
      el.hidden = true;
    for (const el of root.querySelectorAll("#methods [data-recording-only]")) {
      el.hidden = !compare.recordings.some(
        (r) => r.id === el.dataset.recordingOnly,
      );
    }
    for (const el of root.querySelectorAll("[data-compare-only]"))
      el.hidden = false;
    for (const t of root.querySelectorAll("[data-compare-toggle]")) {
      t.textContent = "Comparing ✕";
      t.removeAttribute("data-open-compare");
      t.addEventListener("click", () => location.assign("?"));
    }
    view.innerHTML = renderCompare(data, compare);
    view.hidden = false;
    const dists = view.querySelector("[data-dists]");
    if (dists && matchMedia("(max-width: 720px)").matches) dists.open = false;
    view.querySelector("[data-compare-csv]")?.addEventListener("click", () => {
      download(
        `${data.id}-speed-comparison.csv`,
        new Blob([compareCSV(data, compare)], { type: "text/csv" }),
      );
    });
  }
}

function initListing(root) {
  const items = [...root.querySelectorAll("[data-site-row]")];
  const read = (el) => ({ el, ...el.dataset });
  const rows = items.map(read);
  const search = root.querySelector("[data-site-search]");
  const count = root.querySelector("[data-site-count]");
  const empty = root.querySelector("[data-site-empty]");
  const chips = [...root.querySelectorAll("[data-filter]")];
  const views = [...root.querySelectorAll("[data-view-button]")];
  const layout = root.querySelector("[data-survey-layout]");

  const apply = () => {
    const active = chips
      .filter((c) => c.getAttribute("aria-pressed") === "true")
      .map((c) => c.dataset.filter);
    const shown = new Set(
      filterRows(rows, search?.value, active).map((r) => r.site),
    );
    for (const r of rows) r.el.hidden = !shown.has(r.site);
    const n = shown.size;
    if (count) count.textContent = `${n} ${n === 1 ? "site" : "sites"}`;
    if (empty) empty.hidden = n > 0;
  };
  search?.addEventListener("input", apply);
  for (const chip of chips) {
    chip.addEventListener("click", () => {
      chip.setAttribute(
        "aria-pressed",
        String(chip.getAttribute("aria-pressed") !== "true"),
      );
      apply();
    });
  }

  const setView = (name) => {
    for (const b of views)
      b.setAttribute("aria-pressed", String(b.dataset.viewButton === name));
    for (const panel of root.querySelectorAll("[data-view]"))
      panel.hidden = panel.dataset.view !== name;
    layout?.classList.toggle("is-table", name === "table");
    // The map's box changes size with the layout; Leaflet re-measures on resize.
    window.dispatchEvent(new Event("resize"));
    try {
      localStorage.setItem("survey-list-view", name);
    } catch {
      // A per-browser convenience only.
    }
  };
  for (const b of views)
    b.addEventListener("click", () => setView(b.dataset.viewButton));
  try {
    const saved = localStorage.getItem("survey-list-view");
    if (saved === "table" || saved === "list") setView(saved);
  } catch {
    // Storage blocked: keep the default list view.
  }

  // Table sort.
  const tbody = root.querySelector("[data-site-tbody]");
  for (const th of root.querySelectorAll("[data-sort]")) {
    th.querySelector("button")?.addEventListener("click", () => {
      const current = th.getAttribute("aria-sort");
      const dir = current === "ascending" ? -1 : 1;
      for (const other of root.querySelectorAll("[data-sort]"))
        other.removeAttribute("aria-sort");
      th.setAttribute("aria-sort", dir === 1 ? "ascending" : "descending");
      const tableRows = [...tbody.querySelectorAll("[data-site-row]")].map(
        read,
      );
      for (const r of sortRows(tableRows, th.dataset.sort, dir))
        tbody.append(r.el);
    });
  }
  apply();
}

if (typeof document !== "undefined") {
  const page = document.querySelector("[data-survey-page]");
  if (page) initLocation(page);
  const list = document.querySelector("[data-survey-list]");
  if (list) initListing(list);
}
