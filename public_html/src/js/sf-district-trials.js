// Optional district metadata is published by the World pipeline in quest-route.json.
// Until it exists, the city run remains available and district runs stay unavailable.
export function districtTrials(route, manifest) {
  const source = route.districts;
  if (source === undefined) return [];
  if (
    source.version !== 1 ||
    typeof source.scheme !== "string" ||
    !source.scheme ||
    typeof source.revision !== "string" ||
    !source.revision ||
    !Array.isArray(source.areas) ||
    source.frame !== "EPSG:7131" ||
    JSON.stringify(source.origin_epsg7131) !==
      JSON.stringify(manifest.origin_epsg7131)
  ) {
    throw new Error("Unsupported district metadata or coordinate frame");
  }
  const knownTiles = new Set(manifest.tiles.map((t) => t.t));
  const targets = new Map(route.targets.map((t) => [t.t, t]));
  const ids = new Set(),
    assigned = new Set();
  return source.areas.map((area) => {
    if (
      typeof area.id !== "string" ||
      !area.id ||
      ["city", "unavailable"].includes(area.id) ||
      typeof area.name !== "string" ||
      !area.name ||
      ids.has(area.id) ||
      !Array.isArray(area.overlapTiles) ||
      !Array.isArray(area.targetTiles) ||
      !area.targetTiles.length ||
      new Set(area.overlapTiles).size !== area.overlapTiles.length ||
      new Set(area.targetTiles).size !== area.targetTiles.length ||
      area.overlapTiles.some((t) => !knownTiles.has(t)) ||
      area.targetTiles.some((t) => !targets.has(t) || assigned.has(t))
    ) {
      throw new Error(
        "Invalid district targets or overlapping target assignments",
      );
    }
    ids.add(area.id);
    area.targetTiles.forEach((t) => assigned.add(t));
    const polygons =
      area.geometry?.type === "Polygon"
        ? [area.geometry.coordinates]
        : area.geometry?.type === "MultiPolygon"
          ? area.geometry.coordinates
          : null;
    if (
      !Array.isArray(polygons) ||
      !polygons.length ||
      polygons.some(
        (p) =>
          !Array.isArray(p) ||
          !p.length ||
          p.some(
            (r) =>
              !Array.isArray(r) ||
              r.length < 4 ||
              r.some(
                (v) =>
                  !Array.isArray(v) ||
                  v.length !== 2 ||
                  !v.every(Number.isFinite),
              ) ||
              r[0][0] !== r.at(-1)[0] ||
              r[0][1] !== r.at(-1)[1],
          ),
      )
    )
      throw new Error("Invalid district boundary");
    if (
      polygons.some(
        (p) =>
          Math.abs(
            p[0].reduce((sum, a, i, r) => {
              const b = r[(i + 1) % r.length];
              return sum + a[0] * b[1] - b[0] * a[1];
            }, 0),
          ) < 1e-6,
      )
    )
      throw new Error("Empty district polygon");
    const selected = area.targetTiles.map((t) => targets.get(t));
    if (selected.some((t) => !pointInDistrict(t.x, t.y, polygons)))
      throw new Error("Pickup outside its assigned district");
    return {
      id: String(area.id),
      name: String(area.name),
      polygons,
      targets: selected,
      overlapTiles: new Set(area.overlapTiles),
      start: {
        ...selected[0],
        id: `district-${area.id}`,
        name: `${area.name} start`,
      },
      recordScope: JSON.stringify([
        source.scheme,
        source.revision,
        area.id,
        selected[0].t,
        [...area.targetTiles].sort(),
      ]),
    };
  });
}

function ringContains(x, y, ring) {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [ax, ay] = ring[j],
      [bx, by] = ring[i];
    const cross = (x - ax) * (by - ay) - (y - ay) * (bx - ax);
    if (
      Math.abs(cross) < 1e-8 &&
      x >= Math.min(ax, bx) - 1e-8 &&
      x <= Math.max(ax, bx) + 1e-8 &&
      y >= Math.min(ay, by) - 1e-8 &&
      y <= Math.max(ay, by) + 1e-8
    )
      return true;
    if (ay > y !== by > y && x < ((bx - ax) * (y - ay)) / (by - ay) + ax)
      inside = !inside;
  }
  return inside;
}

export function pointInDistrict(x, y, polygons) {
  return polygons.some(
    (rings) =>
      ringContains(x, y, rings[0]) &&
      !rings.slice(1).some((r) => ringContains(x, y, r)),
  );
}

export function seekerDirection(relative) {
  if (relative === null) return "All collected";
  const angle = Math.abs(relative);
  if (angle < 15) return "Straight ahead";
  if (angle > 150) return "Turn around";
  return `${angle < 60 ? "Bear" : "Turn"} ${relative > 0 ? "right" : "left"}`;
}
