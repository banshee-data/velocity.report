export const CYCLING_TILE_URL =
  "https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png";

export const CYCLE_MAP_CONSENT_KEY = "velocity.scene-map.tile-consent";
export const CYCLE_MAP_ENABLED_KEY = "velocity.scene-map.tiles-enabled";

function readStorage(storage, key) {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function writeStorage(storage, key, value) {
  try {
    storage?.setItem(key, value);
  } catch {
    // Storage can be unavailable in private or locked-down browser contexts.
  }
}

export function mountCycleMapConsent({
  L,
  map,
  modal,
  loadButton,
  storage,
  onToggle,
}) {
  if (!modal || !loadButton) return null;
  const closeButton = modal.querySelector('[data-action="close"]');
  const cancelButton = modal.querySelector('[data-action="cancel"]');
  const allowButton = modal.querySelector('[data-action="allow"]');
  let cycling = null;
  let active = false;
  let consent = readStorage(storage, CYCLE_MAP_CONSENT_KEY);
  let enabled =
    consent === "accepted" &&
    readStorage(storage, CYCLE_MAP_ENABLED_KEY) !== "false";

  const renderButton = () => {
    loadButton.textContent =
      consent === "accepted"
        ? `Cycle map: ${enabled ? "on" : "off"}`
        : "Load Cycle map";
    if (consent === "accepted") {
      loadButton.setAttribute("role", "switch");
      loadButton.setAttribute("aria-checked", String(enabled));
    } else {
      loadButton.removeAttribute("role");
      loadButton.removeAttribute("aria-checked");
    }
  };

  const setOpen = (open) => {
    modal.hidden = !open;
    loadButton.hidden = open;
    renderButton();
    if (open) allowButton.focus();
  };

  const createLayer = () => {
    if (!cycling) {
      cycling = L.tileLayer(CYCLING_TILE_URL, {
        pane: "basemap",
        className: "scene-map-cycle-tiles",
        minZoom: 12,
        maxZoom: 16,
        noWrap: true,
        attribution:
          'Cycle map &copy; <a href="https://www.cyclosm.org/">CyclOSM</a>; map data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors; hosted by <a href="https://www.openstreetmap.fr/">OSM France</a>',
      });
    }
    return cycling;
  };

  const setEnabled = (nextEnabled) => {
    enabled = Boolean(nextEnabled) && consent === "accepted";
    writeStorage(storage, CYCLE_MAP_ENABLED_KEY, String(enabled));
    if (enabled && !active) {
      createLayer().addTo(map);
      active = true;
    } else if (!enabled && active) {
      map.removeLayer(cycling);
      active = false;
    }
    onToggle?.(enabled);
    renderButton();
  };

  const cancel = () => {
    consent = "cancelled";
    writeStorage(storage, CYCLE_MAP_CONSENT_KEY, consent);
    setEnabled(false);
    setOpen(false);
  };

  const allow = () => {
    consent = "accepted";
    writeStorage(storage, CYCLE_MAP_CONSENT_KEY, consent);
    setEnabled(true);
    setOpen(false);
  };

  closeButton.addEventListener("click", cancel);
  cancelButton.addEventListener("click", cancel);
  allowButton.addEventListener("click", allow);
  loadButton.addEventListener("click", () => {
    if (consent === "accepted") setEnabled(!enabled);
    else setOpen(true);
  });
  modal.addEventListener("keydown", (event) => {
    if (event.key === "Escape") cancel();
  });

  if (consent === "accepted") {
    setEnabled(enabled);
    setOpen(false);
  } else if (consent === "cancelled") {
    setEnabled(false);
    setOpen(false);
  } else {
    onToggle?.(false);
    setOpen(true);
  }

  return {
    allow,
    cancel,
    get enabled() {
      return enabled;
    },
    get layer() {
      return cycling;
    },
  };
}
