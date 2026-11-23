// Drive baked corridors using the game's keyboard path.
const assert = require("node:assert/strict");
const { readFile } = require("node:fs/promises");
const { join } = require("node:path");
const { gunzipSync } = require("node:zlib");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

const origin = process.env.SF_BASE_URL || "http://127.0.0.1:8090";
const candidate = process.env.SF_CANDIDATE_DIR;
const owners = ["808586d4", "808586dc", "808586ec"];
const i280Owners = ["808f7fd4", "808f7fcc"];
const tilePath = (token) => `tiles/${token}.json.gz`;
const prefix = "sf-contact/";
const cases = {
  northbound: {
    roads: [
      "presidio-nb-entry",
      "main-post-northbound",
      "presidio-nb-between",
      "battery-northbound",
      "presidio-nb-viaduct",
      "presidio-nb-exit",
    ],
    speed: 22,
  },
  southbound: {
    roads: [
      "presidio-sb-entry",
      "presidio-sb-viaduct",
      "battery-southbound",
      "presidio-sb-between",
      "main-post-sb-approach",
      "main-post-southbound",
      "presidio-sb-exit",
    ],
    speed: 22,
  },
  "toll-eastbound": { roads: ["toll-plaza-tunnel"], speed: 8 },
  "toll-westbound": { roads: ["toll-plaza-tunnel"], speed: 8, reverse: true },
};
const defaultCases = Object.keys(cases);
const i280Approach = ["i280-fifth-south-entry", "i280-king-to-mariposa"];
const dogpatchOwners = [
  ...i280Owners,
  "808f7fb4",
  "808f7fac",
  "808f7f04",
  "808f7efc",
];
const dogpatchApproach = [
  ...i280Approach,
  "i280-south-through-18th",
  "i280-dogpatch-approach",
];
const dogpatchClearance = {
  clearance: { offset: 2.2, station: 1954, length: 30 },
};
for (const [name, exit] of [
  ["i280-pennsylvania", "i280-pennsylvania-exit"],
  ["i280-18th", "i280-18th-exit"],
]) {
  cases[name] = {
    roads: [...i280Approach, "i280-18th-off-ramp", exit],
    owners: i280Owners,
    speed: 12,
    rampApproach: true,
  };
}
cases["i280-mainline"] = {
  roads: [
    ...dogpatchApproach,
    "i280-south-through-dogpatch",
    "i280-south-beyond-dogpatch",
  ],
  owners: dogpatchOwners,
  speed: 22,
  speedLimits: { "i280-south-through-dogpatch": 16 },
  laneGuides: {
    "i280-dogpatch-approach": { approach: 1.25, holdApproach: true },
    "i280-south-through-dogpatch": { entranceEast: 1.25, entranceMetres: 20 },
    "i280-south-beyond-dogpatch": dogpatchClearance,
  },
};
for (const direction of ["north", "south"]) {
  const north = direction === "north";
  const exit = `i280-dogpatch-pennsylvania-${direction}`;
  cases[`i280-dogpatch-exit-${direction}`] = {
    roads: [...dogpatchApproach, "i280-dogpatch-off-ramp", exit],
    owners: dogpatchOwners,
    speed: 16,
    speedLimits: { "i280-dogpatch-off-ramp": 12, [exit]: 8 },
    laneGuides: {
      "i280-dogpatch-off-ramp": { approach: -1.25, holdApproach: north },
      [exit]: north
        ? {
            entranceEast: -1.25,
            clearance: { offset: -2, station: 26, length: 13 },
          }
        : null,
    },
  };
  const entry = `i280-dogpatch-pennsylvania-on-${direction}`;
  cases[`i280-dogpatch-enter-${direction}`] = {
    roads: [entry, "i280-dogpatch-on-ramp", "i280-south-beyond-dogpatch"],
    owners: dogpatchOwners,
    speed: 18,
    speedLimits: { [entry]: 9, "i280-dogpatch-on-ramp": 13 },
    laneGuides: { "i280-south-beyond-dogpatch": dogpatchClearance },
  };
}
for (const reverse of [false, true]) {
  cases[reverse ? "alemany-eastbound" : "alemany-westbound"] = {
    roads: ["alemany-i280-overpass"],
    owners: ["808f7e64", "808f7ef4"],
    speed: 10,
    reverse,
  };
  const geary = "geary-masonic-tunnel";
  cases[reverse ? "geary-eastbound" : "geary-westbound"] = {
    roads: [geary],
    owners: ["808580b4", "80858734"],
    speed: 14,
    reverse,
    // The open eastbound approach has planted islands beside the inner lane.
    laneGuides: { [geary]: { lane: reverse ? 2.5 : -4.5 } },
  };
  const macarthur = "macarthur-tunnel";
  cases[reverse ? "macarthur-southbound" : "macarthur-northbound"] = {
    roads: [macarthur],
    owners: ["80858724", "808586e4", "8085871c"],
    speed: 16,
    reverse,
    laneGuides: { [macarthur]: { lane: reverse ? 4.2 : -4.2 } },
  };
}
cases["branch-east-ramp"] = {
  roads: ["branch-east-off-ramp"],
  owners: ["808f7e7c", "808f7e64"],
  speed: 15,
};
cases["branch-east-upper"] = {
  roads: ["branch-east-i280-northbound"],
  owners: ["808f7e7c", "808f7e64"],
  speed: 18,
};
cases["branch-east-san-jose"] = {
  roads: ["branch-east-san-jose-viaduct"],
  owners: ["808f7e64"],
  speed: 15,
};

async function asset(pathname) {
  if (candidate) return readFile(join(candidate, pathname));
  const response = await fetch(`${origin}/sf/${pathname}`);
  assert.ok(response.ok, `${pathname}: ${response.status}`);
  return Buffer.from(await response.arrayBuffer());
}

function driveLine({ id, points }, rampApproach) {
  // Steer around the median tree when approaching northbound, and follow the
  // clear left lane through the roof constriction in the southbound Main Post bore.
  const tree = id === `${prefix}presidio-nb-entry`;
  const ramp = rampApproach && id === `${prefix}i280-king-to-mariposa`;
  if (!ramp && !tree && id !== `${prefix}main-post-southbound`) return points;
  const smooth = (v) => {
    const n = Math.max(0, Math.min(1, v));
    return n * n * (3 - 2 * n);
  };
  const distances = [0];
  for (let i = 1; i < points.length; i++) {
    distances.push(
      distances.at(-1) +
        Math.hypot(
          points[i][0] - points[i - 1][0],
          points[i][1] - points[i - 1][1],
        ),
    );
  }
  return points.map(([x, y, z], i) => {
    const a = points[Math.max(0, i - 1)];
    const b = points[Math.min(points.length - 1, i + 1)];
    const distance = distances[i];
    const remaining = distances.at(-1) - distance;
    let offset =
      2.5 * smooth((distance - 175) / 29) * smooth((273 - distance) / 29);
    if (tree) offset = 1.75 * Math.max(0, 1 - ((distance - 81) / 25) ** 2) ** 2;
    if (ramp)
      offset =
        1.45 * (1 - Math.exp(-remaining / 5)) * smooth((32 - remaining) / 18);
    const dx = b[0] - a[0];
    const dy = b[1] - a[1];
    const length = Math.hypot(dx, dy);
    return [x - (dy / length) * offset, y + (dx / length) * offset, z];
  });
}

async function drive(page, sections, speed) {
  return page.evaluate(
    async ({ sections, speed }) => {
      const driveModule = performance
        .getEntriesByType("resource")
        .find(({ name }) => name.endsWith("/sf-drive.js"));
      const { buildCityGameDrive, cityDriveControls } = await import(
        new URL("sf-city-road-drive.js", driveModule.name).href
      );
      const canvas = document.querySelector("#drive");
      const ids = sections.map(({ id }) => id);
      const lastSection = sections.at(-1);
      const [before, end] = lastSection.positions.slice(-2);
      const heading = Math.atan2(end[1] - before[1], end[0] - before[0]);
      const exterior = {
        ...lastSection,
        id: "exterior-guide",
        kind: "ground",
        positions: [0, 3, 6].map((distance) => [
          end[0] + distance * Math.cos(heading),
          end[1] + distance * Math.sin(heading),
          end[2],
        ]),
      };
      const guide = sections.length < 2 ? [...sections, exterior] : sections;
      const course = buildCityGameDrive({ sections: guide }, () => null);
      course.speed.fill(5, Math.max(0, course.speed.length - 8));

      const held = new Set();
      function key(code, pressed) {
        if (held.has(code) === pressed) return;
        if (pressed) held.add(code);
        else held.delete(code);
        canvas.dispatchEvent(
          new KeyboardEvent(pressed ? "keydown" : "keyup", {
            code,
            bubbles: true,
            cancelable: true,
          }),
        );
      }

      const seen = new Map();
      const enclosed = new Set();
      const started = performance.now();
      let pose;
      let last = 0;
      let pulse = 0;
      let blocked = 0;
      let maxSpeed = 0;
      let lastSupported;
      return new Promise((resolve) => {
        function finish(status) {
          for (const code of held) key(code, false);
          resolve({
            status,
            seen: [...seen.values()],
            enclosed: [...enclosed],
            exit: pose,
            lastSupported,
            maxMph: Math.round(maxSpeed * 2.23694 * 10) / 10,
            seconds: Math.round((performance.now() - started) / 100) / 10,
          });
        }

        function tick(now) {
          const d = canvas.dataset;
          if (d.replay === "crash") return finish("crashed");
          const surfaceId = d.surfaceId || "";
          pose = {
            x: Number(d.east),
            y: Number(d.north),
            z: Number(d.altitude),
            v: Number(d.speedMps),
            heading: Number(d.heading),
            surfaceId,
            camera: d.cameraContact,
            grounded: d.contact === "grounded",
          };
          maxSpeed = Math.max(maxSpeed, pose.v);
          if (surfaceId && !ids.includes(surfaceId))
            return finish("wrong road");
          if (surfaceId && !seen.has(surfaceId)) seen.set(surfaceId, pose);
          if (surfaceId && pose.grounded) lastSupported = pose;
          if (surfaceId && pose.camera === "tunnel") enclosed.add(surfaceId);
          if (!seen.size && now - started > 20_000)
            return finish("missed entrance");
          if (!surfaceId && seen.has(ids.at(-1)) && pose.grounded)
            return finish("left road");
          if (!surfaceId && seen.size && pose.grounded)
            return finish("lost intermediate contact");
          blocked = d.roadBlocked === "true" ? blocked + 1 : 0;
          if (blocked >= 10) return finish("blocked");
          if (now - started > 460_000) return finish("timed out");

          if (now - last >= 27) {
            const input = cityDriveControls(course, {
              ...pose,
              surfaceId: surfaceId || ids[0],
              // The steering guide keeps sending keys during short bounces.
              // Ground contact itself still comes from the browser's physics.
              grounded: true,
            });
            if (!input) return finish("lost course");
            const turn = Math.max(-1, Math.min(1, input.steer));
            pulse += Math.min(1, Math.abs(turn) * 1.15);
            const steer = pulse >= 1;
            if (steer) pulse--;
            const approachLimit = speed > 8 ? 16 : 6;
            key("ArrowLeft", steer && turn > 0.025);
            key("ArrowRight", steer && turn < -0.025);
            key(
              "ArrowUp",
              input.throttle > 0.15 &&
                (seen.size > 0 || pose.v < approachLimit),
            );
            key(
              "Space",
              input.brake > 0.52 || (!seen.size && pose.v > approachLimit + 1),
            );
            last = now;
          }
          requestAnimationFrame(tick);
        }
        requestAnimationFrame(tick);
      });
    },
    { sections, speed },
  );
}

async function main() {
  const { steeringLine } = await import("./steering-line.mjs");
  const names = process.argv.length > 2 ? process.argv.slice(2) : defaultCases;
  for (const name of names)
    assert.ok(Object.hasOwn(cases, name), `Unknown case ${name}`);
  const tokens = [
    ...new Set(names.flatMap((name) => cases[name].owners ?? owners)),
  ];
  const patchedPaths = [
    "tiles/manifest.json",
    "quest-route.json",
    ...tokens.map(tilePath),
  ];
  const snapshots = new Map(
    await Promise.all(
      patchedPaths.map(async (path) => [path, await asset(path)]),
    ),
  );
  const manifest = JSON.parse(snapshots.get("tiles/manifest.json"));
  const quest = JSON.parse(snapshots.get("quest-route.json"));
  const roads = new Map();
  for (const token of tokens) {
    const tile = JSON.parse(gunzipSync(snapshots.get(tilePath(token))));
    for (const road of tile.playableRoads ?? []) roads.set(road.id, road);
  }

  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const name of names) {
      const {
        roads: slugs,
        speed,
        reverse,
        rampApproach,
        laneGuides = {},
        speedLimits = {},
      } = cases[name];
      const sections = slugs.map((slug) => {
        const road = roads.get(prefix + slug);
        assert.ok(road, slug);
        const guide = laneGuides[slug];
        const positions = guide
          ? steeringLine(road.points, guide)
          : driveLine(road, rampApproach);
        return {
          id: road.id,
          kind: road.kind,
          width_m: road.width,
          positions: reverse ? positions.toReversed() : positions,
          speed_mps: speedLimits[slug] ?? speed,
          game_clearance_m: road.game_clearance_m,
          sources: [],
        };
      });
      for (let i = 1; i < sections.length; i++) {
        const previous = sections[i - 1].positions.at(-1);
        const [x, y, z] = sections[i].positions[0];
        assert.ok(
          Math.hypot(previous[0] - x, previous[1] - y) < 0.05 &&
            Math.abs(previous[2] - z) < 0.002,
          `Open steering guide from ${sections[i - 1].id} to ${sections[i].id}`,
        );
        sections[i].positions = [previous, ...sections[i].positions.slice(1)];
      }
      const [start, next] = sections[0].positions;
      const heading = Math.atan2(next[1] - start[1], next[0] - start[0]);
      const approach = speed > 8 ? 11 : 7;
      const checkpoint = {
        x: start[0] - approach * Math.cos(heading),
        y: start[1] - approach * Math.sin(heading),
        heading,
      };
      const page = await browser.newPage({
        viewport: { width: 420, height: 320 },
        serviceWorkers: "block",
      });
      const errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        if (candidate)
          await page.route("**/sf/**", (route) => {
            const path = new URL(route.request().url()).pathname.slice(4);
            if (!snapshots.has(path)) return route.continue();
            return route.fulfill({
              body: snapshots.get(path),
              contentType: path.endsWith(".gz")
                ? "application/gzip"
                : "application/json",
            });
          });
        await page.addInitScript(
          ({ key, checkpoint }) => {
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            );
          },
          {
            key: `sf-progress-v1:sf-trial-v2:${manifest.terrainRebake.progressBaseSha256}:${quest.placementSha256}:downtown`,
            checkpoint,
          },
        );
        await page.goto(`${origin}/sf/?at=downtown`);
        const canvas = page.locator('#drive[data-contact="grounded"]');
        await canvas.waitFor({ timeout: 60_000 });
        const initial = await canvas.evaluate((el) => ({ ...el.dataset }));
        assert.ok(
          Math.hypot(
            +initial.east - checkpoint.x,
            +initial.north - checkpoint.y,
          ) < 1,
        );
        const contactHash = await page.evaluate(async () => {
          const response = await fetch("/sf/tiles/manifest.json");
          return (await response.json()).roadBake.playableSha256;
        });
        assert.equal(contactHash, manifest.roadBake.playableSha256);
        await canvas.focus();
        await confirmManualTakeover(page);
        const result = await drive(page, sections, speed);
        console.log(JSON.stringify({ name, contactHash, ...result, errors }));
        assert.deepEqual(errors, []);
        assert.deepEqual(
          result.seen.map(({ surfaceId }) => surfaceId),
          sections.map(({ id }) => id),
        );
        assert.ok(result.seen.every(({ grounded }) => grounded));
        assert.deepEqual(
          result.enclosed,
          sections.filter(({ kind }) => kind === "tunnel").map(({ id }) => id),
        );
        assert.equal(result.status, "left road");
        const last = sections.at(-1).positions.at(-1);
        assert.ok(
          Math.hypot(result.exit.x - last[0], result.exit.y - last[1]) < 6,
        );
        assert.equal(result.exit.camera, "exterior");
        assert.ok(result.exit.grounded);
        if (process.env.SF_SCREENSHOT_DIR)
          await page.screenshot({
            path: join(
              process.env.SF_SCREENSHOT_DIR,
              `sf-presidio-${name}.png`,
            ),
          });
      } finally {
        await page.close();
      }
    }
  } finally {
    await browser.close();
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
