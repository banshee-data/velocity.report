import { createRoadMarkingCache } from "./sf-road-markings.js";

const cache = createRoadMarkingCache();
self.onmessage = ({ data }) => {
  if (data.type === "set") cache.set(data.token, data.entry);
  else if (data.type === "remove") cache.remove(data.token);
  else if (data.type === "build") {
    try {
      const positions = cache.build(data.token, data.sources);
      self.postMessage({ id: data.id, positions }, [positions.buffer]);
    } catch (error) {
      self.postMessage({ id: data.id, error: String(error) });
    }
  }
};
