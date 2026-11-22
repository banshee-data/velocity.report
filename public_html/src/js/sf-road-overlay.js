import * as THREE from "../vendor/three/three.module.js";
import { fetchJsonGz, loadRoadOverlay } from "./sf-drive-assets.js";
import { terrainTriangles, tileRegion } from "./sf-drive-surface.js";

const colours = { bridge_deck: 0x20b8e0, ground: 0x70d09c };
import { contactGeometry } from "./sf-road-contact.js";

function dispose(group) {
  for (const mesh of [...group.children]) {
    group.remove(mesh);
    mesh.geometry.dispose();
    for (const material of [mesh.material].flat()) material.dispose();
  }
}

export function mountRoadOverlay({
  canvas,
  form,
  input,
  status,
  terrainToggle,
  viewButtons,
  baseUrl,
}) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
  renderer.setPixelRatio(Math.min(devicePixelRatio, 2));
  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0x182733);
  scene.add(new THREE.HemisphereLight(0xf4f9ff, 0x7e8da2, 2.5));
  const surfaces = new THREE.Group();
  const terrain = new THREE.Group();
  scene.add(surfaces, terrain);
  const camera = new THREE.PerspectiveCamera(45, 1, 0.5, 3500);
  const target = new THREE.Vector3();
  const orbit = new THREE.Spherical(180, 1, -0.85);
  let fitRadius = orbit.radius;
  let request;
  let drag;

  function draw() {
    camera.position.setFromSpherical(orbit).add(target);
    camera.lookAt(target);
    renderer.render(scene, camera);
  }

  new ResizeObserver(() => {
    renderer.setSize(canvas.clientWidth, canvas.clientHeight, false);
    camera.aspect = canvas.clientWidth / Math.max(1, canvas.clientHeight);
    camera.updateProjectionMatrix();
    draw();
  }).observe(canvas);

  canvas.addEventListener("pointerdown", (event) => {
    if (drag || event.button !== 0) return;
    drag = { id: event.pointerId, x: event.clientX, y: event.clientY };
    canvas.setPointerCapture(event.pointerId);
  });
  canvas.addEventListener("pointermove", (event) => {
    if (drag?.id !== event.pointerId) return;
    orbit.theta -= (event.clientX - drag.x) * 0.008;
    orbit.phi -= (event.clientY - drag.y) * 0.008;
    orbit.makeSafe();
    drag.x = event.clientX;
    drag.y = event.clientY;
    draw();
  });
  canvas.addEventListener("lostpointercapture", (event) => {
    if (drag?.id === event.pointerId) drag = undefined;
  });
  function zoom(factor) {
    orbit.radius = THREE.MathUtils.clamp(orbit.radius * factor, 5, 1800);
    draw();
  }
  canvas.addEventListener(
    "wheel",
    (event) => {
      event.preventDefault();
      zoom(event.deltaY < 0 ? 0.9 : 1.11);
    },
    { passive: false },
  );
  for (const button of viewButtons) {
    button.addEventListener("click", () => {
      const view = button.dataset.view;
      if (view === "closer" || view === "farther") {
        zoom(view === "closer" ? 0.75 : 1.33);
        return;
      }
      orbit.set(fitRadius, view === "below" ? 1.62 : 0.85, -0.85);
      draw();
    });
  }
  terrainToggle.addEventListener("change", () => {
    terrain.visible = terrainToggle.checked;
    draw();
  });

  async function show() {
    request?.abort();
    const { signal } = (request = new AbortController());
    dispose(surfaces);
    dispose(terrain);
    draw();
    status.textContent = "Loading road surfaces…";
    try {
      const { overlay, base, tiles } = await loadRoadOverlay(
        new URL(input.value),
        baseUrl,
        signal,
      );
      const context = await Promise.all(
        base.tiles
          .filter(({ t }) => tiles.has(t))
          .map(async (tile) => {
            const data = await fetchJsonGz(
              new URL(`${tile.t}.json.gz`, baseUrl),
              { signal },
            );
            return terrainTriangles(data, tileRegion(tile, base.tiles))
              .positions;
          }),
      );
      if (signal.aborted) return;
      const seen = new Set();
      for (const tile of tiles.values()) {
        for (const surface of tile.surfaces) {
          if (!surface.contact || seen.has(surface.id)) continue;
          const geometry = contactGeometry(surface.contact);
          if (!geometry.getAttribute("position").count) {
            geometry.dispose();
            continue;
          }
          seen.add(surface.id);
          const material = [colours[surface.kind] ?? 0xbb9af5, 0xf6b94b].map(
            (color) =>
              new THREE.MeshLambertMaterial({ color, side: THREE.DoubleSide }),
          );
          surfaces.add(new THREE.Mesh(geometry, material));
        }
      }
      if (!seen.size)
        throw new Error("This revision has no supported contact surfaces.");
      for (const positions of context) {
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute(
          "position",
          new THREE.Float32BufferAttribute(positions, 3),
        );
        terrain.add(
          new THREE.Mesh(
            geometry,
            new THREE.MeshBasicMaterial({
              color: 0x7d96a3,
              opacity: 0.32,
              transparent: true,
              depthWrite: false,
            }),
          ),
        );
      }
      const bounds = new THREE.Box3().setFromObject(surfaces);
      bounds.getCenter(target);
      fitRadius = 1.65 * bounds.getSize(new THREE.Vector3()).length();
      orbit.set(fitRadius, 0.85, -0.85);
      status.textContent = `${seen.size} contact surfaces · ${overlay.pilot} · ${overlay.product_hash.slice(0, 12)}`;
      draw();
    } catch (error) {
      if (signal.aborted) return;
      dispose(surfaces);
      dispose(terrain);
      draw();
      status.textContent =
        error instanceof TypeError
          ? "Could not load the manifest or tiles. Check the URL, connection and browser access."
          : error.message;
    }
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    show();
  });
  if (input.value) show();
}
