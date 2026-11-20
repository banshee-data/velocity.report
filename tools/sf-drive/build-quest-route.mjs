// Derive gameplay metadata without modifying the measured city tiles.
// Run from the repository root: node tools/sf-drive/build-quest-route.mjs
import fs from "node:fs";
import { gunzipSync } from "node:zlib";
import { createHash } from "node:crypto";
import { format } from "prettier";
import {
  buildCollisionIndex,
  collisionAt,
  groundAtWorld,
  intersectionsForTile,
} from "../../public_html/src/js/sf-drive-core.js";
const dir = new URL("../../public_html/src/sf/", import.meta.url);
const raw = fs.readFileSync(new URL("tiles/manifest.json", dir));
const manifest = JSON.parse(raw);
const emoji = Array.from(
  "🍎🍊🍋🍉🍇🍓🍒🍑🥭🍍🥝🍅🥥🥑🍆🥔🥕🌽🌶🥒🥬🥦🧄🧅🍄🥜🌰🍞🥐🥖🥨🥯🥞🧇🧀🍖🍗🥩🥓🍔🍟🍕🌭🥪🌮🌯🥙🧆🥚🍳🥘🍲🥣🥗🍿🧈🧂🥫🍱🍘🍙🍚🍛🍜🍝🍠🍢🍣🍤🍥🥮🍡🥟🥠🥡🍦🍧🍨🍩🍪🎂🍰🧁🥧🍫🍬🍭🍮🍯🍼🥛☕🍵🍶🍾🍷🍸🍹🍺🍻🥂🥃🥤🧃🧉🧊🥄🍴🥢🐶🐱🐭🐹🐰🦊🐻🐼🐨🐯🦁🐮🐷🐸🐵🐔🐧🐦🐤🦆🦅🦉🦇🐺🐗🐴🦄🐝🐛🦋🐌🐞🐜🦗🕷🦂🐢🐍🦎🦖🦕🐙🦑🦐🦞🦀🐡🐠🐟🐬🐳🐋🦈🐊🐅🐆🦓🦍🦧🐘🦛🦏🐪🐫🦒🦘🐃🐂🐄🐎🐖🐏🐑🦙🐐🦌🐕🐩🦮🐈🐓🦃🦚🦜🦢🦩🕊🐇🦝🦨🦡🦦🦥🐁🐀🐿🦔",
);
const targets = [],
  skipped = [];
const tiles = manifest.tiles.map((row) =>
  JSON.parse(
    gunzipSync(fs.readFileSync(new URL(`tiles/${row.t}.json.gz`, dir))),
  ),
);
const indexes = new Map();
for (const row of manifest.tiles) {
  const tile = tiles.find((t) => t.t === row.t);
  const neighbours = tiles.filter(
    (t) => Math.hypot(t.o[0] - row.x, t.o[1] - row.y) < 2200,
  );
  const collisionIndexes = neighbours.map((t) => {
    if (!indexes.has(t.t)) indexes.set(t.t, buildCollisionIndex(t));
    return indexes.get(t.t);
  });
  const candidates = intersectionsForTile(tile, neighbours).sort(
    (a, b) =>
      Math.hypot(a.x - row.x, a.y - row.y) -
      Math.hypot(b.x - row.x, b.y - row.y),
  );
  const target = candidates.find((p) => {
    const z = groundAtWorld(neighbours, p.x, p.y, true);
    return z !== null && !collisionAt(collisionIndexes, p, z);
  });
  if (target) {
    if (!emoji[targets.length]) throw new Error("Need more unique emoji");
    targets.push({ t: row.t, emoji: emoji[targets.length], ...target });
  } else
    skipped.push({
      t: row.t,
      reason: tile.s.length
        ? "No clear, grounded intersection"
        : "No signposted intersection",
    });
}
const route = {
  version: 1,
  manifestSha256: createHash("sha256").update(raw).digest("hex"),
  targets,
  skipped,
};
fs.writeFileSync(
  new URL("quest-route.json", dir),
  await format(JSON.stringify(route), { parser: "json" }),
);
console.log(
  `${targets.length} intersection targets; ${skipped.length} tiles without a playable intersection`,
);
