// Shared colour roles for the game, the homepage illustration and the surveys.
// The site stylesheet exposes these same values to the DOM.
export const SCENE_COLOURS = Object.freeze({
  canvas: 0x07090c,
  panel: 0x0d1218,
  panelBorder: 0x34434e,
  text: 0xe6ebf2,
  textSoft: 0xa7b2c0,
  vehicle: 0xf2504b,
  vehicleShade: 0x7f2a33,
  cycle: 0x6aa9ff,
  walking: 0x10b981,
  primary: 0x10b981,
  primaryInk: 0x07090c,
  primaryText: 0x6ee7b7,
  bird: 0xb08bd4,
  dynamic: 0x8899a6,
  noise: 0x55606b,
  asphalt: 0x202d3b,
  path: 0x364552,
  kerb: 0x91a5b0,
  buildingWall: 0x7b858c,
  buildingRoof: 0x9a8f7c,
  buildingOutline: 0xb1aea3,
  buildingInferred: 0xa48469,
  treeCrown: 0x245238,
  treeTrunk: 0x6b503b,
});

export const SCENE_CSS_COLOURS = Object.freeze(
  Object.fromEntries(
    Object.entries(SCENE_COLOURS).map(([name, colour]) => [
      name,
      `#${colour.toString(16).padStart(6, "0")}`,
    ]),
  ),
);

export function roadUserKind(className) {
  switch (className) {
    case "car":
    case "bus":
    case "truck":
      return "vehicle";
    case "pedestrian":
      return "walking";
    case "cyclist":
    case "motorcyclist":
      return "cycle";
    default:
      return null;
  }
}

export function sceneTrackColour(className) {
  const kind = roadUserKind(className);
  if (kind) return SCENE_COLOURS[kind];
  if (className === "bird" || className === "noise")
    return SCENE_COLOURS[className];
  return SCENE_COLOURS.dynamic;
}
