/** Every recorded site, with a private local fallback and an opt-in cycle map. */
import { mountCycleMapConsent } from "./scene-map-mode.js";
import { SCENE_CSS_COLOURS } from "./scene-colours.js";

const SF = [37.7749, -122.4194];
const MIN_ZOOM = 12;
const MAX_ZOOM = 16;
const SVG_NS = "http://www.w3.org/2000/svg";
const SHIMMER_ANGLE_DEG = 55.7;
const SHIMMER_SWEEP_MS = 650;
const FIRST_SHIMMER_DELAY_MS = 1000;
const SECOND_SHIMMER_DELAY_MS = 10000;

export function clipVoronoiCell(point, others, bounds) {
  let polygon = [
    [bounds.min.x, bounds.min.y],
    [bounds.max.x, bounds.min.y],
    [bounds.max.x, bounds.max.y],
    [bounds.min.x, bounds.max.y],
  ];
  for (const other of others) {
    if (other === point || (other.x === point.x && other.y === point.y))
      continue;
    const nx = other.x - point.x;
    const ny = other.y - point.y;
    const c = (other.x ** 2 + other.y ** 2 - point.x ** 2 - point.y ** 2) / 2;
    const inside = ([x, y]) => x * nx + y * ny <= c + 1e-7;
    const clipped = [];
    for (let i = 0; i < polygon.length; i++) {
      const a = polygon[i];
      const b = polygon[(i + 1) % polygon.length];
      const aInside = inside(a);
      const bInside = inside(b);
      if (aInside) clipped.push(a);
      if (aInside !== bInside) {
        const dx = b[0] - a[0];
        const dy = b[1] - a[1];
        const t = (c - a[0] * nx - a[1] * ny) / (dx * nx + dy * ny);
        clipped.push([a[0] + t * dx, a[1] + t * dy]);
      }
    }
    polygon = clipped;
    if (!polygon.length) break;
  }
  return polygon;
}

function wrapPathInLink(layer, href, label) {
  const path = layer.getElement?.();
  if (!path || !href || path.parentElement?.localName === "a") return;
  const link = document.createElementNS(SVG_NS, "a");
  link.setAttribute("href", href);
  link.setAttribute("aria-label", label);
  path.parentNode.insertBefore(link, path);
  link.appendChild(path);
}

export async function readGeoJSONResponse(response) {
  if (!response.ok) throw new Error(`coastline: ${response.status}`);
  const raw = await response.arrayBuffer();
  const bytes = new Uint8Array(raw);
  let decoded = raw;
  if (bytes[0] === 0x1f && bytes[1] === 0x8b) {
    if (typeof DecompressionStream === "undefined") {
      throw new Error("This browser cannot decompress the coastline.");
    }
    decoded = await new Response(
      new Blob([raw]).stream().pipeThrough(new DecompressionStream("gzip")),
    ).arrayBuffer();
  }
  return JSON.parse(new TextDecoder().decode(decoded));
}

function addCoastline(L, map, url) {
  if (!url) return;
  fetch(url)
    .then(readGeoJSONResponse)
    .then((data) =>
      L.geoJSON(data, {
        pane: "coastline",
        interactive: false,
        style: { color: "#4f8495", weight: 2.5, opacity: 0.8, fill: false },
      }).addTo(map),
    )
    .catch(() => {
      /* S2 geometry remains useful without decorative context. */
    });
}

function addSiteLayers(L, map, located) {
  const cells = new Map();
  const areaCells = new Set();
  for (const site of located) {
    const areaRing = site.cell_geometry?.area;
    const areaToken = site.cells?.area?.token;
    if (areaRing && areaToken && !areaCells.has(areaToken)) {
      areaCells.add(areaToken);
      L.polygon(
        areaRing.map((v) => [v.lat, v.lng]),
        {
          pane: "grid",
          color: "#c05621",
          weight: 1.5,
          opacity: 0.65,
          fill: false,
          dashArray: "6 4",
          interactive: false,
        },
      ).addTo(map);
    }
    const token = site.cells?.neighbourhood?.token;
    if (!token || cells.has(token)) continue;
    const ring = site.cell_geometry?.neighbourhood;
    if (!ring) continue;
    const fill = L.polygon(
      ring.map((v) => [v.lat, v.lng]),
      {
        pane: "cellFill",
        stroke: false,
        fillColor: "#34d399",
        fillOpacity: 0.1,
        interactive: false,
      },
    ).addTo(map);
    const outline = L.polygon(
      ring.map((v) => [v.lat, v.lng]),
      {
        pane: "grid",
        color: "#1c6fd6",
        weight: 1.5,
        opacity: 0.7,
        fill: false,
        dashArray: "3 3",
        interactive: false,
      },
    ).addTo(map);
    cells.set(token, { fill, outline });
  }
  const markers = new Map();
  const markerRings = new Map();
  // Project onto the purple-line direction agreed for this map: 55.7 degrees
  // above screen-right. Equal scores sit on the same perpendicular wavefront,
  // so the light reads as one wash rather than a list of dots blinking.
  const angle = (SHIMMER_ANGLE_DEG * Math.PI) / 180;
  const direction = { x: Math.cos(angle), y: -Math.sin(angle) };
  const shimmerScores = new Map(
    located.map((site) => {
      const point = map.project(
        [site.position.lat, site.position.lon],
        map.getZoom(),
      );
      return [site.id, point.x * direction.x + point.y * direction.y];
    }),
  );
  const scoreValues = [...shimmerScores.values()];
  const minScore = Math.min(...scoreValues);
  const scoreSpan = Math.max(Math.max(...scoreValues) - minScore, 1);
  for (const site of located) {
    const outer = L.circleMarker([site.position.lat, site.position.lon], {
      pane: "markers",
      radius: 9,
      stroke: false,
      fillColor: "#000000",
      fillOpacity: 1,
      interactive: false,
    }).addTo(map);
    const inner = L.circleMarker([site.position.lat, site.position.lon], {
      pane: "markers",
      radius: 8,
      stroke: false,
      fillColor: "#ffffff",
      fillOpacity: 1,
      interactive: false,
    }).addTo(map);
    const marker = L.circleMarker([site.position.lat, site.position.lon], {
      pane: "markers",
      radius: 7,
      stroke: false,
      fillColor: SCENE_CSS_COLOURS.primary,
      fillOpacity: site.published ? 0.9 : 0.65,
      interactive: false,
    }).addTo(map);
    const shimmer = L.circleMarker([site.position.lat, site.position.lon], {
      pane: "markers",
      radius: 7,
      stroke: false,
      fillColor: "#ffffff",
      fillOpacity: 0,
      interactive: false,
      className: "scene-map__site-shimmer",
    }).addTo(map);
    shimmer
      .getElement?.()
      ?.style.setProperty(
        "--scene-map-shimmer-delay",
        `${Math.round((((shimmerScores.get(site.id) ?? minScore) - minScore) / scoreSpan) * SHIMMER_SWEEP_MS)}ms`,
      );
    markers.set(site.id, marker);
    markerRings.set(site.id, { outer, inner, shimmer });
  }
  return { cells, markers, markerRings };
}

export function addVoronoiLinks(L, map, located, cells, markers, markerRings) {
  let regions = [];
  const clear = () => {
    for (const region of regions) {
      const link = region.getElement?.()?.parentElement;
      map.removeLayer(region);
      if (link?.localName === "a") link.remove();
    }
    regions = [];
  };
  const reset = (site) => {
    cells
      .get(site.cells?.neighbourhood?.token)
      ?.fill.setStyle({ fillOpacity: 0.1 });
    markers.get(site.id)?.setRadius(7);
    markerRings?.get(site.id)?.inner.setRadius(8);
    markerRings?.get(site.id)?.outer.setRadius(9);
    markerRings?.get(site.id)?.shimmer.setRadius(7);
  };
  const highlight = (site) => {
    cells
      .get(site.cells?.neighbourhood?.token)
      ?.fill.setStyle({ fillOpacity: 0.38 });
    markers.get(site.id)?.setRadius(9);
    markerRings?.get(site.id)?.inner.setRadius(10);
    markerRings?.get(site.id)?.outer.setRadius(11);
    markerRings?.get(site.id)?.shimmer.setRadius(9);
  };
  const rebuild = () => {
    clear();
    const bounds = map.getPixelBounds();
    const points = located.map((site) => {
      const projected = map.project(
        [site.position.lat, site.position.lon],
        map.getZoom(),
      );
      return { site, x: projected.x, y: projected.y };
    });
    for (const point of points) {
      const polygon = clipVoronoiCell(point, points, bounds);
      if (polygon.length < 3) continue;
      const latLngs = polygon.map(([x, y]) =>
        map.unproject([x, y], map.getZoom()),
      );
      const region = L.polygon(latLngs, {
        pane: "hitRegions",
        stroke: false,
        fill: true,
        fillOpacity: 0.001,
      }).addTo(map);
      region.on("mouseover", () => highlight(point.site));
      region.on("mouseout", () => reset(point.site));
      region.bindTooltip(point.site.title, { sticky: true, direction: "top" });
      wrapPathInLink(
        region,
        point.site.page || `#site-${point.site.id}`,
        point.site.page
          ? `Open ${point.site.title}`
          : `Find ${point.site.title} below`,
      );
      regions.push(region);
    }
  };
  rebuild();
  map.on("zoomend moveend resize", rebuild);
  return { rebuild, regions: () => regions };
}

function scheduleSiteShimmer(markerRings) {
  const paths = [...markerRings.values()]
    .map(({ shimmer }) => shimmer.getElement?.())
    .filter(Boolean);
  if (!paths.length) return;

  let isFirstShimmer = true;
  const shimmer = () => {
    for (const path of paths) path.classList.add("is-shimmering");
    window.setTimeout(() => {
      for (const path of paths) path.classList.remove("is-shimmering");
    }, 1200);
    const nextDelay = isFirstShimmer
      ? SECOND_SHIMMER_DELAY_MS
      : 5000 + Math.random() * 10000;
    isFirstShimmer = false;
    window.setTimeout(shimmer, nextDelay);
  };
  window.setTimeout(shimmer, FIRST_SHIMMER_DELAY_MS);
}

export function initSceneMap(container, sites) {
  const L = window.L;
  if (!L || !container) return null;
  const located = sites.filter((site) => site.position);
  if (!located.length) return null;
  container.textContent = "";
  const map = L.map(container, {
    scrollWheelZoom: false,
    minZoom: MIN_ZOOM,
    maxZoom: MAX_ZOOM,
  }).setView(SF, 13);
  const panes = {};
  for (const [name, zIndex] of Object.entries({
    basemap: 200,
    coastline: 250,
    cellFill: 330,
    grid: 360,
    markers: 430,
    hitRegions: 500,
  })) {
    panes[name] = map.createPane(name);
    panes[name].style.zIndex = String(zIndex);
  }
  addCoastline(L, map, container.dataset.coastlineUrl);
  const { cells, markers, markerRings } = addSiteLayers(L, map, located);
  addVoronoiLinks(L, map, located, cells, markers, markerRings);
  scheduleSiteShimmer(markerRings);
  const bounds = located.map((site) => [site.position.lat, site.position.lon]);
  const frame = () => {
    map.invalidateSize();
    map.fitBounds(bounds, { padding: [30, 30], maxZoom: 15 });
  };
  frame();
  let storage = null;
  try {
    storage = window.localStorage;
  } catch {
    // Consent still works for this visit when browser storage is unavailable.
  }
  mountCycleMapConsent({
    L,
    map,
    modal: document.getElementById("scene-map-consent"),
    loadButton: document.getElementById("scene-map-load"),
    storage,
    onToggle: (enabled) => {
      panes.coastline.style.display = enabled ? "none" : "";
    },
  });
  window.requestAnimationFrame(frame);
  window.addEventListener("load", frame, { once: true });
  return map;
}

if (typeof document !== "undefined") {
  const container = document.getElementById("scene-map");
  const payload = document.getElementById("scene-map-data");
  if (container && payload)
    initSceneMap(container, JSON.parse(payload.textContent));
}
