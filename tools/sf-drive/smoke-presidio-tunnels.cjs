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
const lowerBridgeOwners = [
  "80858014",
  "80858024",
  "8085803c",
  "8085806c",
  "80858074",
  "8085807c",
];
const tilePath = (token) => `tiles/${token}.json.gz`;
const prefix = "sf-contact/";
const eastSpanEastbound = "bay-bridge-east-span-eastbound";
const eastSpanWestbound = "bay-bridge-east-span-westbound";
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
    forks: [["presidio-nb-viaduct", "presidio-nb-exit", -1.65]],
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
const marinOwners = [
  "80858424",
  "8085842c",
  "80858434",
  "808585cc",
  "8085869c",
];
cases["alexander-to-waldo-nb"] = {
  owners: marinOwners,
  roads: [
    "us101-north-after-alexander-merge",
    "101-north-alexander-to-waldo",
    "robin-williams-nb",
  ],
  speed: 17,
  rollingStart: true,
  initialSpeedMps: 16,
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 160000,
  expectNoScrapes: true,
};
cases["waldo-to-alexander-sb"] = {
  owners: marinOwners,
  roads: [
    "robin-williams-sb",
    "101-south-waldo-to-alexander",
    "us101-south-alexander-fork",
    "us101-south-over-alexander",
  ],
  speed: 16,
  finishOnDeck: true,
  finishDistance: 60,
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 160000,
  expectNoScrapes: true,
};
cases["alexander-to-baker"] = {
  owners: marinOwners,
  roads: [
    "alexander-avenue-east-junction",
    "bunker-road-to-alexander",
    "bunker-tunnel-exterior",
    "baker-barry",
  ],
  speed: 11,
  rollingStart: true,
  initialSpeedMps: 5,
  reverse: true,
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 185000,
  expectNoScrapes: true,
};
cases["baker-barry-eastbound"] = {
  owners: marinOwners,
  roads: ["baker-barry", "bunker-tunnel-exterior"],
  speed: 12,
  finishOnDeck: true,
  finishDistance: 16,
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 120_000,
  expectNoScrapes: true,
};
cases["baker-to-bunker-underpass"] = {
  owners: marinOwners,
  roads: [
    "baker-barry",
    "bunker-tunnel-exterior",
    "bunker-road-east-short-branch",
    "bunker-road-under-101",
  ],
  rollingStart: true,
  rollingStartMetres: 686,
  speed: 9,
  initialSpeedMps: 9,
  speedLimits: { "bunker-road-east-short-branch": 7 },
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 100000,
  expectNoScrapes: true,
};
const dalyOwners = ["808f7c3c", "808f7c44"];
const dalyUpper = "john-daly-westbound-overpass";
const dalyMerged = "i280-south-after-john-daly-merge";
cases["daly-ca1-north-connector"] = {
  owners: dalyOwners,
  roads: ["daly-ca1-north-connector"],
  speed: 12,
  groundApproachMetres: 9,
  maxAirborneMetres: 0,
  maxDurationMs: 70_000,
  expectNoScrapes: true,
};
cases["i280-john-daly-off-and-back"] = {
  owners: dalyOwners,
  roads: [
    "i280-south-edge-exit",
    "i280-south-john-daly-exit",
    dalyUpper,
    "john-daly-southbound-i280-entry",
    dalyMerged,
  ],
  rollingStart: true,
  rollingStartMetres: 355,
  initialSpeedMps: 14,
  speed: 14,
  speedLimits: {
    "i280-south-john-daly-exit": 11,
    "john-daly-southbound-i280-entry": 11,
  },
  maxAirborneMetresAfterEntrance: 0,
  maxDurationMs: 185_000,
  expectNoScrapes: true,
};
for (const [direction, offset] of [
  ["eastbound", 4.5],
  ["westbound", 3],
]) {
  const road = `john-daly-${direction}`;
  for (const [lane, position] of [
    ["centre", 0],
    ["left", offset],
    ["right", -offset],
  ]) {
    cases[`${road}-${lane}`] = {
      owners: dalyOwners,
      roads: direction === "westbound" ? [road, dalyUpper] : [road],
      laneGuides: {
        [road]: { lane: position },
        [dalyUpper]: { lane: position },
      },
      finishOnDeck: direction === "westbound",
      finishDistance: direction === "westbound" ? 127 : undefined,
      speed: 13,
      groundApproachMetres: 9,
      maxAirborneMetresAfterEntrance: 0,
      expectNoScrapes: true,
      maxDurationMs: 65_000,
    };
  }
}
const dalyApproach = "i280-south-sr82-to-edge";
const dalyTunnel = "i280-south-daly-separation";
const dalyMainline = "i280-daly-tunnel-to-return-merge";
for (const [lane, offset] of [
  ["left", 3.7],
  ["centre", 0],
  ["right", -3.7],
]) {
  cases[`daly-separation-${lane}`] = {
    owners: dalyOwners,
    roads: [dalyApproach, dalyTunnel, dalyMainline, dalyMerged],
    laneGuides: {
      [dalyApproach]: { lane: offset },
      [dalyTunnel]: { lane: offset },
      [dalyMainline]: { lane: offset },
      [dalyMerged]: { lane: offset },
    },
    rollingStart: true,
    rollingStartMetres: 390,
    initialSpeedMps: 18,
    speed: 18,
    maxAirborneMetresAfterEntrance: 0,
    expectNoScrapes: true,
  };
}
for (const reverse of [false, true]) {
  const name = reverse
    ? "garage-concourse-to-fulton"
    : "garage-fulton-to-concourse";
  cases[name] = {
    roads: ["music-concourse-garage"],
    owners: ["80858744"],
    reverse,
    speed: 5.2,
    groundApproachMetres: 7,
    maxAirborneMetresAfterEntrance: 0,
    maxDurationMs: 215_000,
  };
}
const doyleNorthExit = "presidio-doyle-northbound-exit";
cases["presidio-doyle-northbound"] = {
  ...cases.northbound,
  roads: [
    ...cases.northbound.roads.slice(0, -1),
    doyleNorthExit,
    "golden-gate-northbound-1",
  ],
  forks: [["presidio-nb-viaduct", doyleNorthExit, 1.65]],
  finishOnDeck: true,
  finishDistance: 45,
};
cases["presidio-doyle-southbound"] = {
  ...cases.southbound,
  roads: [
    "presidio-doyle-southbound-entry",
    ...cases.southbound.roads.slice(1),
  ],
};
const marinaEntrance = "presidio-marina-northbound-entry";
for (const [name, through] of [
  ["presidio-marina-to-doyle", cases["presidio-doyle-northbound"]],
  ["presidio-marina-to-sr1", cases.northbound],
]) {
  cases[name] = {
    ...through,
    roads: [marinaEntrance, ...through.roads.slice(2)],
    speedLimits: { [marinaEntrance]: 12 },
    laneGuides: {
      [marinaEntrance]: {
        clearance: { offset: -1.7, station: 218, length: 32 },
      },
    },
  };
}
const southboundFifthApproach = "i280-fifth-south-to-sixth-merge";
const i280Approach = [
  "i280-fifth-south-entry",
  southboundFifthApproach,
  "i280-king-to-mariposa",
];
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
const southAlemanyApproach = "i280-south-beyond-dogpatch";
const southAlemanyMainline = "i280-south-to-alemany-mainline";
const southAlemanyLeftExit = "i280-south-to-us101-south-exit";
const southAlemanyMainlineClearance = {
  clearance: { ...dogpatchClearance.clearance, station: 224 },
};
const eighteenthOffRamp = "i280-18th-off-ramp";
const eighteenthExit = "i280-18th-exit";
const eighteenthWestTurn = "i280-18th-exit-west-turn";
const eighteenthEastTurn = "i280-18th-exit-east-turn";
const eighteenthEastDeck = "eighteenth-east-viaduct";
const eighteenthWestDeck = "eighteenth-west-viaduct";
const eighteenthJunction = "eighteenth-viaduct-junction";
const eighteenthLanding = "eighteenth-west-landing";
const eighteenthEastDeparture = [
  [4344.2, 326.4, 12.795],
  [4356.3, 327.3, 12.439],
  [4374.6, 328.8, 11.235],
];
const eighteenthWestExit = [
  eighteenthOffRamp,
  eighteenthExit,
  eighteenthWestTurn,
  eighteenthLanding,
];
const eighteenthEastExit = [
  eighteenthOffRamp,
  eighteenthExit,
  eighteenthEastTurn,
  eighteenthWestDeck,
  eighteenthEastDeck,
];
const eighteenthExitLimits = {
  [eighteenthExit]: 6,
  [eighteenthWestTurn]: 5,
  [eighteenthEastTurn]: 5,
  [eighteenthLanding]: 7,
};
cases["i280-pennsylvania"] = {
  roads: [...i280Approach, eighteenthOffRamp, "i280-pennsylvania-exit"],
  owners: i280Owners,
  speed: 12,
  rampApproach: true,
};
cases["i280-18th"] = {
  roads: [...i280Approach, ...eighteenthWestExit],
  owners: i280Owners,
  speed: 12,
  speedLimits: eighteenthExitLimits,
  rampApproach: true,
  groundedHandoff: true,
};
for (const [name, roads, reverseRoads] of [
  ["westbound", eighteenthWestExit, []],
  ["eastbound", eighteenthEastExit, [eighteenthWestDeck, eighteenthEastDeck]],
]) {
  cases[`i280-18th-exit-${name}`] = {
    roads,
    reverseRoads,
    owners: i280Owners,
    speed: 9,
    speedLimits: eighteenthExitLimits,
    rollingStart: true,
    rollingStartMetres: 305,
    initialSpeedMps: 9,
    ...(name === "eastbound" && {
      connectByGround: true,
      groundDeparture: eighteenthEastDeparture,
    }),
    maxAirborneMetres: 0,
    maxDurationMs: 65_000,
  };
}
cases["i280-mainline"] = {
  roads: [
    ...dogpatchApproach,
    "i280-south-through-dogpatch",
    southAlemanyApproach,
    southAlemanyMainline,
    "i280-south-alemany-merge-tail",
    "i280-south-alemany-through",
    "i280-south-alemany-exit",
  ],
  owners: [...dogpatchOwners, "808f7ef4"],
  speed: 22,
  speedLimits: {
    "i280-south-through-dogpatch": 16,
    "i280-south-alemany-exit": 11,
  },
  forks: [
    [southAlemanyApproach, southAlemanyMainline, -1.5],
    ["i280-south-alemany-through", "i280-south-alemany-exit", -1.6],
  ],
  laneGuides: {
    "i280-dogpatch-approach": { approach: 1.25, holdApproach: true },
    "i280-south-through-dogpatch": { entranceEast: 1.25, entranceMetres: 20 },
    [southAlemanyMainline]: southAlemanyMainlineClearance,
  },
};
cases["sixth-southbound"] = {
  roads: [
    "i280-sixth-southbound",
    "i280-king-to-mariposa",
    ...eighteenthWestExit,
  ],
  owners: i280Owners,
  speed: 16,
  speedLimits: {
    "i280-king-to-mariposa": 14,
    "i280-18th-off-ramp": 12,
    ...eighteenthExitLimits,
  },
  rampApproach: true,
};
const mariposaMergeStation = 91.96837;
const sixthForkApproach = "i280-mariposa-northbound";
const sixthStreetNorthLead = "i280-mariposa-sixth-approach";
const sixthStreetNorthEnd = "i280-sixth-northbound";
const fifthStreetNorthExit = "i280-north-fifth-exit";
const sixthStreetNorthFork = [sixthForkApproach, sixthStreetNorthLead, -1.4];
const fifthStreetNorthFork = [sixthForkApproach, fifthStreetNorthExit, 4.2];
cases["sixth-northbound"] = {
  roads: [
    "eighteenth-east-viaduct",
    "i280-eighteenth-north-entry",
    sixthForkApproach,
    sixthStreetNorthLead,
    sixthStreetNorthEnd,
  ],
  owners: i280Owners,
  speed: 16,
  forks: [sixthStreetNorthFork],
  speedLimits: {
    "eighteenth-east-viaduct": 7,
    "i280-eighteenth-north-entry": 8,
  },
  laneGuides: {
    "eighteenth-east-viaduct": { approach: -2.25, holdApproach: true },
    "i280-eighteenth-north-entry": {
      entranceEast: -0.167,
      entranceNorth: 2.244,
      entranceMetres: 16,
    },
    [sixthStreetNorthEnd]: {
      // Hold the clear lane past the two mapped trees near the descent.
      clearance: { offset: -1.8, station: 696, length: 29 },
    },
  },
};
cases["eighteenth-to-fifth-northbound"] = {
  ...cases["sixth-northbound"],
  roads: [
    ...cases["sixth-northbound"].roads.slice(0, -2),
    fifthStreetNorthExit,
  ],
  forks: [fifthStreetNorthFork],
};
for (const reverse of [false, true]) {
  const viaducts = [
    eighteenthEastDeck,
    eighteenthWestDeck,
    eighteenthJunction,
    eighteenthLanding,
  ];
  cases[reverse ? "eighteenth-eastbound" : "eighteenth-westbound"] = {
    roads: reverse ? viaducts.toReversed() : viaducts,
    owners: i280Owners,
    speed: 8,
    reverse,
    ...(reverse && {
      connectByGround: true,
      groundDeparture: eighteenthEastDeparture,
    }),
  };
}
for (const [name, entry] of [
  ["essex-tiled", "essex-street-eastbound-entry"],
  ["first-street-tiled", "first-street-eastbound-entry"],
]) {
  cases[name] = {
    roads: [
      entry,
      "sf-eastbound-lower-approach",
      "bay-bridge-eastbound-lower",
      "yerba-buena-lower-west-approach",
      "yerba-buena-lower-tunnel",
      "yerba-buena-lower-tunnel-east-of-exit",
      "yerba-buena-lower-east-approach",
      "yerba-buena-lower-east-after-entry",
      eastSpanEastbound,
    ],
    owners: lowerBridgeOwners,
    speed: 22,
    speedLimits: {
      [entry]: 10,
      "yerba-buena-lower-tunnel": 17,
      "yerba-buena-lower-tunnel-east-of-exit": 17,
    },
    forks: [
      [
        "yerba-buena-lower-tunnel",
        "yerba-buena-lower-tunnel-east-of-exit",
        1.5,
      ],
    ],
    finishOnDeck: true,
  };
}
const centralOwners = [
  "8085807c",
  "80858084",
  "8085809c",
  "808580a4",
  "808f7e24",
  "808f7e2c",
];
const fourth = ["fourth-street-entrance", "westbound-i80"];
const centralNorth = [
  ...fourth,
  "i80-westbound-fifth",
  "i80-westbound-interchange",
  "i80-westbound-central-connector",
  "central-freeway-east-northbound",
];
cases["central-from-fourth"] = {
  roads: [...centralNorth, "central-freeway-octavia-exit"],
  owners: centralOwners,
  speed: 20,
  speedLimits: {
    "fourth-street-entrance": 11,
    "i80-westbound-central-connector": 14,
  },
  forks: [["westbound-i80", "i80-westbound-fifth", 1.55]],
  laneGuides: {
    "i80-westbound-fifth": {
      clearance: { offset: -2.2, station: 355, length: 26 },
    },
  },
};
cases["central-thirteenth-exit"] = {
  ...cases["central-from-fourth"],
  roads: [...centralNorth, "central-freeway-13th-exit"],
  forks: [
    ...cases["central-from-fourth"].forks,
    ["central-freeway-east-northbound", "central-freeway-13th-exit", -1.45],
  ],
};
cases["central-fifth-exit"] = {
  ...cases["central-from-fourth"],
  roads: [...fourth, "i80-westbound-fifth-exit"],
  forks: [],
};
const centralEast = [
  "central-freeway-east-southbound",
  "central-freeway-i80-east-connector",
  "i80-eastbound-interchange",
];
const centralEastFork = [centralEast[0], centralEast[1], 1.5];
cases["central-octavia-entry"] = {
  roads: ["central-freeway-octavia-entry", ...centralEast],
  owners: centralOwners,
  speed: 19,
  finishOnDeck: true,
  forks: [centralEastFork],
  laneGuides: {
    "central-freeway-octavia-entry": {
      clearance: { offset: 1.8, station: 103, length: 25 },
    },
  },
};
cases["central-van-ness-entry"] = {
  ...cases["central-octavia-entry"],
  roads: [
    "central-freeway-van-ness-entry",
    "central-freeway-van-ness-merge",
    ...centralEast,
  ],
  speedLimits: { "central-freeway-van-ness-entry": 11 },
  laneGuides: {
    "central-freeway-van-ness-entry": {
      clearance: { offset: 1.35, station: 204, length: 25 },
    },
  },
};
cases["central-to-lower-bridge"] = {
  ...cases["central-van-ness-entry"],
  owners: [...new Set([...centralOwners, "808f7fd4", ...lowerBridgeOwners])],
  roads: [
    ...cases["central-van-ness-entry"].roads,
    "i80-eastbound-midtown",
    "i80-eastbound-fourth",
    "i80-eastbound-soma",
    "i80-eastbound-lower-climb",
    "i80-eastbound-essex-merge",
    "sf-eastbound-lower-approach",
    "bay-bridge-eastbound-lower",
    "yerba-buena-lower-west-approach",
    "yerba-buena-lower-tunnel",
    "yerba-buena-lower-tunnel-east-of-exit",
    "yerba-buena-lower-east-approach",
    "yerba-buena-lower-east-after-entry",
  ],
  maxDurationMs: 660_000,
  forks: [
    ...cases["central-van-ness-entry"].forks,
    ["yerba-buena-lower-tunnel", "yerba-buena-lower-tunnel-east-of-exit", 1.5],
  ],
  laneGuides: {
    ...cases["central-van-ness-entry"].laneGuides,
    "i80-eastbound-soma": {
      clearance: { offset: 2.4, station: 114, length: 31 },
    },
    "i80-eastbound-essex-merge": {
      clearance: [
        { offset: 2.6, station: 39, length: 25 },
        { offset: -2.3, station: 127, length: 25 },
      ],
    },
  },
};
const macallaMerge = "yerba-buena-upper-after-macalla-merge";
const upperFromIsland = [
  "yerba-buena-east-approach",
  macallaMerge,
  "yerba-buena-upper-tunnel",
  "yerba-buena-upper-tunnel-west-of-merge",
  "yerba-buena-west-approach",
  "bay-bridge-westbound",
  "city-approach",
];
const upperOwners = [
  ...new Set([
    ...centralOwners,
    ...lowerBridgeOwners,
    "8085801c",
    "80858064",
    "8085800c",
    "8085805c",
  ]),
];
cases["upper-to-fremont"] = {
  roads: [...upperFromIsland, "fremont-exit"],
  owners: upperOwners,
  speed: 22,
  rollingStart: true,
  speedLimits: { "fremont-exit": 11.5 },
  forks: [["city-approach", "fremont-exit", -4.5]],
  laneGuides: {
    "fremont-exit": {
      clearance: { offset: 1.6, station: 456, length: 30 },
    },
  },
};
cases["upper-to-octavia"] = {
  ...cases["central-from-fourth"],
  roads: [
    ...upperFromIsland,
    "i80-westbound-fremont-split",
    "i80-westbound-upper-soma",
    "i80-westbound-fourth-merge",
    ...cases["central-from-fourth"].roads.slice(1),
  ],
  owners: upperOwners,
  speed: 22,
  rollingStart: true,
  maxDurationMs: 620_000,
  speedLimits: {
    ...cases["central-from-fourth"].speedLimits,
    "i80-westbound-fourth-merge": 18,
  },
  forks: [
    ["city-approach", "i80-westbound-fremont-split", 1.3],
    ["i80-westbound-fremont-split", "i80-westbound-upper-soma", -1.8],
    ["i80-westbound-upper-soma", "i80-westbound-fourth-merge", -12],
    ...cases["central-from-fourth"].forks,
  ],
  laneGuides: {
    ...cases["central-from-fourth"].laneGuides,
    "i80-westbound-upper-soma": {
      approachMetres: 115,
      clearance: { offset: 2.2, station: 40, length: 28 },
    },
    "i80-westbound-fourth-merge": { entranceMetres: 230 },
  },
};
const harrisonFork = "i80-westbound-fremont-split";
const harrisonLoop = "i80-westbound-harrison-loop";
const somaUpper = "i80-westbound-upper-soma";
cases["westbound-harrison-loop"] = {
  roads: [harrisonFork, harrisonLoop],
  owners: upperOwners,
  speed: 17,
  initialSpeedMps: 16,
  rollingStart: true,
  speedLimits: { [harrisonLoop]: 12 },
  forks: [[harrisonFork, harrisonLoop, 1.65]],
  laneGuides: {
    [harrisonLoop]: {
      clearance: [
        { offset: -2.05, station: 289, length: 24 },
        { offset: -2.15, station: 376, length: 23 },
      ],
    },
  },
};
cases["westbound-harrison-fork-through"] = {
  roads: [harrisonFork, somaUpper],
  owners: upperOwners,
  speed: 17,
  initialSpeedMps: 16,
  rollingStart: true,
  finishOnDeck: true,
  finishDistance: 330,
  forks: [[harrisonFork, somaUpper, -1.8]],
  laneGuides: cases["upper-to-octavia"].laneGuides,
};
cases["upper-to-harrison-loop"] = {
  ...cases["westbound-harrison-loop"],
  roads: [...upperFromIsland, harrisonFork, harrisonLoop],
  speed: 22,
  initialSpeedMps: 50 * 0.44704,
  forks: [
    ["city-approach", harrisonFork, 1.3],
    [harrisonFork, harrisonLoop, 1.65],
  ],
};

cases["dogpatch-to-sixth-northbound"] = {
  roads: [
    "i280-dogpatch-east-north-entry",
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
    "i280-mariposa-northbound-approach",
    sixthForkApproach,
    sixthStreetNorthLead,
    sixthStreetNorthEnd,
  ],
  owners: ["808f7fb4", "808f7fbc", "808f7fcc", "808f7fd4", "808f7fac"],
  speed: 17,
  forks: [sixthStreetNorthFork],
  speedLimits: {
    "i280-dogpatch-east-north-entry": 12,
    "i280-sixth-northbound": 14,
  },
  laneGuides: {
    "i280-dogpatch-east-north-entry": {
      clearance: { offset: -1.6, station: 107, length: 26 },
    },
    "i280-dogpatch-northbound": {
      clearance: { offset: 2.2, station: 89, length: 28 },
    },
    "i280-sixth-northbound": {
      clearance: { offset: -1.8, station: 696, length: 32 },
    },
  },
};
const dogpatchNorth = cases["dogpatch-to-sixth-northbound"];
cases["dogpatch-to-fifth-northbound"] = {
  ...dogpatchNorth,
  roads: [...dogpatchNorth.roads.slice(0, -2), fifthStreetNorthExit],
  forks: [
    ["i280-dogpatch-northbound", "i280-south-of-eighteenth-northbound", 2.2],
    fifthStreetNorthFork,
  ],
};
const alemanyWest = "i280-north-alemany-west-approach";
const alemanyThrough = "i280-north-alemany-main";
const alemanyStayNorth = ["i280-north-alemany-entry", alemanyThrough, 1.6];
const alemanyWestThrough = [alemanyWest, alemanyThrough, 1.6];
const alemanyNorthApproach = [
  "i280-north-alemany-entry",
  "i280-north-alemany-main",
  "i280-north-alemany-climb",
  "i280-north-lower-viaduct",
  "i280-north-south-dogpatch",
];
for (const branch of ["fifth", "sixth"]) {
  const exit = cases[`dogpatch-to-${branch}-northbound`];
  cases[`alemany-to-${branch}-northbound`] = {
    ...exit,
    roads: [...alemanyNorthApproach, ...exit.roads.slice(1)],
    owners: [...exit.owners, "808f7ef4", "808f7efc", "808f7f04", "808f7fa4"],
    speed: 17,
    entranceTimeoutMs: 45_000,
    speedLimits: {
      ...exit.speedLimits,
      "i280-north-alemany-entry": 11,
      "i280-north-alemany-climb": 15,
      "i280-north-lower-viaduct": 14,
    },
    forks: [
      ...exit.forks,
      alemanyStayNorth,
      ["i280-north-alemany-main", "i280-north-alemany-climb", -1.5],
      ["i280-north-alemany-climb", "i280-north-lower-viaduct", -1.35],
      ["i280-north-lower-viaduct", "i280-north-south-dogpatch", 1.55],
    ],
    laneGuides: {
      ...exit.laneGuides,
      "i280-north-alemany-entry": {
        clearance: { offset: 1.75, station: 51, length: 24 },
      },
      "i280-north-alemany-climb": {
        clearance: [
          { offset: 2.25, station: 71, length: 24 },
          { offset: -1.8, station: 116, length: 23 },
        ],
      },
      "i280-north-lower-viaduct": {
        clearance: [
          { offset: -1.8, station: 59, length: 18 },
          { offset: 1.9, station: 154, length: 39 },
          { offset: 1.9, station: 621, length: 29 },
        ],
      },
    },
  };
}
const alemanyToSixth = cases["alemany-to-sixth-northbound"];
cases["alemany-to-cesar-chavez-northbound"] = {
  ...alemanyToSixth,
  roads: [...alemanyNorthApproach.slice(0, 4), "i280-north-cesar-chavez-exit"],
  speedLimits: {
    ...alemanyToSixth.speedLimits,
    "i280-north-cesar-chavez-exit": 11,
  },
  forks: [
    alemanyStayNorth,
    ["i280-north-alemany-main", "i280-north-alemany-climb", -1.5],
    ["i280-north-alemany-climb", "i280-north-lower-viaduct", -1.35],
    ["i280-north-lower-viaduct", "i280-north-cesar-chavez-exit", -1.4],
  ],
};
const cesarChavezExit = cases["alemany-to-cesar-chavez-northbound"];
cases["cesar-chavez-pennsylvania-roundtrip"] = {
  ...cesarChavezExit,
  roads: [
    ...cesarChavezExit.roads,
    "i280-dogpatch-pennsylvania-on-south",
    "i280-dogpatch-on-ramp",
    "i280-south-beyond-dogpatch",
  ],
  owners: [...cesarChavezExit.owners, "808f7fac", "808f7fa4"],
  speedLimits: {
    ...cesarChavezExit.speedLimits,
    "i280-dogpatch-pennsylvania-on-south": 9,
    "i280-dogpatch-on-ramp": 12,
    "i280-south-beyond-dogpatch": 15,
  },
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/i280-north-cesar-chavez-exit",
    via: [
      [4160.24, -1189.39],
      [4189.45, -1157.43],
      [4207.36, -1126.84],
      [4206.94, -1125.12],
      [4196.71, -1106.02],
    ],
  },
  finishOnDeck: true,
  finishDistance: 120,
};
for (const direction of ["exit", "through"]) {
  const branch = `us101-north-bayshore-${direction}`;
  cases[`alemany-to-us101-${direction}`] = {
    roads: [
      "i280-north-alemany-entry",
      "i280-north-alemany-main",
      "i280-to-101-north-connector",
      "us101-north-silver-approach",
      branch,
    ],
    owners: ["808f7ef4", "808f7efc", "808f7e54", "808f7e4c"],
    speed: 17,
    finishOnDeck: direction === "through",
    speedLimits: {
      "i280-north-alemany-entry": 11,
      "i280-to-101-north-connector": 13,
      "us101-north-bayshore-exit": 11,
    },
    entranceTimeoutMs: 45_000,
    forks: [
      alemanyStayNorth,
      ["i280-north-alemany-main", "i280-to-101-north-connector", 1.5],
      ["us101-north-silver-approach", branch, direction === "exit" ? -2 : 1.55],
    ],
    laneGuides: {
      "i280-north-alemany-entry": {
        clearance: { offset: 1.75, station: 51, length: 24 },
      },
      "i280-to-101-north-connector": {
        clearance: [
          { offset: 1.95, station: 248, length: 27 },
          { offset: 1.7, station: 820, length: 23 },
        ],
      },
      "us101-north-silver-approach": {
        clearance: { offset: -1.55, station: 490, length: 28 },
      },
      "us101-north-bayshore-exit": {
        entranceMetres: 40,
        clearance: { offset: 1.6, station: 29, length: 15 },
      },
      "us101-north-bayshore-through": {
        clearance: [
          { offset: 1.9, station: 128, length: 28 },
          { offset: -1.15, station: 851, length: 25 },
        ],
      },
    },
  };
}
const us101Through = cases["alemany-to-us101-through"];
const bayEast = cases["central-to-lower-bridge"];
const bayRoads = bayEast.roads.slice(
  bayEast.roads.indexOf("i80-eastbound-interchange"),
);
const us101Potrero = {
  ...us101Through,
  roads: [...us101Through.roads, "us101-north-potrero-approach"],
  owners: [...us101Through.owners, "808f7e34", "808f7e2c"],
  speedLimits: {
    ...us101Through.speedLimits,
    "us101-north-potrero-approach": 15,
  },
  laneGuides: {
    ...us101Through.laneGuides,
    "us101-north-potrero-approach": {
      clearance: [
        { offset: 2.15, station: 183, length: 40 },
        { offset: 1.9, station: 372, length: 31 },
        { offset: -1.25, station: 458, length: 25 },
        { offset: -1.55, station: 842, length: 24 },
        { offset: 1.95, station: 886, length: 32 },
        { offset: -1.55, station: 992, length: 29 },
      ],
    },
  },
};
cases["alemany-to-potrero-exit"] = {
  ...us101Potrero,
  roads: [...us101Potrero.roads, "us101-north-potrero-exit"],
  finishOnDeck: false,
  speedLimits: { ...us101Potrero.speedLimits, "us101-north-potrero-exit": 11 },
  laneGuides: {
    ...us101Potrero.laneGuides,
    "us101-north-potrero-exit": {
      clearance: [
        { offset: 1.7, station: 101, length: 25 },
        { offset: 1.55, station: 152, length: 19 },
      ],
    },
  },
  forks: [
    ...us101Potrero.forks,
    ["us101-north-potrero-approach", "us101-north-potrero-exit", -1.5],
  ],
};
cases["alemany-to-i80-lower-bay"] = {
  ...us101Potrero,
  roads: [
    ...us101Potrero.roads,
    "us101-north-i80-approach",
    "us101-north-i80-east-connector",
    "i80-eastbound-us101-approach",
    ...bayRoads,
  ],
  owners: [...new Set([...us101Potrero.owners, ...bayEast.owners])],
  speed: 19,
  speedLimits: {
    ...us101Potrero.speedLimits,
    "us101-north-i80-east-connector": 15,
  },
  laneGuides: { ...us101Potrero.laneGuides, ...bayEast.laneGuides },
  finishOnDeck: true,
  entranceTimeoutMs: 50_000,
  maxDurationMs: 880_000,
  forks: [
    ...us101Potrero.forks,
    ["us101-north-potrero-approach", "us101-north-i80-approach", 1.4],
  ],
};
const centralViaUS101 = {
  owners: [...new Set([...us101Potrero.owners, ...centralOwners])],
  speed: 18,
  rollingStart: true,
  initialSpeedMps: 16,
  speedLimits: {
    "us101-north-central-viaduct": 15,
    "us101-north-ninth-bryant-exit": 10,
    "us101-north-central-merge": 15,
    "central-freeway-octavia-exit": 13,
  },
};
const centralViaduct = [
  "us101-north-i80-approach",
  "us101-north-central-viaduct",
];
const us101ToCentral = [...centralViaduct, "us101-north-central-merge"];
const centralFork = [centralViaduct[0], centralViaduct[1], 1.55];
cases["us101-to-ninth-bryant"] = {
  ...centralViaUS101,
  roads: [...centralViaduct, "us101-north-ninth-bryant-exit"],
  forks: [
    centralFork,
    [centralViaduct[1], "us101-north-ninth-bryant-exit", -1.6],
  ],
};
cases["us101-to-octavia"] = {
  ...centralViaUS101,
  roads: [
    ...us101ToCentral,
    "central-freeway-east-northbound",
    "central-freeway-octavia-exit",
  ],
  forks: [centralFork, [centralViaduct[1], "us101-north-central-merge", 1.6]],
};
const somaRampOwners = ["8085807c", "80858084", "808f7fd4", "808f7e2c"];
cases["i80-fourth-bryant-exit"] = {
  owners: somaRampOwners,
  speed: 16,
  rollingStart: true,
  initialSpeedMps: 16,
  roads: ["i80-eastbound-fourth", "i80-eastbound-fourth-bryant-exit"],
  speedLimits: { "i80-eastbound-fourth-bryant-exit": 11 },
  forks: [["i80-eastbound-fourth", "i80-eastbound-fourth-bryant-exit", -1.7]],
};
for (const [name, ramp, deck] of [
  ["i80-bryant-entry", "i80-eastbound-bryant-entry", "i80-eastbound-fourth"],
  [
    "i80-fifth-bryant-entry",
    "i80-eastbound-fifth-bryant-entry",
    "i80-eastbound-lower-climb",
  ],
  [
    "i80-harrison-entry",
    "i80-westbound-harrison-entry",
    "i80-westbound-interchange",
  ],
]) {
  cases[name] = {
    owners: somaRampOwners,
    speed: 14,
    finishOnDeck: true,
    roads: [ramp, deck],
    speedLimits: { [ramp]: 11 },
  };
}
const fifthBryantEntry = "i80-eastbound-fifth-bryant-entry";
cases["i80-fifth-bryant-divider-lane"] = {
  ...cases["i80-fifth-bryant-entry"],
  finishDistance: 125,
  laneGuides: {
    [fifthBryantEntry]: {
      clearance: { offset: 2.35, station: 251, length: 114 },
    },
  },
  maxAirborneMetres: 0,
  expectNoScrapes: true,
};
for (const reverse of [false, true]) {
  cases[
    reverse ? "dogpatch-twentieth-eastbound" : "dogpatch-twentieth-westbound"
  ] = {
    roads: ["dogpatch-twentieth-viaduct"],
    owners: ["808f7fb4"],
    speed: 12,
    reverse,
  };
}
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
    finishOnDeck: true,
    speedLimits: { [entry]: 9, "i280-dogpatch-on-ramp": 13 },
    laneGuides: { "i280-south-beyond-dogpatch": dogpatchClearance },
  };
}
cases["dogpatch-pennsylvania-to-i280"] = {
  ...cases["i280-dogpatch-enter-north"],
  finishDistance: 90,
  groundApproachMetres: 0,
  groundArrival: [
    [4174.2, -850],
    [4175.8, -882],
    [4174.8, -891.1],
  ],
  groundedHandoff: true,
  expectNoScrapes: true,
  maxDurationMs: 85_000,
};
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
  const direction = reverse ? "southbound" : "northbound";
  const approach = `sr1-south-of-macarthur-${direction}`;
  const owners = ["8085871c", "80858724", "808586e4"];
  cases[approach] = {
    owners,
    roads: [approach],
    groundApproachMetres: 9,
    speed: 13,
    maxAirborneMetres: 2,
  };
  const roads = reverse ? [macarthur, approach] : [approach, macarthur];
  cases[`macarthur-and-south-crossing-${direction}`] = {
    owners,
    roads,
    speed: 13,
    groundApproachMetres: 9,
    maxAirborneMetres: 5,
    groundedHandoff: true,
    connectByGround: true,
    reverseRoads: reverse ? [macarthur] : [],
    laneGuides: { [macarthur]: { lane: reverse ? 4.2 : -4.2 } },
    groundConnection: [
      {
        after: prefix + roads[0],
        via: [],
      },
    ],
  };
}
for (const reverse of [false, true]) {
  const direction = reverse ? "northeast" : "southwest";
  cases[`san-jose-bosworth-${direction}`] = {
    roads: ["san-jose-bosworth-upper-street"],
    owners: ["808f7e64", "808f7e8c"],
    reverse,
    speed: 12,
    groundApproachMetres: 7,
    maxDurationMs: 85_000,
  };
}
for (const entrance of [
  "bosworth-southbound-entry",
  "san-jose-southbound-entry",
]) {
  cases[entrance] = {
    owners: ["808f7e64", "808f7e7c", "808f7e8c", "808f7e84"],
    roads: [
      entrance,
      "bosworth-i280-southbound-merge",
      "i280-south-monterey-to-ocean",
    ],
    speed: 17,
    speedLimits: { [entrance]: 13 },
    groundApproachMetres: 7,
    finishOnDeck: true,
    finishDistance: 85,
    covered: [prefix + entrance, prefix + "bosworth-i280-southbound-merge"],
    maxDurationMs: 125_000,
  };
}
const sanJoseShared = "san-jose-avenue-shared-deck";
const sanJoseExit = "branch-east-san-jose-viaduct";
const montereyApproach = "monterey-san-jose-approach";
const sanJoseOwners = ["808f7e64", "808f7e7c", "808f7e8c"];
cases["branch-east-ramp"] = {
  roads: ["branch-east-off-ramp", sanJoseExit, sanJoseShared],
  owners: sanJoseOwners,
  speed: 13,
  cameraStations: [
    { x: 397, y: -3221, surface: "branch-east-off-ramp", camera: "exterior" },
    { x: 495.7, y: -3144.2, surface: "branch-east-off-ramp", camera: "tunnel" },
  ],
  groundedHandoff: true,
  maxAirborneMetresAfterEntrance: 2,
  maxDurationMs: 115_000,
};
cases["branch-east-upper"] = {
  roads: ["branch-east-i280-northbound", "i280-north-monterey-merge"],
  owners: ["808f7e7c", "808f7e64"],
  finishOnDeck: true,
  finishDistance: 80,
  speed: 18,
};
cases["branch-east-san-jose"] = {
  roads: [sanJoseExit, sanJoseShared],
  owners: sanJoseOwners,
  speed: 13,
  rollingStart: true,
  initialSpeedMps: 13,
};
for (const reverse of [false, true]) {
  const roads = [montereyApproach, sanJoseShared];
  cases[reverse ? "san-jose-to-monterey" : "monterey-to-san-jose"] = {
    roads: reverse ? roads.toReversed() : roads,
    owners: sanJoseOwners,
    reverse,
    speed: 11.5,
    groundApproachMetres: reverse ? 5 : 9,
    maxAirborneMetres: 2,
  };
}
for (const direction of ["northbound", "southbound"]) {
  const northbound = direction === "northbound";
  const tunnel = "macarthur-tunnel";
  const deck = `kobbe-viaduct-${direction}`;
  const kobbe = {
    roads: northbound ? [tunnel, deck] : [deck, tunnel],
    owners: ["808586e4", "80858724", "8085871c"],
    speed: 15,
    connectByGround: true,
    reverseRoads: northbound ? [] : [tunnel],
    laneGuides: { [tunnel]: { lane: northbound ? -4.2 : 4.2 } },
  };
  const ruckman = `ruckman-viaduct-${direction}`;
  cases[`macarthur-kobbe-${direction}`] = kobbe;
  cases[`macarthur-ruckman-${direction}`] = {
    ...kobbe,
    roads: northbound
      ? [...kobbe.roads, ruckman, "sr1-north-to-golden-gate-approach"]
      : [ruckman, ...kobbe.roads],
    forks: northbound
      ? [[ruckman, "sr1-north-to-golden-gate-approach", 2.1]]
      : [],

    owners: [...kobbe.owners, "808586dc"],
  };
}

const seventhStart = [
  "i80-eastbound-interchange",
  "i80-eastbound-seventh-off-ramp",
];
const seventhFork = [seventhStart[0], seventhStart[1], -1.6];
const seventhDrive = {
  owners: ["808f7e2c", "80858084"],
  speed: 13,
  rollingStart: true,
  initialSpeedMps: 13,
  laneGuides: {
    "i80-eastbound-seventh-off-ramp": {
      clearance: { offset: -1.8, station: 24, length: 20 },
    },
  },
  speedLimits: {
    "i80-eastbound-seventh-off-ramp": 10,
    "i80-eastbound-seventh-west-exit": 9,
    "i80-eastbound-seventh-east-exit": 9,
  },
};
for (const [direction, offset] of [
  ["west", 1.55],
  ["east", -1.55],
]) {
  const exit = `i80-eastbound-seventh-${direction}-exit`;
  cases[`i80-seventh-${direction}-exit`] = {
    ...seventhDrive,
    roads: [...seventhStart, exit],
    forks: [seventhFork, [seventhStart[1], exit, offset]],
  };
}
cases["i80-seventh-harrison-roundtrip"] = {
  ...seventhDrive,
  roads: [
    ...seventhStart,
    "i80-eastbound-seventh-west-exit",
    "i80-westbound-harrison-entry",
    "i80-westbound-interchange",
  ],
  forks: [
    seventhFork,
    [seventhStart[1], "i80-eastbound-seventh-west-exit", 1.55],
  ],
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/i80-eastbound-seventh-west-exit",
    via: [
      [3043, 1654],
      [3036, 1663],
      [3004, 1693],
      [2977.5, 1717.3],
      [2970.0, 1722.4],
      [2966.0, 1719.2],
    ],
  },
  finishOnDeck: true,
};
const harrisonSouth = {
  owners: ["808f7e2c", "808f7e34", "808f7e4c"],
  roads: [
    "i80-westbound-harrison-entry",
    "i80-westbound-interchange",
    "i80-westbound-to-us101-south",
    "us101-south-potrero-main",
    "us101-south-potrero-exit",
    "us101-south-cesar-potrero-approach",
  ],
  speed: 17,
  laneGuides: {
    "us101-south-potrero-main": {
      clearance: [
        { offset: 1.65, station: 455, length: 26 },
        { offset: -1.6, station: 818, length: 23 },
        { offset: 1.6, station: 840, length: 29 },
        { offset: -1.4, station: 884, length: 24 },
        { offset: 1.6, station: 951, length: 24 },
        { offset: 1.35, station: 1356, length: 20 },
        { offset: 1.2, station: 1478, length: 42 },
      ],
    },
    "us101-south-potrero-exit": {
      clearance: { offset: 1.25, station: 14, length: 14 },
    },
  },
  speedLimits: {
    "i80-westbound-harrison-entry": 11,
    "i80-westbound-to-us101-south": 14,
    "us101-south-potrero-exit": 11,
    "us101-south-cesar-potrero-approach": 11,
  },
  forks: [["i80-westbound-interchange", "i80-westbound-to-us101-south", 1.55]],
};
for (const [destination, offset] of [
  ["cesar-chavez", 1.6],
  ["potrero-avenue", -1.6],
]) {
  const exit = `us101-south-${destination}-exit`;
  cases[`harrison-to-${destination}`] = {
    ...harrisonSouth,
    roads: [...harrisonSouth.roads, exit],
    forks: [
      ...harrisonSouth.forks,
      ["us101-south-cesar-potrero-approach", exit, offset],
    ],
    speedLimits: { ...harrisonSouth.speedLimits, [exit]: 10 },
    laneGuides:
      destination === "potrero-avenue"
        ? {
            ...harrisonSouth.laneGuides,
            "us101-south-cesar-potrero-approach": {
              clearance: { offset: 2.05, station: 162, length: 15 },
            },
          }
        : harrisonSouth.laneGuides,
  };
}

const centralSouthViaduct = "us101-south-central-viaduct";
const centralSouthMerge = "us101-south-central-merge";
const tenthBryant = "us101-south-tenth-bryant-entry";
for (const [name, approach, destination, approachForks] of [
  [
    "octavia-to-cesar-chavez",
    ["central-freeway-octavia-entry", centralEast[0], centralSouthViaduct],
    "cesar-chavez",
    [[centralEast[0], centralSouthViaduct, -1.6]],
  ],
  ["tenth-bryant-to-potrero-avenue", [tenthBryant], "potrero-avenue", []],
]) {
  const exit = cases[`harrison-to-${destination}`];
  cases[name] = {
    ...exit,
    roads: [...approach, centralSouthMerge, ...exit.roads.slice(3)],
    owners: [...new Set([...exit.owners, ...centralOwners])],
    speed: 16,
    forks: [
      ...approachForks,
      ["us101-south-potrero-main", "us101-south-potrero-exit", -1.6],
      ...exit.forks.slice(1),
    ],
    speedLimits: { ...exit.speedLimits, [tenthBryant]: 11 },
    laneGuides: {
      ...exit.laneGuides,
      ...cases["central-octavia-entry"].laneGuides,
      [tenthBryant]: {
        clearance: { offset: -2.15, station: 145, length: 36 },
      },
    },
  };
}

const us101SouthSpine = [
  "us101-south-cesar-through",
  "us101-south-silver-through",
  "us101-south-to-i280-south-approach",
];
const alemanyLaneGuides = {
  "us101-south-cesar-through": {
    clearance: { offset: -1.3, station: 492, length: 24 },
  },
  "us101-south-silver-through": {
    clearance: [
      { offset: 2.35, station: 95, length: 28 },
      { offset: -1.4, station: 148, length: 22 },
      { offset: -1.85, station: 230, length: 24 },
      { offset: 1.4, station: 646, length: 23 },
    ],
  },
  "us101-south-to-i280-south-approach": {
    clearance: { offset: -1.5, station: 14, length: 14 },
  },
  "i280-south-alemany-entry": {
    clearance: { offset: -2.05, station: 174, length: 37 },
  },
  "i280-south-beyond-alemany": {
    clearance: [
      { offset: 3.5, station: 32, length: 27 },
      { offset: 1.3, station: 75, length: 26 },
    ],
  },
};
const us101Alemany = {
  laneGuides: alemanyLaneGuides,
  owners: ["808f7e4c", "808f7e54", "808f7efc", "808f7ef4", "808f7e64"],
  rollingStart: true,
  initialSpeedMps: 18,
  speed: 18,
  maxDurationMs: 540_000,
};
cases["us101-south-to-i280-through-alemany"] = {
  ...us101Alemany,
  roads: [
    ...us101SouthSpine,
    "us101-south-to-i280-south-merge",
    "i280-south-alemany-through",
    "i280-south-alemany-main",
    "i280-south-beyond-alemany",
  ],
  finishOnDeck: true,
  speedLimits: {
    "us101-south-to-i280-south-approach": 14,
    "us101-south-to-i280-south-merge": 14,
  },
  forks: [
    [us101SouthSpine.at(-1), "us101-south-to-i280-south-merge", 1.6],
    ["i280-south-alemany-through", "i280-south-alemany-main", 1.55],
  ],
};
cases["us101-alemany-i280-roundtrip"] = {
  ...us101Alemany,
  roads: [
    ...us101SouthSpine,
    "us101-south-alemany-east-exit",
    "i280-south-alemany-entry",
    "i280-south-beyond-alemany",
  ],
  finishOnDeck: true,
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/us101-south-alemany-east-exit",
    via: [
      [2563.9, -2803.64],
      [2307.73, -2965.33],
      [2239.27, -3008.58],
      [2158.34, -3047.89],
      [2068.91, -3072.63],
      [1988.66, -3083.75],
    ],
  },
  speedLimits: {
    "us101-south-to-i280-south-approach": 14,
    "us101-south-alemany-east-exit": 10,
    "i280-south-alemany-entry": 15,
  },
  forks: [[us101SouthSpine.at(-1), "us101-south-alemany-east-exit", -1.6]],
};
cases["i280-south-ellsworth-exit"] = {
  ...us101Alemany,
  roads: [
    "i280-south-alemany-merge-tail",
    "i280-south-alemany-through",
    "i280-south-alemany-exit",
  ],
  speed: 16,
  speedLimits: { "i280-south-alemany-exit": 11 },
  forks: [["i280-south-alemany-through", "i280-south-alemany-exit", -1.6]],
};

const montereyTail = "i280-south-beyond-alemany";
const montereyExit = "i280-south-monterey-exit";
const montereyEntry = "i280-north-monterey-entry";
const montereyOwners = [
  "808f7c2c",
  "808f7dd4",
  "808f7e64",
  "808f7e7c",
  "808f7e84",
  "808f7ef4",
  "808f7efc",
];
const montereyReturn = [
  montereyEntry,
  "i280-north-monterey-to-alemany",
  "i280-north-alemany-west-approach",
  "i280-north-alemany-main",
];
const montereyRolling = {
  owners: montereyOwners,
  speed: 18,
  rollingStart: true,
  initialSpeedMps: 16,
  rollingStartMetres: 110,
  laneGuides: {
    [montereyTail]: alemanyLaneGuides[montereyTail],
    [montereyExit]: { clearance: { offset: -1.65, station: 56, length: 42 } },
    "i280-south-monterey-mainline": {
      clearance: { offset: 1.45, station: 721, length: 28 },
    },
    "i280-south-monterey-to-ocean": {
      clearance: [
        { offset: 1.4, station: 375, length: 29 },
        { offset: 1.65, station: 470, length: 30 },
        { offset: 2.3, station: 628, length: 28 },
      ],
    },
  },
  speedLimits: {
    [montereyExit]: 12,
    [montereyEntry]: 12,
    "i280-south-ocean-approach": 12,
    "i280-south-ocean-exit": 11,
  },
};
cases["i280-monterey-exit"] = {
  ...montereyRolling,
  roads: [montereyTail, montereyExit],
  forks: [[montereyTail, montereyExit, -1.8]],
};
cases["i280-monterey-entry"] = {
  ...montereyRolling,
  rollingStart: false,
  roads: montereyReturn,
  finishOnDeck: true,
  forks: [alemanyWestThrough],
};
cases["i280-ocean-exit"] = {
  ...montereyRolling,
  roads: [
    montereyTail,
    "i280-south-monterey-mainline",
    "i280-south-monterey-to-ocean",
    "i280-south-ocean-approach",
    "i280-south-ocean-exit",
  ],
  forks: [[montereyTail, "i280-south-monterey-mainline", 1.7]],
};

cases["i280-monterey-roundtrip"] = {
  ...cases["i280-monterey-exit"],
  roads: [montereyTail, montereyExit, ...montereyReturn],
  forks: [...cases["i280-monterey-exit"].forks, alemanyWestThrough],
  connectByGround: true,
  finishOnDeck: true,
  laneGuides: {
    ...montereyRolling.laneGuides,
    [montereyExit]: {
      ...montereyRolling.laneGuides[montereyExit],
      approach: 2.9,
      holdApproach: true,
    },
  },
  groundConnection: {
    after: "sf-contact/" + montereyExit,
    minimumMetres: 130,
    via: [
      [282, -3160],
      [264, -3160],
      [244, -3160],
      [229, -3160],
      [222, -3156],
      [223, -3151],
      [231, -3147.7],
      [253, -3147.4],
      [269, -3150],
      [276, -3156.5],
      [285, -3161.5],
      [289, -3162.8],
    ],
  },
};

const mariposaMain = "i280-dogpatch-northbound";
const mariposaExit = "i280-north-mariposa-exit";
cases["i280-mariposa-exit"] = {
  ...dogpatchNorth,
  roads: [...dogpatchNorth.roads.slice(0, 2), mariposaExit],
  forks: [[mariposaMain, mariposaExit, -2.2]],
  speedLimits: { ...dogpatchNorth.speedLimits, [mariposaExit]: 12 },
};
cases["dogpatch-to-sixth-northbound"] = {
  ...dogpatchNorth,
  forks: [
    [mariposaMain, "i280-south-of-eighteenth-northbound", 2.2],
    ...dogpatchNorth.forks,
  ],
};

const silverBayshore = "us101-south-silver-through";
const bayshoreFromSilver = "us101-south-silver-to-industrial";
const bayshoreToSilliman = "us101-south-industrial-to-silliman";
const sillimanOff = "us101-south-silliman-exit";
const sillimanOn = "us101-south-silliman-entry";
const industrialOn = "us101-south-industrial-entry";
const paulApproach = "us101-south-paul-approach";
const sanBrunoOff = "us101-south-san-bruno-exit";
const paulThrough = "us101-south-paul-mainline";
const sillimanThrough = "us101-south-silliman-mainline";
const bayshoreApproach = [
  silverBayshore,
  bayshoreFromSilver,
  bayshoreToSilliman,
];
const bayshoreSouth = ["us101-south-silliman-to-paul", paulApproach];
const bayshoreOwners = [
  ...new Set([...us101Alemany.owners, "808f7ee4", "808f7f24"]),
];
const bayshore = {
  speed: 18,
  owners: bayshoreOwners,
  laneGuides: {
    ...alemanyLaneGuides,
    [bayshoreFromSilver]: {
      clearance: [
        { offset: 2.8, station: 17, length: 17 },
        { offset: -4.85, station: 149, length: 48 },
        { offset: 1.4, station: 290, length: 25 },
        { offset: 1.5, station: 449, length: 24 },
      ],
    },
    [industrialOn]: {
      clearance: [
        { offset: 1, station: 23, length: 20 },
        { offset: -0.9, station: 338, length: 18 },
      ],
    },
    [sillimanOff]: {
      clearance: [
        { offset: -1.8, station: 50, length: 23 },
        { offset: 0.7, station: 96, length: 18 },
      ],
    },
    [paulThrough]: {
      clearance: [
        { offset: -1.4, station: 165, length: 24 },
        { offset: 1.4, station: 244, length: 24 },
      ],
    },
  },
  speedLimits: {
    [industrialOn]: 12,
    [sillimanOff]: 12,
    [sillimanOn]: 12,
    [sanBrunoOff]: 12,
    [bayshoreFromSilver]: 15,
  },
};
cases["silver-to-silliman"] = {
  ...bayshore,
  roads: [...bayshoreApproach, sillimanOff],
  rollingStart: true,
  initialSpeedMps: 16,
  forks: [
    [silverBayshore, bayshoreFromSilver, 2.2],
    [bayshoreToSilliman, sillimanOff, -2],
  ],
};
cases["industrial-to-silliman"] = {
  ...bayshore,
  roads: [industrialOn, bayshoreToSilliman, sillimanOff],
  groundApproachMetres: 3.5,
  forks: [[bayshoreToSilliman, sillimanOff, -2]],
};
cases["silliman-to-san-bruno"] = {
  ...bayshore,
  roads: [sillimanOn, ...bayshoreSouth, sanBrunoOff],
  forks: [[paulApproach, sanBrunoOff, -2]],
};
cases["silver-through-san-bruno"] = {
  ...cases["silver-to-silliman"],
  owners: [...bayshoreOwners, "808f7f2c"],
  roads: [...bayshoreApproach, sillimanThrough, ...bayshoreSouth, paulThrough],
  finishOnDeck: true,
  finishDistance: 240,
  forks: [
    [silverBayshore, bayshoreFromSilver, 2.2],
    [bayshoreToSilliman, sillimanThrough, 2],
    [paulApproach, paulThrough, 2],
  ],
};
for (const name of [
  "us101-south-to-i280-through-alemany",
  "us101-alemany-i280-roundtrip",
]) {
  const route = cases[name];
  route.owners = bayshoreOwners;
  route.forks = [
    [silverBayshore, us101SouthSpine.at(-1), -2.2],
    ...route.forks,
  ];
}

cases["silliman-roundtrip"] = {
  ...cases["silver-to-silliman"],
  roads: [
    ...bayshoreApproach,
    sillimanOff,
    sillimanOn,
    ...bayshoreSouth,
    sanBrunoOff,
  ],
  forks: [
    ...cases["silver-to-silliman"].forks,
    [paulApproach, sanBrunoOff, -2],
  ],
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/" + sillimanOff,
    minimumMetres: 430,
    via: [
      [3075.9, -3194.9],
      [3068, -3195.5],
      [3001.4, -3216.8],
      [2995.7, -3217.6],
      [2991.6, -3215.5],
      [2988.8, -3210.7],
      [2986.8, -3204.4],
      [2950.7, -3102.9],
      [2949.8, -3097.5],
      [2951.8, -3092.5],
      [2956, -3087.7],
      [2960, -3086.2],
      [3020, -3062.8],
      [3026, -3061.5],
      [3030.4, -3064.1],
      [3034, -3071],
      [3073, -3177.4],
      [3075, -3183],
      [3076, -3187],
      [3076.6, -3189.5],
      [3077.7, -3191.0],
    ],
  },
};

const sr1Kobbe = "kobbe-viaduct-northbound";
const ruckmanNorth = "ruckman-viaduct-northbound";
const sr1City = "sr1-north-to-presidio-southbound";
const sr1Bridge = "sr1-north-to-golden-gate-approach";
const sr1Start = {
  roads: [sr1Kobbe, ruckmanNorth, sr1Bridge],
  owners: [...new Set([...owners, "808586e4"])],
  rollingStart: true,
  initialSpeedMps: 16,
  speed: 18,
  speedLimits: { [sr1City]: 12, [sr1Bridge]: 12, [ruckmanNorth]: 15 },
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/" + sr1Kobbe,
    minimumMetres: 47,
    via: [
      [-2590.83, 4396.3],
      [-2590.2, 4423.0],
    ],
  },
};
cases["sr1-kobbe-to-golden-gate"] = {
  ...sr1Start,
  forks: [[ruckmanNorth, sr1Bridge, 2.1]],
};
cases["sr1-kobbe-to-presidio"] = {
  ...sr1Start,
  roads: [sr1Kobbe, ruckmanNorth, sr1City, ...cases.southbound.roads],
  forks: [[ruckmanNorth, sr1City, -0.75]],
};

const us101CityJoin = "us101-south-city-entrance-merge";
const potreroCity = "us101-south-potrero-avenue-approach";
const chavezCity = "us101-south-cesar-chavez-entry";
const cityToSilver = {
  owners: ["808f7e4c", "808f7e54"],
  speed: 17,
  finishOnDeck: true,
  finishDistance: 190,
  laneGuides: alemanyLaneGuides,
  speedLimits: {
    [potreroCity]: 12,
    [chavezCity]: 12,
    [us101CityJoin]: 14,
    "us101-south-potrero-entry": 12,
  },
};
cases["potrero-avenue-to-us101-south"] = {
  ...cityToSilver,
  roads: [
    potreroCity,
    "us101-south-potrero-entry",
    us101CityJoin,
    silverBayshore,
  ],
};
cases["cesar-chavez-street-to-us101-south"] = {
  ...cityToSilver,
  roads: [chavezCity, us101CityJoin, silverBayshore],
  groundApproachMetres: 3.5,
  laneGuides: {
    ...alemanyLaneGuides,
    [chavezCity]: { clearance: { offset: 1.25, station: 39, length: 21 } },
  },
};

const hesterPaul = "us101-south-paul-mainline";
const hesterMain = "us101-south-hester-mainline";
const hesterCollector = "us101-south-hester-collector-entry";
const hesterReturn = "us101-south-hester-collector-return";
const hesterRoad = "us101-south-hester-bayshore-exit";
const hesterEdge = "us101-south-hester-to-city-edge";
const candlestickThrough = "us101-south-past-candlestick-exit";
const candlestickExit = "us101-south-candlestick-exit";
const hesterStart = {
  owners: ["808f7ee4", "808f7f24", "808f7f2c"],
  speed: 18,
  rollingStart: true,
  initialSpeedMps: 16,
  rollingStartMetres: 310,
  speedLimits: { [hesterCollector]: 12, [hesterRoad]: 12, [hesterReturn]: 12 },
  laneGuides: {
    [hesterMain]: { clearance: { offset: 1.85, station: 592, length: 22 } },
    [hesterRoad]: { clearance: { offset: 1.7, station: 24, length: 22 } },
    [candlestickThrough]: {
      clearance: [
        { offset: 1.35, station: 17.5, length: 17.5 },
        { offset: -1.75, station: 108.4, length: 35 },
      ],
    },
  },
};
cases["us101-south-hester-exit"] = {
  ...hesterStart,
  roads: [hesterPaul, hesterCollector, hesterRoad],
  forks: [
    [hesterPaul, hesterCollector, -1.8],
    [hesterCollector, hesterRoad, -1.6],
  ],
};
cases["us101-south-hester-mainline"] = {
  ...hesterStart,
  roads: [hesterPaul, hesterMain, hesterEdge, candlestickThrough],
  forks: [
    [hesterPaul, hesterMain, 1.9],
    [hesterEdge, candlestickThrough, 1.35],
  ],
};
cases["us101-south-hester-collector"] = {
  ...hesterStart,
  roads: [
    hesterPaul,
    hesterCollector,
    hesterReturn,
    hesterEdge,
    candlestickThrough,
  ],
  forks: [
    [hesterPaul, hesterCollector, -1.8],
    [hesterCollector, hesterReturn, 1.65],
    [hesterEdge, candlestickThrough, 1.35],
  ],
};
cases["candlestick-southbound-exit"] = {
  roads: [hesterEdge, candlestickExit],
  owners: ["808f7f24", "808f7f2c"],
  forks: [[hesterEdge, candlestickExit, -3.7]],
  rollingStart: true,
  rollingStartMetres: 575,
  initialSpeedMps: 15,
  speed: 15,
  speedLimits: { [candlestickExit]: 10 },
  connectByGround: true,
  groundDeparture: [
    [3873.755, -5877.573, 6.197],
    [3868.95, -5878.956, 6.125],
    [3864.145, -5880.339, 6.048],
    [3859.333, -5881.697, 5.97],
  ],
  maxDepartureClearanceMetres: 0.15,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  maxDurationMs: 95_000,
};
cases["candlestick-southbound-through"] = {
  roads: [hesterEdge, candlestickThrough],
  owners: ["808f7f24", "808f7f2c"],
  forks: [[hesterEdge, candlestickThrough, 1.35]],
  laneGuides: hesterStart.laneGuides,
  rollingStart: true,
  rollingStartMetres: 575,
  initialSpeedMps: 16,
  speed: 16,
  finishOnDeck: true,
  finishDistance: 172,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  maxDurationMs: 55_000,
};
for (const reverse of [false, true]) {
  const road = "alana-way-under-us101";
  const direction = reverse ? "reverse" : "forward";
  cases[`alana-way-below-candlestick-${direction}`] = {
    roads: [road],
    owners: ["808f7f2c"],
    reverse,
    speed: 8,
    groundApproachMetres: 7,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [3901.339, -5693.336, 6.391],
          [3896.491, -5694.561, 6.424],
          [3891.644, -5695.786, 6.512],
          [3886.796, -5697.01, 6.727],
        ]
      : [
          [4015.134, -5671.009, 5.048],
          [4020.051, -5670.102, 4.976],
          [4024.968, -5669.195, 4.893],
          [4029.885, -5668.287, 4.879],
          [4033.819, -5667.561, 4.898],
        ],
    covered: [prefix + road],
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}

cases["us101-south-bayshore-north-turn"] = {
  owners: ["808f7e4c"],
  speed: 14,
  rollingStart: true,
  rollingStartMetres: 8,
  initialSpeedMps: 15.7,
  connectByGround: true,
  groundDeparture: [
    [3219.62, -1213.0, 6.2],
    [3223.2, -1201.54, 6.47],
    [3225.33, -1197.78, 6.54],
    [3231.4, -1193.43, 6.64],
    [3237.9, -1188.76, 6.85],
  ],
  roads: [
    "us101-south-potrero-exit",
    "us101-south-bayshore-exit-approach",
    "us101-south-bayshore-north-turn-exit",
  ],
  speedLimits: {
    "us101-south-bayshore-exit-approach": 12,
    "us101-south-bayshore-north-turn-exit": 12,
  },
  forks: [
    ["us101-south-potrero-exit", "us101-south-bayshore-exit-approach", 1.65],
  ],
  covered: ["sf-contact/us101-south-bayshore-north-turn-exit"],
};

cases["us101-south-over-bayshore-exit"] = {
  owners: ["808f7e4c", "808f7e54"],
  speed: 18,
  rollingStart: true,
  rollingStartMetres: 315,
  initialSpeedMps: 18,
  roads: ["us101-south-cesar-through", "us101-south-silver-through"],
  laneGuides: alemanyLaneGuides,
  finishOnDeck: true,
  finishDistance: 120,
  covered: [],
};

const southTurn = "us101-south-bayshore-south-turn-exit";
const bayshoreLower = "bayshore-boulevard-under-us101";
const potreroBayshore = "potrero-to-bayshore-boulevard";
const bayshoreSouthDeparture = [
  [3194.17, -1341.23, 6.8],
  [3193.76, -1349.22, 6.89],
  [3193.35, -1357.21, 7.04],
  [3192.94, -1365.2, 7.05],
  [3192.53, -1373.19, 7.06],
];
cases["us101-south-bayshore-south-turn"] = {
  ...cases["us101-south-bayshore-north-turn"],
  roads: [
    "us101-south-potrero-exit",
    "us101-south-bayshore-exit-approach",
    southTurn,
    bayshoreLower,
  ],
  forks: [
    ...cases["us101-south-bayshore-north-turn"].forks,
    ["us101-south-bayshore-exit-approach", southTurn, -1],
  ],
  speedLimits: { "us101-south-bayshore-exit-approach": 12, [southTurn]: 12 },
  covered: [southTurn, bayshoreLower].map((road) => prefix + road),
  groundDeparture: bayshoreSouthDeparture,
};
cases["potrero-avenue-to-bayshore"] = {
  owners: ["808f7e4c"],
  speed: 11.5,
  roads: [potreroCity, potreroBayshore, bayshoreLower],
  forks: [[potreroCity, potreroBayshore, 1.9]],
  covered: [potreroBayshore, bayshoreLower].map((road) => prefix + road),
  connectByGround: true,
  groundDeparture: bayshoreSouthDeparture,
  maxAirborneMetres: 3,
};

const fifthRamp = "i80-westbound-fifth-harrison-exit";
const fourthThrough = "i80-westbound-fourth-merge";
cases["i80-westbound-fifth-outer-join"] = {
  owners: ["8085807c", "80858084"],
  roads: [somaUpper, fourthThrough],
  speed: 16,
  rollingStart: true,
  initialSpeedMps: 15,
  rollingStartMetres: 465,
  finishOnDeck: true,
  finishDistance: 90,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  maxDurationMs: 40_000,
  laneGuides: {
    [somaUpper]: { lane: -10 },
    [fourthThrough]: {
      lane: -10,
      entranceEast: -0.8823,
      entranceNorth: -0.7795,
      entranceMetres: 16,
    },
  },
};
cases["i80-westbound-fourth-outer"] = {
  owners: ["8085807c", "80858084"],
  roads: [fourthThrough],
  speed: 16,
  rollingStart: true,
  initialSpeedMps: 15,
  rollingStartMetres: 25,
  finishOnDeck: true,
  finishDistance: 225,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  maxDurationMs: 40_000,
  laneGuides: {
    [fourthThrough]: { clearance: { offset: -10, station: 115, length: 86 } },
  },
};
const somaMeasuredLaneRuns = [
  {
    name: "soma-measured-westbound-lane",
    road: somaUpper,
    start: 230,
    finish: 455,
    clearance: { offset: -10, station: 343, length: 107 },
  },
  {
    name: "soma-measured-eastbound-lane",
    road: "i80-eastbound-lower-climb",
    start: 52,
    finish: 242,
    clearance: { offset: 10, station: 147, length: 91 },
  },
];
for (const { name, road, start, finish, clearance } of somaMeasuredLaneRuns) {
  cases[name] = {
    owners: ["8085807c"],
    roads: [road],
    speed: 16,
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 15,
    finishOnDeck: true,
    finishDistance: finish,
    maxAirborneMetres: 0,
    expectNoScrapes: true,
    maxDurationMs: 38_000,
    laneGuides: { [road]: { clearance } },
  };
}
cases["soma-lower-climb-merge-lane"] = {
  owners: ["8085807c"],
  roads: ["i80-eastbound-lower-climb"],
  speed: 15,
  rollingStart: true,
  rollingStartMetres: 12,
  initialSpeedMps: 12,
  finishOnDeck: true,
  finishDistance: 171,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  maxDurationMs: 35_000,
  laneGuides: {
    "i80-eastbound-lower-climb": {
      clearance: { offset: 21.5, station: 68, length: 62 },
    },
  },
};
const westboundLaneRuns = [
  {
    name: "west-span-outside-lane",
    road: "bay-bridge-westbound",
    owners: ["8085803c", "80858074"],
    start: 1080,
    finish: 1360,
    station: 1230,
    offset: -7,
    length: 110,
  },
  {
    name: "west-city-approach-outside-lane",
    road: "city-approach",
    owners: ["80858074", "8085807c"],
    start: 296,
    finish: 486,
    station: 385,
    offset: -10,
    length: 91,
  },
  {
    name: "west-soma-before-fourth-outside-lane",
    road: somaUpper,
    owners: ["8085807c"],
    start: 53,
    finish: 209,
    station: 124,
    offset: -10,
    length: 65,
  },
];
for (const {
  name,
  road,
  owners,
  start,
  finish,
  station,
  offset,
  length,
} of westboundLaneRuns) {
  cases[name] = {
    owners,
    roads: [road],
    speed: 18,
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 16,
    finishOnDeck: true,
    finishDistance: finish,
    maxAirborneMetres: 0,
    expectNoScrapes: true,
    maxDurationMs: 36_000,
    laneGuides: { [road]: { clearance: { station, offset, length } } },
  };
}
cases["westbound-city-measured-right-lane"] = {
  owners: ["80858074", "8085807c"],
  roads: ["city-approach"],
  speed: 18,
  rollingStart: true,
  rollingStartMetres: 325,
  initialSpeedMps: 16,
  finishOnDeck: true,
  finishDistance: 462,
  maxAirborneMetres: 0,
  maxDurationMs: 45_000,
  expectNoScrapes: true,
  laneGuides: {
    "city-approach": { clearance: { offset: -17.2, station: 395, length: 85 } },
  },
};
cases["west-city-approach-fremont-exit"] = {
  ...cases["upper-to-fremont"],
  owners: ["80858074", "8085807c"],
  roads: ["city-approach", "fremont-exit"],
  rollingStartMetres: 341,
  maxDurationMs: 125_000,
  maxAirborneMetres: 0,
};
cases["west-city-approach-through"] = {
  ...cases["westbound-harrison-fork-through"],
  owners: ["80858074", "8085807c"],
  roads: ["city-approach", harrisonFork, somaUpper],
  rollingStartMetres: 341,
  forks: [
    ["city-approach", harrisonFork, 1.3],
    ...cases["westbound-harrison-fork-through"].forks,
  ],
  maxAirborneMetres: 0,
};
cases["i80-westbound-fourth-merge-offset"] = {
  owners: ["8085807c", "80858084"],
  roads: [fourthThrough],
  speed: 8,
  rollingStart: true,
  rollingStartMetres: 497,
  initialSpeedMps: 8,
  finishOnDeck: true,
  finishDistance: 552,
  laneGuides: { [fourthThrough]: { lane: 0.75 } },
  maxDurationMs: 25_000,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
};
const fifthStart = {
  owners: ["8085807c", "80858084"],
  speed: 18,
  rollingStart: true,
  rollingStartMetres: 435,
  initialSpeedMps: 17,
};
cases["i80-westbound-fifth-harrison"] = {
  ...fifthStart,
  roads: ["i80-westbound-upper-soma", fifthRamp],
  speedLimits: { [fifthRamp]: 12 },
  laneGuides: {
    [somaUpper]: { lane: -5.4 },
    [fifthRamp]: {
      entranceEast: -3.8068,
      entranceNorth: 3.83,
      entranceMetres: 285,
    },
  },
  covered: ["sf-contact/" + fifthRamp],
};
cases["i80-westbound-over-fifth"] = {
  ...fifthStart,
  roads: ["i80-westbound-upper-soma", fourthThrough],
  speedLimits: { [fourthThrough]: 15 },
  forks: [[somaUpper, fourthThrough, -12]],
  laneGuides: {
    [somaUpper]: { approachMetres: 115 },
    [fourthThrough]: { entranceMetres: 230 },
  },
  finishOnDeck: true,
  finishDistance: 510,
  covered: [],
};

const alemanySouthConnector = "i280-north-to-us101-south-connector";
const alemanySouthOwners = ["808f7ef4", "808f7efc", "808f7ee4"];
const alemanyForkStart = {
  owners: alemanySouthOwners,
  rollingStart: true,
  rollingStartMetres: 675,
  initialSpeedMps: 16,
  speed: 18,
  finishOnDeck: true,
};
cases["alemany-to-southbound-us101"] = {
  ...alemanyForkStart,
  roads: [
    alemanyWest,
    alemanySouthConnector,
    "i280-to-us101-south-descent",
    "us101-south-paul-approach",
  ],
  speedLimits: {
    [alemanySouthConnector]: 14,
    "i280-to-us101-south-descent": 16,
  },
  forks: [[alemanyWest, alemanySouthConnector, -1.6]],
};
cases["alemany-northbound-through-fork"] = {
  ...alemanyForkStart,
  roads: [alemanyWest, alemanyThrough],
  forks: [alemanyWestThrough],
};

const bayshoreNorthEntry = {
  owners: ["808f7e4c", "808f7e54", "808f7e34"],
  speed: 18,
  finishOnDeck: true,
  finishDistance: 100,
  groundApproachMetres: 11,
};
const marinEntry = "us101-north-marin-entry";
const cesarEntry = "us101-north-cesar-chavez-entry";
const driveToPotrero = [
  "us101-north-bayshore-entry-merge",
  "us101-north-potrero-approach",
];
cases["marin-to-northbound-us101"] = {
  ...bayshoreNorthEntry,
  roads: [marinEntry, ...driveToPotrero],
  speedLimits: { [marinEntry]: 12 },
};
cases["cesar-chavez-to-northbound-us101"] = {
  ...bayshoreNorthEntry,
  roads: [cesarEntry, ...driveToPotrero],
  speedLimits: { [cesarEntry]: 12 },
  laneGuides: {
    [cesarEntry]: { clearance: { offset: 1.9, station: 0, length: 26 } },
  },
};
const southMariposa = "i280-south-mariposa-entry";
cases["mariposa-west-to-southbound-i280"] = {
  roads: [southMariposa, "i280-dogpatch-approach"],
  owners: ["808f7fcc", "808f7fb4", "808f7fac", "808f7fd4"],
  speed: 16,
  speedLimits: { [southMariposa]: 12 },
  finishOnDeck: true,
  finishDistance: 92,
  groundApproachMetres: 0,
  maxDurationMs: 95_000,
  groundArrival: [
    [4104, 454.2],
    [4115, 455],
    [4125.4, 456.4],
  ],
  // The finish is within the chase-camera clearance of the 20th Street deck.
  covered: [
    "sf-contact/i280-south-mariposa-entry",
    "sf-contact/i280-dogpatch-approach",
  ],
  exitCamera: "covered",
};

cases["mariposa-upper-18th-viaduct"] = {
  roads: ["i280-18th-off-ramp"],
  owners: i280Owners,
  speed: 13,
  rollingStart: true,
  rollingStartMetres: 185,
  initialSpeedMps: 12,
  finishOnDeck: true,
  finishDistance: 278,
  covered: [],
};

for (const [name, road, start, finish, station, length] of [
  ["mariposa-southbound-outer", southboundFifthApproach, 130, 338, 283, 115],
  [
    "mariposa-northbound-outer",
    sixthForkApproach,
    220 - mariposaMergeStation,
    362 - mariposaMergeStation,
    352 - mariposaMergeStation,
    110,
  ],
]) {
  cases[name] = {
    owners: i280Owners,
    roads: [road],
    speed: 16,
    rollingStart: true,
    initialSpeedMps: 15,
    rollingStartMetres: start,
    finishOnDeck: true,
    finishDistance: finish,
    maxAirborneMetres: 0,
    expectNoScrapes: true,
    maxDurationMs: 42_000,
    laneGuides: { [road]: { clearance: { offset: 9, station, length } } },
  };
}

for (const direction of ["northbound", "southbound"]) {
  cases[`market-viaduct-${direction}`] = {
    roads: [`market-west-viaduct-${direction}`],
    owners: ["808f7e0c"],
    speed: 11,
    groundApproachMetres: 9,
    maxAirborneMetres: 0,
    expectNoScrapes: true,
  };
}

cases["west-alemany-viaduct"] = {
  roads: ["alemany-west-viaduct"],
  owners: ["808f7c4c"],
  speed: 11,
  groundApproachMetres: 14,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
};

const southernUs101Owners = [
  "808f7e54",
  "808f7ee4",
  "808f7efc",
  "808f7f1c",
  "808f7f24",
  "808f7f2c",
];
const northboundUs101Hester = "us101-north-hester-to-bayshore";
cases["southern-city-edge-to-northbound-us101"] = {
  roads: [
    "us101-north-city-edge-to-hester",
    northboundUs101Hester,
    "us101-north-bayshore-to-paul",
    "us101-north-paul-to-silliman",
    "us101-north-silliman-to-alemany",
    "us101-north-alemany-to-silver",
    "us101-north-silver-approach",
  ],
  owners: southernUs101Owners,
  speed: 21,
  rollingStart: true,
  rollingStartMetres: 24,
  initialSpeedMps: 20.2,
  finishOnDeck: true,
  finishDistance: 100,
  maxDurationMs: 330_000,
  forks: [["us101-north-city-edge-to-hester", northboundUs101Hester, 1.8]],
  covered: [`sf-contact/${northboundUs101Hester}`],
  allowedCovered: [
    "sf-contact/us101-north-paul-to-silliman",
    "sf-contact/us101-north-silliman-to-alemany",
  ],
  maxAirborneMetres: 0,
};
cases["alemany-street-to-northbound-us101"] = {
  roads: [
    "us101-north-alemany-entry",
    "us101-north-alemany-to-silver",
    "us101-north-silver-approach",
  ],
  owners: ["808f7e54", "808f7efc"],
  speed: 15,
  speedLimits: { "us101-north-alemany-entry": 10 },
  finishOnDeck: true,
  finishDistance: 120,
  groundApproachMetres: 12,
  maxDurationMs: 115_000,
  covered: ["sf-contact/us101-north-alemany-entry"],
};

const northboundSilliman = "us101-north-silliman-exit";
const northboundSillimanApproach = "us101-north-paul-to-silliman";
cases["northbound-us101-silliman-to-bayshore"] = {
  roads: [northboundSillimanApproach, northboundSilliman],
  owners: ["808f7ee4", "808f7efc"],
  forks: [[northboundSillimanApproach, northboundSilliman, -1.8]],
  speed: 13,
  rollingStart: true,
  rollingStartMetres: 490,
  initialSpeedMps: 12,
  maxDurationMs: 85_000,
  covered: [prefix + northboundSilliman],
  exitCamera: "covered",
};
cases["northbound-us101-silliman-alemany-loop"] = {
  ...cases["northbound-us101-silliman-to-bayshore"],
  roads: [
    northboundSillimanApproach,
    northboundSilliman,
    "silver-avenue-bridge",
    "us101-north-alemany-entry",
    "us101-north-alemany-to-silver",
    "us101-north-silver-approach",
  ],
  owners: ["808f7ee4", "808f7efc", "808f7e54"],
  speedLimits: {
    "us101-north-alemany-entry": 10,
    "silver-avenue-bridge": 7,
  },
  connectByGround: true,
  reverseRoads: ["silver-avenue-bridge"],
  groundConnection: [
    {
      after: prefix + northboundSilliman,
      via: [[3115.82, -3001.56]],
      minimumMetres: 125,
    },
    {
      after: prefix + "silver-avenue-bridge",
      via: [
        [3005.426, -3000.007],
        [2978.861, -2941.871],
        [2951.955, -2882.991],
        [2943.17, -2864.49],
        [2925.435, -2827.145],
        [2897.008, -2767.284],
        [2884.072, -2751.66],
        [2865.487, -2719.116],
        [2856.31, -2704.91],
        [2852.686, -2692.267],
        [2853.701, -2683.367],
        [2855.46, -2675.651],
        [2860.124, -2669.074],
      ],
      minimumMetres: 420,
    },
  ],
  finishOnDeck: true,
  finishDistance: 100,
  maxDurationMs: 240_000,
  covered: [
    prefix + northboundSilliman,
    "sf-contact/us101-north-alemany-entry",
  ],
  exitCamera: "exterior",
};

const alemanySouthboundConnector = "us101-north-to-i280-south";
const alemanyNorthboundApproach = "us101-north-bayshore-to-paul";
const alemanyUpperConnector = "i280-north-to-us101-south-connector";
cases["northbound-us101-to-southbound-i280"] = {
  roads: [
    alemanyNorthboundApproach,
    alemanySouthboundConnector,
    "i280-south-alemany-through",
  ],
  owners: [
    "808f7ee4",
    "808f7efc",
    "808f7ef4",
    "808f7f1c",
    "808f7f04",
    "808f7e5c",
  ],
  forks: [[alemanyNorthboundApproach, alemanySouthboundConnector, -2]],
  speed: 20,
  speedLimits: { [alemanySouthboundConnector]: 18 },
  rollingStart: true,
  rollingStartMetres: 406,
  initialSpeedMps: 18,
  finishOnDeck: true,
  finishDistance: 115,
  maxDurationMs: 165_000,
  covered: [prefix + alemanySouthboundConnector],
};
cases["alemany-upper-over-northbound-connector"] = {
  roads: [alemanyUpperConnector, "i280-to-us101-south-descent"],
  owners: ["808f7ef4", "808f7efc", "808f7ee4"],
  speed: 15,
  rollingStart: true,
  rollingStartMetres: 970,
  initialSpeedMps: 14,
  finishOnDeck: true,
  finishDistance: 155,
  maxDurationMs: 55_000,
  covered: [],
};
const southAlemanyFork = {
  owners: [...dogpatchOwners, "808f7e54", "808f7ef4", "808f7ee4"],
  speed: 17,
  rollingStart: true,
  rollingStartMetres: 1650,
  initialSpeedMps: 16,
  finishOnDeck: true,
  covered: [],
};
cases["southbound-i280-left-exit-to-us101"] = {
  ...southAlemanyFork,
  roads: [
    southAlemanyApproach,
    southAlemanyLeftExit,
    "i280-to-us101-south-descent",
    "us101-south-paul-approach",
  ],
  forks: [[southAlemanyApproach, southAlemanyLeftExit, 1.7]],
  finishDistance: 80,
  maxDurationMs: 150_000,
};
cases["southbound-i280-through-left-exit-fork"] = {
  ...southAlemanyFork,
  roads: [southAlemanyApproach, southAlemanyMainline],
  forks: [[southAlemanyApproach, southAlemanyMainline, -1.7]],
  laneGuides: { [southAlemanyMainline]: southAlemanyMainlineClearance },
  finishDistance: 275,
  maxDurationMs: 55_000,
};
const northAlemanyClimb = "i280-north-alemany-climb";
const northAlemanyLower = "i280-north-lower-viaduct";
cases["northbound-i280-below-alemany-left-exit"] = {
  ...southAlemanyFork,
  roads: [northAlemanyClimb, northAlemanyLower],
  rollingStartMetres: 695,
  finishDistance: 240,
  speed: 14,
  forks: [[northAlemanyClimb, northAlemanyLower, -1.35]],
  laneGuides: alemanyToSixth.laneGuides,
  maxAirborneMetres: 0,
  covered: [prefix + northAlemanyClimb, prefix + northAlemanyLower],
  exitCamera: "covered",
  maxDurationMs: 55_000,
};
cases["northbound-i280-clears-remaining-cover"] = {
  roads: [northAlemanyLower],
  owners: ["808f7efc", "808f7fac", "808f7f04"],
  rollingStart: true,
  rollingStartMetres: 640,
  initialSpeedMps: 12,
  speed: 13,
  finishOnDeck: true,
  finishDistance: 761,
  covered: [prefix + northAlemanyLower],
  exitCamera: "exterior",
  maxDurationMs: 40_000,
};
cases["northbound-us101-through-alemany-fork"] = {
  ...cases["northbound-us101-to-southbound-i280"],
  roads: [alemanyNorthboundApproach, "us101-north-paul-to-silliman"],
  forks: [[alemanyNorthboundApproach, "us101-north-paul-to-silliman", 1.7]],
  covered: [],
  finishDistance: 130,
  maxDurationMs: 55_000,
};
cases["alemany-late-lower-crossings-camera"] = {
  roads: [alemanySouthboundConnector],
  owners: ["808f7ee4", "808f7efc", "808f7f1c"],
  speed: 15,
  rollingStart: true,
  rollingStartMetres: 1240,
  initialSpeedMps: 14,
  finishOnDeck: true,
  finishDistance: 1430,
  maxDurationMs: 45_000,
  covered: [prefix + alemanySouthboundConnector],
  exitCamera: "covered",
};

const hesterUpperStreet = "bayshore-north-hester-approach";
const hesterEntry = "us101-north-bayshore-hester-entry";
cases["bayshore-hester-street-through"] = {
  roads: [hesterUpperStreet, "bayshore-north-hester-through"],
  owners: ["808f7f24", "808f7f1c", "808f7ee4"],
  speed: 10,
  groundApproachMetres: 11,
  forks: [[hesterUpperStreet, "bayshore-north-hester-through", -1.8]],
  maxDurationMs: 90_000,
};
cases["bayshore-hester-entry-us101"] = {
  ...cases["bayshore-hester-street-through"],
  roads: [hesterUpperStreet, hesterEntry, "us101-north-bayshore-to-paul"],
  forks: [[hesterUpperStreet, hesterEntry, 1.8]],
  speedLimits: { [hesterEntry]: 14, "us101-north-bayshore-to-paul": 16 },
  finishOnDeck: true,
  finishDistance: 70,
  maxDurationMs: 130_000,
};
const hesterFrontage = [
  "bayshore-hester-frontage-lead",
  "bayshore-hester-frontage-to-third",
];
cases["bayshore-hester-frontage-to-third"] = {
  roads: hesterFrontage,
  owners: ["808f7f24", "808f7f1c", "808f7f2c"],
  speed: 11,
  groundApproachMetres: 7,
  maxAirborneMetres: 0,
  maxDurationMs: 115_000,
};
cases["hester-third-street-northbound-approach"] = {
  ...cases["bayshore-hester-frontage-to-third"],
  roads: ["hester-third-northbound-bridge-approach", hesterFrontage[1]],
  speed: 8.5,
};
cases["hester-exit-to-frontage"] = {
  roads: [hesterRoad, ...hesterFrontage],
  owners: ["808f7f24", "808f7f1c", "808f7f2c"],
  rollingStart: true,
  rollingStartMetres: 217,
  initialSpeedMps: 7,
  speed: 9,
  connectByGround: true,
  groundConnection: { after: prefix + hesterRoad, via: [], speedMps: 4 },
  finishOnDeck: true,
  finishDistance: 95,
  maxAirborneMetres: 0,
  maxDurationMs: 90_000,
};

const hesterThirdEntry = "us101-south-hester-third-entry";
const hesterSalinasEntry = "us101-south-hester-salinas-entry";
const hesterLowerEntrance = "us101-south-hester-entrance-collector";
const hesterStreetEntries = {
  owners: ["808f7f24", "808f7f1c", "808f7f2c", "808f7ee4"],
  speed: 11,
  groundApproachMetres: 12,
  maxDurationMs: 145_000,
};
cases["hester-third-street-to-southbound-us101"] = {
  ...hesterStreetEntries,
  roads: [hesterThirdEntry, hesterLowerEntrance, hesterReturn, hesterEdge],
  groundApproachMetres: 0,
  groundArrival: [
    [3825.8, -4439.3],
    [3826.2, -4422],
    [3823, -4413.1],
  ],
  speedLimits: { [hesterEdge]: 16 },
  forks: [[hesterLowerEntrance, hesterReturn, 1.65]],
  finishOnDeck: true,
  finishDistance: 150,
};
cases["hester-salinas-to-bayshore"] = {
  ...hesterStreetEntries,
  roads: [hesterSalinasEntry, hesterLowerEntrance, hesterRoad],
  forks: [[hesterLowerEntrance, hesterRoad, -1.6]],
  laneGuides: hesterStart.laneGuides,
};

for (const [road, tile, station, lane, exitCamera] of [
  ["i280-north-geneva-entry", "808f7c2c", 190, -0.75, "covered"],
  ["bosworth-southbound-entry", "808f7e64", 85, -0.75, "covered"],
  [hesterLowerEntrance, "808f7f24", 84, 0.75, "exterior"],
]) {
  cases[`${road}-offset-lane`] = {
    owners: [tile],
    roads: [road],
    laneGuides: { [road]: { lane } },
    rollingStart: true,
    rollingStartMetres: station - 23,
    finishOnDeck: true,
    finishDistance: station + 28,
    exitCamera,
    speed: 8,
    initialSpeedMps: 8,
    maxAirborneMetres: 0,
    expectNoScrapes: true,
    maxDurationMs: 25_000,
  };
}

cases["hester-upper-northbound-over-entrances"] = {
  ...hesterStreetEntries,
  roads: [northboundUs101Hester],
  speed: 15,
  rollingStart: true,
  rollingStartMetres: 472,
  initialSpeedMps: 14,
  finishOnDeck: true,
  finishDistance: 600,
  covered: [],
};

const hesterNorthApproach = "us101-north-city-edge-to-hester";
const hesterNorthExit = "us101-north-hester-off-ramp";
const hesterNorthThird = "us101-north-hester-third-off-ramp";
const hesterNorthBayshore = "us101-north-hester-bayshore-off-ramp";
const hesterNorthDeparture = {
  owners: ["808f7f24", "808f7f2c", "808f7ee4", "808f7f1c"],
  rollingStart: true,
  rollingStartMetres: 706,
  initialSpeedMps: 14,
  speed: 14,
  maxDurationMs: 120_000,
};
cases["northbound-hester-exit-to-third-street"] = {
  ...hesterNorthDeparture,
  roads: [hesterNorthApproach, hesterNorthExit, hesterNorthThird],
  forks: [
    [hesterNorthApproach, hesterNorthExit, -2],
    [hesterNorthExit, hesterNorthThird, -1.8],
  ],
  speedLimits: { [hesterNorthThird]: 10 },
};
cases["northbound-hester-exit-to-bayshore"] = {
  ...hesterNorthDeparture,
  roads: [hesterNorthApproach, hesterNorthExit, hesterNorthBayshore],
  forks: [
    [hesterNorthApproach, hesterNorthExit, -2],
    [hesterNorthExit, hesterNorthBayshore, 1.8],
  ],
  speedLimits: { [hesterNorthBayshore]: 11 },
};
cases["northbound-hester-stays-on-us101"] = {
  ...hesterNorthDeparture,
  roads: [hesterNorthApproach, northboundUs101Hester],
  forks: [[hesterNorthApproach, northboundUs101Hester, 1.8]],
  finishOnDeck: true,
  finishDistance: 115,
};

cases["hester-third-street-exit-and-reentry"] = {
  ...cases["northbound-hester-exit-to-third-street"],
  roads: [
    ...cases["northbound-hester-exit-to-third-street"].roads,
    hesterThirdEntry,
    hesterLowerEntrance,
    hesterReturn,
    hesterEdge,
  ],
  forks: [
    ...cases["northbound-hester-exit-to-third-street"].forks,
    [hesterLowerEntrance, hesterReturn, 1.65],
  ],
  speedLimits: {
    [hesterNorthThird]: 10,
    [hesterThirdEntry]: 11,
    [hesterLowerEntrance]: 11,
  },
  connectByGround: true,
  groundConnection: {
    after: prefix + hesterNorthThird,
    via: [
      [3747.3, -4571.1],
      [3751.4, -4567],
      [3760.7, -4563.2],
      [3777.2, -4541.4],
      [3796.8, -4515.3],
      [3796.5, -4496.7],
      [3804.6, -4489.2],
      [3815.3, -4461.3],
      [3825, -4433.9],
      [3825.65, -4422.3],
      [3825.61, -4417.8],
      [3823.4, -4414.1],
      [3817.2, -4413.8],
    ],
    minimumMetres: 155,
  },
  finishOnDeck: true,
  finishDistance: 150,
  maxDurationMs: 220_000,
};

const islandLower = "bay-bridge-eastbound-lower";
const islandWestExit = "yerba-buena-eastbound-off-ramp";
const islandWestEntry = "yerba-buena-westbound-on-ramp";
const islandUpper = "bay-bridge-westbound";
const islandWestOwners = [
  "8085803c",
  "8085801c",
  "80858024",
  "8085800c",
  "80858014",
  "8085806c",
];
const islandWestExitCase = {
  owners: islandWestOwners,
  roads: [islandLower, islandWestExit],
  rollingStart: true,
  rollingStartMetres: 2670,
  initialSpeedMps: 19,
  speed: 19,
  speedLimits: { [islandWestExit]: 12 },
  forks: [[islandLower, islandWestExit, 2.25]],
  maxDurationMs: 135_000,
};
cases["island-west-eastbound-exit"] = islandWestExitCase;
cases["island-west-westbound-entry"] = {
  owners: islandWestOwners,
  roads: [islandWestEntry, islandUpper],
  speed: 12,
  speedLimits: { [islandUpper]: 18 },
  groundApproachMetres: 13,
  finishOnDeck: true,
  finishDistance: 160,
  maxDurationMs: 130_000,
};
cases["island-west-eastbound-through"] = {
  ...islandWestExitCase,
  roads: [islandLower, "yerba-buena-lower-west-approach"],
  forks: [[islandLower, "yerba-buena-lower-west-approach", -2.25]],
  finishOnDeck: true,
  finishDistance: 130,
  exitCamera: "covered",
};

cases["island-west-exit-and-reentry"] = {
  ...islandWestExitCase,
  roads: [islandLower, islandWestExit, islandWestEntry, islandUpper],
  speedLimits: {
    [islandWestExit]: 12,
    [islandWestEntry]: 11,
    [islandUpper]: 18,
  },
  connectByGround: true,
  groundConnection: {
    after: prefix + islandWestExit,
    via: [
      [6174, 5479.2],
      [6162, 5480.86],
    ],
    minimumMetres: 31,
    turn: { east: 6162, north: 5480.86, direction: -1 },
  },
  finishOnDeck: true,
  finishDistance: 155,
  maxDurationMs: 215_000,
};

const islandEastOwners = ["80858024", "8085803c"];
cases["island-east-westbound-entry"] = {
  owners: islandEastOwners,
  roads: [
    "yerba-buena-east-westbound-on-ramp",
    "yerba-buena-east-westbound-entrance-tunnel",
    "yerba-buena-upper-tunnel-west-of-merge",
    "yerba-buena-west-approach",
  ],
  speed: 11,
  speedLimits: { "yerba-buena-west-approach": 16 },
  groundApproachMetres: 9,
  finishOnDeck: true,
  finishDistance: 77,
  maxDurationMs: 120_000,
};
const macallaEntrance = "macalla-road-westbound-i80-entrance";
cases["macalla-to-westbound-i80"] = {
  owners: islandEastOwners,
  roads: [macallaEntrance, ...upperFromIsland.slice(1, 5)],
  groundArrival: [
    [6620.4, 5768.0],
    [6631.5, 5763.9],
    [6649.0, 5759.4],
  ],
  groundApproachMetres: 0,
  speed: 14,
  speedLimits: { [macallaEntrance]: 11 },
  finishOnDeck: true,
  finishDistance: 77,
  maxDurationMs: 150_000,
};
cases["island-east-upper-mainline"] = {
  owners: islandEastOwners,
  roads: upperFromIsland.slice(0, 5),
  rollingStart: true,
  initialSpeedMps: 18,
  speed: 18,
  finishOnDeck: true,
  finishDistance: 77,
  maxDurationMs: 120_000,
};

const potreroCrossingTiles = ["808f7e34", "808f7e4c"];
for (const reverse of [false, true]) {
  cases[reverse ? "potrero-23rd-eastbound" : "potrero-23rd-westbound"] = {
    owners: potreroCrossingTiles,
    roads: ["potrero-23rd-us101-overpass"],
    reverse,
    groundApproachMetres: 13,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [3310, -607.31, 29.9],
          [3335, -605.4, 30],
        ]
      : [
          [3205, -615.35, 26.8],
          [3176, -617.57, 26.5],
        ],
    speed: 8,
    maxDurationMs: 65000,
  };
}
for (const [direction, road, start, end] of [
  ["northbound", "us101-north-potrero-approach", 130, 255],
  ["southbound", "us101-south-potrero-main", 1350, 1495],
]) {
  cases[`potrero-23rd-under-${direction}`] = {
    owners: potreroCrossingTiles,
    roads: [road],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 15,
    speed: 15,
    finishOnDeck: true,
    finishDistance: end,
    covered: [prefix + road],
    maxDurationMs: 65000,
  };
}

const lowerEastOwners = ["80858024", "8085803c"];
const firstLowerBore = "yerba-buena-lower-tunnel";
const eastExitBore = "yerba-buena-east-eastbound-exit-tunnel";
const lowerEastExit = "yerba-buena-east-eastbound-off-ramp";
const afterLowerExit = "yerba-buena-lower-tunnel-east-of-exit";
cases["island-east-eastbound-exit"] = {
  owners: lowerEastOwners,
  roads: [firstLowerBore, eastExitBore, lowerEastExit],
  rollingStart: true,
  rollingStartMetres: 96,
  initialSpeedMps: 14,
  speed: 12,
  speedLimits: { [eastExitBore]: 10, [lowerEastExit]: 8 },
  forks: [[firstLowerBore, eastExitBore, -2.4]],
  connectByGround: true,
  groundDeparture: [
    [6743.1, 5580, 47.9],
    [6744.1, 5560, 47.8],
  ],
  maxDurationMs: 115_000,
};
const southgateFeeder = "yerba-buena-eastbound-exit-to-southgate";
const southgateShared = "yerba-buena-southgate-to-macalla-shared";
cases["island-east-southgate-offset"] = {
  owners: ["80858024"],
  roads: [southgateFeeder],
  speed: 6,
  rollingStart: true,
  rollingStartMetres: 43,
  initialSpeedMps: 6,
  finishOnDeck: true,
  finishDistance: 102,
  laneGuides: { [southgateFeeder]: { lane: 0.75 } },
  maxDurationMs: 25_000,
  maxAirborneMetres: 0,
  expectNoScrapes: true,
  covered: [prefix + southgateFeeder],
};
cases["island-east-eastbound-exit-to-macalla"] = {
  ...cases["island-east-eastbound-exit"],
  roads: [
    firstLowerBore,
    eastExitBore,
    lowerEastExit,
    southgateFeeder,
    southgateShared,
  ],
  speedLimits: {
    ...cases["island-east-eastbound-exit"].speedLimits,
    [southgateFeeder]: 6,
    [southgateShared]: 7,
  },
  groundDeparture: [
    [6705.0757, 5746.5103, 48.2052],
    [6700.4928, 5748.5095, 48.7135],
    [6695.9099, 5750.5088, 49.202],
    [6691.168, 5752.094, 49.7117],
    [6686.4248, 5753.6758, 50.2454],
  ],
  covered: [prefix + southgateFeeder, prefix + southgateShared],
  exitCamera: "covered",
  maxDepartureClearanceMetres: 0.25,
};
cases["island-east-lower-mainline"] = {
  owners: lowerEastOwners,
  roads: [
    firstLowerBore,
    afterLowerExit,
    "yerba-buena-lower-east-approach",
    "yerba-buena-lower-east-after-entry",
  ],
  rollingStart: true,
  rollingStartMetres: 96,
  initialSpeedMps: 16,
  speed: 16,
  forks: [[firstLowerBore, afterLowerExit, 1.5]],
  finishOnDeck: true,
  finishDistance: 70,
  covered: ["yerba-buena-lower-east-approach"],
  // The rear camera probes can still reach an overhead deck just past the join.
  allowedCovered: ["yerba-buena-lower-east-after-entry"],
  maxDurationMs: 115_000,
};
const hillcrestEntry = "yerba-buena-east-eastbound-on-ramp";
const hillcrestMerge = "bay-bridge-east-span-eastbound";
cases["island-east-eastbound-entry"] = {
  owners: lowerEastOwners,
  roads: [hillcrestEntry, hillcrestMerge],
  groundApproachMetres: 10,
  speed: 10,
  speedLimits: { [hillcrestMerge]: 17 },
  finishOnDeck: true,
  finishDistance: 45,
  cameraStations: [
    { x: 6783.7, y: 5747.4, surface: hillcrestEntry, camera: "exterior" },
  ],
  maxAirborneMetres: 0,
  maxDurationMs: 90_000,
};
cases["island-east-roundtrip"] = {
  ...cases["island-east-eastbound-exit-to-macalla"],
  roads: [
    firstLowerBore,
    eastExitBore,
    lowerEastExit,
    southgateFeeder,
    southgateShared,
    hillcrestEntry,
    hillcrestMerge,
  ],
  groundDeparture: undefined,
  exitCamera: undefined,
  maxDepartureClearanceMetres: undefined,
  groundConnection: {
    after: prefix + southgateShared,
    via: [
      [6704, 5747],
      [6693, 5750],
      [6681, 5753],
      [6670, 5756],
      [6640, 5766],
      [6623, 5770],
      [6622, 5765],
      [6631, 5764.5],
      [6642, 5763],
      [6652, 5760],
      [6666, 5757],
      [6682, 5752],
      [6702, 5747],
      [6718, 5743],
      [6724, 5735],
      [6733.5, 5715],
      [6736.8, 5706],
      [6737.2, 5697],
      [6743.6, 5674],
      [6750, 5658],
      [6759, 5645],
      [6765, 5632],
      [6763, 5624],
      [6757, 5616],
      [6749, 5613],
      [6744, 5618],
      [6740, 5630],
      [6737, 5641],
    ],
    minimumMetres: 120,
    turn: {
      east: 6623,
      north: 5770,
      direction: 1,
      returnSpeedMps: 3.1,
      returnVia: [
        [6630.5, 5764.9],
        [6641.5, 5762.5],
        [6651.6, 5759.5],
        [6665.1, 5756.5],
        [6680, 5753],
        [6702, 5747],
        [6716, 5743],
        [6724.5, 5738],
        [6728.6, 5725],
        [6733.5, 5715],
        [6736.8, 5706],
        [6737.2, 5697],
        [6738.3, 5687],
        [6743.6, 5674],
        [6750, 5658],
        [6760, 5645],
        [6765, 5632],
        [6763, 5624],
        [6758, 5618],
        [6751, 5614],
        [6746, 5616],
        [6741, 5628],
        [6737, 5643],
      ],
    },
  },
  speedLimits: {
    ...cases["island-east-eastbound-exit-to-macalla"].speedLimits,
    [hillcrestEntry]: 12,
    [hillcrestMerge]: 17,
  },
  finishOnDeck: true,
  finishDistance: 55,
  covered: [prefix + southgateFeeder, prefix + southgateShared],
  allowedCovered: [prefix + hillcrestEntry, hillcrestMerge],
  maxAirborneMetres: 0,
  maxDurationMs: 375_000,
};
for (const reverse of [false, true]) {
  const direction = reverse ? "eastbound" : "westbound";
  cases[`broadway-${direction}`] = {
    roads: [`broadway-${direction}`],
    owners: ["808580ec", "808580f4"],
    reverse,
    groundApproachMetres: 8,
    speed: 14,
    maxDurationMs: 105_000,
  };
}
for (const reverse of [false, true]) {
  const road = "hyde-street-over-broadway";
  const direction = reverse ? "southbound" : "northbound";
  cases[`hyde-street-${direction}`] = {
    roads: [road],
    owners: ["808580ec"],
    reverse,
    speed: 10.5,
    laneGuides: { [road]: { lane: reverse ? 1.5 : -1.5 } },
    groundApproachMetres: 8,
    connectByGround: true,
    groundDeparture: reverse
      ? [[1901.59, 3988.22, 64.94]]
      : [[1891.82, 4067.68, 62.77]],
    maxApproachClearanceMetres: 0.18,
    maxDepartureClearanceMetres: 0.18,
    maxDurationMs: 60_000,
  };
}
for (const reverse of [false, true]) {
  const road = "bryant-street-delancey-viaduct";
  const direction = reverse ? "northeastbound" : "southwestbound";
  cases[`bryant-street-${direction}`] = {
    roads: [road],
    owners: ["80858074", "8085807c"],
    reverse,
    speed: 11,
    laneGuides: { [road]: { lane: reverse ? 1.55 : -1.55 } },
    groundApproachMetres: 8,
    connectByGround: true,
    groundDeparture: reverse
      ? [[4415, 2896.3, 3.97]]
      : [[4242.4, 2711.2, 18.45]],
    maxApproachClearanceMetres: 0.18,
    maxDepartureClearanceMetres: 0.18,
    maxDurationMs: 65_000,
  };
}
for (const reverse of [false, true]) {
  const road = "delancey-street-beside-bryant";
  cases[reverse ? "delancey-northeastbound" : "delancey-southwestbound"] = {
    roads: [road],
    owners: ["80858074", "8085807c"],
    reverse,
    speed: 8.5,
    laneGuides: { [road]: { lane: reverse ? 1.25 : -1.25 } },
    groundApproachMetres: reverse ? 4 : 8,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [4432.1, 2898.9, 3.5],
          [4434.7, 2902.3, 3.52],
          [4436.6, 2910.4, 3.59],
        ]
      : [
          [4302.1, 2749.5, 6.2],
          [4306, 2742.3, 6.1],
          [4313.3, 2734, 6.1],
          [4322.9, 2719.8, 6.2],
          [4327.6, 2714.3, 6.2],
        ],
    maxApproachClearanceMetres: 0.18,
    maxDepartureClearanceMetres: 0.18,
    maxDurationMs: 65_000,
  };
}
for (const [road, forward, backward, departures] of [
  [
    "sickles-avenue-under-i280",
    "sickles-northwestbound",
    "sickles-southeastbound",
    [
      [
        [-1392.5, -5414.15, 91.34],
        [-1390.1, -5408.3, 91.4],
        [-1384.8, -5400.2, 91.5],
      ],
      [[-1318.25, -5492.32, 91.63]],
    ],
  ],
  [
    "san-jose-avenue-first-under-i280-at-sickles",
    "san-jose-first-southwestbound",
    "san-jose-first-northeastbound",
    [
      [[-1487.6, -5526, 93.79]],
      [
        [-1395.5, -5417.4, 91.37],
        [-1391.5, -5410.8, 91.4],
        [-1386.5, -5403.2, 91.6],
        [-1378.1, -5391.9, 92.0],
      ],
    ],
  ],
  [
    "san-jose-avenue-second-under-i280-at-sickles",
    "san-jose-second-southwestbound",
    "san-jose-second-northeastbound",
    [
      [
        [-1499.1, -5528.1, 94.09],
        [-1504, -5538.3, 94.4],
        [-1514.8, -5550.6, 95.1],
        [-1525, -5563.6, 95.9],
      ],
      [
        [-1408, -5424.7, 91.47],
        [-1395.5, -5417.4, 91.37],
        [-1391.5, -5410.8, 91.4],
        [-1386.5, -5403.2, 91.6],
        [-1378.1, -5391.9, 92.0],
      ],
    ],
  ],
]) {
  for (const reverse of [false, true]) {
    cases[reverse ? backward : forward] = {
      roads: [road],
      owners: ["808f7c24"],
      reverse,
      speed: 9,
      laneGuides: { [road]: { lane: reverse ? 1.5 : -1.5 } },
      groundApproachMetres: 8,
      connectByGround: true,
      groundDeparture: departures[Number(reverse)],
      covered: [prefix + road],
      maxApproachClearanceMetres: 0.18,
      maxDepartureClearanceMetres: 0.18,
      maxDurationMs: 60_000,
    };
  }
}
for (const [direction, road, start, finish] of [
  ["northbound", "i280-north-edge-to-sr82", 1335, 1490],
  ["southbound", "i280-south-geneva-to-sr82", 480, 630],
]) {
  cases[`i280-sickles-${direction}`] = {
    roads: [road],
    owners: ["808f7c24", "808f7c3c"],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 15,
    speed: 18,
    finishOnDeck: true,
    finishDistance: finish,
    maxAirborneMetres: 0,
    maxDurationMs: 65000,
  };
}
cases["east-span-eastbound"] = {
  roads: ["yerba-buena-lower-east-after-entry", eastSpanEastbound],
  owners: islandEastOwners,
  speed: 22,
  rollingStart: true,
  finishOnDeck: true,
};
const oaklandOwners = [
  "80858024",
  "8085802c",
  "80857f8c",
  "80857f94",
  "80857fbc",
  "80857fb4",
];
cases["east-span-to-oakland"] = {
  owners: oaklandOwners,
  roads: [eastSpanEastbound, "bay-bridge-oakland-eastbound"],
  rollingStart: true,
  rollingStartMetres: 400,
  initialSpeedMps: 22.35,
  speed: 22.35,
  maxAirborneMetresAfterEntrance: 0,
  expectNoScrapes: true,
  maxDurationMs: 235_000,
};
cases["oakland-to-east-span"] = {
  owners: oaklandOwners,
  roads: ["bay-bridge-oakland-westbound", eastSpanWestbound],
  speed: 22.35,
  groundApproachMetres: 9,
  finishOnDeck: true,
  finishDistance: 250,
  maxAirborneMetresAfterEntrance: 0,
  expectNoScrapes: true,
  maxDurationMs: 235_000,
};
// Both outer lanes pass the observation deck on each separate carriageway.
for (const [direction, road, start, finish] of [
  ["eastbound", eastSpanEastbound, 217, 536],
  ["westbound", eastSpanWestbound, 366, 686],
]) {
  for (const [side, lane] of [
    ["left", 7.2],
    ["right", -7.2],
  ]) {
    cases[`east-span-observation-${direction}-${side}`] = {
      owners: ["80858024", "80857f8c"],
      roads: [road],
      speed: 19,
      rollingStart: true,
      rollingStartMetres: start,
      initialSpeedMps: 0,
      finishOnDeck: true,
      finishDistance: finish,
      laneGuides: { [road]: { lane } },
      maxAirborneMetres: 0,
      expectNoScrapes: true,
      maxDurationMs: 55_000,
    };
  }
}
const eastSpan = "yerba-buena-upper-east-span";
const westExit = "yerba-buena-east-westbound-off-ramp";
const eastThrough = "yerba-buena-upper-east-through";
cases["island-east-westbound-exit"] = {
  owners: islandEastOwners,
  roads: [eastSpanWestbound, eastSpan, westExit],
  rollingStart: true,
  startPlace: "yerba-buena-westbound",
  speed: 14,
  speedLimits: { [westExit]: 9 },
  forks: [[eastSpan, westExit, -2.25]],
  connectByGround: true,
  groundDeparture: [
    [6628, 5768, 56],
    [6618, 5772, 54.9],
  ],
  maxDurationMs: 165_000,
};
cases["island-east-westbound-through-exit"] = {
  owners: islandEastOwners,
  roads: [eastSpan, eastThrough, ...upperFromIsland.slice(0, 5)],
  rollingStart: true,
  rollingStartMetres: 5,
  speed: 18,
  forks: [[eastSpan, eastThrough, 2]],
  finishOnDeck: true,
  finishDistance: 77,
  maxDurationMs: 125_000,
};
cases["macalla-westbound-roundtrip"] = {
  ...cases["island-east-westbound-exit"],
  roads: [
    eastSpanWestbound,
    eastSpan,
    westExit,
    macallaEntrance,
    ...upperFromIsland.slice(1, 5),
  ],
  speedLimits: { [westExit]: 9, [macallaEntrance]: 11 },
  groundDeparture: undefined,
  groundConnection: {
    after: prefix + westExit,
    via: [
      [6631, 5767],
      [6623, 5770],
      [6622, 5765],
      [6632, 5764],
      [6642, 5762.3],
      [6647.5, 5761.5],
    ],
    minimumMetres: 38,
    turn: {
      east: 6623,
      north: 5770,
      direction: 1,
      returnVia: [
        [6630.5, 5764.9],
        [6641.5, 5762.6],
        [6648.4, 5761.0],
      ],
    },
  },
  finishOnDeck: true,
  finishDistance: 77,
  maxDurationMs: 195_000,
};
const goldenGateOwners = [
  "808586dc",
  "808586e4",
  "808586ec",
  "80858694",
  "8085869c",
  "80858424",
  "8085842c",
];
const goldenGateSections = (direction) =>
  Array.from({ length: 6 }, (_, i) => `golden-gate-${direction}-${i + 1}`);
const visitorMerge = "golden-gate-northbound-from-visitor";
const vistaFork = "golden-gate-northbound-6";
const vistaExit = "golden-gate-vista-exit";
const vistaEntry = "golden-gate-vista-entry";
const vistaMainline = "golden-gate-northbound-vista-mainline";
const pastVista = "golden-gate-northbound-past-vista";
const merchantFork = "golden-gate-southbound-to-merchant";
const merchantExit = "golden-gate-merchant-exit";
const visitorEntry = "golden-gate-visitor-entry";
const southBridgeApproach = "golden-gate-southbound-6";
const southboundSpan = goldenGateSections("southbound").slice(0, -1);
function northboundSpan(hasRoad) {
  const [first, ...span] = goldenGateSections("northbound");
  return hasRoad(visitorMerge)
    ? [first, visitorMerge, ...span]
    : [first, ...span];
}

cases["golden-gate-presidio-to-marin"] = {
  owners: goldenGateOwners,
  forBake: (hasRoad) => ({
    roads: [
      doyleNorthExit,
      ...northboundSpan(hasRoad),
      ...(hasRoad(vistaExit) ? [vistaExit] : []),
    ],
  }),
  rollingStart: true,
  rollingStartMetres: 28,
  speed: 22,
  speedLimits: {
    "golden-gate-northbound-1": 16,
    [vistaFork]: 11,
    [vistaExit]: 11,
  },
  maxDurationMs: 280_000,
};
cases["golden-gate-marin-to-presidio"] = {
  owners: goldenGateOwners,
  forBake: (hasRoad) => ({
    roads: [
      ...southboundSpan,
      ...(hasRoad(merchantFork) ? [merchantFork] : []),
      southBridgeApproach,
      "presidio-doyle-southbound-entry",
    ],
    forks: [
      ...(hasRoad(merchantFork)
        ? [[merchantFork, southBridgeApproach, 1.8]]
        : []),
      [southBridgeApproach, "presidio-doyle-southbound-entry", 1.75],
    ],
  }),
  groundApproachMetres: 8,
  speed: 22,
  speedLimits: {
    "golden-gate-southbound-1": 14,
    [merchantFork]: 16,
    [southBridgeApproach]: 15,
  },
  finishOnDeck: true,
  finishDistance: 175,
  maxDurationMs: 265_000,
};
const northboundBridge = cases["golden-gate-presidio-to-marin"];
cases["golden-gate-full-span-past-vista"] = {
  ...northboundBridge,
  forBake: (hasRoad) => ({
    roads: [
      doyleNorthExit,
      ...northboundSpan(hasRoad),
      vistaMainline,
      pastVista,
    ],
  }),
  finishOnDeck: true,
  finishDistance: 225,
  maxAirborneMetres: 0,
};
cases["golden-gate-full-span-vista-and-back"] = {
  ...northboundBridge,
  forBake: (hasRoad) => ({
    roads: [
      doyleNorthExit,
      ...northboundSpan(hasRoad),
      vistaExit,
      vistaEntry,
      pastVista,
    ],
  }),
  speedLimits: {
    ...northboundBridge.speedLimits,
    [vistaFork]: 8,
    [vistaExit]: 7,
    [vistaEntry]: 9,
    [pastVista]: 13,
  },
  connectByGround: true,
  groundConnection: {
    after: prefix + vistaExit,
    speedMps: 5,
    minimumMetres: 295,
    via: [
      [-3606.21, 8109.03],
      [-3597.08, 8094.99],
      [-3581.6, 8073.55],
      [-3574.32, 8066.45],
      [-3565.05, 8060.52],
      [-3527.07, 8040.49],
      [-3519.98, 8037.9],
      [-3512.67, 8037.26],
      [-3506.23, 8038.87],
      [-3500.52, 8041.82],
      [-3495.72, 8045.82],
      [-3492, 8050.6],
      [-3489.73, 8056.34],
      [-3488.95, 8062.26],
      [-3489.36, 8068.3],
      [-3491.04, 8073.32],
      [-3493.19, 8077.09],
      [-3499.25, 8084],
      [-3511.71, 8092.36],
      [-3525.59, 8100.22],
      [-3543.8, 8109.6],
      [-3564.82, 8118.58],
    ],
  },
  finishOnDeck: true,
  finishDistance: 210,
  maxDurationMs: 420_000,
};
const tollPlaza = "toll-plaza-tunnel";
cases["golden-gate-full-span-merchant-toll-and-back"] = {
  owners: goldenGateOwners,
  roads: [
    ...southboundSpan,
    merchantFork,
    merchantExit,
    tollPlaza,
    visitorEntry,
    visitorMerge,
  ],
  groundApproachMetres: 8,
  speed: 18,
  speedLimits: {
    "golden-gate-southbound-1": 14,
    [merchantFork]: 8,
    [merchantExit]: 6,
    [tollPlaza]: 6.25,
    [visitorEntry]: 8,
    [visitorMerge]: 12,
  },
  forks: [[merchantFork, merchantExit, -2.1]],
  connectByGround: true,
  groundConnection: [
    {
      after: prefix + merchantExit,
      minimumMetres: 45,
      via: [
        [-3116.5, 5148.35],
        [-3118.25, 5147.28],
        [-3120.1, 5147.19],
        [-3122.1, 5148.03],
        [-3124.2, 5150.4],
        [-3127.5, 5154.4],
        [-3132.1, 5158.6],
        [-3142.5, 5170.7],
        [-3152.6, 5182.9],
        [-3158, 5188.9],
      ],
    },
    {
      after: prefix + tollPlaza,
      minimumMetres: 77,
      via: [
        [-3078.2, 5273.4],
        [-3072.7, 5276.8],
        [-3065, 5282.8],
        [-3058.8, 5285.5],
        [-3051.2, 5286.2],
        [-3042.3, 5285],
        [-3034.1, 5281.4],
        [-3027.9, 5275.4],
        [-3015.4, 5260],
        [-3010.7, 5255.9],
        [-3008.9, 5253.6],
      ],
    },
  ],
  finishOnDeck: true,
  finishDistance: 85,
  maxDurationMs: 400_000,
};
const sr1SouthMerge = "presidio-to-sr1-south-merge";
const presidioLoop = "presidio-nb-loop-to-sr1-south";
const goldenGateSr1Exit = "presidio-southbound-to-sr1-exit";
const fromDoyleToSr1 = {
  owners: goldenGateOwners,
  rollingStart: true,
  initialSpeedMps: 14,
  speed: 16,
  speedLimits: { [presidioLoop]: 11, [goldenGateSr1Exit]: 13 },
  finishOnDeck: true,
  finishDistance: 100,
  maxAirborneMetres: 0,
};
cases["doyle-northbound-loop-to-sr1"] = {
  ...fromDoyleToSr1,
  roads: [
    "presidio-nb-viaduct",
    "presidio-nb-exit",
    presidioLoop,
    sr1SouthMerge,
    "ruckman-viaduct-southbound",
  ],
  forks: [["presidio-nb-viaduct", "presidio-nb-exit", -1.65]],
  covered: [prefix + presidioLoop],
};
cases["golden-gate-southbound-exit-to-sr1"] = {
  ...fromDoyleToSr1,
  roads: [
    "golden-gate-southbound-6",
    goldenGateSr1Exit,
    sr1SouthMerge,
    "ruckman-viaduct-southbound",
  ],
  forks: [["golden-gate-southbound-6", goldenGateSr1Exit, -2.1]],
};
cases["ruckman-northbound-to-golden-gate"] = {
  ...fromDoyleToSr1,
  roads: [
    ruckmanNorth,
    sr1Bridge,
    "sr1-north-to-doyle-merge",
    "golden-gate-northbound-1",
  ],
  forks: [[ruckmanNorth, sr1Bridge, 2.1]],
  finishDistance: 90,
  covered: [prefix + "sr1-north-to-doyle-merge"],
};
for (const name of ["northbound", "presidio-marina-to-sr1"]) {
  const previous = cases[name];
  cases[name] = {
    ...previous,
    roads: [
      ...previous.roads,
      ...cases["doyle-northbound-loop-to-sr1"].roads.slice(2),
    ],
    speedLimits: { ...previous.speedLimits, [presidioLoop]: 11 },
    finishOnDeck: true,
    finishDistance: 100,
  };
}
for (const name of [
  "macarthur-ruckman-northbound",
  "sr1-kobbe-to-golden-gate",
]) {
  const previous = cases[name];
  cases[name] = {
    ...previous,
    owners: [...new Set([...previous.owners, "808586ec"])],
    roads: [
      ...previous.roads,
      ...cases["ruckman-northbound-to-golden-gate"].roads.slice(2),
    ],
    finishOnDeck: true,
    finishDistance: 90,
  };
}
for (const street of ["richland", "highland"]) {
  for (const [direction, reverse, approach] of [
    ["eastbound", false, 2.5],
    ["westbound", true, 7],
  ]) {
    cases[`bernal-cut-${street}-${direction}`] = {
      owners: ["808f7e64"],
      roads: [`${street}-avenue-bridge`],
      reverse,
      groundApproachMetres: approach,
      speed: 8,
    };
  }
}
const oceanOwners = [
  "808f7c2c",
  "808f7e84",
  "808f7e7c",
  "808f7e64",
  "808f7ef4",
];
const oceanNorthbound = [
  "i280-north-ocean-entry",
  "i280-north-ocean-connector",
  "i280-north-ocean-mainline",
  "i280-north-ocean-monterey",
  "branch-east-i280-northbound",
  "i280-north-monterey-merge",
  "i280-north-monterey-to-alemany",
];
for (const [direction, reverse] of [
  ["eastbound", true],
  ["westbound", false],
]) {
  cases[`ocean-avenue-bridge-${direction}`] = {
    owners: oceanOwners,
    roads: ["ocean-avenue-bridge"],
    reverse,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [-633, -4111.55, 69.3],
          [-624, -4111.6, 68.5],
        ]
      : [
          [-747, -4111.07, 74.1],
          [-754, -4111.05, 74.45],
        ],
    groundApproachMetres: 6,
    speed: 8,
  };
}
for (const [name, reverse, lane] of [
  ["westbound-mapped", false, -10],
  ["eastbound-inside-mapped", true, 4.6],
  ["eastbound-outside-mapped", true, 7.7],
]) {
  cases[`ocean-avenue-bridge-${name}`] = {
    owners: oceanOwners,
    roads: ["ocean-avenue-bridge"],
    reverse,
    laneGuides: { "ocean-avenue-bridge": { lane } },
    groundApproachMetres: 10,
    speed: 8,
    maxAirborneMetres: 0,
  };
}
cases["i280-ocean-northbound-entry"] = {
  owners: oceanOwners,
  roads: oceanNorthbound,
  groundApproachMetres: 4,
  finishOnDeck: true,
  finishDistance: 80,
  speed: 19,
  speedLimits: { "i280-north-ocean-entry": 9 },
};
cases["ocean-bridge-to-i280-northbound"] = {
  owners: oceanOwners,
  roads: ["ocean-avenue-bridge", ...oceanNorthbound.slice(0, 1)],
  reverseRoads: ["ocean-avenue-bridge"],
  connectByGround: true,
  groundConnection: {
    after: "sf-contact/ocean-avenue-bridge",
    via: [
      [-637.7, -4110.2],
      [-635.8, -4106.8],
    ],
    minimumMetres: 7,
  },
  groundApproachMetres: 6,
  finishOnDeck: true,
  finishDistance: 85,
  maxDurationMs: 55_000,
  speed: 8,
};
cases["i280-ocean-exit-and-reentry"] = {
  ...cases["i280-ocean-northbound-entry"],
  roads: [
    "i280-south-ocean-approach",
    "i280-south-ocean-exit",
    "ocean-avenue-bridge",
    ...oceanNorthbound,
  ],
  rollingStart: true,
  rollingStartMetres: 24,
  reverseRoads: ["ocean-avenue-bridge"],
  connectByGround: true,
  groundConnection: [
    {
      after: "sf-contact/i280-south-ocean-exit",
      minimumMetres: 420,
      via: [
        [-840, -4108.7],
        [-869, -4103.2],
        [-929, -4099.4],
        [-996, -4098.3],
        [-1031, -4105.8],
        [-1075, -4098.8],
        [-1083, -4103.2],
        [-1079, -4107],
        [-1064.6, -4118.6],
        [-1014.4, -4151.7],
        [-960.4, -4179.4],
        [-907, -4206.7],
        [-894.1, -4210],
        [-887.8, -4202.3],
        [-869.7, -4165.2],
        [-847, -4120.4],
        [-839, -4114.7],
        [-800, -4115.7],
        [-770, -4115.7],
        [-745, -4113],
      ],
    },
    cases["ocean-bridge-to-i280-northbound"].groundConnection,
  ],
  speedLimits: {
    "i280-south-ocean-approach": 12,
    "i280-south-ocean-exit": 11,
    "ocean-avenue-bridge": 8,
    "i280-north-ocean-entry": 9,
  },
};
const genevaOwners = [
  "808f7c24",
  "808f7c2c",
  "808f7e84",
  "808f7e7c",
  "808f7e64",
];
for (const reverse of [false, true]) {
  const direction = reverse ? "westbound" : "eastbound";
  cases[`geneva-avenue-${direction}`] = {
    owners: ["808f7c2c"],
    roads: [`geneva-avenue-${direction}-over-i280`],
    speed: 6.5,
    groundApproachMetres: 6,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [-744.4, -4287.3, 76.25],
          [-749, -4283.2, 76.45],
          [-754, -4278.8, 76.6],
        ]
      : [
          [-679.5, -4309.4, 74.72],
          [-673, -4313.2, 74.53],
          [-668.6, -4317.6, 74.4],
        ],
    maxApproachClearanceMetres: 0.18,
    maxDepartureClearanceMetres: 0.18,
    maxDurationMs: 55_000,
  };
}
cases["geneva-southbound-through"] = {
  owners: genevaOwners,
  roads: [
    "i280-south-monterey-to-ocean",
    "i280-south-ocean-to-geneva",
    "i280-south-beyond-geneva",
  ],
  rollingStart: true,
  rollingStartMetres: 530,
  finishOnDeck: true,
  finishDistance: 140,
  speed: 19,
  maxDurationMs: 110_000,
};
cases["geneva-northbound-through"] = {
  owners: genevaOwners,
  roads: [
    "i280-north-geneva-approach",
    "i280-north-geneva-to-ocean",
    "i280-north-ocean-mainline",
  ],
  rollingStart: true,
  rollingStartMetres: 3,
  initialSpeedMps: 10,
  finishOnDeck: true,
  finishDistance: 160,
  speed: 18,
  maxDurationMs: 110_000,
};
cases["geneva-southbound-exit"] = {
  owners: genevaOwners,
  roads: ["i280-south-ocean-approach", "i280-south-geneva-exit"],
  rollingStart: true,
  rollingStartMetres: 190,
  speed: 11,
  maxDurationMs: 95_000,
};
cases["geneva-southbound-entry"] = {
  owners: genevaOwners,
  roads: ["i280-south-geneva-entry", "i280-south-beyond-geneva"],
  groundApproachMetres: 3,
  finishOnDeck: true,
  finishDistance: 130,
  speed: 12,
  maxDurationMs: 95_000,
};
cases["geneva-northbound-entry"] = {
  owners: genevaOwners,
  roads: [
    "i280-north-geneva-entry",
    "i280-north-ocean-connector",
    "i280-north-ocean-mainline",
  ],
  groundApproachMetres: 3,
  finishOnDeck: true,
  finishDistance: 125,
  speed: 12,
  maxDurationMs: 95_000,
};
cases["geneva-northbound-exit"] = {
  owners: genevaOwners,
  roads: ["i280-north-geneva-approach", "i280-north-geneva-exit"],
  rollingStart: true,
  rollingStartMetres: 3,
  initialSpeedMps: 10,
  forks: [["i280-north-geneva-approach", "i280-north-geneva-exit", -3.1]],
  speed: 12,
  maxDurationMs: 90_000,
};
cases["geneva-northbound-exit-rolling"] = {
  ...cases["geneva-northbound-exit"],
  rollingStart: true,
  initialSpeedMps: 50 * 0.44704,
  rollingStartMetres: 5,
};
cases["geneva-exit-and-reentry"] = {
  owners: genevaOwners,
  roads: [
    "i280-south-ocean-approach",
    "i280-south-geneva-exit",
    "geneva-avenue-eastbound-over-i280",
    "i280-north-geneva-entry",
    "i280-north-ocean-connector",
    "i280-north-ocean-mainline",
  ],
  rollingStart: true,
  rollingStartMetres: 115,
  connectByGround: true,
  groundConnection: [],
  groundedHandoff: true,
  finishOnDeck: true,
  finishDistance: 125,
  speed: 12,
  maxDurationMs: 165_000,
  speedLimits: {
    "i280-south-geneva-exit": 4.6,
    "geneva-avenue-eastbound-over-i280": 7,
    "i280-north-geneva-entry": 9,
  },
};
cases["geneva-westbound-exit-and-reentry"] = {
  ...cases["geneva-northbound-exit"],
  roads: [
    "i280-north-geneva-approach",
    "i280-north-geneva-exit",
    "geneva-avenue-westbound-over-i280",
    "i280-south-geneva-entry",
    "i280-south-beyond-geneva",
  ],
  connectByGround: true,
  groundConnection: [],
  groundedHandoff: true,
  finishOnDeck: true,
  finishDistance: 115,
  speedLimits: {
    "i280-north-geneva-exit": 5,
    "geneva-avenue-westbound-over-i280": 7,
    "i280-south-geneva-entry": 9,
  },
  maxDurationMs: 145_000,
};
for (const reverse of [false, true]) {
  cases[`congdon-${reverse ? "northbound" : "southbound"}`] = {
    roads: ["justin-congdon-over-i280"],
    owners: ["808f7e64", "808f7e8c"],
    reverse,
    speed: 6.8,
    groundApproachMetres: 7,
    connectByGround: true,
    groundDeparture: reverse
      ? [[1271.7, -3104, 36.3]]
      : [[1221.4, -3221.3, 36.1]],
    maxApproachClearanceMetres: 0.18,
    maxDepartureClearanceMetres: 0.18,
    maxDurationMs: 70_000,
  };
}
for (const [direction, road, start, finish, exitCamera] of [
  ["northbound", "i280-north-monterey-to-alemany", 450, 650, "covered"],
  ["southbound", "i280-south-beyond-alemany", 65, 255, "exterior"],
]) {
  cases[`i280-under-congdon-${direction}`] = {
    roads: [road],
    owners: ["808f7e64", "808f7e8c", "808f7ef4"],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 16,
    finishOnDeck: true,
    finishDistance: finish,
    exitCamera,
    speed: 16,
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}
for (const [name, road, owner, start, finish, exitCamera = "exterior"] of [
  ["presidio-marina", "presidio-marina-northbound-entry", "808586d4", 187, 248],
  [
    "tenth-bryant",
    "us101-south-tenth-bryant-entry",
    "808f7e2c",
    112,
    176,
    "covered",
  ],
  ["i280-sixth", "i280-sixth-northbound", "808f7fd4", 657, 724],
]) {
  cases[`${name}-deck-tree-clearance`] = {
    roads: [road],
    owners: [owner],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 10,
    finishOnDeck: true,
    finishDistance: finish,
    exitCamera,
    speed: 10,
    expectNoScrapes: true,
    maxAirborneMetres: 0,
    maxDurationMs: 40_000,
  };
}
for (const reverse of [false, true]) {
  cases[`blanken-underpass-${reverse ? "westbound" : "eastbound"}`] = {
    owners: ["808f7f2c"],
    roads: ["blanken-avenue-underpass"],
    reverse,
    groundApproachMetres: 7,
    speed: 9,
    maxDurationMs: 40_000,
  };
}
for (const [direction, road, start, finish] of [
  ["northbound", "us101-north-city-edge-to-hester", 250, 380],
  ["southbound", "us101-south-hester-to-city-edge", 460, 585],
]) {
  cases[`blanken-us101-${direction}`] = {
    owners: ["808f7f2c", "808f7f24"],
    roads: [road],
    rollingStart: true,
    rollingStartMetres: start,
    finishOnDeck: true,
    finishDistance: finish,
    speed: 19,
    maxDurationMs: 35_000,
  };
}
for (const [road, owners, speed, offset] of [
  ["crossover-drive-over-jfk", ["8085876c"], 16, 2],
  ["islais-creek-illinois-street", ["808f7fa4"], 11, 1.35],
  ["mission-creek-fourth-street", ["808f7fd4"], 11, 1.8],
]) {
  for (const reverse of [false, true]) {
    cases[`${road}-${reverse ? "northbound" : "southbound"}`] = {
      owners,
      roads: [road],
      speed,
      reverse,
      laneGuides: { [road]: { lane: reverse ? offset : -offset } },
      groundApproachMetres: 9,
      maxAirborneMetres: 0,
      maxDurationMs: 60_000,
    };
  }
}
const islaisThird = "islais-creek-third-street";
for (const [direction, reverse, inside, outside] of [
  ["northbound", true, 10.4, 14.15],
  ["southbound", false, -2.1, -5.5],
]) {
  for (const [suffix, lane] of [
    ["", inside],
    ["-outside-lane", outside],
  ]) {
    cases[`${islaisThird}-${direction}${suffix}`] = {
      owners: ["808f7fa4"],
      roads: [islaisThird],
      speed: 12,
      reverse,
      laneGuides: { [islaisThird]: { lane } },
      groundApproachMetres: 12,
      maxAirborneMetres: 0,
      maxDurationMs: 60_000,
    };
  }
}
for (const [direction, reverse] of [
  ["northbound", false],
  ["southbound", true],
]) {
  const road = "fillmore-geary-bridge";
  cases[`${road}-${direction}`] = {
    owners: ["808580bc"],
    roads: [road],
    speed: 11,
    reverse,
    laneGuides: { [road]: { lane: reverse ? 1.8 : -1.8 } },
    groundApproachMetres: 9,
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}
for (const [direction, reverse] of [
  ["eastbound", true],
  ["westbound", false],
]) {
  const road = `geary-under-fillmore-${direction}`;
  cases[road] = {
    owners: ["808580bc"],
    roads: [road],
    speed: 14,
    reverse,
    groundApproachMetres: 12,
    covered: [prefix + road],
    maxApproachClearanceMetres: 0.18,
    maxAirborneMetresAfterEntrance: 0,
    maxDurationMs: 65_000,
  };
}
for (const [direction, reverse] of [
  ["eastbound", true],
  ["westbound", false],
]) {
  const road = `geary-fillmore-${direction}-frontage`;
  cases[road] = {
    owners: ["808580bc"],
    roads: [road],
    speed: 9,
    reverse,
    groundApproachMetres: 10,
    maxApproachClearanceMetres: 0.18,
    maxAirborneMetresAfterEntrance: 0,
    maxDurationMs: 65_000,
  };
}
for (const road of [
  "williams-avenue-caltrain-bridge",
  "paul-avenue-caltrain-bridge",
]) {
  for (const reverse of [false, true]) {
    cases[`${road}-${reverse ? "eastbound" : "westbound"}`] = {
      owners: ["808f7f1c"],
      roads: [road],
      speed: 11,
      reverse,
      laneGuides: { [road]: { lane: reverse ? 1.8 : -1.8 } },
      groundApproachMetres: 10,
      maxApproachClearanceMetres: 0.18,
      maxAirborneMetresAfterEntrance: 0,
      maxDurationMs: 45_000,
    };
  }
}
for (const [crossing, owner, westOrSouth, eastOrNorth] of [
  ["lincoln-way-sunset", "8085879c", "westbound", "eastbound"],
  ["sloat-boulevard-sunset", "808f7d74", "westbound", "eastbound"],
  ["junipero-serra-brotherhood", "808f7c4c", "southbound", "northbound"],
]) {
  for (const [direction, reverse] of [
    [westOrSouth, false],
    [eastOrNorth, true],
  ]) {
    const road = `${crossing}-${direction}`;
    cases[road] = {
      owners: [owner],
      roads: [road],
      speed: 11,
      reverse,
      laneGuides: { [road]: { lane: reverse ? 1.6 : -1.6 } },
      groundApproachMetres: 7,
      maxApproachClearanceMetres: 0.18,
      maxAirborneMetresAfterEntrance: 0,
      maxDurationMs: 45_000,
    };
  }
}
for (const [road, owner, direction, opposite, approach, offset] of [
  [
    "oakdale-avenue-caltrain-bridge",
    "808f7f04",
    "westbound",
    "eastbound",
    8,
    1.6,
  ],
  [
    "mission-street-i280-viaduct",
    "808f7e64",
    "southbound",
    "northbound",
    8,
    1.7,
  ],
  [
    "baden-street-i280-bridge",
    "808f7e7c",
    "northwestbound",
    "southeastbound",
    3,
    1.4,
  ],
  ["paulding-street-i280-bridge", "808f7e7c", "westbound", "eastbound", 6, 1.4],
]) {
  for (const reverse of [false, true]) {
    cases[`${road}-${reverse ? opposite : direction}`] = {
      owners: [owner],
      roads: [road],
      speed: 10.5,
      reverse,
      laneGuides: { [road]: { lane: reverse ? offset : -offset } },
      groundApproachMetres: approach,
      maxApproachClearanceMetres: 0.18,
      maxAirborneMetresAfterEntrance: 0,
      maxDurationMs: 55_000,
    };
  }
}
for (const [crossing, direction, road, owner, start, finish] of [
  [
    "mission",
    "northbound",
    "i280-north-monterey-to-alemany",
    "808f7e64",
    292,
    430,
  ],
  ["mission", "southbound", "i280-south-beyond-alemany", "808f7e64", 268, 405],
  ["baden", "northbound", "i280-north-ocean-monterey", "808f7e7c", 15, 140],
  ["baden", "southbound", "i280-south-monterey-mainline", "808f7e7c", 672, 782],
  ["paulding", "northbound", "i280-north-ocean-mainline", "808f7e7c", 310, 445],
  // The chase camera trails the car beneath the skewed deck on this departure.
  [
    "paulding",
    "southbound",
    "i280-south-monterey-to-ocean",
    "808f7e7c",
    280,
    476,
  ],
]) {
  cases[`i280-${direction}-under-${crossing}`] = {
    roads: [road],
    owners: [owner],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 10,
    finishOnDeck: true,
    finishDistance: finish,
    speed: 19,
    covered: [prefix + road],
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}
for (const [road, owners, forward, approach, departures] of [
  [
    "cortland-avenue-under-us101",
    ["808f7e54"],
    "eastbound",
    8,
    [[[2901, -2268, 6.563]], [[2747, -2253.3, 13.637]]],
  ],
  [
    "bacon-street-under-us101",
    ["808f7ee4"],
    "westbound",
    7,
    [
      [[3209.9, -3589.8, 20.2]],
      [
        [3323.2, -3556.1, 18.121],
        [3326.4, -3553.3, 18.116],
        [3329.5, -3548.55, 18.106],
        [3332.2, -3543.55, 18.045],
        [3333.25, -3540.85, 17.992],
      ],
    ],
  ],
  [
    "paul-avenue-under-us101",
    ["808f7ee4", "808f7f1c"],
    "westbound",
    3,
    [[[3347, -4002.64, 21.416]], [[3496, -4050.3, 20.925]]],
  ],
  [
    "dogpatch-23rd-lower",
    ["808f7fb4"],
    "eastbound",
    8,
    // The chase camera follows the car out from under the east entrance ramp.
    [[[4324, -529.7, 6.6]], [[4150, -543.02, 15.95]]],
  ],
  [
    "dogpatch-twenty-second-under-i280",
    ["808f7fb4"],
    "westbound",
    8,
    [[[4143, -256.8, 14.53]], [[4274, -249.85, 12.2]]],
  ],
]) {
  const opposite = forward === "eastbound" ? "westbound" : "eastbound";
  for (const [direction, reverse] of [
    [forward, false],
    [opposite, true],
  ]) {
    cases[`${road}-${direction}`] = {
      owners,
      roads: [road],
      reverse,
      speed: 10.5,
      laneGuides: { [road]: { lane: reverse ? 1.55 : -1.55 } },
      groundApproachMetres: reverse ? 8 : approach,
      connectByGround: true,
      groundDeparture: departures[Number(reverse)],
      covered: [prefix + road],
      maxApproachClearanceMetres: 0.18,
      maxDepartureClearanceMetres: 0.18,
      maxDurationMs: 60_000,
    };
  }
}
for (const [direction, road, start, finish] of [
  ["northbound", "i280-north-south-dogpatch", 1420, 1545],
  ["southbound", "i280-south-through-dogpatch", 85, 210],
]) {
  cases[`i280-${direction}-over-dogpatch-23rd`] = {
    owners: ["808f7fb4", "808f7fac"],
    roads: [road],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 12,
    speed: 17,
    finishOnDeck: true,
    finishDistance: finish,
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}
for (const reverse of [false, true]) {
  const road = "jfk-drive-under-crossover";
  cases[`${road}-${reverse ? "eastbound" : "westbound"}`] = {
    owners: ["8085876c"],
    roads: [road],
    speed: 11,
    reverse,
    laneGuides: { [road]: { lane: reverse ? 1.5 : -1.5 } },
    groundApproachMetres: 9,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [-3415.08, 1208.22, 74.48],
          [-3392.95, 1219.86, 75.31],
        ]
      : [
          [-3509.49, 1169.39, 70.06],
          [-3513.16, 1167.82, 69.85],
        ],
    covered: [prefix + road],
    maxAirborneMetres: 0,
    maxDurationMs: 45_000,
  };
}
for (const reverse of [false, true]) {
  const road = "lyell-street-under-i280";
  cases[`${road}-${reverse ? "southbound" : "northbound"}`] = {
    owners: ["808f7e64"],
    roads: [road],
    speed: 11,
    reverse,
    laneGuides: { [road]: { lane: reverse ? 1.5 : -1.5 } },
    groundApproachMetres: 6,
    connectByGround: true,
    groundDeparture: reverse
      ? [
          [748.564, -3134.009, 34.905],
          [753.181, -3135.93, 34.815],
          [758.73, -3138.24, 34.75],
        ]
      : [
          [716.232, -3027.95, 43.623],
          [715.195, -3022.041, 44.536],
          [713.122, -3010.221, 46.371],
        ],
    covered: [prefix + road],
    maxAirborneMetres: 0,
    maxDurationMs: 35_000,
  };
}
for (const reverse of [false, true]) {
  const bridge = "harrison-beale-bridge";
  cases[`${bridge}-${reverse ? "northeast" : "southwest"}bound`] = {
    owners: ["8085807c"],
    roads: [bridge],
    speed: 12,
    reverse,
    laneGuides: { [bridge]: { lane: reverse ? 1.4 : -1.4 } },
    groundApproachMetres: 8,
    exitCamera: reverse ? "covered" : "exterior",
    maxAirborneMetres: 0,
    maxDurationMs: 35_000,
  };
  const street = "beale-street-under-harrison";
  cases[`${street}-${reverse ? "northwest" : "southeast"}bound`] = {
    owners: ["8085807c"],
    roads: [street],
    speed: 11,
    reverse,
    laneGuides: { [street]: { lane: reverse ? 1.5 : -1.5 } },
    groundApproachMetres: 8,
    covered: [prefix + street],
    exitCamera: "covered",
    maxAirborneMetres: 0,
    maxDurationMs: 35_000,
  };
}
const mountVernonNorth = "san-jose-avenue-north-over-i280";
const mountVernonSouth = "san-jose-avenue-south-at-mount-vernon";
const mountVernonWest = "mount-vernon-avenue-over-i280";
const junction = (first, second) => `mount-vernon-junction-${first}-${second}`;
const northSouth = junction("north", "south");
const northWest = junction("north", "west");
const southWest = junction("south", "west");
const northDeparture = [
  [-698.5, -4556.1, 81.41],
  [-695.5, -4553.0, 81.17],
];
const southDeparture = [
  [-790.7, -4656.8, 88.7],
  [-793.6, -4662.2, 88.7],
];
const westDeparture = [
  [-791.5, -4595.2, 90.77],
  [-797.5, -4591.4, 91.65],
];
for (const [direction, roads, reverseRoads, groundDeparture, speed] of [
  [
    "southwestbound",
    [mountVernonNorth, northSouth, mountVernonSouth],
    [],
    southDeparture,
    6,
  ],
  [
    "northeastbound",
    [mountVernonSouth, northSouth, mountVernonNorth],
    [mountVernonSouth, northSouth, mountVernonNorth],
    northDeparture,
    6,
  ],
  [
    "westbound",
    [mountVernonNorth, northWest, mountVernonWest],
    [],
    westDeparture,
    4.5,
  ],
  [
    "eastbound",
    [mountVernonWest, northWest, mountVernonNorth],
    [mountVernonWest, northWest, mountVernonNorth],
    northDeparture,
    4.5,
  ],
  [
    "from-south",
    [mountVernonSouth, southWest, mountVernonWest],
    [mountVernonSouth],
    westDeparture,
    4,
  ],
  [
    "from-west",
    [mountVernonWest, southWest, mountVernonSouth],
    [mountVernonWest, southWest],
    southDeparture,
    4,
  ],
]) {
  cases[`mount-vernon-${direction}`] = {
    owners: ["808f7c2c"],
    roads,
    reverseRoads,
    turnEntryOffset: {
      westbound: -2.4,
      eastbound: 1.9,
      "from-south": 2.7,
      "from-west": -2.4,
    }[direction],
    speed,
    groundApproachMetres: 6,
    connectByGround: true,
    groundDeparture,
    maxDepartureClearanceMetres: 0.025,
    maxDurationMs: 55000,
  };
}
for (const [direction, road, start, finish, camera] of [
  ["northbound", "i280-north-sr82-to-geneva", 865, 938, "covered"],
  ["southbound", "i280-south-beyond-geneva", 7, 78, "covered"],
]) {
  cases[`i280-mount-vernon-${direction}`] = {
    owners: ["808f7c24", "808f7c2c", "808f7e84"],
    roads: [road],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 13,
    speed: 16,
    finishOnDeck: true,
    finishDistance: finish,
    exitCamera: camera,
    covered: [prefix + road],
    maxAirborneMetres: 0,
    maxDurationMs: 50000,
  };
}

for (const [name, road, reverse, groundDeparture, speed, covered] of [
  [
    "saint-charles-southbound",
    "saint-charles-avenue-over-i280",
    false,
    [
      [-2488.3, -5805.9, 82.68],
      [-2482.2, -5810.8, 82.69],
    ],
    8,
    false,
  ],
  [
    "saint-charles-northbound",
    "saint-charles-avenue-over-i280",
    true,
    [
      [-2580.0, -5701.9, 76.8],
      [-2580.5, -5695.8, 76.6],
    ],
    8,
    false,
  ],
  [
    "western-alemany-eastbound",
    "western-alemany-eastbound-under-i280",
    true,
    [
      [-1799.5, -5514.2, 89.34],
      [-1793, -5514.29, 89.5],
    ],
    7.5,
    true,
  ],
  [
    "western-alemany-westbound",
    "western-alemany-westbound-under-i280",
    false,
    [
      [-1873.6, -5464.9, 86.92],
      [-1879.1, -5461.9, 86.42],
    ],
    7.5,
    true,
  ],
]) {
  cases[name] = {
    owners: ["808f7c3c"],
    roads: [road],
    speed,
    reverse,
    groundApproachMetres: 6,
    connectByGround: true,
    groundDeparture,
    exitCamera: covered ? "covered" : "exterior",
    covered: covered ? [prefix + road] : [],
    maxApproachClearanceMetres: 0.025,
    maxDepartureClearanceMetres: 0.025,
    maxDurationMs: 55000,
  };
}
for (const [name, road, owners, exitCamera] of [
  [
    "broadway-west-ground-approach",
    "broadway-westbound",
    ["808580f4", "808580ec"],
    "tunnel",
  ],
  [
    "western-alemany-ground-approach",
    "western-alemany-westbound-under-i280",
    ["808f7c3c"],
    "covered",
  ],
  [
    "tenth-bryant-ground-approach",
    tenthBryant,
    ["808f7e2c", "80858084", "808f7e34"],
    "exterior",
  ],
]) {
  cases[name] = {
    roads: [road],
    owners,
    groundApproachMetres: road === tenthBryant ? 16 : 18,
    finishOnDeck: true,
    finishDistance: 60,
    speed: 8,
    exitCamera,
    expectNoScrapes: true,
    maxDurationMs: 45_000,
  };
}

cases["san-jose-tingley-ground-approach"] = {
  roads: ["san-jose-tingley-northbound"],
  owners: ["808f7e7c", "808f7e64"],
  groundApproachMetres: 0,
  groundArrival: [
    [358.2, -3269.9],
    [366.9, -3265.8],
    [382.6, -3255.1],
  ],
  covered: ["sf-contact/san-jose-tingley-northbound"],
  exitCamera: "covered",
  speed: 9,
  expectNoScrapes: true,
  groundedHandoff: true,
  maxAirborneMetres: 0,
  maxDurationMs: 65_000,
};
cases["san-jose-tingley-southbound"] = {
  roads: ["san-jose-tingley-southbound"],
  owners: ["808f7e7c", "808f7e64"],
  groundApproachMetres: 12,
  covered: ["sf-contact/san-jose-tingley-southbound"],
  speed: 9,
  expectNoScrapes: true,
  maxAirborneMetres: 0,
  maxDurationMs: 65_000,
};
for (const [name, road, start, finish, under] of [
  ["saint-charles-i280-northbound", "i280-north-edge-to-sr82", 111, 222, true],
  ["saint-charles-i280-southbound", "i280-south-sr82-to-edge", 420, 530, true],
  ["saint-charles-i280-south-exit", "i280-south-edge-exit", 433, 535, true],
  [
    "western-alemany-i280-northbound",
    "i280-north-edge-to-sr82",
    899,
    1008,
    false,
  ],
  [
    "western-alemany-i280-southbound",
    "i280-south-geneva-to-sr82",
    963,
    1066,
    false,
  ],
]) {
  cases[name] = {
    owners: ["808f7c24", "808f7c3c"],
    roads: [road],
    rollingStart: true,
    rollingStartMetres: start,
    initialSpeedMps: 13,
    speed: 16,
    finishOnDeck: true,
    finishDistance: finish,
    covered: under ? [prefix + road] : [],
    maxAirborneMetres: 0,
    maxDurationMs: 50000,
  };
}

const sr82Owners = ["808f7c24", "808f7c2c", "808f7c34", "808f7c3c", "808f7e84"];
const southGenevaTail = "i280-south-beyond-geneva";
const southSr82Main = "i280-south-geneva-to-sr82";
const southSr82Exit = "i280-south-sr82-exit";
const southEdgeExit = "i280-south-edge-exit";
const northSr82Main = "i280-north-sr82-to-geneva";
cases["i280-sr82-northbound-through"] = {
  owners: sr82Owners,
  roads: [
    "i280-north-edge-to-sr82",
    northSr82Main,
    "i280-north-geneva-approach",
    "i280-north-geneva-to-ocean",
  ],
  rollingStart: true,
  rollingStartMetres: 665,
  finishOnDeck: true,
  finishDistance: 130,
  speed: 20,
  maxDurationMs: 195_000,
};
cases["i280-sr82-southbound-through"] = {
  owners: sr82Owners,
  roads: [southGenevaTail, southSr82Main, "i280-south-sr82-to-edge"],
  rollingStart: true,
  rollingStartMetres: 475,
  speed: 20,
  finishOnDeck: true,
  finishDistance: 615,
  maxDurationMs: 195_000,
};
cases["i280-sr82-northbound-entry"] = {
  owners: sr82Owners,
  roads: ["i280-north-sr82-entry", northSr82Main],
  groundApproachMetres: 6,
  finishOnDeck: true,
  finishDistance: 150,
  speed: 15,
  maxDurationMs: 100_000,
};
cases["i280-sr82-southbound-exit"] = {
  owners: sr82Owners,
  roads: [southGenevaTail, southSr82Exit],
  forks: [[southGenevaTail, southSr82Exit, -3.1]],
  rollingStart: true,
  rollingStartMetres: 470,
  speed: 14,
  maxDurationMs: 100_000,
};
cases["i280-sr82-southbound-edge-exit"] = {
  owners: sr82Owners,
  roads: [southSr82Main, southEdgeExit],
  forks: [[southSr82Main, southEdgeExit, -3.1]],
  rollingStart: true,
  rollingStartMetres: 1220,
  speed: 17,
  finishOnDeck: true,
  finishDistance: 645,
  maxDurationMs: 100_000,
};
cases["i280-sr82-exit-and-reentry"] = {
  ...cases["i280-sr82-southbound-exit"],
  roads: [
    southGenevaTail,
    southSr82Exit,
    "i280-north-sr82-entry",
    northSr82Main,
  ],
  connectByGround: true,
  groundConnection: {
    after: `sf-contact/${southSr82Exit}`,
    minimumMetres: 390,
    via: [
      [-1374.3, -5385.7],
      [-1388.0, -5400.3],
      [-1392.5, -5410.0],
      [-1392.8, -5414.1],
      [-1385.8, -5421.5],
      [-1336.0, -5472.3],
      [-1311.9, -5496.5],
      [-1286.9, -5522.0],
      [-1285.3, -5526.8],
      [-1289.1, -5533.2],
      [-1388.5, -5632.5],
      [-1393.5, -5635.2],
      [-1399.2, -5631.7],
      [-1480.6, -5562.6],
      [-1499.5, -5543.1],
      [-1500.5, -5538.9],
      [-1497.9, -5535.3],
      [-1494.1, -5532.8],
    ],
  },
  finishOnDeck: true,
  finishDistance: 150,
  maxDurationMs: 245_000,
};
function fromGround(points, firstRoad) {
  const [start, turn, tangent] = points;
  const end = firstRoad.positions[0];
  const positions = Array.from({ length: 23 }, (_, index) => {
    const t = index / 23;
    const u = 1 - t;
    const [x, y] = start.map(
      (value, axis) =>
        u ** 3 * value +
        3 * u * u * t * turn[axis] +
        3 * u * t * t * tangent[axis] +
        t ** 3 * end[axis],
    );
    return [x, y, end[2]];
  });
  return {
    id: "between-streets",
    kind: "ground",
    width_m: 8,
    speed_mps: 4,
    positions: [...positions, end],
    sources: [],
  };
}

const mainPostNorth = "main-post-northbound";
const mainPostSouthWest = "main-post-southbound-west";
const northCauseway = "presidio-nb-causeway";
const southCauseway = "presidio-sb-causeway";
const gorgasInterior = "presidio-gorgas-southbound-covered-exit";
const gorgasRamp = "presidio-gorgas-southbound-exit";
const northMainPostCameras = [
  { x: -1100, y: 4825, surface: northCauseway, camera: "exterior" },
  { x: -1390, y: 4784, surface: mainPostNorth, camera: "tunnel" },
];
const southMainPostCameras = [
  { x: -1390, y: 4764, surface: mainPostSouthWest, camera: "tunnel" },
  { x: -1100, y: 4802, surface: southCauseway, camera: "exterior" },
];
for (const name of ["northbound", "presidio-doyle-northbound"]) {
  const route = cases[name];
  route.forBake = (has) =>
    has(northCauseway)
      ? {
          roads: route.roads.flatMap((id) =>
            id === mainPostNorth ? [northCauseway, id] : [id],
          ),
          cameraStations: northMainPostCameras,
        }
      : {};
}
for (const name of ["presidio-marina-to-doyle", "presidio-marina-to-sr1"]) {
  const route = cases[name];
  route.forBake = (has) =>
    has(northCauseway)
      ? {
          roads: route.roads.flatMap((id) =>
            id === "presidio-nb-between" ? [mainPostNorth, id] : [id],
          ),
          cameraStations: [northMainPostCameras[1]],
        }
      : {};
}
for (const name of ["southbound", "presidio-doyle-southbound"]) {
  const route = cases[name];
  route.forBake = (has) =>
    has(southCauseway)
      ? {
          roads: route.roads.flatMap((id) => {
            if (id === "main-post-sb-approach") return [mainPostSouthWest];
            if (id === "main-post-southbound") return [id, southCauseway];
            return [id];
          }),
          forks: [
            ...(route.forks ?? []),
            [mainPostSouthWest, "main-post-southbound", 1.4],
          ],
          cameraStations: southMainPostCameras,
        }
      : {};
}
cases["presidio-main-post-to-gorgas"] = {
  roads: ["presidio-sb-between", mainPostSouthWest, gorgasInterior, gorgasRamp],
  speed: 14,
  rollingStart: true,
  rollingStartMetres: 282,
  speedLimits: { [gorgasRamp]: 9 },
  forks: [[mainPostSouthWest, gorgasInterior, -1.45]],
  cameraStations: [
    southMainPostCameras[0],
    { x: -1100, y: 4753, surface: gorgasRamp, camera: "exterior" },
  ],
  expectNoScrapes: true,
  maxAirborneMetres: 0,
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

function withGroundConnections(roads, connection) {
  if (Array.isArray(connection))
    return connection.reduce(
      (sections, link, index) =>
        withGroundConnections(sections, {
          ...link,
          id: `between-streets-${index}`,
        }),
      roads,
    );
  if (connection) {
    const index = roads.findIndex(({ id }) => id === connection.after) + 1;
    assert.ok(
      index > 0 && index < roads.length,
      "Ground connection needs two roads",
    );
    const start = roads[index - 1].positions.at(-1);
    const end = roads[index].positions[0];
    const waypoints = [
      start,
      ...connection.via.map(([x, y], i) => [
        x,
        y,
        start[2] +
          (end[2] - start[2]) * ((i + 1) / (connection.via.length + 1)),
      ]),
      end,
    ];
    const positions = [start];
    for (const [i, to] of waypoints.slice(1).entries()) {
      const from = waypoints[i];
      const steps = Math.ceil(Math.hypot(to[0] - from[0], to[1] - from[1]) / 2);
      for (let step = 1; step <= steps; step++) {
        positions.push(
          from.map((value, j) => value + ((to[j] - value) * step) / steps),
        );
      }
    }
    return [
      ...roads.slice(0, index),
      {
        id: connection.id ?? "between-streets",
        kind: "ground",
        positions,
        width_m: 7,
        speed_mps: connection.speedMps ?? 8,
        sources: [],
      },
      ...roads.slice(index),
    ];
  }
  const extension = (from, toward, metres) => {
    const distance = Math.hypot(toward[0] - from[0], toward[1] - from[1]);
    return from.map(
      (value, i) => value + ((toward[i] - value) * metres) / distance,
    );
  };
  const ground = (id, from, to) => ({
    id,
    kind: "ground",
    width_m: 9,
    speed_mps: 13,
    positions: Array.from({ length: 13 }, (_, i) =>
      from.map((value, j) => value + ((to[j] - value) * i) / 12),
    ),
    sources: [],
  });
  const [start, next] = roads[0].positions;
  const [previous, end] = roads.at(-1).positions.slice(-2);
  return [
    ground("approach", extension(start, next, -14), start),
    ...roads.flatMap((road, i) => {
      const next = roads[i + 1];
      if (!next) return [road];
      const from = road.positions.at(-1);
      const to = next.positions[0];
      if (Math.hypot(from[0] - to[0], from[1] - to[1]) < 0.05) return [road];
      return [road, ground(`between-${i}`, from, to)];
    }),
    ground("departure", end, extension(end, previous, -16)),
  ];
}

async function drive(
  page,
  sections,
  speed,
  connectByGround,
  finishOnDeck,
  maxDurationMs,
  entranceTimeoutMs,
  groundTurn,
  sampleApproach,
  sampleDeparture,
  cameraStations,
) {
  return page.evaluate(
    async ({
      sections,
      speed,
      connectByGround,
      finishOnDeck,
      maxDurationMs,
      entranceTimeoutMs,
      groundTurn,
      sampleApproach,
      sampleDeparture,
      cameraStations,
    }) => {
      const driveModule = performance
        .getEntriesByType("resource")
        .find(({ name }) => name.endsWith("/sf-drive.js"));
      const { buildCityGameDrive, cityDriveControls } = await import(
        new URL("sf-city-road-drive.js", driveModule.name).href
      );
      const canvas = document.querySelector("#drive");
      const ids = sections
        .filter(({ kind }) => kind !== "ground")
        .map(({ id }) => id);
      const [beforePortal, portal] = sections
        .findLast(({ kind }) => kind !== "ground")
        .positions.slice(-2);
      const portalHeading = Math.atan2(
        portal[1] - beforePortal[1],
        portal[0] - beforePortal[0],
      );
      const portalEast = Math.cos(portalHeading);
      const portalNorth = Math.sin(portalHeading);
      const groundHints = new Map();
      let entered = 0;
      for (const { id, kind } of sections) {
        if (kind === "ground") groundHints.set(entered, id);
        else entered++;
      }
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

      let turnPhase = 0;
      let returnPoint = 0;
      let turnHeading;
      let reversedOnGround = false;
      // Use a three-point turn where the narrow island junction has no room
      // for a continuous steering arc. All phases operate the normal keys.
      function steerToward(x, y) {
        const rotation = Math.atan2(y - pose.y, x - pose.x) - pose.heading;
        const demand = Math.atan(
          (5.4 * Math.sin(rotation)) /
            Math.max(1, Math.hypot(x - pose.x, y - pose.y)),
        );
        return Math.abs(demand) > 0.18 * (0.58 / (1 + Math.abs(pose.v) / 20))
          ? Math.sign(demand)
          : 0;
      }
      function turnaround() {
        const { east, north, direction, entrance } = groundTurn;
        const angle =
          direction * (pose.heading - (turnHeading ?? pose.heading));
        let steer = direction;
        let gear = 1;
        let limit = 2.3;
        const braking = turnPhase === 2 || turnPhase === 4;
        if (turnPhase === 0) {
          steer = steerToward(east - 1, north);
          limit = pose.x > east + 6 ? 4 : 2;
          if (pose.x <= east) {
            turnHeading = pose.heading;
            turnPhase = 1;
          }
        } else if (turnPhase === 1 && angle >= 1.1) {
          turnPhase = 2;
        } else if (turnPhase === 2 && Math.abs(pose.v) < 0.12) {
          turnPhase = 3;
        } else if (turnPhase === 3) {
          steer = -direction;
          gear = -1;
          if (angle >= 2.05) turnPhase = 4;
        } else if (turnPhase === 4) {
          steer = -direction;
          if (Math.abs(pose.v) < 0.12) turnPhase = 5;
        } else if (turnPhase === 5) {
          limit = groundTurn.returnSpeedMps ?? limit;
          const points = groundTurn.returnVia ?? [];
          const [x, y] = points[returnPoint] ?? entrance;
          if (Math.hypot(pose.x - x, pose.y - y) < 1.6)
            returnPoint = Math.min(returnPoint + 1, points.length);
          steer = steerToward(...(points[returnPoint] ?? entrance));
        }
        const brake = braking || Math.abs(pose.v) > limit + 0.45;
        const moving = !brake && Math.abs(pose.v) < limit;
        key("ArrowLeft", steer > 0);
        key("ArrowRight", steer < 0);
        key("ArrowUp", moving && gear > 0);
        key("ArrowDown", moving && gear < 0);
        key("Space", brake);
        if (pose.v < -0.45 && pose.grounded) reversedOnGround = true;
      }

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
      const covered = new Set();
      const started = performance.now();
      let pose;
      let last = 0;
      let pulse = 0;
      let blocked = 0;
      let maxSpeed = 0;
      let lastSupported;
      let groundPose;
      let groundMetres = 0;
      let groundedMetres = 0;
      let airborneMetres = 0;
      let airborneMetresAfterEntrance = 0;
      let airborneMetresInCrossing = 0;
      let airborneBeforeLastRoad = false;
      const approachPoses = [];
      const departurePoses = [];
      const notices = new Set();
      const cameraObservations = new Map();
      const driveNotice = document.querySelector("#driveNotice");
      return new Promise((resolve) => {
        function finish(status) {
          for (const code of held) key(code, false);
          resolve({
            status,
            seen: [...seen.values()],
            enclosed: [...enclosed],
            covered: [...covered],
            exit: pose,
            lastSupported,
            groundMetres: Math.round(groundMetres * 10) / 10,
            groundedMetres: Math.round(groundedMetres * 10) / 10,
            airborneMetres: Math.round(airborneMetres * 100) / 100,
            airborneMetresAfterEntrance:
              Math.round(airborneMetresAfterEntrance * 100) / 100,
            ...(sampleDeparture && {
              airborneMetresInCrossing:
                Math.round(airborneMetresInCrossing * 100) / 100,
            }),
            airborneBeforeLastRoad,
            approachPoses,
            departurePoses,
            ...(notices.size && { scrapeNotices: [...notices] }),
            ...(cameraStations.length && {
              cameraObservations: [...cameraObservations.values()],
            }),
            ...(groundTurn && { reversedOnGround }),
            maxMph: Math.round(maxSpeed * 2.23694 * 10) / 10,
            seconds: Math.round((performance.now() - started) / 100) / 10,
          });
        }

        function tick(now) {
          const d = canvas.dataset;
          const notice = driveNotice?.textContent;
          if (notice?.startsWith("Scraped a ")) notices.add(notice);
          if (d.replay === "crash") return finish("crashed");
          const surfaceId = d.surfaceId || "";
          const previousPose = pose;
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
          if (previousPose && d.contact === "airborne") {
            const distance = Math.hypot(
              pose.x - previousPose.x,
              pose.y - previousPose.y,
            );
            airborneMetres += distance;
            if (seen.size) {
              airborneMetresAfterEntrance += distance;
              if (sampleDeparture) {
                const alongPortal =
                  (pose.x - portal[0]) * portalEast +
                  (pose.y - portal[1]) * portalNorth;
                // The tyre footprint can reach the ground while the car's
                // centre is still within the game's 3 m portal reach.
                if (seen.has(ids.at(-1)) && !surfaceId && alongPortal >= -3)
                  departurePoses.push(pose);
                else airborneMetresInCrossing += distance;
              }
            } else if (sampleApproach && !surfaceId) approachPoses.push(pose);
          }
          maxSpeed = Math.max(maxSpeed, pose.v);
          if (surfaceId && !ids.includes(surfaceId))
            return finish("wrong road");
          if (
            d.contact === "airborne" &&
            seen.size > 0 &&
            seen.size < ids.length
          )
            airborneBeforeLastRoad = true;
          if (surfaceId && !seen.has(surfaceId)) seen.set(surfaceId, pose);
          for (const [i, station] of cameraStations.entries()) {
            if (
              !cameraObservations.has(i) &&
              Math.hypot(pose.x - station.x, pose.y - station.y) < 8
            ) {
              cameraObservations.set(i, {
                surface: pose.surfaceId,
                camera: pose.camera,
                grounded: pose.grounded,
              });
            }
          }
          if (surfaceId && pose.grounded) lastSupported = pose;
          if (
            !surfaceId &&
            d.contact !== "loading" &&
            groundHints.get(seen.size)?.startsWith("between-streets")
          ) {
            if (groundPose) {
              const run = Math.hypot(
                pose.x - groundPose.x,
                pose.y - groundPose.y,
              );
              groundMetres += run;
              if (groundPose.grounded && pose.grounded) groundedMetres += run;
            }
            groundPose = pose;
          } else {
            groundPose = undefined;
          }
          if (surfaceId && pose.camera === "tunnel") enclosed.add(surfaceId);
          if (surfaceId && pose.grounded && pose.camera === "covered")
            covered.add(surfaceId);
          if (!seen.size && now - started > entranceTimeoutMs)
            return finish("missed entrance");
          const distanceToFinish = Math.hypot(pose.x - end[0], pose.y - end[1]);
          if (
            finishOnDeck &&
            surfaceId === ids.at(-1) &&
            pose.grounded &&
            distanceToFinish < 12
          )
            return finish("reached deck finish");
          const atFinish = !connectByGround || distanceToFinish < 4;
          if (!surfaceId && seen.has(ids.at(-1)) && pose.grounded && atFinish)
            return finish("left road");
          if (!connectByGround && !surfaceId && seen.size && pose.grounded)
            return finish("lost intermediate contact");
          blocked = d.roadBlocked === "true" ? blocked + 1 : 0;
          if (blocked >= 10) return finish("blocked");
          if (now - started > maxDurationMs) return finish("timed out");

          if (
            now - last >= 27 &&
            groundTurn &&
            !surfaceId &&
            groundHints.has(seen.size)
          ) {
            turnaround();
            last = now;
          } else if (now - last >= 27) {
            key("ArrowDown", false);
            const input = cityDriveControls(course, {
              ...pose,
              surfaceId:
                surfaceId ||
                groundHints.get(seen.size) ||
                ids[Math.min(seen.size, ids.length - 1)],
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
                (connectByGround || seen.size > 0 || pose.v < approachLimit),
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
    {
      sections,
      speed,
      connectByGround,
      finishOnDeck,
      maxDurationMs,
      entranceTimeoutMs,
      groundTurn,
      sampleApproach,
      sampleDeparture,
      cameraStations,
    },
  );
}

async function main() {
  const { steeringLine } = await import("./steering-line.mjs");
  const { groundAtWorld } =
    await import("../../public_html/src/js/sf-drive-core.js");
  const names = process.argv.length > 2 ? process.argv.slice(2) : defaultCases;
  for (const name of names)
    assert.ok(Object.hasOwn(cases, name), `Unknown case ${name}`);
  const tokens = [
    ...new Set(names.flatMap((name) => cases[name].owners ?? owners)),
  ];
  const patchedPaths = [
    "tiles/manifest.json",
    "quest-route.json",
    ...(names.some((name) => cases[name].startPlace)
      ? ["gamedrives.json"]
      : []),
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
  const tiles = tokens.map((token) =>
    JSON.parse(gunzipSync(snapshots.get(tilePath(token)))),
  );
  for (const tile of tiles) {
    for (const road of tile.playableRoads ?? []) roads.set(road.id, road);
  }

  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const name of names) {
      const definition = cases[name];
      const {
        roads: slugs,
        speed,
        reverse,
        rampApproach,
        connectByGround = false,
        groundConnection,
        groundDeparture,
        groundArrival,
        groundedHandoff = false,
        finishOnDeck = false,
        finishDistance,
        maxDurationMs = 460_000,
        maxAirborneMetres,
        maxAirborneMetresAfterEntrance,
        maxApproachClearanceMetres,
        maxDepartureClearanceMetres,
        entranceTimeoutMs = 20_000,
        rollingStart = false,
        initialSpeedMps = 50 * 0.44704,
        rollingStartMetres,
        startPlace,
        groundApproachMetres,
        reverseRoads = [],
        laneGuides = {},
        turnEntryOffset,
        speedLimits = {},
        forks = [],
        covered,
        allowedCovered = [],
        cameraStations = [],
      } = {
        ...definition,
        ...definition.forBake?.((slug) => roads.has(prefix + slug)),
      };
      const guides = { ...laneGuides };
      for (const [outgoing, incoming, offset] of forks) {
        const road = roads.get(prefix + outgoing) ?? roads.get(outgoing);
        const [a, b] = road.points.slice(-2);
        const heading = Math.atan2(b[1] - a[1], b[0] - a[0]);
        guides[outgoing] = {
          ...guides[outgoing],
          approach: offset,
          holdApproach: true,
        };
        guides[incoming] = {
          ...guides[incoming],
          entranceEast: -offset * Math.sin(heading),
          entranceNorth: offset * Math.cos(heading),
          entranceMetres: guides[incoming]?.entranceMetres ?? 21,
        };
      }
      const contactSections = slugs.map((slug) => {
        const road = roads.get(prefix + slug) ?? roads.get(slug);
        assert.ok(road, slug);
        const guide = guides[slug];
        const positions = guide
          ? steeringLine(road.points, guide)
          : driveLine(road, rampApproach);
        return {
          id: road.id,
          kind: road.kind,
          width_m: road.width,
          positions:
            reverse || reverseRoads.includes(slug)
              ? positions.toReversed()
              : positions,
          speed_mps: speedLimits[slug] ?? speed,
          game_clearance_m: road.game_clearance_m,
          sources: [],
        };
      });
      if (turnEntryOffset) {
        // Move into the turning lane before the seam, then rejoin the curve.
        const [first, second] = contactSections;
        const [previous, boundary] = first.positions.slice(-2);
        const heading = Math.atan2(
          boundary[1] - previous[1],
          boundary[0] - previous[0],
        );
        const dx = -Math.sin(heading) * turnEntryOffset;
        const dy = Math.cos(heading) * turnEntryOffset;
        for (const section of [first, second]) {
          const start = section.positions[0];
          const end = section.positions.at(-1);
          const limit = Math.min(
            10,
            0.8 * Math.hypot(end[0] - start[0], end[1] - start[1]),
          );
          section.positions = section.positions.map(([x, y, z]) => {
            const scale = Math.max(
              0,
              1 - Math.hypot(x - boundary[0], y - boundary[1]) / limit,
            );
            return [x + scale * dx, y + scale * dy, z];
          });
        }
      }
      if (finishDistance) {
        const last = contactSections.at(-1);
        let distance = 0;
        const finish = last.positions.findIndex(([x, y], i) => {
          if (i === 0) return false;
          const [a, b] = last.positions[i - 1];
          distance += Math.hypot(x - a, y - b);
          return distance >= finishDistance;
        });
        assert.ok(finish > 0, "Deck finish must be inside the last section");
        last.positions = last.positions.slice(0, finish + 1);
      }
      const departureStart = contactSections.at(-1).positions.at(-1);
      const departureTail =
        groundDeparture?.length === 1
          ? [
              departureStart.map(
                (value, axis) => (value + groundDeparture[0][axis]) / 2,
              ),
              ...groundDeparture,
            ]
          : groundDeparture;
      let sections = groundDeparture
        ? [
            ...contactSections,
            {
              id: "departure",
              kind: "ground",
              width_m: 8,
              speed_mps: 8,
              positions: [departureStart, ...departureTail],
              sources: [],
            },
          ]
        : connectByGround
          ? withGroundConnections(contactSections, groundConnection)
          : contactSections;
      if (groundArrival)
        sections = [fromGround(groundArrival, sections[0]), ...sections];
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
      const first = sections[0].positions;
      let startIndex = rollingStart && !startPlace ? 2 : 0;
      if (rollingStart && rollingStartMetres) {
        let station = 0;
        startIndex = first.findIndex(([x, y], i) => {
          if (i === 0) return false;
          station += Math.hypot(x - first[i - 1][0], y - first[i - 1][1]);
          return station >= rollingStartMetres;
        });
      }
      assert.ok(startIndex >= 0 && startIndex < first.length - 1);
      const [start, next] = first.slice(startIndex, startIndex + 2);
      const heading = Math.atan2(next[1] - start[1], next[0] - start[0]);
      const approach = rollingStart
        ? 0
        : (groundApproachMetres ?? (speed > 8 ? 11 : 7));
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
      const externalRoadRequests = new Set();
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("request", (request) => {
        const url = new URL(request.url());
        if (
          url.hostname.split(".")[0] === "arrow-worker" ||
          url.pathname.startsWith("/city-tiles/")
        )
          externalRoadRequests.add(url.href);
      });
      try {
        const place =
          rollingStart && !startPlace
            ? {
                ...checkpoint,
                id: "sample-upper-deck",
                name: "Upper deck test start",
                surfaceId: sections[0].id,
                initialSpeed: initialSpeedMps,
              }
            : null;
        if (candidate || place)
          await page.route("**/sf/**", (route) => {
            const path = new URL(route.request().url()).pathname.slice(4);
            if (!snapshots.has(path)) return route.continue();
            // Seed the baked surface with a temporary start and exercise its normal physics.
            const body =
              place && path === "tiles/manifest.json"
                ? JSON.stringify({
                    ...manifest,
                    places: [place, ...manifest.places],
                  })
                : snapshots.get(path);
            return route.fulfill({
              body,
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
        await page.goto(
          `${origin}/sf/?at=${startPlace ?? place?.id ?? "downtown"}`,
        );
        const canvas = page.locator('#drive[data-contact="grounded"]');
        await canvas.waitFor({ timeout: 60_000 });
        const initial = await canvas.evaluate((el) => ({ ...el.dataset }));
        assert.ok(
          Math.hypot(
            +initial.east - checkpoint.x,
            +initial.north - checkpoint.y,
          ) < (place || startPlace ? 24 : 1),
          `${name} started at (${initial.east}, ${initial.north}) instead of (${checkpoint.x}, ${checkpoint.y})`,
        );
        if (place) assert.equal(initial.surfaceId, place.surfaceId);
        if (startPlace) {
          assert.equal(initial.surfaceId, sections[0].id);
          assert.ok(Number(initial.speedMps) >= initialSpeedMps * 0.9);
        }
        const contactHash = await page.evaluate(async () => {
          const response = await fetch("/sf/tiles/manifest.json");
          return (await response.json()).roadBake.playableSha256;
        });
        assert.equal(contactHash, manifest.roadBake.playableSha256);
        await page.bringToFront();
        await canvas.focus();
        await confirmManualTakeover(page);
        const turn = groundConnection?.turn;
        let groundTurn;
        if (turn) {
          const street = sections.findIndex(
            ({ id }) => id === "between-streets",
          );
          const [start, next] = sections[street + 1].positions;
          const length = Math.hypot(next[0] - start[0], next[1] - start[1]);
          groundTurn = {
            ...turn,
            entrance: start
              .slice(0, 2)
              .map(
                (value, axis) => value + (6 * (next[axis] - value)) / length,
              ),
          };
        }
        const sampleApproach = maxApproachClearanceMetres !== undefined;
        const sampleDeparture = maxDepartureClearanceMetres !== undefined;
        const { approachPoses, departurePoses, ...result } = await drive(
          page,
          sections,
          speed,
          connectByGround,
          finishOnDeck,
          maxDurationMs,
          entranceTimeoutMs,
          groundTurn,
          sampleApproach,
          sampleDeparture,
          cameraStations,
        );
        const clearance = (poses) =>
          Math.max(
            0,
            ...poses.map(({ x, y, z }) => {
              const ground = groundAtWorld(tiles, x, y, true);
              assert.notEqual(
                ground,
                null,
                "Missing ground outside the portal",
              );
              return z - ground;
            }),
          );
        if (sampleApproach) {
          result.approachClearanceMetres = clearance(approachPoses);
          assert.ok(
            result.approachClearanceMetres <= maxApproachClearanceMetres,
            `Excessive clearance on the ground approach: ${result.approachClearanceMetres} m`,
          );
        }
        if (sampleDeparture) {
          result.departureClearanceMetres = clearance(departurePoses);
        }
        console.log(
          JSON.stringify({
            name,
            contactHash,
            ...result,
            errors,
            externalRoadRequests: [...externalRoadRequests],
          }),
        );
        assert.deepEqual(errors, []);
        assert.deepEqual([...externalRoadRequests], []);
        if (sampleDeparture) {
          assert.equal(result.airborneMetresInCrossing, 0);
          assert.ok(
            result.departureClearanceMetres <= maxDepartureClearanceMetres,
            `Excessive clearance on the ground departure: ${result.departureClearanceMetres} m`,
          );
        }
        if (turn) assert.ok(result.reversedOnGround);
        assert.deepEqual(
          result.seen.map(({ surfaceId }) => surfaceId),
          contactSections.map(({ id }) => id),
        );
        assert.ok(result.seen.every(({ grounded }) => grounded));
        if (cameraStations.length)
          assert.deepEqual(
            result.cameraObservations,
            cameraStations.map(({ surface, camera }) => ({
              surface: prefix + surface,
              camera,
              grounded: true,
            })),
          );
        assert.deepEqual(
          result.enclosed,
          sections.filter(({ kind }) => kind === "tunnel").map(({ id }) => id),
        );
        if (covered) {
          assert.deepEqual(
            result.covered.filter((id) => !allowedCovered.includes(id)),
            covered,
          );
        }
        assert.equal(
          result.status,
          finishOnDeck ? "reached deck finish" : "left road",
        );
        const last = sections.at(-1).positions.at(-1);
        assert.ok(
          Math.hypot(result.exit.x - last[0], result.exit.y - last[1]) <
            (finishOnDeck ? 12 : 6),
        );
        assert.equal(result.exit.camera, cases[name].exitCamera ?? "exterior");
        assert.ok(result.exit.grounded);
        if (cases[name].expectNoScrapes)
          assert.equal(
            result.scrapeNotices,
            undefined,
            "Scraped scenery on the deck",
          );
        if (groundedHandoff)
          assert.equal(
            result.airborneBeforeLastRoad,
            false,
            "Lost ground contact before entering the next road",
          );
        if (maxAirborneMetres !== undefined) {
          assert.ok(
            result.airborneMetres <= maxAirborneMetres,
            `Excessive airborne travel: ${result.airborneMetres} m`,
          );
        }
        if (maxAirborneMetresAfterEntrance !== undefined) {
          assert.ok(
            result.airborneMetresAfterEntrance <=
              maxAirborneMetresAfterEntrance,
            `Excessive airborne travel after entering the road: ${result.airborneMetresAfterEntrance} m`,
          );
        }
        if (groundArrival) {
          assert.ok(result.groundMetres > 20);
          assert.ok(result.groundedMetres > 18);
          assert.ok(
            Math.abs(result.seen[0].z - contactSections[0].positions[0][2]) <
              0.15,
          );
        }
        if (Array.isArray(groundConnection)) {
          const minimum = groundConnection.reduce(
            (sum, link) => sum + (link.minimumMetres ?? 0),
            0,
          );
          assert.ok(
            result.groundedMetres >= minimum,
            "Incomplete drive on city pavement",
          );
        } else if (groundConnection) {
          const ground = sections.find(({ id }) => id === "between-streets");
          const [east, north] = ground.positions[0];
          const [endEast, endNorth] = ground.positions.at(-1);
          const distance = Math.hypot(endEast - east, endNorth - north);
          assert.ok(
            result.groundMetres >=
              (groundConnection.minimumMetres ?? distance - 6),
            "Incomplete drive on city pavement",
          );
          assert.ok(result.groundedMetres > 0, "No contact with city pavement");
        }
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
