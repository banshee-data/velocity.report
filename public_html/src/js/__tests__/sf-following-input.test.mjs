import test from "node:test";
import assert from "node:assert/strict";
import { updateFollowingKey } from "../sf-following-input.js";

function event(type, code, focusedControl = false) {
  return {
    type,
    code,
    target: { closest: () => focusedControl },
    prevented: false,
    preventDefault() {
      this.prevented = true;
    },
  };
}
for (const code of ["ArrowUp", "ArrowDown", "Space"]) {
  test(`${code} releases after focus moves to an interactive control`, () => {
    const keys = new Set();
    updateFollowingKey(keys, event("keydown", code));
    assert.equal(keys.has(code), true);
    // Tab changes focus while the driving key is still held. Repeated presses
    // over the control must not interfere with the eventual release.
    updateFollowingKey(keys, event("keydown", code, true));
    updateFollowingKey(keys, event("keyup", code, true));
    assert.equal(keys.size, 0);
  });
  test(`${code} does not start driving from a focused control`, () => {
    const keys = new Set();
    const press = event("keydown", code, true);
    const release = event("keyup", code, true);
    updateFollowingKey(keys, press);
    updateFollowingKey(keys, release);
    assert.equal(keys.size, 0);
    assert.equal(press.prevented, false);
    assert.equal(release.prevented, false);
  });
}
test("releasing one driving key preserves a different held key", () => {
  const keys = new Set(["ArrowUp", "Space"]);
  updateFollowingKey(keys, event("keyup", "ArrowUp", true));
  assert.deepEqual([...keys], ["Space"]);
});
