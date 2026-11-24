import { meshSteps } from "./sf-mesh-steps.js";

// Apply at most one mesh batch per animation frame. Tile surfaces take
// priority over replaceable foliage. Dropped or superseded jobs never apply.
export function createMeshQueue(
  worker,
  requestFrame = requestAnimationFrame,
  now = () => performance.now(),
  yieldTask = (callback) => setTimeout(callback, 0),
) {
  const pending = new Set(),
    tiles = new Set();
  let active = null,
    scheduled = false,
    serial = 0;
  function schedule() {
    if (!scheduled) {
      scheduled = true;
      requestFrame(flush);
    }
  }
  function fallback() {
    if (!worker) return;
    worker.terminate();
    worker = null;
    if (active?.group) enqueue(active.group);
    active = null;
    schedule();
  }
  function flush() {
    scheduled = false;
    if (active?.error) {
      active.group?.fail(active.error);
      active = null;
    }
    if (active?.result) {
      active.group?.apply(active.result);
      active = null;
    }
    if (!active && (tiles.size || pending.size)) {
      const queue = tiles.size ? tiles : pending;
      const group = queue.values().next().value;
      queue.delete(group);
      const data = group.prepare();
      active = { group, id: ++serial };
      if (worker) {
        try {
          worker.postMessage({ id: serial, data });
        } catch {
          fallback();
        }
        return;
      }
      active.steps = meshSteps(data);
    }
    if (active?.steps) runSlice();
    else if (!active && (tiles.size || pending.size)) schedule();
  }
  function enqueue(group) {
    (group.priority ? tiles : pending).add(group);
  }
  function runSlice() {
    if (!active.group) {
      active = null;
      schedule();
      return;
    }
    const deadline = now() + 3;
    try {
      let step;
      do {
        step = active.steps.next();
      } while (!step.done && now() < deadline);
      if (!step.done) {
        // Yield to input and paint. A slow GPU must not limit geometry
        // preparation to 3 ms of CPU per displayed frame.
        yieldTask(runSlice);
        return;
      }
      active.result = step.value;
    } catch (error) {
      active.error = error;
    }
    active.steps = null;
    schedule();
  }
  if (worker) {
    worker.onmessage = ({ data }) => {
      if (active?.id !== data.id) return;
      if (data.error) active.error = new Error(data.error);
      else active.result = data.result;
      schedule();
    };
    worker.onerror = (event) => {
      event.preventDefault?.();
      fallback();
    };
  }
  function cancel(group) {
    pending.delete(group);
    if (active?.group === group) active.group = null;
  }
  return {
    cancel,
    tile(data) {
      return new Promise((resolve, reject) => {
        enqueue({
          priority: Boolean(data.region),
          prepare: () => data,
          apply: resolve,
          fail: reject,
        });
        if (!active) schedule();
      });
    },
    update(group) {
      cancel(group);
      pending.add(group);
      if (!active) schedule();
    },
  };
}
