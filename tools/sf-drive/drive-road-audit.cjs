// Drive exported road audit courses with the public game's keyboard inputs.
const assert = require("node:assert/strict");
const { createHash } = require("node:crypto");
const { readFileSync } = require("node:fs");
const { chromium } = require("playwright");
const { confirmManualTakeover } = require("./manual-takeover.cjs");

async function drive(page, course, key) {
  return page.evaluate(
    async ({ course, key }) => {
      const canvas = document.querySelector("#drive");
      const { waypoints, from, to, streetSegment } = course;
      const held = new Set();
      const surfaces = new Set();
      const enclosed = new Set();
      const grounded = [];
      const began = performance.now();
      const [finishX, finishY, lastStation] = waypoints.at(-1);
      let at = 0;
      let inputTime = 0;
      let motionTime = began;
      let pulse = 0;
      let car;
      let distance = 0;
      let airborneMetres = 0;
      let loading = 0;
      let groundExit;
      let before;
      let maxSpeed = 0;
      let collected = [];
      let status;
      const round = (value) => Math.round(value * 1000) / 1000;
      const press = (code, down) => {
        if (held.has(code) === down) return;
        if (down) held.add(code);
        else held.delete(code);
        canvas.dispatchEvent(
          new KeyboardEvent(down ? "keydown" : "keyup", {
            code,
            bubbles: true,
            cancelable: true,
          }),
        );
      };
      while (!status) {
        const now = await new Promise(requestAnimationFrame);
        const d = canvas.dataset;
        car = {
          x: +d.east,
          y: +d.north,
          z: +d.altitude,
          v: +d.speedMps,
          contact: d.contact,
          road: d.surfaceId || null,
        };
        if (car.road) surfaces.add(car.road);
        if (d.cameraContact === "tunnel" && car.road) enclosed.add(car.road);
        maxSpeed = Math.max(maxSpeed, car.v);
        if (before) {
          const step = Math.hypot(car.x - before.x, car.y - before.y);
          const onRoad =
            car.contact === "grounded" && before.contact === "grounded";
          if (onRoad && before.road && !car.road) groundExit = distance;
          while (step > 0 && grounded.length <= distance + step) {
            const fraction = (grounded.length - distance) / step;
            grounded.push({
              x: before.x + fraction * (car.x - before.x),
              y: before.y + fraction * (car.y - before.y),
              z: onRoad ? before.z + fraction * (car.z - before.z) : null,
            });
          }
          distance += step;
          if (step > 0.005) motionTime = now;
          if (car.contact === "airborne") airborneMetres += step;
          if (car.contact === "loading" || d.roadBlocked === "true")
            loading += now - before.now;
        }
        before = { ...car, now };
        const progress = JSON.parse(localStorage.getItem(key) || "{}");
        collected = (progress.collected ?? []).filter(
          (tile) => tile === from || tile === to,
        );
        if (streetSegment && car.road) status = "selected baked contact";
        else if (
          !streetSegment &&
          collected.includes(from) &&
          collected.includes(to)
        )
          status = "both emoji collected";
        else if (d.replay === "crash" || progress.crashes > 0) status = "crash";
        else if (now - motionTime > 12000) status = "stalled";
        else if (now - began > (lastStation / 1.5 + 75) * 1000)
          status = "timed out";
        if (status || now - inputTime < 27) continue;
        inputTime = now;
        let best = Infinity;
        let nearest = at;
        for (
          let i = Math.max(0, at - 3);
          i < Math.min(waypoints.length, at + 19);
          i++
        ) {
          const [x, y] = waypoints[i];
          const gap = Math.hypot(car.x - x, car.y - y);
          if (gap < best) {
            best = gap;
            nearest = i;
          }
        }
        at = nearest;
        if (best > 16) {
          status = "left course";
          continue;
        }
        if (
          streetSegment &&
          car.contact === "grounded" &&
          Math.hypot(car.x - finishX, car.y - finishY) < 2.5
        ) {
          status = "street segment completed";
          continue;
        }
        let aim = at;
        const lookAhead = Math.max(3.3, Math.min(11, 3.3 + car.v * 0.37));
        while (
          aim + 1 < waypoints.length &&
          waypoints[aim][2] - waypoints[at][2] < lookAhead
        )
          aim++;
        const [x, y] = waypoints[aim];
        const remaining = Math.hypot(x - car.x, y - car.y);
        const rotation = Math.atan2(y - car.y, x - car.x) - +d.heading;
        const demand = Math.atan(
          (5.4 * Math.sin(rotation)) / Math.max(1, remaining),
        );
        const steer = demand / (0.58 / (1 + car.v / 20));
        pulse += Math.min(1, Math.abs(steer) * 1.15);
        const turn = pulse >= 1;
        if (turn) pulse--;
        const correction = waypoints[at][3] - car.v;
        press("ArrowUp", correction > -0.15);
        press("Space", correction < -0.9);
        press("ArrowLeft", turn && steer > 0.025);
        press("ArrowRight", turn && steer < -0.025);
      }
      for (const code of held) press(code, false);
      const grades = [];
      let deepestDip = 0;
      let steepest = 0;
      let dipAt = null;
      let steepestAt = null;
      let dips = 0;
      for (let i = 1; i < grounded.length; i++) {
        const { x, y, z } = grounded[i];
        const prior = grounded[i - 1].z;
        if (z === null || prior === null) continue;
        const grade = Math.abs(z - prior) * 100;
        grades.push(grade);
        if (grade > steepest) {
          steepest = grade;
          steepestAt = [round(x), round(y)];
        }
        if (i < 4 || i + 4 >= grounded.length) continue;
        const window = grounded.slice(i - 4, i + 5);
        if (window.some((sample) => sample.z === null)) continue;
        const depth = (window[0].z + window[8].z) / 2 - z;
        if (depth <= 0.5) continue;
        dips++;
        if (depth > deepestDip) {
          deepestDip = depth;
          dipAt = [round(x), round(y)];
        }
      }
      grades.sort((a, b) => a - b);
      return {
        status,
        collected,
        seconds: round((performance.now() - began) / 1000),
        distance: round(distance),
        progress: waypoints[at][2],
        maxMph: round(maxSpeed * 2.236936),
        airborneMetres: round(airborneMetres),
        blockedSeconds: round(loading / 1000),
        afterExitMetres:
          groundExit === undefined ? null : round(distance - groundExit),
        p95Grade: grades.length
          ? round(grades[Math.floor((grades.length - 1) * 0.95)])
          : null,
        peakGrade: grades.length ? round(grades.at(-1)) : null,
        dips,
        deepestDip: round(deepestDip),
        dipAt,
        steepestAt,
        surfaces: [...surfaces],
        enclosed: [...enclosed],
        stop: car,
      };
    },
    { course, key },
  );
}

async function main() {
  const args = process.argv.slice(2);
  const verify = args.includes("--verify");
  const segmentArg = args.find((arg) => arg.startsWith("--street-segment="));
  const [path, ...names] = args.filter(
    (arg) => arg !== "--verify" && arg !== segmentArg,
  );
  const segment = segmentArg
    ?.slice("--street-segment=".length)
    .split(":")
    .map(Number);
  if (segment)
    assert.ok(
      names.length === 1 &&
        segment.length === 2 &&
        segment.every(Number.isFinite) &&
        segment[0] >= 0 &&
        segment[1] > segment[0] + 8,
      "Provide one course and --street-segment=start:end in metres",
    );
  assert.ok(
    path,
    "Provide the NDJSON courses from audit-road-routes.mjs --export-courses",
  );
  const courses = readFileSync(path, "utf8").trim().split("\n").map(JSON.parse);
  const origin = process.env.SF_BASE_URL || "http://127.0.0.1:8090";
  const manifest = await fetch(`${origin}/sf/tiles/manifest.json`);
  assert.ok(manifest.ok, "Could not load the game manifest");
  const sha = createHash("sha256")
    .update(Buffer.from(await manifest.arrayBuffer()))
    .digest("hex");
  assert.ok(
    courses.every((course) => course.manifestSha256 === sha),
    "Course and browser use different bakes",
  );
  const ids = names.length ? names : courses.map(({ id }) => id);
  assert.ok(
    ids.every((id) => courses.some((course) => course.id === id)),
    "Unknown course",
  );
  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const id of ids) {
      const course = courses.find((course) => course.id === id);
      if (segment) {
        const [start, end] = segment;
        assert.ok(end <= course.waypoints.at(-1)[2], "Segment exceeds course");
        const points = course.waypoints.filter(
          ([, , s]) => s >= start && s <= end,
        );
        assert.ok(
          points.length > 1 && points.at(-1)[2] - points[0][2] >= 4,
          "Street segment has no usable waypoints",
        );
        course.streetSegment = segment;
        course.waypoints = points.map(([x, y, s, speed]) => [
          x,
          y,
          s - points[0][2],
          speed,
        ]);
      }
      const [x, y] = course.waypoints[0];
      const [east, north] = course.waypoints.find((point) => point[2] >= 4);
      const checkpoint = { x, y, heading: Math.atan2(north - y, east - x) };
      const key = `sf-progress-v1:sf-trial-v2:${course.progressBaseSha256}:${course.placementSha256}:downtown`;
      const page = await browser.newPage({
        viewport: { width: 480, height: 320 },
        serviceWorkers: "block",
      });
      const errors = [];
      const arrow = new Set();
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("request", (request) => {
        const url = request.url();
        if (/arrow-worker|\/city-tiles\//.test(url)) arrow.add(url);
      });
      try {
        await page.addInitScript(
          ({ key, checkpoint }) => {
            localStorage.setItem(
              key,
              JSON.stringify({ collected: [], elapsed: 0, checkpoint }),
            );
          },
          { key, checkpoint },
        );
        const manifestUrl = new URL("/sf/tiles/manifest.json", origin).href;
        const [observed] = await Promise.all([
          page.waitForResponse(manifestUrl),
          page.goto(`${origin}/sf/?at=downtown`),
        ]);
        assert.ok(
          observed.ok(),
          "The browser could not load the course manifest",
        );
        assert.equal(
          createHash("sha256")
            .update(await observed.body())
            .digest("hex"),
          sha,
          "The deployed game switched bakes before this course started",
        );
        const canvas = page.locator('#drive[data-contact="grounded"]');
        await canvas.waitFor({ timeout: 60_000 });
        await canvas.focus();
        await confirmManualTakeover(page);
        const initial = await canvas.evaluate(({ dataset }) => [
          Number(dataset.east),
          Number(dataset.north),
        ]);
        assert.ok(
          Math.hypot(initial[0] - x, initial[1] - y) < 4,
          "Checkpoint did not load",
        );
        const result = await drive(page, course, key);
        console.log(
          JSON.stringify({
            id,
            manifest: sha,
            streetSegment: segment,
            ...result,
            errors,
            arrow: [...arrow],
          }),
        );
        assert.deepEqual(errors, []);
        assert.deepEqual([...arrow], []);
        if (segment) {
          assert.equal(result.status, "street segment completed", id);
          assert.equal(result.airborneMetres, 0, id);
        } else if (verify) {
          assert.equal(result.status, "both emoji collected", id);
          if (course.expectedSurfaces) {
            assert.deepEqual(result.surfaces, course.expectedSurfaces, id);
            assert.ok(
              result.afterExitMetres >= 20 && result.stop.road === null,
              `${id} did not clear its final road contact`,
            );
          }
          if (course.expectedEnclosed)
            assert.deepEqual(result.enclosed, course.expectedEnclosed, id);
          if (course.maxAirborneMetres !== undefined)
            assert.ok(
              result.airborneMetres <= course.maxAirborneMetres,
              `${id} travelled ${result.airborneMetres} m airborne`,
            );
        }
      } catch (error) {
        console.log(
          JSON.stringify({
            id,
            manifest: sha,
            status: "browser error",
            error: error.message,
          }),
        );
        process.exitCode = 1;
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
