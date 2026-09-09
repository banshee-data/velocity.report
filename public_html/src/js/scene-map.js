/**
 * The scene map: every recorded site as a point on a real street map.
 *
 * Points carry no labels. A label on a map at this zoom either overlaps its
 * neighbours or hides the street it is standing on, and the street is the
 * thing worth seeing — so the name lives in the popup, one click away, and
 * the list below the map carries the full set.
 *
 * Tiles come from a third party, which is a privacy cost the rest of this
 * project does not pay: a visitor's IP reaches the tile server on every pan.
 * The map is therefore built only when the page asks for it, and the fallback
 * is the self-contained SVG that needs no network at all.
 */
const SF = [37.7749, -122.4194];

function tileLayer(L, key) {
  // OpenCycleMap is Thunderforest's layer and wants an API key; without one it
  // serves a watermarked tile. Standard OSM needs no key, so it is what an
  // unconfigured site gets rather than a map covered in "API key required".
  if (key) {
    return L.tileLayer(
      `https://{s}.tile.thunderforest.com/cycle/{z}/{x}/{y}.png?apikey=${key}`,
      {
        maxZoom: 19,
        attribution:
          'Maps &copy; <a href="https://www.thunderforest.com/">Thunderforest</a>, ' +
          'Data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
      },
    );
  }
  return L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
    maxZoom: 19,
    attribution:
      '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
  });
}

function popup(site) {
  const name = document.createElement("strong");
  name.textContent = site.title;
  const wrap = document.createElement("div");
  wrap.appendChild(name);
  if (site.page) {
    const link = document.createElement("a");
    link.href = site.page;
    link.textContent = "Open this survey";
    link.style.display = "block";
    link.style.marginTop = "0.35rem";
    wrap.appendChild(link);
  } else {
    const note = document.createElement("div");
    note.textContent = "Recorded; not yet published.";
    note.style.marginTop = "0.35rem";
    note.style.opacity = "0.75";
    wrap.appendChild(note);
  }
  return wrap;
}

export function initSceneMap(container, sites, apiKey) {
  const L = window.L;
  if (!L || !container) return null;

  const located = sites.filter((s) => s.position);
  if (located.length === 0) return null;

  container.textContent = "";
  const map = L.map(container, { scrollWheelZoom: false }).setView(SF, 13);
  tileLayer(L, apiKey).addTo(map);

  const bounds = [];
  for (const site of located) {
    const at = [site.position.lat, site.position.lon];
    bounds.push(at);
    // Published surveys read as solid, unpublished as hollow: the difference a
    // visitor cares about is whether there is something to open.
    const marker = L.circleMarker(at, {
      radius: 7,
      weight: 2,
      color: site.published ? "#1c6fd6" : "#8a8a8a",
      fillColor: site.published ? "#1c6fd6" : "#ffffff",
      fillOpacity: site.published ? 0.85 : 0.5,
    }).addTo(map);
    marker.bindPopup(popup(site));
    if (site.page) {
      // A point that opens something should behave like a link.
      marker.on("dblclick", () => {
        window.location.href = site.page;
      });
    }
  }
  // Frame the whole survey. invalidateSize first: the container is styled by
  // the page's own stylesheet, and if the map is built before that height has
  // settled Leaflet measures a collapsed viewport and fitBounds resolves to a
  // maximum zoom on the first point.
  const frame = () => {
    map.invalidateSize();
    map.fitBounds(bounds, { padding: [30, 30], maxZoom: 15 });
  };
  frame();
  // Once more after layout and webfonts settle, which can change the height.
  window.requestAnimationFrame(frame);
  window.addEventListener("load", frame, { once: true });
  return map;
}

const container = document.getElementById("scene-map");
const payload = document.getElementById("scene-map-data");
if (container && payload) {
  initSceneMap(
    container,
    JSON.parse(payload.textContent),
    container.dataset.tileKey || "",
  );
}
