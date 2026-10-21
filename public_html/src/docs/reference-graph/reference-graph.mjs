// Reference graph explorer for the public site's /docs/reference-graph/ page
// (public_html/src/docs/reference-graph/index.md).
//
// Reads the checked-in citation graph (schema version 2, written by
// citation-needed), the bibliography's section headings and the candidate
// exclusion list, and draws papers on a time axis: older work to the left,
// one lane per bibliography section. The pure functions are exported for the
// tests in public_html/src/js/__tests__/reference-graph.test.mjs; mount() is
// the page.

export const SCHEMA_VERSION = 2;
const EARLIEST_YEAR = 1600;
const REQUIRED_FIELDS = [
  "id",
  "aliases",
  "depth",
  "title",
  "authors_short",
  "year",
  "venue",
  "type",
  "status",
  "references",
  "cited_by_members",
];

// A year outside [1600, next year] is a data error, not a date.
export function cleanYear(year, nowYear) {
  if (!Number.isInteger(year)) return null;
  if (year < EARLIEST_YEAR || year > nowYear + 1) return null;
  return year;
}

export function decodeGraph(raw, { nowYear = new Date().getFullYear() } = {}) {
  if (!raw || raw.schema_version !== SCHEMA_VERSION) {
    throw new Error(
      `unsupported reference graph schema: ${raw && raw.schema_version}`,
    );
  }
  const field = Object.fromEntries(raw.node_fields.map((name, i) => [name, i]));
  for (const name of REQUIRED_FIELDS) {
    if (!(name in field))
      throw new Error(`reference graph lacks node field ${name}`);
  }
  const bit = Object.fromEntries(
    raw.status_bits.map((name, i) => [name, 1 << i]),
  );
  const nodes = raw.nodes.map((row, index) => {
    const aliases = row[field.aliases] || [];
    const bib = aliases.find((a) => a.startsWith("bibtex:"));
    const status = row[field.status] || 0;
    const refs = row[field.references];
    const venue = row[field.venue];
    const rawYear = row[field.year];
    return {
      index,
      id: row[field.id],
      aliases,
      depth: row[field.depth],
      title: row[field.title] || "",
      authors: row[field.authors_short] || "",
      year: cleanYear(rawYear, nowYear),
      rawYear: rawYear ?? null,
      venue: venue == null ? null : (raw.venues[venue] ?? null),
      type: row[field.type] || null,
      inBibliography: Boolean(status & (bit.in_bibliography || 0)),
      downloaded: Boolean(status & (bit.downloaded || 0)),
      indexed: Boolean(status & (bit.indexed || 0)),
      referencesKnown: refs ? refs[0] : "none",
      referencesCount: refs ? refs[1] : 0,
      citedByMembers: row[field.cited_by_members] || 0,
      bibKey: bib ? bib.slice("bibtex:".length) : null,
    };
  });
  const edges = raw.edges.map(([from, to, mask]) => ({
    from,
    to,
    sources: raw.edge_sources.filter((_, i) => mask & (1 << i)),
  }));
  return {
    meta: {
      generatedAt: raw.generated_at,
      generator: raw.generator,
      bibliography: raw.bibliography,
      sources: raw.sources,
    },
    nodes,
    edges,
  };
}

// out[i]: works node i cites; in[i]: works that cite node i.
export function buildAdjacency(nodeCount, edges) {
  const out = Array.from({ length: nodeCount }, () => []);
  const into = Array.from({ length: nodeCount }, () => []);
  for (const e of edges) {
    out[e.from].push(e.to);
    into[e.to].push(e.from);
  }
  return { out, in: into };
}

// Sections are comment lines framed by two `% ====` rules; the file's own
// title banner is not one. Mirrors scripts/check-bibliography-budget.py.
export function parseBibSections(text) {
  const rule = /^% =+\s*$/;
  const lines = text.split(/\r?\n/);
  const sections = new Map();
  const order = [];
  let section = null;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (
      rule.test(line) &&
      i + 2 < lines.length &&
      rule.test(lines[i + 2]) &&
      lines[i + 1].startsWith("% ")
    ) {
      const title = lines[i + 1].slice(2).trim();
      if (!title.endsWith("references.bib")) {
        section = title;
        if (!order.includes(title)) order.push(title);
      }
      continue;
    }
    const m = /^@\w+\{([^,\s]+),/.exec(line);
    if (m) sections.set(m[1], section);
  }
  return { sections, order };
}

export function parseCuration(curation) {
  const excluded = new Map();
  for (const item of (curation && curation.exclude) || []) {
    excluded.set(item.id, { reason: item.reason, note: item.note || "" });
  }
  return { excluded, reasons: (curation && curation.reasons) || {} };
}

// Candidates are works our papers cite that are not in the bibliography.
// Score = papers citing it + bibliography sections those papers come from
// (data/maths/citation-rubric.md).
export function rankCandidates(graph, adjacency, sections, excluded) {
  const ranked = [];
  for (const node of graph.nodes) {
    if (node.inBibliography) continue;
    const citing = adjacency.in[node.index]
      .map((i) => graph.nodes[i])
      .filter((n) => n.inBibliography);
    if (citing.length === 0) continue;
    const sectionCounts = new Map();
    for (const n of citing) {
      const s = sections.get(n.bibKey) || "Unsectioned";
      sectionCounts.set(s, (sectionCounts.get(s) || 0) + 1);
    }
    ranked.push({
      node,
      members: citing.length,
      sections: sectionCounts.size,
      score: citing.length + sectionCounts.size,
      sectionCounts,
      citingKeys: citing.map((n) => n.bibKey).sort(),
      exclusion: excluded.get(node.id) || null,
    });
  }
  ranked.sort(
    (a, b) =>
      b.score - a.score ||
      b.members - a.members ||
      (a.node.year ?? 9999) - (b.node.year ?? 9999) ||
      a.node.id.localeCompare(b.node.id),
  );
  return ranked;
}

// The lane a candidate sits in: the section most of its citing papers come
// from, ties to the earlier section in the bibliography.
export function dominantSection(sectionCounts, order) {
  let best = null;
  let bestCount = -1;
  for (const s of order) {
    const c = sectionCounts.get(s) || 0;
    if (c > bestCount) {
      best = s;
      bestCount = c;
    }
  }
  return bestCount > 0 ? best : order[0];
}

// The time axis is logarithmic in age, so recent years get the room: the
// `recentYears` before now take `recentShare` of the plot, everything older
// is compressed into the first `oldShare`, and the rest holds undated works.
// Age runs from the end of the current year: a year y spans ages
// (nowYear - y, nowYear + 1 - y]. `offset` softens the log near zero so the
// current year does not swallow the axis.
export function timeScale({
  minYear,
  nowYear,
  x0,
  x1,
  recentYears = 20,
  oldShare = 0.2,
  recentShare = 0.75,
  offset = 2,
}) {
  const width = x1 - x0;
  const firstRecent = nowYear - recentYears;
  const g = (age) => Math.log(age + offset);
  const recentAge = nowYear + 1 - firstRecent;
  const oldestAge = Math.max(nowYear + 1 - minYear, recentAge + 1);
  const oldEnd = x0 + oldShare * width;
  const recentEnd = oldEnd + recentShare * width;
  // x for a continuous time t (year plus fraction).
  const x = (t) => {
    const age = Math.max(0, nowYear + 1 - t);
    if (age <= recentAge) {
      return (
        recentEnd -
        ((g(age) - g(0)) / (g(recentAge) - g(0))) * (recentEnd - oldEnd)
      );
    }
    const f =
      (g(oldestAge) - g(Math.min(age, oldestAge))) /
      (g(oldestAge) - g(recentAge));
    return x0 + f * (oldEnd - x0);
  };
  return {
    x,
    yearSpan: (year) => [x(year), x(year + 1)],
    undated: [recentEnd, x1],
    firstRecent,
    minYear: Math.min(minYear, firstRecent),
    nowYear,
  };
}

// Group consecutive years into bins at least `minWidth` wide, oldest first,
// so works from years too narrow to hold a dot share one bin instead of
// overlapping. Recent years are wide enough to be bins of their own.
export function yearBins(scale, minWidth) {
  const bins = [];
  let current = null;
  for (let y = scale.minYear; y <= scale.nowYear; y++) {
    const [a, b] = scale.yearSpan(y);
    if (!current) current = { from: y, to: y, x0: a, x1: b };
    else {
      current.to = y;
      current.x1 = b;
    }
    if (current.x1 - current.x0 >= minWidth) {
      bins.push(current);
      current = null;
    }
  }
  if (current) {
    // A narrow tail joins the previous bin rather than standing alone.
    const last = bins[bins.length - 1];
    if (last) {
      last.to = current.to;
      last.x1 = current.x1;
    } else bins.push(current);
  }
  const byYear = new Map();
  for (const bin of bins)
    for (let y = bin.from; y <= bin.to; y++) byYear.set(y, bin);
  return { bins, byYear };
}

// Tick years at year starts, decades first, then fives, then single years,
// each only where it is at least `minGap` from every tick already placed.
export function pickTicks(scale, minGap = 34) {
  const years = [];
  for (let y = scale.minYear; y <= scale.nowYear; y++) years.push(y);
  const rank = (y) => (y % 10 === 0 ? 0 : y % 5 === 0 ? 1 : 2);
  const ordered = [...years].sort((a, b) => rank(a) - rank(b) || b - a);
  const placed = [];
  for (const y of ordered) {
    const px = scale.x(y);
    if (placed.every((p) => Math.abs(p.x - px) >= minGap))
      placed.push({ year: y, x: px });
  }
  return placed.sort((a, b) => a.year - b.year);
}

// Lay works out in their lane and time bin: a bin fills row by row across its
// width before it grows downwards, so a busy year spreads sideways instead of
// stacking. items: [{index, lane, x0, x1}] in drawing order; returns
// positions by index, lane tops and heights, and the total height.
export function layoutLanes(
  items,
  laneOrder,
  { spacing = 12, pad = 10, top = 0 } = {},
) {
  const byLane = new Map(laneOrder.map((l) => [l, []]));
  for (const item of items) {
    if (!byLane.has(item.lane)) byLane.set(item.lane, []);
    byLane.get(item.lane).push(item);
  }
  const positions = new Map();
  const lanes = [];
  let y = top;
  for (const [lane, laneItems] of byLane) {
    if (laneItems.length === 0) continue;
    const groups = new Map();
    for (const item of laneItems) {
      const key = `${item.x0}:${item.x1}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(item);
    }
    let rows = 1;
    for (const group of groups.values()) {
      const { x0, x1 } = group[0];
      const cols = Math.max(1, Math.floor((x1 - x0) / spacing));
      const used = Math.min(group.length, cols);
      const centre = (x0 + x1) / 2;
      group.forEach((item, k) => {
        const col = k % cols;
        const row = Math.floor(k / cols);
        rows = Math.max(rows, row + 1);
        positions.set(item.index, {
          x: centre + (col - (used - 1) / 2) * spacing,
          y: y + pad + row * spacing,
        });
      });
    }
    const height = pad * 2 + (rows - 1) * spacing;
    lanes.push({ lane, top: y, height });
    y += height;
  }
  return { positions, lanes, height: y - top };
}

// Move `lane` one place up (-1) or down (+1) in `order`; returns a new array.
export function moveLane(order, lane, step) {
  const i = order.indexOf(lane);
  const j = i + step;
  if (i < 0 || j < 0 || j >= order.length) return [...order];
  const next = [...order];
  [next[i], next[j]] = [next[j], next[i]];
  return next;
}

// A saved lane order, kept only where it still names the current sections:
// dropped sections disappear, new ones join at the end in bibliography order.
export function restoreLaneOrder(saved, lanes) {
  const known = new Set(lanes);
  const kept = Array.isArray(saved) ? saved.filter((l) => known.has(l)) : [];
  const seen = new Set(kept);
  return [...kept, ...lanes.filter((l) => !seen.has(l))];
}

export function externalLinks(node) {
  const links = [];
  const ids = [node.id, ...node.aliases];
  const doi = ids.find((a) => a.startsWith("doi:"));
  const openalex = ids.find((a) => a.startsWith("openalex:"));
  const arxiv = ids.find((a) => a.startsWith("arxiv:"));
  if (doi)
    links.push({ label: "DOI", href: `https://doi.org/${doi.slice(4)}` });
  if (arxiv)
    links.push({
      label: "arXiv",
      href: `https://arxiv.org/abs/${arxiv.slice(6)}`,
    });
  if (openalex)
    links.push({
      label: "OpenAlex",
      href: `https://openalex.org/${openalex.slice(9)}`,
    });
  const query = encodeURIComponent(
    [node.title, node.authors].filter(Boolean).join(" "),
  );
  if (node.title) {
    links.push({
      label: "Semantic Scholar",
      href: `https://www.semanticscholar.org/search?q=${encodeURIComponent(node.title)}`,
    });
    links.push({
      label: "Google Scholar",
      href: `https://scholar.google.com/scholar?q=${query}`,
    });
  }
  return links;
}

export function searchNodes(nodes, query, limit = 20) {
  const q = query.trim().toLowerCase();
  if (q.length < 2) return [];
  const hits = [];
  for (const n of nodes) {
    const hay =
      `${n.title} ${n.authors} ${n.bibKey || ""} ${n.id} ${n.year ?? ""}`.toLowerCase();
    if (hay.includes(q)) {
      hits.push(n);
      if (hits.length >= limit * 4) break;
    }
  }
  hits.sort(
    (a, b) =>
      Number(b.inBibliography) - Number(a.inBibliography) ||
      b.citedByMembers - a.citedByMembers,
  );
  return hits.slice(0, limit);
}

export function describe(node) {
  const who = node.authors || "Unknown authors";
  const when = node.year ?? "n.d.";
  return `${who} (${when}). ${node.title || node.id}`;
}

// ---------------------------------------------------------------- page ----

const SVG_NS = "http://www.w3.org/2000/svg";
const STYLE = `
.refgraph { --rg-gap: 0.75rem; display: grid; gap: var(--rg-gap);
  /* Own palette, matching the public site's Tailwind greys and emerald, so the
     graph does not depend on which page's stylesheet defines what. */
  --rg-bg: #ffffff; --rg-panel: #f9fafb; --rg-line: #e5e7eb; --rg-text: #111827;
  --rg-muted: #6b7280; --rg-accent: #059669; --rg-accent-soft: #d1fae5; }
@media (prefers-color-scheme: dark) {
  .refgraph { --rg-bg: #111827; --rg-panel: #1f2937; --rg-line: #374151; --rg-text: #f3f4f6;
    --rg-muted: #9ca3af; --rg-accent: #34d399; --rg-accent-soft: #064e3b; }
}
.refgraph-controls { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: center;
  padding: 0.6rem 0.8rem; border: 1px solid var(--rg-line); border-radius: 8px; background: var(--rg-panel); }
.refgraph-controls label { display: inline-flex; gap: 0.35rem; align-items: center; font-size: 0.9rem; }
.refgraph-controls input[type="number"] { width: 4rem; }
.refgraph-controls select, .refgraph-controls input { font: inherit; color: var(--rg-text); background: var(--rg-bg);
  border: 1px solid var(--rg-line); border-radius: 6px; padding: 0.15rem 0.35rem; }
.refgraph-summary { color: var(--rg-muted); font-size: 0.85rem; margin: 0; }
.refgraph-body { display: grid; grid-template-columns: minmax(0, 1fr) 24rem; gap: var(--rg-gap); align-items: start; }
@media (max-width: 1100px) { .refgraph-body { grid-template-columns: minmax(0, 1fr); } }
.refgraph-canvas { overflow-x: auto; overflow-y: hidden; border: 1px solid var(--rg-line); border-radius: 8px; background: var(--rg-panel); }
.refgraph-canvas svg { display: block; }
.refgraph-panel { border: 1px solid var(--rg-line); border-radius: 8px; background: var(--rg-panel); padding: 0.8rem;
  font-size: 0.9rem; position: sticky; top: 0.5rem; max-height: calc(100vh - 1rem); overflow: auto; }
.refgraph-panel h3 { margin: 0 0 0.4rem; font-size: 1rem; }
.refgraph-panel ul { padding-left: 1.1rem; margin: 0.3rem 0 0.8rem; }
.refgraph-panel li { margin-bottom: 0.3rem; line-height: 1.35; }
.refgraph-panel button.rg-link { all: unset; cursor: pointer; color: var(--rg-accent); }
.refgraph-panel button.rg-link:focus-visible { outline: 2px solid var(--rg-accent); }
.rg-badges { display: flex; flex-wrap: wrap; gap: 0.3rem; margin: 0.3rem 0 0.6rem; }
.rg-badge { font-size: 0.75rem; padding: 0.05rem 0.45rem; border-radius: 999px; border: 1px solid var(--rg-line); color: var(--rg-muted); }
.rg-badge.on { background: var(--rg-accent-soft); color: var(--rg-text); border-color: var(--rg-accent); }
.rg-node { cursor: pointer; }
.rg-node:focus { outline: none; }
.rg-node:focus-visible circle, .rg-node.selected circle { stroke-width: 3; }
.rg-lane-label { font-size: 11px; fill: var(--rg-muted); }
.rg-lane-move { font-size: 11px; fill: var(--rg-muted); cursor: pointer; }
.rg-lane-move:hover, .rg-lane-move:focus-visible { fill: var(--rg-accent); outline: none; }
.rg-node.dim { opacity: 0.22; }
.rg-tick { font-size: 11px; fill: var(--rg-muted); }
.refgraph-table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
.refgraph-table th, .refgraph-table td { text-align: left; padding: 0.25rem 0.4rem; border-bottom: 1px solid var(--rg-line); vertical-align: top; }
.refgraph-table td.num { text-align: right; font-variant-numeric: tabular-nums; }
.refgraph-legend { display: flex; flex-wrap: wrap; gap: 0.4rem 1rem; font-size: 0.85rem; color: var(--rg-muted); }
.refgraph-legend svg { vertical-align: middle; }
`;

function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "text") node.textContent = v;
    else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
    else node.setAttribute(k, v === true ? "" : String(v));
  }
  for (const c of children) if (c) node.append(c);
  return node;
}

function svg(tag, attrs = {}) {
  const node = document.createElementNS(SVG_NS, tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v !== undefined && v !== null) node.setAttribute(k, String(v));
  }
  return node;
}

async function fetchJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.json();
}

async function fetchText(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.text();
}

export async function mount(root) {
  const resolve = (attr) =>
    new URL(root.getAttribute(attr), document.baseURI).href;
  if (!document.getElementById("refgraph-style")) {
    document.head.append(el("style", { id: "refgraph-style", text: STYLE }));
  }
  root.textContent = "Loading the reference graph…";
  let raw, bibText, curationRaw;
  try {
    [raw, bibText, curationRaw] = await Promise.all([
      fetchJSON(resolve("data-graph")),
      fetchText(resolve("data-bib")),
      fetchJSON(resolve("data-curation")),
    ]);
  } catch (err) {
    root.textContent = `The reference graph could not be loaded: ${err.message}`;
    return;
  }
  const graph = decodeGraph(raw);
  const adjacency = buildAdjacency(graph.nodes.length, graph.edges);
  const { sections, order } = parseBibSections(bibText);
  const { excluded, reasons } = parseCuration(curationRaw);
  const candidates = rankCandidates(graph, adjacency, sections, excluded);
  const candidateByIndex = new Map(candidates.map((c) => [c.node.index, c]));
  const members = graph.nodes.filter((n) => n.inBibliography);
  const lanes = [...order];
  if (members.some((n) => !sections.get(n.bibKey))) lanes.push("Unsectioned");

  // A reader's own section order, kept in this browser only.
  const LANE_ORDER_KEY = "refgraph-lane-order";
  let savedOrder = null;
  try {
    savedOrder = JSON.parse(localStorage.getItem(LANE_ORDER_KEY));
  } catch {
    savedOrder = null;
  }

  const state = {
    laneOrder: restoreLaneOrder(savedOrder, lanes),
    showMembers: true,
    showCandidates: true,
    includeExcluded: false,
    minScore: 5,
    section: "all",
    memberLinks: false,
    selected: null,
    pinned: new Set(),
  };

  root.textContent = "";
  root.classList.add("refgraph");

  // Controls.
  const sectionSelect = el("select", { "aria-label": "Bibliography section" });
  const fillSections = () => {
    sectionSelect.textContent = "";
    sectionSelect.append(
      el("option", { value: "all", text: "All sections" }),
      ...state.laneOrder.map((s) => el("option", { value: s, text: s })),
    );
    sectionSelect.value = state.section;
  };
  fillSections();
  const moveSection = (lane, step) => {
    state.laneOrder = moveLane(state.laneOrder, lane, step);
    try {
      localStorage.setItem(LANE_ORDER_KEY, JSON.stringify(state.laneOrder));
    } catch {
      // Private windows and blocked storage: the order lasts this visit.
    }
    fillSections();
    render();
  };
  const resetOrder = el("button", {
    type: "button",
    text: "Reset section order",
    onclick: () => {
      state.laneOrder = [...lanes];
      try {
        localStorage.removeItem(LANE_ORDER_KEY);
      } catch {
        // Nothing saved to remove.
      }
      fillSections();
      render();
    },
  });
  const minScore = el("input", {
    type: "number",
    min: 2,
    max: 30,
    value: state.minScore,
    "aria-label": "Minimum candidate score",
  });
  const search = el("input", {
    type: "search",
    placeholder: "Search title, author, key, DOI",
    "aria-label": "Search papers",
    size: 30,
  });
  const checkbox = (label, key) => {
    const box = el("input", { type: "checkbox" });
    box.checked = state[key];
    box.addEventListener("change", () => {
      state[key] = box.checked;
      render();
    });
    return el("label", {}, box, document.createTextNode(label));
  };
  sectionSelect.addEventListener("change", () => {
    state.section = sectionSelect.value;
    render();
  });
  minScore.addEventListener("change", () => {
    state.minScore = Math.max(2, Number(minScore.value) || 2);
    render();
  });
  const results = el("ul", {
    class: "refgraph-results",
    "aria-live": "polite",
  });
  search.addEventListener("input", () => {
    results.textContent = "";
    for (const n of searchNodes(graph.nodes, search.value)) {
      results.append(el("li", {}, nodeButton(n)));
    }
  });

  const controls = el(
    "div",
    { class: "refgraph-controls" },
    checkbox("Our papers (references.bib)", "showMembers"),
    checkbox("Candidates to read next", "showCandidates"),
    el("label", {}, document.createTextNode("Minimum score"), minScore),
    checkbox("Include excluded candidates", "includeExcluded"),
    checkbox("Links between our papers", "memberLinks"),
    sectionSelect,
    resetOrder,
    search,
  );
  const summary = el("p", { class: "refgraph-summary" });
  const legend = el(
    "div",
    { class: "refgraph-legend" },
    legendItem("member", "In references.bib, downloaded and indexed"),
    legendItem("member-missing", "In references.bib, no indexed copy"),
    legendItem("candidate", "Candidate; size grows with score"),
    legendItem("excluded", "Excluded candidate (rubric)"),
  );
  const canvas = el("div", {
    class: "refgraph-canvas",
    tabindex: "0",
    "aria-label": "Papers by year and section",
  });
  const panel = el("div", { class: "refgraph-panel", "aria-live": "polite" });
  const tableBody = el("tbody");
  const table = el(
    "table",
    { class: "refgraph-table" },
    el(
      "thead",
      {},
      el(
        "tr",
        {},
        el("th", { text: "Score" }),
        el("th", { text: "Papers" }),
        el("th", { text: "Sections" }),
        el("th", { text: "Year" }),
        el("th", { text: "Work" }),
      ),
    ),
    tableBody,
  );
  root.append(
    controls,
    results,
    summary,
    legend,
    el("div", { class: "refgraph-body" }, canvas, panel),
    el("h3", { text: "Read next" }),
    table,
  );

  function legendItem(kind, text) {
    const s = svg("svg", { width: 16, height: 16, "aria-hidden": "true" });
    s.append(nodeShape(kind, 8, 8, kind === "candidate" ? 5 : 5));
    return el("span", {}, s, document.createTextNode(` ${text}`));
  }

  function nodeShape(kind, x, y, r) {
    const c = svg("circle", { cx: x, cy: y, r });
    if (kind === "member")
      c.setAttribute(
        "style",
        "fill: var(--rg-accent); stroke: var(--rg-text); stroke-width: 1",
      );
    else if (kind === "member-missing")
      c.setAttribute(
        "style",
        "fill: var(--rg-panel); stroke: var(--rg-accent); stroke-width: 2",
      );
    else if (kind === "excluded")
      c.setAttribute(
        "style",
        "fill: none; stroke: var(--rg-muted); stroke-width: 1; stroke-dasharray: 2 2",
      );
    else
      c.setAttribute(
        "style",
        "fill: var(--rg-accent-soft); stroke: var(--rg-text); stroke-width: 1; opacity: 0.85",
      );
    return c;
  }

  function kindOf(node) {
    if (node.inBibliography)
      return node.indexed && node.downloaded ? "member" : "member-missing";
    const c = candidateByIndex.get(node.index);
    return c && c.exclusion ? "excluded" : "candidate";
  }

  function laneOf(node, fallback = state.laneOrder[0]) {
    if (node.inBibliography) return sections.get(node.bibKey) || "Unsectioned";
    const c = candidateByIndex.get(node.index);
    return c ? dominantSection(c.sectionCounts, state.laneOrder) : fallback;
  }

  // The selected work and everything it cites or is cited by, so its links
  // can always be drawn, whatever the filters hide.
  function neighbourhood() {
    if (state.selected == null) return null;
    return new Set([
      state.selected,
      ...adjacency.out[state.selected],
      ...adjacency.in[state.selected],
    ]);
  }

  function visibleNodes() {
    const out = [];
    const inSection = (node) => {
      if (state.section === "all") return true;
      if (node.inBibliography) return laneOf(node) === state.section;
      const c = candidateByIndex.get(node.index);
      return Boolean(c && c.sectionCounts.has(state.section));
    };
    if (state.showMembers)
      for (const n of members) if (inSection(n)) out.push(n);
    if (state.showCandidates) {
      for (const c of candidates) {
        if (c.score < state.minScore) break;
        if (c.exclusion && !state.includeExcluded) continue;
        if (inSection(c.node)) out.push(c.node);
      }
    }
    const shown = new Set(out.map((n) => n.index));
    const extra = [...state.pinned, ...(neighbourhood() || [])];
    for (const i of extra) {
      if (shown.has(i)) continue;
      shown.add(i);
      out.push(graph.nodes[i]);
    }
    return out;
  }

  function nodeButton(node) {
    return el("button", {
      type: "button",
      class: "rg-link",
      text: describe(node),
      onclick: () => select(node.index, true),
    });
  }

  function select(index, pin = false) {
    state.selected = index;
    if (pin) state.pinned.add(index);
    render();
    const target = canvas.querySelector(`[data-index="${index}"]`);
    if (target && target.scrollIntoView)
      target.scrollIntoView({ block: "nearest" });
  }

  function render() {
    const nodes = visibleNodes();
    const near = neighbourhood();
    const selectedLane =
      state.selected == null ? undefined : laneOf(graph.nodes[state.selected]);
    const years = nodes.filter((n) => n.year != null).map((n) => n.year);
    const nowYear = Math.max(new Date().getFullYear(), ...years);
    const minYear = years.length ? Math.min(...years) : nowYear - 50;
    const labelWidth = 210;
    // Fit the canvas; below 640px the plot scrolls sideways instead.
    const width = Math.max(640, canvas.clientWidth || 900);
    const scale = timeScale({
      minYear,
      nowYear,
      x0: labelWidth,
      x1: width - 8,
    });
    const { byYear } = yearBins(scale, 12);
    const ticks = pickTicks(scale);
    const [ndStart, ndEnd] = scale.undated;
    const items = nodes.map((n) => {
      const bin = n.year == null ? null : byYear.get(n.year);
      return {
        index: n.index,
        lane: laneOf(n, selectedLane),
        x0: bin ? bin.x0 : ndStart,
        x1: bin ? bin.x1 : ndEnd,
      };
    });
    const axisHeight = 28;
    const layout = layoutLanes(items, state.laneOrder, { top: axisHeight });
    const height = axisHeight + layout.height + 8;

    const root2 = svg("svg", {
      width,
      height,
      role: "img",
      "aria-label": "Citation graph by year",
    });
    // Lanes, each with controls to move it up or down.
    layout.lanes.forEach((lane, i) => {
      root2.append(
        svg("rect", {
          x: 0,
          y: lane.top,
          width,
          height: lane.height,
          style: `fill: ${i % 2 ? "var(--rg-bg)" : "var(--rg-panel)"}`,
        }),
      );
      const position = state.laneOrder.indexOf(lane.lane);
      const mover = (glyph, step, x, label) => {
        const disabled =
          (step < 0 && position === 0) ||
          (step > 0 && position === state.laneOrder.length - 1);
        if (disabled) return;
        const t = svg("text", {
          x,
          y: lane.top + 14,
          class: "rg-lane-move",
          role: "button",
          tabindex: 0,
          "aria-label": `${label}: ${lane.lane}`,
        });
        t.textContent = glyph;
        t.addEventListener("click", () => moveSection(lane.lane, step));
        t.addEventListener("keydown", (ev) => {
          if (ev.key === "Enter" || ev.key === " ") {
            ev.preventDefault();
            moveSection(lane.lane, step);
          }
        });
        root2.append(t);
      };
      mover("▲", -1, 6, "Move section up");
      mover("▼", 1, 18, "Move section down");
      const label = svg("text", {
        x: 32,
        y: lane.top + 14,
        class: "rg-lane-label",
      });
      label.textContent =
        lane.lane.length > 30 ? `${lane.lane.slice(0, 29)}…` : lane.lane;
      const full = svg("title");
      full.textContent = lane.lane;
      label.append(full);
      root2.append(label);
    });
    // Year ticks at year starts, and the undated column.
    for (const t of ticks) {
      root2.append(
        svg("line", {
          x1: t.x,
          x2: t.x,
          y1: 20,
          y2: height - 4,
          style: "stroke: var(--rg-line); stroke-width: 1",
        }),
      );
      const label = svg("text", { x: t.x + 2, y: 14, class: "rg-tick" });
      label.textContent = String(t.year);
      root2.append(label);
    }
    root2.append(
      svg("line", {
        x1: ndStart,
        x2: ndStart,
        y1: 20,
        y2: height - 4,
        style: "stroke: var(--rg-line); stroke-width: 1; stroke-dasharray: 2 3",
      }),
    );
    const nd = svg("text", { x: ndStart + 4, y: 14, class: "rg-tick" });
    nd.textContent = "n.d.";
    root2.append(nd);

    // Edges: the selected work's references (solid) and citers (dashed), and
    // optionally every link between our papers.
    const edgeLayer = svg("g", { "aria-hidden": "true" });
    const drawEdge = (from, to, dashed) => {
      const a = layout.positions.get(from);
      const b = layout.positions.get(to);
      if (!a || !b) return;
      edgeLayer.append(
        svg("line", {
          x1: a.x,
          y1: a.y,
          x2: b.x,
          y2: b.y,
          style: `stroke: var(--rg-accent); stroke-width: 1; opacity: 0.55${dashed ? "; stroke-dasharray: 4 3" : ""}`,
        }),
      );
    };
    if (state.memberLinks) {
      for (const m of members) {
        for (const to of adjacency.out[m.index]) {
          if (graph.nodes[to].inBibliography) drawEdge(m.index, to, false);
        }
      }
    }
    if (state.selected != null) {
      for (const to of adjacency.out[state.selected])
        drawEdge(state.selected, to, false);
      for (const from of adjacency.in[state.selected])
        drawEdge(from, state.selected, true);
    }
    root2.append(edgeLayer);

    // Nodes.
    for (const n of nodes) {
      const pos = layout.positions.get(n.index);
      const kind = kindOf(n);
      const c = candidateByIndex.get(n.index);
      const r = n.inBibliography ? 5 : 3 + Math.min(c ? c.score : 2, 14) * 0.35;
      const g = svg("g", {
        class: `rg-node${state.selected === n.index ? " selected" : ""}${near && !near.has(n.index) ? " dim" : ""}`,
        tabindex: 0,
        role: "button",
        "data-index": n.index,
        "aria-label": describe(n),
      });
      g.append(nodeShape(kind, pos.x, pos.y, r));
      const tip = svg("title");
      tip.textContent = describe(n) + (c ? ` | score ${c.score}` : "");
      g.append(tip);
      g.addEventListener("click", () => select(n.index));
      g.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" || ev.key === " ") {
          ev.preventDefault();
          select(n.index);
        }
      });
      root2.append(g);
    }

    canvas.textContent = "";
    canvas.append(root2);

    const shownCandidates = nodes.filter((n) => !n.inBibliography).length;
    const shownMembers = nodes.length - shownCandidates;
    summary.textContent =
      `${shownMembers} of our ${members.length} papers and ${shownCandidates} candidates shown. ` +
      `Graph of ${graph.nodes.length.toLocaleString()} works and ${graph.edges.length.toLocaleString()} links, ` +
      `built ${String(graph.meta.generatedAt).slice(0, 10)}.`;

    renderPanel();
    renderTable();
  }

  function renderPanel() {
    panel.textContent = "";
    if (state.selected == null) {
      panel.append(
        el("p", {
          text: "Select a paper to see what it cites, what cites it, and where to find it.",
        }),
      );
      return;
    }
    const n = graph.nodes[state.selected];
    const c = candidateByIndex.get(n.index);
    panel.append(el("h3", { text: n.title || n.id }));
    panel.append(
      el("p", {
        text: `${n.authors || "Unknown authors"}, ${n.year ?? "n.d."}${n.venue ? `. ${n.venue}` : ""}`,
      }),
    );
    const badge = (on, text) =>
      el("span", { class: `rg-badge${on ? " on" : ""}`, text });
    panel.append(
      el(
        "div",
        { class: "rg-badges" },
        badge(
          n.inBibliography,
          n.bibKey ? `references.bib: ${n.bibKey}` : "not in references.bib",
        ),
        badge(n.downloaded, n.downloaded ? "downloaded" : "not downloaded"),
        badge(n.indexed, n.indexed ? "indexed" : "not indexed"),
        badge(
          n.referencesKnown !== "none",
          `reference list: ${n.referencesKnown}`,
        ),
      ),
    );
    if (c) {
      const parts = `${c.members} of our papers (${c.citingKeys.join(", ")}) in ${c.sections} section${c.sections === 1 ? "" : "s"}`;
      panel.append(
        el("p", { text: `Candidate score ${c.score}: cited by ${parts}.` }),
      );
      if (c.exclusion) {
        panel.append(
          el("p", {
            text: `Excluded from the reading list: ${reasons[c.exclusion.reason] || c.exclusion.reason}.`,
          }),
        );
      }
    }
    if (n.rawYear != null && n.year == null) {
      panel.append(
        el("p", {
          text: `The graph records the year ${n.rawYear}, which is not a plausible date; shown as n.d.`,
        }),
      );
    }
    const links = el("p", {});
    externalLinks(n).forEach((l, i) => {
      if (i) links.append(document.createTextNode(" · "));
      links.append(
        el("a", {
          href: l.href,
          rel: "noopener noreferrer",
          target: "_blank",
          text: l.label,
        }),
      );
    });
    panel.append(links);
    panel.append(el("p", { class: "refgraph-summary", text: n.id }));
    const list = (title, indices) => {
      if (!indices.length) return;
      const sorted = indices
        .map((i) => graph.nodes[i])
        .sort(
          (a, b) =>
            Number(b.inBibliography) - Number(a.inBibliography) ||
            (a.year ?? 9999) - (b.year ?? 9999),
        );
      panel.append(el("h3", { text: `${title} (${indices.length})` }));
      const ul = el("ul");
      for (const m of sorted.slice(0, 60))
        ul.append(el("li", {}, nodeButton(m)));
      if (sorted.length > 60)
        ul.append(el("li", { text: `…and ${sorted.length - 60} more` }));
      panel.append(ul);
    };
    // For an older work the interesting direction is what cites it.
    if (n.inBibliography) {
      list("Cites", adjacency.out[n.index]);
      list("Cited by", adjacency.in[n.index]);
    } else {
      list("Cited by", adjacency.in[n.index]);
      list("Cites", adjacency.out[n.index]);
    }
  }

  function renderTable() {
    tableBody.textContent = "";
    let shown = 0;
    for (const c of candidates) {
      if (c.exclusion && !state.includeExcluded) continue;
      if (state.section !== "all" && !c.sectionCounts.has(state.section))
        continue;
      tableBody.append(
        el(
          "tr",
          {},
          el("td", { class: "num", text: c.score }),
          el("td", { class: "num", text: c.members }),
          el("td", { class: "num", text: c.sections }),
          el("td", { class: "num", text: c.node.year ?? "n.d." }),
          el(
            "td",
            {},
            nodeButton(c.node),
            c.exclusion
              ? el("span", { class: "rg-badge", text: c.exclusion.reason })
              : null,
          ),
        ),
      );
      if (++shown >= 40) break;
    }
  }

  render();

  // The axis is laid out for the canvas width, so redraw when it changes:
  // on window resizes, and through a ResizeObserver for layout changes the
  // window does not see. A timer debounces both (requestAnimationFrame is
  // paused in background tabs).
  let lastWidth = canvas.clientWidth;
  let pending = false;
  const redrawIfResized = () => {
    if (pending) return;
    pending = true;
    setTimeout(() => {
      pending = false;
      if (canvas.clientWidth === lastWidth) return;
      lastWidth = canvas.clientWidth;
      render();
    }, 120);
  };
  window.addEventListener("resize", redrawIfResized);
  if (typeof ResizeObserver !== "undefined") {
    new ResizeObserver(redrawIfResized).observe(canvas);
  }
}

if (typeof document !== "undefined") {
  for (const root of document.querySelectorAll("[data-reference-graph]"))
    mount(root);
}
