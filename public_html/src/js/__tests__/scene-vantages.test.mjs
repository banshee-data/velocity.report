// Guards that a scene's vantages have exactly one source.
//
// These read the viewer's own source rather than running it, because the thing
// being protected is an absence: no second place to read vantages from. The
// player needs a DOM and a GPU to run, but the invariant is structural, and a
// structural check is what catches a well-meaning fallback being added back.
//
// The bug this remembers: vantages lived in both the export header and a
// vantages.json, the player preferred one, and edits to the other looked like
// they did nothing at all.

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const src = (name) =>
  readFileSync(fileURLToPath(new URL(`../${name}`, import.meta.url)), "utf8");

const player = src("scene-player.js");
const camera = src("scene-camera.js");

describe("a scene's vantages have one source", () => {
  test("the player never reads vantages off a part header", () => {
    const offenders = [...player.matchAll(/^.*header\??\.\s*vantages.*$/gim)];
    assert.deepEqual(
      offenders.map((m) => m[0].trim()),
      [],
      "the export header is not a vantage store; vantages.json is the only one",
    );
  });

  test("the player fetches vantages from exactly one url", () => {
    const uses = [...player.matchAll(/ui\.vantages[A-Za-z]*/g)].map(
      (m) => m[0],
    );
    assert.deepEqual(
      [...new Set(uses)],
      ["ui.vantagesURL"],
      `expected one vantage source, found ${uses.join(", ")}`,
    );
  });

  test("no fallback chain picks between two lists", () => {
    // `a ?? b` over two vantage lists is precisely the precedence rule that
    // made an edited file lose to a stale one.
    assert.equal(
      /vantages\s*\?\?|\?\?\s*[A-Za-z.]*vantages/i.test(
        player.replace(/vantages\[0\]\?\.id/g, ""),
      ),
      false,
      "the player is choosing between two vantage lists again",
    );
  });

  test("the compass defaults live in one file", () => {
    assert.ok(
      camera.includes("export const DEFAULT_VANTAGES"),
      "DEFAULT_VANTAGES should be defined in scene-camera.js",
    );
    assert.equal(
      /DEFAULT_VANTAGES\s*=/.test(player),
      false,
      "the player should import the defaults, not define its own",
    );
  });
});

// Published assets, not just the code that writes them. An export made before
// the rule existed can still carry a second copy, and a stale copy in a file
// nobody reads is the same trap as a live one for whoever reads it next.
describe("published scene assets carry one vantage file", () => {
  const scenes = fileURLToPath(new URL("../../scenes", import.meta.url));

  /** Every .json under a directory, recursively.
   *
   * A scene directory exists from the moment its export is queued and gains
   * assets only when the export completes, so an unpublished scene is a normal
   * state rather than a fault — and reading it as one made this suite fail
   * whenever a batch was mid-run.
   */
  function jsonFiles(dir) {
    const out = [];
    let entries;
    try {
      entries = readdirSync(dir);
    } catch {
      return out; // nothing published here yet
    }
    for (const entry of entries) {
      const path = join(dir, entry);
      if (statSync(path).isDirectory()) out.push(...jsonFiles(path));
      else if (entry.endsWith(".json")) out.push(path);
    }
    return out;
  }

  test("only vantages.json mentions vantages", () => {
    const offenders = [];
    for (const scene of readdirSync(scenes)) {
      const assets = join(scenes, scene, "assets");
      for (const file of jsonFiles(assets)) {
        if (file.endsWith("vantages.json")) continue;
        if (readFileSync(file, "utf8").toLowerCase().includes("vantage")) {
          offenders.push(file.slice(scenes.length + 1));
        }
      }
    }
    assert.deepEqual(
      offenders,
      [],
      "these published files carry a second copy of a scene's vantages",
    );
  });

  test("a vantages.json is a list the viewer can use", () => {
    for (const scene of readdirSync(scenes)) {
      const file = join(scenes, scene, "assets", "vantages.json");
      // Vantages are hand-authored per scene and most scenes have none: the
      // viewer falls back to its own defaults. This checks the file is usable
      // where it exists, not that every scene has been framed by hand.
      if (!existsSync(file)) continue;
      const body = JSON.parse(readFileSync(file, "utf8"));
      const list = Array.isArray(body) ? body : body.vantages;
      assert.ok(list?.length, `${scene}: vantages.json names no vantages`);
      for (const v of list) {
        assert.match(v.id, /^[a-z0-9][a-z0-9_-]*$/, `${scene}: bad id ${v.id}`);
        assert.ok(v.label?.trim(), `${scene}: ${v.id} has no label`);
        assert.ok(
          v.polar_deg >= 0 && v.polar_deg <= 90,
          `${scene}: ${v.id} angle ${v.polar_deg} is not between 0 and 90`,
        );
        assert.ok(
          v.zoom > 0 && v.zoom <= 4,
          `${scene}: ${v.id} zoom ${v.zoom}`,
        );
      }
    }
  });
});
