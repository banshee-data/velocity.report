// Developer preferences are local; measured orientation belongs to map-marks.json.
export const DEV_STORAGE_KEY = "velocity.scene.dev";

export function resolveDevMode(search, storage) {
  const value = new URLSearchParams(search).get("dev");
  if (value === "true" || value === "false") {
    try {
      storage?.setItem(DEV_STORAGE_KEY, value);
    } catch {
      /* Storage may be blocked. */
    }
    return value === "true";
  }
  try {
    return storage?.getItem(DEV_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

export function parseAzimuth(value) {
  if (value === "" || value === null) return null;
  const number = Number(value);
  if (!Number.isFinite(number))
    throw new Error("Enter a finite angle in degrees.");
  return ((number % 360) + 360) % 360;
}

export function updateMapMark(source, id, angles) {
  const document = JSON.parse(source);
  const marks = document.marks?.filter((mark) => mark.id === id);
  if (marks?.length !== 1)
    throw new Error(`Expected exactly one map mark for ${id}.`);
  for (const key of ["grid_azimuth_deg", "north_azimuth_deg"]) {
    marks[0][key] = parseAzimuth(angles[key]);
  }
  return JSON.stringify(document, null, 2) + "\n";
}

export function mountSceneDev({
  panel,
  captureView,
  siteId,
  gridAzimuthDeg,
  northAzimuthDeg,
  onChange,
  onTop,
}) {
  let storage;
  try {
    storage = window.localStorage;
  } catch {
    /* Private browsing. */
  }
  const enabled = resolveDevMode(window.location.search, storage);
  if (captureView) captureView.hidden = !enabled;
  if (!panel || !enabled) return;
  panel.hidden = false;
  const grid = panel.querySelector('[name="grid_azimuth_deg"]');
  const north = panel.querySelector('[name="north_azimuth_deg"]');
  const output = panel.querySelector("output");
  grid.value = gridAzimuthDeg ?? "";
  north.value = northAzimuthDeg ?? "";
  const read = () => {
    if (!grid.checkValidity() || !north.checkValidity())
      throw new Error("Enter valid angles in degrees.");
    return {
      grid_azimuth_deg: parseAzimuth(grid.value),
      north_azimuth_deg: parseAzimuth(north.value),
    };
  };
  const preview = () => {
    try {
      const angles = read();
      onChange(angles);
      output.textContent = JSON.stringify({ id: siteId, ...angles });
    } catch (error) {
      output.textContent = error.message;
    }
  };
  grid.addEventListener("input", preview);
  north.addEventListener("input", preview);
  panel.querySelector('[data-action="top"]').addEventListener("click", onTop);
  panel.querySelector('[data-action="reset"]').addEventListener("click", () => {
    grid.value = gridAzimuthDeg ?? "";
    north.value = northAzimuthDeg ?? "";
    preview();
  });
  panel
    .querySelector('[data-action="save"]')
    .addEventListener("click", async () => {
      try {
        const angles = read();
        if (!window.showOpenFilePicker)
          throw new Error(
            "To save directly, open this page in Chrome or Edge on HTTPS or localhost. Select tools/s2-archive/map-marks.json.",
          );
        const [handle] = await window.showOpenFilePicker({
          multiple: false,
          types: [
            {
              description: "Canonical map marks",
              accept: { "application/json": [".json"] },
            },
          ],
        });
        if (handle.name !== "map-marks.json")
          throw new Error("Select tools/s2-archive/map-marks.json.");
        const source = await (await handle.getFile()).text();
        const updated = updateMapMark(source, siteId, angles);
        const writable = await handle.createWritable();
        await writable.write(updated);
        await writable.close();
        output.textContent = `Saved ${siteId} to map-marks.json. Rebuild the site index and scene map to publish these angles.`;
      } catch (error) {
        output.textContent =
          error.name === "AbortError"
            ? "Save cancelled; preview only."
            : error.message;
      }
    });
  preview();
}
