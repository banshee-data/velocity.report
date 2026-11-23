// Screen ground-draped carriageways for grade, grade changes and missing contact.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { gunzipSync } from "node:zlib";
import { groundAt } from "../../public_html/src/js/sf-drive-core.js";

const directory = process.argv[2] || "public_html/src/sf/tiles";
const { tiles } = JSON.parse(readFileSync(join(directory, "manifest.json")));
const world = tiles.map((tile) => ({
  ...tile,
  data: JSON.parse(
    gunzipSync(readFileSync(join(directory, `${tile.t}.json.gz`))),
  ),
}));

function ground(x, y) {
  let owner,
    best = Infinity;
  for (const tile of world) {
    const d = (x - tile.x) ** 2 + (y - tile.y) ** 2;
    if (d < best) {
      best = d;
      owner = tile;
    }
  }
  return groundAt(
    owner.data.g,
    x - owner.data.o[0],
    y - owner.data.o[1],
    false,
  );
}

const report = {
  roadRows: 0,
  supportedIntervals: 0,
  missingIntervals: 0,
  gradeOver25Percent: 0,
  gradeOver50Percent: 0,
  gradeChangeOver25Points: 0,
};
const worst = [];
for (const { t, data } of world) {
  for (const [name, kind, , xy, widths] of data.r) {
    if (kind < 2 || kind > 5 || !widths.length || xy.length < 4) continue;
    report.roadRows++;
    let x = data.o[0] + xy[0] / 10;
    let y = data.o[1] + xy[1] / 10;
    let z = ground(x, y),
      previousGrade;
    let maxGrade = 0,
      peak;
    for (let i = 2; i < xy.length; i += 2) {
      const endX = data.o[0] + xy[i] / 10;
      const endY = data.o[1] + xy[i + 1] / 10;
      const length = Math.hypot(endX - x, endY - y);
      if (length < 1e-6) continue;
      const steps = Math.ceil(length / 2);
      const dx = (endX - x) / steps,
        dy = (endY - y) / steps;
      for (let step = 0; step < steps; step++) {
        x += dx;
        y += dy;
        const next = ground(x, y);
        if (z === null || next === null) {
          report.missingIntervals++;
          previousGrade = undefined;
        } else {
          report.supportedIntervals++;
          const grade = ((next - z) / length) * steps;
          const magnitude = Math.abs(grade);
          if (magnitude > 0.25) report.gradeOver25Percent++;
          if (magnitude > 0.5) report.gradeOver50Percent++;
          if (
            previousGrade !== undefined &&
            Math.abs(grade - previousGrade) > 0.25
          )
            report.gradeChangeOver25Points++;
          previousGrade = grade;
          if (magnitude > maxGrade) {
            maxGrade = magnitude;
            peak = [Math.round(x * 10) / 10, Math.round(y * 10) / 10];
          }
        }
        z = next;
      }
      x = endX;
      y = endY;
    }
    if (maxGrade > 0.5)
      worst.push({ tile: t, name: data.n[name], peak, grade: maxGrade });
  }
}
worst.sort((a, b) => b.grade - a.grade);
console.log(JSON.stringify({ ...report, worst: worst.slice(0, 10) }, null, 2));
