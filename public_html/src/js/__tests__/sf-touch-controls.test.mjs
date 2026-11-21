import test from "node:test";
import assert from "node:assert/strict";
import { bindDrivingPad } from "../sf-touch-controls.js";

class Button extends EventTarget {
  constructor(act) {
    super();
    this.dataset = { act };
    this.attributes = {};
    this.captured = [];
    this.classList = {
      toggle: (name, on) => {
        this[name] = on;
      },
    };
  }
  setAttribute(k, v) {
    this.attributes[k] = v;
  }
  setPointerCapture(id) {
    this.captured.push(id);
  }
}
const event = (type, props = {}) =>
  Object.assign(new Event(type, { cancelable: true }), props);
function pad() {
  const root = new EventTarget(),
    buttons = Object.fromEntries(
      ["up", "down", "left", "right", "brake"].map((a) => [a, new Button(a)]),
    );
  let state;
  const control = bindDrivingPad(
    Object.values(buttons),
    (s) => (state = s),
    root,
  );
  return {
    buttons,
    root,
    control,
    get state() {
      return state;
    },
  };
}

test("touch input prevents native gestures, captures the pointer, and remains held outside the button", () => {
  const p = pad(),
    e = event("pointerdown", { pointerId: 1, button: 0 });
  p.buttons.up.dispatchEvent(e);
  assert.ok(e.defaultPrevented);
  assert.deepEqual(p.buttons.up.captured, [1]);
  assert.equal(p.state.throttle, 1);
  p.buttons.up.dispatchEvent(event("pointerleave", { pointerId: 1 }));
  assert.equal(p.state.throttle, 1);
  p.root.dispatchEvent(event("pointerup", { pointerId: 1 }));
  assert.equal(p.state.throttle, 0);
  assert.equal(p.buttons.up.attributes["aria-pressed"], "false");
  for (const type of [
    "touchstart",
    "touchmove",
    "touchend",
    "click",
    "dblclick",
    "contextmenu",
    "selectstart",
    "dragstart",
  ]) {
    const e = event(type);
    p.buttons.up.dispatchEvent(e);
    assert.ok(e.defaultPrevented, type);
    assert.equal(p.state.throttle, 0);
  }
});

test("independent fingers, opposing directions and repeated holds cannot release each other", () => {
  const p = pad();
  const down = (act, id) =>
    p.buttons[act].dispatchEvent(
      event("pointerdown", { pointerId: id, button: 0 }),
    );
  down("up", 1);
  down("left", 2);
  down("right", 3);
  down("up", 4);
  assert.deepEqual(p.state, { throttle: 1, steer: 0, brake: false });
  p.buttons.right.dispatchEvent(event("lostpointercapture", { pointerId: 3 }));
  assert.equal(p.state.steer, 1);
  p.root.dispatchEvent(event("pointercancel", { pointerId: 1 }));
  assert.equal(p.state.throttle, 1);
  p.root.dispatchEvent(event("pointerup", { pointerId: 4 }));
  assert.equal(p.state.throttle, 0);
  assert.equal(p.state.steer, 1);
  p.control.clear();
  assert.deepEqual(p.state, { throttle: 0, steer: 0, brake: false });
  assert.ok(!p.buttons.left.held);
});

test("keyboard holds remain available without a click toggle", () => {
  const p = pad();
  p.buttons.brake.dispatchEvent(event("keydown", { code: "Space" }));
  assert.equal(p.state.brake, true);
  p.buttons.brake.dispatchEvent(event("keyup", { code: "Space" }));
  assert.equal(p.state.brake, false);
  p.buttons.up.dispatchEvent(event("keydown", { code: "Enter" }));
  assert.equal(p.state.throttle, 1);
  p.buttons.up.dispatchEvent(event("blur"));
  assert.equal(p.state.throttle, 0);
});
