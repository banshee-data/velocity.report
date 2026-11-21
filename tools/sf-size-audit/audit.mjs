// Size accounting for the /sf/ city tiles: what the shipped bytes are made of,
// and what the same data costs under other serialisations and compressions.
//
// Reads public_html/src/sf/tiles/ (never writes it) and prints JSON with every
// figure measured from those files. Node's built-in gzip (level 9) and Brotli
// (quality 11) are the only codecs. Nothing is estimated here except where a
// key says so.
//
// Usage: node tools/sf-size-audit/audit.mjs [--out FILE]
import fs from "node:fs";
import path from "node:path";
import zlib from "node:zlib";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../..",
);
const TILES = path.join(ROOT, "public_html/src/sf/tiles");

const gz = (buf) => zlib.gzipSync(buf, { level: 9 }).length;
const br = (buf, lgwin = 22) =>
  zlib.brotliCompressSync(buf, {
    params: {
      [zlib.constants.BROTLI_PARAM_QUALITY]: 11,
      [zlib.constants.BROTLI_PARAM_LGWIN]: lgwin,
      [zlib.constants.BROTLI_PARAM_SIZE_HINT]: buf.length,
    },
  }).length;
const json = (v) => Buffer.from(JSON.stringify(v));

// ---- load -------------------------------------------------------------------
const manifestRaw = fs.readFileSync(path.join(TILES, "manifest.json"));
const manifest = JSON.parse(manifestRaw);
const tiles = manifest.tiles.map((m) => {
  const file = fs.readFileSync(path.join(TILES, `${m.t}.json.gz`));
  const raw = zlib.gunzipSync(file);
  return { m, file, raw, d: JSON.parse(raw) };
});
const LAYERS = ["g", "b", "tr", "r", "k", "s", "n"];
const LAYER_NAMES = {
  g: "ground grid",
  b: "buildings",
  tr: "trees",
  r: "roads (centrelines, class, one-way, widths)",
  k: "kerbs",
  s: "signposts",
  n: "street-name table",
};
const sum = (xs) => xs.reduce((a, b) => a + b, 0);

// ---- 1. whole payload, shipped JSON under each compression -----------------
const shipped = {
  tile_files_bytes: sum(tiles.map((t) => t.file.length)),
  manifest_tiles_bytes: sum(manifest.tiles.map((t) => t.bytes)),
  manifest_json_bytes: manifestRaw.length,
  tiles: tiles.length,
  raw_json_bytes: sum(tiles.map((t) => t.raw.length)),
};
const bundle = Buffer.concat(tiles.map((t) => t.raw));
const compression = {
  per_tile_gzip9: sum(tiles.map((t) => gz(t.raw))),
  per_tile_brotli11_lgwin22: sum(tiles.map((t) => br(t.raw))),
  per_tile_brotli11_lgwin24: sum(tiles.map((t) => br(t.raw, 24))),
  bundle_gzip9: gz(bundle),
  bundle_brotli11_lgwin24: br(bundle, 24),
};

// ---- 2. layers in the shipped JSON ----------------------------------------
// standalone: each layer serialised and gzipped on its own per tile;
// ablation: the full tile minus that layer, the difference from the full tile.
const fullGz = tiles.map((t) => gz(t.raw));
const layers = {};
for (const k of LAYERS) {
  const alone = tiles.map((t) => json(t.d[k]));
  const without = tiles.map((t) => {
    const c = { ...t.d };
    delete c[k];
    return gz(json(c));
  });
  layers[k] = {
    name: LAYER_NAMES[k],
    raw_json_bytes: sum(alone.map((b) => b.length)),
    standalone_per_tile_gzip9: sum(alone.map(gz)),
    ablation_per_tile_gzip9: sum(fullGz) - sum(without),
  };
}

// ---- 3. counts, precision and duplication ---------------------------------
const key = (x, y) => `${x},${y}`;
const counts = {
  buildings: 0,
  buildings_3dep: 0,
  faces: 0,
  ring_vertices: 0,
  unique_vertices_per_building_sum: 0,
  trees: 0,
  roads: 0,
  road_vertices: 0,
  roads_with_widths: 0,
  kerb_lines: 0,
  kerb_vertices: 0,
  signposts: 0,
  names_listed: 0,
  ground_nodes: 0,
  ground_missing: 0,
  ground_nodes_nearest_other_tile: 0,
};
const treeSeen = new Map();
const nameSet = new Set();
const origins = tiles.map((t) => t.d.o);
const near = new Map(
  tiles.map((t) => [
    t,
    origins.filter(
      ([px, py]) =>
        (px !== t.d.o[0] || py !== t.d.o[1]) &&
        Math.max(Math.abs(px - t.d.o[0]), Math.abs(py - t.d.o[1])) < 2000,
    ),
  ]),
);
const nearerElsewhere = (t, wx, wy) => {
  const own = Math.max(Math.abs(wx - t.d.o[0]), Math.abs(wy - t.d.o[1]));
  return near
    .get(t)
    .some(([px, py]) => Math.max(Math.abs(wx - px), Math.abs(wy - py)) < own);
};
for (const t of tiles) {
  const d = t.d;
  const [ox, oy] = d.o;
  for (const b of d.b) {
    counts.buildings += 1;
    if (b[2] === 1) counts.buildings_3dep += 1;
    const uniq = new Set();
    for (const face of b[1])
      for (const ring of face) {
        counts.faces += 1;
        counts.ring_vertices += ring.length / 3;
        for (let i = 0; i < ring.length; i += 3)
          uniq.add(`${ring[i]},${ring[i + 1]},${ring[i + 2]}`);
      }
    counts.unique_vertices_per_building_sum += uniq.size;
  }
  for (const tr of d.tr) {
    counts.trees += 1;
    const k = key(Math.round(ox * 10) + tr[0], Math.round(oy * 10) + tr[1]);
    treeSeen.set(k, (treeSeen.get(k) || 0) + 1);
  }
  for (const r of d.r) {
    counts.roads += 1;
    counts.road_vertices += r[3].length / 2;
    if (r[4].length) counts.roads_with_widths += 1;
  }
  for (const k of d.k) {
    counts.kerb_lines += 1;
    counts.kerb_vertices += k.length / 2;
  }
  counts.signposts += d.s.length;
  counts.names_listed += d.n.length;
  d.n.forEach((n) => nameSet.add(n));
  // ground: nodes, gaps, and nodes nearer another tile's origin (overlap)
  const g = d.g;
  for (let j = 0; j < g.n; j++)
    for (let i = 0; i < g.n; i++) {
      counts.ground_nodes += 1;
      if (g.z[j * g.n + i] === -32768) counts.ground_missing += 1;
      if (nearerElsewhere(t, ox + g.x0 + i * g.s, oy + g.y0 + j * g.s))
        counts.ground_nodes_nearest_other_tile += 1;
    }
}
counts.names_unique = nameSet.size;
counts.tree_positions_repeated = [...treeSeen.values()].filter(
  (v) => v > 1,
).length;
counts.ground_grid_step_m = tiles[0].d.g.s;
counts.ground_area_km2_gridded = (counts.ground_nodes * 64) / 1e6;

// ---- 4. the same data in a compact binary encoding ------------------------
// Zig-zag LEB128 varints of deltas, one column at a time (the scheme of the
// asset study's delta_stream), Brotli 11 with a 16 MiB window on one citywide
// stream per layer, as the 9.5 MB budget measured its layers.
class Col {
  constructor() {
    this.bytes = [];
  }
  u(v) {
    let x = v >>> 0;
    if (v > 0xffffffff) x = v; // not reached at these magnitudes
    while (x >= 0x80) {
      this.bytes.push((x & 0x7f) | 0x80);
      x >>>= 7;
    }
    this.bytes.push(x);
  }
  s(v) {
    this.u(v >= 0 ? v * 2 : -v * 2 - 1);
  }
  buf() {
    return Buffer.from(this.bytes);
  }
}
const cols = (names) => Object.fromEntries(names.map((n) => [n, new Col()]));
const pack = (c) => Buffer.concat(Object.values(c).map((x) => x.buf()));
const city = (t, x, y) => [
  Math.round(t.d.o[0] * 10) + x,
  Math.round(t.d.o[1] * 10) + y,
];

function encBuildings(welded) {
  const c = cols(["nf", "nr", "nv", "x", "y", "z", "zb", "idx", "flag"]);
  let px = 0,
    py = 0;
  for (const t of tiles)
    for (const b of t.d.b) {
      c.zb.s(b[0]);
      c.flag.u(b[2] === 1 ? 1 : 0);
      // faces, then each face's ring count: its outer ring and holes
      c.nf.u(b[1].length);
      for (const face of b[1]) c.nr.u(face.length);
      const rings = b[1].flat();
      if (!welded) {
        for (const ring of rings) {
          c.nv.u(ring.length / 3);
          let pz = b[0];
          for (let i = 0; i < ring.length; i += 3) {
            const [x, y] = city(t, ring[i], ring[i + 1]);
            c.x.s(x - px);
            c.y.s(y - py);
            c.z.s(ring[i + 2] - pz);
            px = x;
            py = y;
            pz = ring[i + 2];
          }
        }
        continue;
      }
      // welded: one vertex table per building, faces as indices into it
      const table = new Map();
      const verts = [];
      for (const ring of rings)
        for (let i = 0; i < ring.length; i += 3) {
          const k = `${ring[i]},${ring[i + 1]},${ring[i + 2]}`;
          if (!table.has(k)) {
            table.set(k, verts.length);
            verts.push([ring[i], ring[i + 1], ring[i + 2]]);
          }
        }
      c.nv.u(verts.length);
      let pz = b[0];
      for (const [vx, vy, vz] of verts) {
        const [x, y] = city(t, vx, vy);
        c.x.s(x - px);
        c.y.s(y - py);
        c.z.s(vz - pz);
        px = x;
        py = y;
        pz = vz;
      }
      for (const ring of rings) {
        c.nv.u(ring.length / 3);
        let pi = 0;
        for (let i = 0; i < ring.length; i += 3) {
          const ix = table.get(`${ring[i]},${ring[i + 1]},${ring[i + 2]}`);
          c.idx.s(ix - pi);
          pi = ix;
        }
      }
    }
  return pack(c);
}

function encTrees(posStepDm) {
  const all = [];
  for (const t of tiles)
    for (const tr of t.d.tr) {
      const [x, y] = city(t, tr[0], tr[1]);
      all.push([
        Math.round(x / posStepDm),
        Math.round(y / posStepDm),
        tr[2],
        tr[3],
      ]);
    }
  // the study's order: 50 m rows, then x
  const row = 500 / posStepDm;
  all.sort(
    (a, b) => Math.round(a[0] / row) - Math.round(b[0] / row) || a[1] - b[1],
  );
  const c = cols(["x", "y", "h", "r"]);
  let px = 0,
    py = 0;
  for (const [x, y, h, r] of all) {
    c.x.s(x - px);
    c.y.s(y - py);
    c.h.u(h);
    c.r.u(r);
    px = x;
    py = y;
  }
  return pack(c);
}

const nameIndex = new Map([...nameSet].sort().map((n, i) => [n, i]));
function encRoads(withWidths) {
  const c = cols(["name", "cls", "dir", "nv", "x", "y", "wl", "wr"]);
  let px = 0,
    py = 0;
  for (const t of tiles)
    for (const r of t.d.r) {
      c.name.u(nameIndex.get(t.d.n[r[0]]) ?? 0);
      c.cls.u(r[1]);
      c.dir.u(r[2]);
      const p = r[3];
      c.nv.u(p.length / 2);
      for (let i = 0; i < p.length; i += 2) {
        const [x, y] = city(t, p[i], p[i + 1]);
        c.x.s(x - px);
        c.y.s(y - py);
        px = x;
        py = y;
      }
      if (!withWidths) continue;
      const w = r[4];
      let pl = 0,
        pr = 0;
      for (let i = 0; i < w.length; i += 2) {
        c.wl.s(w[i] - pl);
        c.wr.s(w[i + 1] - pr);
        pl = w[i];
        pr = w[i + 1];
      }
    }
  return pack(c);
}
function encKerbs() {
  const c = cols(["nv", "x", "y"]);
  let px = 0,
    py = 0;
  for (const t of tiles)
    for (const k of t.d.k) {
      c.nv.u(k.length / 2);
      for (let i = 0; i < k.length; i += 2) {
        const [x, y] = city(t, k[i], k[i + 1]);
        c.x.s(x - px);
        c.y.s(y - py);
        px = x;
        py = y;
      }
    }
  return pack(c);
}
function encSigns() {
  const c = cols(["n", "x", "y", "name", "angle"]);
  let px = 0,
    py = 0;
  for (const t of tiles)
    for (const s of t.d.s) {
      const [x, y] = city(t, s[0], s[1]);
      c.x.s(x - px);
      c.y.s(y - py);
      px = x;
      py = y;
      c.n.u((s.length - 2) / 2);
      for (let i = 2; i + 1 < s.length; i += 2) {
        c.name.u(nameIndex.get(t.d.n[s[i]]) ?? 0);
        c.angle.u(s[i + 1]);
      }
    }
  return pack(c);
}
const encNames = () => Buffer.from([...nameIndex.keys()].join("\n"));
// ground as shipped (every tile's whole grid) or only the nodes nearest the
// tile's own origin (each place once); valid heights delta-coded in row
// order, gaps kept as alternating run lengths (valid, missing, valid, ...)
function encGround(ownOnly) {
  const c = cols(["hdr", "gap", "z"]);
  for (const t of tiles) {
    const g = t.d.g;
    const [ox, oy] = t.d.o;
    c.hdr.s(Math.round(g.x0));
    c.hdr.s(Math.round(g.y0));
    c.hdr.u(g.n);
    let prev = 0,
      missing = false,
      run = 0;
    for (let j = 0; j < g.n; j++)
      for (let i = 0; i < g.n; i++) {
        if (
          ownOnly &&
          nearerElsewhere(t, ox + g.x0 + i * g.s, oy + g.y0 + j * g.s)
        )
          continue;
        const z = g.z[j * g.n + i];
        if ((z === -32768) !== missing) {
          c.gap.u(run);
          missing = !missing;
          run = 0;
        }
        run += 1;
        if (!missing) {
          c.z.s(z - prev);
          prev = z;
        }
      }
    c.gap.u(run);
  }
  return pack(c);
}

const streams = {
  buildings_unwelded: encBuildings(false),
  buildings_welded_vertex_table: encBuildings(true),
  trees_shipped_precision: encTrees(1),
  trees_budget_precision_0p5m: encTrees(5),
  roads_centrelines_names_class: encRoads(false),
  roads_with_widths: encRoads(true),
  kerbs: encKerbs(),
  signposts: encSigns(),
  street_names: encNames(),
  ground_as_shipped: encGround(false),
  ground_each_place_once: encGround(true),
};
const compact = {};
for (const [k, b] of Object.entries(streams))
  compact[k] = {
    raw_bytes: b.length,
    brotli11_lgwin24: br(b, 24),
    gzip9: gz(b),
  };
// whole payloads in the compact encoding (one Brotli stream over the layers listed)
const whole = (ks) => br(Buffer.concat(ks.map((k) => streams[k])), 24);
compact.whole_budget_layers_one_stream = whole([
  "buildings_welded_vertex_table",
  "trees_budget_precision_0p5m",
  "roads_centrelines_names_class",
  "kerbs",
  "street_names",
]);
compact.whole_all_layers_shipped_precision_one_stream = whole([
  "buildings_welded_vertex_table",
  "trees_shipped_precision",
  "roads_with_widths",
  "kerbs",
  "signposts",
  "street_names",
  "ground_as_shipped",
]);
compact.whole_all_layers_ground_once_one_stream = whole([
  "buildings_welded_vertex_table",
  "trees_shipped_precision",
  "roads_with_widths",
  "kerbs",
  "signposts",
  "street_names",
  "ground_each_place_once",
]);

const out = {
  generated_by: "tools/sf-size-audit/audit.mjs",
  node: process.version,
  codecs: "gzip level 9; Brotli quality 11 (lgwin 22 unless named)",
  tiles_manifest_built: manifest.provenance?.built,
  shipped,
  compression,
  layers,
  counts,
  compact,
};
const i = process.argv.indexOf("--out");
const text = JSON.stringify(out, null, 1) + "\n";
if (i > 0) fs.writeFileSync(process.argv[i + 1], text);
process.stdout.write(text);
