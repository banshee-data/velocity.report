// Focus can move between a driving key's press and release (for example via Tab).
// Always release tracked keys; only new presses belong behind the focus guard.
export function updateFollowingKey(keys, event) {
  if (!["ArrowUp", "ArrowDown", "Space"].includes(event.code)) return;
  if (event.type === "keyup") {
    if (keys.delete(event.code)) event.preventDefault();
    return;
  }
  if (event.target.closest("button, a")) return;
  event.preventDefault();
  keys.add(event.code);
}
