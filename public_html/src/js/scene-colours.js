// One palette for the homepage and every public scene. A class should not
// change colour merely because somebody followed a link.
export const SCENE_COLOURS = {
  canvas: 0x07090c,
  vehicle: 0xf2504b,
  cycle: 0x6aa9ff,
  walking: 0x10b981,
  primary: 0x10b981,
  primaryInk: 0x07090c,
  bird: 0xb08bd4,
  dynamic: 0x8899a6,
  noise: 0x55606b,
};

export const SCENE_CSS_COLOURS = Object.fromEntries(
  Object.entries(SCENE_COLOURS).map(([name, colour]) => [
    name,
    `#${colour.toString(16).padStart(6, "0")}`,
  ]),
);
