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

test("yearScale squeezes years before the pivot and stays monotonic", async () => {
  const { yearScale } = await load();
  const { scale, ticks } = yearScale(1818, 2026, 0, 1000);
  assert.equal(scale(1818), 0);
  assert.ok(Math.abs(scale(1960) - 120) < 1e-9);
  assert.equal(scale(2026), 1000);
  for (let y = 1818; y < 2026; y++) assert.ok(scale(y) < scale(y + 1));
  assert.equal(ticks[0], 1818);
  assert.ok(ticks.includes(1960) && ticks.includes(2020));
  const recent = yearScale(1990, 2026, 0, 1000);
  assert.equal(
    recent.scale(1960),
    0,
    "no squeeze when nothing predates the pivot",
  );
});

test("layoutLanes stacks shared columns and skips empty lanes", async () => {
  const { layoutLanes } = await load();
  const items = [
    { index: 0, lane: "Tracking", x: 100 },
    { index: 1, lane: "Tracking", x: 101 },
    { index: 2, lane: "Safety", x: 300 },
  ];
  const { positions, lanes, height } = layoutLanes(
    items,
    ["Tracking", "Empty", "Safety"],
    {
      columnWidth: 12,
      rowHeight: 12,
      pad: 10,
    },
  );
  assert.deepEqual(
    lanes.map((l) => l.lane),
    ["Tracking", "Safety"],
  );
  assert.equal(positions.get(0).x, positions.get(1).x);
  assert.equal(positions.get(1).y - positions.get(0).y, 12);
  assert.equal(lanes[0].height, 32);
  assert.equal(lanes[1].top, 32);
  assert.equal(height, 52);
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
