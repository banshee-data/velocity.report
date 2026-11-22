export const SOMA_SURVEY_DRIVE = "soma-survey-drive";
export const SOMA_SURVEY_NAME = "SoMa survey deliveries";

export const SOMA_SURVEY_STOPS = ["1st-mission", "3rd-folsom", "howard-6th"];

// Junctions in the bundled planar street grid, in the direction of travel.
const junctions = [
  [3685.1, 3356.4],
  [3736.6, 3304.7],
  [3872.3, 3168.6],
  [4145.4, 2894.8],
  [3949.6, 2699.7],
  [3753.7, 2504.6],
  [3617.6, 2641.1],
  [3344.8, 2914.4],
  [2952.6, 2523.9],
  [3088.9, 2387.2],
  [2892.9, 2191.7],
  [2835, 2134],
];

const mix = (a, b, t) => a.map((value, i) => value + (b[i] - value) * t);
const distance = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);

export function somaSurveyDrive() {
  const points = [];
  const append = (point) => {
    const previous = points.at(-1);
    const steps = previous ? Math.ceil(distance(previous, point) / 2) : 1;
    for (let i = 1; i <= steps; i++)
      points.push(previous ? mix(previous, point, i / steps) : point);
  };
  append(junctions[0]);
  for (const [i, corner] of junctions.slice(1, -1).entries()) {
    const previous = junctions[i];
    const next = junctions[i + 2];
    const a = mix(
      corner,
      previous,
      Math.min(1 / 3, 12 / distance(corner, previous)),
    );
    const b = mix(corner, next, Math.min(1 / 3, 12 / distance(corner, next)));
    append(a);
    for (let step = 1; step <= 12; step++) {
      const t = step / 12;
      append(mix(mix(a, corner, t), mix(corner, b, t), t));
    }
  }
  append(junctions.at(-1));
  const seam = Math.floor(points.length / 2);
  return {
    id: SOMA_SURVEY_DRIVE,
    label: SOMA_SURVEY_NAME,
    // These inferred footprints span the carriageways by Fifth and Howard.
    scene_exclusions: {
      80858084: { b: { 954: [62, 60, 1], 962: [71, 71, 1] } },
    },
    sections: [points.slice(0, seam + 1), points.slice(seam)].map(
      (line, i) => ({
        id: i ? "soma-west" : "soma-east",
        label: SOMA_SURVEY_NAME,
        kind: "ground",
        width_m: 8,
        speed_mps: 11.18,
        positions: line.map(([x, y]) => [x, y, 0]),
        sources: [],
      }),
    ),
  };
}
