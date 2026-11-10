const assert = require("node:assert/strict");
const path = require("node:path");
const test = require("node:test");

const buildDataPath = path.resolve(__dirname, "../src/_data/build.js");
const keys = [
  "VELOCITY_DOCS_VERSION",
  "VELOCITY_DOCS_GIT_SHA",
  "VELOCITY_DOCS_COMMIT_TIME",
];

function loadWith(env) {
  const saved = Object.fromEntries(keys.map((key) => [key, process.env[key]]));
  try {
    for (const key of keys) {
      if (env[key] === undefined) delete process.env[key];
      else process.env[key] = env[key];
    }
    delete require.cache[buildDataPath];
    return require(buildDataPath);
  } finally {
    for (const key of keys) {
      if (saved[key] === undefined) delete process.env[key];
      else process.env[key] = saved[key];
    }
    delete require.cache[buildDataPath];
  }
}

test("stamps the footer with the commit time it is given", () => {
  const build = loadWith({
    VELOCITY_DOCS_VERSION: "0.5.1",
    VELOCITY_DOCS_GIT_SHA: "0123456789abcdef0123",
    VELOCITY_DOCS_COMMIT_TIME: "2026-11-10T21:45:38Z",
  });
  assert.deepEqual(build, {
    version: "0.5.1",
    gitSHA: "0123456789abcdef0123",
    gitShort: "0123456789ab",
    commitTime: "2026-11-10T21:45:38Z",
  });
});

test("never falls back to the wall clock", () => {
  const before = loadWith({});
  const after = loadWith({});
  assert.equal(before.commitTime, "unknown");
  assert.deepEqual(before, after);
  assert.equal("buildTime" in before, false);
});
