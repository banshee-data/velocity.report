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
    ["tiles", "../coverage.json", "src/sf/coverage.json"],
    ["tiles", "../gamedrives.json", "src/sf/gamedrives.json"],
    ...[
      "sf-drive.js",
      "sf-drive-assets.js",
      "sf-city-road-drive.js",
      "sf-soma-drive.js",
      "sf-insurance.js",
      "sf-survey-beacons.js",
      "sf-road-contact.js",
      "sf-road-network.js",
      "sf-highway-edge.js",
      "sf-building-walls.js",
      "sf-airtime.js",
      "sf-progress.js",
      "sf-replay.js",
      "sf-solo.js",
      "sf-survey-replay.js",
      "scene-colours.js",
      "scene-coords.js",
      "scene-fade.js",
      "scene-following.js",
      "scene-playback.js",
      "scene-reader.js",
      "scene-track-box.js",
      "survey-charts.js",
      "survey-model.js",
      "sf-drive-core.js",
      "sf-chassis-contact.js",
      "sf-crash.js",
      "sf-drive-surface.js",
      "sf-download-size.js",
      "sf-touch-controls.js",
      "sf-district-trials.js",
      "sf-drive-camera.js",
      "sf-car-reveal.js",
      "sf-road-markings.js",
      "sf-road-markings-worker.js",
      "sf-terrain-painter.js",
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
