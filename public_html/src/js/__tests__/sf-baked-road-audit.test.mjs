import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

test("the baked road audit distinguishes a clear lane from a closed cross-section", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "sf-contact-audit-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const token = "aabbccdd";
  const profile = (id, kind, y, z, width, groundEnds = []) => ({
    id,
    kind,
    width,
    groundEnds,
    owners: [token, token],
    points: [0, 10, 20].map((x) => [x, y, z]),
  });
  const tile = {
    t: token,
    o: [0, 0],
    g: { x0: -40, y0: -40, s: 8, n: 11, z: Array(121).fill(0) },
    tr: [[100, 0, 9, 2]],
    b: [[0, [[[90, 110, 45, 110, 110, 45, 110, 210, 45, 90, 210, 45]]]]],
    playableRoads: [
      profile("wide", "tunnel", 0, 0, 8),
      profile("obstructed", "ramp", 16, 0, 6),
      profile("portal", "ramp", 30, 1, 6, ["start"]),
    ],
  };
  const compressed = gzipSync(JSON.stringify(tile));
  writeFileSync(join(directory, `${token}.json.gz`), compressed);
  writeFileSync(
    join(directory, "manifest.json"),
    JSON.stringify({ tiles: [{ t: token, bytes: compressed.length }] }),
  );
  const command = fileURLToPath(
    new URL(
      "../../../../tools/sf-drive/audit-baked-road-contact.mjs",
      import.meta.url,
    ),
  );
  const report = JSON.parse(
    execFileSync(process.execPath, [command, directory]),
  );
  assert.equal(report.surfaces, 3);
  assert.equal(report.surfacesWithCentreHits, 2);
  assert.equal(report.surfacesWithClosedCrossSections, 1);
  assert.equal(report.surfacesWithoutConnectedLane, 1);
  const wide = report.flagged.find(({ id }) => id === "wide");
  const blocked = report.flagged.find(({ id }) => id === "obstructed");
  assert.deepEqual(wide.centreObstacles[0].tree, [token, 0]);
  assert.equal(wide.disconnectedStation, undefined);
  assert.equal(blocked.closedStation, blocked.disconnectedStation);
  assert.deepEqual(report.unmatchedPortals, [
    { id: "portal", position: [0, 30, 1], ground: 0 },
  ]);
});
