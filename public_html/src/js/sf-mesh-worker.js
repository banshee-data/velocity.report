import { meshSteps } from "./sf-mesh-steps.js";

self.onmessage = ({ data: { id, data } }) => {
  try {
    const batch = meshSteps(data);
    let step;
    do {
      step = batch.next();
    } while (!step.done);
    const result = step.value;
    const attributes =
      data.type === "tile"
        ? [...result.buildings, result.terrain]
            .filter(Boolean)
            .flatMap((group) => Object.values(group))
        : [result.position, result.normal, result.index];
    self.postMessage(
      { id, result },
      attributes.filter(Boolean).map((a) => a.buffer),
    );
  } catch (error) {
    self.postMessage({ id, error: String(error) });
  }
};
