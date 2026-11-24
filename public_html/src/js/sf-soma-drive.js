export const BRIDGE_SURVEY_DRIVE = "bridge-survey-drive";
export const BRIDGE_SURVEY_NAME = "Bay Bridge · SoMa survey deliveries";
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

function roundedStreet(junctions) {
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
  return points;
}

const surveyScenery = {
  80858084: {
    buildingFootprints: [
      { signature: [62, 60, 1], firstPoint: [1452, 391, 735] },
      { signature: [71, 71, 1], firstPoint: [1612, 1131, 800] },
    ],
  },
};

export function somaSurveyDrive() {
  const points = roundedStreet(junctions);
  const seam = Math.floor(points.length / 2);
  return {
    id: SOMA_SURVEY_DRIVE,
    label: SOMA_SURVEY_NAME,
    // These inferred footprints span the carriageways by Fifth and Howard.
    scene_exclusions: surveyScenery,
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

/** Authored street itinerary between the same verified Fremont and Fourth Street joins. */
export function bridgeSurveyDrive(bridge) {
  const street = bridge.sections.find(({ id }) => id === "city-streets");
  // Find the validated straight approach stations by position, rather than
  // by array index, so a changed bake cannot silently splice another block.
  const station = (anchor) =>
    street?.positions.findIndex((p) => distance(p, anchor) < 0.01) ?? -1;
  const from = station([3972.2164, 3224.0635]);
  const to = station([3893.087, 2643.4133]);
  if (from < 1 || to <= from || to >= street.positions.length - 1)
    throw new Error(
      "Survey connectors require the supported Fremont and Fourth Street route.",
    );
  const points = roundedStreet([
    street.positions[from].slice(0, 2),
    [3814.4, 3382.2], // Fremont / Mission
    [3541.0, 3109.8], // Mission past 1st to 2nd
    [3813.7, 2836.5], // 2nd / Folsom
    [3029.1, 2054.9], // Folsom past 3rd to 6th
    [2892.8, 2191.6], // 6th / Howard
    [3089.0, 2387.1], // Howard / 5th
    [3225.0, 2250.3], // 5th / Folsom
    [3813.7, 2836.5], // Folsom / 2nd
    [3949.8, 2699.9], // 2nd / Harrison
    street.positions[to].slice(0, 2),
  ]).map(([x, y]) => [x, y, 0]);
  // Separate outbound and return street spans so a crossing cannot select a later leg.
  const split = points.findIndex(
    ([x, y], i) => i > points.length / 2 && x < 2960 && y > 2140,
  );
  if (split < 1) throw new Error("Survey street handoff is missing.");
  const first = [
    ...street.positions.slice(0, from + 1),
    ...points.slice(1, split + 1),
  ];
  const last = [...points.slice(split, -1), ...street.positions.slice(to)];
  const streets = [first, last].map((positions, i) => ({
    ...street,
    id: i ? "survey-streets-return" : "survey-streets-outbound",
    label: i ? "SoMa · back to Fourth Street" : "SoMa · survey deliveries",
    speed_mps: 11.18,
    sources: [],
    positions,
  }));
  return {
    ...bridge,
    id: BRIDGE_SURVEY_DRIVE,
    label: BRIDGE_SURVEY_NAME,
    initial_speed_mps: 50 * 0.44704,
    scene_exclusions: {
      ...bridge.scene_exclusions,
      80858084: {
        ...bridge.scene_exclusions[80858084],
        ...surveyScenery[80858084],
      },
    },
    sections: bridge.sections.flatMap((span) =>
      span === street ? streets : [span],
    ),
  };
}
