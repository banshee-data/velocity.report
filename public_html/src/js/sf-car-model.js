import * as THREE from "../vendor/three/three.module.js";
import { SCENE_COLOURS } from "./scene-colours.js";

export function makeCar() {
  const car = new THREE.Group();
  const body = new THREE.Mesh(
    new THREE.BoxGeometry(4.4, 0.9, 1.85),
    new THREE.MeshLambertMaterial({ color: SCENE_COLOURS.vehicleShade }),
  );
  body.position.y = 0.7;
  const cab = new THREE.Mesh(
    new THREE.BoxGeometry(2.2, 0.7, 1.65),
    new THREE.MeshLambertMaterial({ color: SCENE_COLOURS.vehicleShade }),
  );
  cab.position.set(-0.3, 1.45, 0);
  const outline = new THREE.LineBasicMaterial({ color: SCENE_COLOURS.vehicle });
  for (const part of [body, cab])
    part.add(
      new THREE.LineSegments(new THREE.EdgesGeometry(part.geometry), outline),
    );
  car.add(body, cab);
  // Rear lamps face the chase camera; a high centre lamp sits on the cabin.
  const brakeMaterial = new THREE.MeshBasicMaterial({ color: 0x661018 });
  for (const z of [-0.62, 0.62]) {
    const lamp = new THREE.Mesh(
      new THREE.BoxGeometry(0.07, 0.24, 0.48),
      brakeMaterial,
    );
    lamp.position.set(-2.23, 0.83, z);
    car.add(lamp);
  }
  const centreLamp = new THREE.Mesh(
    new THREE.BoxGeometry(0.04, 0.1, 0.56),
    brakeMaterial,
  );
  centreLamp.position.set(-1.42, 1.6, 0);
  car.add(centreLamp);
  car.userData.brakeMaterial = brakeMaterial;
  const wheels = [];
  const tyreGeometry = new THREE.CylinderGeometry(0.36, 0.36, 0.22, 12);
  tyreGeometry.rotateX(Math.PI / 2);
  const tyreMaterial = new THREE.MeshLambertMaterial({ color: 0x20262a });
  for (const x of [-1.35, 1.35])
    for (const z of [-0.91, 0.91]) {
      const pivot = new THREE.Group();
      pivot.position.set(x, 0.36, z);
      const tyre = new THREE.Mesh(tyreGeometry, tyreMaterial);
      pivot.add(tyre);
      car.add(pivot);
      wheels.push({ pivot, tyre, front: x > 0 });
    }
  car.userData.wheels = wheels;
  return car;
}

// Ghosts share static geometry and materials; clone the hierarchy without the
// player's wheel references in userData.
export function ghostCarFactory() {
  const prototype = makeCar();
  prototype.userData = {};
  prototype.traverse(({ material }) => {
    if (!material) return;
    material.transparent = true;
    material.opacity = 0.5;
    material.depthWrite = false;
  });
  return () => prototype.clone();
}
