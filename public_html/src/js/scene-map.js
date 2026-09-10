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

// Must match the zooms fetch-basemap.mjs caches; asking for a zoom outside
// them shows empty grid.
const MIN_ZOOM = 12;
const MAX_ZOOM = 16;

function tileLayer(L) {
  // Tiles are served from this site's own assets, cached at build time by
  // tools/s2-hilbert/fetch-basemap.mjs. That is the same bargain the report
  // map already strikes: fetch once, deliberately, and serve every reader
  // from our own origin afterwards. A reader looking at where a sensor stood
  // does not hand their address to a tile server to do it.
  //
  // Attribution is still required and still shown: the data is OpenStreetMap's
  // under ODbL whoever serves the bytes.
  return L.tileLayer("/img/tiles/{z}/{x}/{y}.png", {
    minZoom: MIN_ZOOM,
    maxZoom: MAX_ZOOM,
    // Only the cached window exists. Without this a pan past the edge asks
    // for tiles that were never fetched and paints broken images.
    noWrap: true,
    attribution:
      'Map data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> ' +
      "contributors, cached locally",
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

export function initSceneMap(container, sites) {
  const L = window.L;
  if (!L || !container) return null;

  const located = sites.filter((s) => s.position);
  if (located.length === 0) return null;

  container.textContent = "";
  const map = L.map(container, {
    scrollWheelZoom: false,
    minZoom: MIN_ZOOM,
    maxZoom: MAX_ZOOM,
  }).setView(SF, 13);
  tileLayer(L).addTo(map);

  // S2 cells first, so the markers sit above them. Leaflet draws overlays in
  // the order they are added, and a filled cell added later covers the points
  // it is meant to frame — which is why the outlines appeared for an instant
  // at load and then vanished under the layer above them.
  //
  // One outline per cell, not per site: twenty-four sites share four
  // neighbourhood cells and a single area cell, and drawing the same square
  // twenty-four times stacks its stroke into a solid band.
  const drawn = new Set();
  for (const level of ["area", "neighbourhood"]) {
    for (const site of located) {
      const ring = site.cell_geometry?.[level];
      const token = site.cells?.[level]?.token;
      if (!ring || !token || drawn.has(token)) continue;
      drawn.add(token);
      const area = level === "area";
      L.polygon(
        ring.map((v) => [v.lat, v.lng]),
        {
          color: area ? "#c05621" : "#1c6fd6",
          weight: area ? 1.5 : 1,
          opacity: area ? 0.65 : 0.5,
          fill: false,
          dashArray: area ? "6 4" : "3 3",
          interactive: false,
        },
      )
        .addTo(map)
        .bindTooltip(
          `${site.cells[level].display} (level ${site.cells[level].level})`,
          {
            sticky: true,
          },
        );
    }
  }

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
