import test from "node:test";
import assert from "node:assert/strict";
import { mountMapMode, readMapMode, MAP_MODE_KEY } from "../scene-map-mode.js";

test("Bike is the default; a saved Car choice survives navigation", () => {
  assert.equal(readMapMode(), "bike");
  assert.equal(readMapMode({ getItem: () => "invalid" }), "bike");
  assert.equal(readMapMode({ getItem: () => "car" }), "car");
  assert.equal(
    readMapMode({
      getItem() {
        throw Error();
      },
    }),
    "bike",
  );
});

test("mode switching reuses the overlay, preserves the map, and reports tile failures", () => {
  const layers = new Set();
  const handlers = {};
  const layer = {
    on(event, callback) {
      handlers[event] = callback;
    },
    addTo() {
      layers.add(layer);
    },
  };
  const map = {
    createPane: () => ({ style: {} }),
    hasLayer: (l) => layers.has(l),
    removeLayer: (l) => layers.delete(l),
  };
  const buttons = ["bike", "car"].map((mode) => ({
    dataset: { mapMode: mode },
    setAttribute(key, value) {
      this[key] = value;
    },
    addEventListener(event, handler) {
      this[event] = handler;
    },
  }));
  const status = {};
  const stored = new Map();
  const storage = {
    getItem: (k) => stored.get(k),
    setItem: (k, v) => stored.set(k, v),
  };
  let constructed = 0;
  mountMapMode({
    L: {
      tileLayer() {
        constructed++;
        return layer;
      },
    },
    map,
    controls: { querySelectorAll: () => buttons },
    status,
    storage,
  });
  assert.equal(layers.size, 1);
  assert.equal(buttons[0]["aria-pressed"], "true");
  handlers.tileerror();
  handlers.load();
  assert.match(status.textContent, /could not load/);
  buttons[1].click();
  assert.equal(layers.size, 0);
  assert.equal(stored.get(MAP_MODE_KEY), "car");
  assert.equal(buttons[0]["aria-pressed"], "false");
  assert.equal(status.textContent, "Street map");
  buttons[0].click();
  handlers.loading();
  handlers.load();
  assert.equal(layers.size, 1);
  assert.equal(constructed, 1);
  assert.match(status.textContent, /Cycling routes/);
});
