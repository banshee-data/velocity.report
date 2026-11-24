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
const i280Approach = ["i280-fifth-south-entry", "i280-king-to-mariposa"];
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
for (const [name, exit] of [
  ["i280-pennsylvania", "i280-pennsylvania-exit"],
  ["i280-18th", "i280-18th-exit"],
]) {
  cases[name] = {
    roads: [...i280Approach, "i280-18th-off-ramp", exit],
    owners: i280Owners,
    speed: 12,
    rampApproach: true,
  };
}
cases["i280-mainline"] = {
  roads: [
    ...dogpatchApproach,
    "i280-south-through-dogpatch",
    "i280-south-beyond-dogpatch",
  ],
  owners: dogpatchOwners,
  speed: 22,
  speedLimits: { "i280-south-through-dogpatch": 16 },
  laneGuides: {
    "i280-dogpatch-approach": { approach: 1.25, holdApproach: true },
    "i280-south-through-dogpatch": { entranceEast: 1.25, entranceMetres: 20 },
    "i280-south-beyond-dogpatch": dogpatchClearance,
  },
};
cases["sixth-southbound"] = {
  roads: [
    "i280-sixth-southbound",
    "i280-king-to-mariposa",
    "i280-18th-off-ramp",
    "i280-18th-exit",
  ],
  owners: i280Owners,
  speed: 16,
  speedLimits: {
    "i280-king-to-mariposa": 14,
    "i280-18th-off-ramp": 12,
    "i280-18th-exit": 8,
  },
  rampApproach: true,
};
cases["sixth-northbound"] = {
  roads: [
    "eighteenth-east-viaduct",
    "i280-eighteenth-north-entry",
    "i280-mariposa-northbound",
    "i280-sixth-northbound",
  ],
  owners: i280Owners,
  speed: 16,
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
    "i280-sixth-northbound": {
      // Hold the clear lane past the two mapped trees near the descent.
      clearance: { offset: -1.8, station: 696, length: 29 },
    },
  },
};
for (const reverse of [false, true]) {
  const viaducts = ["eighteenth-east-viaduct", "eighteenth-west-viaduct"];
  cases[reverse ? "eighteenth-eastbound" : "eighteenth-westbound"] = {
    roads: reverse ? viaducts.toReversed() : viaducts,
    owners: i280Owners,
    speed: 8,
    reverse,
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
      "yerba-buena-lower-east-approach",
    ],
    owners: lowerBridgeOwners,
    speed: 22,
    speedLimits: { [entry]: 10, "yerba-buena-lower-tunnel": 17 },
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
cases["central-octavia-entry"] = {
  roads: ["central-freeway-octavia-entry", ...centralEast],
  owners: centralOwners,
  speed: 19,
  finishOnDeck: true,
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
    "yerba-buena-lower-east-approach",
  ],
  maxDurationMs: 660_000,
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
const upperFromIsland = [
  "yerba-buena-east-approach",
  "yerba-buena-upper-tunnel",
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
  forks: [["city-approach", "fremont-exit", -1.2]],
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
    ...cases["central-from-fourth"].forks,
  ],
  laneGuides: {
    ...cases["central-from-fourth"].laneGuides,
    "i80-westbound-upper-soma": {
      clearance: { offset: 2.2, station: 40, length: 28 },
    },
  },
};
cases["dogpatch-to-sixth-northbound"] = {
  roads: [
    "i280-dogpatch-east-north-entry",
    "i280-dogpatch-northbound",
    "i280-south-of-eighteenth-northbound",
    "i280-mariposa-northbound",
    "i280-sixth-northbound",
  ],
  owners: ["808f7fb4", "808f7fbc", "808f7fcc", "808f7fd4", "808f7fac"],
  speed: 17,
  forks: [["i280-mariposa-northbound", "i280-sixth-northbound", -1.4]],
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
  roads: [...dogpatchNorth.roads.slice(0, -1), "i280-north-fifth-exit"],
  forks: [["i280-mariposa-northbound", "i280-north-fifth-exit", 1.65]],
};
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
    speedLimits: { [entry]: 9, "i280-dogpatch-on-ramp": 13 },
    laneGuides: { "i280-south-beyond-dogpatch": dogpatchClearance },
  };
}
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
}
cases["branch-east-ramp"] = {
  roads: ["branch-east-off-ramp"],
  owners: ["808f7e7c", "808f7e64"],
  speed: 15,
};
cases["branch-east-upper"] = {
  roads: ["branch-east-i280-northbound"],
  owners: ["808f7e7c", "808f7e64"],
  speed: 18,
};
cases["branch-east-san-jose"] = {
  roads: ["branch-east-san-jose-viaduct"],
  owners: ["808f7e64"],
  speed: 15,
};
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
    roads: northbound ? [...kobbe.roads, ruckman] : [ruckman, ...kobbe.roads],
    owners: [...kobbe.owners, "808586dc"],
  };
}

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
        id: "between-streets",
        kind: "ground",
        positions,
        width_m: 7,
        speed_mps: 8,
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
      return next
        ? [
            road,
            ground(`between-${i}`, road.positions.at(-1), next.positions[0]),
          ]
        : [road];
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
) {
  return page.evaluate(
    async ({
      sections,
      speed,
      connectByGround,
      finishOnDeck,
      maxDurationMs,
      entranceTimeoutMs,
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
      const started = performance.now();
      let pose;
      let last = 0;
      let pulse = 0;
      let blocked = 0;
      let maxSpeed = 0;
      let lastSupported;
      return new Promise((resolve) => {
        function finish(status) {
          for (const code of held) key(code, false);
          resolve({
            status,
            seen: [...seen.values()],
            enclosed: [...enclosed],
            exit: pose,
            lastSupported,
            maxMph: Math.round(maxSpeed * 2.23694 * 10) / 10,
            seconds: Math.round((performance.now() - started) / 100) / 10,
          });
        }

        function tick(now) {
          const d = canvas.dataset;
          if (d.replay === "crash") return finish("crashed");
          const surfaceId = d.surfaceId || "";
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
          maxSpeed = Math.max(maxSpeed, pose.v);
          if (surfaceId && !ids.includes(surfaceId))
            return finish("wrong road");
          if (surfaceId && !seen.has(surfaceId)) seen.set(surfaceId, pose);
          if (surfaceId && pose.grounded) lastSupported = pose;
          if (surfaceId && pose.camera === "tunnel") enclosed.add(surfaceId);
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

          if (now - last >= 27) {
            const input = cityDriveControls(course, {
              ...pose,
              surfaceId: surfaceId || groundHints.get(seen.size) || ids[0],
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
    },
  );
}

async function main() {
  const { steeringLine } = await import("./steering-line.mjs");
  const names = process.argv.length > 2 ? process.argv.slice(2) : defaultCases;
  for (const name of names)
    assert.ok(Object.hasOwn(cases, name), `Unknown case ${name}`);
  const tokens = [
    ...new Set(names.flatMap((name) => cases[name].owners ?? owners)),
  ];
  const patchedPaths = [
    "tiles/manifest.json",
    "quest-route.json",
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
  for (const token of tokens) {
    const tile = JSON.parse(gunzipSync(snapshots.get(tilePath(token))));
    for (const road of tile.playableRoads ?? []) roads.set(road.id, road);
  }

  const browser = await chromium.launch({
    executablePath: process.env.CHROMIUM_PATH,
    args: ["--enable-unsafe-swiftshader"],
  });
  try {
    for (const name of names) {
      const {
        roads: slugs,
        speed,
        reverse,
        rampApproach,
        connectByGround = false,
        groundConnection,
        finishOnDeck = false,
        finishDistance,
        maxDurationMs = 460_000,
        entranceTimeoutMs = 20_000,
        rollingStart = false,
        reverseRoads = [],
        laneGuides = {},
        speedLimits = {},
        forks = [],
      } = cases[name];
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
      const sections = connectByGround
        ? withGroundConnections(contactSections, groundConnection)
        : contactSections;
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
      const [start, next] = rollingStart
        ? sections[0].positions.slice(2, 4)
        : sections[0].positions;
      const heading = Math.atan2(next[1] - start[1], next[0] - start[0]);
      const approach = rollingStart ? 0 : speed > 8 ? 11 : 7;
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
      page.on("pageerror", (error) => errors.push(error.message));
      try {
        const place = rollingStart
          ? {
              ...checkpoint,
              id: "sample-upper-deck",
              name: "Upper deck test start",
              surfaceId: sections[0].id,
              initialSpeed: 50 * 0.44704,
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
        await page.goto(`${origin}/sf/?at=${place?.id ?? "downtown"}`);
        const canvas = page.locator('#drive[data-contact="grounded"]');
        await canvas.waitFor({ timeout: 60_000 });
        const initial = await canvas.evaluate((el) => ({ ...el.dataset }));
        assert.ok(
          Math.hypot(
            +initial.east - checkpoint.x,
            +initial.north - checkpoint.y,
          ) < (place ? 24 : 1),
        );
        if (place) assert.equal(initial.surfaceId, place.surfaceId);
        const contactHash = await page.evaluate(async () => {
          const response = await fetch("/sf/tiles/manifest.json");
          return (await response.json()).roadBake.playableSha256;
        });
        assert.equal(contactHash, manifest.roadBake.playableSha256);
        await canvas.focus();
        await confirmManualTakeover(page);
        const result = await drive(
          page,
          sections,
          speed,
          connectByGround,
          finishOnDeck,
          maxDurationMs,
          entranceTimeoutMs,
        );
        console.log(JSON.stringify({ name, contactHash, ...result, errors }));
        assert.deepEqual(errors, []);
        assert.deepEqual(
          result.seen.map(({ surfaceId }) => surfaceId),
          contactSections.map(({ id }) => id),
        );
        assert.ok(result.seen.every(({ grounded }) => grounded));
        assert.deepEqual(
          result.enclosed,
          sections.filter(({ kind }) => kind === "tunnel").map(({ id }) => id),
        );
        assert.equal(
          result.status,
          finishOnDeck ? "reached deck finish" : "left road",
        );
        const last = sections.at(-1).positions.at(-1);
        assert.ok(
          Math.hypot(result.exit.x - last[0], result.exit.y - last[1]) <
            (finishOnDeck ? 12 : 6),
        );
        assert.equal(result.exit.camera, "exterior");
        assert.ok(result.exit.grounded);
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
