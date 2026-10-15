// The capture run's lifecycle with no browser: a launcher that fails, and a
// browser that fails while closing. Either way the local server must be
// closed, or the listening socket keeps the process alive after the error is
// printed and the command never exits.

import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { runCapture, CaptureError } from "../run-capture.mjs";

/** Listening TCP servers this process holds. */
function listeningServers() {
  return process
    .getActiveResourcesInfo()
    .filter((kind) => kind === "TCPServerWrap").length;
}

/**
 * The listening servers once a closed one has left the list: a handle is
 * still listed in the tick its close callback runs, and gone by the next
 * timer. One left open stays listed, and keeps the process alive.
 */
async function settledServers(expected) {
  for (let waited = 0; waited < 1000; waited += 10) {
    if (listeningServers() === expected) break;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  return listeningServers();
}

describe("runCapture lifecycle", () => {
  let dir;
  let recipePath;

  beforeEach(async () => {
    dir = await mkdtemp(path.join(os.tmpdir(), "scene-capture-test-"));
    const part = path.join(dir, "source", "part-000");
    await mkdir(part, { recursive: true });
    await writeFile(
      path.join(dir, "source", "manifest.json"),
      JSON.stringify({ version: 1, parts: [{ url: "./part-000/" }] }),
    );
    await writeFile(path.join(part, "header.json"), "{}");
    recipePath = path.join(dir, "recipe.json");
    await writeFile(
      recipePath,
      JSON.stringify({
        version: 1,
        source: "./source",
        frame: { frame_index: 0 },
        views: [
          {
            name: "plan",
            camera: { x: 0, y: -10, z: 50 },
            target: { x: 0, y: 0, z: 0 },
          },
        ],
        layers: { lidar: false, boxes: true, trails: true },
        output: { dir: "./out" },
      }),
    );
  });

  afterEach(async () => {
    await rm(dir, { recursive: true, force: true });
  });

  test("a browser that does not start closes the server and says how to install one", async () => {
    const before = listeningServers();
    const launchBrowser = async () => {
      throw new Error(
        "browserType.launch: Executable doesn't exist at /opt/pw/headless_shell\n" +
          "╔════════════════════════╗\n║ npx playwright install ║",
      );
    };

    await assert.rejects(runCapture(recipePath, { launchBrowser }), (err) => {
      assert.ok(
        err instanceof CaptureError,
        "a CaptureError, so the CLI prints one line",
      );
      assert.match(
        err.message,
        /did not start: browserType\.launch: Executable doesn't exist/,
      );
      assert.match(err.message, /make install-scene-capture-browser/);
      assert.doesNotMatch(err.message, /╔/);
      assert.match(err.cause.message, /npx playwright install/);
      return true;
    });
    assert.equal(await settledServers(before), before);
  });

  test("a browser that fails to close still lets the server close", async () => {
    const before = listeningServers();
    const launchBrowser = async () => ({
      page: {
        goto: async () => {
          throw new Error("navigation failed");
        },
      },
      version: "test",
      close: async () => {
        throw new Error("browser close failed");
      },
    });

    await assert.rejects(
      runCapture(recipePath, { launchBrowser }),
      /browser close failed/,
    );
    assert.equal(await settledServers(before), before);
  });
});
