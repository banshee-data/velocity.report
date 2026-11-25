import * as THREE from "../vendor/three/three.core.js";
import { treeCuts } from "./sf-tree-clearance.js";
import { treeModel } from "./sf-mesh-steps.js";
import {
  treeRadius,
  treeFallAngle,
  treeFallProgress,
} from "./sf-tree-damage.js";

// Keep intact trees instanced. Only trees which could intersect pavement
// enter the clip queue. Small batches bound both transfers and GPU uploads.
// Group nearby rows so a road change replaces one bounded GPU buffer.
const batchSize = 256;
export function addTrees(grp, tile, gz, C, reveal, deckIndex, queue) {
  if (!tile.tr.length) return;
  const [ox, oy] = tile.o;
  // Keep GPU instance draws local to the camera. Zero-scale hidden rows
  // still run vertex shaders when their entire tile is one visible draw.
  const models = [treeModel(false), treeModel(true)];
  const materials = [C.tree, C.trunk].map((color) => {
    const material = new THREE.MeshLambertMaterial({ color });
    reveal.enableOn(material);
    return material;
  });
  const sectors = new Map(),
    slots = [];
  for (const [i, [x, y]] of tile.tr.entries()) {
    const key = Math.floor(x / 1600) + "," + Math.floor(y / 1600); // 160 m in tile decimetres
    let sector = sectors.get(key);
    if (!sector) {
      sector = { count: 0, bounds: new THREE.Box3() };
      sectors.set(key, sector);
    }
    slots[i] = { sector, row: sector.count++ };
  }
  let first = 0;
  for (const sector of sectors.values()) {
    sector.first = first;
    first += sector.count;
    sector.instances = models.map(
      (geometry, k) =>
        new THREE.InstancedMesh(geometry, materials[k], sector.count),
    );
    grp.add(...sector.instances);
  }
  function instanceAt(i, k, matrix) {
    const { sector, row } = slots[i],
      mesh = sector.instances[k];
    mesh.setMatrixAt(row, matrix);
    mesh.instanceMatrix.needsUpdate = true;
  }
  const mtx = new THREE.Matrix4(),
    qn = new THREE.Quaternion(),
    prior = new THREE.Quaternion();
  const s = new THREE.Vector3(),
    p = new THREE.Vector3(),
    axis = new THREE.Vector3(),
    base = new THREE.Vector3();
  const hiddenMatrix = new THREE.Matrix4().makeScale(0, 0, 0);
  const poses = new Map(),
    groups = new Map(),
    dirty = new Set();

  function discard(mesh) {
    if (!mesh) return;
    grp.remove(mesh);
    mesh.geometry.dispose();
  }

  function groupFor(k, i) {
    const { sector, row } = slots[i];
    const key = Math.floor((sector.first + row) / batchSize) * 2 + k;
    let group = groups.get(key);
    if (group) return group;
    group = {
      rows: new Map(),
      mesh: null,
      prepare() {
        const cutIds = new Map(),
          planes = [],
          rows = [];
        for (const [index, row] of group.rows) {
          const ids = [];
          for (const cut of row.cuts) {
            if (!cutIds.has(cut)) {
              cutIds.set(cut, planes.length);
              planes.push(
                cut.planes.map(({ normal: { x, y, z }, constant }) => [
                  x,
                  y,
                  z,
                  constant,
                ]),
              );
            }
            ids.push(cutIds.get(cut));
          }
          rows.push({ index, ids, transform: row.matrix.elements });
        }
        return { trunk: k === 1, rows, planes };
      },
      fail(error) {
        console.warn("Tree mesh batch failed", error);
      },
      apply({ position, normal, index, sphere, uncut }) {
        for (const i of uncut) instanceAt(i, k, group.rows.get(i).matrix);
        if (!position.length) return;
        const geometry = new THREE.BufferGeometry();
        geometry.setAttribute(
          "position",
          new THREE.BufferAttribute(position, 3),
        );
        geometry.setAttribute(
          "normal",
          new THREE.BufferAttribute(normal, 3, true),
        );
        geometry.setIndex(new THREE.BufferAttribute(index, 1));
        geometry.boundingSphere = new THREE.Sphere(
          new THREE.Vector3().fromArray(sphere),
          sphere[3],
        );
        group.mesh = new THREE.Mesh(geometry, materials[k]);
        grp.add(group.mesh);
      },
    };
    groups.set(key, group);
    return group;
  }
  function flushGroups() {
    for (const group of dirty) {
      discard(group.mesh);
      group.mesh = null;
      if (group.rows.size) queue.update(group);
      else queue.cancel(group);
    }
    dirty.clear();
  }
  function setPart(k, i, animated, refresh) {
    const group = groupFor(k, i);
    // Falling trees stay instanced during the animation. Once at rest,
    // their final geometry goes through the same queued cuts as upright trees.
    const cuts = animated ? null : treeCuts(models[k], mtx, base.y, deckIndex);
    const previous = group.rows.get(i);
    if (refresh && (animated || (!previous && !cuts?.size))) return;
    if (!cuts?.size) {
      if (group.rows.delete(i)) dirty.add(group);
      instanceAt(i, k, mtx);
      return;
    }
    if (
      refresh &&
      previous &&
      previous.cuts.size === cuts.size &&
      [...cuts].every((cut) => previous.cuts.has(cut))
    )
      return;
    group.rows.set(i, { matrix: mtx.clone(), cuts });
    dirty.add(group);
    instanceAt(i, k, hiddenMatrix);
  }
  function setTreePose(i, event, elapsed = 1, refresh = false) {
    const [x, y, h, r] = tile.tr[i];
    base.set(ox + x / 10, gz(x / 10, y / 10), -oy - y / 10);
    const animated = event && treeFallProgress(elapsed) < 1;
    qn.identity();
    if (event) {
      // Rotate the trunk and canopy about the same ground anchor, away from impact.
      axis.set(-event.north, 0, -event.east).normalize();
      if (event.priorLean) {
        qn.setFromAxisAngle(axis, treeFallAngle(event, 1));
        const [east, north] = event.priorLean;
        axis.set(-north, 0, -east).normalize();
        prior.setFromAxisAngle(axis, treeFallAngle({ stage: "cracked" }, 1));
        qn.copy(prior.slerp(qn, treeFallProgress(elapsed)));
      } else {
        qn.setFromAxisAngle(axis, treeFallAngle(event, elapsed));
      }
    }
    const ch = Math.max(1.5, Math.min(h * 0.6, 2.2 * r));
    p.set(0, h - ch / 2, 0)
      .applyQuaternion(qn)
      .add(base);
    s.set(r, ch / 2, r);
    mtx.compose(p, qn, s);
    setPart(0, i, animated, refresh);
    const tw = treeRadius(h);
    p.set(0, (h - ch) / 2, 0)
      .applyQuaternion(qn)
      .add(base);
    s.set(tw, Math.max(0.5, h - ch), tw);
    mtx.compose(p, qn, s);
    setPart(1, i, animated, refresh);
  }

  tile.tr.forEach(([, , h, r], i) => {
    setTreePose(i);
    const reach = h + r + 1,
      bounds = slots[i].sector.bounds;
    bounds.expandByPoint(p.copy(base).addScalar(reach));
    bounds.expandByPoint(p.copy(base).addScalar(-reach));
  });
  // Include future poses even when a batch is empty for its first render.
  for (const sector of sectors.values())
    for (const instance of sector.instances)
      instance.boundingSphere = sector.bounds.getBoundingSphere(
        new THREE.Sphere(),
      );
  flushGroups();
  grp.userData.drawTree = (i, event, seconds) => {
    if (!tile.tr[i]) return;
    if (event) poses.set(i, [event, seconds]);
    else poses.delete(i);
    setTreePose(i, event, seconds);
    flushGroups();
  };
  grp.userData.refreshTreeCuts = (bounds) => {
    tile.tr.forEach(([x, y, h, r], i) => {
      const east = ox + x / 10,
        south = -oy - y / 10;
      const extent = poses.get(i)?.[0] ? h + r : r;
      if (
        !bounds.some(
          (b) =>
            east + extent >= b.min.x &&
            east - extent <= b.max.x &&
            south + extent >= b.min.z &&
            south - extent <= b.max.z,
        )
      )
        return;
      const [event, seconds] = poses.get(i) || [];
      setTreePose(i, event, seconds, true);
    });
    flushGroups();
  };

  grp.userData.cancelTreeCuts = () => {
    for (const group of groups.values()) queue.cancel(group);
  };
}
