// Tests for the reference graph page (/docs/reference-graph/). Pure
// functions only: decoding, layout, ranking and links, no DOM.

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../..",
);
const pageDir = path.join(repoRoot, "public_html/src/docs/reference-graph");
const load = () => import("../../docs/reference-graph/reference-graph.mjs");

// A schema-2 graph in miniature: two of our papers (A cites B and C; B cites
// C and D), C and D outside the bibliography, D with an impossible year.
function fixture() {
  return {
    schema_version: 2,
    generated_at: "2026-10-20T00:00:00Z",
    generator: { name: "citation-needed" },
    bibliography: { path: "data/maths/references.bib" },
    sources: [],
    node_fields: [
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
    ],
    edge_fields: ["from", "to", "sources"],
    edge_sources: ["openalex", "crossref", "opencitations"],
    status_bits: ["indexed", "downloaded", "in_bibliography"],
    venues: ["Journal A"],
    nodes: [
      [
        "doi:10.1/a",
        ["bibtex:Alpha2020", "openalex:W1"],
        0,
        "Alpha",
        "Alpha et al.",
        2020,
        0,
        "article",
        7,
        ["api", 2],
        0,
      ],
      [
        "doi:10.1/b",
        ["bibtex:Beta1999", "arxiv:1234.5678"],
        0,
        "Beta",
        "Beta",
        1999,
        null,
        "article",
        4,
        ["extracted", 2],
        1,
      ],
      [
        "doi:10.1/c",
        ["openalex:W3"],
        1,
        "Gamma",
        "Gamma",
        1955,
        null,
        null,
        0,
        null,
        2,
      ],
      ["fp:abcd", [], 1, "Delta", "", 5555, null, null, 0, null, 1],
    ],
    edges: [
      [0, 1, 1],
      [0, 2, 3],
      [1, 2, 4],
      [1, 3, 1],
    ],
  };
}

const BIB = [
  "% ====",
  "% data/maths/references.bib",
  "% ====",
  "",
  "% ====",
  "% Tracking",
  "% ====",
  "",
  "@article{Alpha2020,",
  "}",
  "% ====",
  "% Safety",
  "% ====",
  "",
  "@article{Beta1999,",
  "}",
].join("\n");

test("decodeGraph unpacks rows, bitmasks, venues and sources", async () => {
  const { decodeGraph } = await load();
  const g = decodeGraph(fixture(), { nowYear: 2026 });
  assert.equal(g.nodes.length, 4);
  const [a, b, c, d] = g.nodes;
  assert.deepEqual(
    [a.inBibliography, a.downloaded, a.indexed, a.bibKey, a.venue],
    [true, true, true, "Alpha2020", "Journal A"],
  );
  assert.deepEqual(
    [b.inBibliography, b.downloaded, b.indexed],
    [true, false, false],
  );
  assert.equal(b.referencesKnown, "extracted");
  assert.equal(c.referencesKnown, "none");
  assert.equal(c.inBibliography, false);
  assert.equal(d.year, null);
  assert.equal(d.rawYear, 5555);
  assert.deepEqual(g.edges[1], {
    from: 0,
    to: 2,
    sources: ["openalex", "crossref"],
  });
  assert.deepEqual(g.edges[2].sources, ["opencitations"]);
});

test("decodeGraph refuses other schema versions and missing fields", async () => {
  const { decodeGraph } = await load();
  assert.throws(
    () => decodeGraph({ ...fixture(), schema_version: 1 }),
    /schema: 1/,
  );
  const raw = fixture();
  raw.node_fields = raw.node_fields.filter((f) => f !== "status");
  assert.throws(() => decodeGraph(raw), /node field status/);
});

test("cleanYear keeps plausible years only", async () => {
  const { cleanYear } = await load();
  assert.equal(cleanYear(1818, 2026), 1818);
  assert.equal(cleanYear(2027, 2026), 2027);
  assert.equal(cleanYear(2028, 2026), null);
  assert.equal(cleanYear(1200, 2026), null);
  assert.equal(cleanYear(null, 2026), null);
});

test("parseBibSections skips the title banner and keeps file order", async () => {
  const { parseBibSections } = await load();
  const { sections, order } = parseBibSections(BIB);
  assert.deepEqual(order, ["Tracking", "Safety"]);
  assert.equal(sections.get("Alpha2020"), "Tracking");
  assert.equal(sections.get("Beta1999"), "Safety");
});

test("rankCandidates scores papers plus sections and marks exclusions", async () => {
  const {
    decodeGraph,
    buildAdjacency,
    parseBibSections,
    parseCuration,
    rankCandidates,
  } = await load();
  const g = decodeGraph(fixture(), { nowYear: 2026 });
  const adj = buildAdjacency(g.nodes.length, g.edges);
  const { sections } = parseBibSections(BIB);
  const { excluded } = parseCuration({
    exclude: [{ id: "fp:abcd", reason: "generic-ml" }],
  });
  const ranked = rankCandidates(g, adj, sections, excluded);
  assert.deepEqual(
    ranked.map((c) => c.node.id),
    ["doi:10.1/c", "fp:abcd"],
  );
  const [gamma, delta] = ranked;
  assert.deepEqual([gamma.members, gamma.sections, gamma.score], [2, 2, 4]);
  assert.deepEqual(gamma.citingKeys, ["Alpha2020", "Beta1999"]);
  assert.equal(gamma.exclusion, null);
  assert.deepEqual([delta.score, delta.exclusion.reason], [2, "generic-ml"]);
});

test("dominantSection picks the most-citing section, ties to the earlier", async () => {
  const { dominantSection } = await load();
  const order = ["Tracking", "Safety", "Maps"];
  assert.equal(
    dominantSection(
      new Map([
        ["Safety", 2],
        ["Tracking", 1],
      ]),
      order,
    ),
    "Safety",
  );
  assert.equal(
    dominantSection(
      new Map([
        ["Maps", 1],
        ["Safety", 1],
      ]),
      order,
    ),
    "Safety",
  );
  assert.equal(dominantSection(new Map(), order), "Tracking");
});

test("timeScale gives the last 20 years 75% and older years 20%", async () => {
  const { timeScale } = await load();
  const scale = timeScale({ minYear: 1936, nowYear: 2026, x0: 0, x1: 1000 });
  const near = (a, b) => Math.abs(a - b) < 1e-6;
  assert.ok(near(scale.x(1936), 0), "oldest work at the left edge");
  assert.ok(near(scale.x(2006), 200), "pre-2006 in the first fifth");
  assert.ok(near(scale.x(2027), 950), "the recent span ends at 95%");
  assert.deepEqual(scale.undated, [950, 1000]);
  for (let y = 1936; y < 2027; y++) {
    assert.ok(scale.x(y) < scale.x(y + 1), `monotonic at ${y}`);
  }
  // Logarithmic: later years are wider, and the last five take a large share.
  const widthOf = (y) => scale.x(y + 1) - scale.x(y);
  assert.ok(widthOf(2026) > widthOf(2016) && widthOf(2016) > widthOf(2006));
  const lastFive = scale.x(2027) - scale.x(2022);
  assert.ok(lastFive > 0.4 * 750, `last five years ${lastFive}px of 750`);
});

test("timeScale keeps its shares when nothing is old", async () => {
  const { timeScale } = await load();
  const scale = timeScale({ minYear: 2015, nowYear: 2026, x0: 100, x1: 1100 });
  assert.equal(scale.minYear, 2006);
  assert.ok(Math.abs(scale.x(2006) - 300) < 1e-6);
});

test("yearBins merges years too narrow for a dot, keeps recent years apart", async () => {
  const { timeScale, yearBins } = await load();
  const scale = timeScale({ minYear: 1936, nowYear: 2026, x0: 0, x1: 1400 });
  const { bins, byYear } = yearBins(scale, 12);
  assert.equal(bins[0].from, 1936);
  assert.equal(bins[bins.length - 1].to, 2026);
  for (const bin of bins)
    assert.ok(bin.x1 - bin.x0 >= 12, `bin ${bin.from}-${bin.to}`);
  for (let i = 1; i < bins.length; i++)
    assert.equal(bins[i].from, bins[i - 1].to + 1);
  assert.ok(
    byYear.get(1950).to - byYear.get(1950).from > 0,
    "old years share a bin",
  );
  assert.equal(byYear.get(2025).from, 2025, "a recent year is its own bin");
});

test("pickTicks prefers decades and never crowds labels", async () => {
  const { timeScale, pickTicks } = await load();
  const scale = timeScale({ minYear: 1936, nowYear: 2026, x0: 0, x1: 1400 });
  const ticks = pickTicks(scale, 34);
  const years = ticks.map((t) => t.year);
  assert.ok(years.includes(2020) && years.includes(2026));
  for (let i = 1; i < ticks.length; i++) {
    assert.ok(ticks[i].x - ticks[i - 1].x >= 34, `gap before ${ticks[i].year}`);
  }
});

test("layoutLanes spreads a bin sideways before stacking, skips empty lanes", async () => {
  const { layoutLanes } = await load();
  const items = [];
  for (let i = 0; i < 7; i++)
    items.push({ index: i, lane: "Tracking", x0: 100, x1: 160 });
  items.push({ index: 7, lane: "Safety", x0: 300, x1: 305 });
  const { positions, lanes, height } = layoutLanes(
    items,
    ["Tracking", "Empty", "Safety"],
    {
      spacing: 12,
      pad: 10,
    },
  );
  assert.deepEqual(
    lanes.map((l) => l.lane),
    ["Tracking", "Safety"],
  );
  // 60px fits five 12px columns: a row of five, then a row of two.
  const row0 = [0, 1, 2, 3, 4].map((i) => positions.get(i));
  assert.ok(row0.every((p) => p.y === 10));
  assert.equal(positions.get(5).y, 22);
  for (const p of positions.values()) {
    if (p.y < 40)
      assert.ok(p.x >= 100 && p.x <= 160, `x ${p.x} inside its bin`);
  }
  assert.equal(lanes[0].height, 32);
  // A bin narrower than one column still holds its work, centred.
  assert.equal(positions.get(7).x, 302.5);
  assert.equal(lanes[1].top, 32);
  assert.equal(height, 52);
});

test("moveLane swaps neighbours and ignores moves off either end", async () => {
  const { moveLane } = await load();
  const order = ["A", "B", "C"];
  assert.deepEqual(moveLane(order, "B", -1), ["B", "A", "C"]);
  assert.deepEqual(moveLane(order, "B", 1), ["A", "C", "B"]);
  assert.deepEqual(moveLane(order, "A", -1), order);
  assert.deepEqual(moveLane(order, "C", 1), order);
  assert.deepEqual(order, ["A", "B", "C"], "input untouched");
});

test("restoreLaneOrder keeps a saved order for sections that still exist", async () => {
  const { restoreLaneOrder } = await load();
  const lanes = ["A", "B", "C", "D"];
  assert.deepEqual(restoreLaneOrder(["C", "A", "Gone"], lanes), [
    "C",
    "A",
    "B",
    "D",
  ]);
  assert.deepEqual(restoreLaneOrder(null, lanes), lanes);
  assert.deepEqual(restoreLaneOrder("junk", lanes), lanes);
});

test("externalLinks prefers identifiers and adds title searches", async () => {
  const { decodeGraph, externalLinks } = await load();
  const g = decodeGraph(fixture(), { nowYear: 2026 });
  const labels = externalLinks(g.nodes[1]).map((l) => l.label);
  assert.deepEqual(labels, [
    "DOI",
    "arXiv",
    "Semantic Scholar",
    "Google Scholar",
  ]);
  assert.equal(externalLinks(g.nodes[0])[0].href, "https://doi.org/10.1/a");
  assert.equal(
    externalLinks(g.nodes[2]).find((l) => l.label === "OpenAlex").href,
    "https://openalex.org/W3",
  );
});

test("searchNodes matches title, author and key, ours first", async () => {
  const { decodeGraph, searchNodes, describe } = await load();
  const g = decodeGraph(fixture(), { nowYear: 2026 });
  assert.deepEqual(searchNodes(g.nodes, "a"), []);
  assert.deepEqual(
    searchNodes(g.nodes, "gamma").map((n) => n.id),
    ["doi:10.1/c"],
  );
  assert.deepEqual(
    searchNodes(g.nodes, "alpha2020").map((n) => n.id),
    ["doi:10.1/a"],
  );
  assert.equal(describe(g.nodes[3]), "Unknown authors (n.d.). Delta");
});

test("the checked-in graph, bibliography and exclusions agree with the page", async () => {
  const {
    decodeGraph,
    buildAdjacency,
    parseBibSections,
    parseCuration,
    rankCandidates,
  } = await load();
  const raw = JSON.parse(
    fs.readFileSync(path.join(pageDir, "graph.json"), "utf8"),
  );
  const g = decodeGraph(raw);
  const bib = fs.readFileSync(
    path.join(repoRoot, "data/maths/references.bib"),
    "utf8",
  );
  const { sections } = parseBibSections(bib);
  const curation = JSON.parse(
    fs.readFileSync(path.join(pageDir, "curation.json"), "utf8"),
  );
  const { excluded, reasons } = parseCuration(curation);
  const ids = new Set(g.nodes.map((n) => n.id));
  for (const [id, item] of excluded) {
    assert.ok(ids.has(id), `excluded id is not in the graph: ${id}`);
    assert.ok(
      item.reason in reasons,
      `exclusion reason is not defined: ${item.reason}`,
    );
  }
  const members = g.nodes.filter((n) => n.inBibliography);
  assert.ok(members.length > 100);
  for (const m of members)
    assert.ok(
      sections.has(m.bibKey),
      `${m.bibKey} has no references.bib section`,
    );
  const ranked = rankCandidates(
    g,
    buildAdjacency(g.nodes.length, g.edges),
    sections,
    excluded,
  );
  assert.ok(ranked.length > 100);
});
