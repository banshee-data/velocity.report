export const WEB_MERCATOR_MAX_LATITUDE = 85.0511287798066;

function clamp(value, minimum, maximum) {
  return Math.min(maximum, Math.max(minimum, value));
}

/** Keep local geometry together if a future selected cell crosses the antimeridian. */
export function unwrapLongitude(lng, anchorLng) {
  let unwrapped = lng;
  while (unwrapped - anchorLng > 180) unwrapped -= 360;
  while (unwrapped - anchorLng < -180) unwrapped += 360;
  return unwrapped;
}

/** Project WGS84 latitude/longitude to Web Mercator world coordinates. */
export function projectWebMercator({ lat, lng }, anchorLng = lng) {
  const safeLatitude = clamp(lat, -WEB_MERCATOR_MAX_LATITUDE, WEB_MERCATOR_MAX_LATITUDE);
  const longitude = unwrapLongitude(lng, anchorLng);
  const latitudeRadians = (safeLatitude * Math.PI) / 180;
  return {
    x: (longitude + 180) / 360,
    y:
      (1 -
        Math.log(Math.tan(latitudeRadians) + 1 / Math.cos(latitudeRadians)) / Math.PI) /
      2,
  };
}

export function parseViewBox(viewBox, width, height) {
  const values = viewBox ?? [0, 0, width, height];
  if (
    !Array.isArray(values) ||
    values.length !== 4 ||
    !values.every(Number.isFinite) ||
    values[2] <= 0 ||
    values[3] <= 0
  ) {
    throw new Error("viewBox must contain four finite numbers with positive width and height.");
  }
  return [...values];
}

function projectedBounds(points) {
  if (points.length === 0) throw new Error("Cannot fit an empty set of projected points.");
  return points.reduce(
    (bounds, point) => ({
      minX: Math.min(bounds.minX, point.x),
      minY: Math.min(bounds.minY, point.y),
      maxX: Math.max(bounds.maxX, point.x),
      maxY: Math.max(bounds.maxY, point.y),
    }),
    { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity },
  );
}

/** Create a shared affine fit so every cell and mask uses identical SVG coordinates. */
export function createSvgFit(points, options = {}) {
  const width = options.width ?? 1000;
  const height = options.height ?? 1000;
  const padding = options.padding ?? 40;
  const viewBox = parseViewBox(options.viewBox, width, height);
  if (![width, height, padding].every(Number.isFinite) || width <= 0 || height <= 0) {
    throw new Error("width, height, and padding must be finite; dimensions must be positive.");
  }

  const [viewX, viewY, viewWidth, viewHeight] = viewBox;
  if (padding < 0 || padding * 2 >= viewWidth || padding * 2 >= viewHeight) {
    throw new RangeError("padding must leave a positive drawable area inside the viewBox.");
  }

  const bounds = projectedBounds(points);
  const sourceWidth = Math.max(bounds.maxX - bounds.minX, Number.EPSILON);
  const sourceHeight = Math.max(bounds.maxY - bounds.minY, Number.EPSILON);
  const scale = Math.min(
    (viewWidth - padding * 2) / sourceWidth,
    (viewHeight - padding * 2) / sourceHeight,
  );
  const drawnWidth = sourceWidth * scale;
  const drawnHeight = sourceHeight * scale;
  const offsetX = viewX + (viewWidth - drawnWidth) / 2 - bounds.minX * scale;
  const offsetY = viewY + (viewHeight - drawnHeight) / 2 - bounds.minY * scale;

  return {
    name: "Web Mercator",
    width,
    height,
    padding,
    viewBox,
    sourceBounds: bounds,
    scale,
    project(point) {
      return {
        x: point.x * scale + offsetX,
        y: point.y * scale + offsetY,
      };
    },
  };
}

function projectGeographicPoint(point, anchorLng) {
  return {
    ...point,
    world: projectWebMercator(point, anchorLng),
  };
}

/** Project a geographic S2 traversal into one fitted SVG geometry model. */
export function projectTraversal(traversal, options = {}) {
  const anchorLng = traversal.parent.centre.lng;
  const parentWorld = traversal.parent.vertices.map((point) =>
    projectGeographicPoint(point, anchorLng),
  );
  const cellsWorld = traversal.cells.map((cell) => ({
    ...cell,
    centre: projectGeographicPoint(cell.centre, anchorLng),
    vertices: cell.vertices.map((point) => projectGeographicPoint(point, anchorLng)),
  }));
  const allPoints = [
    ...parentWorld.map((point) => point.world),
    ...cellsWorld.flatMap((cell) => [
      cell.centre.world,
      ...cell.vertices.map((point) => point.world),
    ]),
  ];
  const fit = createSvgFit(allPoints, options);
  const fitPoint = (point) => ({ ...point, ...fit.project(point.world) });
  const cells = cellsWorld.map((cell) => ({
    ...cell,
    centre: fitPoint(cell.centre),
    vertices: cell.vertices.map(fitPoint),
  }));

  return {
    ...traversal,
    parent: {
      ...traversal.parent,
      centre: fitPoint(projectGeographicPoint(traversal.parent.centre, anchorLng)),
      vertices: parentWorld.map(fitPoint),
    },
    cells,
    path: cells.map((cell) => ({
      index: cell.index,
      token: cell.token,
      lat: cell.centre.lat,
      lng: cell.centre.lng,
      world: cell.centre.world,
      x: cell.centre.x,
      y: cell.centre.y,
    })),
    projection: {
      name: fit.name,
      anchorLng,
      width: fit.width,
      height: fit.height,
      padding: fit.padding,
      viewBox: fit.viewBox,
      sourceBounds: fit.sourceBounds,
      scale: fit.scale,
    },
    projectWorldPoint: fit.project,
  };
}
