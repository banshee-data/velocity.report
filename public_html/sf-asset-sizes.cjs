const fs = require("node:fs");
const path = require("node:path");
const { gzipSync } = require("node:zlib");

// Fallback response-body sizes, rather than uncompressed source-file sizes.
// Tile payloads already carry their exact gzip byte counts in the manifest.
function gzipBytes(bytes) {
  return bytes[0] === 0x1f && bytes[1] === 0x8b
    ? bytes.length
    : gzipSync(bytes, { level: 9 }).length;
}

function sfDownloadAssets(root = __dirname) {
  const files = [
    ["tiles", "manifest.json", "src/sf/tiles/manifest.json"],
    ["tiles", "../quest-route.json", "src/sf/quest-route.json"],
    ...[
      "sf-drive.js",
      "sf-drive-core.js",
      "sf-drive-surface.js",
      "sf-download-size.js",
    ].map((name) => ["modules", `./${name}`, `src/js/${name}`]),
    ...["three.module.js", "three.core.js"].map((name) => [
      "modules",
      `../vendor/three/${name}`,
      `node_modules/three/build/${name}`,
    ]),
  ];
  return files.map(([base, url, file]) => ({
    base,
    url,
    bytes: gzipBytes(fs.readFileSync(path.join(root, file))),
  }));
}

module.exports = { gzipBytes, sfDownloadAssets };
