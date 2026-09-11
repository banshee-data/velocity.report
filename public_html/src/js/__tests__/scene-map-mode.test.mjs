import test from "node:test";
import assert from "node:assert/strict";
import { gzipSync } from "node:zlib";
import { clipVoronoiCell, readGeoJSONResponse } from "../scene-map.js";
import {
  CYCLING_TILE_URL,
  CYCLE_MAP_CONSENT_KEY,
  CYCLE_MAP_ENABLED_KEY,
  mountCycleMapConsent,
} from "../scene-map-mode.js";

function control() {
  return {
    hidden: true,
    handlers: {},
    addEventListener(event, handler) {
      this.handlers[event] = handler;
    },
    setAttribute(name, value) {
      this[name] = value;
    },
    removeAttribute(name) {
      delete this[name];
    },
    focus() {
      this.focused = true;
    },
  };
}

function controls() {
  const close = control();
  const cancel = control();
  const allow = control();
  const modal = control();
  modal.querySelector = (selector) =>
    ({
      '[data-action="close"]': close,
      '[data-action="cancel"]': cancel,
      '[data-action="allow"]': allow,
    })[selector];
  return { allow, cancel, close, load: control(), modal };
}

function storage(initial = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem(key) {
      return values.get(key) ?? null;
    },
    setItem(key, value) {
      values.set(key, value);
    },
    value(key) {
      return values.get(key);
    },
  };
}

function mapFixture() {
  const layers = new Set();
  return {
    layers,
    map: {
      addLayer(layer) {
        layers.add(layer);
      },
      removeLayer(layer) {
        layers.delete(layer);
      },
    },
  };
}

function leafletFixture(map, onCreate = () => {}) {
  return {
    tileLayer(url, options) {
      onCreate(url, options);
      return {
        addTo() {
          map.addLayer(this);
          return this;
        },
      };
    },
  };
}

test("cancelling is remembered and does not reopen the consent modal", () => {
  const choice = storage();
  const first = controls();
  let calls = 0;
  const { map } = mapFixture();
  mountCycleMapConsent({
    L: leafletFixture(map, () => calls++),
    map,
    modal: first.modal,
    loadButton: first.load,
    storage: choice,
  });
  assert.equal(calls, 0);
  assert.equal(first.modal.hidden, false);
  first.cancel.handlers.click();
  assert.equal(choice.value(CYCLE_MAP_CONSENT_KEY), "cancelled");
  assert.equal(choice.value(CYCLE_MAP_ENABLED_KEY), "false");

  const revisit = controls();
  mountCycleMapConsent({
    L: leafletFixture(map, () => calls++),
    map,
    modal: revisit.modal,
    loadButton: revisit.load,
    storage: choice,
  });
  assert.equal(revisit.modal.hidden, true);
  assert.equal(revisit.load.hidden, false);
  assert.equal(revisit.load.textContent, "Load Cycle map");
  assert.equal(calls, 0);
  revisit.load.handlers.click();
  assert.equal(revisit.modal.hidden, false, "the button can reopen consent");
});

test("accepting persists an on/off Cycle map toggle", () => {
  const choice = storage();
  const ui = controls();
  const { map, layers } = mapFixture();
  const toggles = [];
  let calls = 0;
  const consent = mountCycleMapConsent({
    L: leafletFixture(map, (url, options) => {
      calls++;
      assert.equal(url, CYCLING_TILE_URL);
      assert.equal(options.pane, "basemap");
    }),
    map,
    modal: ui.modal,
    loadButton: ui.load,
    storage: choice,
    onToggle: (enabled) => toggles.push(enabled),
  });
  ui.allow.handlers.click();
  assert.equal(calls, 1);
  assert.equal(layers.size, 1);
  assert.equal(choice.value(CYCLE_MAP_CONSENT_KEY), "accepted");
  assert.equal(choice.value(CYCLE_MAP_ENABLED_KEY), "true");
  assert.equal(ui.load.hidden, false);
  assert.equal(ui.load.textContent, "Cycle map: on");
  assert.equal(ui.load["aria-checked"], "true");

  ui.load.handlers.click();
  assert.equal(consent.enabled, false);
  assert.equal(layers.size, 0);
  assert.equal(choice.value(CYCLE_MAP_ENABLED_KEY), "false");
  assert.equal(ui.load.textContent, "Cycle map: off");

  ui.load.handlers.click();
  assert.equal(consent.enabled, true);
  assert.equal(layers.size, 1);
  assert.equal(calls, 1, "switching back on reuses the existing tile layer");
  assert.deepEqual(toggles, [false, true, false, true]);
});

test("a remembered choice restores without showing the modal", () => {
  for (const remembered of ["true", "false"]) {
    const choice = storage({
      [CYCLE_MAP_CONSENT_KEY]: "accepted",
      [CYCLE_MAP_ENABLED_KEY]: remembered,
    });
    const ui = controls();
    const { map, layers } = mapFixture();
    let calls = 0;
    mountCycleMapConsent({
      L: leafletFixture(map, () => calls++),
      map,
      modal: ui.modal,
      loadButton: ui.load,
      storage: choice,
    });
    assert.equal(ui.modal.hidden, true);
    assert.equal(ui.load.hidden, false);
    assert.equal(calls, remembered === "true" ? 1 : 0);
    assert.equal(layers.size, remembered === "true" ? 1 : 0);
    if (remembered === "false") {
      ui.load.handlers.click();
      assert.equal(calls, 1, "stored consent permits an explicit reload");
    }
  }
});

test("Voronoi cells cover the frame and divide nearest points at their bisector", () => {
  const bounds = { min: { x: 0, y: 0 }, max: { x: 100, y: 80 } };
  const points = [
    { x: 25, y: 40 },
    { x: 75, y: 40 },
  ];
  const left = clipVoronoiCell(points[0], points, bounds);
  const right = clipVoronoiCell(points[1], points, bounds);
  assert.ok(left.every(([x]) => x <= 50 + 1e-7));
  assert.ok(right.every(([x]) => x >= 50 - 1e-7));
  const area = (polygon) =>
    Math.abs(
      polygon.reduce((sum, point, index) => {
        const next = polygon[(index + 1) % polygon.length];
        return sum + point[0] * next[1] - next[0] * point[1];
      }, 0) / 2,
    );
  assert.equal(area(left) + area(right), 8000);
});

test("Voronoi handles dozens of coincident and nearby sites", () => {
  const bounds = { min: { x: 0, y: 0 }, max: { x: 1000, y: 1000 } };
  const points = Array.from({ length: 48 }, (_, index) => ({
    x: index < 2 ? 500 : 100 + (index % 8) * 100,
    y: index < 2 ? 500 : 100 + Math.floor(index / 8) * 120,
  }));
  for (const point of points) {
    const polygon = clipVoronoiCell(point, points, bounds);
    assert.ok(polygon.length >= 3);
    assert.ok(
      polygon.every(([x, y]) => Number.isFinite(x) && Number.isFinite(y)),
    );
  }
});

test("coastline reader accepts opaque gzip and host-decompressed GeoJSON", async () => {
  const geojson = JSON.stringify({ type: "FeatureCollection", features: [] });
  const compressed = new Response(gzipSync(geojson));
  const expanded = new Response(geojson);
  assert.deepEqual(await readGeoJSONResponse(compressed), JSON.parse(geojson));
  assert.deepEqual(await readGeoJSONResponse(expanded), JSON.parse(geojson));
});
