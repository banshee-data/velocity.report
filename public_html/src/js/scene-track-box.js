import * as THREE from "../vendor/three/three.module.js";
import { toSceneX, toSceneY, toSceneZ } from "./scene-coords.js";

export const TRACK_OUTLINE_OPACITY = 0.95;
export const TRACK_FILL_OPACITY = 0.22;

// Use a complete published box centre, or fall back to the track position.
export function trackBoxAnchor(track) {
  return Number.isFinite(track.bx) && Number.isFinite(track.by)
    ? [track.bx, track.by]
    : [track.x, track.y];
}

export function trackBoxSize(track) {
  // The minimum extent keeps small measurements legible at street scale.
  return [track.l, track.h, track.w].map((extent) =>
    Math.max(extent || 0, 0.9),
  );
}

export function createTrackBoxGeometry() {
  const fill = new THREE.BoxGeometry(1, 1, 1);
  return { fill, edges: new THREE.EdgesGeometry(fill) };
}

export function createTrackBoxMaterials(colour) {
  return {
    edges: new THREE.LineBasicMaterial({
      color: colour,
      transparent: true,
      opacity: TRACK_OUTLINE_OPACITY,
      depthWrite: false,
    }),
    fill: new THREE.MeshBasicMaterial({
      color: colour,
      transparent: true,
      opacity: TRACK_FILL_OPACITY,
      depthWrite: false,
    }),
  };
}

export function placeTrackBox(box, anchor, size, yaw) {
  // The recording's z is the box base. Positive ENU yaw takes +X
  // north, which is -Z after the conversion to Three.js.
  box.position.set(
    toSceneX(anchor[0]),
    toSceneY(anchor[2]) + size[1] / 2,
    toSceneZ(anchor[1]),
  );
  box.scale.set(...size);
  box.rotation.set(0, yaw, 0);
}
