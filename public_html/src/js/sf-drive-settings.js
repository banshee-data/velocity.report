const storageKey = "sf-map-sources-v1";
const overlayPath =
  "city-tiles/abc3640983c94385554836af876f5498b963001b801830172ccf3165a1a78594/manifest.json";
export const BAY_BRIDGE_OVERLAY = `http://arrow-worker:50061/${overlayPath}`;

function httpUrl(value) {
  const url = new URL(value);
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password
  )
    throw new Error("Use an HTTP or HTTPS address without credentials.");
  return url;
}

function directory(url) {
  return new URL(
    url.pathname.endsWith("/") ? url.pathname : `${url.pathname}/`,
    url,
  ).href;
}

export function mapSources({ tiles = "", gerardus = "" }) {
  const map = tiles.trim() ? httpUrl(tiles.trim()) : null;
  const source = gerardus.trim() ? httpUrl(gerardus.trim()) : null;
  return {
    tiles: map
      ? map.pathname.endsWith("/manifest.json")
        ? new URL(".", map).href
        : directory(map)
      : "",
    overlay: source
      ? source.pathname.endsWith("/manifest.json")
        ? source.href
        : new URL(overlayPath, directory(source)).href
      : "",
  };
}

export function mountDriveSettings({
  menu,
  menuButton,
  form,
  tiles,
  gerardus,
  error,
}) {
  menuButton.addEventListener("click", () => {
    menu.showModal();
    menuButton.setAttribute("aria-expanded", "true");
  });
  menu.addEventListener("close", () =>
    menuButton.setAttribute("aria-expanded", "false"),
  );

  let saved;
  try {
    saved = JSON.parse(localStorage.getItem(storageKey));
  } catch {
    // Query parameters still work when the browser disables storage.
  }
  const url = new URL(location.href);
  tiles.value = url.searchParams.get("tiles") ?? saved?.tiles ?? "";
  gerardus.value = url.searchParams.get("overlay") ?? saved?.gerardus ?? "";
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const values = {
      tiles: tiles.value.trim(),
      gerardus: gerardus.value.trim(),
    };
    try {
      mapSources(values);
    } catch {
      error.textContent = "Enter full HTTP or HTTPS URLs, without credentials.";
      return;
    }
    try {
      localStorage.setItem(storageKey, JSON.stringify(values));
    } catch {
      // The URL carries the settings across this reload.
    }
    for (const [key, value] of [
      ["tiles", values.tiles],
      ["overlay", values.gerardus],
    ]) {
      if (value) url.searchParams.set(key, value);
      else url.searchParams.delete(key);
    }
    if (
      !values.gerardus &&
      url.searchParams.get("at")?.startsWith("bay-bridge-")
    )
      url.searchParams.delete("at");
    location.assign(url.href);
  });
  return { tiles: tiles.value, gerardus: gerardus.value };
}
