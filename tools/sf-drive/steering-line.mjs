// Offset the test driver's target line within paved lanes. Contact geometry
// and obstacle checks continue to use the original game assets.
export function steeringLine(points, guide = {}) {
  const {
    lane = 0,
    approach = 0,
    approachMetres = 32,
    holdApproach = false,
    entranceEast = 0,
    entranceNorth = 0,
    entranceMetres = 12,
    clearance,
  } = guide;
  const clearances = Array.isArray(clearance)
    ? clearance
    : [clearance].filter(Boolean);
  const smooth = (value) => {
    const t = Math.max(0, Math.min(1, value));
    return t * t * (3 - 2 * t);
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
    const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
    const remaining = distances.at(-1) - distances[i];
    const exitLane =
      approach *
      smooth((approachMetres - remaining) / ((approachMetres * 18) / 32)) *
      (holdApproach ? 1 : 1 - Math.exp(-remaining / 5));
    const entranceLane = smooth(1 - distances[i] / entranceMetres);
    const clearanceLane = clearances.reduce(
      (sum, { offset, station, length }) =>
        sum +
        offset * Math.max(0, 1 - ((distances[i] - station) / length) ** 2) ** 2,
      0,
    );
    const lateral = lane + exitLane + clearanceLane;
    return [
      x + entranceEast * entranceLane - lateral * Math.sin(heading),
      y + entranceNorth * entranceLane + lateral * Math.cos(heading),
      z,
    ];
  });
}

export function streetCourseWaypoints(points, start, finish, speed) {
  const segments = [];
  let station = 0;
  for (const [i, [x, y]] of points.slice(0, -1).entries()) {
    const [bx, by] = points[i + 1];
    const [dx, dy] = [bx - x, by - y];
    const length = Math.hypot(dx, dy);
    if (!length) continue;
    segments.push({ station, x, y, dx, dy, length });
    station += length;
  }
  function at(distance) {
    const segment = segments.find(
      ({ station, length }) => distance <= station + length,
    );
    const { x, y, dx, dy, length } = segment;
    const t = (distance - segment.station) / length;
    return [x + t * dx, y + t * dy];
  }
  const run = finish - start;
  const intervals = Math.ceil(Math.abs(run) / 2.3);
  return Array.from({ length: intervals + 1 }, (_, i) => {
    const progress = (run * i) / intervals;
    const s = start + progress;
    const [x, y] = at(s);
    const [ax, ay] = at(s - 0.75);
    const [bx, by] = at(s + 0.75);
    const right = (Math.sign(run) * 1.5) / Math.hypot(bx - ax, by - ay);
    return [
      x + right * (by - ay),
      y - right * (bx - ax),
      Math.abs(progress),
      speed,
    ];
  });
}
