export const MAP_MODE_KEY = "velocity.scene-map.mode";
export const CYCLING_TILE_URL =
  "https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm-lite/{z}/{x}/{y}.png";

export function readMapMode(storage) {
  try {
    return storage?.getItem(MAP_MODE_KEY) === "car" ? "car" : "bike";
  } catch {
    return "bike";
  }
}

export function mountMapMode({ L, map, controls, status, storage }) {
  if (!controls) return;
  const pane = map.createPane("cycling");
  pane.style.zIndex = "250"; // Above streets, below site markers and S2 outlines.
  pane.style.pointerEvents = "none";
  const cycling = L.tileLayer(CYCLING_TILE_URL, {
    pane: "cycling",
    minZoom: 12,
    maxZoom: 16,
    noWrap: true,
    attribution:
      '<a href="https://www.cyclosm.org/">CyclOSM</a> cycling overlay, hosted by <a href="https://www.openstreetmap.fr/">OSM France</a>',
  });
  let mode = readMapMode(storage);
  let failed = false;
  const updateStatus = () => {
    if (status)
      status.textContent =
        mode === "car"
          ? "Street map"
          : failed
            ? "Some cycling tiles could not load. The street map and survey markers remain available."
            : "Cycling routes · CyclOSM overlay";
  };
  cycling.on("loading", () => {
    failed = false;
    updateStatus();
  });
  cycling.on("tileerror", () => {
    failed = true;
    updateStatus();
  });
  cycling.on("load", updateStatus);
  const apply = () => {
    if (mode === "bike" && !map.hasLayer(cycling)) cycling.addTo(map);
    if (mode === "car" && map.hasLayer(cycling)) map.removeLayer(cycling);
    for (const button of controls.querySelectorAll("[data-map-mode]")) {
      button.setAttribute(
        "aria-pressed",
        String(button.dataset.mapMode === mode),
      );
    }
    updateStatus();
  };
  for (const button of controls.querySelectorAll("[data-map-mode]")) {
    button.addEventListener("click", () => {
      mode = button.dataset.mapMode === "car" ? "car" : "bike";
      try {
        storage?.setItem(MAP_MODE_KEY, mode);
      } catch {
        /* The toggle still works. */
      }
      apply();
    });
  }
  controls.hidden = false;
  apply();
}
