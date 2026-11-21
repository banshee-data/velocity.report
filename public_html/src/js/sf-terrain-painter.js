// One texture repaint and at most one mesh upload per animation frame. The
// clipping worker has one build in flight; pending invalidations coalesce.
export function createTerrainPainter({
  worker,
  paint,
  apply,
  onError = () => {},
  requestFrame = requestAnimationFrame,
  cancelFrame = cancelAnimationFrame,
}) {
  const entries = new Map(),
    textures = new Set(),
    meshes = new Set();
  let serial = 0,
    frame = null,
    inFlight = null,
    completed = null,
    disposed = false;
  let fallback = !worker;
  const bounds = (points) => {
    const b = [Infinity, Infinity, -Infinity, -Infinity];
    for (const [x, y] of points) {
      b[0] = Math.min(b[0], x);
      b[1] = Math.min(b[1], y);
      b[2] = Math.max(b[2], x);
      b[3] = Math.max(b[3], y);
    }
    return b;
  };
  const contributes = (target, source) => {
    const a = target.regionBounds,
      b = source.paintBounds;
    return (
      Math.hypot(
        target.entry.tile.o[0] - source.entry.tile.o[0],
        target.entry.tile.o[1] - source.entry.tile.o[1],
      ) <=
        target.entry.tile.half + source.entry.tile.half + 100 &&
      a[0] <= b[2] &&
      a[2] >= b[0] &&
      a[1] <= b[3] &&
      a[3] >= b[1]
    );
  };
  const sourcesFor = (target) =>
    [...entries.values()].filter((source) => contributes(target, source));
  const schedule = () => {
    if (!disposed && frame === null) frame = requestFrame(flush);
  };
  const fail = (error) => {
    worker?.terminate();
    worker = null;
    fallback = true;
    inFlight = completed = null;
    meshes.clear();
    for (const record of entries.values()) {
      apply(record.entry, new Float32Array());
      textures.add(record.token);
    }
    onError(error);
    schedule();
  };
  const send = (message) => {
    try {
      worker?.postMessage(message);
    } catch (error) {
      fail(error);
    }
  };
  const invalidate = (changed) => {
    for (const target of entries.values()) {
      if (target !== changed && !contributes(target, changed)) continue;
      target.revision++;
      textures.add(target.token);
      if (worker) meshes.add(target.token);
    }
    schedule();
  };
  function flush() {
    frame = null;
    if (disposed) return;
    if (completed) {
      const record = entries.get(inFlight.token);
      if (record === inFlight.record && record.revision === inFlight.revision)
        apply(record.entry, completed.positions);
      completed = inFlight = null;
    }
    const token = textures.values().next().value;
    if (token !== undefined) {
      textures.delete(token);
      const record = entries.get(token);
      paint(
        record.entry,
        sourcesFor(record).map((source) => source.entry),
        fallback,
      );
    }
    if (worker && !inFlight && meshes.size) {
      const token = meshes.values().next().value;
      meshes.delete(token);
      const record = entries.get(token);
      inFlight = { token, record, revision: record.revision, id: ++serial };
      send({
        type: "build",
        id: inFlight.id,
        token,
        sources: sourcesFor(record).map((source) => source.token),
      });
    }
    if (textures.size || (!inFlight && meshes.size)) schedule();
  }
  if (worker) {
    worker.onmessage = ({ data }) => {
      if (disposed || !inFlight || data.id !== inFlight.id) return;
      if (data.error) fail(new Error(data.error));
      else {
        completed = data;
        schedule();
      }
    };
    worker.onerror = (event) => {
      event.preventDefault?.();
      if (!disposed && worker)
        fail(new Error(event.message || "Marking worker failed"));
    };
  }
  const remove = (token) => {
    const record = entries.get(token);
    if (!record) return;
    entries.delete(token);
    textures.delete(token);
    meshes.delete(token);
    send({ type: "remove", token });
    invalidate(record);
  };
  return {
    get idle() {
      return (
        frame === null &&
        !textures.size &&
        !meshes.size &&
        !inFlight &&
        !completed
      );
    },
    add(token, entry) {
      if (disposed) return;
      remove(token);
      const paintBounds = bounds([
        ...entry.roads.flatMap((road) => road.polygon),
        ...entry.markings.flat(),
        ...(entry.kerbs ?? []).flat(),
      ]);
      // Include the half-width of the kerb stroke outside its centreline.
      paintBounds[0] -= 0.15;
      paintBounds[1] -= 0.15;
      paintBounds[2] += 0.15;
      paintBounds[3] += 0.15;
      const record = {
        token,
        entry,
        revision: 0,
        regionBounds: bounds(entry.region),
        paintBounds,
      };
      entries.set(token, record);
      send({
        type: "set",
        token,
        entry: {
          tile: { o: entry.tile.o, g: entry.tile.g },
          region: entry.region,
          markings: entry.markings,
        },
      });
      invalidate(record);
    },
    remove,
    dispose() {
      disposed = true;
      if (frame !== null) cancelFrame(frame);
      worker?.terminate();
      frame = worker = null;
      entries.clear();
      textures.clear();
      meshes.clear();
      completed = inFlight = null;
    },
  };
}
