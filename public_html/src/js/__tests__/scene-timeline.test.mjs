// Tests for the timeline strip. The 2D context records enough paint operations
// to check which side of the centre line each series uses.

import { test, describe } from "node:test";
import assert from "node:assert/strict";

import { createTimelineStrip } from "../scene-timeline.js";

/** A canvas stand-in that lets tests fire events and read attributes. */
function fakeCanvas({ width = 600 } = {}) {
  const handlers = new Map();
  const attrs = new Map();
  const noop = () => {};
  const fills = [];
  const strokes = [];
  let path = [];
  const ctx = new Proxy(
    {
      canvas: null,
      fillRect(x, y, width, height) {
        fills.push({ colour: this.fillStyle, x, y, width, height });
      },
      beginPath() {
        path = [];
      },
      moveTo(x, y) {
        path.push([x, y]);
      },
      lineTo(x, y) {
        path.push([x, y]);
      },
      stroke() {
        strokes.push({ colour: this.strokeStyle, path: [...path] });
      },
    },
    {
      get: (t, k) =>
        k in t ? t[k] : typeof k === "string" ? t[k] ?? noop : undefined,
      set: (t, k, v) => ((t[k] = v), true),
    },
  );
  return {
    clientWidth: width,
    clientHeight: 76,
    paint: { fills, strokes },
    style: {},
    getContext: () => ctx,
    getBoundingClientRect: () => ({ left: 0, width, top: 0, height: 76 }),
    addEventListener(type, fn) {
      handlers.set(type, fn);
    },
    setPointerCapture: noop,
    setAttribute(k, v) {
      attrs.set(k, v);
    },
    getAttribute: (k) => attrs.get(k) ?? null,
    fire(type, event) {
      handlers.get(type)?.({ preventDefault() {}, ...event });
    },
  };
}

function setup({ duration = 600, width = 600 } = {}) {
  const canvas = fakeCanvas({ width });
  const seeks = [];
  const strip = createTimelineStrip({
    canvas,
    summary: {
      bucket_seconds: 5,
      max_speed: 20,
      buckets: Array.from({ length: 120 }, (_, i) => ({
        ped: i % 3,
        cyc: i % 5 === 0 ? 1 : 0,
        veh: i % 2,
        spd: 10,
      })),
    },
    duration,
    onSeek: (s) => seeks.push(s),
  });
  return { canvas, strip, seeks };
}

describe("timeline strip", () => {
  test("vehicles and speed rise above the line while walking and cycling fall below", () => {
    const canvas = fakeCanvas({ width: 100 });
    createTimelineStrip({
      canvas,
      summary: {
        bucket_seconds: 5,
        max_speed: 20,
        buckets: [{ ped: 1, cyc: 1, veh: 1, spd: 20 }],
      },
      duration: 5,
    });
    const middle = canvas.clientHeight / 2;
    const vehicle = canvas.paint.fills.find((fill) => fill.colour === "#f2504b");
    const walking = canvas.paint.fills.find((fill) => fill.colour === "#4b9df2");
    const cycling = canvas.paint.fills.find((fill) => fill.colour === "#6fd14b");
    const speed = canvas.paint.strokes.find((stroke) => stroke.colour === "#f2a65a");
    assert.ok(vehicle.y < middle && vehicle.y + vehicle.height <= middle);
    assert.ok(walking.y >= middle);
    assert.ok(cycling.y >= walking.y + walking.height);
    assert.ok(speed.path.every(([, y]) => y < middle));
  });

  test("a click seeks to the fraction of the recording under it", () => {
    const { canvas, seeks } = setup({ duration: 600, width: 600 });
    canvas.fire("pointerdown", { pointerId: 1, clientX: 150 });
    assert.equal(seeks.at(-1), 150);
  });

  test("a seek is clamped to the recording", () => {
    const { canvas, seeks } = setup({ duration: 600, width: 600 });
    canvas.fire("pointerdown", { pointerId: 1, clientX: -80 });
    canvas.fire("pointerdown", { pointerId: 1, clientX: 5000 });
    assert.deepEqual(seeks, [0, 600]);
  });

  // The strip replaced a range input, which was the only way to scrub without
  // a pointer. These keys are that capability, not a nicety.
  describe("keyboard scrubbing", () => {
    const cases = [
      ["ArrowRight", {}, 101],
      ["ArrowLeft", {}, 99],
      ["ArrowRight", { shiftKey: true }, 130],
      ["PageDown", {}, 130],
      ["PageUp", {}, 70],
      ["Home", {}, 0],
      ["End", {}, 600],
    ];
    for (const [key, mods, want] of cases) {
      test(`${key}${mods.shiftKey ? " with shift" : ""} seeks to ${want}`, () => {
        const { canvas, strip, seeks } = setup({ duration: 600 });
        strip.setPlayhead(100);
        canvas.fire("keydown", { key, ...mods });
        assert.equal(seeks.at(-1), want);
      });
    }

    test("stepping past either end stops at it", () => {
      const { canvas, strip, seeks } = setup({ duration: 600 });
      strip.setPlayhead(0);
      canvas.fire("keydown", { key: "ArrowLeft" });
      assert.equal(seeks.at(-1), 0);
      strip.setPlayhead(600);
      canvas.fire("keydown", { key: "ArrowRight" });
      assert.equal(seeks.at(-1), 600);
    });

    test("an unrelated key is left to the page", () => {
      const { canvas, seeks } = setup();
      canvas.fire("keydown", { key: "a" });
      assert.deepEqual(seeks, []);
    });
  });

  test("the strip announces its range to a screen reader", () => {
    const { canvas } = setup({ duration: 663 });
    assert.equal(canvas.getAttribute("aria-valuemin"), "0");
    assert.equal(canvas.getAttribute("aria-valuemax"), "663");
  });

  test("a collapsed strip seeks to zero rather than NaN", () => {
    const { canvas, seeks } = setup({ width: 0 });
    canvas.fire("pointerdown", { pointerId: 1, clientX: 40 });
    assert.equal(seeks.at(-1), 0);
  });
});
