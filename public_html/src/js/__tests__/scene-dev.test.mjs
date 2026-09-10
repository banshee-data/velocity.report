import test from "node:test";
import assert from "node:assert/strict";
import {
  DEV_STORAGE_KEY,
  resolveDevMode,
  parseAzimuth,
  updateMapMark,
} from "../scene-dev.js";

test("dev query persists across visits until explicitly disabled", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key),
    setItem: (key, value) => values.set(key, value),
  };
  assert.equal(resolveDevMode("", storage), false);
  assert.equal(resolveDevMode("?dev=true", storage), true);
  assert.equal(values.get(DEV_STORAGE_KEY), "true");
  assert.equal(resolveDevMode("?other=1", storage), true);
  assert.equal(resolveDevMode("?dev=invalid", storage), true);
  assert.equal(resolveDevMode("?dev=false", storage), false);
  assert.equal(resolveDevMode("", storage), false);
});

test("explicit dev query works even when storage is blocked", () => {
  const storage = {
    getItem() {
      throw Error();
    },
    setItem() {
      throw Error();
    },
  };
  assert.equal(resolveDevMode("?dev=true", storage), true);
  assert.equal(resolveDevMode("?dev=false", storage), false);
  assert.equal(resolveDevMode("", storage), false);
});

test("angles preserve unknown and zero and normalise wrapped measurements", () => {
  assert.equal(parseAzimuth(""), null);
  assert.equal(parseAzimuth(null), null);
  assert.equal(parseAzimuth("0"), 0);
  assert.equal(parseAzimuth("-90"), 270);
  assert.equal(parseAzimuth("450.5"), 90.5);
  assert.throws(() => parseAzimuth("NaN"));
  assert.throws(() => parseAzimuth(Infinity));
});

test("canonical save changes only the matching mark's angles", () => {
  const source = {
    comment: ["Keep provenance"],
    marks: [
      { id: "a", lat: 1, note: "Keep me", vantages: [1] },
      { id: "b", north_azimuth_deg: 20 },
    ],
  };
  const angles = { grid_azimuth_deg: 0, north_azimuth_deg: -45 };
  const saved = JSON.parse(updateMapMark(JSON.stringify(source), "a", angles));
  assert.deepEqual(saved, {
    ...source,
    marks: [
      { ...source.marks[0], grid_azimuth_deg: 0, north_azimuth_deg: 315 },
      source.marks[1],
    ],
  });
  assert.throws(() => updateMapMark(JSON.stringify(source), "missing", angles));
  assert.throws(() =>
    updateMapMark(
      JSON.stringify({ marks: [{ id: "a" }, { id: "a" }] }),
      "a",
      angles,
    ),
  );
  assert.throws(() =>
    updateMapMark(JSON.stringify(source), "a", {
      ...angles,
      north_azimuth_deg: Infinity,
    }),
  );
});

test("save action writes the selected canonical file and reports completion only after close", async () => {
  const { mountSceneDev } = await import("../scene-dev.js");
  const elements = new Map();
  for (const selector of [
    '[name="grid_azimuth_deg"]',
    '[name="north_azimuth_deg"]',
    "output",
    '[data-action="top"]',
    '[data-action="reset"]',
    '[data-action="save"]',
  ]) {
    elements.set(selector, {
      value: "",
      handlers: {},
      checkValidity: () => true,
      addEventListener(event, handler) {
        this.handlers[event] = handler;
      },
    });
  }
  const source = JSON.stringify({ marks: [{ id: "site", note: "preserved" }] });
  let written;
  let closed = false;
  const originalWindow = globalThis.window;
  globalThis.window = {
    location: { search: "?dev=true" },
    showOpenFilePicker: async () => [
      {
        name: "map-marks.json",
        getFile: async () => ({ text: async () => source }),
        createWritable: async () => ({
          write: async (text) => {
            written = text;
          },
          close: async () => {
            closed = true;
          },
        }),
      },
    ],
  };
  try {
    mountSceneDev({
      panel: { querySelector: (selector) => elements.get(selector) },
      siteId: "site",
      gridAzimuthDeg: 0,
      northAzimuthDeg: 90,
      onChange() {},
      onTop() {},
    });
    await elements.get('[data-action="save"]').handlers.click();
    assert.equal(closed, true);
    assert.deepEqual(JSON.parse(written).marks[0], {
      id: "site",
      note: "preserved",
      grid_azimuth_deg: 0,
      north_azimuth_deg: 90,
    });
    assert.match(elements.get("output").textContent, /^Saved site/);
    globalThis.window.showOpenFilePicker = async () => {
      throw Object.assign(new Error(), { name: "AbortError" });
    };
    await elements.get('[data-action="save"]').handlers.click();
    assert.match(elements.get("output").textContent, /cancelled; preview only/);
  } finally {
    if (originalWindow === undefined) delete globalThis.window;
    else globalThis.window = originalWindow;
  }
});
