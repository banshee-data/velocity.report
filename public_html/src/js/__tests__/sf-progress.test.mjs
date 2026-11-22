import assert from "node:assert/strict";
import { test } from "node:test";
import {
  bestAirtimes,
  readProgress,
  restoreTrial,
  writeProgress,
} from "../sf-progress.js";

test("only the three longest valid landings survive a reload", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key),
    setItem: (key, value) => values.set(key, value),
  };
  const key = "airtimes";
  for (const seconds of [1, 3, -1, 4, 2]) {
    writeProgress(
      key,
      bestAirtimes(readProgress(key, storage), { seconds, peak: 2 }),
      storage,
    );
  }
  assert.deepEqual(readProgress(key, storage), [
    { seconds: 4, peak: 2 },
    { seconds: 3, peak: 2 },
    { seconds: 2, peak: 2 },
  ]);
});

test("a saved trial restores unique eligible pickups and its safe checkpoint", () => {
  const trial = { targets: [{ t: "here" }, { t: "there" }], status: "ready" };
  const checkpoint = { x: 2, y: 5, heading: 1 };
  const saved = {
    collected: ["here", "here", "elsewhere"],
    elapsed: 30,
    crashes: 2,
    checkpoint,
  };
  assert.deepEqual(restoreTrial(trial, saved), checkpoint);
  assert.deepEqual([...trial.collected], ["here"]);
  assert.equal(trial.status, "running");
  assert.equal(trial.elapsed, 30);
  assert.equal(trial.crashes, 2);
  assert.equal(
    restoreTrial(trial, {
      ...saved,
      checkpoint: { ...checkpoint, heading: null },
      collected: ["here", "there"],
    }),
    null,
  );
  assert.equal(trial.status, "finished");
});
