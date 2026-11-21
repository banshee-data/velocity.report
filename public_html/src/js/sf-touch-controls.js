// A driving pad holds actions; it does not select text, focus buttons or zoom.
export function bindDrivingPad(buttons, onChange, root = window) {
  const held = new Map();
  const list = [...buttons];
  const prevent = (e) => {
    if (e.cancelable) e.preventDefault();
  };
  const update = () => {
    const actions = new Set(held.values());
    for (const button of list) {
      const active = actions.has(button.dataset.act);
      button.classList.toggle("held", active);
      button.setAttribute("aria-pressed", String(active));
    }
    onChange({
      throttle: Number(actions.has("up")) - Number(actions.has("down")),
      steer: Number(actions.has("left")) - Number(actions.has("right")),
      brake: actions.has("brake"),
    });
  };
  const release = (e) => {
    if (!held.delete(e.pointerId)) return;
    prevent(e);
    update();
  };
  for (const button of list) {
    button.addEventListener("pointerdown", (e) => {
      if (e.button !== 0) return;
      prevent(e);
      held.set(e.pointerId, button.dataset.act);
      try {
        button.setPointerCapture(e.pointerId);
      } catch {
        /* Global release is the fallback. */
      }
      update();
    });
    for (const event of ["pointerup", "pointercancel", "lostpointercapture"])
      button.addEventListener(event, release);
    for (const event of [
      "touchstart",
      "touchmove",
      "touchend",
      "click",
      "dblclick",
      "contextmenu",
      "selectstart",
      "dragstart",
    ])
      button.addEventListener(event, prevent, { passive: false });
    button.addEventListener("keydown", (e) => {
      if (e.code !== "Space" && e.code !== "Enter") return;
      prevent(e);
      held.set(button, button.dataset.act);
      update();
    });
    const releaseKey = () => {
      if (held.delete(button)) update();
    };
    button.addEventListener("keyup", (e) => {
      if (e.code === "Space" || e.code === "Enter") {
        prevent(e);
        releaseKey();
      }
    });
    button.addEventListener("blur", releaseKey);
  }
  root.addEventListener("pointerup", release);
  root.addEventListener("pointercancel", release);
  update();
  return {
    clear() {
      held.clear();
      update();
    },
  };
}
